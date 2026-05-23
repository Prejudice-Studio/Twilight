package store

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LegacySQLiteImportResult struct {
	Detected      bool                  `json:"detected"`
	Imported      bool                  `json:"imported"`
	Source        LegacySQLiteReport    `json:"source"`
	Counts        map[string]int        `json:"counts"`
	TableCounts   map[string]int        `json:"table_counts,omitempty"`
	MappedTables  []string              `json:"mapped_tables,omitempty"`
	SkippedTables []string              `json:"skipped_tables,omitempty"`
	Mappings      []LegacySQLiteMapping `json:"mappings,omitempty"`
	Warnings      []string              `json:"warnings,omitempty"`
	SnapshotBytes int                   `json:"snapshot_bytes,omitempty"`
}

type LegacySQLiteMapping struct {
	SourceDatabase string                     `json:"source_database"`
	SourceTable    string                     `json:"source_table"`
	SourceKey      string                     `json:"source_key"`
	Target         string                     `json:"target"`
	Fields         []LegacySQLiteFieldMapping `json:"fields,omitempty"`
	Rows           int                        `json:"rows"`
	Mapped         bool                       `json:"mapped"`
}

type LegacySQLiteFieldMapping struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Transform string `json:"transform,omitempty"`
}

type legacyRow map[string]string

var legacyUploadAssetFilenamePattern = regexp.MustCompile(`^[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$`)

