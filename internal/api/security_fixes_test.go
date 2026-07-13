package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

// TestCSVSafeNeutralizesFormulaInjection 验证导出列经过 csvSafe 中和后，
// 以 = + - @ / Tab / CR / LF 开头的值会被前置单引号，普通文本与空串原样保留。
// 防回归点：handleExportUsers 的用户名、邮箱
// 直接写进 CSV，恶意用户名形如 "=HYPERLINK(...)" 会在管理员用电子表格打开时执行。
func TestCSVSafeNeutralizesFormulaInjection(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"alice", "alice"},
		{"=1+2", "'=1+2"},
		{"+cmd", "'+cmd"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\tTabbed", "'\tTabbed"},
		{"\rCarriage", "'\rCarriage"},
		{"\nNewline", "'\nNewline"},
		{"normal=mid", "normal=mid"}, // 仅首字符触发
		{"=HYPERLINK(\"http://evil\",\"x\")", "'=HYPERLINK(\"http://evil\",\"x\")"},
	}
	for _, c := range cases {
		if got := csvSafe(c.in); got != c.want {
			t.Errorf("csvSafe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestExportUsersCSVSanitizesMaliciousUsername 端到端验证：注册一个公式注入
// 用户名后，管理员导出的 users.csv 对应单元格被前置单引号中和。
func TestExportUsersCSVSanitizesMaliciousUsername(t *testing.T) {
	app := newTestApp(t)
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")

	// 直接通过 store 注入一个公式注入用户名（绕过注册期用户名字符校验，
	// 因为校验未禁用 = + -，本测试聚焦导出端中和而非注册校验）。
	if _, err := app.store().CreateUser(store.User{Username: "=2+5+cmd", Active: true, PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}

	resp := doJSONWithHeaders(app, http.MethodGet, "/api/v1/batch/export/users", "", adminCookies, map[string]string{"X-Twilight-Client": "webui"})
	if resp.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if strings.Contains(body, ",=2+5+cmd,") || strings.Contains(body, "\"=2+5+cmd\"") {
		t.Fatalf("malicious username was not neutralized in CSV: %s", body)
	}
	if !strings.Contains(body, "'=2+5+cmd") {
		t.Fatalf("expected single-quote-prefixed username in CSV: %s", body)
	}
}

// TestInviteMeDoesNotLeakSensitiveFields 防回归：invite/me 的 parent / children
// 不得包含 email / telegram_id / telegram_username / emby_id 等敏感字段（旧实现
// 直接复用 publicUser 会把上下级私密信息互相泄露）。
func TestInviteMeDoesNotLeakSensitiveFields(t *testing.T) {
	app := newTestApp(t)
	// 父账号（admin = 第一个注册）。
	parentCookies := registerAndLogin(t, app, "parentuser", "Admin123456")
	parent, _ := app.store().FindUserByUsername("parentuser")
	// 给父账号注入敏感字段，确保即便有值也不外泄。
	if _, err := app.store().UpdateUser(parent.UID, func(u *store.User) error {
		u.Email = "parent@example.com"
		u.TelegramID = 111222333
		u.TelegramUsername = "parent_tg"
		u.EmbyID = "emby-parent"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 子账号，通过消费邀请码建立邀请关系（store 没有公开的 SetParent）。
	child, err := app.store().CreateUser(store.User{Username: "childuser", Active: true, PasswordHash: "x", Email: "child@example.com", TelegramID: 444555666, TelegramUsername: "child_tg", EmbyID: "emby-child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "INVITE-CHILD", InviterUID: parent.UID, Active: true, UseCountLimit: -1}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("INVITE-CHILD", child.UID); err != nil {
		t.Fatal(err)
	}

	resp := doJSON(app, http.MethodGet, "/api/v1/invite/me", "", parentCookies)
	if resp.Code != http.StatusOK {
		t.Fatalf("invite/me status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, leak := range []string{"child@example.com", "444555666", "child_tg", "emby-child", "parent@example.com", "111222333", "parent_tg", "emby-parent"} {
		if strings.Contains(body, leak) {
			t.Fatalf("invite/me leaked sensitive value %q: %s", leak, body)
		}
	}

	// 同时确认 children 仍带展示所需字段。
	var env struct {
		Data struct {
			Children []map[string]any `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(env.Data.Children))
	}
	c := env.Data.Children[0]
	if c["username"] != "childuser" {
		t.Fatalf("child username missing/wrong: %#v", c)
	}
	if _, ok := c["has_emby"]; !ok {
		t.Fatalf("child has_emby missing: %#v", c)
	}
	for _, banned := range []string{"email", "telegram_id", "telegram_username", "emby_id", "emby_username"} {
		if _, ok := c[banned]; ok {
			t.Fatalf("child DTO must not contain %q: %#v", banned, c)
		}
	}
}

// TestOpenAPIOnlyExposesPublicRoutes 防回归：未鉴权的 openapi.json 不得枚举
// admin 攻击面（旧实现列出全部路由，含 /admin/* 与 system/admin/*）。
func TestOpenAPIOnlyExposesPublicRoutes(t *testing.T) {
	app := newTestApp(t)
	resp := doJSON(app, http.MethodGet, "/api/v1/openapi.json", "", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("openapi status=%d body=%s", resp.Code, resp.Body.String())
	}
	var env struct {
		Data struct {
			Paths map[string]any `json:"paths"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	for pattern := range env.Data.Paths {
		if strings.Contains(pattern, "/admin/") {
			t.Fatalf("openapi.json must not expose admin route: %s", pattern)
		}
	}
	// 正向断言：公开路由仍然存在（如登录）。
	if _, ok := env.Data.Paths["/api/v1/auth/login"]; !ok {
		t.Fatalf("openapi.json should still list public routes; got %#v", env.Data.Paths)
	}
}

// TestFirstRegistrantIsNotAutoAdmin 防回归：已移除"空数据库首注册者无条件
// 成为 Admin"通道。管理员身份只能来自配置文件 admin_uids / admin_usernames。
// 本测试用一个不含任何 admin 配置的 App，确认首个注册用户只是普通用户。
func TestFirstRegistrantIsNotAutoAdmin(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(config.Config{
		AppName:         "Twilight Test",
		Version:         "test",
		Host:            "127.0.0.1",
		Port:            0,
		DatabaseDir:     dir,
		DatabaseDriver:  store.BackendJSON,
		StateFile:       filepath.Join(dir, "state.json"),
		SessionCookie:   "twilight_session",
		SessionTTL:      time.Hour,
		CookieSameSite:  "lax",
		RegisterEnabled: true,
		// 关键：不配置任何 admin_uids / admin_usernames（生产默认）。
	}, st)
	if err != nil {
		t.Fatal(err)
	}

	resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"firstcomer","password":"First123456"}`, nil)
	if resp.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", resp.Code, resp.Body.String())
	}
	u, ok := app.store().FindUserByUsername("firstcomer")
	if !ok {
		t.Fatal("first user was not created")
	}
	if u.Role == store.RoleAdmin {
		t.Fatalf("SECURITY REGRESSION: first registrant auto-granted admin role: %#v", u)
	}
	// 响应也不得把首注册者标记为 first_admin。
	var env struct {
		Data struct {
			FirstAdmin bool `json:"first_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.FirstAdmin {
		t.Fatalf("first_admin must be false when no admin is configured: %s", resp.Body.String())
	}
}

// TestConfiguredUsernameIsPromotedToAdmin 正向：配置了 admin_usernames 时，
// 用该用户名注册的账户会被提升为管理员（且仅该用户名）。
func TestConfiguredUsernameIsPromotedToAdmin(t *testing.T) {
	app := newTestApp(t) // harness 配置 AdminUsernames=["admin"]
	// 先注册一个非配置用户名 —— 即便是首个用户也不应是 admin。
	if resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"plainuser","password":"Plain123456"}`, nil); resp.Code != http.StatusCreated {
		t.Fatalf("register plainuser status=%d body=%s", resp.Code, resp.Body.String())
	}
	plain, _ := app.store().FindUserByUsername("plainuser")
	if plain.Role == store.RoleAdmin {
		t.Fatalf("non-configured first user must not be admin: %#v", plain)
	}
	// 配置的用户名注册后应成为 admin。
	if resp := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"admin","password":"Admin123456"}`, nil); resp.Code != http.StatusCreated {
		t.Fatalf("register admin status=%d body=%s", resp.Code, resp.Body.String())
	}
	admin, ok := app.store().FindUserByUsername("admin")
	if !ok || admin.Role != store.RoleAdmin || !admin.Active {
		t.Fatalf("configured admin username was not promoted: ok=%v user=%#v", ok, admin)
	}
}

// TestRepoURLCannotBeChangedViaWebConfig 防回归：[SystemUpdate].repo_url 不得经
// 网页配置接口改写。被盗管理员会话若能把 origin 指向攻击者 fork，再触发 git 自动
// 更新即可在服务器上 RCE。saveConfigContent 必须把提交内容里的 repo_url 就地还原
// 为磁盘原值。
func TestRepoURLCannotBeChangedViaWebConfig(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	databaseConfig := "[Database]\n" +
		"driver = \"json\"\n" +
		"state_file = " + strconv.Quote(app.cfg().StateFile) + "\n" +
		"backup_dir = " + strconv.Quote(app.cfg().DatabaseBackupDir) + "\n"
	existing := "[Global]\nserver_name = \"old\"\n\n" + databaseConfig +
		"\n[SystemUpdate]\nrepo_url = \"https://github.com/trusted/twilight.git\"\nbranch = \"main\"\n"
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	// 提交一份把 repo_url 改成攻击者仓库的配置。
	malicious := "[Global]\nserver_name = \"new\"\n\n" + databaseConfig +
		"\n[SystemUpdate]\nrepo_url = \"https://github.com/attacker/evil.git\"\nbranch = \"main\"\n"
	info, status, message := app.saveConfigContent(malicious)
	if status != http.StatusOK {
		t.Fatalf("saveConfigContent status=%d message=%s info=%v", status, message, info)
	}
	data, err := os.ReadFile(app.cfg().ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "attacker/evil") {
		t.Fatalf("SECURITY REGRESSION: repo_url was overwritten via web config: %s", content)
	}
	if !strings.Contains(content, "trusted/twilight") {
		t.Fatalf("original repo_url was not preserved: %s", content)
	}
}

func TestAPIDocsConsoleUsesPublicFallbackAndRealAPIKeyHeader(t *testing.T) {
	app := newTestApp(t)
	resp := doJSON(app, http.MethodGet, "/api/v1/docs", "", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("docs status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, want := range []string{
		"Twilight API 控制台",
		"/api/v1/openapi.json",
		"/api/v1/system/admin/apis",
		"X-API-Key",
		"公开 OpenAPI",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs page missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"X-Twilight-API-Key",
		"replace(//$",
		"replace(/^/api/v1/",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("docs page contains stale/broken snippet %q", forbidden)
		}
	}
}
