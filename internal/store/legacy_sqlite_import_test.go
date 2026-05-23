package store

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildLegacySQLiteSnapshotImportsMappedDatabases(t *testing.T) {
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 command is not available")
	}
	dir := t.TempDir()
	runSQLite(t, sqliteBin, filepath.Join(dir, "users.db"), `
CREATE TABLE users (
UID INTEGER PRIMARY KEY, TELEGRAM_ID INTEGER, USERNAME VARCHAR, EMAIL VARCHAR, ROLE INTEGER NOT NULL,
ACTIVE_STATUS BOOLEAN, CREATE_AT INTEGER, REGISTER_TIME INTEGER, EXPIRED_AT INTEGER, EMBYID VARCHAR,
PASSWORD VARCHAR, BGM_MODE BOOLEAN, BGM_TOKEN VARCHAR, APIKEY_STATUS BOOLEAN, APIKEY VARCHAR,
APIKEY_PERMISSIONS VARCHAR, AVATAR VARCHAR, OTHER VARCHAR, PENDING_EMBY BOOLEAN DEFAULT 0, PENDING_EMBY_DAYS INTEGER
);
INSERT INTO users (UID, TELEGRAM_ID, USERNAME, EMAIL, ROLE, ACTIVE_STATUS, CREATE_AT, REGISTER_TIME, EXPIRED_AT, EMBYID, PASSWORD, APIKEY_STATUS, APIKEY_PERMISSIONS)
VALUES (7, 10001, 'admin', 'a@example.com', 0, 1, 1700000000, 1700000001, -1, 'emby-7', 'salt$hash', 1, '["account:read"]');
`)
	runSQLite(t, sqliteBin, filepath.Join(dir, "regcode.db"), `
CREATE TABLE regcode (
CODE VARCHAR PRIMARY KEY, VALIDITY_TIME INTEGER NOT NULL, TYPE INTEGER NOT NULL, UID VARCHAR, TELEGRAM_ID VARCHAR,
USE_COUNT_LIMIT INTEGER NOT NULL, USE_COUNT INTEGER NOT NULL, CREATED_TIME INTEGER NOT NULL, DAYS INTEGER,
ACTIVE BOOLEAN NOT NULL, OTHER VARCHAR
);
INSERT INTO regcode VALUES ('TW-ABC', 24, 1, '7', '10001', 1, 1, 1700000100, 30, 1, '{"note":"legacy","decoy":false}');
`)
	runSQLite(t, sqliteBin, filepath.Join(dir, "invites.db"), `
CREATE TABLE invite_codes (
CODE VARCHAR(64) PRIMARY KEY, INVITER_UID INTEGER NOT NULL, DAYS INTEGER NOT NULL, USE_COUNT_LIMIT INTEGER NOT NULL,
USE_COUNT INTEGER NOT NULL, EXPIRES_AT INTEGER NOT NULL, USED_BY_UID INTEGER, USED_AT INTEGER, ACTIVE BOOLEAN NOT NULL,
CREATED_AT INTEGER NOT NULL, NOTE VARCHAR(255)
);
CREATE TABLE invite_relations (CHILD_UID INTEGER PRIMARY KEY, PARENT_UID INTEGER NOT NULL, CODE VARCHAR(64), CREATED_AT INTEGER NOT NULL);
INSERT INTO invite_codes VALUES ('inv-1', 7, 30, 1, 1, 1800000000, 8, 1700000200, 0, 1700000000, 'note');
INSERT INTO invite_relations VALUES (8, 7, 'inv-1', 1700000200);
`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, result, err := BuildLegacySQLiteSnapshot(ctx, dir)
	if err != nil {
		t.Fatalf("BuildLegacySQLiteSnapshot returned error: %v", err)
	}
	if !result.Detected || result.Counts["users"] != 1 || result.Counts["regcodes"] != 1 || result.Counts["invite_codes"] != 1 {
		t.Fatalf("unexpected import counts: %#v", result)
	}
	if len(result.Mappings) == 0 {
		t.Fatalf("expected migration field mappings in result")
	}
	if !legacyMappingPresent(result.Mappings, "users.users", "users", true) || !legacyMappingPresent(result.Mappings, "regcode.regcode", "regcodes", true) {
		t.Fatalf("expected mapped legacy tables in result: %#v", result.Mappings)
	}
	var state State
	if err := json.Unmarshal(snapshot, &state); err != nil {
		t.Fatal(err)
	}
	admin := state.Users[7]
	if admin.Username != "admin" || admin.Role != RoleAdmin || admin.TelegramID != 10001 || admin.LegacyPermissions[0] != "account:read" {
		t.Fatalf("user was not mapped correctly: %#v", admin)
	}
	if state.RegCodes["TW-ABC"].Note != "legacy" || state.InviteRelations[8].ParentUID != 7 {
		t.Fatalf("codes were not mapped correctly: reg=%#v rel=%#v", state.RegCodes["TW-ABC"], state.InviteRelations[8])
	}
}

func legacyMappingPresent(mappings []LegacySQLiteMapping, key, target string, mapped bool) bool {
	for _, item := range mappings {
		if item.SourceKey == key && item.Target == target && item.Mapped == mapped && len(item.Fields) > 0 {
			return true
		}
	}
	return false
}

func TestBackupLegacySQLiteCopiesDatabaseAndWALFiles(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.WriteFile(filepath.Join(dir, "users.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users.db-wal"), []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, detected, err := BackupLegacySQLite(dir, backupDir)
	if err != nil || !detected {
		t.Fatalf("BackupLegacySQLite detected=%v err=%v", detected, err)
	}
	if info.FileCount != 2 {
		t.Fatalf("expected 2 copied files, got %#v", info)
	}
	names := []string{info.Files[0].Name, info.Files[1].Name}
	if !strings.Contains(strings.Join(names, ","), "users.db") || !strings.Contains(strings.Join(names, ","), "users.db-wal") {
		t.Fatalf("unexpected copied files: %#v", names)
	}
}

func runSQLite(t *testing.T, sqliteBin, path, sql string) {
	t.Helper()
	cmd := exec.Command(sqliteBin, path, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 %s failed: %v: %s", filepath.Base(path), err, strings.TrimSpace(string(out)))
	}
}