var legacySQLiteMappingCatalog = []LegacySQLiteMapping{
	{
		SourceDatabase: "users", SourceTable: "users", SourceKey: "users.users", Target: "users",
		Fields: []LegacySQLiteFieldMapping{
			{Source: "UID", Target: "users.uid"},
			{Source: "USERNAME", Target: "users.username"},
			{Source: "EMAIL", Target: "users.email"},
			{Source: "TELEGRAM_ID", Target: "users.telegram_id"},
			{Source: "ROLE", Target: "users.role"},
			{Source: "ACTIVE_STATUS", Target: "users.active", Transform: "legacy boolean"},
			{Source: "EXPIRED_AT", Target: "users.expired_at"},
			{Source: "EMBYID", Target: "users.emby_id"},
			{Source: "PASSWORD", Target: "users.password_hash"},
			{Source: "APIKEY/APIKEY_STATUS/APIKEY_PERMISSIONS", Target: "users.legacy_api_key_*"},
			{Source: "AVATAR", Target: "users.avatar", Transform: "safe uploaded asset path"},
			{Source: "OTHER.background", Target: "users.background", Transform: "sanitized background JSON"},
			{Source: "PENDING_EMBY/PENDING_EMBY_DAYS", Target: "users.pending_emby_*"},
		},
	},
	{
		SourceDatabase: "api_keys", SourceTable: "api_keys", SourceKey: "api_keys.api_keys", Target: "api_keys",
		Fields: []LegacySQLiteFieldMapping{
			{Source: "ID", Target: "api_keys.id"},
			{Source: "UID", Target: "api_keys.uid"},
			{Source: "KEY_HASH", Target: "api_keys.hash"},
			{Source: "KEY_PREFIX/KEY_SUFFIX", Target: "api_keys.key_prefix/key_suffix"},
			{Source: "PERMISSIONS", Target: "api_keys.permissions", Transform: "JSON/list normalization"},
		},
	},
	{
		SourceDatabase: "regcode", SourceTable: "regcode", SourceKey: "regcode.regcode", Target: "regcodes",
		Fields: []LegacySQLiteFieldMapping{
			{Source: "CODE", Target: "regcodes.code"},
			{Source: "TYPE/DAYS/VALIDITY_TIME", Target: "regcodes.type/days/validity_time"},
			{Source: "UID/TELEGRAM_ID", Target: "regcodes.used_by_*", Transform: "single value or list"},
			{Source: "OTHER.note/decoy", Target: "regcodes.note/is_decoy", Transform: "metadata JSON"},
		},
	},
	{
		SourceDatabase: "invites", SourceTable: "invite_codes", SourceKey: "invites.invite_codes", Target: "invite_codes",
		Fields: []LegacySQLiteFieldMapping{
			{Source: "CODE", Target: "invite_codes.code"},
			{Source: "INVITER_UID", Target: "invite_codes.inviter_uid"},
			{Source: "DAYS/USE_COUNT_LIMIT/USE_COUNT", Target: "invite_codes.days/use_count_*"},
			{Source: "USED_BY_UID/USED_AT", Target: "invite_codes.used_by_uid/used_at"},
		},
	},
	{
		SourceDatabase: "invites", SourceTable: "invite_relations", SourceKey: "invites.invite_relations", Target: "invite_relations",
		Fields: []LegacySQLiteFieldMapping{{Source: "CHILD_UID/PARENT_UID/CODE", Target: "invite_relations.child_uid/parent_uid/code"}},
	},
	{
		SourceDatabase: "announcements", SourceTable: "announcements", SourceKey: "announcements.announcements", Target: "announcements",
		Fields: []LegacySQLiteFieldMapping{{Source: "ID/TITLE/CONTENT/VISIBLE/LEVEL", Target: "announcements.*"}},
	},
	{
		SourceDatabase: "login_log", SourceTable: "login_log", SourceKey: "login_log.login_log", Target: "login_logs",
		Fields: []LegacySQLiteFieldMapping{{Source: "ID/UID/IP_ADDRESS/DEVICE_ID/LOGIN_TIME", Target: "login_logs.*"}},
	},
	{
		SourceDatabase: "login_log", SourceTable: "user_device", SourceKey: "login_log.user_device", Target: "devices",
		Fields: []LegacySQLiteFieldMapping{{Source: "UID/DEVICE_ID/DEVICE_NAME/CLIENT", Target: "devices.*"}},
	},
	{
		SourceDatabase: "login_log", SourceTable: "ip_blacklist", SourceKey: "login_log.ip_blacklist", Target: "ip_blacklist",
		Fields: []LegacySQLiteFieldMapping{{Source: "IP_ADDRESS/REASON/EXPIRE_AT", Target: "ip_blacklist.*"}},
	},
	{
		SourceDatabase: "playback", SourceTable: "playback", SourceKey: "playback.playback", Target: "playback_records",
		Fields: []LegacySQLiteFieldMapping{{Source: "UID/ITEM_ID/ITEM_NAME/ITEM_TYPE/DURATION/END_TIME", Target: "playback_records.*"}},
	},
	{
		SourceDatabase: "signin", SourceTable: "user_points", SourceKey: "signin.user_points", Target: "signin",
		Fields: []LegacySQLiteFieldMapping{{Source: "UID/CURRENT_POINTS/CURRENT_STREAK/LAST_SIGNIN_DATE", Target: "signin.*"}},
	},
	{
		SourceDatabase: "signin", SourceTable: "signin_record", SourceKey: "signin.signin_record", Target: "signin.records",
		Fields: []LegacySQLiteFieldMapping{{Source: "UID/SIGNIN_DATE/DAILY_POINTS/BONUS_POINTS", Target: "signin.records[]"}},
	},
	{
		SourceDatabase: "score", SourceTable: "scores", SourceKey: "score.scores", Target: "signin",
		Fields: []LegacySQLiteFieldMapping{{Source: "UID/SCORE/CHECKIN_COUNT/CHECKIN_TIME", Target: "signin.*", Transform: "fallback points/streak"}},
	},
	{
		SourceDatabase: "scheduler_run", SourceTable: "scheduler_run", SourceKey: "scheduler_run.scheduler_run", Target: "scheduler_runs",
		Fields: []LegacySQLiteFieldMapping{{Source: "ID/JOB_ID/STATUS/SUMMARY/LOGS/STARTED_AT", Target: "scheduler_runs.*"}},
	},
	{
		SourceDatabase: "scheduler_schedule", SourceTable: "scheduler_schedule", SourceKey: "scheduler_schedule.scheduler_schedule", Target: "scheduler_schedules",
		Fields: []LegacySQLiteFieldMapping{{Source: "JOB_ID/TRIGGER_*", Target: "scheduler_schedules.trigger_spec", Transform: "legacy trigger normalization"}},
	},
	{
		SourceDatabase: "telegram_roster", SourceTable: "telegram_group_roster", SourceKey: "telegram_roster.telegram_group_roster", Target: "telegram_roster",
		Fields: []LegacySQLiteFieldMapping{{Source: "CHAT_ID/TELEGRAM_ID/LAST_STATUS/FIRST_SEEN_AT", Target: "telegram_roster.*"}},
	},
	{
		SourceDatabase: "bangumi", SourceTable: "user", SourceKey: "bangumi.user", Target: "users.bgm_*",
		Fields: []LegacySQLiteFieldMapping{{Source: "TELEGRAM_ID/ACCESS_TOKEN/AUTO_UPDATE", Target: "users.bgm_token/bgm_mode", Transform: "matched by telegram_id"}},
	},
	{
		SourceDatabase: "bangumi", SourceTable: "require_bangumi", SourceKey: "bangumi.require_bangumi", Target: "media_requests",
		Fields: []LegacySQLiteFieldMapping{{Source: "ID/TELEGRAM_ID/BANGUMI_ID/TITLE/STATUS", Target: "media_requests.*", Transform: "source=bangumi"}},
	},
	{
		SourceDatabase: "bangumi", SourceTable: "require_tmdb", SourceKey: "bangumi.require_tmdb", Target: "media_requests",
		Fields: []LegacySQLiteFieldMapping{{Source: "ID/TELEGRAM_ID/TMDB_ID/TITLE/STATUS", Target: "media_requests.*", Transform: "source=tmdb"}},
	},
}

