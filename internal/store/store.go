package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	RoleAdmin        = 0
	RoleNormal       = 1
	RoleWhitelist    = 2
	RoleUnrecognized = -1
)

type Store struct {
	mu      sync.RWMutex
	path    string
	backend string
	db      *sql.DB
	state   State
	// JSON 后端进程级排他锁。
	// Postgres 后端为 nil；同进程内并发由 mu 已经串行化，
	// 这里防的是多个 Twilight 进程共用同一份 state.json 时的丢更新。
	lock *fileLock

	// telegramIDMap 是 telegram_id → UID 的二级索引，避免每次都全表扫描 Users。
	// 在 Open/OpenPostgres 时重建，后续通过 maintainTelegramIDIndex 增量维护。
	telegramIDMap map[int64]int64
	// embyIDMap 是 emby_id → UID 的二级索引，服务 Emby webhook / 会话 / 管理操作等高频反查。
	embyIDMap map[string]int64
}

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
	TelegramRoster          map[string]TelegramRosterEntry         `json:"telegram_roster"`
	ViolationLogs           []ViolationLog                         `json:"violation_logs"`
	AuditLogs               []AuditLog                             `json:"audit_logs"`
	BangumiSyncLogs         []BangumiSyncLog                       `json:"bangumi_sync_logs"`
	BangumiCollectionCache  map[string]BangumiCollectionCacheEntry `json:"bangumi_collection_cache,omitempty"`
	BangumiSubjectCache     map[string]BangumiSubjectCacheEntry    `json:"bangumi_subject_cache,omitempty"`
	Tickets                 map[int64]Ticket                       `json:"tickets"`
	TicketTypes             []string                               `json:"ticket_types,omitempty"`
	DeveloperJSPresets      map[int64]DeveloperJSPreset            `json:"developer_js_presets,omitempty"`
	DeveloperModeEnabled    bool                                   `json:"developer_mode_enabled,omitempty"`
	// TelegramBotOffset 持久化最近一次成功 ack 的 update_id+1。
	// 重启 / token 切换时直接从这个值恢复，避免对 24h backlog 重新分发。
	// 0 表示未设置 / 历史 state，按"从 0 开始"处理（getUpdates 会拿到队列里
	// 全部待 ack 消息，与历史行为一致；只是后续 ack 会立即收敛）。
	// 不放入 RuntimeMeta：为了让所有写入路径自然走 mutateAndSaveLocked，
	// 与 NextSchedulerRunID 这类整数计数器保持同构。
	TelegramBotOffset int64 `json:"telegram_bot_offset"`
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
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Visible      bool   `json:"visible"`
	Level        string `json:"level"`
	RenderMode   string `json:"render_mode,omitempty"`
	Pinned       bool   `json:"pinned"`
	CreatedByUID int64  `json:"created_by_uid,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	ExpiredAt    int64  `json:"expired_at,omitempty"`
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

func Open(path string) (*Store, error) {
	if path == "" {
		path = filepath.Join("db", "twilight_go_state.json")
	}
	// 先确保父目录存在，再尝试拿排他锁；锁文件是 path + ".lock"。
	// 第二个 Twilight 进程在这里会立刻拿到 ErrLockBusy，启动失败而不是
	// 静默与首个进程竞写 state.json。
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("ensure state dir: %w", err)
	}
	// 启动期可写校验：失败立即 fail-fast，避免运行时第一次 saveLocked
	// 才发现盘满 / 权限不对，把请求半途打断。
	if err := probeWritable(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("state dir not writable: %w", err)
	}
	lock, err := acquireStateLock(path)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return nil, fmt.Errorf("state file %q is locked by another Twilight process; multi-process JSON backend is not safe — use Postgres or stop the other process", path)
		}
		return nil, err
	}
	st := &Store{path: path, backend: BackendJSON, state: emptyState(), lock: lock}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, st.saveLocked()
		}
		_ = lock.Release()
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &st.state); err != nil {
			// 主文件坏掉时尝试 .bak 兜底 —— Open 比 refreshLocked 更激进，
			// 直接走 fallback 同时保留 lock，避免管理员被 "无法启动" 卡死。
			if bak, bakErr := os.ReadFile(path + ".bak"); bakErr == nil && len(bak) > 0 {
				if err := json.Unmarshal(bak, &st.state); err != nil {
					_ = lock.Release()
					return nil, err
				}
			} else {
				_ = lock.Release()
				return nil, err
			}
		}
	}
	st.state.ensure()
	st.rebuildTelegramIDIndex()
	st.rebuildEmbyIDIndex()
	return st, nil
}

// probeWritable 在目标目录里写一字节 sentinel 再删除，确认 dir 实际可写。
func probeWritable(dir string) error {
	tmp, err := os.CreateTemp(dir, ".twilight-write-probe-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, _ = tmp.Write([]byte{0})
	_ = tmp.Close()
	_ = os.Remove(name)
	return nil
}

func OpenPostgres(ctx context.Context, dsn string) (*Store, error) {
	db, _, err := openPreparedPostgres(ctx, dsn)
	if err != nil {
		return nil, err
	}

	st := &Store{backend: BackendPostgres, path: "postgres", db: db, state: emptyState()}
	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT state FROM twilight_state WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, st.saveLocked()
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &st.state); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	st.state.ensure()
	st.rebuildTelegramIDIndex()
	st.rebuildEmbyIDIndex()
	return st, nil
}

func CreatePostgresDatabase(ctx context.Context, dsn string) error {
	cfg, err := pgconn.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return err
	}
	target := strings.TrimSpace(cfg.Database)
	if target == "" {
		return fmt.Errorf("target database name is empty")
	}
	if strings.EqualFold(target, "postgres") || strings.EqualFold(target, "template1") {
		return fmt.Errorf("refusing to auto-create maintenance database %q", target)
	}
	maintenanceDSNs := maintenancePostgresDSNs(dsn)
	var lastErr error
	for _, maintenanceDSN := range maintenanceDSNs {
		maintenance := postgresTargetInfo(maintenanceDSN)
		zap.L().Info("attempting PostgreSQL database creation through maintenance database", zap.String("target_database", target), zap.String("maintenance_database", maintenance.Database), zap.String("user", maintenance.User), zap.String("host", maintenance.Host))
		db, err := sql.Open("pgx", maintenanceDSN)
		if err != nil {
			lastErr = err
			continue
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(0)
		_, err = db.ExecContext(ctx, `CREATE DATABASE `+quotePostgresIdentifier(target))
		closeErr := db.Close()
		if err == nil && closeErr == nil {
			return nil
		}
		if err == nil {
			err = closeErr
		}
		if isDuplicateDatabaseError(err) {
			return nil
		}
		targetInfo := maintenance
		targetInfo.Database = target
		lastErr = describePostgresConnectionError(targetInfo, err)
		zap.L().Warn("PostgreSQL automatic database creation attempt failed", zap.String("target_database", target), zap.String("maintenance_database", maintenance.Database), zap.String("user", maintenance.User), zap.String("host", maintenance.Host), zap.Error(lastErr))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no maintenance database connection strings could be built")
	}
	return lastErr
}

func maintenancePostgresDSNs(dsn string) []string {
	databases := []string{"postgres", "template1"}
	out := make([]string, 0, len(databases))
	for _, database := range databases {
		if rewritten, ok := rewritePostgresDatabaseInDSN(dsn, database); ok {
			out = append(out, rewritten)
		}
	}
	return out
}

func rewritePostgresDatabaseInDSN(dsn, database string) (string, bool) {
	dsn = strings.TrimSpace(dsn)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "", false
		}
		parsed.Path = "/" + database
		return parsed.String(), true
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil || cfg.Host == "" || cfg.User == "" {
		return "", false
	}
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))),
		Path:   "/" + database,
	}
	if cfg.Password == "" {
		u.User = url.User(cfg.User)
	} else {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	q := u.Query()
	if cfg.TLSConfig == nil {
		q.Set("sslmode", "disable")
	}
	for key, value := range cfg.RuntimeParams {
		if strings.TrimSpace(value) != "" {
			q.Set(key, value)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), true
}

func isUndefinedDatabaseError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "3D000" {
		return true
	}
	return strings.Contains(err.Error(), "SQLSTATE 3D000")
}

func isDuplicateDatabaseError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P04" {
		return true
	}
	return strings.Contains(err.Error(), "SQLSTATE 42P04")
}

type postgresInfo struct {
	Host     string
	User     string
	Database string
}

func postgresTargetInfo(dsn string) postgresInfo {
	cfg, err := pgconn.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return postgresInfo{}
	}
	return postgresInfo{Host: cfg.Host, User: cfg.User, Database: cfg.Database}
}

func describePostgresConnectionError(info postgresInfo, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "28P01":
			return fmt.Errorf("PostgreSQL authentication failed for user %q on host %q: password is incorrect or pg_hba.conf rejected the login (SQLSTATE 28P01): %w", info.User, info.Host, err)
		case "28000":
			return fmt.Errorf("PostgreSQL login rejected for user %q on host %q (SQLSTATE 28000): %w", info.User, info.Host, err)
		case "42501":
			return fmt.Errorf("PostgreSQL user %q does not have permission to create or modify database %q; grant CREATEDB or create the database manually (SQLSTATE 42501): %w", info.User, info.Database, err)
		case "3D000":
			return fmt.Errorf("PostgreSQL database %q does not exist (SQLSTATE 3D000): %w", info.Database, err)
		case "42P04":
			return fmt.Errorf("PostgreSQL database %q already exists (SQLSTATE 42P04): %w", info.Database, err)
		}
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "SQLSTATE 28P01"):
		return fmt.Errorf("PostgreSQL authentication failed for user %q on host %q: password is incorrect or pg_hba.conf rejected the login (SQLSTATE 28P01): %w", info.User, info.Host, err)
	case strings.Contains(text, "SQLSTATE 42501"):
		return fmt.Errorf("PostgreSQL user %q does not have permission to create or modify database %q; grant CREATEDB or create the database manually (SQLSTATE 42501): %w", info.User, info.Database, err)
	case strings.Contains(text, "SQLSTATE 3D000"):
		return fmt.Errorf("PostgreSQL database %q does not exist (SQLSTATE 3D000): %w", info.Database, err)
	default:
		return fmt.Errorf("PostgreSQL connection failed for user %q database %q host %q: %w", info.User, info.Database, info.Host, err)
	}
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func CheckPostgres(ctx context.Context, dsn string) error {
	db, _, err := openPreparedPostgres(ctx, dsn)
	if err != nil {
		return err
	}
	return db.Close()
}

func CheckPostgresTarget(ctx context.Context, dsn string) (PostgresTargetStatus, error) {
	db, status, err := openPreparedPostgres(ctx, dsn)
	if err != nil {
		return status, err
	}
	if closeErr := db.Close(); closeErr != nil {
		return status, closeErr
	}
	return status, nil
}

func openPreparedPostgres(ctx context.Context, dsn string) (*sql.DB, PostgresTargetStatus, error) {
	dsn = strings.TrimSpace(dsn)
	target := postgresTargetInfo(dsn)
	status := PostgresTargetStatus{
		Host:     target.Host,
		User:     target.User,
		Database: target.Database,
	}
	if dsn == "" {
		return nil, status, fmt.Errorf("postgres dsn is empty")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, status, err
	}
	configurePostgresDB(db, 8, 4)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		if !isUndefinedDatabaseError(err) {
			return nil, status, describePostgresConnectionError(target, err)
		}
		zap.L().Warn("PostgreSQL database does not exist; attempting automatic creation", zap.String("database", target.Database), zap.String("user", target.User), zap.String("host", target.Host))
		if createErr := CreatePostgresDatabase(ctx, dsn); createErr != nil {
			return nil, status, fmt.Errorf("PostgreSQL database %q does not exist and automatic creation failed: %w", target.Database, describePostgresConnectionError(target, createErr))
		}
		status.DatabaseCreated = true
		zap.L().Info("PostgreSQL database created", zap.String("database", target.Database), zap.String("user", target.User), zap.String("host", target.Host))
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			return nil, status, err
		}
		configurePostgresDB(db, 8, 4)
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, status, describePostgresConnectionError(target, err)
		}
	}
	status.Connected = true
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_state (
	id integer PRIMARY KEY,
	state jsonb NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_runtime_logs (
	id bigserial PRIMARY KEY,
	time bigint NOT NULL,
	level text NOT NULL,
	message text NOT NULL,
	attrs jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_runtime_logs_time_idx ON twilight_runtime_logs (time DESC)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_runtime_logs_id_desc_idx ON twilight_runtime_logs (id DESC)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_sessions (
	token text PRIMARY KEY,
	uid bigint NOT NULL,
	expires_at bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_sessions_uid_idx ON twilight_sessions (uid)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_sessions_expires_at_idx ON twilight_sessions (expires_at)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_playback_records (
	id bigserial PRIMARY KEY,
	uid bigint NOT NULL,
	item_id text NOT NULL,
	title text NOT NULL DEFAULT '',
	series_name text NOT NULL DEFAULT '',
	media_type text NOT NULL DEFAULT '',
	index_number int NOT NULL DEFAULT 0,
	duration bigint NOT NULL DEFAULT 0,
	played_at bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_playback_records_uid_idx ON twilight_playback_records (uid)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_playback_records_played_at_idx ON twilight_playback_records (played_at DESC)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS twilight_playback_records_uid_item_played_idx ON twilight_playback_records (uid, item_id, played_at)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS twilight_playback_records_item_id_idx ON twilight_playback_records (item_id)`); err != nil {
		_ = db.Close()
		return nil, status, describePostgresConnectionError(target, err)
	}
	status.SchemaReady = true
	return db, status, nil
}

