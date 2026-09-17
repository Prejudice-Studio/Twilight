package api

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// 一次可视化保存曾经会把 schema 之外的键整个删掉：管理员手写的段（Admin）
// 与尚未纳管的字段（Emby.emby_public_url）都会消失，而且再也无法从页面补回。
// 这两个用例把"合并而不是覆盖"的行为钉住。
func TestMergeConfigTOMLKeepsUnmanagedSections(t *testing.T) {
	source := strings.Join([]string{
		`[Emby]`,
		`emby_url = "http://127.0.0.1:8096"`,
		`custom_note = "hand written field"`,
		``,
		`[Admin]`,
		`uids = [1, 2]`,
		`usernames = ["root"]`,
		``,
		`[Custom]`,
		`note = "hand written"`,
		``,
	}, "\n")

	values := map[string]map[string]any{
		"Emby": {"emby_url": "http://127.0.0.1:8096", "emby_token": "token", "emby_public_url": "https://emby.example.com"},
	}

	merged, err := mergeConfigTOML(source, values)
	if err != nil {
		t.Fatalf("mergeConfigTOML: %v", err)
	}

	tree := map[string]any{}
	if err := toml.Unmarshal([]byte(merged), &tree); err != nil {
		t.Fatalf("merged output is not valid TOML: %v\n%s", err, merged)
	}
	emby, ok := tree["Emby"].(map[string]any)
	if !ok {
		t.Fatalf("Emby section missing:\n%s", merged)
	}
	if emby["custom_note"] != "hand written field" {
		t.Fatalf("unmanaged field inside managed section dropped: %#v", emby)
	}
	if emby["emby_public_url"] != "https://emby.example.com" {
		t.Fatalf("managed field not applied: %#v", emby)
	}
	if emby["emby_url"] != "http://127.0.0.1:8096" {
		t.Fatalf("managed field not applied: %#v", emby)
	}
	admin, ok := tree["Admin"].(map[string]any)
	if !ok {
		t.Fatalf("unmanaged section dropped:\n%s", merged)
	}
	if len(admin) != 2 {
		t.Fatalf("Admin section lost keys: %#v", admin)
	}
	custom, ok := tree["Custom"].(map[string]any)
	if !ok || custom["note"] != "hand written" {
		t.Fatalf("custom section dropped:\n%s", merged)
	}
}

// 顶层标量（例如 SetupMode）既不属于任何表，也不能被排到表头之后——否则会
// 被 TOML 解析成上一个表的字段。
func TestMergeConfigTOMLKeepsTopLevelScalars(t *testing.T) {
	source := "SetupMode = false\n\n[Global]\nserver_name = \"twilight\"\n"
	values := map[string]map[string]any{"Global": {"server_name": "renamed"}}

	merged, err := mergeConfigTOML(source, values)
	if err != nil {
		t.Fatalf("mergeConfigTOML: %v", err)
	}
	tree := map[string]any{}
	if err := toml.Unmarshal([]byte(merged), &tree); err != nil {
		t.Fatalf("merged output is not valid TOML: %v\n%s", err, merged)
	}
	if tree["SetupMode"] != false {
		t.Fatalf("top-level scalar lost or moved: %#v\n%s", tree, merged)
	}
	global, ok := tree["Global"].(map[string]any)
	if !ok || global["server_name"] != "renamed" {
		t.Fatalf("managed value not applied: %#v", tree)
	}
}

// 底稿为空（首次保存）时不应报错，行为退化为"只写 schema 管理的字段"。
func TestMergeConfigTOMLWithEmptySource(t *testing.T) {
	values := map[string]map[string]any{"Global": {"server_name": "twilight"}}
	merged, err := mergeConfigTOML("", values)
	if err != nil {
		t.Fatalf("mergeConfigTOML: %v", err)
	}
	if !strings.Contains(merged, "[Global]") || !strings.Contains(merged, `server_name = "twilight"`) {
		t.Fatalf("unexpected output:\n%s", merged)
	}
}