func BuildLegacySQLiteSnapshot(ctx context.Context, databaseDir string) ([]byte, LegacySQLiteImportResult, error) {
	report := InspectLegacySQLite(ctx, databaseDir)
	result := LegacySQLiteImportResult{
		Detected:    report.Detected,
		Source:      report,
		TableCounts: report.TableCounts,
		Warnings:    append([]string{}, report.Warnings...),
	}
	if !report.Detected {
		result.Counts = databaseStateCountsStore(emptyState())
		return nil, result, ErrNotFound
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, result, fmt.Errorf("sqlite3 command is unavailable: %w", err)
	}

	files := legacySQLiteFileMap(report.Files)
	state := emptyState()
	mapped := map[string]bool{}
	addWarning := func(format string, args ...any) {
		result.Warnings = append(result.Warnings, fmt.Sprintf(format, args...))
	}

	if path := files["users.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM users ORDER BY UID;`)
		if err != nil {
			addWarning("users.db.users: %v", err)
		} else {
			mapped["users.users"] = true
			importLegacyUsers(&state, rows, addWarning)
		}
	}

	if path := files["api_keys.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM api_keys ORDER BY ID;`)
		if err != nil {
			addWarning("api_keys.db.api_keys: %v", err)
		} else {
			mapped["api_keys.api_keys"] = true
			importLegacyAPIKeys(&state, rows, addWarning)
		}
	}

	if path := files["regcode.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM regcode ORDER BY CREATED_TIME, CODE;`)
		if err != nil {
			addWarning("regcode.db.regcode: %v", err)
		} else {
			mapped["regcode.regcode"] = true
			importLegacyRegCodes(&state, rows, addWarning)
		}
	}

	if path := files["invites.db"]; path != "" {
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM invite_codes ORDER BY CREATED_AT, CODE;`); err != nil {
			addWarning("invites.db.invite_codes: %v", err)
		} else {
			mapped["invites.invite_codes"] = true
			importLegacyInviteCodes(&state, rows, addWarning)
		}
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM invite_relations ORDER BY CREATED_AT, CHILD_UID;`); err != nil {
			addWarning("invites.db.invite_relations: %v", err)
		} else {
			mapped["invites.invite_relations"] = true
			importLegacyInviteRelations(&state, rows, addWarning)
		}
	}

	if path := files["announcements.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM announcements ORDER BY ID;`)
		if err != nil {
			addWarning("announcements.db.announcements: %v", err)
		} else {
			mapped["announcements.announcements"] = true
			importLegacyAnnouncements(&state, rows, addWarning)
		}
	}

	if path := files["login_log.db"]; path != "" {
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM login_log ORDER BY ID;`); err != nil {
			addWarning("login_log.db.login_log: %v", err)
		} else {
			mapped["login_log.login_log"] = true
			importLegacyLoginLogs(&state, rows, addWarning)
		}
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM user_device ORDER BY ID;`); err != nil {
			addWarning("login_log.db.user_device: %v", err)
		} else {
			mapped["login_log.user_device"] = true
			importLegacyDevices(&state, rows, addWarning)
		}
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM ip_blacklist ORDER BY ID;`); err != nil {
			addWarning("login_log.db.ip_blacklist: %v", err)
		} else {
			mapped["login_log.ip_blacklist"] = true
			importLegacyIPBlacklist(&state, rows, addWarning)
		}
	}

	if path := files["playback.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM playback ORDER BY ID;`)
		if err != nil {
			addWarning("playback.db.playback: %v", err)
		} else {
			mapped["playback.playback"] = true
			importLegacyPlayback(&state, rows, addWarning)
		}
	}

	if path := files["signin.db"]; path != "" {
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM user_points ORDER BY UID;`); err != nil {
			addWarning("signin.db.user_points: %v", err)
		} else {
			mapped["signin.user_points"] = true
			importLegacySigninPoints(&state, rows, addWarning)
		}
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM signin_record ORDER BY UID, SIGNIN_DATE, ID;`); err != nil {
			addWarning("signin.db.signin_record: %v", err)
		} else {
			mapped["signin.signin_record"] = true
			importLegacySigninRecords(&state, rows, addWarning)
		}
	}

	if path := files["score.db"]; path != "" {
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM scores ORDER BY UID;`); err != nil {
			addWarning("score.db.scores: %v", err)
		} else {
			mapped["score.scores"] = true
			importLegacyScores(&state, rows, addWarning)
		}
	}

	if path := files["scheduler_run.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM scheduler_run ORDER BY ID;`)
		if err != nil {
			addWarning("scheduler_run.db.scheduler_run: %v", err)
		} else {
			mapped["scheduler_run.scheduler_run"] = true
			importLegacySchedulerRuns(&state, rows, addWarning)
		}
	}

	if path := files["scheduler_schedule.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM scheduler_schedule ORDER BY JOB_ID;`)
		if err != nil {
			addWarning("scheduler_schedule.db.scheduler_schedule: %v", err)
		} else {
			mapped["scheduler_schedule.scheduler_schedule"] = true
			importLegacySchedulerSchedules(&state, rows, addWarning)
		}
	}

	if path := files["telegram_roster.db"]; path != "" {
		rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM telegram_group_roster ORDER BY CHAT_ID, TELEGRAM_ID;`)
		if err != nil {
			addWarning("telegram_roster.db.telegram_group_roster: %v", err)
		} else {
			mapped["telegram_roster.telegram_group_roster"] = true
			importLegacyTelegramRoster(&state, rows, addWarning)
		}
	}

	if path := files["bangumi.db"]; path != "" {
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM user ORDER BY telegram_id;`); err != nil {
			addWarning("bangumi.db.user: %v", err)
		} else {
			mapped["bangumi.user"] = true
			importLegacyBangumiUsers(&state, rows, addWarning)
		}
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM require_bangumi ORDER BY id;`); err != nil {
			addWarning("bangumi.db.require_bangumi: %v", err)
		} else {
			mapped["bangumi.require_bangumi"] = true
			importLegacyMediaRequests(&state, rows, "bangumi", addWarning)
		}
		if rows, err := legacySQLiteRows(ctx, sqliteBin, path, `SELECT * FROM require_tmdb ORDER BY id;`); err != nil {
			addWarning("bangumi.db.require_tmdb: %v", err)
		} else {
			mapped["bangumi.require_tmdb"] = true
			importLegacyMediaRequests(&state, rows, "tmdb", addWarning)
		}
	}

	state.ensure()
	result.MappedTables = sortedMapKeys(mapped)
	result.SkippedTables = legacySkippedTables(report.TableCounts, mapped)
	result.Mappings = legacySQLiteMappingDetails(report.TableCounts, mapped)
	result.Counts = databaseStateCountsStore(state)
	data, err := json.Marshal(state)
	if err != nil {
		return nil, result, err
	}
	result.SnapshotBytes = len(data)
	return data, result, nil
}

func legacySQLiteFileMap(files []LegacySQLiteFile) map[string]string {
	out := map[string]string{}
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file.Name), ".db") {
			out[strings.ToLower(file.Name)] = file.Path
		}
	}
	return out
}

func legacySQLiteRows(ctx context.Context, sqliteBin, dbPath, query string) ([]legacyRow, error) {
	cmd := exec.CommandContext(ctx, sqliteBin, "-readonly", "-header", "-csv", dbPath, query)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	reader := csv.NewReader(stdout)
	reader.FieldsPerRecord = -1
	headerRow, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			if waitErr := cmd.Wait(); waitErr != nil {
				msg := strings.TrimSpace(stderr.String())
				if msg == "" {
					msg = waitErr.Error()
				}
				return nil, errors.New(msg)
			}
			return nil, nil
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	header := make([]string, len(headerRow))
	for i, name := range headerRow {
		header[i] = strings.ToUpper(strings.TrimSpace(name))
	}
	outRows := make([]legacyRow, 0)
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, err
		}
		item := legacyRow{}
		for i, name := range header {
			if i < len(row) {
				item[name] = strings.TrimSpace(row[i])
			} else {
				item[name] = ""
			}
		}
		outRows = append(outRows, item)
	}
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return outRows, nil
}

func importLegacyUsers(state *State, rows []legacyRow, warn func(string, ...any)) {
	for _, row := range rows {
		uid := legacyInt64(row.get("UID"), 0)
		username := row.get("USERNAME")
		password := row.get("PASSWORD")
		if uid <= 0 || username == "" {
			warn("跳过旧 users.db 中 UID 或用户名为空的用户")
			continue
		}
		days := legacyInt(row.get("PENDING_EMBY_DAYS"), 0)
		user := User{
			UID:                uid,
			Username:           username,
			Email:              row.get("EMAIL"),
			TelegramID:         legacyInt64(row.get("TELEGRAM_ID"), 0),
			Role:               legacyInt(row.get("ROLE"), RoleNormal),
			Active:             legacyBool(row.get("ACTIVE_STATUS")),
			ExpiredAt:          legacyInt64(row.get("EXPIRED_AT"), -1),
			EmbyID:             row.get("EMBYID"),
			Avatar:             legacySafeUploadAsset(row.get("AVATAR"), "avatar"),
			Background:         legacySafeBackground(row.get("OTHER")),
			BGMMode:            legacyBool(row.get("BGM_MODE")),
			BGMToken:           row.get("BGM_TOKEN"),
			CreatedAt:          legacyInt64(row.get("CREATE_AT"), 0),
			RegisterTime:       legacyInt64(row.get("REGISTER_TIME"), 0),
			PendingEmby:        legacyBool(row.get("PENDING_EMBY")),
			LegacyAPIKeyHash:   row.get("APIKEY"),
			LegacyAPIKeyStatus: legacyBool(row.get("APIKEY_STATUS")),
			LegacyPermissions:  legacyPermissionList(row.get("APIKEY_PERMISSIONS")),
			PasswordHash:       password,
		}
		if user.EmbyID != "" {
			user.EmbyUsername = user.Username
		}
		if user.CreatedAt == 0 {
			user.CreatedAt = user.RegisterTime
		}
		if user.RegisterTime == 0 {
			user.RegisterTime = user.CreatedAt
		}
		if user.ExpiredAt == 0 {
			user.ExpiredAt = -1
		}
		if days > 0 {
			user.PendingEmbyDays = &days
		}
		state.Users[uid] = user
		if uid >= state.NextUserID {
			state.NextUserID = uid + 1
		}
		if password == "" {
			warn("用户 %s 没有密码哈希，迁移后无法密码登录", username)
		}
	}
}

func importLegacyAPIKeys(state *State, rows []legacyRow, warn func(string, ...any)) {
	for _, row := range rows {
		id := legacyInt64(row.get("ID"), 0)
		uid := legacyInt64(row.get("UID"), 0)
		hash := row.get("KEY_HASH")
		if id <= 0 || uid <= 0 || hash == "" {
			warn("跳过旧 api_keys.db 中 ID/UID/hash 不完整的 API Key")
			continue
		}
		state.APIKeys[id] = APIKey{
			ID:           id,
			UID:          uid,
			Name:         firstNonEmptyStore(row.get("NAME"), "Legacy API Key"),
			Hash:         hash,
			Prefix:       row.get("KEY_PREFIX"),
			Suffix:       row.get("KEY_SUFFIX"),
			Enabled:      legacyBool(row.get("ENABLED")),
			AllowQuery:   legacyBool(row.get("ALLOW_QUERY")),
			Permissions:  legacyPermissionList(row.get("PERMISSIONS")),
			RateLimit:    legacyInt(row.get("RATE_LIMIT"), 100),
			RequestCount: legacyInt64(row.get("REQUEST_COUNT"), 0),
			LastUsed:     legacyInt64(row.get("LAST_USED_AT"), 0),
			CreatedAt:    legacyInt64(row.get("CREATED_AT"), 0),
			ExpiredAt:    legacyInt64(row.get("EXPIRED_AT"), 0),
		}
		if id >= state.NextAPIKeyID {
			state.NextAPIKeyID = id + 1
		}
	}
}

func importLegacyRegCodes(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		code := strings.ToUpper(row.get("CODE"))
		if code == "" {
			continue
		}
		meta := legacyMetadata(row.get("OTHER"))
		reg := RegCode{
			Code:              code,
			Type:              legacyInt(row.get("TYPE"), 1),
			ValidityTime:      legacyInt64(row.get("VALIDITY_TIME"), 0),
			Days:              legacyInt(row.get("DAYS"), 0),
			UseCountLimit:     legacyInt(row.get("USE_COUNT_LIMIT"), 1),
			UseCount:          legacyInt(row.get("USE_COUNT"), 0),
			UsedByUIDs:        legacyInt64List(row.get("UID")),
			UsedByTelegramIDs: legacyInt64List(row.get("TELEGRAM_ID")),
			Active:            legacyBool(row.get("ACTIVE")),
			CreatedAt:         legacyInt64(row.get("CREATED_TIME"), 0),
			CreatedTime:       legacyInt64(row.get("CREATED_TIME"), 0),
			Note:              legacyStringMeta(meta, "note"),
			IsDecoy:           legacyBool(fmt.Sprint(meta["decoy"])),
		}
		if len(reg.UsedByUIDs) > 0 {
			reg.UsedBy = reg.UsedByUIDs[len(reg.UsedByUIDs)-1]
		}
		state.RegCodes[code] = reg
	}
}

func importLegacyInviteCodes(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		code := row.get("CODE")
		if code == "" {
			continue
		}
		limit := legacyInt(row.get("USE_COUNT_LIMIT"), 1)
		used := legacyInt(row.get("USE_COUNT"), 0)
		invite := InviteCode{
			Code:          code,
			UID:           legacyInt64(row.get("INVITER_UID"), 0),
			InviterUID:    legacyInt64(row.get("INVITER_UID"), 0),
			Days:          legacyInt(row.get("DAYS"), 0),
			UseCountLimit: limit,
			UseCount:      used,
			UsedByUID:     legacyInt64(row.get("USED_BY_UID"), 0),
			UsedAt:        legacyInt64(row.get("USED_AT"), 0),
			Active:        legacyBool(row.get("ACTIVE")),
			Note:          row.get("NOTE"),
			CreatedAt:     legacyInt64(row.get("CREATED_AT"), 0),
			ExpiredAt:     legacyInt64(row.get("EXPIRES_AT"), 0),
		}
		invite.Used = invite.UsedByUID != 0 || (limit != -1 && used >= limit)
		state.InviteCodes[code] = invite
	}
}

func importLegacyInviteRelations(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		child := legacyInt64(row.get("CHILD_UID"), 0)
		parent := legacyInt64(row.get("PARENT_UID"), 0)
		if child <= 0 || parent <= 0 {
			continue
		}
		state.InviteRelations[child] = InviteRelation{
			ParentUID: parent,
			ChildUID:  child,
			Code:      row.get("CODE"),
			CreatedAt: legacyInt64(row.get("CREATED_AT"), 0),
		}
	}
}

func importLegacyAnnouncements(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		id := legacyInt64(row.get("ID"), 0)
		if id <= 0 {
			continue
		}
		state.Announcements[id] = Announcement{
			ID:        id,
			Title:     firstNonEmptyStore(row.get("TITLE"), "公告"),
			Content:   row.get("CONTENT"),
			Visible:   legacyBool(row.get("VISIBLE")),
			Level:     firstNonEmptyStore(strings.ToLower(row.get("LEVEL")), "info"),
			CreatedAt: legacyInt64(row.get("CREATED_AT"), 0),
			UpdatedAt: legacyInt64(row.get("UPDATED_AT"), 0),
			ExpiredAt: legacyInt64(row.get("EXPIRES_AT"), 0),
		}
		if id >= state.NextAnnouncementID {
			state.NextAnnouncementID = id + 1
		}
	}
}

func importLegacyLoginLogs(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		id := legacyInt64(row.get("ID"), 0)
		log := LoginLog{
			ID:         id,
			UID:        legacyInt64(row.get("UID"), 0),
			IP:         row.get("IP_ADDRESS"),
			DeviceID:   row.get("DEVICE_ID"),
			DeviceName: row.get("DEVICE_NAME"),
			Client:     firstNonEmptyStore(row.get("CLIENT"), row.get("CLIENT_VERSION")),
			Time:       legacyInt64(row.get("LOGIN_TIME"), 0),
			Blocked:    legacyBool(row.get("IS_BLOCKED")),
			Country:    row.get("COUNTRY"),
			City:       row.get("CITY"),
		}
		state.LoginLogs = append(state.LoginLogs, log)
		if id >= state.NextLoginLogID {
			state.NextLoginLogID = id + 1
		}
	}
	sort.Slice(state.LoginLogs, func(i, j int) bool { return state.LoginLogs[i].Time > state.LoginLogs[j].Time })
}

func importLegacyDevices(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		uid := legacyInt64(row.get("UID"), 0)
		deviceID := row.get("DEVICE_ID")
		if uid <= 0 || deviceID == "" {
			continue
		}
		device := Device{
			UID:        uid,
			DeviceID:   deviceID,
			DeviceName: row.get("DEVICE_NAME"),
			Client:     row.get("CLIENT"),
			FirstSeen:  legacyInt64(row.get("FIRST_SEEN"), 0),
			LastSeen:   legacyInt64(row.get("LAST_SEEN"), 0),
			Trusted:    legacyBool(row.get("IS_TRUSTED")),
			Blocked:    legacyBool(row.get("IS_BLOCKED")),
		}
		state.Devices[deviceKey(uid, deviceID)] = device
	}
}

func importLegacyIPBlacklist(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		ip := row.get("IP_ADDRESS")
		if ip == "" {
			continue
		}
		state.IPBlacklist[ip] = IPBlacklistEntry{
			IP:        ip,
			Reason:    row.get("REASON"),
			CreatedAt: legacyInt64(row.get("CREATED_AT"), 0),
			ExpireAt:  legacyInt64(row.get("EXPIRE_AT"), 0),
		}
	}
}

func importLegacyPlayback(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		playedAt := legacyInt64(row.get("END_TIME"), 0)
		if playedAt == 0 {
			playedAt = legacyInt64(row.get("START_TIME"), 0)
		}
		title := firstNonEmptyStore(row.get("ITEM_NAME"), row.get("SERIES_NAME"), row.get("ITEM_ID"))
		state.PlaybackRecords = append(state.PlaybackRecords, PlaybackRecord{
			UID:       legacyInt64(row.get("UID"), 0),
			ItemID:    row.get("ITEM_ID"),
			Title:     title,
			MediaType: row.get("ITEM_TYPE"),
			Duration:  legacyInt64(row.get("DURATION"), 0),
			PlayedAt:  playedAt,
		})
	}
	sort.Slice(state.PlaybackRecords, func(i, j int) bool { return state.PlaybackRecords[i].PlayedAt > state.PlaybackRecords[j].PlayedAt })
}

func importLegacySigninPoints(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		uid := legacyInt64(row.get("UID"), 0)
		if uid <= 0 {
			continue
		}
		si := state.Signin[uid]
		si.UID = uid
		si.Points = legacyInt(row.get("CURRENT_POINTS"), si.Points)
		si.Streak = legacyInt(row.get("CURRENT_STREAK"), si.Streak)
		si.LastSignin = row.get("LAST_SIGNIN_DATE")
		state.Signin[uid] = si
	}
}

func importLegacySigninRecords(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		uid := legacyInt64(row.get("UID"), 0)
		if uid <= 0 {
			continue
		}
		si := state.Signin[uid]
		si.UID = uid
		record := SigninRecord{
			Date:      row.get("SIGNIN_DATE"),
			Points:    legacyInt(row.get("DAILY_POINTS"), 0) + legacyInt(row.get("BONUS_POINTS"), 0),
			CreatedAt: legacyInt64(row.get("CREATED_AT"), 0),
		}
		if record.Date != "" {
			si.Records = append(si.Records, record)
			if si.LastSignin == "" || record.Date > si.LastSignin {
				si.LastSignin = record.Date
			}
		}
		state.Signin[uid] = si
	}
}

func importLegacyScores(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		uid := legacyInt64(row.get("UID"), 0)
		if uid <= 0 {
			continue
		}
		si := state.Signin[uid]
		si.UID = uid
		if si.Points == 0 {
			si.Points = legacyInt(row.get("SCORE"), 0)
		}
		if si.Streak == 0 {
			si.Streak = legacyInt(row.get("CHECKIN_COUNT"), 0)
		}
		if si.LastSignin == "" {
			if ts := legacyInt64(row.get("CHECKIN_TIME"), 0); ts > 0 {
				si.LastSignin = time.Unix(ts, 0).Format("2006-01-02")
			}
		}
		state.Signin[uid] = si
	}
}

func importLegacySchedulerRuns(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		id := legacyInt64(row.get("ID"), 0)
		run := SchedulerRun{
			ID:         id,
			JobID:      row.get("JOB_ID"),
			Type:       firstNonEmptyStore(row.get("TYPE"), "auto"),
			Trigger:    row.get("TRIGGER"),
			Status:     row.get("STATUS"),
			Message:    row.get("ERROR"),
			Summary:    legacyJSONMap(row.get("SUMMARY")),
			Logs:       legacyJSONStringList(row.get("LOGS")),
			Error:      row.get("ERROR"),
			StartedAt:  legacyInt64(row.get("STARTED_AT"), 0),
			FinishedAt: legacyInt64(row.get("FINISHED_AT"), 0),
		}
		run.EndedAt = run.FinishedAt
		state.SchedulerRuns = append(state.SchedulerRuns, run)
		if id >= state.NextSchedulerRunID {
			state.NextSchedulerRunID = id + 1
		}
	}
	sort.Slice(state.SchedulerRuns, func(i, j int) bool { return state.SchedulerRuns[i].StartedAt > state.SchedulerRuns[j].StartedAt })
}

func importLegacySchedulerSchedules(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		jobID := row.get("JOB_ID")
		if jobID == "" {
			continue
		}
		custom := legacyBool(row.get("IS_CUSTOM"))
		if !custom && !legacyBool(row.get("ENABLED")) {
			continue
		}
		state.SchedulerSchedules[jobID] = SchedulerSchedule{
			JobID:       jobID,
			TriggerSpec: legacyTriggerSpec(row),
			IsCustom:    custom,
			UpdatedAt:   legacyInt64(row.get("UPDATED_AT"), 0),
		}
	}
}

func importLegacyTelegramRoster(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		chatID := row.get("CHAT_ID")
		telegramID := legacyInt64(row.get("TELEGRAM_ID"), 0)
		if chatID == "" || telegramID <= 0 {
			continue
		}
		state.TelegramRoster[telegramRosterKey(chatID, telegramID)] = TelegramRosterEntry{
			ChatID:     chatID,
			TelegramID: telegramID,
			IsBot:      legacyBool(row.get("IS_BOT")),
			LastStatus: firstNonEmptyStore(row.get("LAST_STATUS"), "member"),
			FirstSeen:  legacyInt64(row.get("FIRST_SEEN_AT"), 0),
			LastSeen:   legacyInt64(row.get("LAST_SEEN_AT"), 0),
		}
	}
}

func importLegacyBangumiUsers(state *State, rows []legacyRow, _ func(string, ...any)) {
	for _, row := range rows {
		telegramID := legacyInt64(row.get("TELEGRAM_ID"), 0)
		if telegramID <= 0 {
			continue
		}
		for uid, user := range state.Users {
			if user.TelegramID == telegramID {
				if token := row.get("ACCESS_TOKEN"); token != "" {
					user.BGMToken = token
				}
				user.BGMMode = legacyBool(row.get("AUTO_UPDATE"))
				state.Users[uid] = user
				break
			}
		}
	}
}

func importLegacyMediaRequests(state *State, rows []legacyRow, source string, _ func(string, ...any)) {
	usersByTelegram := map[int64]User{}
	for _, user := range state.Users {
		if user.TelegramID != 0 {
			usersByTelegram[user.TelegramID] = user
		}
	}
	for _, row := range rows {
		id := legacyInt64(row.get("ID"), 0)
		if id <= 0 {
			continue
		}
		telegramID := legacyInt64(row.get("TELEGRAM_ID"), 0)
		user := usersByTelegram[telegramID]
		mediaID := legacyInt64(row.get("BANGUMI_ID"), 0)
		if source == "tmdb" {
			mediaID = legacyInt64(row.get("TMDB_ID"), 0)
		}
		req := MediaRequest{
			ID:         id,
			RequireKey: row.get("REQUIRE_KEY"),
			UID:        user.UID,
			TelegramID: telegramID,
			Username:   user.Username,
			Title:      row.get("TITLE"),
			Source:     source,
			MediaID:    mediaID,
			MediaType:  row.get("MEDIA_TYPE"),
			Season:     legacyInt(row.get("SEASON"), 0),
			Year:       row.get("YEAR"),
			Status:     legacyMediaStatus(row.get("STATUS")),
			AdminNote:  row.get("ADMIN_NOTE"),
			MediaInfo:  legacyJSONMap(row.get("OTHER_INFO")),
			CreatedAt:  legacyInt64(row.get("TIMESTAMP"), 0),
			UpdatedAt:  legacyInt64(row.get("TIMESTAMP"), 0),
		}
		if req.RequireKey == "" {
			req.RequireKey = randomKey("legacy_req", id, req.CreatedAt)
		}
		if req.Title == "" {
			req.Title = req.RequireKey
		}
		if req.MediaType == "" {
			req.MediaType = "unknown"
		}
		state.MediaRequests[id] = req
		if id >= state.NextRequestID {
			state.NextRequestID = id + 1
		}
	}
}

func legacyTriggerSpec(row legacyRow) map[string]any {
	switch strings.ToLower(row.get("TRIGGER_TYPE")) {
	case "cron_daily", "daily", "cron":
		return map[string]any{
			"type":   "cron_daily",
			"hour":   legacyInt(row.get("CRON_HOUR"), 0),
			"minute": legacyInt(row.get("CRON_MINUTE"), 0),
		}
	default:
		seconds := legacyInt(row.get("INTERVAL_SECONDS"), 3600)
		if seconds <= 0 {
			seconds = 3600
		}
		return map[string]any{"type": "interval", "seconds": seconds}
	}
}

func legacyMediaStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "pending", "unhandled", "pending_review":
		return "UNHANDLED"
	case "1", "accepted", "approved":
		return "ACCEPTED"
	case "2", "rejected", "reject":
		return "REJECTED"
	case "3", "completed", "complete", "done":
		return "COMPLETED"
	case "4", "downloading", "download":
		return "DOWNLOADING"
	default:
		return "UNHANDLED"
	}
}

func legacySkippedTables(tableCounts map[string]int, mapped map[string]bool) []string {
	if len(tableCounts) == 0 {
		return nil
	}
	out := []string{}
	for key := range tableCounts {
		if !mapped[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func legacySQLiteMappingDetails(tableCounts map[string]int, mapped map[string]bool) []LegacySQLiteMapping {
	seen := map[string]bool{}
	out := make([]LegacySQLiteMapping, 0, len(legacySQLiteMappingCatalog))
	for _, item := range legacySQLiteMappingCatalog {
		item.Rows = tableCounts[item.SourceKey]
		item.Mapped = mapped[item.SourceKey]
		out = append(out, item)
		seen[item.SourceKey] = true
	}
	for key, rows := range tableCounts {
		if seen[key] {
			continue
		}
		parts := strings.SplitN(key, ".", 2)
		item := LegacySQLiteMapping{
			SourceKey: key,
			Target:    "unmapped",
			Rows:      rows,
			Mapped:    false,
		}
		if len(parts) == 2 {
			item.SourceDatabase = parts[0]
			item.SourceTable = parts[1]
		} else {
			item.SourceTable = key
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceKey < out[j].SourceKey })
	return out
}

func sortedMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func databaseStateCountsStore(state State) map[string]int {
	return map[string]int{
		"users":               len(state.Users),
		"api_keys":            len(state.APIKeys),
		"regcodes":            len(state.RegCodes),
		"invite_codes":        len(state.InviteCodes),
		"invite_relations":    len(state.InviteRelations),
		"media_requests":      len(state.MediaRequests),
		"announcements":       len(state.Announcements),
		"bind_codes":          len(state.BindCodes),
		"signin":              len(state.Signin),
		"scheduler_runs":      len(state.SchedulerRuns),
		"scheduler_schedules": len(state.SchedulerSchedules),
		"devices":             len(state.Devices),
		"login_logs":          len(state.LoginLogs),
		"ip_blacklist":        len(state.IPBlacklist),
		"playback_records":    len(state.PlaybackRecords),
		"rebind_requests":     len(state.RebindRequests),
		"telegram_roster":     len(state.TelegramRoster),
	}
}

func (r legacyRow) get(name string) string {
	return strings.TrimSpace(r[strings.ToUpper(name)])
}

func legacyBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func legacyInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func legacyInt64(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func legacyInt64List(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []int64
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return values
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	})
	out := []int64{}
	for _, part := range parts {
		if v := legacyInt64(strings.Trim(part, `"'[]`), 0); v != 0 {
			out = appendUniqueInt64(out, v)
		}
	}
	return out
}