func configurePostgresDB(db *sql.DB, maxOpen, maxIdle int) {
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(30 * time.Minute)
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	// JSON 后端释放进程级 flock；Postgres 后端 lock=nil 走 noop。
	if s.lock != nil {
		_ = s.lock.Release()
		s.lock = nil
	}
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB when using PostgreSQL backend, or nil otherwise.
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

func (s *Store) Backend() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.backend == "" {
		return BackendJSON
	}
	return s.backend
}

func (s *Store) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
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
	if s.TelegramRoster == nil {
		s.TelegramRoster = map[string]TelegramRosterEntry{}
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

	now := time.Now().Unix()
	for id, t := range s.Tickets {
		if t.AdminNote != "" && len(t.Replies) == 0 {
			t.Replies = []TicketReply{{
				UID:       0,
				Username:  "管理员",
				Role:      0,
				Content:   t.AdminNote,
				CreatedAt: t.UpdatedAt,
			}}
			t.AdminNote = ""
			s.Tickets[id] = t
			_ = now
		}
	}
	if s.TicketTypes == nil {
		s.TicketTypes = []string{"all"}
	}
}

func (s *State) compactHistory() {
	s.RuntimeLogs = compactTail(s.RuntimeLogs, defaultRuntimeLogLimit)
	s.LoginLogs = compactHead(s.LoginLogs, maxStoredLoginLogs)
	s.PlaybackRecords = compactHead(s.PlaybackRecords, maxStoredPlaybackRecords)
	s.PlaybackSessions = compactTail(s.PlaybackSessions, maxPlaybackSessions)
	s.EmbyActivityLogs = compactTail(s.EmbyActivityLogs, maxEmbyActivityLogs)
	s.BangumiSyncLogs = compactTail(s.BangumiSyncLogs, maxStoredBangumiSyncLogs)
	for uid, signin := range s.Signin {
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

// snapshotStateLocked 把当前 s.state 序列化再反序列化得到一份独立副本，
// 用于 mutateAndSaveLocked 的回滚。State 内大量 map/slice 是引用类型，
// 浅拷贝不能隔离后续修改；JSON 往返一次比 reflect.DeepCopy 简单且与
// 已有的 refreshLocked 走同一条路径（次数等量级，开销可接受）。
func (s *Store) snapshotStateLocked() (State, error) {
	s.state.ensure()
	data, err := json.Marshal(&s.state)
	if err != nil {
		return State{}, err
	}
	var clone State
	if err := json.Unmarshal(data, &clone); err != nil {
		return State{}, err
	}
	clone.ensure()
	return clone, nil
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
	if err := s.refreshLocked(); err != nil {
		return err
	}
	prev, err := s.snapshotStateLocked()
	if err != nil {
		return err
	}
	if err := mutate(); err != nil {
		// mutate 失败：本身就不打算落盘，状态可能被改了一半，回滚到快照。
		s.state = prev
		s.rebuildUserIndexes()
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.state = prev
		s.rebuildUserIndexes()
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	s.state.ensure()
	var (
		data []byte
		err  error
	)
	if s.db != nil {
		data, err = json.Marshal(s.state)
	} else {
		data, err = json.Marshal(s.state)
	}
	if err != nil {
		return err
	}
	if s.db != nil {
		// 整 jsonb 一次写入大对象（用户表 + 邀请关系 + 登录历史 + … 数 MB）；
		// 之前裸 context.Background() 一旦 PG 抖动会让 saveLocked 永久挂起，
		// graceful shutdown 与并发 handler 全部跟着卡死。这里用 30s
		// WithTimeout 兜底：到期 ExecContext 自行退出释放连接，调用方拿到
		// context.DeadlineExceeded 走回滚分支（mutateAndSaveLocked 把内存 state
		// 还原到 snapshot），磁盘和内存仍保持一致。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err = s.db.ExecContext(
			ctx,
			`INSERT INTO twilight_state (id, state, updated_at) VALUES (1, $1::jsonb, now())
			 ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`,
			string(data),
		)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	// 写前先把上一份 state.json 复制到 .bak —— refreshLocked 解析失败时可
	// 回退到 .bak（避免一次坏写盖掉所有用户数据）。复用 writeFileAtomicSync
	// 走 tmp + fsync(file) + rename + fsync(dir)：之前裸 os.WriteFile + Rename
	// 没有 fsync，掉电后 .bak 可能仍是 page cache 里半字节状态，最坏 state.json
	// 与 .bak 同时损坏 = 没有恢复路径。helper 失败仅写 stderr，**不能**走
	// zap.L() —— saveLocked 当前持有 s.mu，而全局 zap sink 注册了 runtime
	// log 路由会回调 AddRuntimeLog 再次申请 s.mu，构成自旋死锁（Windows CI
	// 上 dir.Sync 必然失败，必中此路径，而 Linux/macOS 通常成功才掩盖）。
	if existing, readErr := os.ReadFile(s.path); readErr == nil && len(existing) > 0 {
		if err := writeFileAtomicSync(s.path+".bak", existing, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "twilight: state .bak shadow copy failed path=%s err=%v\n", s.path, err)
		}
	}
	tmp := s.path + ".tmp"
	// tmp 写完后必须 fsync 数据 + 父目录，否则 os.Rename 仅在 VFS 层原子，
	// crash/掉电时数据可能仍在 page cache。顺序：
	// write → fsync(file) → close → rename → chmod → fsync(dir)。
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
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
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	// 强制 0o600：即使外部 umask / 拷贝把权限放宽到 group/other，重写后
	// 也立刻收敛回仅 owner 可读写，避免 state.json 被同机其它用户读取。
	_ = os.Chmod(s.path, 0o600)
	// 父目录 fsync 让 rename 自身的目录条目落盘。Windows 不支持以这种方式
	// 同步目录；文件内容已通过 f.Sync() 落盘，目录同步在该平台降级为 best effort。
	_ = syncParentDir(filepath.Dir(s.path), false)
	return nil
}

func (s *Store) refreshLocked() error {
	if s == nil {
		return nil
	}
	// Multiple Twilight processes can share the same state backend; refresh before
	// writes so a stale process does not overwrite newer persisted state.
	var data []byte
	var err error
	if s.db != nil {
		// 与 saveLocked 对齐：裸 context.Background 一旦 PG 抖动 / 主从切换
		// 会让 refreshLocked 永久挂起，而 refreshLocked 是所有 mutating 路径
		// 的前置（mutateAndSaveLocked / 直裸写者都走它）。整个 store mutex
		// 会跟着卡死，进而把 HTTP handler、scheduler、bot 全部排队挂起。
		// 30s 与 saveLocked 同档，超时由调用方拿到 context.DeadlineExceeded
		// 后走错误回滚 / 报错路径。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err = s.db.QueryRowContext(ctx, `SELECT state FROM twilight_state WHERE id = 1`).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			s.state = emptyState()
			return nil
		}
		if err != nil {
			return err
		}
	} else {
		if strings.TrimSpace(s.path) == "" {
			return nil
		}
		data, err = os.ReadFile(s.path)
		if errors.Is(err, os.ErrNotExist) {
			s.state = emptyState()
			return nil
		}
		if err != nil {
			return err
		}
	}
	var state State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			// 解析失败：尝试 fallback 到 .bak（saveLocked 写前快照），
			// 避免一次坏写或文件截断把整库拖死。日志里只暴露文件大小 +
			// 解析错误位置，不暴露内容（state.json 可能含 token）。
			if s.db == nil && strings.TrimSpace(s.path) != "" {
				if bak, bakErr := os.ReadFile(s.path + ".bak"); bakErr == nil && len(bak) > 0 {
					var bakState State
					if json.Unmarshal(bak, &bakState) == nil {
						bakState.ensure()
						s.state = bakState
						return nil
					}
				}
			}
			return err
		}
	}
	state.ensure()
	s.state = state
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
	// PG 后端：runtime logs 落在独立表 `twilight_runtime_logs`，不会进入
	// `twilight_state` 的 jsonb。Snapshot 必须把它们也读出来塞进 State，否则：
	//   - PG → JSON 迁移会永久丢日志；
	//   - 备份/恢复时 state 与 audit/runtime 两条线时点错位（admin 看到"已恢复"
	//     但日志仍是恢复点之后的最新数据）。
	// 这里持锁期间额外做一次 SELECT，不影响并发写（只读快照）。
	if s.db != nil {
		logs, nextID, err := s.snapshotRuntimeLogsLocked()
		if err != nil {
			return nil, err
		}
		state.RuntimeLogs = logs
		if nextID > state.NextRuntimeLogID {
			state.NextRuntimeLogID = nextID
		}
	}
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

