package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	RoleAdmin        = 0
	RoleNormal       = 1
	RoleWhitelist    = 2
	RoleUnrecognized = -1
)

type Store struct {
	mu    sync.RWMutex
	db    *sql.DB
	state State

	// usernameMap / emailMap / verifiedEmailMap 是用户身份字段的二级索引，
	// 让登录、注册冲突检查、密码找回等高频路径避免全量扫描 Users。
	usernameMap      map[string]int64
	emailMap         map[string]int64
	verifiedEmailMap map[string]int64
	// telegramIDMap 是 telegram_id → UID 的二级索引，避免每次都全表扫描 Users。
	// 在 Open/OpenPostgres 时重建，后续通过 maintainTelegramIDIndex 增量维护。
	telegramIDMap map[int64]int64
	// embyIDMap 是 emby_id → UID 的二级索引，服务 Emby webhook / 会话 / 管理操作等高频反查。
	embyIDMap map[string]int64
	// userUIDs 按 UID 升序保存现有用户 ID，让 ListUsers 避免每次读都排序 Users map。
	userUIDs []int64

	// apiKeyHashMap（hash → APIKey.ID）/ legacyAPIKeyHashMap（hash → UID）是 API Key
	// 鉴权热路径的二级索引。FindAPIKeyByHash 原先要全表扫 APIKeys + 全表扫 Users（找
	// 遗留 key），非法 key（攻击者拿垃圾串狂打）每次都吃满 O(键数+用户数) 且全程持读锁，
	// 是可被放大的 DoS 面。索引化后命中/未命中都是 O(1)。安全不变量：命中后仍回 s.state
	// 复核并对 hash 做常量时间比对——索引陈旧至多导致「假未命中」（功能性、下轮 refresh
	// 自愈），绝不会「假命中」放行已删除/轮换的 key。随 rebuildUserIndexes 在每个 refresh
	// /回滚点重建，并在 CreateAPIKey / DeleteAPIKey / UpdateUser(遗留 key) 处增量维护。
	apiKeyHashMap       map[string]int64
	legacyAPIKeyHashMap map[string]int64

	// stateVersion 是 Postgres 乐观并发版本号，与 twilight_state.version 列对应。
	// refreshLocked 读 state 时一并读回 version 存于此；saveLocked 走「version = 期望值」
	// 守卫的 UPSERT 并把 version+1 RETURNING 回来刷新本字段。多个 Twilight 进程
	// （api / bot / scheduler）各持独立 Store 与 s.mu，跨进程唯一的串行点就是这一列：
	// 谁的期望版本落后就 UPSERT 命中 0 行、拿到 errStateVersionConflict 回滚重试，
	// 取代此前「refresh(SELECT) 与 save(盲写整份 jsonb) 之间被他进程插入提交后仍整份覆盖」
	// 的丢更新——正是「用户建了工单、TG 也收到了，工单却随后凭空消失」的根因。
	stateVersion int64

	// stateRaw 是与 stateVersion 对应的最近一次权威 JSONB 字节。refreshLocked 在版本
	// 未变化时让 PostgreSQL 只返回 NULL state，跳过大对象网络传输、反序列化和索引重建；
	// snapshotStateLocked 复用 stateRaw 作失败回滚快照，save 成功后再原子替换为新字节。
	// 该切片只读持有，不与 s.state 的可变 map/slice 共享底层对象。
	stateRaw []byte

	// API Key 调用统计（RequestCount / LastUsed）是纯展示字段，不参与任何鉴权判定，
	// 却处在每个 API Key 请求的认证热路径上。此前每请求都为自增计数走一遍 mutateAndSaveLocked
	// （全表 SELECT+unmarshal+重建索引 + 双份全量 marshal + jsonb UPSERT，全程持写锁），
	// 是高频调用下的主要 CPU/IO 开销。改为：热路径只在 apiKeyUsageMu 下累加内存增量，
	// 由后台协程按 apiKeyUsageFlushInterval 批量落盘一次；Close 时做最后一次 flush。
	// 崩溃至多丢一个刷新周期的计数（可接受，非关键数据）。
	apiKeyUsageMu   sync.Mutex
	apiKeyUsage     map[int64]apiKeyUsageDelta
	apiKeyFlushStop chan struct{}
	apiKeyFlushDone chan struct{}
	apiKeyFlushOnce sync.Once

	// telegramRosterCache absorbs repeated ordinary group-message observations.
	// Roster history itself lives in PostgreSQL; keeping only a bounded hot set
	// avoids both one SQL write per message and an unbounded in-process mirror.
	telegramRosterCacheMu  sync.Mutex
	telegramRosterCache    map[telegramRosterCacheKey]telegramRosterCacheEntry
	telegramRosterCacheSeq uint64
}

// apiKeyUsageDelta 是单个 API Key 在一个刷新周期内累积的调用增量。
type apiKeyUsageDelta struct {
	count    int64
	lastUsed int64
}

const apiKeyUsageFlushInterval = 30 * time.Second

const (
	BackendJSON     = "json"
	BackendPostgres = "postgres"
)

type BackupInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
	Note      string `json:"note,omitempty"`
}

type PostgresTargetStatus struct {
	Host            string `json:"host,omitempty"`
	User            string `json:"user,omitempty"`
	Database        string `json:"database,omitempty"`
	Connected       bool   `json:"connected"`
	DatabaseCreated bool   `json:"database_created"`
	SchemaReady     bool   `json:"schema_ready"`
}

type State struct {
	NextUserID              int64                                  `json:"next_user_id"`
	NextAPIKeyID            int64                                  `json:"next_api_key_id"`
	NextRequestID           int64                                  `json:"next_request_id"`
	NextAnnouncementID      int64                                  `json:"next_announcement_id"`
	NextLoginLogID          int64                                  `json:"next_login_log_id"`
	NextRuntimeLogID        int64                                  `json:"next_runtime_log_id"`
	NextSchedulerRunID      int64                                  `json:"next_scheduler_run_id"`
	NextRebindRequestID     int64                                  `json:"next_rebind_request_id"`
	NextViolationLogID      int64                                  `json:"next_violation_log_id"`
	NextAuditLogID          int64                                  `json:"next_audit_log_id"`
	NextBangumiSyncLogID    int64                                  `json:"next_bangumi_sync_log_id"`
	NextTicketID            int64                                  `json:"next_ticket_id"`
	NextDeveloperJSPresetID int64                                  `json:"next_developer_js_preset_id"`
	Users                   map[int64]User                         `json:"users"`
	APIKeys                 map[int64]APIKey                       `json:"api_keys"`
	MediaRequests           map[int64]MediaRequest                 `json:"media_requests"`
	Announcements           map[int64]Announcement                 `json:"announcements"`
	InviteCodes             map[string]InviteCode                  `json:"invite_codes"`
	InviteRelations         map[int64]InviteRelation               `json:"invite_relations"`
	RegCodes                map[string]RegCode                     `json:"regcodes"`
	BindCodes               map[string]BindCode                    `json:"bind_codes"`
	EmailVerifications      map[string]EmailVerification           `json:"email_verifications"`
	Signin                  map[int64]Signin                       `json:"signin"`
	SchedulerRuns           []SchedulerRun                         `json:"scheduler_runs"`
	SchedulerSchedules      map[string]SchedulerSchedule           `json:"scheduler_schedules"`
	Devices                 map[string]Device                      `json:"devices"`
	LoginLogs               []LoginLog                             `json:"login_logs"`
	RuntimeLogs             []RuntimeLogEntry                      `json:"runtime_logs"`
	IPBlacklist             map[string]IPBlacklistEntry            `json:"ip_blacklist"`
	PlaybackRecords         []PlaybackRecord                       `json:"playback_records"`
	PlaybackSessions        []PlaybackSession                      `json:"playback_sessions,omitempty"`
	EmbyActivityLogs        []EmbyActivityLog                      `json:"emby_activity_logs,omitempty"`
	NextEmbyActivityLogID   int64                                  `json:"next_emby_activity_log_id"`
	RebindRequests          map[int64]RebindRequest                `json:"rebind_requests"`
	TelegramRoster          map[string]TelegramRosterEntry         `json:"telegram_roster,omitempty"`
	ViolationLogs           []ViolationLog                         `json:"violation_logs"`
	AuditLogs               []AuditLog                             `json:"audit_logs"`
	BangumiSyncLogs         []BangumiSyncLog                       `json:"bangumi_sync_logs"`
	BangumiCollectionCache  map[string]BangumiCollectionCacheEntry `json:"bangumi_collection_cache,omitempty"`
	BangumiSubjectCache     map[string]BangumiSubjectCacheEntry    `json:"bangumi_subject_cache,omitempty"`
	Tickets                 map[int64]Ticket                       `json:"tickets"`
	TicketTypes             []string                               `json:"ticket_types,omitempty"`
	DeveloperJSPresets      map[int64]DeveloperJSPreset            `json:"developer_js_presets,omitempty"`
	BangumiRegcodeClaims    map[string]BangumiRegcodeClaim         `json:"bangumi_regcode_claims,omitempty"`
	DeveloperModeEnabled    bool                                   `json:"developer_mode_enabled,omitempty"`
	// TelegramBotOffset 是旧 JSONB 版本的 getUpdates 游标。PostgreSQL 启动时
	// 仅用它播种独立的 twilight_telegram_runtime 行，运行期不再改写此字段。
	TelegramBotOffset int64 `json:"telegram_bot_offset,omitempty"`
}

type User struct {
	UID              int64  `json:"uid"`
	Username         string `json:"username"`
	Email            string `json:"email,omitempty"`
	EmailVerified    bool   `json:"email_verified,omitempty"`
	EmailVerifiedAt  int64  `json:"email_verified_at,omitempty"`
	TelegramID       int64  `json:"telegram_id,omitempty"`
	TelegramUsername string `json:"telegram_username,omitempty"`
	Role             int    `json:"role"`
	Active           bool   `json:"active"`
	ExpiredAt        int64  `json:"expired_at"`
	EmbyID           string `json:"emby_id,omitempty"`
	EmbyUsername     string `json:"emby_username,omitempty"`
	// EmbyDisabled 是远端 Emby 账号「当前是否被禁用」的尽力镜像（true=已禁用）。
	// 由每次启停 Emby 时回写、并在强制刷新时按远端真值校正。让用户列表无需逐行
	// 查 Emby 即可区分「Web 正常但 Emby 被单独禁用」。仅在 EmbyID 非空时有意义。
	EmbyDisabled                            bool     `json:"emby_disabled"`
	Avatar                                  string   `json:"avatar,omitempty"`
	Background                              string   `json:"background,omitempty"`
	BGMMode                                 bool     `json:"bgm_mode"`
	BGMManageMode                           bool     `json:"bgm_manage_mode"`
	BGMToken                                string   `json:"bgm_token,omitempty"`
	CreatedAt                               int64    `json:"created_at"`
	RegisterTime                            int64    `json:"register_time"`
	EmbyGrantLocked                         bool     `json:"emby_grant_locked"`
	RegistrationSource                      string   `json:"registration_source,omitempty"`
	RegistrationCode                        string   `json:"registration_code,omitempty"`
	PendingEmby                             bool     `json:"pending_emby"`
	PendingEmbyDays                         *int     `json:"pending_emby_days,omitempty"`
	NotifyOnLoginTelegram                   bool     `json:"notify_on_login_telegram,omitempty"`
	NotifyOnLoginEmail                      bool     `json:"notify_on_login_email,omitempty"`
	NotifyOnTicketTelegram                  bool     `json:"notify_on_ticket_telegram,omitempty"`
	SigninAutoRenewal                       bool     `json:"signin_auto_renewal,omitempty"`
	RequireEmailForPasswordChange           bool     `json:"require_email_for_password_change,omitempty"`
	RequireEmailForEmbyPasswordChange       bool     `json:"require_email_for_emby_password_change,omitempty"`
	RequireOldPasswordForEmbyPasswordChange bool     `json:"require_old_password_for_emby_password_change,omitempty"`
	LegacyAPIKeyHash                        string   `json:"legacy_api_key_hash,omitempty"`
	LegacyAPIKeyPrefix                      string   `json:"legacy_api_key_prefix,omitempty"`
	LegacyAPIKeySuffix                      string   `json:"legacy_api_key_suffix,omitempty"`
	LegacyAPIKeyStatus                      bool     `json:"legacy_api_key_status"`
	LegacyPermissions                       []string `json:"legacy_permissions,omitempty"`
	PasswordHash                            string   `json:"password_hash"`
	RebindingInProgress                     bool     `json:"rebinding_in_progress"`
	RebindingSince                          int64    `json:"rebinding_since,omitempty"`
	SeenAnnouncementIDs                     []int64  `json:"seen_announcement_ids,omitempty"`
}

type UserSummaryCounts struct {
	Total         int
	Active        int
	Admins        int
	TelegramBound int
	EmbyBound     int
	PendingEmby   int
	EmailBound    int
	EmailVerified int
}

type APIKey struct {
	ID           int64    `json:"id"`
	UID          int64    `json:"uid"`
	Name         string   `json:"name"`
	Hash         string   `json:"hash"`
	Prefix       string   `json:"key_prefix"`
	Suffix       string   `json:"key_suffix"`
	Enabled      bool     `json:"enabled"`
	AllowQuery   bool     `json:"allow_query"`
	Permissions  []string `json:"permissions"`
	RateLimit    int      `json:"rate_limit"`
	RequestCount int64    `json:"request_count"`
	LastUsed     int64    `json:"last_used"`
	CreatedAt    int64    `json:"created_at"`
	ExpiredAt    int64    `json:"expired_at,omitempty"`
}

type MediaRequest struct {
	ID            int64          `json:"id"`
	Revision      int64          `json:"revision,omitempty"`
	RequireKey    string         `json:"require_key"`
	UID           int64          `json:"uid"`
	TelegramID    int64          `json:"telegram_id,omitempty"`
	Username      string         `json:"username,omitempty"`
	Title         string         `json:"title"`
	OriginalTitle string         `json:"original_title,omitempty"`
	Source        string         `json:"source"`
	MediaID       int64          `json:"media_id"`
	MediaType     string         `json:"media_type"`
	Season        int            `json:"season,omitempty"`
	Year          string         `json:"year,omitempty"`
	Status        string         `json:"status"`
	AdminNote     string         `json:"admin_note,omitempty"`
	Note          string         `json:"note,omitempty"`
	MediaInfo     map[string]any `json:"media_info,omitempty"`
	CreatedAt     int64          `json:"created_at"`
	UpdatedAt     int64          `json:"updated_at"`
}

type Announcement struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	Visible          bool   `json:"visible"`
	Level            string `json:"level"`
	RenderMode       string `json:"render_mode,omitempty"`
	Pinned           bool   `json:"pinned"`
	ForceRead        bool   `json:"force_read"`
	ForceReadSeconds int    `json:"force_read_seconds,omitempty"`
	CreatedByUID     int64  `json:"created_by_uid,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	ExpiredAt        int64  `json:"expired_at,omitempty"`
}

type DeveloperJSPreset struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Code        string `json:"code"`
	CreatorUID  int64  `json:"creator_uid,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// BangumiRegcodeClaim 记录一个 Bangumi 账号（以其数值 id 为唯一键）已经通过自助
// 发码通道领取过的注册码。它是「一个 Bangumi 账号永远只能领一次」这一全局不变量的
// 唯一持久化依据：自助发码在生成码之前先查此表，命中即复用旧码而绝不再生成。
// 键为 Bangumi 数值 id 的字符串形式（由服务端持 Token 调 /me 回填，不信任 JS 传入值）。
type BangumiRegcodeClaim struct {
	BangumiID       string `json:"bangumi_id"`
	BangumiUsername string `json:"bangumi_username,omitempty"`
	Code            string `json:"code"`
	UID             int64  `json:"uid,omitempty"`
	Username        string `json:"username,omitempty"`
	TelegramID      int64  `json:"telegram_id,omitempty"`
	Days            int    `json:"days,omitempty"`
	Source          string `json:"source,omitempty"`
	ClaimedAt       int64  `json:"claimed_at"`
}

type InviteCode struct {
	Code           string `json:"code"`
	UID            int64  `json:"uid"`
	InviterUID     int64  `json:"inviter_uid"`
	Days           int    `json:"days"`
	UseCountLimit  int    `json:"use_count_limit"`
	UseCount       int    `json:"use_count"`
	UsedByUID      int64  `json:"used_by_uid,omitempty"`
	UsedAt         int64  `json:"used_at,omitempty"`
	Active         bool   `json:"active"`
	Note           string `json:"note,omitempty"`
	Used           bool   `json:"used"`
	TargetUsername string `json:"target_username,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	ExpiredAt      int64  `json:"expired_at,omitempty"`
}

type RegCode struct {
	Code                   string  `json:"code"`
	Type                   int     `json:"type"`
	ValidityTime           int64   `json:"validity_time"`
	Days                   int     `json:"days"`
	Note                   string  `json:"note,omitempty"`
	UseCountLimit          int     `json:"use_count_limit"`
	UseCount               int     `json:"use_count"`
	UsedBy                 int64   `json:"used_by,omitempty"`
	UsedByUIDs             []int64 `json:"used_by_uids,omitempty"`
	UsedByTelegramIDs      []int64 `json:"used_by_telegram_ids,omitempty"`
	Active                 bool    `json:"active"`
	IsDecoy                bool    `json:"is_decoy"`
	TargetUsername         string  `json:"target_username,omitempty"`
	TargetTelegramUsername string  `json:"target_telegram_username,omitempty"`
	TargetTelegramID       int64   `json:"target_telegram_id,omitempty"`
	TargetUID              int64   `json:"target_uid,omitempty"`
	CreatedAt              int64   `json:"created_at"`
	CreatedTime            int64   `json:"created_time"`
	ExpiredAt              int64   `json:"expired_at,omitempty"`
	// PausedSeconds 累计暂停时长（秒），停用期间暂停计算有效期。
	PausedSeconds int64 `json:"paused_seconds,omitempty"`
	// PauseStart 当前暂停起始时间戳（秒），0 表示未处于暂停状态。
	PauseStart int64 `json:"pause_start,omitempty"`
	// Source 区分卡码来源："admin" 管理员手动创建、"invite" 邀请系统自动生成。
	// 历史数据该字段为空字符串，视作 "admin"。
	Source string `json:"source,omitempty"`
	// CreatorUID 记录创建者 UID。管理员创建时为管理员 UID，邀请续期码为邀请人 UID。
	CreatorUID int64 `json:"creator_uid,omitempty"`
}

type InviteRelation struct {
	ParentUID int64  `json:"parent_uid"`
	ChildUID  int64  `json:"child_uid"`
	Code      string `json:"code"`
	CreatedAt int64  `json:"created_at"`
}

type BindCode struct {
	Code             string `json:"code"`
	Scene            string `json:"scene"`
	UID              int64  `json:"uid,omitempty"`
	Confirmed        bool   `json:"confirmed"`
	TelegramID       int64  `json:"telegram_id,omitempty"`
	TelegramUsername string `json:"telegram_username,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	ExpiresAt        int64  `json:"expires_at"`
}

// ViolationLog records attempts to use decoy codes or codes restricted to
// a specific username by an unauthorized user.
type ViolationLog struct {
	ID         int64  `json:"id"`
	UID        int64  `json:"uid"`
	Username   string `json:"username"`
	Code       string `json:"code"`
	CodeType   string `json:"code_type"`
	Reason     string `json:"reason"`
	Action     string `json:"action"`
	IP         string `json:"ip,omitempty"`
	TelegramID int64  `json:"telegram_id,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

// AuditLog 记录用户和管理员的关键操作，用于安全审计和运营追溯。
type AuditLog struct {
	ID        int64          `json:"id"`
	UID       int64          `json:"uid"`                  // 操作者 UID
	Username  string         `json:"username"`             // 操作者用户名（快照）
	Action    string         `json:"action"`               // 操作动作，如 "create_regcode"、"disable_user"
	Category  string         `json:"category"`             // 分类：admin / user / system
	Source    string         `json:"source,omitempty"`     // 来源：http / telegram / scheduler / system
	Method    string         `json:"method,omitempty"`     // HTTP 方法；非 HTTP 来源为空
	TargetUID int64          `json:"target_uid,omitempty"` // 被操作对象 UID（如有）
	Detail    map[string]any `json:"detail,omitempty"`     // 操作详情（结构化）
	IP        string         `json:"ip,omitempty"`         // 操作者 IP
	CreatedAt int64          `json:"created_at"`
}

type RuntimeLogEntry struct {
	ID      int64             `json:"id"`
	Time    int64             `json:"time"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}

type Signin struct {
	UID           int64          `json:"uid"`
	Points        int            `json:"points"`
	Streak        int            `json:"streak"`
	LongestStreak int            `json:"longest_streak,omitempty"`
	LastSignin    string         `json:"last_signin"`
	Records       []SigninRecord `json:"records"`
}

