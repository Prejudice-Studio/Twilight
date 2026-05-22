package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	mu      sync.RWMutex
	path    string
	backend string
	db      *sql.DB
	state   State
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
}

type State struct {
	NextUserID          int64                          `json:"next_user_id"`
	NextAPIKeyID        int64                          `json:"next_api_key_id"`
	NextRequestID       int64                          `json:"next_request_id"`
	NextAnnouncementID  int64                          `json:"next_announcement_id"`
	NextLoginLogID      int64                          `json:"next_login_log_id"`
	NextSchedulerRunID  int64                          `json:"next_scheduler_run_id"`
	NextRebindRequestID int64                          `json:"next_rebind_request_id"`
	Users               map[int64]User                 `json:"users"`
	APIKeys             map[int64]APIKey               `json:"api_keys"`
	MediaRequests       map[int64]MediaRequest         `json:"media_requests"`
	Announcements       map[int64]Announcement         `json:"announcements"`
	InviteCodes         map[string]InviteCode          `json:"invite_codes"`
	InviteRelations     map[int64]InviteRelation       `json:"invite_relations"`
	RegCodes            map[string]RegCode             `json:"regcodes"`
	BindCodes           map[string]BindCode            `json:"bind_codes"`
	Signin              map[int64]Signin               `json:"signin"`
	SchedulerRuns       []SchedulerRun                 `json:"scheduler_runs"`
	SchedulerSchedules  map[string]SchedulerSchedule   `json:"scheduler_schedules"`
	Devices             map[string]Device              `json:"devices"`
	LoginLogs           []LoginLog                     `json:"login_logs"`
	IPBlacklist         map[string]IPBlacklistEntry    `json:"ip_blacklist"`
	PlaybackRecords     []PlaybackRecord               `json:"playback_records"`
	RebindRequests      map[int64]RebindRequest        `json:"rebind_requests"`
	TelegramRoster      map[string]TelegramRosterEntry `json:"telegram_roster"`
}

type User struct {
	UID                int64    `json:"uid"`
	Username           string   `json:"username"`
	Email              string   `json:"email,omitempty"`
	TelegramID         int64    `json:"telegram_id,omitempty"`
	TelegramUsername   string   `json:"telegram_username,omitempty"`
	Role               int      `json:"role"`
	Active             bool     `json:"active"`
	ExpiredAt          int64    `json:"expired_at"`
	EmbyID             string   `json:"emby_id,omitempty"`
	EmbyUsername       string   `json:"emby_username,omitempty"`
	Avatar             string   `json:"avatar,omitempty"`
	Background         string   `json:"background,omitempty"`
	BGMMode            bool     `json:"bgm_mode"`
	BGMToken           string   `json:"bgm_token,omitempty"`
	CreatedAt          int64    `json:"created_at"`
	RegisterTime       int64    `json:"register_time"`
	PendingEmby        bool     `json:"pending_emby"`
	PendingEmbyDays    *int     `json:"pending_emby_days,omitempty"`
	LibrarySelfService bool     `json:"library_self_service"`
	LegacyAPIKeyHash   string   `json:"legacy_api_key_hash,omitempty"`
	LegacyAPIKeyPrefix string   `json:"legacy_api_key_prefix,omitempty"`
	LegacyAPIKeySuffix string   `json:"legacy_api_key_suffix,omitempty"`
	LegacyAPIKeyStatus bool     `json:"legacy_api_key_status"`
	LegacyPermissions  []string `json:"legacy_permissions,omitempty"`
	PasswordHash       string   `json:"password_hash"`
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
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Visible   bool   `json:"visible"`
	Level     string `json:"level"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	ExpiredAt int64  `json:"expired_at,omitempty"`
}

type InviteCode struct {
	Code          string `json:"code"`
	UID           int64  `json:"uid"`
	InviterUID    int64  `json:"inviter_uid"`
	Days          int    `json:"days"`
	UseCountLimit int    `json:"use_count_limit"`
	UseCount      int    `json:"use_count"`
	UsedByUID     int64  `json:"used_by_uid,omitempty"`
	UsedAt        int64  `json:"used_at,omitempty"`
	Active        bool   `json:"active"`
	Note          string `json:"note,omitempty"`
	Used          bool   `json:"used"`
	CreatedAt     int64  `json:"created_at"`
	ExpiredAt     int64  `json:"expired_at,omitempty"`
}

type RegCode struct {
	Code              string  `json:"code"`
	Type              int     `json:"type"`
	ValidityTime      int64   `json:"validity_time"`
	Days              int     `json:"days"`
	Note              string  `json:"note,omitempty"`
	UseCountLimit     int     `json:"use_count_limit"`
	UseCount          int     `json:"use_count"`
	UsedBy            int64   `json:"used_by,omitempty"`
	UsedByUIDs        []int64 `json:"used_by_uids,omitempty"`
	UsedByTelegramIDs []int64 `json:"used_by_telegram_ids,omitempty"`
	Active            bool    `json:"active"`
	IsDecoy           bool    `json:"is_decoy"`
	CreatedAt         int64   `json:"created_at"`
	CreatedTime       int64   `json:"created_time"`
	ExpiredAt         int64   `json:"expired_at,omitempty"`
}

type InviteRelation struct {
	ParentUID int64  `json:"parent_uid"`
	ChildUID  int64  `json:"child_uid"`
	Code      string `json:"code"`
	CreatedAt int64  `json:"created_at"`
}

type BindCode struct {
	Code       string `json:"code"`
	Scene      string `json:"scene"`
	UID        int64  `json:"uid,omitempty"`
	Confirmed  bool   `json:"confirmed"`
	TelegramID int64  `json:"telegram_id,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  int64  `json:"expires_at"`
}