func (s *Store) LoadSnapshot(data []byte) error {
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	state.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	if err := s.saveLocked(); err != nil {
		return err
	}
	// PG 后端：把 snapshot 里的 runtime_logs 显式回写到独立表，避免恢复后
	// twilight_state 走到老时点而 twilight_runtime_logs 仍是最新数据。
	// 失败不致命（state 已经写回），但要 surface 给调用方决定是否重试。
	if s.db != nil {
		if err := s.replaceRuntimeLogsLocked(state.RuntimeLogs); err != nil {
			return err
		}
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
		s.maintainTelegramIDIndex(0, u.TelegramID, u.UID)
		s.maintainEmbyIDIndex("", u.EmbyID, u.UID)
		created = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return created, nil
}

func (s *Store) usernameExistsLocked(username string) bool {
	for _, existing := range s.state.Users {
		if strings.EqualFold(existing.Username, username) {
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
		reg, err := s.consumableRegCodeLocked(regCode, now)
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
		s.maintainTelegramIDIndex(0, u.TelegramID, u.UID)
		s.maintainEmbyIDIndex("", u.EmbyID, u.UID)
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
			reg, err := s.consumableRegCodeLocked(regCode, now)
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
		s.maintainTelegramIDIndex(0, u.TelegramID, u.UID)
		s.maintainEmbyIDIndex("", u.EmbyID, u.UID)
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

func normalizeTelegramUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

func (s *Store) FindUserByUsername(username string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.state.Users {
		if strings.EqualFold(u.Username, username) {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) FindUserByEmail(email string) (User, bool) {
	if email == "" {
		return User{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.state.Users {
		if strings.EqualFold(strings.TrimSpace(u.Email), email) {
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
		oldTGID := u.TelegramID
		oldEmbyID := u.EmbyID
		if err := fn(&u); err != nil {
			return err
		}
		if u.EmbyID != oldEmbyID && s.embyIDTakenLocked(u.EmbyID, uid) {
			return ErrConflict
		}
		s.state.Users[uid] = u
		s.maintainTelegramIDIndex(oldTGID, u.TelegramID, uid)
		s.maintainEmbyIDIndex(oldEmbyID, u.EmbyID, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return updated, nil
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
			u.Email = ""
			s.state.Users[uid] = u
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
		s.state = prev
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

// BindUserTelegramAtomic 同把锁内：唯一性校验 + admin 自保 + 写入。
// 解决 handleAdminBindTelegram 闭包外 FindUserByTelegramID 与 UpdateUser
// 闭包写之间的 TOCTOU。
func (s *Store) BindUserTelegramAtomic(uid int64, tgid int64, currentUID int64) (User, int64, error) {
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
		if tgid != 0 {
			for _, other := range s.state.Users {
				if other.UID != uid && other.TelegramID == tgid {
					return ErrConflict
				}
			}
		}
		old = u.TelegramID
		u.TelegramID = tgid
		s.state.Users[uid] = u
		s.maintainTelegramIDIndex(old, tgid, uid)
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
				s.rebuildEmbyIDIndex()
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
		oldEmbyID := u.EmbyID
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
		if u.EmbyID != oldEmbyID && s.embyIDTakenLocked(u.EmbyID, uid) {
			return ErrConflict
		}
		s.state.Users[uid] = u
		s.maintainEmbyIDIndex(oldEmbyID, u.EmbyID, uid)
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
		s.maintainTelegramIDIndex(u.TelegramID, 0, uid)
		s.maintainEmbyIDIndex(u.EmbyID, "", uid)
		delete(s.state.Users, uid)

		// API keys 与会话凭证：必须清理，否则用户被删后旧 key 仍可调用接口。
		for id, key := range s.state.APIKeys {
			if key.UID == uid {
				delete(s.state.APIKeys, id)
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]User, 0, len(s.state.Users))
	for _, u := range s.state.Users {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].UID < users[j].UID })
	return users
}

// rebuildTelegramIDIndex 从当前 Users map 重建 telegramID → UID 索引。
// 在 Open / OpenPostgres 加载 state 后调用，无需持锁（调用方已持有写锁或尚未暴露引用）。
func (s *Store) rebuildTelegramIDIndex() {
	m := make(map[int64]int64, len(s.state.Users))
	for _, u := range s.state.Users {
		if u.TelegramID != 0 {
			m[u.TelegramID] = u.UID
		}
	}
	s.telegramIDMap = m
}

// maintainTelegramIDIndex 在用户变更后增量维护 telegramID → UID 索引。
// 调用方须持有 s.mu 写锁。索引为空时自动从 state 重建。
func (s *Store) maintainTelegramIDIndex(oldTGID, newTGID int64, uid int64) {
	if s.telegramIDMap == nil {
		s.rebuildTelegramIDIndex()
	}
	if oldTGID != 0 {
		delete(s.telegramIDMap, oldTGID)
	}
	if newTGID != 0 {
		s.telegramIDMap[newTGID] = uid
	}
}

func (s *Store) rebuildUserIndexes() {
	s.rebuildTelegramIDIndex()
	s.rebuildEmbyIDIndex()
}

// rebuildEmbyIDIndex 从当前 Users map 重建 embyID → UID 索引。
// 在 Open / OpenPostgres 加载 state 后调用，无需持锁（调用方已持有写锁或尚未暴露引用）。
func (s *Store) rebuildEmbyIDIndex() {
	m := make(map[string]int64, len(s.state.Users))
	for _, u := range s.state.Users {
		if u.EmbyID != "" {
			m[u.EmbyID] = u.UID
		}
	}
	s.embyIDMap = m
}

// maintainEmbyIDIndex 在用户变更后增量维护 embyID → UID 索引。
// 调用方必须已持有 s.mu 写锁。
func (s *Store) maintainEmbyIDIndex(oldEmbyID, newEmbyID string, uid int64) {
	if s.embyIDMap == nil {
		s.rebuildEmbyIDIndex()
	}
	if oldEmbyID != "" {
		delete(s.embyIDMap, oldEmbyID)
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
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}
	return k, nil
}

func (s *Store) ListAPIKeys(uid int64) []APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]APIKey, 0)
	for _, k := range s.state.APIKeys {
		if k.UID == uid {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	return keys
}

func (s *Store) FindAPIKeyByHash(hash string) (APIKey, User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// hash 用常量时间比对，避免攻击者通过响应时间差推断 hash 前缀
	// 加速 API key 爆破。Enabled / Status 检查
	// 仍是普通短路，因为它们不携带秘密值。
	hashBytes := []byte(hash)
	for _, k := range s.state.APIKeys {
		if !k.Enabled {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(k.Hash), hashBytes) == 1 {
			u, ok := s.state.Users[k.UID]
			return k, u, ok
		}
	}
	for _, u := range s.state.Users {
		if !u.LegacyAPIKeyStatus {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(u.LegacyAPIKeyHash), hashBytes) == 1 {
			return APIKey{UID: u.UID, Enabled: true, Permissions: u.LegacyPermissions, RateLimit: 100}, u, true
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

func (s *Store) RecordAPIKeyUse(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		k, ok := s.state.APIKeys[id]
		if !ok {
			return ErrNotFound
		}
		k.RequestCount++
		k.LastUsed = time.Now().Unix()
		s.state.APIKeys[id] = k
		return nil
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.MediaRequests[id]; !ok {
			return ErrNotFound
		}
		delete(s.state.MediaRequests, id)
		return nil
	})
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
			u.TelegramID = telegramID
			u.TelegramUsername = bind.TelegramUsername
			s.state.Users[u.UID] = u
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
		s.rebuildTelegramIDIndex()
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
		if !includeHidden && (!a.Visible || (a.ExpiredAt > 0 && a.ExpiredAt < now)) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].ID > out[j].ID
	})
	return out
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

// ---- Tickets ----

func NormalizeTicketStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case TicketStatusInProgress:
		return TicketStatusInProgress
	case TicketStatusResolved:
		return TicketStatusResolved
	case TicketStatusClosed:
		return TicketStatusClosed
	default:
		return TicketStatusOpen
	}
}

func ValidTicketStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case TicketStatusOpen, TicketStatusInProgress, TicketStatusResolved, TicketStatusClosed:
		return true
	}
	return false
}

func NormalizeTicketPriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case TicketPriorityLow:
		return TicketPriorityLow
	case TicketPriorityHigh:
		return TicketPriorityHigh
	case TicketPriorityUrgent:
		return TicketPriorityUrgent
	default:
		return TicketPriorityMedium
	}
}

func ValidTicketPriority(priority string) bool {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case TicketPriorityLow, TicketPriorityMedium, TicketPriorityHigh, TicketPriorityUrgent:
		return true
	}
	return false
}

func NormalizeTicketType(types []string, input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = TicketTypeDefault
	}
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), trimmed) {
			return strings.TrimSpace(t)
		}
	}
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), TicketTypeDefault) {
			return strings.TrimSpace(t)
		}
	}
	if len(types) > 0 {
		if first := strings.TrimSpace(types[0]); first != "" {
			return first
		}
	}
	return TicketTypeDefault
}