type SigninRecord struct {
	Date        string `json:"date"`
	Points      int    `json:"points"`
	BonusPoints int    `json:"bonus_points,omitempty"`
	Total       int    `json:"total,omitempty"`
	Streak      int    `json:"streak,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

type SchedulerRun struct {
	ID         int64          `json:"id"`
	JobID      string         `json:"job_id"`
	Type       string         `json:"type"`
	Trigger    string         `json:"trigger"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	Summary    map[string]any `json:"summary,omitempty"`
	Logs       []string       `json:"logs,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  int64          `json:"started_at"`
	FinishedAt int64          `json:"finished_at,omitempty"`
	EndedAt    int64          `json:"ended_at"`
}

type SchedulerSchedule struct {
	JobID         string         `json:"job_id"`
	TriggerSpec   map[string]any `json:"trigger_spec"`
	RuntimeParams map[string]any `json:"runtime_params,omitempty"`
	IsCustom      bool           `json:"is_custom"`
	UpdatedAt     int64          `json:"updated_at"`
}

type Device struct {
	UID           int64  `json:"uid"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	Client        string `json:"client"`
	ClientVersion string `json:"client_version,omitempty"`
	LastIP        string `json:"last_ip,omitempty"`
	FirstSeen     int64  `json:"first_seen"`
	LastSeen      int64  `json:"last_seen"`
	Trusted       bool   `json:"is_trusted"`
	Blocked       bool   `json:"is_blocked"`
}

type LoginLog struct {
	ID         int64  `json:"id"`
	UID        int64  `json:"uid"`
	IP         string `json:"ip"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device"`
	Client     string `json:"client"`
	Time       int64  `json:"time"`
	Blocked    bool   `json:"blocked"`
	Country    string `json:"country,omitempty"`
	City       string `json:"city,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type IPBlacklistEntry struct {
	IP        string `json:"ip"`
	Reason    string `json:"reason"`
	CreatedAt int64  `json:"created_at"`
	ExpireAt  int64  `json:"expire_at"`
}

type PlaybackRecord struct {
	UID         int64  `json:"uid"`
	ItemID      string `json:"item_id"`
	Title       string `json:"title"`
	SeriesName  string `json:"series_name,omitempty"`
	MediaType   string `json:"media_type"`
	IndexNumber int    `json:"index_number,omitempty"`
	Duration    int64  `json:"duration"`
	PlayedAt    int64  `json:"played_at"`
}

type PlaybackSession struct {
	UID       int64  `json:"uid"`
	ItemID    string `json:"item_id"`
	ItemName  string `json:"item_name"`
	MediaType string `json:"media_type"`
	SessionID string `json:"session_id"`
	StartAt   int64  `json:"start_at"`
	EndAt     int64  `json:"end_at,omitempty"`
	Duration  int64  `json:"duration,omitempty"`
}

type EmbyActivityLog struct {
	ID        int64  `json:"id"`
	EmbyLogID int64  `json:"emby_log_id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	ItemID    string `json:"item_id,omitempty"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	Overview  string `json:"overview,omitempty"`
	Date      int64  `json:"date"`
	CreatedAt int64  `json:"created_at"`
}

type BangumiSyncLog struct {
	ID           int64  `json:"id"`
	UID          int64  `json:"uid"`
	RecordItemID string `json:"record_item_id"`
	SubjectID    string `json:"subject_id,omitempty"`
	SubjectName  string `json:"subject_name,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

type BangumiCollectionCacheEntry struct {
	UID      int64  `json:"uid"`
	Username string `json:"username,omitempty"`
	Type     int    `json:"type"`
	// Entries stores user-scoped collection data only. Subject details are
	// de-duplicated in State.BangumiSubjectCache and hydrated on read.
	Entries     []map[string]any `json:"entries"`
	Total       int              `json:"total"`
	UpdatedAt   int64            `json:"updated_at"`
	ExpiresAt   int64            `json:"expires_at,omitempty"`
	LastError   string           `json:"last_error,omitempty"`
	LastErrorAt int64            `json:"last_error_at,omitempty"`
}

type BangumiSubjectCacheEntry struct {
	SubjectID int64          `json:"subject_id"`
	Subject   map[string]any `json:"subject"`
	UpdatedAt int64          `json:"updated_at"`
	ExpiresAt int64          `json:"expires_at,omitempty"`
}

type Ticket struct {
	ID             int64              `json:"id"`
	UID            int64              `json:"uid"`
	Username       string             `json:"username"`
	Title          string             `json:"title"`
	Content        string             `json:"content"`
	Type           string             `json:"type"`
	Status         string             `json:"status"`
	Priority       string             `json:"priority"`
	AdminNote      string             `json:"admin_note,omitempty"`
	Replies        []TicketReply      `json:"replies,omitempty"`
	Attachments    []TicketAttachment `json:"attachments,omitempty"`
	NotifyTelegram *bool              `json:"notify_telegram,omitempty"`
	CreatedAt      int64              `json:"created_at"`
	UpdatedAt      int64              `json:"updated_at"`
	ResolvedAt     int64              `json:"resolved_at,omitempty"`
	ClosedAt       int64              `json:"closed_at,omitempty"`
}

type TicketReply struct {
	UID       int64  `json:"uid"`
	Username  string `json:"username"`
	Role      int    `json:"role"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// TicketAttachment 描述挂在工单上的一张交流图片。文件按工单 ID 存放在
// uploads/tickets/<ticket_id>/<filename>，这里只持久化元数据。
type TicketAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	UploadedUID int64  `json:"uploaded_uid"`
	CreatedAt   int64  `json:"created_at"`
}

const (
	TicketStatusOpen       = "open"
	TicketStatusInProgress = "in_progress"
	TicketStatusResolved   = "resolved"
	TicketStatusClosed     = "closed"

	TicketPriorityLow    = "low"
	TicketPriorityMedium = "medium"
	TicketPriorityHigh   = "high"
	TicketPriorityUrgent = "urgent"

	TicketTypeDefault = "all"
)

type TicketUpdate struct {
	Status    *string
	Priority  *string
	Type      *string
	AdminNote *string
	Reply     *TicketReply
}

type RebindRequest struct {
	ID            int64  `json:"id"`
	UID           int64  `json:"uid"`
	Username      string `json:"username,omitempty"`
	OldTelegramID int64  `json:"old_telegram_id,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	AdminNote     string `json:"admin_note,omitempty"`
	ReviewerUID   int64  `json:"reviewer_uid,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	ReviewedAt    int64  `json:"reviewed_at,omitempty"`
}

type TelegramRosterEntry struct {
	ChatID     string `json:"chat_id"`
	TelegramID int64  `json:"telegram_id"`
	IsBot      bool   `json:"is_bot"`
	LastStatus string `json:"last_status"`
	FirstSeen  int64  `json:"first_seen_at"`
	LastSeen   int64  `json:"last_seen_at"`
}

type TelegramRosterUpdate struct {
	ChatID     string
	TelegramID int64
	Status     string
	IsBot      bool
}

// ReadLegacyStateFile 读入历史 JSON 后端的 state.json（含 .bak 兜底），返回可直接
// 交给 Store.LoadSnapshot 的快照字节。它是 PostgreSQL-only 收敛后唯一残留的 JSON
// 读路径，仅供一次性迁移命令（twilight migrate-json）使用——运行时不再有 JSON 后端。
// 内嵌的 runtime log 旁路文件（.runtimelog）刻意不迁移：那是纯诊断数据，价值不足以
// 拖着整套旁路解析器。
func ReadLegacyStateFile(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("legacy state file path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			// 主文件损坏时回退 .bak（历史 saveLocked 写前的影子拷贝）。
			bak, bakErr := os.ReadFile(path + ".bak")
			if bakErr != nil || len(bak) == 0 {
				return nil, fmt.Errorf("parse legacy state %q: %w", path, err)
			}
			if err := json.Unmarshal(bak, &state); err != nil {
				return nil, fmt.Errorf("parse legacy state .bak %q: %w", path, err)
			}
		}
	}
	state.ensure()
	return json.Marshal(state)
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	// 先停后台 flush 协程并落盘最后一批 API Key 调用统计，再关闭连接。
	s.stopAPIKeyUsageFlusher()
	return s.db.Close()
}

// DB 返回底层 *sql.DB。PostgreSQL-only 收敛后恒非 nil（Store 只能经 OpenPostgres 构造）。
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) ConfigurePostgres(maxOpen, maxIdle int) {
	if s == nil || s.db == nil {
		return
	}
	if maxOpen > 0 {
		s.db.SetMaxOpenConns(maxOpen)
	}
	if maxIdle > 0 {
		s.db.SetMaxIdleConns(maxIdle)
	}
}

// Backend 恒返回 BackendPostgres：Twilight 现在只有 PostgreSQL 一种后端。
// 保留此方法（而非删除）是为了兼容诊断 / 迁移面板里「当前生效后端」的展示位。
func (s *Store) Backend() string {
	return BackendPostgres
}

func emptyState() State {
	state := State{}
	state.ensure()
	return state
}

func (s *State) ensure() {
	if s.NextUserID <= 0 {
		s.NextUserID = 1
	}
	if s.NextAPIKeyID <= 0 {
		s.NextAPIKeyID = 1
	}
	if s.NextRequestID <= 0 {
		s.NextRequestID = 1
	}
	if s.NextAnnouncementID <= 0 {
		s.NextAnnouncementID = 1
	}
	if s.NextLoginLogID <= 0 {
		s.NextLoginLogID = 1
	}
	if s.NextRuntimeLogID <= 0 {
		s.NextRuntimeLogID = 1
	}
	if s.NextSchedulerRunID <= 0 {
		s.NextSchedulerRunID = 1
	}
	if s.NextRebindRequestID <= 0 {
		s.NextRebindRequestID = 1
	}
	// 历史 state 没有 NextViolationLogID 字段；走兜底取 max(existing IDs)+1，
	// 避免新计数器从 1 开始与已经存在的旧 ID 撞车。
	if s.NextViolationLogID <= 0 {
		max := int64(0)
		for _, log := range s.ViolationLogs {
			if log.ID > max {
				max = log.ID
			}
		}
		s.NextViolationLogID = max + 1
	}
	if s.Users == nil {
		s.Users = map[int64]User{}
	}
	for uid, u := range s.Users {
		changed := false
		if !u.EmbyGrantLocked && (u.PendingEmby || strings.TrimSpace(u.RegistrationSource) != "" || strings.TrimSpace(u.RegistrationCode) != "") {
			u.EmbyGrantLocked = true
			changed = true
		}
		// Backfill BGMManageMode to true for users who already have BGMToken set, so they keep management features by default
		if u.BGMToken != "" && !u.BGMManageMode && u.BGMMode {
			u.BGMManageMode = true
			changed = true
		}
		if changed {
			s.Users[uid] = u
		}
	}
	if s.APIKeys == nil {
		s.APIKeys = map[int64]APIKey{}
	}
	if s.MediaRequests == nil {
		s.MediaRequests = map[int64]MediaRequest{}
	}
	if s.Announcements == nil {
		s.Announcements = map[int64]Announcement{}
	}
	if s.InviteCodes == nil {
		s.InviteCodes = map[string]InviteCode{}
	}
	if s.InviteRelations == nil {
		s.InviteRelations = map[int64]InviteRelation{}
	}
	if s.RegCodes == nil {
		s.RegCodes = map[string]RegCode{}
	}
	if s.BindCodes == nil {
		s.BindCodes = map[string]BindCode{}
	}
	if s.EmailVerifications == nil {
		s.EmailVerifications = map[string]EmailVerification{}
	}
	if s.Signin == nil {
		s.Signin = map[int64]Signin{}
	}
	if s.SchedulerSchedules == nil {
		s.SchedulerSchedules = map[string]SchedulerSchedule{}
	}
	if s.Devices == nil {
		s.Devices = map[string]Device{}
	}
	if s.IPBlacklist == nil {
		s.IPBlacklist = map[string]IPBlacklistEntry{}
	}
	if s.RebindRequests == nil {
		s.RebindRequests = map[int64]RebindRequest{}
	}
	if s.ViolationLogs == nil {
		s.ViolationLogs = []ViolationLog{}
	}
	if s.RuntimeLogs == nil {
		s.RuntimeLogs = []RuntimeLogEntry{}
	}
	if s.NextAuditLogID <= 0 {
		max := int64(0)
		for _, log := range s.AuditLogs {
			if log.ID > max {
				max = log.ID
			}
		}
		s.NextAuditLogID = max + 1
	}
	if s.AuditLogs == nil {
		s.AuditLogs = []AuditLog{}
	}
	if s.NextBangumiSyncLogID <= 0 {
		max := int64(0)
		for _, log := range s.BangumiSyncLogs {
			if log.ID > max {
				max = log.ID
			}
		}
		s.NextBangumiSyncLogID = max + 1
	}
	if s.NextTicketID <= 0 {
		max := int64(0)
		for _, t := range s.Tickets {
			if t.ID > max {
				max = t.ID
			}
		}
		s.NextTicketID = max + 1
	}
	if s.NextDeveloperJSPresetID <= 0 {
		max := int64(0)
		for _, preset := range s.DeveloperJSPresets {
			if preset.ID > max {
				max = preset.ID
			}
		}
		s.NextDeveloperJSPresetID = max + 1
	}
	if s.BangumiSyncLogs == nil {
		s.BangumiSyncLogs = []BangumiSyncLog{}
	}
	if s.BangumiCollectionCache == nil {
		s.BangumiCollectionCache = map[string]BangumiCollectionCacheEntry{}
	}
	if s.BangumiSubjectCache == nil {
		s.BangumiSubjectCache = map[string]BangumiSubjectCacheEntry{}
	}
	s.normalizeBangumiCollectionSubjectCache()
	s.compactHistory()
	if s.Tickets == nil {
		s.Tickets = map[int64]Ticket{}
	}
	if s.TicketTypes == nil || len(s.TicketTypes) == 0 {
		s.TicketTypes = []string{"all"}
	}
	if s.DeveloperJSPresets == nil {
		s.DeveloperJSPresets = map[int64]DeveloperJSPreset{}
	}
	if s.BangumiRegcodeClaims == nil {
		s.BangumiRegcodeClaims = map[string]BangumiRegcodeClaim{}
	}

	// 历史遗留的「AdminNote → 合成一条管理员回复」迁移已移除。它本是 Replies[] 模型
	// 出现前的一次性数据迁移，却写在 ensure() 里每次 save/load 都跑，且不 gate：
	// 一旦「处理备注」被设而工单尚无回复，就凭空造出一条 UID:0「管理员」回复。
	// 在 AdminNote 与聊天回复解耦后（见 applyTicketReplyLocked），这会把纯元数据的
	// 处理备注反复伪造成对话消息，正是「管理员回复与聊天信息互相覆盖 / 部分消失」的一环。
	// 早已运行过该迁移的部署，其合成回复已落盘、不受影响；用户端在无回复时仍会
	// 回退展示 admin_note，legacy 可见性不丢失。此处不再自动把备注提升为回复。
}

func (s *State) compactHistory() {
	s.RuntimeLogs = compactTail(s.RuntimeLogs, defaultRuntimeLogLimit)
	s.LoginLogs = compactHead(s.LoginLogs, maxStoredLoginLogs)
	s.PlaybackRecords = compactHead(s.PlaybackRecords, maxStoredPlaybackRecords)
	s.PlaybackSessions = compactTail(s.PlaybackSessions, maxPlaybackSessions)
	s.EmbyActivityLogs = compactTail(s.EmbyActivityLogs, maxEmbyActivityLogs)
	s.BangumiSyncLogs = compactTail(s.BangumiSyncLogs, maxStoredBangumiSyncLogs)
	// 仅对真正超限的用户回写：compactTail 在未超限时原样返回同一底层切片，
	// 旧实现仍对每个用户做一次 map 赋值（value 类型 SigninState 是整值拷贝写回）。
	// 绝大多数用户签到记录远未及 maxSigninRecords，跳过回写省掉每次落盘对全体
	// 用户的无谓结构体拷贝（用户越多、写越频，收益越明显）。
	for uid, signin := range s.Signin {
		if len(signin.Records) <= maxSigninRecords {
			continue
		}
		signin.Records = compactTail(signin.Records, maxSigninRecords)
		s.Signin[uid] = signin
	}
}

func (s *State) EnsureForMigration() {
	s.ensure()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

// Refresh reloads the persisted state document into memory.
// Mutating paths call refreshLocked through mutateAndSaveLocked; admin list/detail
// handlers use this explicit read refresh when cross-process or PostgreSQL state
// changes must be visible before serving a response.
func (s *Store) Refresh() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshLocked()
}

// stateSnapshot 持有变更前 State 的序列化副本，仅在真正回滚时才反序列化。
// State 内大量 map/slice 是引用类型，浅拷贝无法隔离后续修改；旧实现在拍
// 快照时就 marshal+unmarshal 造出一份完整 State 对象图，但绝大多数写路径
// 成功落盘后这份副本立即变垃圾。改为只留字节、把 unmarshal 推迟到 restore，
// 常见成功路径每次写少一次全量 unmarshal 与一份多 MB State 分配。
type stateSnapshot struct {
	data []byte
}

// snapshotStateLocked 把当前 s.state 序列化成快照字节，用于失败回滚。
// 调用方必须已持有 s.mu 写锁。
// snapshotStateLocked 取「变更前状态」的序列化快照用于失败回滚。调用方必须已持有
// s.mu 写锁，且在同一临界区内紧接 refreshLocked 之后调用（mutateAndSaveLocked 保证）。
//
// 优先复用与当前 stateVersion 对齐的权威字节 stateRaw：restoreStateLocked 走
// unmarshal+ensure+rebuild 从它还原，结果与 refresh 产出的 s.state 逻辑等价，故可省去
// 一次多 MB 的全量 marshal。stateRaw 在 save 成功前始终代表变更前状态，可安全跨过 mutate。
func (s *Store) snapshotStateLocked() (stateSnapshot, error) {
	if len(s.stateRaw) > 0 {
		return stateSnapshot{data: s.stateRaw}, nil
	}
	s.state.ensure()
	data, err := json.Marshal(&s.state)
	if err != nil {
		return stateSnapshot{}, err
	}
	return stateSnapshot{data: data}, nil
}

// restoreStateLocked 用快照覆盖内存 state 并重建派生索引，只有回滚时才会
// 反序列化。集中回滚逻辑同时保证每个回滚点都重建索引（历史上多处
// `s.state = prev` 漏掉 rebuildUserIndexes，留下索引与用户表发散的隐患）。
// 调用方必须已持有 s.mu 写锁。
func (s *Store) restoreStateLocked(snap stateSnapshot) {
	var clone State
	if err := json.Unmarshal(snap.data, &clone); err != nil {
		// 快照字节来自刚刚成功的 json.Marshal，理论上不可能再反序列化失败；
		// 兜底从持久层重载（写失败时磁盘仍是变更前状态），避免内存停在半改态。
		// 先把本地版本设为不可能值，强制 refreshLocked 返回完整 state，不能命中
		// “版本未变化”快路径后原样保留待回滚的内存对象。
		s.stateVersion = -1
		s.stateRaw = nil
		_ = s.refreshLocked()
		return
	}
	clone.ensure()
	s.state = clone
	s.stateRaw = snap.data
	s.rebuildUserIndexes()
}

// mutateAndSaveLocked 把"读最新状态 → 变更 → 持久化 → 失败回滚"模板化。
// 调用方必须已经持有 s.mu 写锁；helper 内部不再额外加锁。
//
// 旧的写者模板"refreshLocked → 改 s.state → saveLocked"在 saveLocked 失败
// 后让内存与磁盘发散：DeleteUser 这种串改 12+ 个 map 的级联尤其危险，
// 一次磁盘故障即留下"用户已从 Users 删除但 InviteRelations 还在"的孤儿态。
// 这里在变更前用 snapshotStateLocked 拍快照，save 失败时用快照覆盖回去，
// 保证内存与磁盘要么一起前进、要么一起回到上一个一致点。
//
// mutate 自身返回 error 时不会触发 save / 回滚——还没真改盘，调用方自行处理。
func (s *Store) mutateAndSaveLocked(mutate func() error) error {
	// Postgres 后端多进程并发写会撞版本守卫（errStateVersionConflict）。撞上时
	// 说明他进程已提交更新：重新 refreshLocked 拉到最新 state + version、以新基线
	// 重放 mutate 再写。mutate 闭包必须基于「当前 s.state」重新计算（分配新 ID、
	// 读队列长度等都在闭包内基于最新状态进行），故重放天然吸收他进程的写而非覆盖。
	// 有界重试防病态活锁；JSON 后端不会返回该哨兵，一次即走完。
	const maxAttempts = 8
	for attempt := 0; ; attempt++ {
		if err := s.refreshLocked(); err != nil {
			return err
		}
		prev, err := s.snapshotStateLocked()
		if err != nil {
			return err
		}
		if err := mutate(); err != nil {
			// mutate 失败：本身就不打算落盘，状态可能被改了一半，回滚到快照。
			s.restoreStateLocked(prev)
			return err
		}
		err = s.saveLocked()
		if err == nil {
			return nil
		}
		// 无论何种失败都先把内存回滚到 mutate 前，保证内存与持久层一致。
		s.restoreStateLocked(prev)
		if errors.Is(err, errStateVersionConflict) && attempt < maxAttempts-1 {
			// 版本冲突且仍有重试预算：回到循环顶重新 refresh（拿到他进程的写与新
			// version）再重放。不 sleep——冲突窗口极短，且 refresh 本身即让路。
			continue
		}
		return err
	}
}