func legacyPermissionList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return cleanLegacyStringList(values)
	}
	raw = strings.Trim(raw, "[]")
	return cleanLegacyStringList(strings.Split(raw, ","))
}

func cleanLegacyStringList(values []string) []string {
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

func legacyMetadata(raw string) map[string]any {
	var payload map[string]any
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &payload) != nil {
		return map[string]any{}
	}
	return payload
}

func legacyStringMeta(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func legacyJSONMap(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	return nil
}

func legacyJSONStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return cleanLegacyStringList(values)
	}
	return cleanLegacyStringList(strings.Split(raw, "\n"))
}

func legacySafeUploadAsset(raw, kind string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	base := filepath.Base(raw)
	if legacyUploadAssetFilenamePattern.MatchString(base) {
		return "/api/v1/users/assets/" + kind + "/" + base
	}
	return ""
}

func legacySafeBackground(raw string) string {
	payload := legacyMetadata(raw)
	background, ok := payload["background"].(map[string]any)
	if !ok {
		return ""
	}
	clean := map[string]any{}
	for _, key := range []string{"lightBg", "darkBg"} {
		if value := legacySafeGradient(fmt.Sprint(background[key])); value != "" {
			clean[key] = value
		}
	}
	for _, key := range []string{"lightBgImage", "darkBgImage"} {
		if value := legacySafeBackgroundImage(fmt.Sprint(background[key])); value != "" {
			clean[key] = value
		}
	}
	for _, key := range []string{"lightFlow", "darkFlow"} {
		if value, ok := background[key].(bool); ok {
			clean[key] = value
		}
	}
	for _, key := range []string{"lightBlur", "darkBlur", "lightOpacity", "darkOpacity"} {
		if value := legacyInt(fmt.Sprint(background[key]), -1); value >= 0 {
			clean[key] = value
		}
	}
	if len(clean) == 0 {
		return ""
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return ""
	}
	return string(data)
}

func legacySafeGradient(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 || strings.ContainsAny(value, ";\x00\r\n") || strings.Contains(strings.ToLower(value), "url(") {
		return ""
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"linear-gradient(", "radial-gradient(", "conic-gradient(", "repeating-linear-gradient(", "repeating-radial-gradient("} {
		if strings.HasPrefix(lower, prefix) {
			return value
		}
	}
	return ""
}

func legacySafeBackgroundImage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "url(") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "url("), ")"))
		value = strings.Trim(value, `"'`)
	}
	base := filepath.Base(value)
	if legacyUploadAssetFilenamePattern.MatchString(base) {
		return `url("/api/v1/users/assets/background/` + base + `")`
	}
	return ""
}