func TicketStatusOpenForQuota(status string) bool {
	switch NormalizeTicketStatus(status) {
	case TicketStatusOpen, TicketStatusInProgress:
		return true
	}
	return false
}

func TicketStatusAllowsConversation(status string) bool {
	return NormalizeTicketStatus(status) != TicketStatusClosed
}

func applyTicketStatusTimestamps(t *Ticket, existing Ticket, now int64) {
	previousStatus := NormalizeTicketStatus(existing.Status)
	nextStatus := NormalizeTicketStatus(t.Status)
	t.Status = nextStatus
	if t.ResolvedAt == 0 {
		t.ResolvedAt = existing.ResolvedAt
	}
	if t.ClosedAt == 0 {
		t.ClosedAt = existing.ClosedAt
	}
	if nextStatus == TicketStatusResolved {
		if previousStatus != TicketStatusResolved || t.ResolvedAt == 0 {
			t.ResolvedAt = now
		}
	} else {
		t.ResolvedAt = 0
	}
	if nextStatus == TicketStatusClosed {
		if previousStatus != TicketStatusClosed || t.ClosedAt == 0 {
			t.ClosedAt = now
		}
	} else {
		t.ClosedAt = 0
	}
}

func (s *Store) UpsertTicket(t Ticket) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		t = s.upsertTicketLocked(t, time.Now().Unix())
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return t, nil
}