// saveLocked 走版本守卫写：要求持久层 version 仍等于本进程读到的 s.stateVersion，
// 被他进程抢先递增则返回 errStateVersionConflict 交由调用方处理（mutateAndSaveLocked
// 重试 / 直裸写者 fail-closed）。
func (s *Store) saveLocked() error {
	return s.saveStateLocked(false)
}

// saveLockedForce 无条件覆盖持久层并把 version 推到「读到值 +1」，绕过版本守卫。
// 仅用于 admin 恢复 / 迁移这类「本次快照就是权威、要盖掉一切」的场景（LoadSnapshot），
// 常规写路径一律走 saveLocked 的守卫版本，避免退回丢更新老路。
func (s *Store) saveLockedForce() error {
	return s.saveStateLocked(true)
}

func (s *Store) saveStateLocked(force bool) error {
	s.state.ensure()
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	// 整 jsonb 一次写入大对象（用户表 + 邀请关系 + 登录历史 + … 数 MB）；
	// 之前裸 context.Background() 一旦 PG 抖动会让 saveLocked 永久挂起，
	// graceful shutdown 与并发 handler 全部跟着卡死。这里用 30s
	// WithTimeout 兜底：到期 ExecContext 自行退出释放连接，调用方拿到
	// context.DeadlineExceeded 走回滚分支（mutateAndSaveLocked 把内存 state
	// 还原到 snapshot），磁盘和内存仍保持一致。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if force {
		// 强制覆盖：无视持久层现有 version，把它推进到「本进程读到值 +1」。
		// RETURNING 回填本地版本，使后续守卫写以此为新基线。
		var newVersion int64
		err = s.db.QueryRowContext(
			ctx,
			`INSERT INTO twilight_state (id, state, version, updated_at) VALUES (1, $1::jsonb, $2, now())
			 ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, version = twilight_state.version + 1, updated_at = now()
			 RETURNING version`,
			string(data), s.stateVersion+1,
		).Scan(&newVersion)
		if err != nil {
			return err
		}
		s.stateVersion = newVersion
		s.stateRaw = data
		return nil
	}
	// 版本守卫写：仅当持久层 version 仍等于本进程读到的 s.stateVersion 时才更新，
	// 命中即 version+1 并 RETURNING；被他进程抢先递增则 UPDATE 匹配 0 行、
	// QueryRow 得 sql.ErrNoRows，转成 errStateVersionConflict 交调用方重试 / 上抛。
	// id=1 尚不存在（冷启动首写）时 INSERT 分支生效，version 落为期望值。
	var newVersion int64
	err = s.db.QueryRowContext(
		ctx,
		`INSERT INTO twilight_state (id, state, version, updated_at) VALUES (1, $1::jsonb, $2, now())
		 ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, version = twilight_state.version + 1, updated_at = now()
		 WHERE twilight_state.version = $3
		 RETURNING version`,
		string(data), s.stateVersion+1, s.stateVersion,
	).Scan(&newVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return errStateVersionConflict
	}
	if err != nil {
		return err
	}
	s.stateVersion = newVersion
	s.stateRaw = data
	return nil
}

// refreshLocked 在每次写前从 PostgreSQL 全量拉取最新 state + version。多个 Twilight
// 进程（api / bot / scheduler）各持独立 Store，refresh 保证本进程在守卫 UPSERT 前拿到
// 他进程的最新提交（丢更新防护的读侧）；stateVersion 一并对齐，作为 saveLocked 守卫写
// 的期望基线。
func (s *Store) refreshLocked() error {
	if s == nil {
		return nil
	}
	// 裸 context.Background 一旦 PG 抖动 / 主从切换会让 refreshLocked 永久挂起，
	// 而它是所有 mutating 路径的前置（mutateAndSaveLocked / 直裸写者都走它）。
	// 整个 store mutex 会跟着卡死，进而把 HTTP handler、scheduler、bot 全部排队挂起。
	// 30s 与 saveLocked 同档，超时由调用方拿到 context.DeadlineExceeded 后走错误
	// 回滚 / 报错路径。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var data []byte
	var version int64
	// CASE 让版本未变化的常见路径只返回 NULL，而不是把整份 JSONB 从 PostgreSQL
	// 传回 Go 后再丢弃。版本变化时仍一次查询取回 state + version，保持 Refresh 的
	// 跨进程可见性与 mutateAndSaveLocked 的乐观并发基线。
	err := s.db.QueryRowContext(ctx, `
SELECT CASE WHEN version = $1 THEN NULL ELSE state END, version
FROM twilight_state WHERE id = 1`, s.stateVersion).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		s.state = emptyState()
		s.rebuildUserIndexes()
		s.stateVersion = 0
		s.stateRaw, _ = json.Marshal(&s.state)
		return nil
	}
	if err != nil {
		return err
	}
	if version == s.stateVersion && len(data) == 0 {
		// 本进程刚保存或上次刷新过的 state 仍是权威版本；stateRaw 同样继续有效。
		return nil
	}
	var state State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
	}
	state.ensure()
	s.state = state
	s.stateVersion = version
	s.stateRaw = data
	s.rebuildUserIndexes()
	return nil
}

func (s *Store) Snapshot() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return nil, err
	}
	state := s.state
	state.ensure()
	// runtime logs 落在独立表 `twilight_runtime_logs`，不会进入 `twilight_state`
	// 的 jsonb。Snapshot 必须把它们也读出来塞进 State，否则备份/恢复时 state 与
	// runtime 两条线时点错位（admin 看到"已恢复"但日志仍是恢复点之后的最新数据）。
	// 这里持锁期间额外做一次 SELECT，不影响并发写（只读快照）。
	logs, nextID, err := s.snapshotRuntimeLogsLocked()
	if err != nil {
		return nil, err
	}
	state.RuntimeLogs = logs
	if nextID > state.NextRuntimeLogID {
		state.NextRuntimeLogID = nextID
	}
	// Audit logs also live in a dedicated high-write table. Merge them into the
	// exported State so JSON backups remain complete and portable.
	auditLogs, nextAuditID, err := s.snapshotAuditLogsLocked()
	if err != nil {
		return nil, err
	}
	state.AuditLogs = auditLogs
	state.NextAuditLogID = nextAuditID
	// Telegram roster is another dedicated runtime table. Merge it only for the
	// portable JSON snapshot; the Store's live State keeps no historical roster
	// map resident on the Go heap.
	roster, err := s.snapshotTelegramRosterLocked()
	if err != nil {
		return nil, err
	}
	state.TelegramRoster = roster
	return json.MarshalIndent(state, "", "  ")
}

// snapshotRuntimeLogsLocked 必须在持有 s.mu 的情况下调用，从 PG 拉出所有
// runtime_logs（按 id 升序）以及 next_id（max(id)+1）。limit 暂不裁剪：
// 备份要求时点完整，超大表的取舍交由保留策略（PruneRuntimeLogs）控制。
//
// 这里走显式 5min 超时：备份场景容忍时间长一些，但不能裸
// context.Background 让备份卡死时把整个 store 写锁也卡死（Snapshot 由
// s.mu.Lock 持有写锁调用本函数）。超时回 caller 让 admin 看到错误信息，
// 比让全站登录 / 注册排队挂起强。
func (s *Store) snapshotRuntimeLogsLocked() ([]RuntimeLogEntry, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT id, time, level, message, COALESCE(attrs, '{}'::jsonb)::text
FROM twilight_runtime_logs
ORDER BY id ASC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var maxID int64
	out := []RuntimeLogEntry{}
	for rows.Next() {
		var entry RuntimeLogEntry
		var attrsText string
		if err := rows.Scan(&entry.ID, &entry.Time, &entry.Level, &entry.Message, &attrsText); err != nil {
			return nil, 0, err
		}
		if attrsText != "" {
			_ = json.Unmarshal([]byte(attrsText), &entry.Attrs)
		}
		if entry.ID > maxID {
			maxID = entry.ID
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	nextID := int64(1)
	if maxID > 0 {
		nextID = maxID + 1
	}
	return out, nextID, nil
}

func (s *Store) snapshotAuditLogsLocked() ([]AuditLog, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT id, uid, username, action, category, source, method, target_uid,
       COALESCE(detail, '{}'::jsonb)::text, ip, created_at
FROM twilight_audit_logs ORDER BY id ASC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var maxID int64
	out := make([]AuditLog, 0)
	for rows.Next() {
		entry, scanErr := scanAuditLog(rows.Scan)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		if entry.ID > maxID {
			maxID = entry.ID
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, maxID + 1, nil
}

func (s *Store) LoadSnapshot(data []byte) error {
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	state.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	// 快照里的 runtime_logs 单独接管：回写到独立表 twilight_runtime_logs，不进
	// twilight_state 的 jsonb。先把它从待落 state 里摘出，避免这批日志被写进 state
	// 且此后随每次 saveLocked 反复落盘。
	logs := state.RuntimeLogs
	auditLogs := state.AuditLogs
	telegramRoster := state.TelegramRoster
	legacyTelegramBotOffset := state.TelegramBotOffset
	state.RuntimeLogs = nil
	state.AuditLogs = nil
	state.TelegramRoster = nil
	state.NextAuditLogID = 1
	state.TelegramBotOffset = 0
	s.state = state
	// admin 恢复 / 迁移：本次快照就是权威，必须无条件盖掉持久层现值（不能因版本守卫
	// 失败而拒绝恢复）。saveLockedForce 的 ON CONFLICT 分支从持久层真实 version 递增，
	// 无论本地 s.stateVersion 是否新鲜都保证覆盖生效，并回填本地版本作后续写基线。
	if err := s.saveLockedForce(); err != nil {
		return err
	}
	if err := s.advanceTelegramBotOffset(legacyTelegramBotOffset); err != nil {
		return err
	}
	// 把 snapshot 里的 runtime_logs 显式回写到独立表，避免恢复后 twilight_state
	// 走到老时点而 twilight_runtime_logs 仍是最新数据。失败不致命（state 已经写回），
	// 但要 surface 给调用方决定是否重试。
	if err := s.replaceRuntimeLogsLocked(logs); err != nil {
		return err
	}
	if err := s.replaceAuditLogsLocked(auditLogs); err != nil {
		return err
	}
	if err := s.replaceTelegramRosterLocked(telegramRoster); err != nil {
		return err
	}
	return nil
}

// replaceRuntimeLogsLocked 必须在持有 s.mu 的情况下调用：用事务清空表再批量
// COPY 入库。中途失败回滚。空 entries 也走 TRUNCATE，使快照"无日志"语义被严格
// 遵守（避免恢复后还能看到恢复点之后的日志）。
func (s *Store) replaceRuntimeLogsLocked(entries []RuntimeLogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `TRUNCATE TABLE twilight_runtime_logs RESTART IDENTITY`); err != nil {
		return err
	}
	if len(entries) > 0 {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO twilight_runtime_logs (id, time, level, message, attrs) VALUES ($1, $2, $3, $4, $5::jsonb)`)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			attrs, mErr := json.Marshal(entry.Attrs)
			if mErr != nil {
				_ = stmt.Close()
				return mErr
			}
			id := entry.ID
			if id <= 0 {
				continue
			}
			if _, err := stmt.ExecContext(ctx, id, entry.Time, entry.Level, entry.Message, string(attrs)); err != nil {
				_ = stmt.Close()
				return err
			}
		}
		if err := stmt.Close(); err != nil {
			return err
		}
		// 显式把 sequence 推到 max(id) 之后，下一次 INSERT 不会撞上历史 id。
		if _, err := tx.ExecContext(ctx, `SELECT setval(pg_get_serial_sequence('twilight_runtime_logs', 'id'), COALESCE((SELECT MAX(id) FROM twilight_runtime_logs), 1))`); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

// replaceAuditLogsLocked mirrors replaceRuntimeLogsLocked for backup restore.
// Audit rows are restored transactionally and the sequence is synchronized so
// the next append cannot collide with a historical ID.
func (s *Store) replaceAuditLogsLocked(entries []AuditLog) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `TRUNCATE TABLE twilight_audit_logs RESTART IDENTITY`); err != nil {
		return err
	}
	if err := insertAuditLogsTx(ctx, tx, entries); err != nil {
		return err
	}
	if err := syncAuditLogSequence(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Backup(dir string) (BackupInfo, error) {
	return s.BackupWithNote(dir, "")
}

func (s *Store) BackupWithNote(dir, note string) (BackupInfo, error) {
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("db", "backups")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BackupInfo{}, err
	}
	data, err := s.Snapshot()
	if err != nil {
		return BackupInfo{}, err
	}
	now := time.Now().UTC()
	name := "twilight_state_" + now.Format("20060102_150405") + "_" + strconv.FormatInt(now.UnixNano()%1e9, 10) + ".json"
	path := filepath.Join(dir, name)
	// 备份必须走 tmp + fsync(file) + rename + fsync(dir)：旧实现只是 OpenFile +
	// Write + Close，crash/掉电会留下半截 JSON 文件，ListBackups 仍把它列出来；
	// 后续 RestoreFrom 解析失败 → admin 永远没法回到那一时点。
	if err := writeFileAtomicSync(path, data, 0o600); err != nil {
		return BackupInfo{}, err
	}
	info := BackupInfo{Name: name, Path: path, Size: int64(len(data)), CreatedAt: now.Unix(), Note: normalizeBackupNote(note)}
	if info.Note != "" {
		if err := writeBackupNote(path, info.Note); err != nil {
			return BackupInfo{}, err
		}
	}
	return info, nil
}

func (s *Store) RestoreFrom(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return s.LoadSnapshot(data)
}

func ListBackups(dir string) ([]BackupInfo, error) {
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("db", "backups")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}
	backups := make([]BackupInfo, 0, len(entries))
	for _, entry := range entries {
		lowerName := strings.ToLower(entry.Name())
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(lowerName, ".json") || strings.HasSuffix(lowerName, ".meta.json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		backups = append(backups, BackupInfo{
			Name:      entry.Name(),
			Path:      filepath.Join(dir, entry.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime().Unix(),
			Note:      ReadBackupNote(filepath.Join(dir, entry.Name())),
		})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt > backups[j].CreatedAt })
	return backups, nil
}

func BackupMetaPath(path string) string {
	return path + ".meta.json"
}

func ReadBackupNote(path string) string {
	data, err := os.ReadFile(BackupMetaPath(path))
	if err != nil {
		return ""
	}
	var meta struct {
		Note string `json:"note"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return normalizeBackupNote(meta.Note)
}

func writeBackupNote(path, note string) error {
	note = normalizeBackupNote(note)
	if note == "" {
		_ = os.Remove(BackupMetaPath(path))
		return nil
	}
	meta := struct {
		Note string `json:"note"`
	}{Note: note}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	// note 元数据同样走 atomic 写盘，避免半截 JSON 让 ReadBackupNote 静默丢 note。
	return writeFileAtomicSync(BackupMetaPath(path), data, 0o600)
}

// WriteFileAtomicSync 是 writeFileAtomicSync 的导出别名，供 api 层数据库迁移
// 等需要落地"用户可恢复备份"的路径复用同一份原子写盘语义。
func WriteFileAtomicSync(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomicSync(path, data, perm)
}

// writeFileAtomicSync 把 data 原子地写入 path：tmp 写完先 fsync 文件，
// 关闭后 rename，再 fsync 父目录。任一步失败清理 tmp 后返回 error。
// saveLocked / BackupWithNote / writeBackupNote / 数据库迁移文件状态写盘
// 共用此 helper，避免每个调用点单独维护一份持久化语义。
//
// tmp 用 unix.O_NOFOLLOW + O_EXCL 打开，杜绝 TOCTOU symlink 攻击：攻击者
// 若把 path.tmp 提前换成指向其它文件的 symlink，O_NOFOLLOW 会让 OpenFile
// 返回 ELOOP 而不是顺着链写穿；O_EXCL 阻止覆写已经存在的 .tmp 残留。
// 父目录 dir.Sync 错误同样需要回报：之前 dir.Sync 错误被静默吞掉，rename
// 已经走完但元数据未落盘，断电后 path 又指回 tmp 残留。
func writeFileAtomicSync(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	// 先把可能残留的 tmp 干掉。OpenFile 带 O_EXCL 时若 tmp 已存在会直接失败；
	// 旧调用未走 fsync 也可能留下半字节 tmp，这里清掉再开。
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|fsNoFollow, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(path, perm)
	if err := syncParentDir(filepath.Dir(path), true); err != nil {
		fmt.Fprintf(os.Stderr, "twilight: atomic write parent dir sync failed path=%s err=%v\n", path, err)
	}
	return nil
}

func syncParentDir(dirPath string, report bool) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(dirPath)
	if err != nil {
		return nil
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil && report {
		return err
	}
	return nil
}

func normalizeBackupNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	note = strings.Join(strings.Fields(note), " ")
	const maxRunes = 200
	runes := []rune(note)
	if len(runes) > maxRunes {
		note = string(runes[:maxRunes])
	}
	return note
}

func ResolveBackupPath(dir, name string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("db", "backups")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNotFound
	}
	if filepath.IsAbs(name) {
		return "", ErrNotFound
	}
	lowerName := strings.ToLower(name)
	if filepath.Base(name) != name || strings.Contains(name, "..") || !strings.HasSuffix(lowerName, ".json") || strings.HasSuffix(lowerName, ".meta.json") {
		return "", ErrNotFound
	}
	base, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, name))
	if err != nil {
		return "", err
	}
	if filepath.Dir(target) != base {
		return "", ErrNotFound
	}
	info, err := os.Lstat(target)
	if err != nil {
		return "", ErrNotFound
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", ErrNotFound
	}
	return target, nil
}

func (s *Store) UserCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.Users)
}

// UserCounts returns total and active user counts without copying the complete
// user slice. It is intended for status panels and scheduled summaries.
func (s *Store) UserCounts() (total int, active int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total = len(s.state.Users)
	for _, user := range s.state.Users {
		if user.Active {
			active++
		}
	}
	return total, active
}