type Signin struct {
	UID        int64          `json:"uid"`
	Points     int            `json:"points"`
	Streak     int            `json:"streak"`
	LastSignin string         `json:"last_signin"`
	Records    []SigninRecord `json:"records"`
}

type SigninRecord struct {
	Date      string `json:"date"`
	Points    int    `json:"points"`
	CreatedAt int64  `json:"created_at"`
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
	JobID       string         `json:"job_id"`
	TriggerSpec map[string]any `json:"trigger_spec"`
	IsCustom    bool           `json:"is_custom"`
	UpdatedAt   int64          `json:"updated_at"`
}

type Device struct {
	UID           int64  `json:"uid"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	Client        string `json:"client"`
	ClientVersion string `json:"client_version,omitempty"`
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
	UID       int64  `json:"uid"`
	ItemID    string `json:"item_id"`
	Title     string `json:"title"`
	MediaType string `json:"media_type"`
	Duration  int64  `json:"duration"`
	PlayedAt  int64  `json:"played_at"`
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

func Open(path string) (*Store, error) {
	if path == "" {
		path = filepath.Join("db", "twilight_go_state.json")
	}
	st := &Store{path: path, backend: BackendJSON, state: emptyState()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, st.saveLocked()
		}
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &st.state); err != nil {
			return nil, err
		}
	}
	st.state.ensure()
	return st, nil
}

func OpenPostgres(ctx context.Context, dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("postgres dsn is empty")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS twilight_state (
	id integer PRIMARY KEY,
	state jsonb NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		_ = db.Close()
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
	return st, nil
}

func CheckPostgres(ctx context.Context, dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return fmt.Errorf("postgres dsn is empty")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(30 * time.Second)
	return db.PingContext(ctx)
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
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
	if s.NextSchedulerRunID <= 0 {
		s.NextSchedulerRunID = 1
	}
	if s.NextRebindRequestID <= 0 {
		s.NextRebindRequestID = 1
	}
	if s.Users == nil {
		s.Users = map[int64]User{}
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
}

func (s *State) EnsureForMigration() {
	s.ensure()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	s.state.ensure()
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if s.db != nil {
		_, err = s.db.ExecContext(
			context.Background(),
			`INSERT INTO twilight_state (id, state, updated_at) VALUES (1, $1::jsonb, now())
			 ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`,
			string(data),
		)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Snapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.state
	state.ensure()
	return json.MarshalIndent(state, "", "  ")
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
	return s.saveLocked()
}

func (s *Store) Backup(dir string) (BackupInfo, error) {
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
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return BackupInfo{}, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return BackupInfo{}, err
	}
	if err := file.Close(); err != nil {
		return BackupInfo{}, err
	}
	return BackupInfo{Name: name, Path: path, Size: int64(len(data)), CreatedAt: now.Unix()}, nil
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
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
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
		})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt > backups[j].CreatedAt })
	return backups, nil
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
	if filepath.Base(name) != name || strings.Contains(name, "..") || !strings.HasSuffix(strings.ToLower(name), ".json") {
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
	for _, existing := range s.state.Users {
		if strings.EqualFold(existing.Username, u.Username) {
			return User{}, ErrConflict
		}
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
	return u, s.saveLocked()
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

func (s *Store) FindUserByEmbyID(embyID string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.state.Users {
		if embyID != "" && u.EmbyID == embyID {
			return u, true
		}
	}
	return User{}, false
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
	u, ok := s.state.Users[uid]
	if !ok {
		return User{}, ErrNotFound
	}
	if err := fn(&u); err != nil {
		return User{}, err
	}
	s.state.Users[uid] = u
	return u, s.saveLocked()
}

func (s *Store) DeleteUser(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Users[uid]; !ok {
		return ErrNotFound
	}
	delete(s.state.Users, uid)
	for id, key := range s.state.APIKeys {
		if key.UID == uid {
			delete(s.state.APIKeys, id)
		}
	}
	for code, invite := range s.state.InviteCodes {
		if invite.InviterUID == uid || invite.UID == uid {
			delete(s.state.InviteCodes, code)
		}
	}
	delete(s.state.InviteRelations, uid)
	for child, rel := range s.state.InviteRelations {
		if rel.ParentUID == uid {
			delete(s.state.InviteRelations, child)
		}
	}
	for id, req := range s.state.MediaRequests {
		if req.UID == uid {
			delete(s.state.MediaRequests, id)
		}
	}
	return s.saveLocked()
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

func (s *Store) FindUserByTelegramID(telegramID int64) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.state.Users {
		if telegramID != 0 && u.TelegramID == telegramID {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) CreateAPIKey(k APIKey) (APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return k, s.saveLocked()
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
	for _, k := range s.state.APIKeys {
		if k.Hash == hash && k.Enabled {
			u, ok := s.state.Users[k.UID]
			return k, u, ok
		}
	}
	for _, u := range s.state.Users {
		if u.LegacyAPIKeyHash == hash && u.LegacyAPIKeyStatus {
			return APIKey{UID: u.UID, Enabled: true, Permissions: u.LegacyPermissions, RateLimit: 100}, u, true
		}
	}
	return APIKey{}, User{}, false
}

func (s *Store) UpdateAPIKey(uid, id int64, fn func(*APIKey) error) (APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.state.APIKeys[id]
	if !ok || k.UID != uid {
		return APIKey{}, ErrNotFound
	}
	if err := fn(&k); err != nil {
		return APIKey{}, err
	}
	s.state.APIKeys[id] = k
	return k, s.saveLocked()
}

func (s *Store) RecordAPIKeyUse(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.state.APIKeys[id]
	if !ok {
		return ErrNotFound
	}
	k.RequestCount++
	k.LastUsed = time.Now().Unix()
	s.state.APIKeys[id] = k
	return s.saveLocked()
}

func (s *Store) DeleteAPIKey(uid, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.state.APIKeys[id]
	if !ok || k.UID != uid {
		return ErrNotFound
	}
	delete(s.state.APIKeys, id)
	return s.saveLocked()
}

func (s *Store) CreateMediaRequest(r MediaRequest) (MediaRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.state.MediaRequests {
		if strings.EqualFold(existing.Source, r.Source) && existing.MediaID == r.MediaID && existing.Season == r.Season && isActiveMediaStatus(existing.Status) {
			return existing, ErrConflict
		}
	}
	now := time.Now().Unix()
	r.ID = s.state.NextRequestID
	s.state.NextRequestID++
	if r.RequireKey == "" {
		r.RequireKey = randomKey("req", r.ID, now)
	}
	if r.Status == "" {
		r.Status = "UNHANDLED"
	}
	r.CreatedAt = now
	r.UpdatedAt = now
	s.state.MediaRequests[r.ID] = r
	return r, s.saveLocked()
}

func (s *Store) ActiveMediaRequestCount(uid int64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, r := range s.state.MediaRequests {
		if r.UID == uid && isActiveMediaStatus(r.Status) {
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
	r, ok := s.state.MediaRequests[id]
	if !ok {
		return MediaRequest{}, ErrNotFound
	}
	if err := fn(&r); err != nil {
		return MediaRequest{}, err
	}
	r.UpdatedAt = time.Now().Unix()
	s.state.MediaRequests[id] = r
	return r, s.saveLocked()
}

func (s *Store) DeleteMediaRequest(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.MediaRequests[id]; !ok {
		return ErrNotFound
	}
	delete(s.state.MediaRequests, id)
	return s.saveLocked()
}

func (s *Store) UpsertBindCode(code BindCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.BindCodes[code.Code] = code
	return s.saveLocked()
}

func (s *Store) BindCode(code string) (BindCode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.state.BindCodes[code]
	return b, ok
}

func (s *Store) UpsertAnnouncement(a Announcement) (Announcement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	if a.ID == 0 {
		a.ID = s.state.NextAnnouncementID
		s.state.NextAnnouncementID++
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if a.Level == "" {
		a.Level = "info"
	}
	s.state.Announcements[a.ID] = a
	return a, s.saveLocked()
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
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) DeleteAnnouncement(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Announcements[id]; !ok {
		return ErrNotFound
	}
	delete(s.state.Announcements, id)
	return s.saveLocked()
}

func (s *Store) UpsertInviteCode(code InviteCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return s.saveLocked()
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

func (s *Store) DeleteInviteCode(uid int64, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return s.saveLocked()
}

func (s *Store) ConsumeInviteCode(code string, childUID int64) (InviteCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.state.InviteCodes[code]
	if !ok || !c.Active {
		return InviteCode{}, ErrNotFound
	}
	if c.UseCountLimit != -1 && c.UseCount >= c.UseCountLimit {
		return InviteCode{}, ErrConflict
	}
	now := time.Now().Unix()
	c.UseCount++
	c.Used = true
	c.UsedByUID = childUID
	c.UsedAt = now
	if c.UseCountLimit != -1 && c.UseCount >= c.UseCountLimit {
		c.Active = false
	}
	s.state.InviteCodes[code] = c
	s.state.InviteRelations[childUID] = InviteRelation{ParentUID: c.InviterUID, ChildUID: childUID, Code: code, CreatedAt: now}
	return c, s.saveLocked()
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
	rel, ok := s.state.InviteRelations[uid]
	return rel, ok
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

func (s *Store) DetachInvite(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.InviteRelations, uid)
	return s.saveLocked()
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
	if !code.Active && code.UseCount == 0 {
		code.Active = true
	}
	s.state.RegCodes[code.Code] = code
	return s.saveLocked()
}

func (s *Store) ConsumeRegCode(code string, uid, telegramID int64) (RegCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.state.RegCodes[code]
	if !ok || !r.Active {
		return RegCode{}, ErrNotFound
	}
	if r.UseCountLimit != -1 && r.UseCount >= r.UseCountLimit {
		return RegCode{}, ErrConflict
	}
	now := time.Now().Unix()
	if r.ValidityTime > 0 && r.CreatedAt+r.ValidityTime*3600 < now {
		return RegCode{}, ErrExpired
	}
	r.UseCount++
	r.UsedBy = uid
	r.UsedByUIDs = appendUniqueInt64(r.UsedByUIDs, uid)
	if telegramID != 0 {
		r.UsedByTelegramIDs = appendUniqueInt64(r.UsedByTelegramIDs, telegramID)
	}
	if r.UseCountLimit != -1 && r.UseCount >= r.UseCountLimit {
		r.Active = false
	}
	s.state.RegCodes[code] = r
	return r, s.saveLocked()
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
	if _, ok := s.state.RegCodes[code]; !ok {
		return ErrNotFound
	}
	delete(s.state.RegCodes, code)
	return s.saveLocked()
}

func (s *Store) DeleteRegCodes(codes []string) (deleted []string, missing []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if len(deleted) == 0 {
		return deleted, missing, nil
	}
	return deleted, missing, s.saveLocked()
}

func (s *Store) Signin(uid int64) Signin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Signin[uid]
}

func (s *Store) AddSignin(uid int64, points int) (Signin, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := time.Now().Format("2006-01-02")
	si := s.state.Signin[uid]
	if si.UID == 0 {
		si.UID = uid
	}
	if si.LastSignin == today {
		return si, false, nil
	}
	si.LastSignin = today
	si.Streak++
	si.Points += points
	si.Records = append(si.Records, SigninRecord{Date: today, Points: points, CreatedAt: time.Now().Unix()})
	s.state.Signin[uid] = si
	return si, true, s.saveLocked()
}

func (s *Store) AddSchedulerRun(run SchedulerRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.ID == 0 {
		run.ID = s.state.NextSchedulerRunID
		s.state.NextSchedulerRunID++
	}
	if run.Type == "" {
		run.Type = "manual"
	}
	if run.Trigger == "" {
		run.Trigger = "manual"
	}
	if run.FinishedAt == 0 && run.EndedAt != 0 {
		run.FinishedAt = run.EndedAt
	}
	s.state.SchedulerRuns = append([]SchedulerRun{run}, s.state.SchedulerRuns...)
	if len(s.state.SchedulerRuns) > 200 {
		s.state.SchedulerRuns = s.state.SchedulerRuns[:200]
	}
	return s.saveLocked()
}

func (s *Store) SchedulerRuns(jobID string, limit int) []SchedulerRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	out := make([]SchedulerRun, 0, limit)
	for _, run := range s.state.SchedulerRuns {
		if jobID == "" || run.JobID == jobID {
			out = append(out, run)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func (s *Store) SetSchedulerSchedule(jobID string, spec map[string]any, custom bool) (SchedulerSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	schedule := SchedulerSchedule{JobID: jobID, TriggerSpec: spec, IsCustom: custom, UpdatedAt: time.Now().Unix()}
	if !custom {
		delete(s.state.SchedulerSchedules, jobID)
		return schedule, s.saveLocked()
	}
	s.state.SchedulerSchedules[jobID] = schedule
	return schedule, s.saveLocked()
}

func (s *Store) SchedulerSchedule(jobID string) (SchedulerSchedule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	schedule, ok := s.state.SchedulerSchedules[jobID]
	return schedule, ok
}

func (s *Store) UpsertDevice(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.FirstSeen == 0 {
		d.FirstSeen = time.Now().Unix()
	}
	if d.LastSeen == 0 {
		d.LastSeen = d.FirstSeen
	}
	s.state.Devices[deviceKey(d.UID, d.DeviceID)] = d
	return s.saveLocked()
}

func (s *Store) ListDevices(uid int64) []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0)
	for _, d := range s.state.Devices {
		if d.UID == uid && !d.Blocked {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

func (s *Store) UpdateDevice(uid int64, deviceID string, fn func(*Device)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deviceKey(uid, deviceID)
	d, ok := s.state.Devices[key]
	if !ok {
		d = Device{UID: uid, DeviceID: deviceID, DeviceName: deviceID, FirstSeen: time.Now().Unix(), LastSeen: time.Now().Unix()}
	}
	fn(&d)
	s.state.Devices[key] = d
	return s.saveLocked()
}

func (s *Store) DeleteDevice(uid int64, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Devices, deviceKey(uid, deviceID))
	return s.saveLocked()
}

func (s *Store) AddLoginLog(log LoginLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log.ID == 0 {
		log.ID = s.state.NextLoginLogID
		s.state.NextLoginLogID++
	}
	if log.Time == 0 {
		log.Time = time.Now().Unix()
	}
	s.state.LoginLogs = append([]LoginLog{log}, s.state.LoginLogs...)
	if len(s.state.LoginLogs) > 1000 {
		s.state.LoginLogs = s.state.LoginLogs[:1000]
	}
	return s.saveLocked()
}

func (s *Store) LoginHistory(uid int64, blockedOnly bool, since int64, limit int) []LoginLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	out := make([]LoginLog, 0, limit)
	for _, log := range s.state.LoginLogs {
		if uid != 0 && log.UID != uid {
			continue
		}
		if blockedOnly && !log.Blocked {
			continue
		}
		if since > 0 && log.Time < since {
			continue
		}
		out = append(out, log)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) AddPlaybackRecord(record PlaybackRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.PlayedAt == 0 {
		record.PlayedAt = time.Now().Unix()
	}
	s.state.PlaybackRecords = append([]PlaybackRecord{record}, s.state.PlaybackRecords...)
	if len(s.state.PlaybackRecords) > 10000 {
		s.state.PlaybackRecords = s.state.PlaybackRecords[:10000]
	}
	return s.saveLocked()
}

func (s *Store) PlaybackRecords(uid int64, since int64, limit int) []PlaybackRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	out := make([]PlaybackRecord, 0, minInt(limit, len(s.state.PlaybackRecords)))
	for _, record := range s.state.PlaybackRecords {
		if uid != 0 && record.UID != uid {
			continue
		}
		if since > 0 && record.PlayedAt < since {
			continue
		}
		out = append(out, record)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) AddIPBlacklist(ip, reason string, expireAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.IPBlacklist[ip] = IPBlacklistEntry{IP: ip, Reason: reason, CreatedAt: time.Now().Unix(), ExpireAt: expireAt}
	return s.saveLocked()
}

func (s *Store) RemoveIPBlacklist(ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.IPBlacklist, ip)
	return s.saveLocked()
}

func (s *Store) ListIPBlacklist() []IPBlacklistEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]IPBlacklistEntry, 0, len(s.state.IPBlacklist))
	for _, entry := range s.state.IPBlacklist {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *Store) IsIPBlacklisted(ip string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.IPBlacklist[ip]
	if !ok {
		return false
	}
	return entry.ExpireAt == -1 || entry.ExpireAt > time.Now().Unix()
}

func (s *Store) CreateRebindRequest(req RebindRequest) (RebindRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.state.RebindRequests {
		if existing.UID == req.UID && existing.Status == "pending" {
			return existing, ErrConflict
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
	return req, s.saveLocked()
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
	req, ok := s.state.RebindRequests[id]
	if !ok {
		return RebindRequest{}, ErrNotFound
	}
	req.Status = status
	req.AdminNote = note
	req.ReviewerUID = reviewerUID
	req.ReviewedAt = time.Now().Unix()
	s.state.RebindRequests[id] = req
	return req, s.saveLocked()
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
	return s.saveLocked()
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
	key := telegramRosterKey(chatID, telegramID)
	entry, ok := s.state.TelegramRoster[key]
	if !ok {
		entry = TelegramRosterEntry{ChatID: chatID, TelegramID: telegramID, FirstSeen: time.Now().Unix()}
	}
	entry.LastSeen = time.Now().Unix()
	entry.LastStatus = status
	s.state.TelegramRoster[key] = entry
	return s.saveLocked()
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

func isActiveMediaStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "UNHANDLED", "PENDING", "ACCEPTED", "DOWNLOADING":
		return true
	default:
		return false
	}
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func deviceKey(uid int64, deviceID string) string {
	return strconv36(uid) + ":" + deviceID
}

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrExpired  = errors.New("expired")
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