func (s *Store) CreateTicket(t Ticket, userOpenLimit, globalOpenLimit int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		if userOpenLimit > 0 && s.countOpenTicketsLocked(t.UID) >= userOpenLimit {
			return ErrTicketUserOpenLimit
		}
		if globalOpenLimit > 0 && s.countOpenTicketsLocked(0) >= globalOpenLimit {
			return ErrTicketGlobalOpenLimit
		}
		t.ID = 0
		if strings.TrimSpace(t.Status) == "" {
			t.Status = TicketStatusOpen
		}
		t = s.upsertTicketLocked(t, time.Now().Unix())
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return t, nil
}

func (s *Store) upsertTicketLocked(t Ticket, now int64) Ticket {
	t.Type = NormalizeTicketType(s.state.TicketTypes, t.Type)
	t.Status = NormalizeTicketStatus(t.Status)
	t.Priority = NormalizeTicketPriority(t.Priority)
	if t.ID == 0 {
		t.ID = s.state.NextTicketID
		s.state.NextTicketID++
		t.CreatedAt = now
	} else if existing, ok := s.state.Tickets[t.ID]; ok {
		if t.CreatedAt == 0 {
			t.CreatedAt = existing.CreatedAt
		}
		if t.UID == 0 {
			t.UID = existing.UID
		}
		if t.Username == "" {
			t.Username = existing.Username
		}
		// 附件由专用方法维护，普通 upsert 不携带附件时保留原有列表，
		// 避免管理员更新状态 / 用户改内容时把交流图片清空。
		if t.Attachments == nil {
			t.Attachments = existing.Attachments
		}
		if t.Replies == nil {
			t.Replies = existing.Replies
		}
		// NotifyTelegram nil 表示沿用已有值，避免更新其他字段时意外重置。
		if t.NotifyTelegram == nil {
			t.NotifyTelegram = existing.NotifyTelegram
		}
		applyTicketStatusTimestamps(&t, existing, now)
	} else {
		empty := Ticket{Status: TicketStatusOpen}
		applyTicketStatusTimestamps(&t, empty, now)
	}
	t.UpdatedAt = now
	s.state.Tickets[t.ID] = t
	return t
}

func (s *Store) ListTickets(filter TicketFilter) []Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Ticket, 0)
	for _, t := range s.state.Tickets {
		if filter.UID > 0 && t.UID != filter.UID {
			continue
		}
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		if filter.Type != "" && t.Type != filter.Type {
			continue
		}
		if filter.Priority != "" && t.Priority != filter.Priority {
			continue
		}
		if filter.ActiveOnly && !TicketStatusOpenForQuota(t.Status) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

type TicketFilter struct {
	UID        int64
	Status     string
	Type       string
	Priority   string
	ActiveOnly bool
}

func (s *Store) Ticket(id int64) (Ticket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.state.Tickets[id]
	return t, ok
}

func (s *Store) DeleteTicket(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.Tickets[id]; !ok {
			return ErrNotFound
		}
		delete(s.state.Tickets, id)
		return nil
	})
}

// CountUserOpenTickets 统计某用户当前处于待处理 / 处理中的工单数量。
func (s *Store) CountUserOpenTickets(uid int64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countOpenTicketsLocked(uid)
}

// CountOpenTickets 统计全局处于待处理 / 处理中的工单数量。
func (s *Store) CountOpenTickets() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countOpenTicketsLocked(0)
}

func (s *Store) countOpenTicketsLocked(uid int64) int {
	count := 0
	for _, t := range s.state.Tickets {
		if (uid == 0 || t.UID == uid) && TicketStatusOpenForQuota(t.Status) {
			count++
		}
	}
	return count
}

func (s *Store) UpdateTicket(ticketID int64, patch TicketUpdate) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		now := time.Now().Unix()
		existing := t
		if patch.Status != nil {
			t.Status = NormalizeTicketStatus(*patch.Status)
		}
		if patch.Priority != nil {
			t.Priority = NormalizeTicketPriority(*patch.Priority)
		}
		if patch.Type != nil {
			t.Type = NormalizeTicketType(s.state.TicketTypes, *patch.Type)
		}
		if patch.AdminNote != nil {
			t.AdminNote = strings.TrimSpace(*patch.AdminNote)
		}
		applyTicketStatusTimestamps(&t, existing, now)
		if patch.Reply != nil {
			applyTicketReplyLocked(&t, *patch.Reply, now)
		}
		t.UpdatedAt = now
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

func (s *Store) SetTicketNotify(ticketID int64, enabled bool) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		t.NotifyTelegram = &enabled
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// AddTicketAttachment 给工单追加一张图片元数据。返回更新后的工单。
func (s *Store) AddTicketAttachment(ticketID int64, att TicketAttachment, actorRole int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if !TicketStatusAllowsConversation(t.Status) && actorRole != RoleAdmin {
			return ErrTicketClosed
		}
		if att.CreatedAt == 0 {
			att.CreatedAt = time.Now().Unix()
		}
		t.Attachments = append(t.Attachments, att)
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// AddTicketReply 向工单追加一条回复。
func (s *Store) AddTicketReply(ticketID int64, reply TicketReply) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if !TicketStatusAllowsConversation(t.Status) && reply.Role != RoleAdmin {
			return ErrTicketClosed
		}
		now := time.Now().Unix()
		applyTicketReplyLocked(&t, reply, now)
		t.UpdatedAt = now
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

func applyTicketReplyLocked(t *Ticket, reply TicketReply, now int64) {
	if reply.CreatedAt == 0 {
		reply.CreatedAt = now
	}
	t.Replies = append(t.Replies, reply)
	if reply.Role == RoleAdmin && NormalizeTicketStatus(t.Status) == TicketStatusOpen {
		t.Status = TicketStatusInProgress
	}
	if reply.Role != RoleAdmin && NormalizeTicketStatus(t.Status) == TicketStatusResolved {
		t.Status = TicketStatusOpen
		t.ResolvedAt = 0
	}
}

// RemoveTicketAttachment 从工单移除指定文件名的图片元数据。返回更新后的工单。
func (s *Store) RemoveTicketAttachment(ticketID int64, filename string, actorRole int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if !TicketStatusAllowsConversation(t.Status) && actorRole != RoleAdmin {
			return ErrTicketClosed
		}
		idx := -1
		for i, att := range t.Attachments {
			if att.Filename == filename {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		t.Attachments = append(t.Attachments[:idx], t.Attachments[idx+1:]...)
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// ClosedTicketsWithAttachmentsBefore 返回所有已关闭且 ClosedAt 早于 cutoff 的工单，
// 用于定时清理过期的工单交流图片。
func (s *Store) ClosedTicketsWithAttachmentsBefore(cutoff int64) []Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Ticket, 0)
	for _, t := range s.state.Tickets {
		if NormalizeTicketStatus(t.Status) == TicketStatusClosed && t.ClosedAt > 0 && t.ClosedAt < cutoff && len(t.Attachments) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// ClearTicketAttachments 清空某工单的图片元数据（用于保留期清理后同步状态）。
func (s *Store) ClearTicketAttachments(ticketID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if len(t.Attachments) == 0 {
			return nil
		}
		t.Attachments = nil
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		return nil
	})
}

// TicketTypes 返回当前工单类型列表。
func (s *Store) TicketTypes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.state.TicketTypes))
	copy(out, s.state.TicketTypes)
	return out
}

// SetTicketTypes 原子替换工单类型列表。
func (s *Store) SetTicketTypes(types []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalized := normalizeTicketTypeList(types)
	// 确保至少一个类型
	if len(normalized) == 0 {
		normalized = []string{TicketTypeDefault}
	}
	return s.mutateAndSaveLocked(func() error {
		s.state.TicketTypes = normalized
		return nil
	})
}

// AddTicketType 添加工单类型，已存在则返回 ErrConflict。
func (s *Store) AddTicketType(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if !validTicketTypeName(name) {
		return ErrInvalid
	}
	return s.mutateAndSaveLocked(func() error {
		for _, t := range s.state.TicketTypes {
			if strings.EqualFold(t, name) {
				return ErrConflict
			}
		}
		s.state.TicketTypes = append(s.state.TicketTypes, name)
		return nil
	})
}