func (s *Store) CreateUser(u User) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var created User
	err := s.mutateAndSaveLocked(func() error {
		if s.usernameExistsLocked(u.Username) || s.emailTakenLocked(u.Email, 0) || s.telegramIDTakenLocked(u.TelegramID, 0) || s.embyIDTakenLocked(u.EmbyID, 0) {
			return ErrConflict
		}
		now := time.Now().Unix()
		u.UID = s.state.NextUserID
		s.state.NextUserID++
		if u.CreatedAt == 0 {
			u.CreatedAt = now
		}
		if u.RegisterTime == 0 {
			u.RegisterTime = now
		}
		if u.ExpiredAt == 0 {
			u.ExpiredAt = -1
		}
		u.Active = true
		s.state.Users[u.UID] = u
		s.maintainUserIndexes(User{}, u, u.UID)
		created = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return created, nil
}

func (s *Store) usernameExistsLocked(username string) bool {
	return s.usernameTakenLocked(username, 0)
}

func (s *Store) usernameTakenLocked(username string, allowedUID int64) bool {
	target := normalizeUsernameKey(username)
	if target == "" {
		return false
	}
	if s.usernameMap != nil {
		if uid, ok := s.usernameMap[target]; ok {
			if uid != allowedUID {
				return true
			}
			for _, existing := range s.state.Users {
				if existing.UID != allowedUID && normalizeUsernameKey(existing.Username) == target {
					return true
				}
			}
		}
		return false
	}
	for _, existing := range s.state.Users {
		if existing.UID != allowedUID && normalizeUsernameKey(existing.Username) == target {
			return true
		}
	}
	return false
}

func (s *Store) emailTakenLocked(email string, allowedUID int64) bool {
	target := normalizeEmailKey(email)
	if target == "" {
		return false
	}
	if s.emailMap != nil {
		if uid, ok := s.emailMap[target]; ok {
			if uid != allowedUID {
				return true
			}
			for _, existing := range s.state.Users {
				if existing.UID != allowedUID && normalizeEmailKey(existing.Email) == target {
					return true
				}
			}
		}
		return false
	}
	for _, existing := range s.state.Users {
		if existing.UID != allowedUID && normalizeEmailKey(existing.Email) == target {
			return true
		}
	}
	return false
}

func (s *Store) telegramIDTakenLocked(telegramID, allowedUID int64) bool {
	if telegramID == 0 {
		return false
	}
	if s.telegramIDMap != nil {
		if uid, ok := s.telegramIDMap[telegramID]; ok && uid != allowedUID {
			return true
		}
		return false
	}
	// 索引未初始化，回退扫描
	for _, existing := range s.state.Users {
		if existing.TelegramID == telegramID && existing.UID != allowedUID {
			return true
		}
	}
	return false
}

func (s *Store) CreateUserWithRegCode(u User, regCode string, telegramID int64) (User, RegCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var created User
	var consumed RegCode
	err := s.mutateAndSaveLocked(func() error {
		if s.usernameExistsLocked(u.Username) || s.emailTakenLocked(u.Email, 0) || s.telegramIDTakenLocked(u.TelegramID, 0) || s.telegramIDTakenLocked(telegramID, 0) || s.embyIDTakenLocked(u.EmbyID, 0) {
			return ErrConflict
		}
		now := time.Now().Unix()
		// 创建路径：账号尚未分配 UID，传 (0,0) 跳过 per-identity 守卫（每次都是新身份）。
		reg, err := s.consumableRegCodeLocked(regCode, 0, 0, now)
		if err != nil {
			return err
		}
		if reg.Type != 1 || reg.IsDecoy || !regCodeMatchesUser(reg, u) {
			return ErrNotFound
		}

		u.UID = s.state.NextUserID
		s.state.NextUserID++
		if u.CreatedAt == 0 {
			u.CreatedAt = now
		}
		if u.RegisterTime == 0 {
			u.RegisterTime = now
		}
		if u.ExpiredAt == 0 {
			u.ExpiredAt = -1
		}
		u.Active = true
		consumed = s.consumeRegCodeLocked(reg, u.UID, telegramID)
		s.state.Users[u.UID] = u
		s.maintainUserIndexes(User{}, u, u.UID)
		created = u
		return nil
	})
	if err != nil {
		return User{}, RegCode{}, err
	}
	return created, consumed, nil
}

func (s *Store) CreateUserForRegistration(u User, regCode, telegramBindCode string, now int64, fn func(*User, RegCode, BindCode) error) (User, RegCode, BindCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var created User
	var consumed RegCode
	var consumedBind BindCode
	err := s.mutateAndSaveLocked(func() error {
		if s.usernameExistsLocked(u.Username) || s.emailTakenLocked(u.Email, 0) || s.embyIDTakenLocked(u.EmbyID, 0) {
			return ErrConflict
		}
		if now == 0 {
			now = time.Now().Unix()
		}
		if telegramBindCode != "" {
			bind, ok := s.state.BindCodes[telegramBindCode]
			if !ok {
				return ErrNotFound
			}
			if bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
				delete(s.state.BindCodes, telegramBindCode)
				return ErrExpired
			}
			if bind.Scene != "register" || !bind.Confirmed || bind.TelegramID == 0 {
				return ErrConflict
			}
			if s.telegramIDTakenLocked(bind.TelegramID, 0) {
				return ErrConflict
			}
			u.TelegramID = bind.TelegramID
			u.TelegramUsername = bind.TelegramUsername
			consumedBind = bind
		} else if s.telegramIDTakenLocked(u.TelegramID, 0) {
			return ErrConflict
		}

		if regCode != "" {
			// 创建路径：账号尚未分配 UID，传 (0,0) 跳过 per-identity 守卫（每次都是新身份）。
			reg, err := s.consumableRegCodeLocked(regCode, 0, 0, now)
			if err != nil {
				return err
			}
			if reg.Type != 1 || reg.IsDecoy || !regCodeMatchesUser(reg, u) {
				return ErrNotFound
			}
			consumed = reg
		}

		u.UID = s.state.NextUserID
		s.state.NextUserID++
		if u.CreatedAt == 0 {
			u.CreatedAt = now
		}
		if u.RegisterTime == 0 {
			u.RegisterTime = now
		}
		if u.ExpiredAt == 0 {
			u.ExpiredAt = -1
		}
		u.Active = true
		if consumed.Code != "" {
			consumed = s.consumeRegCodeLocked(consumed, u.UID, u.TelegramID)
		}
		if fn != nil {
			if err := fn(&u, consumed, consumedBind); err != nil {
				return err
			}
		}
		if s.embyIDTakenLocked(u.EmbyID, u.UID) {
			return ErrConflict
		}
		if telegramBindCode != "" {
			delete(s.state.BindCodes, telegramBindCode)
		}
		s.state.Users[u.UID] = u
		s.maintainUserIndexes(User{}, u, u.UID)
		created = u
		return nil
	})
	if err != nil {
		return User{}, RegCode{}, BindCode{}, err
	}
	return created, consumed, consumedBind, nil
}

func regCodeMatchesUser(reg RegCode, user User) bool {
	if reg.TargetUsername != "" && !strings.EqualFold(reg.TargetUsername, user.Username) {
		return false
	}
	if reg.TargetTelegramID != 0 && reg.TargetTelegramID != user.TelegramID {
		return false
	}
	if reg.TargetTelegramUsername != "" {
		if normalizeTelegramUsername(reg.TargetTelegramUsername) != normalizeTelegramUsername(user.TelegramUsername) {
			return false
		}
	}
	return true
}

func normalizeUsernameKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func normalizeTelegramUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

func (s *Store) userIdentityConflictLocked(oldUser, newUser User, allowedUID int64) error {
	if normalizeUsernameKey(oldUser.Username) != normalizeUsernameKey(newUser.Username) && s.usernameTakenLocked(newUser.Username, allowedUID) {
		return ErrConflict
	}
	if normalizeEmailKey(oldUser.Email) != normalizeEmailKey(newUser.Email) && s.emailTakenLocked(newUser.Email, allowedUID) {
		return ErrConflict
	}
	if oldUser.TelegramID != newUser.TelegramID && s.telegramIDTakenLocked(newUser.TelegramID, allowedUID) {
		return ErrConflict
	}
	if oldUser.EmbyID != newUser.EmbyID && s.embyIDTakenLocked(newUser.EmbyID, allowedUID) {
		return ErrConflict
	}
	return nil
}

