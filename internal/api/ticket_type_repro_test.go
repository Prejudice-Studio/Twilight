package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureTicketDefaultsPreservesStringList 回归测试历史 bug：
// ensureTicketDefaults 早期只对 types 做 []any 类型断言，而 configValues 由
// cfg.TicketTypes 产出的是 []string，断言失败导致每次保存都把用户新增的工单
// 类型整体丢弃、回退成 ["all"]。这里直接覆盖各种输入形态。
func TestEnsureTicketDefaultsPreservesStringList(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"string slice", []string{"all", "bug"}, []string{"all", "bug"}},
		{"any slice", []any{"all", "feature"}, []string{"all", "feature"}},
		{"single string", "all", []string{"all"}},
		{"empty falls back", []string{}, []string{"all"}},
		{"dedupe case-insensitive", []string{"All", "all", "Bug"}, []string{"All", "Bug"}},
		{"trims blanks", []string{" all ", "", "bug"}, []string{"all", "bug"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]map[string]any{"Ticket": {"types": tc.in}}
			ensureTicketDefaults(values)
			got, ok := values["Ticket"]["types"].([]string)
			if !ok {
				t.Fatalf("expected []string result, got %T", values["Ticket"]["types"])
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("ensureTicketDefaults(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestTicketTypeAddPersistsToConfigFile 通过 HTTP 入口新增工单类型，断言它真正
// 落盘到 config.toml（这是重启 / 热重载后仍保留的唯一持久化来源）。
func TestTicketTypeAddPersistsToConfigFile(t *testing.T) {
	app := newTestApp(t)
	app.cfg().ConfigFile = filepath.Join(app.cfg().DatabaseDir, "config.toml")
	initial := "[Admin]\nusernames = [\"admin\"]\n\n[Ticket]\nenabled = true\ntypes = [\"all\"]\n"
	if err := os.WriteFile(app.cfg().ConfigFile, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	adminCookies := registerAndLogin(t, app, "admin", "Admin123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	resp := doJSONWithHeaders(app, http.MethodPost, "/api/v1/admin/ticket-types", `{"name":"bug"}`, adminCookies, headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("add ticket type status=%d body=%s", resp.Code, resp.Body.String())
	}
	content, err := os.ReadFile(app.cfg().ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"bug"`) {
		t.Fatalf("expected config.toml to contain new type, got:\n%s", string(content))
	}
	if !strings.Contains(string(content), `"all"`) {
		t.Fatalf("expected config.toml to keep existing type, got:\n%s", string(content))
	}
}