// DeleteTicketType 删除工单类型，不允许删除最后一个。
func (s *Store) DeleteTicketType(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if !validTicketTypeName(name) {
		return ErrInvalid
	}
	return s.mutateAndSaveLocked(func() error {
		if len(s.state.TicketTypes) <= 1 {
			return ErrConflict
		}
		idx := -1
		for i, t := range s.state.TicketTypes {
			if strings.EqualFold(t, name) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		s.state.TicketTypes = append(s.state.TicketTypes[:idx], s.state.TicketTypes[idx+1:]...)
		return nil
	})
}

// RenameTicketType 重命名工单类型。
func (s *Store) RenameTicketType(oldName, newName string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if !validTicketTypeName(oldName) || !validTicketTypeName(newName) {
		return 0, ErrInvalid
	}
	err := s.mutateAndSaveLocked(func() error {
		for _, t := range s.state.TicketTypes {
			if strings.EqualFold(t, newName) && !strings.EqualFold(t, oldName) {
				return ErrConflict
			}
		}
		found := false
		for i, t := range s.state.TicketTypes {
			if strings.EqualFold(t, oldName) {
				s.state.TicketTypes[i] = newName
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		now := time.Now().Unix()
		for id, ticket := range s.state.Tickets {
			if strings.EqualFold(ticket.Type, oldName) {
				ticket.Type = newName
				ticket.UpdatedAt = now
				s.state.Tickets[id] = ticket
				count++
			}
		}
		return nil
	})
	return count, err
}

// SyncTicketTypesFromConfig 从配置同步类型（首次启动或配置变更时调用），
// 配置中的类型会覆盖 store 中的类型。
func (s *Store) SyncTicketTypesFromConfig(cfgTypes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	types := normalizeTicketTypeList(cfgTypes)
	if len(types) == 0 {
		return
	}
	_ = s.mutateAndSaveLocked(func() error {
		s.state.TicketTypes = types
		return nil
	})
}

func normalizeTicketTypeList(types []string) []string {
	out := make([]string, 0, len(types))
	seen := map[string]bool{}
	for _, raw := range types {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func validTicketTypeName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && len(name) <= 50
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
		c, ok := s.state.InviteCodes[code]
		if !ok || c.UID != uid {
			return ErrNotFound
		}
		if c.UseCount > 0 || c.Used {
			c.Active = false
			s.state.InviteCodes[code] = c
		} else {
			delete(s.state.InviteCodes, code)
		}
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
		oldTGID := u.TelegramID
		oldEmbyID := u.EmbyID
		if fn != nil {
			if err := fn(&u, c); err != nil {
				return err
			}
		}
		if u.TelegramID != oldTGID && s.telegramIDTakenLocked(u.TelegramID, childUID) {
			return ErrConflict
		}
		if u.EmbyID != oldEmbyID && s.embyIDTakenLocked(u.EmbyID, childUID) {
			return ErrConflict
		}
		s.state.Users[childUID] = u
		s.maintainTelegramIDIndex(oldTGID, u.TelegramID, childUID)
		s.maintainEmbyIDIndex(oldEmbyID, u.EmbyID, childUID)
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

// UpsertRegCodes 在一次状态写入中批量插入/更新注册码。批量生成注册码时用它替代
// 逐条 UpsertRegCode，避免每条都触发一次全量状态落盘（saveLocked 每次序列化整个
// state，逐条写入时磁盘开销随数量线性放大）。单条字段默认值与 UpsertRegCode 保持一致。
func (s *Store) UpsertRegCodes(codes []RegCode) error {
	if len(codes) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		now := time.Now().Unix()
		for _, code := range codes {
			_, exists := s.state.RegCodes[code.Code]
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
			if !exists && !code.Active && code.UseCount == 0 {
				code.Active = true
			}
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
		r, err := s.consumableRegCodeLocked(code, now)
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
		r, err := s.consumableRegCodeLocked(code, now)
		if err != nil {
			return err
		}
		if telegramID == 0 {
			telegramID = u.TelegramID
		}
		consumed = s.consumeRegCodeLocked(r, uid, telegramID)
		oldTGID := u.TelegramID
		oldEmbyID := u.EmbyID
		if fn != nil {
			if err := fn(&u, consumed); err != nil {
				return err
			}
		}
		if u.TelegramID != oldTGID && s.telegramIDTakenLocked(u.TelegramID, uid) {
			return ErrConflict
		}
		if u.EmbyID != oldEmbyID && s.embyIDTakenLocked(u.EmbyID, uid) {
			return ErrConflict
		}
		s.state.Users[uid] = u
		s.maintainTelegramIDIndex(oldTGID, u.TelegramID, uid)
		s.maintainEmbyIDIndex(oldEmbyID, u.EmbyID, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, RegCode{}, err
	}
	return updated, consumed, nil
}

func isRegCodeExpiredLocked(r RegCode, now int64) bool {
	if r.ValidityTime <= 0 {
		return false
	}
	elapsed := now - r.CreatedAt - r.PausedSeconds
	if r.PauseStart > 0 {
		elapsed = r.PauseStart - r.CreatedAt - r.PausedSeconds
	}
	return elapsed >= r.ValidityTime*3600
}

func (s *Store) consumableRegCodeLocked(code string, now int64) (RegCode, error) {
	r, ok := s.state.RegCodes[code]
	if !ok || !r.Active {
		return RegCode{}, ErrNotFound
	}
	if r.UseCountLimit != -1 && r.UseCount >= r.UseCountLimit {
		return RegCode{}, ErrConflict
	}
	if isRegCodeExpiredLocked(r, now) {
		return RegCode{}, ErrExpired
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

func (s *Store) DeleteRegCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.RegCodes[code]; !ok {
			return ErrNotFound
		}
		delete(s.state.RegCodes, code)
		return nil
	})
}

func (s *Store) DeleteRegCodes(codes []string) (deleted []string, missing []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

func (s *Store) UpsertTelegramRoster(chatID string, telegramID int64, status string, isBot bool) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || telegramID <= 0 {
		return nil
	}
	if status == "" {
		status = "member"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		key := telegramRosterKey(chatID, telegramID)
		now := time.Now().Unix()
		entry, ok := s.state.TelegramRoster[key]
		if !ok {
			entry = TelegramRosterEntry{ChatID: chatID, TelegramID: telegramID, FirstSeen: now}
		}
		entry.LastSeen = now
		entry.LastStatus = status
		if isBot {
			entry.IsBot = true
		}
		s.state.TelegramRoster[key] = entry
		return nil
	})
}

func (s *Store) MarkTelegramRosterLeft(chatID string, telegramID int64, status string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || telegramID <= 0 {
		return nil
	}
	if status == "" {
		status = "left"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		key := telegramRosterKey(chatID, telegramID)
		entry, ok := s.state.TelegramRoster[key]
		if !ok {
			entry = TelegramRosterEntry{ChatID: chatID, TelegramID: telegramID, FirstSeen: time.Now().Unix()}
		}
		entry.LastSeen = time.Now().Unix()
		entry.LastStatus = status
		s.state.TelegramRoster[key] = entry
		return nil
	})
}

func (s *Store) ApplyTelegramRosterUpdates(updates []TelegramRosterUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		now := time.Now().Unix()
		for _, update := range updates {
			chatID := strings.TrimSpace(update.ChatID)
			if chatID == "" || update.TelegramID <= 0 {
				continue
			}
			status := strings.TrimSpace(update.Status)
			if status == "" {
				status = "member"
			}
			key := telegramRosterKey(chatID, update.TelegramID)
			entry, ok := s.state.TelegramRoster[key]
			if !ok {
				entry = TelegramRosterEntry{ChatID: chatID, TelegramID: update.TelegramID, FirstSeen: now}
			}
			entry.LastSeen = now
			entry.LastStatus = status
			if update.IsBot {
				entry.IsBot = true
			}
			s.state.TelegramRoster[key] = entry
		}
		return nil
	})
}

func (s *Store) TelegramRoster(chatID string, activeOnly bool) []TelegramRosterEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TelegramRosterEntry, 0)
	for _, entry := range s.state.TelegramRoster {
		if chatID != "" && entry.ChatID != chatID {
			continue
		}
		if activeOnly && !telegramRosterActive(entry.LastStatus) {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ChatID == out[j].ChatID {
			return out[i].TelegramID < out[j].TelegramID
		}
		return out[i].ChatID < out[j].ChatID
	})
	return out
}