func (s *Store) FindUserByUsername(username string) (User, bool) {
	target := normalizeUsernameKey(username)
	if target == "" {
		return User{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.usernameMap == nil {
		for _, u := range s.state.Users {
			if normalizeUsernameKey(u.Username) == target {
				return u, true
			}
		}
		return User{}, false
	}
	if uid, ok := s.usernameMap[target]; ok {
		u, found := s.state.Users[uid]
		if found && normalizeUsernameKey(u.Username) == target {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) FindUserByEmail(email string) (User, bool) {
	target := normalizeEmailKey(email)
	if target == "" {
		return User{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.emailMap == nil {
		for _, u := range s.state.Users {
			if normalizeEmailKey(u.Email) == target {
				return u, true
			}
		}
		return User{}, false
	}
	if uid, ok := s.emailMap[target]; ok {
		u, found := s.state.Users[uid]
		if found && normalizeEmailKey(u.Email) == target {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) FindUserByEmbyID(embyID string) (User, bool) {
	if embyID == "" {
		return User{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.embyIDMap == nil {
		for _, u := range s.state.Users {
			if u.EmbyID == embyID {
				return u, true
			}
		}
		return User{}, false
	}
	if uid, ok := s.embyIDMap[embyID]; ok {
		u, found := s.state.Users[uid]
		if found && u.EmbyID == embyID {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) embyIDTakenLocked(embyID string, allowedUID int64) bool {
	if embyID == "" {
		return false
	}
	if s.embyIDMap != nil {
		if uid, ok := s.embyIDMap[embyID]; ok && uid != allowedUID {
			return true
		}
		return false
	}
	for _, u := range s.state.Users {
		if u.EmbyID == embyID && u.UID != allowedUID {
			return true
		}
	}
	return false
}

func (s *Store) User(uid int64) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.state.Users[uid]
	return u, ok
}

func (s *Store) UpdateUser(uid int64, fn func(*User) error) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		old := u
		if err := fn(&u); err != nil {
			return err
		}
		if err := s.userIdentityConflictLocked(old, u, uid); err != nil {
			return ErrConflict
		}
		s.state.Users[uid] = u
		s.maintainUserIndexes(old, u, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return updated, nil
}

// UpdateUsers 把「对一组用户各跑一次 fn」压成一次 store 写：一次 refreshLocked +
// snapshotStateLocked + saveLocked，取代逐用户 UpdateUser 的 N 次「整份 state
// marshal + fsync（JSON）/ 整 jsonb upsert（PG）」。批量续期这类纯本地写在 JSON
// 后端上原本会持 s.mu 做上百次整库落盘（多 MB × N），把所有并发读写全卡在锁上；
// 收敛成一次落盘后锁占用与 IO 都降到 O(1) 份。
//
// 语义对齐 UpdateUser 的逐用户路径：fn 返回 error / 身份字段冲突 / 用户不存在都
// 只记进返回的 map 且**不**中断整批（该用户的改动像 UpdateUser 的 mutate 失败分支
// 一样被丢弃，不落盘）。只有 saveLocked 失败才整批回滚到变更前快照并以第二个返回值
// 抛出——与 mutateAndSaveLocked 的「要么一起前进、要么一起回到上一个一致点」保持一致。
// 重复 uid 只处理一次；没有任何用户真正改动时跳过 save。身份索引随每个成功用户增量
// 维护，因此同一批内后来的用户能正确看到前面用户占用/释放的用户名/邮箱/TG/Emby 标识。
func (s *Store) UpdateUsers(uids []int64, fn func(*User) error) (map[int64]error, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := make(map[int64]error, len(uids))
	if len(uids) == 0 {
		return results, nil
	}
	if err := s.refreshLocked(); err != nil {
		return nil, err
	}
	prev, err := s.snapshotStateLocked()
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{}, len(uids))
	changed := false
	for _, uid := range uids {
		if _, dup := seen[uid]; dup {
			continue
		}
		seen[uid] = struct{}{}
		u, ok := s.state.Users[uid]
		if !ok {
			results[uid] = ErrNotFound
			continue
		}
		old := u
		if ferr := fn(&u); ferr != nil {
			results[uid] = ferr
			continue
		}
		if cerr := s.userIdentityConflictLocked(old, u, uid); cerr != nil {
			results[uid] = ErrConflict
			continue
		}
		s.state.Users[uid] = u
		s.maintainUserIndexes(old, u, uid)
		results[uid] = nil
		changed = true
	}
	if !changed {
		return results, nil
	}
	if err := s.saveLocked(); err != nil {
		s.restoreStateLocked(prev)
		return nil, err
	}
	return results, nil
}

func (s *Store) ClearUserEmails() (total int, cleared int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.mutateAndSaveLocked(func() error {
		total = len(s.state.Users)
		for uid, u := range s.state.Users {
			if u.Email == "" {
				continue
			}
			old := u
			u.Email = ""
			s.state.Users[uid] = u
			s.maintainUserIndexes(old, u, uid)
			cleared++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return total, cleared, nil
}

// LockEmbyGrantForBoundUsers sets EmbyGrantLocked=true for bound users in one
// store write. Users without Emby are returned as skipped instead of being
// treated as failures so bulk UI actions can safely target broad filters.
func (s *Store) LockEmbyGrantForBoundUsers(uids []int64) (updated []int64, missing []int64, skippedNoEmby []int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return nil, nil, nil, err
	}
	prev, err := s.snapshotStateLocked()
	if err != nil {
		return nil, nil, nil, err
	}
	seen := map[int64]bool{}
	changed := false
	for _, uid := range uids {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		u, ok := s.state.Users[uid]
		if !ok {
			missing = append(missing, uid)
			continue
		}
		if strings.TrimSpace(u.EmbyID) == "" {
			skippedNoEmby = append(skippedNoEmby, uid)
			continue
		}
		if !u.EmbyGrantLocked {
			u.EmbyGrantLocked = true
			s.state.Users[uid] = u
			changed = true
		}
		updated = append(updated, uid)
	}
	if !changed {
		return updated, missing, skippedNoEmby, nil
	}
	if err := s.saveLocked(); err != nil {
		s.restoreStateLocked(prev)
		return nil, nil, nil, err
	}
	return updated, missing, skippedNoEmby, nil
}

// ClearEmbyGrantResult 汇总一次"清理无 Emby 账号用户的注册码/邀请码使用记录"的结果。
type ClearEmbyGrantResult struct {
	Cleared        []int64 // 确实有记录被抹除的 UID
	AlreadyClean   []int64 // 无 Emby 但本来就没有任何使用记录可清
	SkippedHasEmby []int64 // 已绑定 Emby，跳过（已注册用户的注册资格不可重置）
	SkippedPending []int64 // 处于 PendingEmby 在飞队列，保留其待开通资格
	Missing        []int64 // UID 不存在
	RegcodeRefs    int     // 从注册码 UsedBy/UsedByUIDs 抹除的引用数
	InviteRefs     int     // 抹除的邀请使用记录 / 解除的邀请关系数
}

// ClearEmbyGrantForUnboundUsers 清理"没有 Emby 账号"用户的注册码/邀请码使用记录，
// 让他们重新可以使用注册码 / 邀请码。专门针对历史迁移（refreshLocked 里把
// PendingEmby/RegistrationSource/RegistrationCode 非空的用户一律置 EmbyGrantLocked=true）
// 把从未真正开通过 Emby 的用户错误判定为"已用过注册资格"的脏数据场景。
//
// 对每个 UID：
//   - 不存在            → Missing
//   - 已绑定 Emby        → SkippedHasEmby（已注册用户不能重置注册资格）
//   - PendingEmby 在飞   → SkippedPending（保留其待开通资格，避免取消进行中的注册）
//   - 其余（无 Emby）    → 清用户侧 EmbyGrantLocked / RegistrationSource / RegistrationCode，
//     并从注册码、邀请码及邀请关系里抹除其使用引用（码侧 UseCount 相应回退，
//     回退后低于上限的码恢复可用）。
func (s *Store) ClearEmbyGrantForUnboundUsers(uids []int64) (ClearEmbyGrantResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result ClearEmbyGrantResult
	err := s.mutateAndSaveLocked(func() error {
		result = ClearEmbyGrantResult{}
		seen := map[int64]bool{}
		for _, uid := range uids {
			if seen[uid] {
				continue
			}
			seen[uid] = true
			u, ok := s.state.Users[uid]
			if !ok {
				result.Missing = append(result.Missing, uid)
				continue
			}
			if strings.TrimSpace(u.EmbyID) != "" {
				result.SkippedHasEmby = append(result.SkippedHasEmby, uid)
				continue
			}
			if u.PendingEmby {
				result.SkippedPending = append(result.SkippedPending, uid)
				continue
			}
			userChanged := false
			if u.EmbyGrantLocked || strings.TrimSpace(u.RegistrationSource) != "" || strings.TrimSpace(u.RegistrationCode) != "" {
				u.EmbyGrantLocked = false
				u.RegistrationSource = ""
				u.RegistrationCode = ""
				s.state.Users[uid] = u
				userChanged = true
			}
			regRefs := s.clearRegCodeRefsForUIDLocked(uid)
			invRefs := s.clearInviteUsageForUIDLocked(uid)
			result.RegcodeRefs += regRefs
			result.InviteRefs += invRefs
			if userChanged || regRefs > 0 || invRefs > 0 {
				result.Cleared = append(result.Cleared, uid)
			} else {
				result.AlreadyClean = append(result.AlreadyClean, uid)
			}
		}
		return nil
	})
	if err != nil {
		return ClearEmbyGrantResult{}, err
	}
	return result, nil
}

// clearRegCodeRefsForUIDLocked 从所有注册码抹除对该 UID 的使用引用，UseCount 相应
// 回退；回退后若码因"用满次数"被自动停用且现在低于上限，则恢复 Active=true。
// 返回抹除的引用条数（每个码对同一 UID 至多一条）。UsedByTelegramIDs 保持不动：
// TG 维度的占用无法可靠映射回单个 UID，避免误删他人记录。
func (s *Store) clearRegCodeRefsForUIDLocked(uid int64) int {
	if uid == 0 {
		return 0
	}
	removed := 0
	for code, rc := range s.state.RegCodes {
		dirty := false
		if rc.UsedBy == uid {
			rc.UsedBy = 0
			dirty = true
		}
		if len(rc.UsedByUIDs) > 0 {
			pruned := make([]int64, 0, len(rc.UsedByUIDs))
			for _, u := range rc.UsedByUIDs {
				if u == uid {
					removed++
					if rc.UseCount > 0 {
						rc.UseCount--
					}
					continue
				}
				pruned = append(pruned, u)
			}
			if len(pruned) != len(rc.UsedByUIDs) {
				if len(pruned) == 0 {
					rc.UsedByUIDs = nil
				} else {
					rc.UsedByUIDs = pruned
				}
				dirty = true
			}
		}
		if dirty {
			if !rc.Active && rc.UseCountLimit != -1 && rc.UseCount < rc.UseCountLimit {
				rc.Active = true
			}
			s.state.RegCodes[code] = rc
		}
	}
	return removed
}

// clearInviteUsageForUIDLocked 解除该 UID 作为"被邀请者(invitee)"的邀请使用记录：
// 断开邀请关系并抹除其在邀请码上的占用（UsedByUID/Used/UseCount/Active），使其可
// 重新加入邀请树 / 使用邀请码。只清理其作为 child 的记录；其作为邀请人(inviter)
// 生成、被他人使用的邀请码不受影响。返回处理的邀请记录数。
func (s *Store) clearInviteUsageForUIDLocked(uid int64) int {
	if uid == 0 {
		return 0
	}
	handled := 0
	detachedCodes := map[string]bool{}
	for key, rel := range s.state.InviteRelations {
		if key != uid && rel.ChildUID != uid {
			continue
		}
		if rel.Code != "" {
			detachedCodes[rel.Code] = true
		}
		delete(s.state.InviteRelations, key)
		handled++
	}
	for code, c := range s.state.InviteCodes {
		if c.UsedByUID != uid && !detachedCodes[code] {
			continue
		}
		c.UsedByUID = 0
		c.Used = false
		if c.UseCount > 0 {
			c.UseCount--
		}
		if !c.Active && c.UseCountLimit != -1 && c.UseCount < c.UseCountLimit {
			c.Active = true
		}
		s.state.InviteCodes[code] = c
		handled++
	}
	return handled
}

// SetUserRoleAtomic 在同一把写锁内做 last-admin 计数 + 写入。
// 解决了原 handleAdminUpdateUser / handleAdminSetRole 把"读 ListUsers 计数"
// 与"UpdateUser 闭包"分两段执行导致的 TOCTOU：两个 admin 并发降级两个不同 admin
// 时，原先各自看到 adminCount=2 都通过校验，事后剩 0 admin。
//
// 当目标当前是 active admin、新 role 不是 admin 时，要求剩余 active admin >=1，
// 否则返回 ErrLastAdmin 让 handler 转 409。
func (s *Store) SetUserRoleAtomic(uid int64, newRole int) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		if u.Role == RoleAdmin && u.Active && newRole != RoleAdmin {
			others := 0
			for _, other := range s.state.Users {
				if other.UID != u.UID && other.Role == RoleAdmin && other.Active {
					others++
				}
			}
			if others == 0 {
				return ErrLastAdmin
			}
		}
		u.Role = newRole
		s.state.Users[uid] = u
		updated = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return updated, nil
}

// SetUserActiveAtomic 与 SetUserRoleAtomic 同理，处理"禁用最后一个 active admin"。
// 解决 handleAdminToggleUser 把"是否最后 admin"放在闭包外快照读取的问题。
func (s *Store) SetUserActiveAtomic(uid int64, active bool) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		if u.Active && !active && u.Role == RoleAdmin {
			others := 0
			for _, other := range s.state.Users {
				if other.UID != u.UID && other.Role == RoleAdmin && other.Active {
					others++
				}
			}
			if others == 0 {
				return ErrLastAdmin
			}
		}
		u.Active = active
		s.state.Users[uid] = u
		updated = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return updated, nil
}

// TelegramMembershipRebindProtections returns users whose Telegram group
// membership checks must pause while a legitimate rebind workflow is active.
// Pending and approved requests are both actionable workflow states; an
// in-progress rebind is tracked directly on the user after the old binding is
// consumed. Callers must still re-check immediately before a destructive
// change because a request can be submitted after this snapshot is read.
func (s *Store) TelegramMembershipRebindProtections() map[int64]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	protected := make(map[int64]string)
	for _, req := range s.state.RebindRequests {
		if req.UID == 0 || (req.Status != "pending" && req.Status != "approved") {
			continue
		}
		if req.Status == "pending" || protected[req.UID] == "" {
			protected[req.UID] = req.Status
		}
	}
	for uid, user := range s.state.Users {
		if user.RebindingInProgress {
			protected[uid] = "in_progress"
		}
	}
	return protected
}

// DisableUserForTelegramMembership disables a user only when no rebind
// workflow is active at the exact mutation point. It is deliberately separate
// from SetUserActiveAtomic: group-membership enforcement has a rebind-specific
// safety exception, while administrator and expiry actions must remain able to
// disable accounts normally.
func (s *Store) DisableUserForTelegramMembership(uid int64) (User, bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		updated          User
		disabled         bool
		protectionReason string
	)
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		updated = u
		if reason := s.telegramMembershipRebindProtectionLocked(uid, u); reason != "" {
			protectionReason = reason
			return errTelegramMembershipRebindProtected
		}
		if !u.Active {
			return errTelegramMembershipNoChange
		}
		if u.Role == RoleAdmin {
			others := 0
			for _, other := range s.state.Users {
				if other.UID != u.UID && other.Role == RoleAdmin && other.Active {
					others++
				}
			}
			if others == 0 {
				return ErrLastAdmin
			}
		}
		u.Active = false
		s.state.Users[uid] = u
		updated = u
		disabled = true
		return nil
	})
	if errors.Is(err, errTelegramMembershipRebindProtected) || errors.Is(err, errTelegramMembershipNoChange) {
		return updated, false, protectionReason, nil
	}
	if err != nil {
		return User{}, false, "", err
	}
	return updated, disabled, "", nil
}

// telegramMembershipRebindProtectionLocked is intentionally a final, local
// scan. The outer scheduler uses the bulk snapshot for speed; this lock-held
// check closes the submit-request versus disable-account race.
func (s *Store) telegramMembershipRebindProtectionLocked(uid int64, user User) string {
	if user.RebindingInProgress {
		return "in_progress"
	}
	reason := ""
	for _, req := range s.state.RebindRequests {
		if req.UID != uid || (req.Status != "pending" && req.Status != "approved") {
			continue
		}
		if req.Status == "pending" || reason == "" {
			reason = req.Status
		}
	}
	return reason
}

// BindUserTelegramAtomic 同把锁内：唯一性校验 + admin 自保 + 写入。
// 解决 handleAdminBindTelegram 闭包外 FindUserByTelegramID 与 UpdateUser
// 闭包写之间的 TOCTOU。
func (s *Store) BindUserTelegramAtomic(uid int64, tgid int64, currentUID int64) (User, int64, error) {
	return s.BindUserTelegramAtomicWithUsername(uid, tgid, "", currentUID)
}

// BindUserTelegramAtomicWithUsername binds the Telegram identity and username
// in one state mutation. An empty username clears a stale name when the ID is
// changed, but preserves the current name for an idempotent same-ID bind.
func (s *Store) BindUserTelegramAtomicWithUsername(uid int64, tgid int64, telegramUsername string, currentUID int64) (User, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		updated User
		old     int64
	)
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		if u.Role == RoleAdmin && u.UID != currentUID {
			return ErrConflict
		}
		if s.telegramIDTakenLocked(tgid, uid) {
			return ErrConflict
		}
		oldUser := u
		old = u.TelegramID
		u.TelegramID = tgid
		username := strings.TrimPrefix(strings.TrimSpace(telegramUsername), "@")
		if username != "" {
			u.TelegramUsername = username
		} else if old != tgid {
			u.TelegramUsername = ""
		}
		s.state.Users[uid] = u
		s.maintainUserIndexes(oldUser, u, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, 0, err
	}
	return updated, old, nil
}

// BindUserEmbyAtomic 同把锁内做 EmbyID 唯一性 + force rebind。
// force=true 时若 EmbyID 已绑在另一用户身上，会先把对方解绑再绑给目标，
// 一次写入完成；非 force 模式下冲突直接 ErrConflict。
// 解决 handleAdminBindEmby 在两段独立锁之间被第三方再次绑定的窗口。
func (s *Store) BindUserEmbyAtomic(uid int64, embyID, embyUsername string, force bool) (User, int64, error) {
	return s.BindUserEmbyAtomicWithUpdate(uid, embyID, embyUsername, force, nil)
}

func (s *Store) BindUserEmbyAtomicWithUpdate(uid int64, embyID, embyUsername string, force bool, fn func(*User, User) error) (User, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		updated   User
		displaced int64
	)
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		before := u
		if embyID != "" {
			if s.embyIDMap == nil {
				s.rebuildUserIndexes()
			}
			if otherUID, ok := s.embyIDMap[embyID]; ok && otherUID != uid {
				if !force {
					return ErrConflict
				}
				other := s.state.Users[otherUID]
				oldOtherEmbyID := other.EmbyID
				other.EmbyID = ""
				other.EmbyUsername = ""
				other.PendingEmby = true
				s.state.Users[other.UID] = other
				s.maintainEmbyIDIndex(oldOtherEmbyID, other.EmbyID, other.UID)
				displaced = other.UID
			}
		}
		old := u
		u.EmbyID = embyID
		u.EmbyUsername = embyUsername
		if embyID != "" {
			u.PendingEmby = false
			u.PendingEmbyDays = nil
		}
		if fn != nil {
			if err := fn(&u, before); err != nil {
				return err
			}
		}
		if err := s.userIdentityConflictLocked(old, u, uid); err != nil {
			return ErrConflict
		}
		s.state.Users[uid] = u
		s.maintainUserIndexes(old, u, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, 0, err
	}
	return updated, displaced, nil
}

// DeleteUser 删除用户并级联清理所有 UID-键控的衍生数据。
// 级联策略：
//
//	删除（GDPR right-to-erasure，含个人指纹/设备/行为）：
//	  Users / APIKeys / InviteCodes / InviteRelations / MediaRequests
//	  Signin / Devices / LoginLogs / PlaybackRecords / BindCodes
//	  RebindRequests
//	匿名化（保留业务/审计载体，但抹除 UID 引用）：
//	  RegCodes.UsedBy / RegCodes.UsedByUIDs（保留 regcode 本身的有效性）
//	  Announcements.CreatedByUID（公告内容必须保留，不能因作者被删而消失）
//	保留原样（安全审计 / 合规追溯）：
//	  ViolationLogs（违规记录是安全审计 artefact，不随用户删除）
//	  RebindRequests.ReviewerUID（保留审核者 UID，便于回溯审核轨迹；
//	    用户作为 reviewer 被删时同样不抹除——只删 UID 字段对应的请求体）
//
// 漏一处会留下"幽灵关联"：例如设备指纹被旧 UID 占住，新建同名用户登录
// 时会被错误识别为"老设备已信任"。这条函数是用户生命周期的最终清算点。
func (s *Store) DeleteUser(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		s.maintainUserIndexes(u, User{}, uid)
		delete(s.state.Users, uid)

		// API keys 与会话凭证：必须清理，否则用户被删后旧 key 仍可调用接口。
		for id, key := range s.state.APIKeys {
			if key.UID == uid {
				delete(s.state.APIKeys, id)
				// 同步摘除 hash 索引，保持其为现存 key 的准确超集（复核已能挡住陈旧项，
				// 此处清理只为不留悬垂条目；save 失败回滚会整表重建）。
				if s.apiKeyHashMap != nil && key.Hash != "" && s.apiKeyHashMap[key.Hash] == id {
					delete(s.apiKeyHashMap, key.Hash)
				}
			}
		}

		// 邀请码：邀请人 / 接收人任一为该用户都失效。
		for code, invite := range s.state.InviteCodes {
			if invite.InviterUID == uid || invite.UID == uid || invite.UsedByUID == uid {
				delete(s.state.InviteCodes, code)
			}
		}

		// 邀请关系：自身作为 child 与作为 parent 的关系都断开（避免邀请树留孤儿）。
		delete(s.state.InviteRelations, uid)
		for child, rel := range s.state.InviteRelations {
			if rel.ParentUID == uid {
				delete(s.state.InviteRelations, child)
			}
		}

		// 求片记录：用户撤销，待办求片随之消失。
		for id, req := range s.state.MediaRequests {
			if req.UID == uid {
				delete(s.state.MediaRequests, id)
			}
		}

		// 签到积分 / 历史。
		delete(s.state.Signin, uid)

		// 设备指纹：要必须清，否则同 UID 重新创建（管理员复用编号）会继承
		// 旧设备的 trusted 标记，等价于"复用 UID 直接绕过设备校验"。
		for id, dev := range s.state.Devices {
			if dev.UID == uid {
				delete(s.state.Devices, id)
			}
		}

		// 登录日志：包含 IP / 设备名 / Country 等个人信息，按 GDPR 右擦除。
		if len(s.state.LoginLogs) > 0 {
			filtered := s.state.LoginLogs[:0]
			for _, log := range s.state.LoginLogs {
				if log.UID != uid {
					filtered = append(filtered, log)
				}
			}
			s.state.LoginLogs = filtered
		}

		// 播放记录。
		if len(s.state.PlaybackRecords) > 0 {
			filtered := s.state.PlaybackRecords[:0]
			for _, p := range s.state.PlaybackRecords {
				if p.UID != uid {
					filtered = append(filtered, p)
				}
			}
			s.state.PlaybackRecords = filtered
		}

		// 待审/已审的换绑请求：业务对象随用户消亡。
		// 注意：仅清理"作为申请者 UID"的记录；ReviewerUID 字段保留（审计轨迹）。
		for id, req := range s.state.RebindRequests {
			if req.UID == uid {
				delete(s.state.RebindRequests, id)
			}
		}

		// 绑定码（注册/绑定 telegram 流程的临时 ticket）。
		for code, bc := range s.state.BindCodes {
			if bc.UID == uid {
				delete(s.state.BindCodes, code)
			}
		}

		// RegCode：删除用户时清理所有引用并回退 UseCount，释放被占用的码额度。
		for code, rc := range s.state.RegCodes {
			dirty := false
			if rc.UsedBy == uid {
				rc.UsedBy = 0
				dirty = true
			}
			if len(rc.UsedByUIDs) > 0 {
				pruned := rc.UsedByUIDs[:0]
				for _, u := range rc.UsedByUIDs {
					if u == uid {
						if rc.UseCount > 0 {
							rc.UseCount--
						}
						continue
					}
					pruned = append(pruned, u)
				}
				if len(pruned) != len(rc.UsedByUIDs) {
					if len(pruned) == 0 {
						rc.UsedByUIDs = nil
					} else {
						rc.UsedByUIDs = pruned
					}
					dirty = true
				}
			}
			if dirty {
				if !rc.Active && rc.UseCountLimit != -1 && rc.UseCount < rc.UseCountLimit {
					rc.Active = true
				}
				s.state.RegCodes[code] = rc
			}
		}

		// 公告作者匿名化：公告本体不删，只清掉 CreatedByUID 引用。
		for id, ann := range s.state.Announcements {
			if ann.CreatedByUID == uid {
				ann.CreatedByUID = 0
				s.state.Announcements[id] = ann
			}
		}

		// 工单：用户删除时连带删除其提交的工单。
		for id, ticket := range s.state.Tickets {
			if ticket.UID == uid {
				delete(s.state.Tickets, id)
			}
		}

		for key, entry := range s.state.BangumiCollectionCache {
			if entry.UID == uid {
				delete(s.state.BangumiCollectionCache, key)
			}
		}

		return nil
	})
}

func (s *Store) ListUsers() []User {
	return s.UsersMatching(0, nil)
}

// UsersMatching 按 UID 升序遍历所有用户，对每个用户调用 matches；matches 为 nil
// 等价于全量返回。命中即 append 到 out，超过 limit 时提前返回（limit<=0 表示不限）。
// 相较 ListUsers 先构造整份 []User 切片再让调用方二次过滤，本方法在满足 matches
// 的用户达到 limit 时即可停止，省去无关用户的一次整份切片拷贝与残留分配。
//
// 顺序与 ListUsers 完全一致（userUIDs 缓存已按 UID 升序；缓存失效时回退到
// map 全量 + 排序，同样升序）。调用方必须在 store 锁外使用返回的 User 副本。
func (s *Store) UsersMatching(limit int, matches func(User) bool) []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = len(s.state.Users)
	}
	if len(s.userUIDs) == len(s.state.Users) {
		out := make([]User, 0, min(limit, len(s.userUIDs)))
		for _, uid := range s.userUIDs {
			u, ok := s.state.Users[uid]
			if !ok {
				continue
			}
			if matches != nil && !matches(u) {
				continue
			}
			out = append(out, u)
			if len(out) >= limit {
				return out
			}
		}
		return out
	}

	// 索引修复回退：userUIDs 暂时不可用时仍保序（按 UID 升序）。
	users := make([]User, 0, len(s.state.Users))
	for _, u := range s.state.Users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].UID < users[j].UID })
	out := make([]User, 0, min(limit, len(users)))
	for _, u := range users {
		if matches != nil && !matches(u) {
			continue
		}
		out = append(out, u)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

// UserUIDsMatching is the UID-only counterpart to UsersMatching. It keeps the
// same UID ordering and full-match count, but never allocates a []User backing
// array. Batch selection paths should use it when they only need IDs for later
// per-user authorization or mutation.
func (s *Store) UserUIDsMatching(limit int, matches func(User) bool) ([]int64, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	capacity := len(s.state.Users)
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	uids := make([]int64, 0, capacity)
	matched := 0
	appendMatch := func(u User) {
		if matches != nil && !matches(u) {
			return
		}
		matched++
		if limit <= 0 || len(uids) < limit {
			uids = append(uids, u.UID)
		}
	}

	if len(s.userUIDs) == len(s.state.Users) {
		for _, uid := range s.userUIDs {
			if u, ok := s.state.Users[uid]; ok {
				appendMatch(u)
			}
		}
		return uids, matched
	}

	// Index repair fallback: sort only the compact UID list rather than copying
	// every User value before applying the predicate.
	orderedUIDs := make([]int64, 0, len(s.state.Users))
	for uid := range s.state.Users {
		orderedUIDs = append(orderedUIDs, uid)
	}
	sort.Slice(orderedUIDs, func(i, j int) bool { return orderedUIDs[i] < orderedUIDs[j] })
	for _, uid := range orderedUIDs {
		appendMatch(s.state.Users[uid])
	}
	return uids, matched
}

type UserIdentitySearchField uint8

const (
	UserIdentitySearchAny UserIdentitySearchField = iota
	UserIdentitySearchUID
	UserIdentitySearchUsername
	UserIdentitySearchTelegramID
	UserIdentitySearchTelegramUsername
)

// SearchUsers returns UID-ordered broad matches without first copying and
// sorting the complete user set.
func (s *Store) SearchUsers(query string, limit int) []User {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	numericID, numericQueryErr := strconv.ParseInt(query, 10, 64)
	numericQuery := numericQueryErr == nil && strconv.FormatInt(numericID, 10) == query
	matches := func(u User) bool {
		if numericQuery && (u.UID == numericID || (u.TelegramID != 0 && u.TelegramID == numericID)) {
			return true
		}
		return userSearchFieldMatches(u.Username, query) ||
			userSearchFieldMatches(u.Email, query) ||
			userSearchFieldMatches(u.TelegramUsername, query) ||
			userSearchFieldMatches(u.EmbyUsername, query) ||
			userSearchFieldMatches(u.EmbyID, query)
	}
	return s.searchUsersMatching(limit, matches)
}

// SearchUsersByIdentity is the narrow Telegram administration search. It
// intentionally searches only UID, Web username, Telegram ID, and Telegram
// username so an ambiguous panel query does not disclose or accidentally match
// email/Emby identity fields.
func (s *Store) SearchUsersByIdentity(query string, field UserIdentitySearchField, limit int) []User {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	telegramQuery := strings.TrimPrefix(query, "@")
	matches := func(u User) bool {
		switch field {
		case UserIdentitySearchUID:
			return strings.Contains(strconv.FormatInt(u.UID, 10), query)
		case UserIdentitySearchUsername:
			return userSearchFieldMatches(u.Username, query)
		case UserIdentitySearchTelegramID:
			return u.TelegramID != 0 && strings.Contains(strconv.FormatInt(u.TelegramID, 10), query)
		case UserIdentitySearchTelegramUsername:
			return telegramQuery != "" && userSearchFieldMatches(normalizeTelegramUsername(u.TelegramUsername), telegramQuery)
		default:
			return strings.Contains(strconv.FormatInt(u.UID, 10), query) ||
				userSearchFieldMatches(u.Username, query) ||
				(u.TelegramID != 0 && strings.Contains(strconv.FormatInt(u.TelegramID, 10), query)) ||
				(telegramQuery != "" && userSearchFieldMatches(normalizeTelegramUsername(u.TelegramUsername), telegramQuery))
		}
	}
	return s.searchUsersMatching(limit, matches)
}

func (s *Store) searchUsersMatching(limit int, matches func(User) bool) []User {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, min(limit, len(s.state.Users)))
	if len(s.userUIDs) == len(s.state.Users) {
		for _, uid := range s.userUIDs {
			u, ok := s.state.Users[uid]
			if ok && matches(u) {
				out = append(out, u)
				if len(out) >= limit {
					return out
				}
			}
		}
		return out
	}

	// Index repair fallback: preserve the public UID ordering even if the cached
	// UID slice is temporarily unavailable.
	users := make([]User, 0, len(s.state.Users))
	for _, u := range s.state.Users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].UID < users[j].UID })
	for _, u := range users {
		if matches(u) {
			out = append(out, u)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func userSearchFieldMatches(field, query string) bool {
	return field != "" && strings.Contains(strings.ToLower(field), query)
}

// rebuildUserIndexes 从当前 Users map 一次性重建所有用户身份字段索引。
// 在 Open / OpenPostgres 加载 state 后调用，无需持锁（调用方已持有写锁或尚未暴露引用）。
func (s *Store) rebuildUserIndexes() {
	usernameMap := make(map[string]int64, len(s.state.Users))
	emailMap := make(map[string]int64, len(s.state.Users))
	verifiedEmailMap := make(map[string]int64, len(s.state.Users))
	telegramIDMap := make(map[int64]int64, len(s.state.Users))
	embyIDMap := make(map[string]int64, len(s.state.Users))
	legacyAPIKeyHashMap := make(map[string]int64, len(s.state.Users))
	userUIDs := make([]int64, 0, len(s.state.Users))
	for _, u := range s.state.Users {
		userUIDs = append(userUIDs, u.UID)
		if key := normalizeUsernameKey(u.Username); key != "" {
			usernameMap[key] = u.UID
		}
		if key := normalizeEmailKey(u.Email); key != "" {
			emailMap[key] = u.UID
			if u.EmailVerified {
				verifiedEmailMap[key] = u.UID
			}
		}
		if u.TelegramID != 0 {
			telegramIDMap[u.TelegramID] = u.UID
		}
		if u.EmbyID != "" {
			embyIDMap[u.EmbyID] = u.UID
		}
		if u.LegacyAPIKeyStatus && u.LegacyAPIKeyHash != "" {
			legacyAPIKeyHashMap[u.LegacyAPIKeyHash] = u.UID
		}
	}
	sort.Slice(userUIDs, func(i, j int) bool { return userUIDs[i] < userUIDs[j] })
	// apiKeyHashMap 索引所有 key（含 Enabled=false）——hash 恒定不随 Enabled 变，读路径
	// 复核时再判 Enabled，故 Enable/Disable 切换无需维护索引。
	apiKeyHashMap := make(map[string]int64, len(s.state.APIKeys))
	for id, k := range s.state.APIKeys {
		if k.Hash != "" {
			apiKeyHashMap[k.Hash] = id
		}
	}
	s.usernameMap = usernameMap
	s.emailMap = emailMap
	s.verifiedEmailMap = verifiedEmailMap
	s.telegramIDMap = telegramIDMap
	s.embyIDMap = embyIDMap
	s.userUIDs = userUIDs
	s.apiKeyHashMap = apiKeyHashMap
	s.legacyAPIKeyHashMap = legacyAPIKeyHashMap
}

func (s *Store) maintainUserIndexes(oldUser, newUser User, uid int64) {
	if s.usernameMap == nil || s.emailMap == nil || s.verifiedEmailMap == nil || s.telegramIDMap == nil || s.embyIDMap == nil || s.userUIDs == nil {
		s.rebuildUserIndexes()
	}
	s.maintainUserOrderIndex(oldUser.UID, newUser.UID, uid)
	s.maintainUsernameIndex(oldUser.Username, newUser.Username, uid)
	s.maintainEmailIndexes(oldUser, newUser, uid)
	s.maintainTelegramIDIndex(oldUser.TelegramID, newUser.TelegramID, uid)
	s.maintainEmbyIDIndex(oldUser.EmbyID, newUser.EmbyID, uid)
	s.maintainLegacyAPIKeyIndex(oldUser, newUser, uid)
}

// maintainLegacyAPIKeyIndex 在用户变更后增量维护「遗留 API Key hash → UID」索引。
// 遗留 key 的 hash 存在 User 上，仅经 UpdateUser 变动（设置 / 清除 / 关闭），因此在
// 用户身份索引维护链里一并更新。调用方必须已持有 s.mu 写锁。
func (s *Store) maintainLegacyAPIKeyIndex(oldUser, newUser User, uid int64) {
	if s.legacyAPIKeyHashMap == nil {
		s.rebuildUserIndexes()
	}
	// 旧 hash 若曾登记在本 uid 名下，先摘除（关闭 / 清除 / 轮换都要先撤旧值）。
	if oldUser.LegacyAPIKeyStatus && oldUser.LegacyAPIKeyHash != "" {
		if s.legacyAPIKeyHashMap[oldUser.LegacyAPIKeyHash] == uid {
			delete(s.legacyAPIKeyHashMap, oldUser.LegacyAPIKeyHash)
		}
	}
	// 新状态启用且有 hash 才登记；关闭态（Status=false）一律不进索引。
	if newUser.LegacyAPIKeyStatus && newUser.LegacyAPIKeyHash != "" {
		s.legacyAPIKeyHashMap[newUser.LegacyAPIKeyHash] = uid
	}
}

func (s *Store) maintainUserOrderIndex(oldUID, newUID, uid int64) {
	if s.userUIDs == nil {
		s.rebuildUserIndexes()
	}
	if oldUID != 0 && newUID == 0 {
		index := sort.Search(len(s.userUIDs), func(i int) bool { return s.userUIDs[i] >= uid })
		if index < len(s.userUIDs) && s.userUIDs[index] == uid {
			s.userUIDs = append(s.userUIDs[:index], s.userUIDs[index+1:]...)
		}
		return
	}
	if oldUID == 0 && newUID != 0 {
		index := sort.Search(len(s.userUIDs), func(i int) bool { return s.userUIDs[i] >= uid })
		if index < len(s.userUIDs) && s.userUIDs[index] == uid {
			return
		}
		s.userUIDs = append(s.userUIDs, 0)
		copy(s.userUIDs[index+1:], s.userUIDs[index:])
		s.userUIDs[index] = uid
	}
}

func (s *Store) maintainUsernameIndex(oldUsername, newUsername string, uid int64) {
	if s.usernameMap == nil {
		s.rebuildUserIndexes()
	}
	if key := normalizeUsernameKey(oldUsername); key != "" {
		if s.usernameMap[key] == uid {
			delete(s.usernameMap, key)
		}
	}
	if key := normalizeUsernameKey(newUsername); key != "" {
		s.usernameMap[key] = uid
	}
}

func (s *Store) maintainEmailIndexes(oldUser, newUser User, uid int64) {
	if s.emailMap == nil || s.verifiedEmailMap == nil {
		s.rebuildUserIndexes()
	}
	oldKey := normalizeEmailKey(oldUser.Email)
	if oldKey != "" {
		if s.emailMap[oldKey] == uid {
			delete(s.emailMap, oldKey)
		}
		if oldUser.EmailVerified && s.verifiedEmailMap[oldKey] == uid {
			delete(s.verifiedEmailMap, oldKey)
		}
	}
	newKey := normalizeEmailKey(newUser.Email)
	if newKey != "" {
		s.emailMap[newKey] = uid
		if newUser.EmailVerified {
			s.verifiedEmailMap[newKey] = uid
		}
	}
}

// maintainTelegramIDIndex 在用户变更后增量维护 telegramID → UID 索引。
// 调用方须持有 s.mu 写锁。索引为空时自动从 state 重建。
func (s *Store) maintainTelegramIDIndex(oldTGID, newTGID int64, uid int64) {
	if s.telegramIDMap == nil {
		s.rebuildUserIndexes()
	}
	if oldTGID != 0 {
		if s.telegramIDMap[oldTGID] == uid {
			delete(s.telegramIDMap, oldTGID)
		}
	}
	if newTGID != 0 {
		s.telegramIDMap[newTGID] = uid
	}
}

// maintainEmbyIDIndex 在用户变更后增量维护 embyID → UID 索引。
// 调用方必须已持有 s.mu 写锁。
func (s *Store) maintainEmbyIDIndex(oldEmbyID, newEmbyID string, uid int64) {
	if s.embyIDMap == nil {
		s.rebuildUserIndexes()
	}
	if oldEmbyID != "" {
		if s.embyIDMap[oldEmbyID] == uid {
			delete(s.embyIDMap, oldEmbyID)
		}
	}
	if newEmbyID != "" {
		s.embyIDMap[newEmbyID] = uid
	}
}

func (s *Store) FindUserByTelegramID(telegramID int64) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.telegramIDMap == nil {
		// 索引未初始化（测试等路径），回退到全表扫描
		for _, u := range s.state.Users {
			if telegramID != 0 && u.TelegramID == telegramID {
				return u, true
			}
		}
		return User{}, false
	}
	if uid, ok := s.telegramIDMap[telegramID]; ok {
		u, found := s.state.Users[uid]
		if found && u.TelegramID == telegramID {
			return u, true
		}
	}
	return User{}, false
}

// UpdateTelegramUsernameIfBound refreshes the username learned from a
// Telegram update only while the same Telegram ID is still bound. The final
// identity check runs inside the version-guarded mutation so a concurrent Web
// unbind or rebind cannot write a stale username to another account.
func (s *Store) UpdateTelegramUsernameIfBound(telegramID int64, rawUsername string) (User, bool, error) {
	if telegramID <= 0 {
		return User{}, false, nil
	}
	username := strings.TrimPrefix(strings.TrimSpace(rawUsername), "@")
	if username == "" {
		return User{}, false, nil
	}

	s.mu.RLock()
	uid, indexed := s.telegramIDMap[telegramID]
	current, found := s.state.Users[uid]
	if !indexed {
		for candidateUID, candidate := range s.state.Users {
			if candidate.TelegramID == telegramID {
				uid, current, indexed, found = candidateUID, candidate, true, true
				break
			}
		}
	}
	if !indexed || !found || current.TelegramID != telegramID || current.TelegramUsername == username {
		s.mu.RUnlock()
		return current, false, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	changed := false
	err := s.mutateAndSaveLocked(func() error {
		uid, indexed := s.telegramIDMap[telegramID]
		current, found := s.state.Users[uid]
		if !indexed {
			for candidateUID, candidate := range s.state.Users {
				if candidate.TelegramID == telegramID {
					uid, current, indexed, found = candidateUID, candidate, true, true
					break
				}
			}
		}
		if !indexed || !found || current.TelegramID != telegramID {
			return nil
		}
		updated = current
		if current.TelegramUsername == username {
			return nil
		}
		old := current
		current.TelegramUsername = username
		s.state.Users[uid] = current
		s.maintainUserIndexes(old, current, uid)
		updated = current
		changed = true
		return nil
	})
	if err != nil {
		return User{}, false, err
	}
	return updated, changed, nil
}

func (s *Store) CreateAPIKey(k APIKey) (APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		k.ID = s.state.NextAPIKeyID
		s.state.NextAPIKeyID++
		if k.CreatedAt == 0 {
			k.CreatedAt = time.Now().Unix()
		}
		if k.RateLimit <= 0 {
			k.RateLimit = 100
		}
		k.Enabled = true
		s.state.APIKeys[k.ID] = k
		// 增量登记到 hash 索引：CreateAPIKey 在 mutate 闭包内分配新 ID，refreshLocked
		// 阶段的重建看不到它。save 失败时 restoreStateLocked 会整表重建，天然撤销此登记。
		if s.apiKeyHashMap == nil {
			s.rebuildUserIndexes()
		} else if k.Hash != "" {
			s.apiKeyHashMap[k.Hash] = k.ID
		}
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}
	return k, nil
}

func (s *Store) ListAPIKeys(uid int64) []APIKey {
	s.mu.RLock()
	keys := make([]APIKey, 0)
	for _, k := range s.state.APIKeys {
		if k.UID == uid {
			keys = append(keys, k)
		}
	}
	s.mu.RUnlock()
	// 合并尚未落盘的内存增量，让展示的调用次数/最后使用时间保持最新
	// （否则刚发生的调用要等到下个 flush 周期才可见）。
	s.apiKeyUsageMu.Lock()
	for i := range keys {
		if d, ok := s.apiKeyUsage[keys[i].ID]; ok {
			keys[i].RequestCount += d.count
			if d.lastUsed > keys[i].LastUsed {
				keys[i].LastUsed = d.lastUsed
			}
		}
	}
	s.apiKeyUsageMu.Unlock()
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	return keys
}

func (s *Store) FindAPIKeyByHash(hash string) (APIKey, User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hashBytes := []byte(hash)
	// 现代 key：hash → ID 索引命中后回 s.state 复核。命中项仍做常量时间比对以保留
	// 「hash 比对不泄露时序」的原不变量；Enabled 判定不携带秘密值可普通短路。索引陈旧
	// 只会让此处 ok=false（假未命中，下轮 refresh 自愈），绝不放行已删/轮换的 key。
	if s.apiKeyHashMap != nil {
		if id, ok := s.apiKeyHashMap[hash]; ok {
			if k, exists := s.state.APIKeys[id]; exists && k.Enabled &&
				subtle.ConstantTimeCompare([]byte(k.Hash), hashBytes) == 1 {
				u, uok := s.state.Users[k.UID]
				return k, u, uok
			}
		}
	} else {
		for _, k := range s.state.APIKeys { // 索引未建（异常兜底）：退回全表扫描保证正确性。
			if k.Enabled && subtle.ConstantTimeCompare([]byte(k.Hash), hashBytes) == 1 {
				u, ok := s.state.Users[k.UID]
				return k, u, ok
			}
		}
	}
	// 遗留 key：hash → UID 索引命中后回 s.state 复核 Status + 常量时间比对。
	if s.legacyAPIKeyHashMap != nil {
		if uid, ok := s.legacyAPIKeyHashMap[hash]; ok {
			if u, exists := s.state.Users[uid]; exists && u.LegacyAPIKeyStatus &&
				subtle.ConstantTimeCompare([]byte(u.LegacyAPIKeyHash), hashBytes) == 1 {
				return APIKey{UID: u.UID, Enabled: true, Permissions: u.LegacyPermissions, RateLimit: 100}, u, true
			}
		}
	} else {
		for _, u := range s.state.Users { // 索引未建兜底。
			if u.LegacyAPIKeyStatus && subtle.ConstantTimeCompare([]byte(u.LegacyAPIKeyHash), hashBytes) == 1 {
				return APIKey{UID: u.UID, Enabled: true, Permissions: u.LegacyPermissions, RateLimit: 100}, u, true
			}
		}
	}
	return APIKey{}, User{}, false
}

func (s *Store) UpdateAPIKey(uid, id int64, fn func(*APIKey) error) (APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated APIKey
	err := s.mutateAndSaveLocked(func() error {
		k, ok := s.state.APIKeys[id]
		if !ok || k.UID != uid {
			return ErrNotFound
		}
		if err := fn(&k); err != nil {
			return err
		}
		s.state.APIKeys[id] = k
		updated = k
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}
	return updated, nil
}

// RecordAPIKeyUse 只在内存里累加调用增量，不触碰 s.state / 不落盘。真正的持久化由
// flushAPIKeyUsageLocked 批量完成（后台协程周期性触发 + Close 时兜底）。因此它不再
// 返回 ErrNotFound——id 是否存在留待 flush 时对照 s.state 校验（不存在的增量直接丢弃）。
func (s *Store) RecordAPIKeyUse(id int64) error {
	now := time.Now().Unix()
	s.apiKeyUsageMu.Lock()
	if s.apiKeyUsage == nil {
		s.apiKeyUsage = map[int64]apiKeyUsageDelta{}
	}
	d := s.apiKeyUsage[id]
	d.count++
	if now > d.lastUsed {
		d.lastUsed = now
	}
	s.apiKeyUsage[id] = d
	s.apiKeyUsageMu.Unlock()
	return nil
}

// drainAPIKeyUsage 原子取走当前累积的增量并清空缓冲，供 flush 使用。
func (s *Store) drainAPIKeyUsage() map[int64]apiKeyUsageDelta {
	s.apiKeyUsageMu.Lock()
	defer s.apiKeyUsageMu.Unlock()
	if len(s.apiKeyUsage) == 0 {
		return nil
	}
	pending := s.apiKeyUsage
	s.apiKeyUsage = map[int64]apiKeyUsageDelta{}
	return pending
}

// mergeBackAPIKeyUsage 在 flush 落盘失败时把取走的增量并回缓冲，避免丢计数；
// 若期间又有新增量，二者相加、lastUsed 取较大值。
func (s *Store) mergeBackAPIKeyUsage(pending map[int64]apiKeyUsageDelta) {
	if len(pending) == 0 {
		return
	}
	s.apiKeyUsageMu.Lock()
	defer s.apiKeyUsageMu.Unlock()
	if s.apiKeyUsage == nil {
		s.apiKeyUsage = map[int64]apiKeyUsageDelta{}
	}
	for id, d := range pending {
		cur := s.apiKeyUsage[id]
		cur.count += d.count
		if d.lastUsed > cur.lastUsed {
			cur.lastUsed = d.lastUsed
		}
		s.apiKeyUsage[id] = cur
	}
}

// flushAPIKeyUsage 把累积的调用增量一次性合并进 state 并落盘（单次 mutateAndSaveLocked）。
// 无待落盘增量时直接返回、不产生任何写。落盘失败时把增量并回缓冲留待下次重试。
func (s *Store) flushAPIKeyUsage() error {
	pending := s.drainAPIKeyUsage()
	if len(pending) == 0 {
		return nil
	}
	s.mu.Lock()
	err := s.mutateAndSaveLocked(func() error {
		for id, d := range pending {
			k, ok := s.state.APIKeys[id]
			if !ok {
				// 密钥已被删除：丢弃其增量。
				continue
			}
			k.RequestCount += d.count
			if d.lastUsed > k.LastUsed {
				k.LastUsed = d.lastUsed
			}
			s.state.APIKeys[id] = k
		}
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		s.mergeBackAPIKeyUsage(pending)
		return err
	}
	return nil
}

// startAPIKeyUsageFlusher 启动后台协程，按固定周期批量落盘 API Key 调用统计。
func (s *Store) startAPIKeyUsageFlusher() {
	s.apiKeyFlushStop = make(chan struct{})
	s.apiKeyFlushDone = make(chan struct{})
	go func() {
		defer close(s.apiKeyFlushDone)
		ticker := time.NewTicker(apiKeyUsageFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.flushAPIKeyUsage()
			case <-s.apiKeyFlushStop:
				return
			}
		}
	}()
}

// stopAPIKeyUsageFlusher 停止后台协程并做最后一次 flush，确保关闭前不丢计数。幂等。
func (s *Store) stopAPIKeyUsageFlusher() {
	s.apiKeyFlushOnce.Do(func() {
		if s.apiKeyFlushStop != nil {
			close(s.apiKeyFlushStop)
		}
		if s.apiKeyFlushDone != nil {
			<-s.apiKeyFlushDone
		}
		_ = s.flushAPIKeyUsage()
	})
}

func (s *Store) DeleteAPIKey(uid, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		k, ok := s.state.APIKeys[id]
		if !ok || k.UID != uid {
			return ErrNotFound
		}
		delete(s.state.APIKeys, id)
		// 同步摘除 hash 索引；仅当该 hash 仍指向本 id 时删除，避免误删同 hash 的其它登记
		// （理论上 hash 唯一，此判定纯属防御）。save 失败回滚亦会整表重建。
		if s.apiKeyHashMap != nil && k.Hash != "" && s.apiKeyHashMap[k.Hash] == id {
			delete(s.apiKeyHashMap, k.Hash)
		}
		return nil
	})
}

