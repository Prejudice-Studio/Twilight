package api

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
)

// schemaUncoveredConfigFields 列出"存在于 config.Config 但故意不进可视化配置页"
// 的字段。新增字段一旦忘记挂到 configSectionDefs，本测试会失败——要么把它加进
// schema（页面能编辑），要么在这里写明为什么不能暴露。禁止无脑往清单里塞。
var schemaUncoveredConfigFields = map[string]string{
	"Version":         "构建期常量，不接受用户配置",
	"ConfigFile":      "由启动参数决定，写进文件反而自指",
	"AdminUIDs":       "管理员名单，走后台用户管理，页面误改会把自己锁在外面",
	"AdminUsernames":  "同 AdminUIDs",
	"SetupMode":       "首次安装向导状态机由程序推进",
	"SessionTTL":      "会话有效期，单位/格式不适合表单，改动会立刻踢掉所有登录态",
	"AllowCredential": "CORS 凭证开关，与 CORSOrigins 联动，单独暴露容易配出自相矛盾的组合",
}

// schemaRoundTripNormalizedFields 列出"确实挂在可视化配置页上，但取值在 config.Load
// 时被归一化"的字段。它们能被编辑、能存回文件，只是不能做原值 round-trip 比对
// （塞进去的任意串会被折叠成合法枚举），因此单独排除，语义上不等于"没纳管"。
var schemaRoundTripNormalizedFields = map[string]string{
	"LogLevel": "config.Load 会用 normalizeLogLevel 折叠非法取值，任意串无法原样读回",
}

// fillDistinctConfigValues 用反射给 config.Config 每个可设置字段塞一个非零的
// 特征值。目的不是构造"合法配置"，而是让任何被漏掉的字段在 round-trip 后必然
// 与原文不同，从而被 TestConfigSchemaSurfacesEveryConfigField 抓出来。
func fillDistinctConfigValues(cfg *config.Config) {
	value := reflect.ValueOf(cfg).Elem()
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || !value.Field(i).CanSet() {
			continue
		}
		target := value.Field(i)
		seed := "twilight-coverage-" + strings.ToLower(field.Name)
		switch target.Kind() {
		case reflect.String:
			target.SetString(seed)
		case reflect.Bool:
			target.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			// SessionTTL 是 time.Duration（底层 int64），单独处理。
			if target.Type() == reflect.TypeOf(time.Duration(0)) {
				target.SetInt(int64(7 * time.Minute))
				continue
			}
			target.SetInt(7)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			target.SetUint(7)
		case reflect.Float32, reflect.Float64:
			target.SetFloat(7.5)
		case reflect.Slice:
			elem := target.Type().Elem()
			switch elem.Kind() {
			case reflect.String:
				target.Set(reflect.ValueOf([]string{seed + "-a", seed + "-b"}))
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				// 切片元素类型必须和字段声明完全一致（[]int 不能塞 []int64）。
				filled := reflect.MakeSlice(target.Type(), 2, 2)
				for idx := 0; idx < 2; idx++ {
					filled.Index(idx).SetInt(int64(11 + idx))
				}
				target.Set(filled)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				filled := reflect.MakeSlice(target.Type(), 2, 2)
				for idx := 0; idx < 2; idx++ {
					filled.Index(idx).SetUint(uint64(11 + idx))
				}
				target.Set(filled)
			default:
				// []Line / []TelegramCommandReply 这类结构体切片有自己的序列化
				// 规则，这里不参与枚举（它们已由各自的专项 round-trip 测试覆盖）。
			}
		}
	}
}

// TestConfigSchemaSurfacesEveryConfigField 防回归：config.Config 里的字段，除了
// schemaUncoveredConfigFields 白名单之外，必须能被可视化配置页"看得见、存得住"。
//
// 背景：配置页的字段清单来自 configSectionDefs()，取值来自 configValues()。两者
// 都是手写的，任何一个新字段忘了挂进去，现象就是"系统里明明有这个配置，页面上
// 要么找不到、要么显示成空值，保存也写不回去"——也就是管理员反馈的"不会补"。
// 这里用 configValues → renderConfigTOML → config.Load 的整条链路做 round-trip，
// 只要某字段没进 schema，它就不会出现在渲染出的 TOML 里，读回来必然对不上。
func TestConfigSchemaSurfacesEveryConfigField(t *testing.T) {
	want := config.Config{}
	fillDistinctConfigValues(&want)
	// 这两个字段必然被 Load 覆写/来自运行环境，不参与比对。
	want.ConfigFile = ""
	want.Version = ""

	content := renderConfigTOML(configValues(want))
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("渲染出的配置无法被 config.Load 读回: %v\n%s", err, content)
	}

	wantValue := reflect.ValueOf(want)
	gotValue := reflect.ValueOf(got)
	missing := []string{}
	for i := 0; i < wantValue.NumField(); i++ {
		field := wantValue.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		if _, allowed := schemaUncoveredConfigFields[field.Name]; allowed {
			continue
		}
		if _, normalized := schemaRoundTripNormalizedFields[field.Name]; normalized {
			continue
		}
		wantField := wantValue.Field(i)
		// 填充器跳过的字段（结构体切片等）保持 nil，读回来常是空切片，
		// nil 与空切片在这里视为等价，不参与比对。
		if isNilable(wantField) && wantField.IsNil() {
			continue
		}
		if !reflect.DeepEqual(wantField.Interface(), gotValue.Field(i).Interface()) {
			missing = append(missing, field.Name)
		}
	}
	sort.Strings(missing)
	if len(missing) == 0 {
		return
	}
	t.Errorf(
		"以下 config.Config 字段没有被可视化配置页纳管（保存时写不回 config.toml，页面上也读不到）：\n  %s\n"+
			"处理方式二选一：在 configSectionDefs/configValues 里补上，或把它加进 "+
			"schemaUncoveredConfigFields 并写明原因。",
		strings.Join(missing, "\n  "),
	)
}

func isNilable(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func:
		return true
	default:
		return false
	}
}

// TestConfigSchemaNoUncoveredFieldWithoutReason 防止白名单变成"垃圾桶"：清单里
// 的每一项都必须对应真实存在的字段，且必须写了原因。
func TestConfigSchemaNoUncoveredFieldWithoutReason(t *testing.T) {
	typ := reflect.TypeOf(config.Config{})
	for _, list := range []map[string]string{schemaUncoveredConfigFields, schemaRoundTripNormalizedFields} {
		for name, reason := range list {
			if _, ok := typ.FieldByName(name); !ok {
				t.Errorf("白名单里的 %q 已经不是 config.Config 的字段了，应删除", name)
			}
			if strings.TrimSpace(reason) == "" {
				t.Errorf("白名单里的 %q 没有写明原因", name)
			}
		}
	}
}
