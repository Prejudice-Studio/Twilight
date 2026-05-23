package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const legacyAdminBootstrapLimit = 20

func bootstrapLegacyAdminsIfNeeded(cfg config.Config, st *store.Store) {
	if st == nil || storeHasAdmin(st) {
		return
	}
	dbPath, ok := legacyUsersDBPath(cfg)
	if !ok {
		return
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		zap.L().Warn("legacy users.db found but sqlite3 command is unavailable; existing admins cannot be bootstrapped automatically", zap.String("path", dbPath))
		return
	}
	admins, err := readLegacySQLiteAdmins(sqliteBin, dbPath)
	if err != nil {
		zap.L().Warn("failed to read legacy users.db administrators", zap.String("path", dbPath), zap.Error(err))
		return
	}
	if len(admins) == 0 {
		return
	}
	imported := 0
	for _, admin := range admins {
		if strings.TrimSpace(admin.Username) == "" || strings.TrimSpace(admin.PasswordHash) == "" {
			continue
		}
		if existing, ok := st.FindUserByUsername(admin.Username); ok {
			if existing.Role == store.RoleAdmin && existing.Active {
				continue
			}
			if _, err := st.UpdateUser(existing.UID, func(u *store.User) error {
				applyLegacyAdmin(u, admin)
				return nil
			}); err != nil {
				zap.L().Warn("failed to promote existing user from legacy users.db", zap.String("username", admin.Username), zap.Error(err))
				continue
			}
			imported++
			continue
		}
		if _, err := st.CreateUser(admin); err != nil {
			zap.L().Warn("failed to bootstrap administrator from legacy users.db", zap.String("username", admin.Username), zap.Error(err))
			continue
		}
		imported++
	}
	if imported > 0 {
		zap.L().Warn("bootstrapped administrators from legacy users.db so existing admins can log in and migrate", zap.String("path", dbPath), zap.Int("count", imported))
	}
}

func legacyUsersDBPath(cfg config.Config) (string, bool) {
	databaseDir := strings.TrimSpace(cfg.DatabaseDir)
	if databaseDir == "" {
		databaseDir = "db"
	}
	dbPath := filepath.Clean(filepath.Join(databaseDir, "users.db"))
	info, err := os.Lstat(dbPath)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() == 0 {
		return "", false
	}
	return dbPath, true
}

func readLegacySQLiteAdmins(sqliteBin, dbPath string) ([]store.User, error) {
	query := `
SELECT
  UID,
  COALESCE(TELEGRAM_ID, 0) AS TELEGRAM_ID,
  COALESCE(USERNAME, '') AS USERNAME,
  COALESCE(EMAIL, '') AS EMAIL,
  COALESCE(ROLE, 1) AS ROLE,
  COALESCE(ACTIVE_STATUS, 0) AS ACTIVE_STATUS,
  COALESCE(CREATE_AT, 0) AS CREATE_AT,
  COALESCE(REGISTER_TIME, 0) AS REGISTER_TIME,
  COALESCE(EXPIRED_AT, -1) AS EXPIRED_AT,
  COALESCE(EMBYID, '') AS EMBYID,
  COALESCE(PASSWORD, '') AS PASSWORD
FROM users
WHERE ROLE = 0
  AND COALESCE(ACTIVE_STATUS, 0) = 1
  AND COALESCE(USERNAME, '') <> ''
  AND COALESCE(PASSWORD, '') <> ''
ORDER BY UID
LIMIT 20;
`
	ctx, cancel := contextWithTimeout()
	defer cancel()
	cmd := exec.CommandContext(ctx, sqliteBin, "-readonly", "-header", "-csv", dbPath, query)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return parseLegacyAdminCSV(out)
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func parseLegacyAdminCSV(data []byte) ([]store.User, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil
	}
	header := map[string]int{}
	for idx, name := range rows[0] {
		header[strings.ToUpper(strings.TrimSpace(name))] = idx
	}
	capacity := len(rows) - 1
	if capacity > legacyAdminBootstrapLimit {
		capacity = legacyAdminBootstrapLimit
	}
	users := make([]store.User, 0, capacity)
	for _, row := range rows[1:] {
		get := func(name string) string {
			idx, ok := header[name]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		if intFromString(get("ROLE"), store.RoleNormal) != store.RoleAdmin || !boolFromString(get("ACTIVE_STATUS")) {
			continue
		}
		user := store.User{
			Username:           get("USERNAME"),
			Email:              get("EMAIL"),
			TelegramID:         int64FromString(get("TELEGRAM_ID"), 0),
			Role:               store.RoleAdmin,
			Active:             true,
			ExpiredAt:          int64FromString(get("EXPIRED_AT"), -1),
			EmbyID:             get("EMBYID"),
			Avatar:             get("AVATAR"),
			BGMMode:            boolFromString(get("BGM_MODE")),
			BGMToken:           get("BGM_TOKEN"),
			CreatedAt:          int64FromString(get("CREATE_AT"), 0),
			RegisterTime:       int64FromString(get("REGISTER_TIME"), 0),
			PendingEmby:        boolFromString(get("PENDING_EMBY")),
			PasswordHash:       get("PASSWORD"),
			LegacyAPIKeyStatus: boolFromString(get("APIKEY_STATUS")),
			LegacyPermissions:  legacyPermissionList(get("APIKEY_PERMISSIONS")),
		}
		if user.CreatedAt == 0 {
			user.CreatedAt = user.RegisterTime
		}
		if user.RegisterTime == 0 {
			user.RegisterTime = user.CreatedAt
		}
		if days := intFromString(get("PENDING_EMBY_DAYS"), 0); days > 0 {
			user.PendingEmbyDays = &days
		}
		if key := get("APIKEY"); key != "" {
			user.LegacyAPIKeyHash = key
		}
		if background := legacyBackground(get("OTHER")); background != "" {
			user.Background = background
		}
		users = append(users, user)
		if len(users) >= legacyAdminBootstrapLimit {
			break
		}
	}
	return users, nil
}

func applyLegacyAdmin(target *store.User, source store.User) {
	target.Role = store.RoleAdmin
	target.Active = true
	target.PasswordHash = source.PasswordHash
	target.Email = firstNonEmpty(source.Email, target.Email)
	target.TelegramID = firstNonZeroInt64(source.TelegramID, target.TelegramID)
	target.EmbyID = firstNonEmpty(source.EmbyID, target.EmbyID)
	target.Avatar = firstNonEmpty(source.Avatar, target.Avatar)
	target.Background = firstNonEmpty(source.Background, target.Background)
	target.BGMMode = source.BGMMode || target.BGMMode
	target.BGMToken = firstNonEmpty(source.BGMToken, target.BGMToken)
	target.ExpiredAt = firstNonZeroInt64(source.ExpiredAt, target.ExpiredAt)
	target.LegacyAPIKeyHash = firstNonEmpty(source.LegacyAPIKeyHash, target.LegacyAPIKeyHash)
	target.LegacyAPIKeyStatus = source.LegacyAPIKeyStatus || target.LegacyAPIKeyStatus
	if len(source.LegacyPermissions) > 0 {
		target.LegacyPermissions = source.LegacyPermissions
	}
}

func legacyPermissionList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return cleanStringList(values)
	}
	raw = strings.Trim(raw, "[]")
	return cleanStringList(strings.Split(raw, ","))
}

func legacyBackground(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	background, ok := payload["background"]
	if !ok {
		return ""
	}
	data, err := json.Marshal(background)
	if err != nil {
		return ""
	}
	return string(data)
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(strings.Trim(value, `"'`))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func boolFromString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func intFromString(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func int64FromString(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