func (s *Store) CreateMediaRequest(r MediaRequest) (MediaRequest, error) {
	return s.CreateMediaRequestWithOptions(r, MediaRequestCreateOptions{})
}

func (s *Store) CreateMediaRequestWithOptions(r MediaRequest, opts MediaRequestCreateOptions) (MediaRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 冲突检查产出 existing 副本要带回给调用方（handler 用它给前端返回
	// "已有同源同集的活跃请求"），这种"非 nil error 同时携带 payload"
	// 的语义 mutateAndSaveLocked 不直接支持——通过闭包外的捕获变量传出。
	var conflict MediaRequest
	var conflictHit bool
	err := s.mutateAndSaveLocked(func() error {
		if opts.UserActiveLimit > 0 && s.countActiveMediaRequestsLocked(r.UID) >= opts.UserActiveLimit {
			return ErrMediaRequestUserActiveLimit
		}
		if opts.GlobalActiveLimit > 0 && s.countActiveMediaRequestsLocked(0) >= opts.GlobalActiveLimit {
			return ErrMediaRequestGlobalActiveLimit
		}
		if !mediaRequestInventoryIssue(r) {
			for _, existing := range s.state.MediaRequests {
				if strings.EqualFold(existing.Source, r.Source) && existing.MediaID == r.MediaID && existing.Season == r.Season && isActiveMediaStatus(existing.Status) {
					conflict = existing
					conflictHit = true
					return ErrConflict
				}
			}
		}
		now := time.Now().Unix()
		r.ID = s.state.NextRequestID
		s.state.NextRequestID++
		if r.RequireKey == "" {
			r.RequireKey = randomKey("req", r.ID, now)
		}
		if strings.TrimSpace(r.Status) == "" {
			r.Status = MediaRequestStatusUnhandled
		} else {
			r.Status = NormalizeMediaRequestStatus(r.Status)
			if r.Status == "" {
				return ErrInvalid
			}
		}
		r.CreatedAt = now
		r.UpdatedAt = now
		if r.Revision <= 0 {
			r.Revision = 1
		}
		s.state.MediaRequests[r.ID] = r
		return nil
	})
	if conflictHit {
		return conflict, ErrConflict
	}
	if err != nil {
		return MediaRequest{}, err
	}
	return r, nil
}

func mediaRequestInventoryIssue(r MediaRequest) bool {
	if r.MediaInfo == nil {
		return false
	}
	value, ok := r.MediaInfo["inventory_issue"]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	default:
		return false
	}
}

func (s *Store) ActiveMediaRequestCount(uid int64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countActiveMediaRequestsLocked(uid)
}

// ActiveMediaRequestCountTotal 统计全站正在处理（未完成 / 未拒绝）的求片数。
// 用于配置里 max_concurrent_requests_global 全局并发上限的判定。
// 不区分用户 / Telegram ID，单纯按 Status 是否仍是活跃流程内（pending / accepted /
// downloading）统计。与 ActiveMediaRequestCount(uid) 共用 isActiveMediaStatus，
// 保证"全局看见的活跃集合 == 各 UID 累加"。
func (s *Store) ActiveMediaRequestCountTotal() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countActiveMediaRequestsLocked(0)
}

func (s *Store) countActiveMediaRequestsLocked(uid int64) int {
	count := 0
	for _, r := range s.state.MediaRequests {
		if (uid == 0 || r.UID == uid) && isActiveMediaStatus(r.Status) {
			count++
		}
	}
	return count
}

func (s *Store) MediaRequest(id int64) (MediaRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.state.MediaRequests[id]
	return r, ok
}

func (s *Store) ListMediaRequests(uid int64, all bool) []MediaRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MediaRequest, 0)
	for _, r := range s.state.MediaRequests {
		if all || r.UID == uid {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) CountMediaRequests(uid int64, all bool) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, r := range s.state.MediaRequests {
		if all || r.UID == uid {
			count++
		}
	}
	return count
}

func (s *Store) FindMediaRequestByKey(key string) (MediaRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.state.MediaRequests {
		if r.RequireKey == key {
			return r, true
		}
	}
	return MediaRequest{}, false
}

func (s *Store) UpdateMediaRequest(id int64, fn func(*MediaRequest) error) (MediaRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated MediaRequest
	err := s.mutateAndSaveLocked(func() error {
		r, ok := s.state.MediaRequests[id]
		if !ok {
			return ErrNotFound
		}
		if err := fn(&r); err != nil {
			return err
		}
		r.Revision++
		r.UpdatedAt = time.Now().Unix()
		s.state.MediaRequests[id] = r
		updated = r
		return nil
	})
	if err != nil {
		return MediaRequest{}, err
	}
	return updated, nil
}

func (s *Store) DeleteMediaRequest(id int64) error {
	_, err := s.DeleteMediaRequestIfRevision(id, nil)
	return err
}

func (s *Store) DeleteMediaRequestIfRevision(id int64, expectedRevision *int64) (MediaRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted MediaRequest
	err := s.mutateAndSaveLocked(func() error {
		request, ok := s.state.MediaRequests[id]
		if !ok {
			return ErrNotFound
		}
		if expectedRevision != nil && request.Revision != *expectedRevision {
			return ErrConflict
		}
		deleted = request
		delete(s.state.MediaRequests, id)
		return nil
	})
	if err != nil {
		return MediaRequest{}, err
	}
	return deleted, nil
}

func (s *Store) UpsertBindCode(code BindCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.BindCodes[code.Code] = code
		return nil
	})
}

func (s *Store) BindCode(code string) (BindCode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.state.BindCodes[code]
	return b, ok
}