func (s *Store) TelegramRosterStats(chatID string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string]any{"chat_id": chatID, "total": 0, "active": 0, "inactive": 0, "bots": 0, "first_seen_at": nil, "last_seen_at": nil}
	firstSeen := int64(0)
	lastSeen := int64(0)
	total := 0
	active := 0
	inactive := 0
	bots := 0
	for _, entry := range s.state.TelegramRoster {
		if chatID != "" && entry.ChatID != chatID {
			continue
		}
		total++
		if entry.IsBot {
			bots++
		}
		if telegramRosterActive(entry.LastStatus) {
			active++
		} else {
			inactive++
		}
		if entry.FirstSeen > 0 && (firstSeen == 0 || entry.FirstSeen < firstSeen) {
			firstSeen = entry.FirstSeen
		}
		if entry.LastSeen > lastSeen {
			lastSeen = entry.LastSeen
		}
	}
	result["total"] = total
	result["active"] = active
	result["inactive"] = inactive
	result["bots"] = bots
	if firstSeen > 0 {
		result["first_seen_at"] = firstSeen
	}
	if lastSeen > 0 {
		result["last_seen_at"] = lastSeen
	}
	return result
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

func telegramRosterKey(chatID string, telegramID int64) string {
	return strings.TrimSpace(chatID) + ":" + strconv36(telegramID)
}

func telegramRosterActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "member", "administrator", "creator", "restricted":
		return true
	default:
		return false
	}
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

// ---------------------------------------------------------------------------
// AuditLog — 操作审计日志
// ---------------------------------------------------------------------------

// AddAuditLog 追加一条操作审计日志。自动分配单调递增 ID 和时间戳。
// limit 为保留上限条数（<=0 不限）。
func (s *Store) AddAuditLog(entry AuditLog, limit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}

	// Audit logs are append-only. Keep a rollback header instead of serializing and
	// deserializing the entire State through mutateAndSaveLocked for every audited
	// mutation. saveLocked still persists the complete state, but this removes one
	// full-state marshal and unmarshal from the high-frequency audit path.
	previousLogs := s.state.AuditLogs
	previousNextID := s.state.NextAuditLogID
	entry.Detail = cloneAuditLogDetail(entry.Detail)
	entry.ID = s.state.NextAuditLogID
	s.state.NextAuditLogID++
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}
	s.state.AuditLogs = append(s.state.AuditLogs, entry)
	if limit > 0 && len(s.state.AuditLogs) > limit {
		s.state.AuditLogs = s.state.AuditLogs[len(s.state.AuditLogs)-limit:]
	}
	if err := s.saveLocked(); err != nil {
		s.state.AuditLogs = previousLogs
		s.state.NextAuditLogID = previousNextID
		return err
	}
	return nil
}

// ListAuditLogs 返回所有审计日志，最新在前。
func (s *Store) ListAuditLogs() []AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AuditLog, len(s.state.AuditLogs))
	for i, entry := range s.state.AuditLogs {
		out[i] = cloneAuditLogEntry(entry)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// DeleteAuditLog 按 ID 删除单条审计日志。
func (s *Store) DeleteAuditLog(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		for i, log := range s.state.AuditLogs {
			if log.ID == id {
				s.state.AuditLogs = append(s.state.AuditLogs[:i], s.state.AuditLogs[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

// ClearAuditLogs 清空全部审计日志。
func (s *Store) ClearAuditLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.AuditLogs = nil
		return nil
	})
}

// PruneAuditLogs 保留最新的 keep 条审计日志，超出部分从尾部裁剪。
func (s *Store) PruneAuditLogs(keep int) error {
	if keep <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if len(s.state.AuditLogs) > keep {
			s.state.AuditLogs = s.state.AuditLogs[len(s.state.AuditLogs)-keep:]
		}
		return nil
	})
}

// PruneAuditLogsByAge 删除早于 cutoff 的审计日志。preserveAdmin 为 true 时保留管理员操作。
func (s *Store) PruneAuditLogsByAge(cutoffUnix int64, preserveAdmin bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	_ = s.mutateAndSaveLocked(func() error {
		filtered := s.state.AuditLogs[:0]
		for _, log := range s.state.AuditLogs {
			if log.CreatedAt < cutoffUnix {
				if preserveAdmin && log.Category == "admin" {
					filtered = append(filtered, log)
					continue
				}
				removed++
				continue
			}
			filtered = append(filtered, log)
		}
		s.state.AuditLogs = filtered
		return nil
	})
	return removed
}

// AuditLogCount 返回当前审计日志总数。
func (s *Store) AuditLogCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.state.AuditLogs)
}

const maxStoredBangumiSyncLogs = 1000

func (s *Store) AddBangumiSyncLog(entry BangumiSyncLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		entry.ID = s.state.NextBangumiSyncLogID
		s.state.NextBangumiSyncLogID++
		if entry.CreatedAt == 0 {
			entry.CreatedAt = time.Now().Unix()
		}
		s.state.BangumiSyncLogs = append(s.state.BangumiSyncLogs, entry)
		if len(s.state.BangumiSyncLogs) > maxStoredBangumiSyncLogs {
			s.state.BangumiSyncLogs = compactTail(s.state.BangumiSyncLogs, maxStoredBangumiSyncLogs)
		}
		return nil
	})
}