func (s *Store) ConfirmBindCodeAtomic(code string, telegramID int64, telegramUsername string, now int64) (BindCode, User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var confirmed BindCode
	var updated User
	var userUpdated bool
	err := s.mutateAndSaveLocked(func() error {
		bind, ok := s.state.BindCodes[code]
		if !ok {
			return ErrNotFound
		}
		if now == 0 {
			now = time.Now().Unix()
		}
		if bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
			delete(s.state.BindCodes, code)
			return ErrExpired
		}
		if telegramID == 0 {
			return ErrConflict
		}
		if bind.Confirmed && bind.TelegramID != 0 {
			if bind.TelegramID != telegramID {
				return ErrConflict
			}
			confirmed = bind
			return nil
		}
		if s.telegramIDTakenLocked(telegramID, bind.UID) {
			return ErrConflict
		}
		bind.Confirmed = true
		bind.TelegramID = telegramID
		bind.TelegramUsername = strings.TrimSpace(telegramUsername)
		confirmed = bind
		if bind.UID != 0 {
			u, ok := s.state.Users[bind.UID]
			if !ok {
				return ErrNotFound
			}
			old := u
			u.TelegramID = telegramID
			u.TelegramUsername = bind.TelegramUsername
			if err := s.userIdentityConflictLocked(old, u, u.UID); err != nil {
				return ErrConflict
			}
			s.state.Users[u.UID] = u
			s.maintainUserIndexes(old, u, u.UID)
			updated = u
			userUpdated = true
			delete(s.state.BindCodes, code)
			return nil
		}
		s.state.BindCodes[code] = bind
		return nil
	})
	if err != nil {
		return BindCode{}, User{}, false, err
	}
	return confirmed, updated, userUpdated, nil
}

func (s *Store) DeleteBindCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.BindCodes[code]; !ok {
			return ErrNotFound
		}
		delete(s.state.BindCodes, code)
		return nil
	})
}

func (s *Store) CleanupExpiredBindCodes(now int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	err := s.mutateAndSaveLocked(func() error {
		for code, bind := range s.state.BindCodes {
			if bind.ExpiresAt > 0 && bind.ExpiresAt <= now {
				delete(s.state.BindCodes, code)
				deleted++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// RepairLegacyTelegramBindResidue removes bind codes persisted by old builds.
// Current Telegram bind codes live only in the in-memory bindStatusHub; a
// persisted confirmed register code can otherwise make Telegram look confirmed
// while no User row exists.
func (s *Store) RepairLegacyTelegramBindResidue() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.BindCodes) == 0 {
		return 0, nil
	}
	deleted := 0
	err := s.mutateAndSaveLocked(func() error {
		for code := range s.state.BindCodes {
			delete(s.state.BindCodes, code)
			deleted++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if deleted > 0 {
		s.rebuildUserIndexes()
	}
	return deleted, nil
}

type RegistrationResidueRepair struct {
	ClearedBoundPending        int
	RestoredGrantLocks         int
	RestoredPendingEntitlement int
}

func (r RegistrationResidueRepair) Total() int {
	return r.ClearedBoundPending + r.RestoredGrantLocks + r.RestoredPendingEntitlement
}

// RepairRegistrationResidue fixes historical registration/Emby entitlement
// residues left by older flows. It is intentionally conservative: it does not
// create invite relations or consume new codes, only reconciles user-side grant
// fields from already-recorded code usage and clears impossible bound+pending
// states.
func (s *Store) RepairRegistrationResidue() (RegistrationResidueRepair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result RegistrationResidueRepair
	err := s.mutateAndSaveLocked(func() error {
		result = RegistrationResidueRepair{}
		grants := map[int64]struct {
			source string
			code   string
			days   int
		}{}
		addGrant := func(uid int64, source, code string, days int) {
			if uid == 0 {
				return
			}
			if _, exists := grants[uid]; exists {
				return
			}
			grants[uid] = struct {
				source string
				code   string
				days   int
			}{source: source, code: code, days: days}
		}
		for _, reg := range s.state.RegCodes {
			if reg.IsDecoy || (reg.Type != 1 && reg.Type != 3) {
				continue
			}
			days := normalizeRegistrationGrantDays(reg.Days)
			if reg.Type == 3 {
				days = -1
			}
			addGrant(reg.UsedBy, "regcode", reg.Code, days)
			for _, uid := range reg.UsedByUIDs {
				addGrant(uid, "regcode", reg.Code, days)
			}
		}
		for _, invite := range s.state.InviteCodes {
			days := normalizeRegistrationGrantDays(invite.Days)
			addGrant(invite.UsedByUID, "invite", invite.Code, days)
		}
		for _, rel := range s.state.InviteRelations {
			if rel.ChildUID != 0 && strings.TrimSpace(rel.Code) != "" {
				days := 30
				if invite, ok := s.state.InviteCodes[rel.Code]; ok {
					days = normalizeRegistrationGrantDays(invite.Days)
				}
				addGrant(rel.ChildUID, "invite", rel.Code, days)
			}
		}
		for uid, u := range s.state.Users {
			changed := false
			if strings.TrimSpace(u.EmbyID) != "" && (u.PendingEmby || u.PendingEmbyDays != nil) {
				u.PendingEmby = false
				u.PendingEmbyDays = nil
				result.ClearedBoundPending++
				changed = true
			}
			if grant, ok := grants[uid]; ok {
				if !u.EmbyGrantLocked || strings.TrimSpace(u.RegistrationSource) == "" || strings.TrimSpace(u.RegistrationCode) == "" {
					u.EmbyGrantLocked = true
					if strings.TrimSpace(u.RegistrationSource) == "" {
						u.RegistrationSource = grant.source
					}
					if strings.TrimSpace(u.RegistrationCode) == "" {
						u.RegistrationCode = grant.code
					}
					result.RestoredGrantLocks++
					changed = true
				}
				if strings.TrimSpace(u.EmbyID) == "" && !u.PendingEmby {
					days := grant.days
					u.PendingEmby = true
					u.PendingEmbyDays = &days
					result.RestoredPendingEntitlement++
					changed = true
				}
			}
			if changed {
				s.state.Users[uid] = u
			}
		}
		return nil
	})
	if err != nil {
		return RegistrationResidueRepair{}, err
	}
	return result, nil
}

func normalizeRegistrationGrantDays(days int) int {
	if days == 0 {
		return 30
	}
	return days
}

func (s *Store) UpsertAnnouncement(a Announcement) (Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		now := time.Now().Unix()
		if a.ID == 0 {
			a.ID = s.state.NextAnnouncementID
			s.state.NextAnnouncementID++
			a.CreatedAt = now
		} else if existing, ok := s.state.Announcements[a.ID]; ok {
			if a.CreatedAt == 0 {
				a.CreatedAt = existing.CreatedAt
			}
			if a.CreatedByUID == 0 {
				a.CreatedByUID = existing.CreatedByUID
			}
		}
		a.UpdatedAt = now
		if a.Level == "" {
			a.Level = "info"
		}
		if a.RenderMode == "" {
			a.RenderMode = "plain"
		}
		if a.ForceRead && a.ForceReadSeconds <= 0 {
			a.ForceReadSeconds = 10
		}
		s.state.Announcements[a.ID] = a
		return nil
	})
	if err != nil {
		return Announcement{}, err
	}
	return a, nil
}

func (s *Store) ListAnnouncements(includeHidden bool) []Announcement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().Unix()
	out := make([]Announcement, 0)
	for _, a := range s.state.Announcements {
		if announcementVisibleForList(a, includeHidden, now) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].ID > out[j].ID
	})
	return out
}

func (s *Store) CountAnnouncements(includeHidden bool) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().Unix()
	count := 0
	for _, a := range s.state.Announcements {
		if announcementVisibleForList(a, includeHidden, now) {
			count++
		}
	}
	return count
}

func announcementVisibleForList(a Announcement, includeHidden bool, now int64) bool {
	return includeHidden || (a.Visible && (a.ExpiredAt <= 0 || a.ExpiredAt >= now))
}

func (s *Store) DeleteAnnouncement(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.Announcements[id]; !ok {
			return ErrNotFound
		}
		delete(s.state.Announcements, id)
		return nil
	})
}

func (s *Store) UnseenForceReadAnnouncements(uid int64) []Announcement {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().Unix()
	seen := map[int64]bool{}
	if u, ok := s.state.Users[uid]; ok {
		for _, id := range u.SeenAnnouncementIDs {
			seen[id] = true
		}
	}
	out := make([]Announcement, 0)
	for _, a := range s.state.Announcements {
		if !a.Visible || (a.ExpiredAt > 0 && a.ExpiredAt < now) {
			continue
		}
		if !a.ForceRead {
			continue
		}
		if seen[a.ID] {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) MarkAnnouncementsSeen(uid int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		existingSet := map[int64]bool{}
		for _, id := range u.SeenAnnouncementIDs {
			existingSet[id] = true
		}
		for _, id := range ids {
			existingSet[id] = true
		}
		seen := make([]int64, 0, len(existingSet))
		for id := range existingSet {
			seen = append(seen, id)
		}
		sort.Slice(seen, func(i, j int) bool { return seen[i] < seen[j] })
		u.SeenAnnouncementIDs = seen
		s.state.Users[uid] = u
		return nil
	})
}

func (s *Store) UpsertDeveloperJSPreset(p DeveloperJSPreset) (DeveloperJSPreset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		now := time.Now().Unix()
		if p.ID == 0 {
			p.ID = s.state.NextDeveloperJSPresetID
			s.state.NextDeveloperJSPresetID++
			p.CreatedAt = now
		} else if existing, ok := s.state.DeveloperJSPresets[p.ID]; ok {
			if p.CreatedAt == 0 {
				p.CreatedAt = existing.CreatedAt
			}
			if p.CreatorUID == 0 {
				p.CreatorUID = existing.CreatorUID
			}
		} else {
			return ErrNotFound
		}
		p.UpdatedAt = now
		s.state.DeveloperJSPresets[p.ID] = p
		return nil
	})
	if err != nil {
		return DeveloperJSPreset{}, err
	}
	return p, nil
}

func (s *Store) DeveloperJSPreset(id int64) (DeveloperJSPreset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	preset, ok := s.state.DeveloperJSPresets[id]
	return preset, ok
}

func (s *Store) ListDeveloperJSPresets() []DeveloperJSPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DeveloperJSPreset, 0, len(s.state.DeveloperJSPresets))
	for _, preset := range s.state.DeveloperJSPresets {
		out = append(out, preset)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out
}

func (s *Store) CountDeveloperJSPresets() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.DeveloperJSPresets)
}

func (s *Store) DeleteDeveloperJSPreset(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.DeveloperJSPresets[id]; !ok {
			return ErrNotFound
		}
		delete(s.state.DeveloperJSPresets, id)
		return nil
	})
}

// BangumiRegcodeClaimByID 返回某 Bangumi 账号（数值 id 的字符串形式）已领取过的注册码
// 领取记录。第二个返回值指示是否存在。只读、无副作用。
func (s *Store) BangumiRegcodeClaimByID(bangumiID string) (BangumiRegcodeClaim, bool) {
	bangumiID = strings.TrimSpace(bangumiID)
	if bangumiID == "" {
		return BangumiRegcodeClaim{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	claim, ok := s.state.BangumiRegcodeClaims[bangumiID]
	return claim, ok
}

// ClaimBangumiRegcode 原子化地为一个 Bangumi 账号登记注册码领取记录，是「一个 Bangumi
// 账号永远只能领一次」这一全局不变量的落点。语义为「首次写入生效」：
//   - 若该 Bangumi id 尚无记录，写入 claim（补齐 BangumiID / ClaimedAt）并返回
//     (写入后的记录, claimed=true)；
//   - 若已存在记录（可能是本进程稍早、也可能是他进程/并发抢先写入的），不覆盖，
//     返回 (既有记录, claimed=false)。
//
// 检查与写入在同一个 mutateAndSaveLocked 闭包内完成，PG 版本守卫撞冲突时整个闭包
// 会基于最新 state 重放，因此并发下也不会两个请求都判定为「首次」而各自发一张码。
// 调用方约定：claimed=false 时说明该账号此前已领码，应复用返回记录里的 Code、
// 并回收自己刚生成的候选码，绝不再下发新码。
func (s *Store) ClaimBangumiRegcode(claim BangumiRegcodeClaim) (BangumiRegcodeClaim, bool, error) {
	claim.BangumiID = strings.TrimSpace(claim.BangumiID)
	claim.Code = strings.TrimSpace(claim.Code)
	if claim.BangumiID == "" || claim.Code == "" {
		return BangumiRegcodeClaim{}, false, ErrInvalid
	}
	var (
		stored  BangumiRegcodeClaim
		claimed bool
	)
	err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.mutateAndSaveLocked(func() error {
			// 闭包可能在版本冲突时被重放，务必每次都重置局部结果，避免沿用上一轮的判定。
			claimed = false
			if existing, ok := s.state.BangumiRegcodeClaims[claim.BangumiID]; ok {
				stored = existing
				return nil
			}
			record := claim
			if record.ClaimedAt == 0 {
				record.ClaimedAt = time.Now().Unix()
			}
			s.state.BangumiRegcodeClaims[record.BangumiID] = record
			stored = record
			claimed = true
			return nil
		})
	}()
	if err != nil {
		return BangumiRegcodeClaim{}, false, err
	}
	return stored, claimed, nil
}

func (s *Store) DeveloperModeEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.DeveloperModeEnabled
}

func (s *Store) SetDeveloperModeEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.DeveloperModeEnabled = enabled
		return nil
	})
}

func (s *Store) UpsertInviteCode(code InviteCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		code.Code = strings.TrimSpace(code.Code)
		if code.Code == "" {
			return ErrInvalid
		}
		if _, exists := s.state.InviteCodes[code.Code]; exists {
			return ErrConflict
		}
		if code.InviterUID == 0 {
			code.InviterUID = code.UID
		}
		if code.UID == 0 {
			code.UID = code.InviterUID
		}
		if code.UseCountLimit == 0 {
			code.UseCountLimit = 1
		}
		if code.CreatedAt == 0 {
			code.CreatedAt = time.Now().Unix()
		}
		if !code.Used && code.UseCount < code.UseCountLimit {
			code.Active = true
		}
		s.state.InviteCodes[code.Code] = code
		return nil
	})
}

func (s *Store) InviteCode(code string) (InviteCode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.state.InviteCodes[code]
	return c, ok
}

func (s *Store) ListAllInviteCodes() []InviteCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]InviteCode, 0, len(s.state.InviteCodes))
	for _, c := range s.state.InviteCodes {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *Store) ListInviteCodes(uid int64) []InviteCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]InviteCode, 0)
	for _, c := range s.state.InviteCodes {
		if c.UID == uid {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *Store) CountInviteCodes() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.InviteCodes)
}

func (s *Store) DeleteInviteCode(uid int64, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		code = strings.TrimSpace(code)
		c, ok := s.state.InviteCodes[code]
		if !ok || c.UID != uid {
			return ErrNotFound
		}
		delete(s.state.InviteCodes, code)
		return nil
	})
}

func (s *Store) ConsumeInviteCode(code string, childUID int64) (InviteCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var consumed InviteCode
	err := s.mutateAndSaveLocked(func() error {
		c, ok := s.state.InviteCodes[code]
		if !ok || !c.Active {
			return ErrNotFound
		}
		if c.UseCountLimit != -1 && c.UseCount >= c.UseCountLimit {
			return ErrConflict
		}
		now := time.Now().Unix()
		if c.ExpiredAt > 0 && c.ExpiredAt <= now {
			return ErrExpired
		}
		if c.InviterUID != 0 && c.InviterUID == childUID {
			return ErrConflict
		}
		// 「一个 child 至多一个上级」必须在消费的同一把写锁内复检。否则 handler
		// 层的 ParentOf() 预检与此处消费之间存在 TOCTOU：同一 childUID 用两个不同
		// 邀请码并发请求会双双通过预检，第二次消费覆盖关系并烧掉两个邀请人的码。
		// 此处一旦发现已有上级即 ErrConflict，让先到者赢、后到者整体回滚。
		if _, exists := s.parentOfLocked(childUID); exists {
			return ErrConflict
		}
		c.UseCount++
		c.Used = true
		c.UsedByUID = childUID
		c.UsedAt = now
		if c.UseCountLimit != -1 && c.UseCount >= c.UseCountLimit {
			c.Active = false
		}
		s.state.InviteCodes[code] = c
		s.state.InviteRelations[childUID] = InviteRelation{ParentUID: c.InviterUID, ChildUID: childUID, Code: code, CreatedAt: now}
		consumed = c
		return nil
	})
	if err != nil {
		return InviteCode{}, err
	}
	return consumed, nil
}

func (s *Store) ConsumeInviteCodeAndUpdateUser(code string, childUID int64, maxDepth int, maxRootUsers int, fn func(*User, InviteCode) error) (User, InviteCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	var consumed InviteCode
	err := s.mutateAndSaveLocked(func() error {
		u, okUser := s.state.Users[childUID]
		if !okUser {
			return ErrNotFound
		}
		c, ok := s.state.InviteCodes[code]
		if !ok || !c.Active {
			return ErrNotFound
		}
		if c.UseCountLimit != -1 && c.UseCount >= c.UseCountLimit {
			return ErrConflict
		}
		now := time.Now().Unix()
		if c.ExpiredAt > 0 && c.ExpiredAt <= now {
			return ErrExpired
		}
		if c.InviterUID != 0 && c.InviterUID == childUID {
			return ErrConflict
		}
		if _, exists := s.parentOfLocked(childUID); exists {
			return ErrConflict
		}
		// 锁内重检邀请树深度与根用户上限，防止并发绕过
		if maxDepth > 0 && c.InviterUID != 0 {
			depth := s.inviteDepthLocked(c.InviterUID, maxDepth)
			if depth >= maxDepth {
				return ErrConflict
			}
		}
		if maxRootUsers > 0 && c.InviterUID != 0 {
			root := s.inviteRootLocked(c.InviterUID)
			if desc := s.inviteDescendantCountLocked(root); desc >= maxRootUsers {
				return ErrConflict
			}
		}
		c.UseCount++
		c.Used = true
		c.UsedByUID = childUID
		c.UsedAt = now
		if c.UseCountLimit != -1 && c.UseCount >= c.UseCountLimit {
			c.Active = false
		}
		s.state.InviteCodes[code] = c
		s.state.InviteRelations[childUID] = InviteRelation{ParentUID: c.InviterUID, ChildUID: childUID, Code: code, CreatedAt: now}
		old := u
		if fn != nil {
			if err := fn(&u, c); err != nil {
				return err
			}
		}
		if err := s.userIdentityConflictLocked(old, u, childUID); err != nil {
			return ErrConflict
		}
		s.state.Users[childUID] = u
		s.maintainUserIndexes(old, u, childUID)
		updated = u
		consumed = c
		return nil
	})
	if err != nil {
		return User{}, InviteCode{}, err
	}
	return updated, consumed, nil
}

func (s *Store) InviteRelations() []InviteRelation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]InviteRelation, 0, len(s.state.InviteRelations))
	for _, rel := range s.state.InviteRelations {
		out = append(out, rel)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ParentUID < out[j].ParentUID || (out[i].ParentUID == out[j].ParentUID && out[i].ChildUID < out[j].ChildUID)
	})
	return out
}

// InviteForestSnapshot returns only the users that can appear in the admin
// invite forest: relation endpoints, plus invite-code owners when prospective
// roots are requested. Keeping the UID selection and user copy under one read
// lock avoids materializing the full user and invite-code maps for every tree
// request.
func (s *Store) InviteForestSnapshot(includeCodeOwners bool) ([]InviteRelation, map[int64]User) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	relations := make([]InviteRelation, 0, len(s.state.InviteRelations))
	userIDs := make(map[int64]struct{}, len(s.state.InviteRelations)*2)
	for _, rel := range s.state.InviteRelations {
		relations = append(relations, rel)
		userIDs[rel.ParentUID] = struct{}{}
		userIDs[rel.ChildUID] = struct{}{}
	}
	if includeCodeOwners {
		for _, code := range s.state.InviteCodes {
			userIDs[code.InviterUID] = struct{}{}
		}
	}
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].ParentUID < relations[j].ParentUID ||
			(relations[i].ParentUID == relations[j].ParentUID && relations[i].ChildUID < relations[j].ChildUID)
	})
	users := make(map[int64]User, len(userIDs))
	for uid := range userIDs {
		if user, ok := s.state.Users[uid]; ok {
			users[uid] = user
		}
	}
	return relations, users
}

func (s *Store) ParentOf(uid int64) (InviteRelation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.parentOfLocked(uid)
}

func (s *Store) ChildrenOf(uid int64) []InviteRelation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]InviteRelation, 0)
	for _, rel := range s.state.InviteRelations {
		if rel.ParentUID == uid {
			out = append(out, rel)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChildUID < out[j].ChildUID })
	return out
}

// locked helpers — caller must hold s.mu (Lock or RLock).

func (s *Store) inviteDepthLocked(uid int64, maxDepth int) int {
	depth := 0
	current := uid
	for depth < maxDepth {
		rel, ok := s.parentOfLocked(current)
		if !ok {
			break
		}
		current = rel.ParentUID
		depth++
	}
	return depth
}

func (s *Store) inviteRootLocked(uid int64) int64 {
	current := uid
	for {
		rel, ok := s.parentOfLocked(current)
		if !ok {
			break
		}
		current = rel.ParentUID
	}
	return current
}

func (s *Store) inviteDescendantCountLocked(rootUID int64) int {
	count := 0
	for _, rel := range s.state.InviteRelations {
		if rel.ParentUID == rootUID || s.isDescendantLocked(rel.ParentUID, rootUID) {
			count++
		}
	}
	return count
}

func (s *Store) isDescendantLocked(uid, ancestor int64) bool {
	current := uid
	for {
		rel, ok := s.parentOfLocked(current)
		if !ok {
			return false
		}
		if rel.ParentUID == ancestor {
			return true
		}
		current = rel.ParentUID
	}
}

func (s *Store) DetachInvite(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.clearInviteUsageForUIDLocked(uid)
		return nil
	})
}

func (s *Store) parentOfLocked(uid int64) (InviteRelation, bool) {
	if uid == 0 {
		return InviteRelation{}, false
	}
	if rel, ok := s.state.InviteRelations[uid]; ok {
		return rel, true
	}
	for _, rel := range s.state.InviteRelations {
		if rel.ChildUID == uid {
			return rel, true
		}
	}
	return InviteRelation{}, false
}

func (s *Store) RegCode(code string) (RegCode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.state.RegCodes[code]
	return r, ok
}

func (s *Store) UpsertRegCode(code RegCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		_, exists := s.state.RegCodes[code.Code]
		if code.CreatedAt == 0 {
			code.CreatedAt = time.Now().Unix()
		}
		if code.CreatedTime == 0 {
			code.CreatedTime = code.CreatedAt
		}
		if code.ValidityTime == 0 {
			code.ValidityTime = -1
		}
		if code.UseCountLimit == 0 {
			code.UseCountLimit = 1
		}
		if !exists && !code.Active && code.UseCount == 0 {
			code.Active = true
		}
		s.state.RegCodes[code.Code] = code
		return nil
	})
}

// UpsertRegCodes 在一次状态写入中批量创建注册码。批量生成注册码时用它替代
// 逐条 UpsertRegCode，避免每条都触发一次全量状态落盘（saveLocked 每次序列化整个
// state，逐条写入时磁盘开销随数量线性放大）。批量生成必须是 create-only：
// 如果持久化 state 在 handler 预检之后已经出现同名码，整批拒绝，避免覆盖旧码。
// 单条字段默认值与 UpsertRegCode 保持一致。
func (s *Store) UpsertRegCodes(codes []RegCode) error {
	if len(codes) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		now := time.Now().Unix()
		seen := map[string]bool{}
		normalized := make([]RegCode, 0, len(codes))
		for _, code := range codes {
			code.Code = strings.TrimSpace(code.Code)
			if code.Code == "" || seen[code.Code] {
				return ErrConflict
			}
			seen[code.Code] = true
			if _, exists := s.state.RegCodes[code.Code]; exists {
				return ErrConflict
			}
			if code.CreatedAt == 0 {
				code.CreatedAt = now
			}
			if code.CreatedTime == 0 {
				code.CreatedTime = code.CreatedAt
			}
			if code.ValidityTime == 0 {
				code.ValidityTime = -1
			}
			if code.UseCountLimit == 0 {
				code.UseCountLimit = 1
			}
			if !code.Active && code.UseCount == 0 {
				code.Active = true
			}
			normalized = append(normalized, code)
		}
		for _, code := range normalized {
			s.state.RegCodes[code.Code] = code
		}
		return nil
	})
}

func (s *Store) ConsumeRegCode(code string, uid, telegramID int64) (RegCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var consumed RegCode
	err := s.mutateAndSaveLocked(func() error {
		now := time.Now().Unix()
		r, err := s.consumableRegCodeLocked(code, uid, telegramID, now)
		if err != nil {
			return err
		}
		consumed = s.consumeRegCodeLocked(r, uid, telegramID)
		return nil
	})
	if err != nil {
		return RegCode{}, err
	}
	return consumed, nil
}

func (s *Store) ConsumeRegCodeAndUpdateUser(code string, uid, telegramID int64, fn func(*User, RegCode) error) (User, RegCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	var consumed RegCode
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		now := time.Now().Unix()
		if telegramID == 0 {
			telegramID = u.TelegramID
		}
		r, err := s.consumableRegCodeLocked(code, uid, telegramID, now)
		if err != nil {
			return err
		}
		if r.Type == 2 && (strings.TrimSpace(u.EmbyID) == "" || u.PendingEmby) {
			return ErrEmbyRequired
		}
		consumed = s.consumeRegCodeLocked(r, uid, telegramID)
		old := u
		if fn != nil {
			if err := fn(&u, consumed); err != nil {
				return err
			}
		}
		if err := s.userIdentityConflictLocked(old, u, uid); err != nil {
			return ErrConflict
		}
		s.state.Users[uid] = u
		s.maintainUserIndexes(old, u, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, RegCode{}, err
	}
	return updated, consumed, nil
}

// RegCodeExpired 判定注册码是否已过有效期。纯函数（不触碰状态），锁内锁外均可调用。
// 有效期基准 elapsed = now - CreatedAt - PausedSeconds；若正处暂停态（PauseStart>0），
// 则把 elapsed 冻结在暂停时刻，使暂停期间不消耗有效期。ValidityTime<=0 视为永久有效。
//
// 三处过期判定必须共用本函数：store 消费校验（consumableRegCodeLocked）、
// API 展示态（regcodeStatus）、系统容量核算（remainingRegCodeUserSlots / remainingRegCodeEmbySlots）。
// 容量核算旧实现漏算 PausedSeconds/PauseStart，会把「暂停但仍有效」的码误判为过期 →
// 名额少算 → 允许越过 emby_user_limit 超发；统一到本函数即可消除该口径分叉。
func RegCodeExpired(r RegCode, now int64) bool {
	if r.ValidityTime <= 0 {
		return false
	}
	elapsed := now - r.CreatedAt - r.PausedSeconds
	if r.PauseStart > 0 {
		elapsed = r.PauseStart - r.CreatedAt - r.PausedSeconds
	}
	return elapsed >= r.ValidityTime*3600
}

// consumableRegCodeLocked 校验某张注册码此刻是否可被 (uid, telegramID) 这个身份消费。
// uid / telegramID 传 0 表示「新用户创建路径」——此时账号尚未分配 UID，每次都是新身份，
// 天然不存在同一身份重复消费的双花问题（同 TG 二次建号由 telegramIDTakenLocked 拦），
// 故跳过 per-identity 守卫、保持创建行为不变。既有用户消费路径必须传真实身份。
func (s *Store) consumableRegCodeLocked(code string, uid, telegramID, now int64) (RegCode, error) {
	r, ok := s.state.RegCodes[code]
	if !ok || !r.Active {
		return RegCode{}, ErrNotFound
	}
	if r.UseCountLimit != -1 && r.UseCount >= r.UseCountLimit {
		return RegCode{}, ErrConflict
	}
	if RegCodeExpired(r, now) {
		return RegCode{}, ErrExpired
	}
	// per-identity 单次守卫（语义：N 次 = N 个人各一次）。UseCount 是可服务人数上限，
	// 不是单人可叠加次数；缺此守卫时用户可对同一张多次数/无限次码反复消费叠加天数、
	// 抢占他人名额。与邀请码 parentOfLocked「不能重复加入邀请树」同源。
	if uid != 0 {
		for _, used := range r.UsedByUIDs {
			if used == uid {
				return RegCode{}, ErrRegCodeAlreadyUsedByUser
			}
		}
	}
	if telegramID != 0 {
		for _, used := range r.UsedByTelegramIDs {
			if used == telegramID {
				return RegCode{}, ErrRegCodeAlreadyUsedByUser
			}
		}
	}
	return r, nil
}

func (s *Store) consumeRegCodeLocked(r RegCode, uid, telegramID int64) RegCode {
	r.UseCount++
	if uid != 0 {
		r.UsedBy = uid
		r.UsedByUIDs = appendUniqueInt64(r.UsedByUIDs, uid)
	}
	if telegramID != 0 {
		r.UsedByTelegramIDs = appendUniqueInt64(r.UsedByTelegramIDs, telegramID)
	}
	if r.UseCountLimit != -1 && r.UseCount >= r.UseCountLimit {
		r.Active = false
	}
	s.state.RegCodes[r.Code] = r
	return r
}

func (s *Store) ListRegCodes() []RegCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RegCode, 0, len(s.state.RegCodes))
	for _, c := range s.state.RegCodes {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *Store) CountRegCodes() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.RegCodes)
}

// RegCodeCounts returns total and currently active registration-code counts
// without allocating and sorting a full list.
func (s *Store) RegCodeCounts() (total int, active int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total = len(s.state.RegCodes)
	for _, code := range s.state.RegCodes {
		if code.Active {
			active++
		}
	}
	return total, active
}

func (s *Store) DeleteRegCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.RegCodes[code]; !ok {
			return ErrNotFound
		}
		delete(s.state.RegCodes, code)
		s.clearRegistrationCodeReferencesLocked(code)
		return nil
	})
}

func (s *Store) DeleteRegCodes(codes []string) (deleted []string, missing []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deletedSet := map[string]bool{}
	mutErr := s.mutateAndSaveLocked(func() error {
		seen := map[string]bool{}
		for _, code := range codes {
			code = strings.TrimSpace(code)
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			if _, ok := s.state.RegCodes[code]; !ok {
				missing = append(missing, code)
				continue
			}
			delete(s.state.RegCodes, code)
			deleted = append(deleted, code)
			deletedSet[code] = true
		}
		for code := range deletedSet {
			s.clearRegistrationCodeReferencesLocked(code)
		}
		return nil
	})
	if mutErr != nil {
		// 失败时整批回滚（mutateAndSaveLocked 已经把内存恢复到快照），
		// 不再向调用方暴露半量结果。
		return nil, nil, mutErr
	}
	return deleted, missing, nil
}

func (s *Store) clearRegistrationCodeReferencesLocked(code string) {
	for uid, user := range s.state.Users {
		if user.RegistrationCode == code {
			user.RegistrationCode = ""
			s.state.Users[uid] = user
		}
	}
}

func (s *Store) CreateRebindRequest(req RebindRequest) (RebindRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var existingHit RebindRequest
	var hit bool
	err := s.mutateAndSaveLocked(func() error {
		for _, existing := range s.state.RebindRequests {
			if existing.UID == req.UID && existing.Status == "pending" {
				existingHit = existing
				hit = true
				return ErrConflict
			}
		}
		if req.ID == 0 {
			req.ID = s.state.NextRebindRequestID
			s.state.NextRebindRequestID++
		}
		if req.Status == "" {
			req.Status = "pending"
		}
		if req.CreatedAt == 0 {
			req.CreatedAt = time.Now().Unix()
		}
		s.state.RebindRequests[req.ID] = req
		return nil
	})
	if hit {
		return existingHit, ErrConflict
	}
	if err != nil {
		return RebindRequest{}, err
	}
	return req, nil
}

func (s *Store) ListRebindRequests(status string) []RebindRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RebindRequest, 0, len(s.state.RebindRequests))
	for _, req := range s.state.RebindRequests {
		if status == "" || status == "all" || req.Status == status {
			out = append(out, req)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) ReviewRebindRequest(id, reviewerUID int64, status, note string) (RebindRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated RebindRequest
	err := s.mutateAndSaveLocked(func() error {
		req, ok := s.state.RebindRequests[id]
		if !ok {
			return ErrNotFound
		}
		req.Status = status
		req.AdminNote = note
		req.ReviewerUID = reviewerUID
		req.ReviewedAt = time.Now().Unix()
		s.state.RebindRequests[id] = req
		updated = req
		return nil
	})
	if err != nil {
		return RebindRequest{}, err
	}
	return updated, nil
}

// UserLatestRebindRequest returns the most recent rebind request for a user,
// regardless of status. Returns (request, true) if found, or (zero, false) if
// the user has never submitted a rebind request.
func (s *Store) UserLatestRebindRequest(uid int64) (RebindRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var (
		best  RebindRequest
		found bool
	)
	for _, req := range s.state.RebindRequests {
		if req.UID != uid {
			continue
		}
		if !found || req.CreatedAt > best.CreatedAt || (req.CreatedAt == best.CreatedAt && req.ID > best.ID) {
			best = req
			found = true
		}
	}
	return best, found
}

// ConsumeRebindRequest marks an approved rebind request as "used" so it cannot
// be reused to bypass force-bind policy again. Called after the user successfully
// unbinds Telegram using the approved permission.
func (s *Store) ConsumeRebindRequest(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		req, ok := s.state.RebindRequests[id]
		if !ok {
			return ErrNotFound
		}
		if req.Status != "approved" {
			return nil // only consume approved requests
		}
		req.Status = "used"
		s.state.RebindRequests[id] = req
		return nil
	})
}

// RevokeApprovedRebindRequests 把所有 status=="approved"（尚未被解绑消费的换绑
// 许可）批量置为 "revoked"，让持有者立即失去解绑权限，但保留再次申请的能力
// （CreateRebindRequest 只拦截 pending；revoked 不影响）。用于策略收紧后一键
// 清理"历史遗留的换绑许可"。不触碰 pending（待审申请）/ used（已用）/
// RebindingInProgress（正在进行中的换绑不应被打断）。返回被撤销的数量，并保留
// 更新审核元数据（reviewer / note / reviewedAt）以便审计"谁在何时批量撤销"。
func (s *Store) RevokeApprovedRebindRequests(reviewerUID int64, note string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	err := s.mutateAndSaveLocked(func() error {
		count = 0
		now := time.Now().Unix()
		for id, req := range s.state.RebindRequests {
			if req.Status != "approved" {
				continue
			}
			req.Status = "revoked"
			req.ReviewerUID = reviewerUID
			if note != "" {
				req.AdminNote = note
			}
			req.ReviewedAt = now
			s.state.RebindRequests[id] = req
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountUsersBy(predicate func(User) bool) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, u := range s.state.Users {
		if predicate(u) {
			count++
		}
	}
	return count
}

func (s *Store) UserSummaryCounts() UserSummaryCounts {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := UserSummaryCounts{Total: len(s.state.Users)}
	for _, u := range s.state.Users {
		if u.Active {
			counts.Active++
		}
		if u.Role == RoleAdmin {
			counts.Admins++
		}
		if u.TelegramID != 0 {
			counts.TelegramBound++
		}
		if strings.TrimSpace(u.EmbyID) != "" {
			counts.EmbyBound++
		}
		if u.PendingEmby {
			counts.PendingEmby++
		}
		if strings.TrimSpace(u.Email) != "" {
			counts.EmailBound++
		}
		if u.EmailVerified {
			counts.EmailVerified++
		}
	}
	return counts
}

func appendUniqueInt64(values []int64, value int64) []int64 {
	if value == 0 {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// AddViolationLog records a code violation attempt.
func (s *Store) AddViolationLog(log ViolationLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		// 用单调递增计数器而非 len()+1：删除条目后再插入会复用旧 ID,
		// 既会破坏外部引用（admin UI / 操作日志按 ID 关联），也会导致审计追溯
		// 错乱。NextViolationLogID 与其他业务域计数器同款 pattern。
		log.ID = s.state.NextViolationLogID
		s.state.NextViolationLogID++
		s.state.ViolationLogs = append(s.state.ViolationLogs, log)
		return nil
	})
}

// ListViolationLogs returns all violation logs, newest first.
func (s *Store) ListViolationLogs() []ViolationLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ViolationLog, len(s.state.ViolationLogs))
	copy(out, s.state.ViolationLogs)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// DeleteViolationLog removes a single violation log entry by ID.
func (s *Store) DeleteViolationLog(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		for i, log := range s.state.ViolationLogs {
			if log.ID == id {
				s.state.ViolationLogs = append(s.state.ViolationLogs[:i], s.state.ViolationLogs[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

// ClearViolationLogs removes all violation logs.
func (s *Store) ClearViolationLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.ViolationLogs = nil
		return nil
	})
}

const telegramRuntimeDBTimeout = 5 * time.Second

// ensureTelegramRuntime creates the singleton cursor row and seeds historical
// deployments from the legacy JSONB field exactly once. Existing runtime data
// wins, including an intentional reset to zero.
func (s *Store) ensureTelegramRuntime(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO twilight_telegram_runtime (id, update_offset, updated_at)
VALUES (1, $1, now())
ON CONFLICT (id) DO NOTHING`, s.state.TelegramBotOffset)
	return err
}

// clearLegacyTelegramBotOffset removes the migration seed after the dedicated
// row exists, preventing future JSON snapshots from carrying a stale cursor.
func (s *Store) clearLegacyTelegramBotOffset() error {
	if s.state.TelegramBotOffset == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.TelegramBotOffset = 0
		return nil
	})
}

// TelegramBotOffset reads the durable getUpdates cursor without refreshing or
// decoding the multi-megabyte twilight_state document.
func (s *Store) TelegramBotOffset() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), telegramRuntimeDBTimeout)
	defer cancel()
	var offset int64
	err := s.db.QueryRowContext(ctx, `SELECT update_offset FROM twilight_telegram_runtime WHERE id = 1`).Scan(&offset)
	return offset, err
}

// SetTelegramBotOffset advances the cursor monotonically with one narrow row
// update. It intentionally avoids mutateAndSaveLocked and all state JSON work.
func (s *Store) SetTelegramBotOffset(offset int64) error {
	if offset <= 0 {
		return nil
	}
	return s.advanceTelegramBotOffset(offset)
}

func (s *Store) advanceTelegramBotOffset(offset int64) error {
	if offset <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), telegramRuntimeDBTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO twilight_telegram_runtime (id, update_offset, updated_at)
VALUES (1, $1, now())
ON CONFLICT (id) DO UPDATE SET
	update_offset = GREATEST(twilight_telegram_runtime.update_offset, EXCLUDED.update_offset),
	updated_at = CASE
		WHEN EXCLUDED.update_offset > twilight_telegram_runtime.update_offset THEN now()
		ELSE twilight_telegram_runtime.updated_at
	END`, offset)
	return err
}

// ResetTelegramBotOffset clears the cursor when getMe proves that configuration
// now points to a different Bot identity.
func (s *Store) ResetTelegramBotOffset() error {
	ctx, cancel := context.WithTimeout(context.Background(), telegramRuntimeDBTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO twilight_telegram_runtime (id, update_offset, updated_at)
VALUES (1, 0, now())
ON CONFLICT (id) DO UPDATE SET update_offset = 0, updated_at = now()`)
	return err
}

var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid")
	ErrConflict    = errors.New("conflict")
	ErrExpired     = errors.New("expired")
	ErrLastAdmin   = errors.New("last admin")
	ErrGrantLocked = errors.New("emby grant locked")
	// ErrRegCodeAlreadyUsedByUser 表示同一身份（UID 或 TelegramID）重复消费同一张
	// 多次数/无限次注册码。语义为「N 次 = N 个人各一次」：UseCount 是可服务人数上限，
	// 不是单人可叠加的次数。缺此守卫时，用户可对同一张 use_count_limit>1（或 -1）的码
	// 反复调用消费接口，每次递增 UseCount 并把天数叠加到自己的到期时间、抢占本应留给
	// 他人的名额。与邀请码 parentOfLocked「不能重复加入邀请树」同源。
	ErrRegCodeAlreadyUsedByUser      = errors.New("reg code already used by this identity")
	ErrTicketClosed                  = errors.New("ticket closed")
	ErrTicketNotClosed               = errors.New("ticket not closed")
	ErrTicketUserOpenLimit           = errors.New("ticket user open limit reached")
	ErrTicketGlobalOpenLimit         = errors.New("ticket global open limit reached")
	ErrMediaRequestUserActiveLimit   = errors.New("media request user active limit reached")
	ErrMediaRequestGlobalActiveLimit = errors.New("media request global active limit reached")
	ErrInsufficientPoints            = errors.New("insufficient points")
	ErrEmbyRequired                  = errors.New("emby binding required")
)

// errStateVersionConflict 是 Postgres 后端乐观并发控制的内部哨兵：saveLocked 的
// 版本守卫 UPSERT 发现 twilight_state.version 已被其它进程递增时返回它。
// 仅在进程内流转——mutateAndSaveLocked 捕获后重新 refresh+mutate 重试，直裸写者
// 则 fail-closed 上抛（这些路径的调用方要么丢弃错误、要么可安全重试），
// 绝不再走「盲写整份 jsonb 覆盖他进程刚提交的写」的丢更新老路。
var errStateVersionConflict = errors.New("state version conflict")

var (
	errTelegramMembershipRebindProtected = errors.New("telegram membership rebind protected")
	errTelegramMembershipNoChange        = errors.New("telegram membership no change")
)

func randomKey(prefix string, id, now int64) string {
	return prefix + "_" + strconv36(id) + "_" + strconv36(now)
}

func strconv36(v int64) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return ""
	}
	var buf [13]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = alphabet[v%36]
		v /= 36
	}
	return string(buf[i:])
}