func (s *Store) ListBangumiSyncLogs(uid int64, limit int) []BangumiSyncLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > maxStoredBangumiSyncLogs {
		limit = maxStoredBangumiSyncLogs
	}
	out := make([]BangumiSyncLog, 0, limit)
	for i := len(s.state.BangumiSyncLogs) - 1; i >= 0; i-- {
		entry := s.state.BangumiSyncLogs[i]
		if uid != 0 && entry.UID != uid {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) DeleteBangumiSyncLog(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		for i, log := range s.state.BangumiSyncLogs {
			if log.ID == id {
				s.state.BangumiSyncLogs = append(s.state.BangumiSyncLogs[:i], s.state.BangumiSyncLogs[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) ClearBangumiSyncLogs(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if uid == 0 {
			s.state.BangumiSyncLogs = nil
			return nil
		}
		filtered := make([]BangumiSyncLog, 0, len(s.state.BangumiSyncLogs))
		for _, log := range s.state.BangumiSyncLogs {
			if log.UID != uid {
				filtered = append(filtered, log)
			}
		}
		s.state.BangumiSyncLogs = filtered
		return nil
	})
}

func bangumiCollectionCacheKey(uid int64, collectType int) string {
	return strconv.FormatInt(uid, 10) + ":" + strconv.Itoa(collectType)
}

const maxBangumiSubjectCacheEntries = 5000

func bangumiSubjectCacheKey(subjectID int64) string {
	return strconv.FormatInt(subjectID, 10)
}

func bangumiSubjectIDFromValue(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func bangumiCollectionSubjectID(item map[string]any) int64 {
	if item == nil {
		return 0
	}
	if id := bangumiSubjectIDFromValue(item["subject_id"]); id > 0 {
		return id
	}
	if subject, ok := item["subject"].(map[string]any); ok {
		return bangumiSubjectIDFromValue(subject["id"])
	}
	return 0
}

func cloneBangumiMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	var copied map[string]any
	data, err := json.Marshal(value)
	if err == nil {
		_ = json.Unmarshal(data, &copied)
	}
	if copied != nil {
		return copied
	}
	copied = make(map[string]any, len(value))
	for key, item := range value {
		copied[key] = item
	}
	return copied
}

func cloneBangumiMapSlice(entries []map[string]any) []map[string]any {
	if entries == nil {
		return nil
	}
	copied := make([]map[string]any, len(entries))
	for i, item := range entries {
		copied[i] = cloneBangumiMap(item)
	}
	return copied
}

func cloneBangumiCollectionCacheEntry(entry BangumiCollectionCacheEntry) BangumiCollectionCacheEntry {
	entry.Entries = cloneBangumiMapSlice(entry.Entries)
	return entry
}

func hydrateBangumiCollectionCacheEntry(entry BangumiCollectionCacheEntry, subjects map[string]BangumiSubjectCacheEntry) BangumiCollectionCacheEntry {
	entry = cloneBangumiCollectionCacheEntry(entry)
	for i, item := range entry.Entries {
		if item == nil {
			continue
		}
		if _, ok := item["subject"]; ok {
			continue
		}
		subjectID := bangumiCollectionSubjectID(item)
		if subjectID <= 0 {
			continue
		}
		if cached, ok := subjects[bangumiSubjectCacheKey(subjectID)]; ok && cached.Subject != nil {
			item["subject"] = cloneBangumiMap(cached.Subject)
			entry.Entries[i] = item
		}
	}
	return entry
}

func normalizeBangumiCollectionCacheEntry(entry BangumiCollectionCacheEntry, now int64, subjects map[string]BangumiSubjectCacheEntry) BangumiCollectionCacheEntry {
	entry = cloneBangumiCollectionCacheEntry(entry)
	for i, item := range entry.Entries {
		if item == nil {
			continue
		}
		subjectID := bangumiCollectionSubjectID(item)
		if subjectID <= 0 {
			continue
		}
		if _, ok := item["subject_id"]; !ok {
			item["subject_id"] = subjectID
		}
		if subject, ok := item["subject"].(map[string]any); ok && subject != nil {
			subjectCopy := cloneBangumiMap(subject)
			if _, ok := subjectCopy["id"]; !ok {
				subjectCopy["id"] = subjectID
			}
			subjects[bangumiSubjectCacheKey(subjectID)] = BangumiSubjectCacheEntry{
				SubjectID: subjectID,
				Subject:   subjectCopy,
				UpdatedAt: now,
				ExpiresAt: entry.ExpiresAt,
			}
			delete(item, "subject")
		}
		entry.Entries[i] = item
	}
	return entry
}

func (s *State) normalizeBangumiCollectionSubjectCache() {
	if s.BangumiCollectionCache == nil {
		return
	}
	if s.BangumiSubjectCache == nil {
		s.BangumiSubjectCache = map[string]BangumiSubjectCacheEntry{}
	}
	now := time.Now().Unix()
	for key, entry := range s.BangumiCollectionCache {
		updatedAt := entry.UpdatedAt
		if updatedAt == 0 {
			updatedAt = now
		}
		s.BangumiCollectionCache[key] = normalizeBangumiCollectionCacheEntry(entry, updatedAt, s.BangumiSubjectCache)
	}
	pruneBangumiSubjectCacheLocked(s.BangumiSubjectCache)
}

func pruneBangumiSubjectCacheLocked(subjects map[string]BangumiSubjectCacheEntry) {
	if len(subjects) <= maxBangumiSubjectCacheEntries {
		return
	}
	type cacheItem struct {
		key       string
		updatedAt int64
	}
	items := make([]cacheItem, 0, len(subjects))
	for key, entry := range subjects {
		items = append(items, cacheItem{key: key, updatedAt: entry.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].updatedAt == items[j].updatedAt {
			return items[i].key < items[j].key
		}
		return items[i].updatedAt < items[j].updatedAt
	})
	for len(subjects) > maxBangumiSubjectCacheEntries && len(items) > 0 {
		delete(subjects, items[0].key)
		items = items[1:]
	}
}

func (s *Store) BangumiCollectionCache(uid int64, collectType int) (BangumiCollectionCacheEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.BangumiCollectionCache[bangumiCollectionCacheKey(uid, collectType)]
	if !ok {
		return BangumiCollectionCacheEntry{}, false
	}
	return hydrateBangumiCollectionCacheEntry(entry, s.state.BangumiSubjectCache), true
}

func (s *Store) RawBangumiSubjectCache(subjectID int64) (BangumiSubjectCacheEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.BangumiSubjectCache[bangumiSubjectCacheKey(subjectID)]
	return entry, ok
}

func (s *Store) UpsertBangumiCollectionCache(entry BangumiCollectionCacheEntry) error {
	if entry.UID <= 0 || entry.Type <= 0 {
		return fmt.Errorf("invalid bangumi collection cache key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if s.state.BangumiCollectionCache == nil {
			s.state.BangumiCollectionCache = map[string]BangumiCollectionCacheEntry{}
		}
		if s.state.BangumiSubjectCache == nil {
			s.state.BangumiSubjectCache = map[string]BangumiSubjectCacheEntry{}
		}
		if entry.UpdatedAt == 0 {
			entry.UpdatedAt = time.Now().Unix()
		}
		entry = normalizeBangumiCollectionCacheEntry(entry, entry.UpdatedAt, s.state.BangumiSubjectCache)
		s.state.BangumiCollectionCache[bangumiCollectionCacheKey(entry.UID, entry.Type)] = entry
		pruneBangumiSubjectCacheLocked(s.state.BangumiSubjectCache)
		return nil
	})
}

func (s *Store) MarkBangumiCollectionCacheError(uid int64, collectType int, message string) error {
	if uid <= 0 || collectType <= 0 {
		return fmt.Errorf("invalid bangumi collection cache key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if s.state.BangumiCollectionCache == nil {
			s.state.BangumiCollectionCache = map[string]BangumiCollectionCacheEntry{}
		}
		key := bangumiCollectionCacheKey(uid, collectType)
		entry := s.state.BangumiCollectionCache[key]
		entry.UID = uid
		entry.Type = collectType
		entry.LastError = message
		entry.LastErrorAt = time.Now().Unix()
		s.state.BangumiCollectionCache[key] = entry
		return nil
	})
}

func (s *Store) DeleteBangumiCollectionCache(uid int64, collectType int) error {
	if uid <= 0 {
		return fmt.Errorf("invalid bangumi collection cache key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if s.state.BangumiCollectionCache == nil {
			return nil
		}
		if collectType > 0 {
			delete(s.state.BangumiCollectionCache, bangumiCollectionCacheKey(uid, collectType))
			return nil
		}
		for key, entry := range s.state.BangumiCollectionCache {
			if entry.UID == uid {
				delete(s.state.BangumiCollectionCache, key)
			}
		}
		return nil
	})
}

// TelegramBotOffset 读取持久化的 getUpdates offset。新部署 / 历史 state
// 没有该字段时返回 0，调用方按"未设置"处理。
func (s *Store) TelegramBotOffset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return 0
	}
	return s.state.TelegramBotOffset
}

// SetTelegramBotOffset 持久化 offset。仅当传入值大于当前值才写入：getUpdates
// 是单调推进的，倒退（极少见，仅 token 切换 / 测试场景）应当走显式
// ResetTelegramBotOffset，不通过常规写路径意外覆盖。
func (s *Store) SetTelegramBotOffset(offset int64) error {
	if offset <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if offset > s.state.TelegramBotOffset {
			s.state.TelegramBotOffset = offset
		}
		return nil
	})
}

// ResetTelegramBotOffset 主动清零，bot 在检测到 username 变更（不同 bot
// 实例）时调用，避免错把旧 bot 的 offset 套到新 bot 上。
func (s *Store) ResetTelegramBotOffset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.TelegramBotOffset = 0
		return nil
	})
}

var (
	ErrNotFound                      = errors.New("not found")
	ErrInvalid                       = errors.New("invalid")
	ErrConflict                      = errors.New("conflict")
	ErrExpired                       = errors.New("expired")
	ErrLastAdmin                     = errors.New("last admin")
	ErrGrantLocked                   = errors.New("emby grant locked")
	ErrTicketClosed                  = errors.New("ticket closed")
	ErrTicketUserOpenLimit           = errors.New("ticket user open limit reached")
	ErrTicketGlobalOpenLimit         = errors.New("ticket global open limit reached")
	ErrMediaRequestUserActiveLimit   = errors.New("media request user active limit reached")
	ErrMediaRequestGlobalActiveLimit = errors.New("media request global active limit reached")
	ErrInsufficientPoints            = errors.New("insufficient points")
)

func randomKey(prefix string, id, now int64) string {
	return prefix + "_" + strconv36(id) + "_" + strconv36(now)
}

func strconv36(v int64) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if v == 0 {
		return "0"
	}
	var out []byte
	for v > 0 {
		out = append([]byte{alphabet[v%36]}, out...)
		v /= 36
	}
	return string(out)
}
