package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const configRestoreConfirmPhrase = "RESTORE_CONFIG_BACKUP"

// secretMaskValue 是 schema GET 接口对所有 type=secret 字段的占位文案。
// 设计契约：
//   - 后端永远不把真实密钥（BotInternalSecret / BangumiWebhookSecret /
//     EmbyToken / TelegramBotToken 等）回传给管理端 UI；只要原始值非空，就回
//     这个 sentinel。空值仍然回空串，便于前端区分"未配置"与"已配置但被遮蔽"。
//   - schema POST 接到这个 sentinel 时，等价于"保持现状不动"，会从内存中的
//     a.cfg 拉真实值再渲染回 TOML，避免遮蔽串落地。
//   - 管理员要清空某个密钥，可以显式提交空串（"" → 写入空），或者走原始 TOML
//     编辑接口；随便填一段非 sentinel 的字符串则视为新值覆盖。
const secretMaskValue = "__TWILIGHT_SECRET_UNCHANGED__"

// isSecretField 集中判定某个 section.field 是否应在响应里被遮蔽 / 在写入时被
// preserve。configSectionDefs 已经声明了 Type=="secret"，这里直接复用，避免在
// 多处重复维护"哪些字段是密钥"的清单。
func isSecretField(sectionKey, fieldKey string) bool {
	for _, section := range configSectionDefs() {
		if section.Key != sectionKey {
			continue
		}
		for _, field := range section.Fields {
			if field.Key == fieldKey {
				return field.Type == "secret"
			}
		}
	}
	return false
}

// maskConfigSecrets 把 values 中所有非空 secret 字段替换为 secretMaskValue。
// 后端永不向管理端 UI 回传真实密钥明文——TOML GET 与 schema GET 必须同口径遮蔽，
// 否则 raw TOML 接口会成为绕过 schema 遮蔽、把全部密钥（Postgres DSN、Emby Token、
// Bot Token、BotInternalSecret 等）泄露到浏览器 DOM/缓存/历史的旁路。
func maskConfigSecrets(values map[string]map[string]any) {
	for sectionKey, fields := range values {
		for fieldKey, v := range fields {
			if !isSecretField(sectionKey, fieldKey) {
				continue
			}
			if text, ok := v.(string); ok && text != "" {
				fields[fieldKey] = secretMaskValue
			}
		}
	}
}

// 历史说明：早期存在一个 values 维度的 restoreConfigSecretSentinels（在解析后的
// map 上还原哨兵），但 raw TOML PUT 走的是文本路径（saveConfigContent 接收原始
// TOML 字符串，不经过 values 往返），因此哨兵还原必须在文本层做。文本层实现见
// restoreTOMLSecrets；schema PUT 则在 handleConfigSchemaUpdateSafe 内联还原哨兵。
// 两条写路径都已覆盖，values 版本不再需要。

// tomlSectionFieldFromLine 在按行扫描 TOML 时识别当前所处 section、以及该行是否
// 是一个 key = value 赋值。section 头形如 [Global] / ["Quoted"]；赋值行用第一个
// '=' 切分 key。返回的 isAssign 为 true 时 key 已 trim+lower。注释行 / 空行 /
// section 头返回 isAssign=false。该函数仅做词法级解析，不依赖完整 TOML parser，
// 因此可在 maskTOMLSecrets / restoreTOMLSecrets 中对"磁盘原文逐行"安全工作。
func tomlSectionFieldFromLine(line, currentSection string) (section string, key string, isAssign bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return currentSection, "", false
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		name := strings.Trim(strings.TrimSpace(strings.Trim(trimmed, "[]")), `"`)
		return name, "", false
	}
	rawKey, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return currentSection, "", false
	}
	return currentSection, strings.TrimSpace(rawKey), true
}

// maskTOMLSecrets 对磁盘原文 TOML 做行级密钥遮蔽：凡是落在某 section 下、且被
// configSectionDefs 标记为 Type=="secret" 的非空字段，整行重写为 key = "<哨兵>"。
// 与 maskConfigSecrets（作用于 values）同口径，保证 handleConfigTOMLGet 的
// content 与 raw_content 两侧都不外泄真实密钥。section 名按大小写不敏感匹配
// （isSecretField 内部精确匹配，这里先归一到 configSectionDefs 的规范名）。
func maskTOMLSecrets(content string) string {
	lines := strings.Split(content, "\n")
	section := ""
	for i, line := range lines {
		nextSection, key, isAssign := tomlSectionFieldFromLine(line, section)
		section = canonicalConfigSection(nextSection)
		if !isAssign || key == "" {
			continue
		}
		if !isSecretField(section, strings.ToLower(key)) {
			continue
		}
		// 已是空值的 secret 行无需遮蔽（区分"未配置"与"已配置但遮蔽"）。
		_, rawVal, _ := strings.Cut(line, "=")
		if tomlScalarIsEmpty(rawVal) {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + key + " = " + strconv.Quote(secretMaskValue)
	}
	return strings.Join(lines, "\n")
}

// restoreTOMLSecrets 把 PUT 回传的 TOML 里仍是 secretMaskValue 哨兵的 secret 行
// 还原为 current（内存配置 values）中的真实值。管理员未改动密钥时前端原样回传
// 哨兵，这里防止哨兵被写盘覆盖真实密钥。非哨兵值视为显式覆盖，保持不动。
func restoreTOMLSecrets(content string, current map[string]map[string]any) string {
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	section := ""
	for i, line := range lines {
		nextSection, key, isAssign := tomlSectionFieldFromLine(line, section)
		section = canonicalConfigSection(nextSection)
		if !isAssign || key == "" {
			continue
		}
		lowerKey := strings.ToLower(key)
		if !isSecretField(section, lowerKey) {
			continue
		}
		_, rawVal, _ := strings.Cut(line, "=")
		if strings.TrimSpace(rawVal) != strconv.Quote(secretMaskValue) {
			continue
		}
		realValue := ""
		if fields, ok := current[section]; ok {
			if text, ok := fields[lowerKey].(string); ok {
				realValue = text
			}
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + key + " = " + strconv.Quote(realValue)
	}
	return strings.Join(lines, "\n")
}

// canonicalConfigSection 把 TOML 里出现的 section 名归一到 configSectionDefs 使用
// 的规范 Key（大小写不敏感匹配）。无法匹配时原样返回，交给 isSecretField 自然
// 落空（非 secret）。
func canonicalConfigSection(section string) string {
	for _, def := range configSectionDefs() {
		if strings.EqualFold(def.Key, section) {
			return def.Key
		}
	}
	return section
}

// tomlScalarIsEmpty 判断 TOML 标量赋值的值部分是否为"空"（空串 "" / ” 或纯空白）。
// 用于 maskTOMLSecrets 跳过未配置的 secret 字段。
func tomlScalarIsEmpty(rawVal string) bool {
	v := strings.TrimSpace(rawVal)
	return v == "" || v == `""` || v == "''"
}

func (a *App) handleConfigTOMLPutSafe(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	// handleConfigTOMLGet 现在向前端回传遮蔽后的密钥哨兵（secretMaskValue）。
	// 管理员若未改动密钥，提交回来的 content 里这些字段仍是哨兵串；这里在写盘
	// 之前把哨兵还原为内存中的真实值，避免哨兵被当作真值覆盖 config.toml，
	// 导致 Emby Token / Bot Token / Postgres 密码 / BotInternalSecret 等被清成
	// 无效字符串。显式提交的新值（非哨兵）一律视为覆盖。
	content := restoreTOMLSecrets(stringValue(payload, "content"), configValues(*a.cfg()))
	info, status, message := a.saveConfigContent(content)
	if status != http.StatusOK {
		failWithCode(w, status, ErrConfigBackupInvalid, message)
		return
	}
	ok(w, "配置已保存并热重载", info)
}

func (a *App) handleConfigBackups(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, err := listConfigBackups(a.configBackupDir())
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrConfigBackupListFailed, "读取配置备份列表失败")
		return
	}
	ok(w, "OK", map[string]any{"backups": backups, "config_file": a.configFilePath(), "backup_dir": a.configBackupDir()})
}

func (a *App) handleConfigBackup(w http.ResponseWriter, r *http.Request, _ Params) {
	info, err := a.createConfigBackup()
	if err != nil {
		if err == store.ErrNotFound {
			failWithCode(w, http.StatusNotFound, ErrConfigFileNotFound, "配置文件不存在")
			return
		}
		failWithCode(w, http.StatusInternalServerError, ErrConfigBackupCreateFailed, "配置备份失败")
		return
	}
	ok(w, "配置备份已创建", map[string]any{"backup": info})
}

func (a *App) handleConfigBackupInspect(w http.ResponseWriter, r *http.Request, params Params) {
	backup, content, err := a.configBackupContent(params["name"])
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrConfigBackupInvalid, "配置备份无效")
		return
	}
	// 备份原文同样含真实密钥（备份是 config.toml 的字节快照）。inspect 仅用于
	// 管理端预览，必须走 maskTOMLSecrets 与 handleConfigTOMLGet 同口径遮蔽，
	// 否则"读取任意历史备份"就成了绕过 GET 遮蔽拿明文密钥的旁路。真正的恢复
	// （handleConfigRestore）读的是磁盘原文、不经此遮蔽，因此预览遮蔽不影响恢复。
	ok(w, "OK", map[string]any{"backup": backup, "content": stripProtectedAdminConfig(maskTOMLSecrets(string(content))), "config_file": a.configFilePath()})
}

func (a *App) handleConfigRestore(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	name := firstNonEmpty(stringValue(payload, "name"), stringValue(payload, "backup"))
	backup, content, err := a.configBackupContent(name)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrConfigBackupInvalid, "配置备份无效")
		return
	}
	if err := validateConfigContent(a.configFilePath(), content); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrConfigBackupVerifyFailed, "配置备份校验失败: "+err.Error())
		return
	}
	result := map[string]any{
		"operation":             "restore_config",
		"dry_run":               true,
		"requires_confirmation": true,
		"confirm":               configRestoreConfirmPhrase,
		"restored":              backup.Name,
		"backup":                backup,
		"config_file":           a.configFilePath(),
		"content_bytes":         len(content),
		"warnings": []string{
			"config restore will replace the active config file",
			"the server will create a protective config backup before applying this restore",
		},
	}
	if boolValue(payload, "dry_run", false) || boolValue(payload, "preview", false) || stringValue(payload, "confirm") != configRestoreConfirmPhrase {
		ok(w, "配置恢复预览已生成", result)
		return
	}

	info, status, message := a.saveConfigContent(string(content))
	if status != http.StatusOK {
		failWithCode(w, status, ErrConfigBackupInvalid, message)
		return
	}
	result["dry_run"] = false
	result["requires_confirmation"] = false
	result["pre_restore_backup"] = info["backup"]
	result["pre_operation_backup"] = info["backup"]
	result["reload"] = info["reload"]
	ok(w, "配置已恢复并热重载", result)
}

func (a *App) handleConfigBackupDelete(w http.ResponseWriter, r *http.Request, params Params) {
	path, err := resolveConfigBackupPath(a.configBackupDir(), params["name"])
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrConfigBackupInvalid, "配置备份无效")
		return
	}
	info, err := configBackupInfo(path)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrConfigBackupInvalid, "配置备份无效")
		return
	}
	if err := os.Remove(path); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrConfigBackupDeleteFailed, "删除配置备份失败")
		return
	}
	ok(w, "配置备份已删除", map[string]any{"backup": info})
}

func (a *App) handleConfigSchemaFull(w http.ResponseWriter, r *http.Request, _ Params) {
	values := configValues(*a.cfg())
	sections := make([]map[string]any, 0, len(configSectionDefs()))
	for _, def := range configSectionDefs() {
		fields := make([]map[string]any, 0, len(def.Fields))
		for _, field := range def.Fields {
			rawValue := values[def.Key][field.Key]
			// 密钥字段不回传明文：非空 → sentinel；空 → 空串。前端 SecretField
			// 在用户没有改动时会原样回传 sentinel，handleConfigSchemaUpdateSafe
			// 会识别并回填真实值。
			if field.Type == "secret" {
				if text, ok := rawValue.(string); ok && text != "" {
					rawValue = secretMaskValue
				} else {
					rawValue = ""
				}
			}
			item := map[string]any{
				"key":         field.Key,
				"label":       field.Label,
				"type":        field.Type,
				"description": field.Description,
				"value":       rawValue,
			}
			if len(field.Options) > 0 {
				item["options"] = field.Options
			}
			fields = append(fields, item)
		}
		sections = append(sections, map[string]any{
			"key":         def.Key,
			"title":       def.Title,
			"description": def.Description,
			"category":    def.Category,
			"fields":      fields,
		})
	}
	ok(w, "OK", map[string]any{
		"categories": []map[string]string{
			{"key": "runtime", "title": "运行"},
			{"key": "integration", "title": "集成"},
			{"key": "policy", "title": "策略"},
			{"key": "security", "title": "安全"},
			{"key": "ops", "title": "运维"},
		},
		"sections": sections,
	})
}

func (a *App) handleConfigSchemaUpdateSafe(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	rawSections, _ := payload["sections"].(map[string]any)
	values := configValues(*a.cfg())
	allowed := map[string]map[string]configFieldDef{}
	for _, section := range configSectionDefs() {
		allowed[section.Key] = map[string]configFieldDef{}
		for _, field := range section.Fields {
			allowed[section.Key][field.Key] = field
		}
	}
	for sectionKey, rawFields := range rawSections {
		fields, okFields := rawFields.(map[string]any)
		if !okFields {
			continue
		}
		for fieldKey, value := range fields {
			field, okField := allowed[sectionKey][fieldKey]
			if !okField {
				continue
			}
			if values[sectionKey] == nil {
				values[sectionKey] = map[string]any{}
			}
			// secret 字段：管理端没改 → 收到 sentinel → 用现存内存值回填，避免
			// 写入 sentinel 串污染 TOML。其它写法（空串 / 新值）一律当作显式
			// 覆盖处理，前端必须主动清空才会写入空。
			if field.Type == "secret" {
				if text, ok := value.(string); ok && text == secretMaskValue {
					value = values[sectionKey][fieldKey]
				}
			}
			values[sectionKey][fieldKey] = normalizeConfigField(field, value)
		}
	}
	info, status, message := a.saveConfigContent(renderConfigTOML(values))
	if status != http.StatusOK {
		failWithCode(w, status, ErrConfigBackupInvalid, message)
		return
	}
	ok(w, "配置已保存并热重载", info)
}

func (a *App) saveConfigContent(content string) (map[string]any, int, string) {
	if strings.TrimSpace(content) == "" {
		return nil, http.StatusBadRequest, "配置内容不能为空"
	}
	configFile := a.configFilePath()
	if err := os.MkdirAll(filepath.Dir(configFile), 0o700); err != nil {
		return nil, http.StatusInternalServerError, "创建配置目录失败"
	}
	normalizedContent, err := normalizeConfigContent(configFile, content)
	if err != nil {
		return nil, http.StatusBadRequest, "配置校验失败: " + err.Error()
	}
	content = normalizedContent
	existing, readErr := os.ReadFile(configFile)
	hadExisting := readErr == nil
	content = mergeProtectedAdminConfig(content, string(existing))
	// repo_url 与 admin_uids/admin_usernames 同属"禁止网页改写"字段：git 自动更新
	// 的来源仓库只能由运维在配置文件侧设定，防止被盗管理员会话改 origin 后触发
	// 更新实现 RCE。这里在写盘前把提交内容中的 repo_url 就地还原为磁盘原值。
	if hadExisting {
		content = restoreProtectedRepoURL(content, string(existing))
	}
	if err := validateConfigContent(configFile, []byte(content)); err != nil {
		return nil, http.StatusBadRequest, "配置校验失败: " + err.Error()
	}
	var backupInfo *store.BackupInfo
	if hadExisting {
		info, err := writeConfigBackupBytes(configFile, a.configBackupDir(), existing)
		if err != nil {
			return nil, http.StatusInternalServerError, "备份现有配置失败"
		}
		backupInfo = &info
	}
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	tmpPath := configFile + "." + stamp + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o600); err != nil {
		return nil, http.StatusInternalServerError, "保存临时配置失败"
	}
	// 不要在 Rename 之前 Remove configFile：POSIX rename(2) 已经原子替换目标，
	// 提前 Remove 反而打开了一个 "configFile 不存在" 的窗口——若此时进程被
	// SIGKILL 或磁盘忽然写满（Rename 失败 + 下一行 WriteFile rollback 也失败）
	// 系统下次启动找不到 config.toml 就直接 fail-fast。
	if err := os.Rename(tmpPath, configFile); err != nil {
		_ = os.Remove(tmpPath)
		if hadExisting {
			// 原文件 Rename 之前从未被改动，无需 rollback；这里仅做保险记录路径。
			return nil, http.StatusInternalServerError, "替换配置失败"
		}
		return nil, http.StatusInternalServerError, "替换配置失败"
	}
	reloadInfo, err := a.reloadConfig()
	if err != nil {
		if hadExisting {
			// 热重载失败回滚：先把旧内容写到 tmp，再原子 rename 替换；避免
			// 走 Remove + WriteFile 这条会再次留下"无 config.toml"窗口的旧路径。
			rollbackTmp := configFile + "." + stamp + ".rollback.tmp"
			if writeErr := os.WriteFile(rollbackTmp, existing, 0o600); writeErr == nil {
				if renameErr := os.Rename(rollbackTmp, configFile); renameErr != nil {
					_ = os.Remove(rollbackTmp)
				}
			}
			_, _ = a.reloadConfig()
		}
		return nil, http.StatusBadRequest, "配置已回滚，热重载失败: " + err.Error()
	}
	info := map[string]any{"path": configFile, "reload": reloadInfo}
	if backupInfo != nil {
		info["backup"] = *backupInfo
		info["backup_path"] = backupInfo.Path
	}
	return info, http.StatusOK, ""
}

func normalizeConfigContent(configFile, content string) (string, error) {
	dir := filepath.Dir(configFile)
	tmpPath := filepath.Join(dir, ".twilight_config_normalize_"+strconv.FormatInt(time.Now().UnixNano(), 10)+".toml")
	if err := os.WriteFile(tmpPath, []byte(content), 0o600); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)
	cfg, err := config.Load(tmpPath)
	if err != nil {
		return "", err
	}
	return renderConfigTOML(configValues(cfg)), nil
}

func (a *App) configBackupDir() string {
	dir := strings.TrimSpace(a.cfg().DatabaseBackupDir)
	if dir == "" {
		dir = filepath.Join(firstNonEmpty(a.cfg().DatabaseDir, "db"), "backups")
	}
	return filepath.Join(dir, "config")
}

func (a *App) configFilePath() string {
	return firstNonEmpty(a.cfg().ConfigFile, "config.toml")
}

func (a *App) createConfigBackup() (store.BackupInfo, error) {
	data, err := os.ReadFile(a.configFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return store.BackupInfo{}, store.ErrNotFound
		}
		return store.BackupInfo{}, err
	}
	return writeConfigBackupBytes(a.configFilePath(), a.configBackupDir(), data)
}

func (a *App) configBackupContent(name string) (store.BackupInfo, []byte, error) {
	path, err := resolveConfigBackupPath(a.configBackupDir(), name)
	if err != nil {
		return store.BackupInfo{}, nil, err
	}
	info, err := configBackupInfo(path)
	if err != nil {
		return store.BackupInfo{}, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return store.BackupInfo{}, nil, err
	}
	return info, data, nil
}

func validateConfigContent(configFile string, content []byte) error {
	if strings.TrimSpace(string(content)) == "" {
		return store.ErrNotFound
	}
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmpPath := filepath.Join(dir, ".twilight_config_validate_"+strconv.FormatInt(time.Now().UnixNano(), 10)+".toml")
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	_, err := config.Load(tmpPath)
	return err
}

func stripProtectedAdminConfig(content string) string {
	var out []string
	inAdmin := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section := strings.Trim(strings.TrimSpace(strings.Trim(trimmed, "[]")), `"`)
			inAdmin = strings.EqualFold(section, "Admin")
			if inAdmin {
				continue
			}
		}
		if inAdmin || protectedAdminConfigLine(trimmed) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

func mergeProtectedAdminConfig(submitted, existing string) string {
	clean := strings.TrimRight(stripProtectedAdminConfig(submitted), "\n")
	protected := strings.TrimSpace(extractProtectedAdminConfig(existing))
	if protected == "" {
		return clean + "\n"
	}
	return clean + "\n\n" + protected + "\n"
}

func extractProtectedAdminConfig(content string) string {
	var adminLines []string
	var rootLines []string
	inAdmin := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section := strings.Trim(strings.TrimSpace(strings.Trim(trimmed, "[]")), `"`)
			inAdmin = strings.EqualFold(section, "Admin")
			if inAdmin {
				adminLines = append(adminLines, "[Admin]")
			}
			continue
		}
		if inAdmin {
			adminLines = append(adminLines, line)
			continue
		}
		if protectedAdminConfigLine(trimmed) {
			rootLines = append(rootLines, line)
		}
	}
	if len(adminLines) > 0 {
		return strings.TrimRight(strings.Join(adminLines, "\n"), "\n")
	}
	if len(rootLines) > 0 {
		return strings.TrimRight(strings.Join(rootLines, "\n"), "\n")
	}
	return ""
}

func protectedAdminConfigLine(trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	key, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "admin_uids", "admin_usernames":
		return true
	default:
		return false
	}
}

// restoreProtectedRepoURL 把提交 TOML 里 [SystemUpdate].repo_url 的值就地还原为
// 磁盘原值（existing），防止经网页配置接口改写 git 自动更新的来源仓库。
//
// 威胁模型：repo_url 决定 git 自动更新 pull 的 origin。若允许网页改写，被盗的
// 管理员会话可把 origin 指向攻击者 fork，再触发更新即可在服务器上 RCE。该字段
// 只能由运维在配置文件 / 环境变量侧设定。
//
// 为什么用"就地替换值"而非 [Admin] 那种"整段剥离 + 末尾追加"：repo_url 位于
// [SystemUpdate] 段内，该段还有 branch / restart_services 等普通字段。若整段剥离
// 再追加一个只含 repo_url 的 [SystemUpdate]，会产生重复 section 头——TOML 规范
// 不允许同名 table 重复定义，直接解析失败。就地替换与 restoreTOMLSecrets 同构，
// 不改变文档结构。
//
// 行为：仅当提交内容在 [SystemUpdate] 段内出现 repo_url 行时才替换其值为磁盘原值；
// 提交侧删除该行（清空 repo_url、停用自动更新）属于合法操作，不阻止。
func restoreProtectedRepoURL(content, existing string) string {
	if content == "" {
		return content
	}
	diskRepoURL, hasDisk := systemUpdateRepoURL(existing)
	if !hasDisk {
		return content
	}
	lines := strings.Split(content, "\n")
	section := ""
	for i, line := range lines {
		nextSection, key, isAssign := tomlSectionFieldFromLine(line, section)
		section = canonicalConfigSection(nextSection)
		if !isAssign || !strings.EqualFold(section, "SystemUpdate") || !strings.EqualFold(key, "repo_url") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + key + " = " + strconv.Quote(diskRepoURL)
	}
	return strings.Join(lines, "\n")
}

// systemUpdateRepoURL 从 TOML 文本里抽取 [SystemUpdate].repo_url 的字符串值。
func systemUpdateRepoURL(content string) (string, bool) {
	section := ""
	for _, line := range strings.Split(content, "\n") {
		nextSection, key, isAssign := tomlSectionFieldFromLine(line, section)
		section = canonicalConfigSection(nextSection)
		if !isAssign || !strings.EqualFold(section, "SystemUpdate") || !strings.EqualFold(key, "repo_url") {
			continue
		}
		_, rawVal, _ := strings.Cut(line, "=")
		if v, err := strconv.Unquote(strings.TrimSpace(rawVal)); err == nil {
			return v, true
		}
		return strings.Trim(strings.TrimSpace(rawVal), `"`), true
	}
	return "", false
}

func writeConfigBackupBytes(configFile, backupDir string, content []byte) (store.BackupInfo, error) {
	if len(content) == 0 {
		return store.BackupInfo{}, store.ErrNotFound
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return store.BackupInfo{}, err
	}
	now := time.Now().UTC()
	base := strings.TrimSuffix(filepath.Base(configFile), filepath.Ext(configFile))
	if base == "" || base == "." {
		base = "config"
	}
	name := base + "_" + now.Format("20060102_150405") + "_" + strconv.FormatInt(now.UnixNano()%1e9, 10) + ".toml"
	path := filepath.Join(backupDir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return store.BackupInfo{}, err
	}
	return store.BackupInfo{Name: name, Path: path, Size: int64(len(content)), CreatedAt: now.Unix()}, nil
}

func listConfigBackups(dir string) ([]store.BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []store.BackupInfo{}, nil
		}
		return nil, err
	}
	backups := make([]store.BackupInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.ToLower(filepath.Ext(entry.Name())) != ".toml" {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		backups = append(backups, store.BackupInfo{Name: entry.Name(), Path: filepath.Join(dir, entry.Name()), Size: info.Size(), CreatedAt: info.ModTime().Unix()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt > backups[j].CreatedAt })
	return backups, nil
}

func resolveConfigBackupPath(dir, name string) (string, error) {
	target, err := ResolveLeafFile(dir, name, "toml")
	if err != nil {
		return "", store.ErrNotFound
	}
	return target, nil
}

func configBackupInfo(path string) (store.BackupInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return store.BackupInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return store.BackupInfo{}, store.ErrNotFound
	}
	return store.BackupInfo{Name: filepath.Base(path), Path: path, Size: info.Size(), CreatedAt: info.ModTime().Unix()}, nil
}

type configSectionDef struct {
	Key         string
	Title       string
	Description string
	Category    string
	Fields      []configFieldDef
}

type configFieldDef struct {
	Key         string
	Label       string
	Type        string
	Description string
	Options     []map[string]any
}

func regCodeRandomAlgorithmOptions() []map[string]any {
	return []map[string]any{
		{"label": "base32-20（默认推荐，易抄写）", "value": "base32-20"},
		{"label": "base32-16（短码，易抄写）", "value": "base32-16"},
		{"label": "base32-24（高强度，易抄写）", "value": "base32-24"},
		{"label": "base32-32（超高强度，易抄写）", "value": "base32-32"},
		{"label": "hex20（旧默认兼容）", "value": "hex20"},
		{"label": "hex32（128-bit 十六进制）", "value": "hex32"},
		{"label": "hex40（长十六进制）", "value": "hex40"},
		{"label": "alnum-16（字母数字短码）", "value": "alnum-16"},
		{"label": "alnum-24（字母数字高强度）", "value": "alnum-24"},
		{"label": "alnum-32（字母数字超高强度）", "value": "alnum-32"},
		{"label": "urlsafe-24（URL 安全字符）", "value": "urlsafe-24"},
		{"label": "urlsafe-32（URL 安全长码）", "value": "urlsafe-32"},
		{"label": "digits-12（纯数字，便于口述）", "value": "digits-12"},
		{"label": "digits-16（纯数字增强）", "value": "digits-16"},
		{"label": "symbols-16（含特殊字符）", "value": "symbols-16"},
		{"label": "symbols-24（含特殊字符高强度）", "value": "symbols-24"},
		{"label": "uuid（UUID v4 风格）", "value": "uuid"},
		{"label": "legacy-sha1（旧风格 40 位 hex）", "value": "legacy-sha1"},
	}
}

func inviteCodeRandomAlgorithmOptions() []map[string]any {
	options := []map[string]any{{"label": "hex10（邀请码旧默认）", "value": "hex10"}}
	return append(options, regCodeRandomAlgorithmOptions()...)
}

func regcodeDecoyActionOptions() []map[string]any {
	return []map[string]any{
		{"label": "仅记录违规审计", "value": "log_only"},
		{"label": "禁用 Web 账号", "value": "disable_user"},
		{"label": "禁用 Emby 账号", "value": "disable_emby"},
	}
}

const telegramGroupUserPanelTemplateDescription = "自定义 /twguser 群组用户面板文本，支持换行；留空使用内置模板。安全限制：不会提供邮箱、Emby ID、密码、Token 或服务器线路占位符。" +
	"占位符：{server_name}=站点名称；{username}=Web 用户名；{uid}=用户 UID；{role}=角色名称；{role_id}=角色数字；{is_admin}=是否管理员；{is_protected}=是否受保护；" +
	"{web_status}=Web 账号启用/禁用；{web_active}=Web 是否启用；{expire_status}=到期状态摘要；{expired_at}=具体到期时间；{register_time}=注册时间；{created_at}=创建时间；" +
	"{telegram_status}=Telegram 绑定摘要；{telegram_username}=Telegram 用户名（无用户名显示 None）；{telegram_userid}=Telegram 用户 ID；{emby_status}=本地 Emby 绑定摘要（含用户名）；{emby_bound_status}=本地 Emby 绑定状态（不含用户名）；{emby_bound}=是否已绑定 Emby；{emby_username}=本地 Emby 用户名；{emby_unbind_allowed}=是否允许用户自助解绑 Emby；" +
	"{registration_source}=注册/授权来源；{registration_code}=注册/授权卡码；{pending_emby}=是否待补建 Emby；{pending_emby_days}=待补建授权天数；" +
	"{emby_remote_block}=完整 Emby 远端信息块；{emby_remote_status}=远端查询状态；{emby_remote_username}=远端用户名；{emby_remote_enabled}=远端启用/禁用；{emby_remote_role}=远端权限；{emby_remote_hidden}=远端是否隐藏；{emby_last_activity}=远端最近活动；" +
	"{bgm_mode}=BGM 同步开关；{bgm_token_status}=BGM Token 是否配置；{bgm_sync_status}=BGM 同步可用状态；{api_key_status}=旧 API Key 开关；{panel_ttl}=面板有效期；{panel_ttl_seconds}=面板有效秒数。"

func configSectionDefs() []configSectionDef {
	selectDriver := []map[string]any{{"label": "PostgreSQL（推荐）", "value": "postgres"}, {"label": "Go JSON 文件（兼容）", "value": "json"}}
	selectUpdate := []map[string]any{{"label": "按间隔", "value": "interval"}, {"label": "每日固定时间", "value": "daily"}, {"label": "手动", "value": "manual"}}
	selectRegCodeRandom := regCodeRandomAlgorithmOptions()
	selectInviteCodeRandom := inviteCodeRandomAlgorithmOptions()
	selectRegcodeDecoyAction := regcodeDecoyActionOptions()
	return []configSectionDef{
		{Key: "Global", Title: "全局", Description: "基础运行参数", Category: "runtime", Fields: []configFieldDef{
			{Key: "server_name", Label: "服务器名称", Type: "string", Description: "前端展示的站点或服务器名称"},
			{Key: "server_icon", Label: "服务器图标", Type: "string", Description: "HTTPS 图片 URL 或本地图片路径；留空使用内置图标"},
			{Key: "auth_background_url", Label: "认证页背景图", Type: "string", Description: "登录/注册页背景图片 HTTPS URL；留空使用默认渐变背景"},
			{Key: "databases_dir", Label: "数据目录", Type: "string", Description: "JSON 状态、备份和迁移文件目录"},
			{Key: "log_level", Label: "日志等级", Type: "select", Description: "后端运行日志等级；兼容旧值 10/20/30/40", Options: []map[string]any{{"label": "DEBUG", "value": "debug"}, {"label": "INFO", "value": "info"}, {"label": "WARN", "value": "warn"}, {"label": "ERROR", "value": "error"}}},
			{Key: "runtime_log_limit", Label: "实时日志保留行数", Type: "int", Description: "后台实时日志缓冲区行数，热重载生效"},
			{Key: "redis_url", Label: "Redis URL", Type: "secret", Description: "会话和限流 Redis，留空使用进程内存"},
			{Key: "telegram_mode", Label: "启用 Telegram", Type: "bool", Description: "启用 Bot 和 Telegram 绑定能力"},
			{Key: "force_bind_telegram", Label: "强制绑定 Telegram", Type: "bool", Description: "登录或注册流程要求绑定 Telegram"},
			{Key: "tmdb_api_key", Label: "TMDB API Key", Type: "secret", Description: "媒体搜索使用的 TMDB Key"},
			{Key: "tmdb_api_url", Label: "TMDB API URL", Type: "string", Description: "TMDB API 基础地址"},
			{Key: "tmdb_image_url", Label: "TMDB 图片 URL", Type: "string", Description: "TMDB 图片 CDN 地址"},
			{Key: "bangumi_token", Label: "Bangumi Token", Type: "secret", Description: "Bangumi 全局 Token"},
			{Key: "bangumi_api_url", Label: "Bangumi API URL", Type: "string", Description: "Bangumi API 基础地址"},
		}},
		{Key: "Database", Title: "数据库", Description: "JSON/PostgreSQL 存储和备份配置", Category: "ops", Fields: []configFieldDef{
			{Key: "driver", Label: "存储后端", Type: "select", Description: "可视化配置仅提供 PostgreSQL 与 Go JSON；SQLite 已禁用", Options: selectDriver},
			{Key: "state_file", Label: "JSON 状态文件", Type: "string", Description: "Go JSON 状态文件路径，留空使用数据目录中的 twilight_go_state.json"},
			{Key: "backup_dir", Label: "备份目录", Type: "string", Description: "数据库快照备份目录"},
			{Key: "migration_panel_enabled", Label: "启用数据库迁移", Type: "bool", Description: "开启后显示数据库迁移面板并允许管理员调用迁移 API"},
			{Key: "url", Label: "PostgreSQL URL", Type: "secret", Description: "完整 postgres:// 连接串，优先级高于分项配置"},
			{Key: "postgres_host", Label: "PG 主机", Type: "string", Description: "PostgreSQL 主机"},
			{Key: "postgres_port", Label: "PG 端口", Type: "int", Description: "PostgreSQL 端口"},
			{Key: "postgres_user", Label: "PG 用户", Type: "string", Description: "PostgreSQL 用户"},
			{Key: "postgres_password", Label: "PG 密码", Type: "secret", Description: "PostgreSQL 密码"},
			{Key: "postgres_database", Label: "PG 数据库", Type: "string", Description: "PostgreSQL 数据库名"},
			{Key: "postgres_sslmode", Label: "PG SSLMode", Type: "string", Description: "disable/require/verify-full 等 pgx 支持值"},
			{Key: "postgres_max_open_conns", Label: "PG 最大连接", Type: "int", Description: "PostgreSQL 最大打开连接数"},
			{Key: "postgres_max_idle_conns", Label: "PG 空闲连接", Type: "int", Description: "PostgreSQL 最大空闲连接数"},
		}},
		{Key: "Emby", Title: "Emby", Description: "Emby 连接和线路", Category: "integration", Fields: []configFieldDef{
			{Key: "emby_url", Label: "Emby URL", Type: "string", Description: "后端访问的 Emby/Jellyfin 地址"},
			{Key: "emby_token", Label: "Emby Token", Type: "secret", Description: "Emby API Key"},
			{Key: "emby_username", Label: "管理员用户名", Type: "string", Description: "备用鉴权用户名"},
			{Key: "emby_password", Label: "管理员密码", Type: "secret", Description: "备用鉴权密码"},
			{Key: "emby_url_list", Label: "普通线路", Type: "list", Description: "格式：名称 : URL"},
			{Key: "emby_url_list_for_whitelist", Label: "白名单线路", Type: "list", Description: "管理员和白名单用户可见线路"},
		}},
		{Key: "Telegram", Title: "Telegram", Description: "Bot、订阅校验和群组管理", Category: "integration", Fields: []configFieldDef{
			{Key: "telegram_api_url", Label: "Bot API URL", Type: "string", Description: "Telegram Bot API 基础地址"},
			{Key: "bot_token", Label: "Bot Token", Type: "secret", Description: "Telegram Bot Token"},
			{Key: "admin_id", Label: "管理员 Telegram ID", Type: "list", Description: "Bot 管理员 ID 列表"},
			{Key: "group_id", Label: "群组 ID", Type: "list", Description: "Bot 管理、强制绑定检查和巡检群组"},
			{Key: "force_bind_group", Label: "强制群组绑定检查", Type: "bool", Description: "用户在 Bot 中确认绑定码时，必须已加入配置的群组"},
			{Key: "channel_id", Label: "频道 ID", Type: "list", Description: "Bot 推送和强制绑定检查的频道"},
			{Key: "force_bind_channel", Label: "强制频道绑定检查", Type: "bool", Description: "用户在 Bot 中确认绑定码时，必须已加入配置的频道"},
			{Key: "enable_tg_panel", Label: "启用 Bot 面板", Type: "bool", Description: "启用更多 Bot 查询命令和管理查询入口"},
			{Key: "group_user_panel_template", Label: "/twguser 面板模板", Type: "textarea", Description: telegramGroupUserPanelTemplateDescription},
			{Key: "require_group_membership", Label: "强制群成员", Type: "bool", Description: "巡检发现退群时禁用本地或 Emby"},
			{Key: "ban_on_leave", Label: "退群封禁", Type: "bool", Description: "退群后在群组永久封禁"},
			{Key: "auto_enable_rejoined", Label: "回群自动启用", Type: "bool", Description: "退群后重新加入且未过期时，巡检自动重新启用 Web 账号；Emby 需单独启用，关闭时进入人工复核"},
			{Key: "group_check_concurrency", Label: "巡检并发", Type: "int", Description: "getChatMember 并发数"},
			{Key: "group_action_concurrency", Label: "写操作并发", Type: "int", Description: "踢出、封禁等动作并发数"},
			{Key: "bot_start_text", Label: "Bot 开始文案", Type: "textarea", Description: "覆盖私聊 /start 文案，支持换行"},
			{Key: "bot_group_start_text", Label: "群聊开始文案", Type: "textarea", Description: "覆盖群聊 /start 提示，支持换行"},
			{Key: "bot_start_title", Label: "开始标题", Type: "string", Description: "内置 /start 文案的标题"},
			{Key: "bot_start_intro", Label: "开始简介", Type: "textarea", Description: "内置 /start 文案的简介，支持换行"},
			{Key: "bot_bind_prompt_text", Label: "绑定提示文案", Type: "textarea", Description: "/bind 无参数时的提示文本，支持换行"},
			{Key: "bot_help_text", Label: "用户帮助文案", Type: "textarea", Description: "覆盖 /help 和 /twihelp 文案，支持换行"},
			{Key: "bot_admin_help_text", Label: "管理员帮助文案", Type: "textarea", Description: "覆盖 /twishelp 文案，支持换行"},
			{Key: "bot_help_header", Label: "帮助页前缀", Type: "textarea", Description: "追加到内置用户帮助顶部，支持换行"},
			{Key: "bot_help_footer", Label: "帮助页后缀", Type: "textarea", Description: "追加到内置用户帮助底部，支持换行"},
			{Key: "bot_about", Label: "Bot 关于文案", Type: "textarea", Description: "/about 的服务说明，支持换行"},
			{Key: "bot_custom_commands", Label: "Bot 自定义指令回复", Type: "command_map", Description: "自定义 /command 与回复内容的映射，回复支持换行"},
		}},
		{Key: "SAR", Title: "注册/邀请", Description: "注册、卡码、邀请树和求片", Category: "policy", Fields: []configFieldDef{
			{Key: "register_mode", Label: "开放注册", Type: "bool", Description: "是否允许注册系统账号"},
			{Key: "register_code_limit", Label: "注册必须用码", Type: "bool", Description: "注册时必须提供注册码"},
			{Key: "allow_pending_register", Label: "允许待补建", Type: "bool", Description: "允许无 Emby 账号先注册"},
			{Key: "emby_direct_register_enabled", Label: "Emby 自助注册", Type: "bool", Description: "用户可自助创建 Emby 账号"},
			{Key: "emby_direct_register_days", Label: "自助注册天数", Type: "int", Description: "Emby 自助注册默认有效期"},
			{Key: "user_limit", Label: "系统用户上限", Type: "int", Description: "-1 表示不限；达到上限后禁止新注册"},
			{Key: "regcode_format", Label: "默认卡码格式（兼容旧配置）", Type: "string", Description: "注册/续期/白名单未配置专用格式时使用；支持 {random}/{type}/{days}/{index}/{validity}/{limit}"},
			{Key: "register_code_format", Label: "注册码专用格式", Type: "string", Description: "仅 type=1 注册码使用；留空则回退到 regcode_format"},
			{Key: "renew_code_format", Label: "续期码专用格式", Type: "string", Description: "type=2 续期码使用；留空则回退到 regcode_format，邀请中心专属续期码留空时保持 REN-{random} 旧格式"},
			{Key: "invite_code_format", Label: "邀请码格式", Type: "string", Description: "邀请树邀请码使用；默认 INV{random} 兼容旧码风格，支持 {random}/{type}/{days}/{index}"},
			{Key: "regcode_random_algorithm", Label: "默认注册码随机算法", Type: "select", Description: "创建注册码未指定算法时使用；包含易抄写、URL 安全和特殊字符预设", Options: selectRegCodeRandom},
			{Key: "invite_code_random_algorithm", Label: "默认邀请码随机算法", Type: "select", Description: "生成邀请码时使用；独立于注册码随机算法，hex10 为旧默认", Options: selectInviteCodeRandom},
			{Key: "regcode_decoy_action", Label: "诱饵/指名码误用动作", Type: "select", Description: "登录用户触碰诱饵注册码或误用指名码时执行的动作；所有选项都会写入违规审计", Options: selectRegcodeDecoyAction},
			{Key: "emby_user_limit", Label: "Emby 用户上限", Type: "int", Description: "-1 表示不限"},
			{Key: "media_request_enabled", Label: "启用求片", Type: "bool", Description: "允许用户提交媒体请求"},
			{Key: "max_concurrent_requests_per_user", Label: "每用户并发求片", Type: "int", Description: "-1 表示不限"},
			{Key: "max_concurrent_requests_global", Label: "全局并发求片", Type: "int", Description: "-1 表示不限；达到上限后所有用户的新求片都会被拒绝"},
			{Key: "invite_enabled", Label: "启用邀请树", Type: "bool", Description: "允许用户生成邀请码或续期码"},
			{Key: "invite_limit", Label: "邀请码数量", Type: "int", Description: "每个用户可持有的邀请码数量"},
			{Key: "invite_root_user_limit", Label: "根邀请上限", Type: "int", Description: "单棵邀请树最多成功邀请人数"},
			{Key: "invite_max_depth", Label: "邀请最大深度", Type: "int", Description: "邀请关系最大层级"},
			{Key: "invite_require_emby", Label: "邀请要求 Emby", Type: "bool", Description: "已绑定 Emby 才能邀请"},
			{Key: "invite_code_default_days", Label: "邀请码默认天数", Type: "int", Description: "新邀请码默认续期或开通天数"},
			{Key: "permanent_invite_max_days", Label: "永久码最大天数", Type: "int", Description: "永久邀请可授予的最大天数"},
			{Key: "auto_cleanup_no_emby", Label: "清理无 Emby 用户", Type: "bool", Description: "定期清理长期未绑定 Emby 的用户"},
			{Key: "auto_cleanup_no_emby_days", Label: "无 Emby 清理天数", Type: "int", Description: "超过该天数后可清理"},
			{Key: "auto_cleanup_pending_emby", Label: "清理 Emby 开通资格", Type: "bool", Description: "定期收回长期未使用的 Emby 开通资格，不删除 Web 账号"},
			{Key: "auto_cleanup_pending_emby_days", Label: "资格清理天数", Type: "int", Description: "超过该天数仍未创建 Emby 时收回资格"},
			{Key: "signin_enabled", Label: "启用签到", Type: "bool", Description: "允许用户进入签到页面并领取每日积分"},
			{Key: "currency_name", Label: "积分名称", Type: "string", Description: "签到积分在前端展示的名称"},
			{Key: "daily_min", Label: "每日最少积分", Type: "int", Description: "单次签到可获得的最少积分"},
			{Key: "daily_max", Label: "每日最多积分", Type: "int", Description: "单次签到可获得的最多积分"},
			{Key: "streak_bonus_enabled", Label: "启用连签奖励", Type: "bool", Description: "按连续签到天数发放额外奖励"},
			{Key: "streak_bonus_days", Label: "连签奖励天数", Type: "list", Description: "数字列表，与连签奖励积分一一对应"},
			{Key: "streak_bonus_points", Label: "连签奖励积分", Type: "list", Description: "数字列表，与连签奖励天数一一对应"},
			{Key: "reset_after_miss", Label: "漏签重置连签", Type: "bool", Description: "漏签后是否从 1 天重新计算连续签到"},
			{Key: "signin_renewal_enabled", Label: "启用积分续期", Type: "bool", Description: "允许用户用签到积分兑换账号续期；关闭时前端不显示兑换入口"},
			{Key: "signin_renewal_cost", Label: "续期消耗积分", Type: "int", Description: "每次积分续期需要消耗的积分数，必须大于 0"},
			{Key: "signin_renewal_days", Label: "续期天数", Type: "int", Description: "每次积分续期增加的天数，必须大于 0"},
		}},
		{Key: "DeviceLimit", Title: "设备限制", Description: "设备和并发播放限制", Category: "policy", Fields: []configFieldDef{
			{Key: "device_limit_enabled", Label: "启用设备限制", Type: "bool", Description: "限制设备数量"},
			{Key: "max_devices", Label: "最大设备数", Type: "int", Description: "每个用户最大设备数"},
			{Key: "max_streams", Label: "最大播放流", Type: "int", Description: "每个用户最大并发流"},
		}},
		{Key: "RateLimit", Title: "限流策略", Description: "接口请求频率限制；0 或负数表示不限制", Category: "security", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用后端限流", Type: "bool", Description: "关闭后不执行 Go 后端限流"},
			{Key: "global_per_minute", Label: "全局每分钟", Type: "int", Description: "同一 IP 每分钟总请求数"},
			{Key: "login_per_minute", Label: "登录每分钟", Type: "int", Description: "同一 IP 登录请求数"},
			{Key: "login_user_per_5m", Label: "单账号登录每 5 分钟", Type: "int", Description: "同一账号（用户名或 API Key）登录请求数"},
			{Key: "register_per_10m", Label: "注册每 10 分钟", Type: "int", Description: "同一 IP 注册请求数"},
			{Key: "forgot_password_ip_per_10m", Label: "找回密码 IP 每 10 分钟", Type: "int", Description: "同一 IP 找回密码请求数"},
			{Key: "forgot_password_user_per_30m", Label: "找回密码账号每 30 分钟", Type: "int", Description: "同一 Emby 用户名找回密码请求数"},
			{Key: "email_code_ip_per_10m", Label: "邮箱发码 IP 每 10 分钟", Type: "int", Description: "同一 IP 获取邮箱验证码请求数"},
			{Key: "email_code_addr_per_10m", Label: "邮箱发码地址每 10 分钟", Type: "int", Description: "同一收件邮箱获取验证码请求数"},
			{Key: "email_code_uid_per_10m", Label: "邮箱发码单账号每 10 分钟", Type: "int", Description: "同一登录账号获取验证码请求数；防止轮换收件邮箱滥刷"},
			{Key: "upload_per_minute", Label: "上传每分钟", Type: "int", Description: "同一用户上传请求数"},
			{Key: "admin_icon_per_minute", Label: "站点图标每分钟", Type: "int", Description: "同一管理员上传站点图标请求数"},
			{Key: "api_key_default_per_minute", Label: "API Key 默认每分钟", Type: "int", Description: "API Key 未单独设置时的默认额度"},
		}},
		{Key: "API", Title: "API", Description: "监听、跨域、上传和 Cookie", Category: "runtime", Fields: []configFieldDef{
			{Key: "host", Label: "监听地址", Type: "string", Description: "修改后需要重启监听器"},
			{Key: "port", Label: "监听端口", Type: "int", Description: "修改后需要重启监听器"},
			{Key: "cors_origins", Label: "CORS Origins", Type: "list", Description: "允许的前端 Origin；生产环境必须显式填写 HTTPS 域名"},
			{Key: "upload_folder", Label: "上传目录", Type: "string", Description: "头像或背景上传目录"},
			{Key: "max_upload_size", Label: "上传上限", Type: "int", Description: "单文件最大字节数"},
			{Key: "session_cookie_name", Label: "Cookie 名称", Type: "string", Description: "会话 Cookie 名称"},
			{Key: "session_cookie_secure", Label: "Secure Cookie", Type: "bool", Description: "HTTPS 部署应开启"},
			{Key: "session_cookie_samesite", Label: "SameSite", Type: "string", Description: "lax/strict/none"},
			{Key: "session_cookie_domain", Label: "Cookie Domain", Type: "string", Description: "跨子域共享会话时填写父域，例如 .example.com；单域部署留空"},
			{Key: "trust_proxy_headers", Label: "信任代理 IP", Type: "bool", Description: "仅在可信反代后开启"},
			{Key: "trusted_proxy_cidrs", Label: "可信反代 CIDR", Type: "list", Description: "上游反代的 IP / CIDR；启用 trust_proxy_headers 时必须配置，否则任何客户端都可伪造 X-Forwarded-For"},
		}},
		{Key: "Security", Title: "安全", Description: "内部密钥和安全开关", Category: "ops", Fields: []configFieldDef{
			{Key: "forgot_password_enabled", Label: "启用找回密码", Type: "bool", Description: "总开关：关闭后所有找回密码途径均不可用"},
			{Key: "forgot_password_emby_enabled", Label: "Emby 找回密码", Type: "bool", Description: "允许通过 Emby 账号验证重置 Web 面板密码；依赖上图总开关"},
			{Key: "forgot_password_email_enabled", Label: "邮箱找回密码", Type: "bool", Description: "允许通过绑定邮箱验证码重置 Web 面板密码；依赖上图总开关"},
			{Key: "bot_internal_secret", Label: "Bot 内部密钥", Type: "secret", Description: "外部更新回调共享密钥"},
		}},
		{Key: "Scheduler", Title: "调度器", Description: "后台任务计划", Category: "ops", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用调度", Type: "bool", Description: "启用后台任务"},
			{Key: "tick_interval_seconds", Label: "调度间隔(秒)", Type: "int", Description: "调度器主循环检查间隔，默认 30 秒，范围 10-300"},
			{Key: "expired_check_time", Label: "过期检查", Type: "string", Description: "每日 HH:MM"},
			{Key: "expiring_check_time", Label: "到期提醒检查", Type: "string", Description: "每日 HH:MM"},
			{Key: "daily_stats_time", Label: "每日统计", Type: "string", Description: "每日 HH:MM"},
			{Key: "session_cleanup_interval", Label: "会话检查间隔", Type: "int", Description: "小时"},
			{Key: "cleanup_no_emby_time", Label: "无Emby清理时间", Type: "string", Description: "每日 HH:MM，清理注册后长期未绑定 Emby 的账号"},
			{Key: "cleanup_pending_emby_time", Label: "未使用资格清理时间", Type: "string", Description: "每日 HH:MM，收回长期未使用的 Emby 开通资格"},
			{Key: "cleanup_unused_uploads_time", Label: "未使用上传清理时间", Type: "string", Description: "每日 HH:MM，清理未被引用的历史上传文件"},
			{Key: "cleanup_audit_logs_time", Label: "审计日志清理时间", Type: "string", Description: "每日 HH:MM，按保留策略清理过期审计日志"},
		}},
		{Key: "SystemUpdate", Title: "自动更新", Description: "Git 拉取和服务重启", Category: "ops", Fields: []configFieldDef{
			{Key: "auto_update_enabled", Label: "启用自动更新", Type: "bool", Description: "允许调度任务自动拉取更新"},
			{Key: "repo_url", Label: "仓库 URL", Type: "string", Description: "仅支持无凭据 HTTPS 仓库"},
			{Key: "branch", Label: "分支", Type: "string", Description: "目标分支"},
			{Key: "restart_services", Label: "重启服务", Type: "bool", Description: "更新后重启 systemd 服务"},
			{Key: "auto_update_trigger_type", Label: "触发方式", Type: "select", Description: "按间隔或每日固定时间", Options: selectUpdate},
			{Key: "auto_update_interval_hours", Label: "更新间隔", Type: "int", Description: "小时"},
			{Key: "auto_update_time", Label: "更新时间", Type: "string", Description: "每日 HH:MM"},
		}},
		{Key: "Notification", Title: "通知", Description: "用户通知策略", Category: "policy", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用通知", Type: "bool", Description: "允许系统通知"},
			{Key: "expiry_remind_days", Label: "到期提醒天数", Type: "int", Description: "提前多少天提醒"},
		}},
		{Key: "BangumiSync", Title: "Bangumi 同步", Description: "Bangumi webhook 和收藏策略", Category: "integration", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用同步", Type: "bool", Description: "启用 Bangumi 同步"},
			{Key: "webhook_secret", Label: "Webhook 密钥", Type: "secret", Description: "Bangumi webhook 校验密钥"},
		}},
		{Key: "Ticket", Title: "工单系统", Description: "用户提交工单与管理员处理", Category: "policy", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用工单系统", Type: "bool", Description: "开启后用户可提交工单，管理员可在后台管理"},
			{Key: "types", Label: "工单类型", Type: "list", Description: "自定义工单类型列表，默认：bug, feature, question, account, other"},
		}},
		{Key: "AuditLog", Title: "操作审计", Description: "审计日志记录与自动清理策略", Category: "security", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用审计日志", Type: "bool", Description: "关闭后不再记录新的审计日志；已有日志不受影响"},
			{Key: "auto_cleanup_enabled", Label: "启用自动清理", Type: "bool", Description: "开启后调度任务按下方策略自动清理过期审计日志；默认关闭"},
			{Key: "retention_days", Label: "保留天数", Type: "int", Description: "超出此天数的日志将被清理；0=不限天数"},
			{Key: "max_entries", Label: "最大保留条数", Type: "int", Description: "超出此条数时保留最新 N 条；0=不限条数"},
			{Key: "preserve_admin", Label: "保留管理员操作", Type: "bool", Description: "按天数清理时跳过管理员 (category=admin) 的操作日志"},
			{Key: "cleanup_check_time", Label: "清理检查时间", Type: "string", Description: "每日执行清理检查的时间 HH:MM"},
		}},
		{Key: "Email", Title: "邮箱验证", Description: "SMTP 发信、验证码与强制绑定策略", Category: "integration", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用邮箱验证", Type: "bool", Description: "总开关：关闭后所有发码 / 邮箱找回 / 强制绑定均降级，前端隐藏入口"},
			{Key: "smtp_host", Label: "SMTP 主机", Type: "string", Description: "发信服务器地址，如 smtp.gmail.com"},
			{Key: "smtp_port", Label: "SMTP 端口", Type: "int", Description: "常见：465（SSL）、587（STARTTLS）、25（明文）"},
			{Key: "smtp_username", Label: "SMTP 用户名", Type: "string", Description: "登录发信服务器的用户名，通常是发件邮箱"},
			{Key: "smtp_password", Label: "SMTP 密码", Type: "secret", Description: "发信服务器密码或授权码"},
			{Key: "smtp_encryption", Label: "加密方式", Type: "select", Description: "ssl=隐式 TLS（465）；starttls=显式 TLS（587）；none=明文（25，不推荐）", Options: []map[string]any{{"label": "STARTTLS（587，推荐）", "value": "starttls"}, {"label": "SSL/TLS（465）", "value": "ssl"}, {"label": "不加密（25）", "value": "none"}}},
			{Key: "smtp_from_address", Label: "发件地址", Type: "string", Description: "From 邮箱地址；留空则使用 SMTP 用户名"},
			{Key: "smtp_from_name", Label: "发件人名称", Type: "string", Description: "From 显示名称；留空则使用站点名称"},
			{Key: "smtp_timeout_seconds", Label: "发信超时", Type: "int", Description: "单次发信超时秒数"},
			{Key: "email_validation_mode", Label: "邮箱验证模式", Type: "select", Description: "白名单=仅允许配置的域名，黑名单=禁止配置的域名，空白=不限制", Options: []map[string]any{{"label": "不限制", "value": ""}, {"label": "白名单模式", "value": "whitelist"}, {"label": "黑名单模式", "value": "blacklist"}}},
			{Key: "email_whitelist", Label: "邮箱白名单", Type: "list", Description: "白名单模式下仅允许的邮箱域名（如 gmail.com, outlook.com）"},
			{Key: "email_blacklist", Label: "邮箱黑名单", Type: "list", Description: "黑名单模式下禁止的邮箱域名（如 mailinator.com, 10minutemail.com），白名单优先"},
			{Key: "force_bind", Label: "强制绑定邮箱", Type: "bool", Description: "开启后普通用户和白名单进入仪表盘必须先验证邮箱；管理员仅提示。改系统/Emby 密码也需邮箱验证码"},
			{Key: "code_length", Label: "验证码长度", Type: "int", Description: "验证码位数，建议 4-8"},
			{Key: "code_type", Label: "验证码类型", Type: "select", Description: "numeric=纯数字；alphanumeric=字母数字混合", Options: []map[string]any{{"label": "纯数字", "value": "numeric"}, {"label": "字母数字混合", "value": "alphanumeric"}}},
			{Key: "code_ttl_minutes", Label: "验证码有效期", Type: "int", Description: "验证码有效分钟数"},
			{Key: "resend_cooldown_seconds", Label: "重发冷却", Type: "int", Description: "同一邮箱两次发码的最小间隔秒数"},
			{Key: "max_attempts", Label: "最大尝试次数", Type: "int", Description: "单个验证码允许的错误尝试次数，超过即失效"},
			{Key: "subject_template", Label: "邮件标题模板", Type: "string", Description: "占位符：{site} 站点名、{code} 验证码、{ttl} 有效分钟数"},
			{Key: "body_template", Label: "邮件正文模板", Type: "textarea", Description: "占位符：{site} 站点名、{code} 验证码、{ttl} 有效分钟数；支持换行"},
		}},
	}
}

func configValues(cfg config.Config) map[string]map[string]any {
	return map[string]map[string]any{
		"Global": {
			"server_name": cfg.AppName, "server_icon": cfg.ServerIcon, "auth_background_url": cfg.AuthBackgroundURL, "databases_dir": cfg.DatabaseDir, "redis_url": cfg.RedisURL, "telegram_mode": cfg.TelegramMode, "force_bind_telegram": cfg.ForceBindTelegram,
			"log_level": cfg.LogLevel, "runtime_log_limit": cfg.RuntimeLogLimit,
			"tmdb_api_key": cfg.TMDBAPIKey, "tmdb_api_url": cfg.TMDBAPIURL, "tmdb_image_url": cfg.TMDBImageURL, "bangumi_token": cfg.BangumiToken, "bangumi_api_url": cfg.BangumiAPIURL,
		},
		"Database": {
			"driver": cfg.DatabaseDriver, "state_file": cfg.StateFile, "url": cfg.DatabaseURL, "backup_dir": cfg.DatabaseBackupDir, "migration_panel_enabled": cfg.DatabaseMigrationPanelEnabled, "postgres_host": cfg.PostgresHost, "postgres_port": cfg.PostgresPort,
			"postgres_user": cfg.PostgresUser, "postgres_password": cfg.PostgresPassword, "postgres_database": cfg.PostgresDatabase, "postgres_sslmode": cfg.PostgresSSLMode,
			"postgres_max_open_conns": cfg.PostgresMaxOpenConns, "postgres_max_idle_conns": cfg.PostgresMaxIdleConns,
		},
		"Emby": {
			"emby_url": cfg.EmbyURL, "emby_token": cfg.EmbyToken, "emby_username": cfg.EmbyUsername, "emby_password": cfg.EmbyPassword,
			"emby_url_list": linesToStrings(cfg.EmbyURLList), "emby_url_list_for_whitelist": linesToStrings(cfg.EmbyWhitelistURLList),
		},
		"Telegram": {
			"telegram_api_url": cfg.TelegramAPIURL, "bot_token": cfg.TelegramBotToken, "admin_id": int64sToAny(cfg.TelegramAdminIDs), "group_id": cfg.TelegramGroupIDs,
			"force_bind_group": cfg.TelegramForceBindGroup, "channel_id": cfg.TelegramChannelIDs, "force_bind_channel": cfg.TelegramForceBindChannel,
			"require_group_membership": cfg.TelegramRequireMembership,
			"enable_tg_panel":          cfg.TelegramEnablePanel, "ban_on_leave": cfg.TelegramBanOnLeave, "auto_enable_rejoined": cfg.TelegramAutoEnableRejoined, "group_check_concurrency": cfg.TelegramGroupCheckConcurrency, "group_action_concurrency": cfg.TelegramGroupActionConcurrency,
			"group_user_panel_template": cfg.TelegramGroupUserPanelTemplate,
			"bot_start_text":            cfg.TelegramBotStartText, "bot_group_start_text": cfg.TelegramBotGroupStartText, "bot_start_title": cfg.TelegramBotStartTitle,
			"bot_start_intro": cfg.TelegramBotStartIntro, "bot_bind_prompt_text": cfg.TelegramBotBindPromptText, "bot_help_text": cfg.TelegramBotHelpText,
			"bot_admin_help_text": cfg.TelegramBotAdminHelpText, "bot_help_header": cfg.TelegramBotHelpHeader, "bot_help_footer": cfg.TelegramBotHelpFooter,
			"bot_about": cfg.TelegramBotAbout, "bot_custom_commands": commandRepliesToAny(cfg.TelegramCustomCommands),
		},
		"SAR": {
			"register_mode": cfg.RegisterEnabled, "register_code_limit": cfg.RegisterCodeLimit, "allow_pending_register": cfg.AllowPendingRegister,
			"emby_direct_register_enabled": cfg.EmbyDirectRegisterEnabled, "emby_direct_register_days": cfg.EmbyDirectRegisterDays, "emby_user_limit": cfg.EmbyUserLimit,
			"user_limit": cfg.UserLimit, "regcode_format": cfg.RegCodeFormat, "register_code_format": cfg.RegisterCodeFormat, "renew_code_format": cfg.RenewCodeFormat, "invite_code_format": cfg.InviteCodeFormat, "regcode_random_algorithm": cfg.RegCodeRandomAlgorithm, "invite_code_random_algorithm": cfg.InviteCodeRandomAlgorithm, "regcode_decoy_action": firstNonEmpty(cfg.DecoyAction, "log_only"),
			"media_request_enabled": cfg.MediaRequestEnabled, "max_concurrent_requests_per_user": cfg.MaxConcurrentRequestsPerUser, "max_concurrent_requests_global": cfg.MaxConcurrentRequestsGlobal, "invite_enabled": cfg.InviteEnabled,
			"invite_limit": cfg.InviteLimit, "invite_root_user_limit": cfg.InviteRootUserLimit, "invite_max_depth": cfg.InviteMaxDepth, "invite_require_emby": cfg.InviteRequireEmby,
			"invite_code_default_days": cfg.InviteDefaultDays, "permanent_invite_max_days": cfg.PermanentInviteMaxDays, "auto_cleanup_no_emby": cfg.AutoCleanupNoEmby,
			"auto_cleanup_no_emby_days": cfg.AutoCleanupNoEmbyDays, "auto_cleanup_pending_emby": cfg.AutoCleanupPendingEmby,
			"auto_cleanup_pending_emby_days": cfg.AutoCleanupPendingEmbyDays,
			"signin_enabled":                 cfg.SigninEnabled, "currency_name": cfg.SigninCurrencyName, "daily_min": cfg.SigninDailyMin, "daily_max": cfg.SigninDailyMax,
			"streak_bonus_enabled": cfg.SigninStreakBonusEnabled, "streak_bonus_days": intsToAny(cfg.SigninStreakBonusDays), "streak_bonus_points": intsToAny(cfg.SigninStreakBonusPoints),
			"reset_after_miss": cfg.SigninResetAfterMiss, "signin_renewal_enabled": cfg.SigninRenewalEnabled, "signin_renewal_cost": cfg.SigninRenewalCost, "signin_renewal_days": cfg.SigninRenewalDays,
		},
		"DeviceLimit": {"device_limit_enabled": cfg.DeviceLimitEnabled, "max_devices": cfg.MaxDevices, "max_streams": cfg.MaxStreams},
		"RateLimit": {
			"enabled": cfg.RateLimitEnabled, "global_per_minute": cfg.RateLimitGlobalPerMinute, "login_per_minute": cfg.RateLimitLoginPerMinute,
			"login_user_per_5m": cfg.RateLimitLoginUserPer5m,
			"register_per_10m":  cfg.RateLimitRegisterPer10m, "forgot_password_ip_per_10m": cfg.RateLimitForgotPasswordIPPer10m,
			"forgot_password_user_per_30m": cfg.RateLimitForgotPasswordUserPer30m, "upload_per_minute": cfg.RateLimitUploadPerMinute,
			"email_code_ip_per_10m": cfg.RateLimitEmailCodeIPPer10m, "email_code_addr_per_10m": cfg.RateLimitEmailCodeAddrPer10m, "email_code_uid_per_10m": cfg.RateLimitEmailCodeUIDPer10m,
			"admin_icon_per_minute": cfg.RateLimitAdminIconPerMinute, "api_key_default_per_minute": cfg.RateLimitAPIKeyDefaultPerMinute,
		},
		"API": {
			"host": cfg.Host, "port": cfg.Port, "cors_origins": cfg.CORSOrigins, "upload_folder": cfg.UploadDir, "max_upload_size": cfg.MaxUploadSize,
			"session_cookie_name": cfg.SessionCookie, "session_cookie_secure": cfg.CookieSecure, "session_cookie_samesite": cfg.CookieSameSite, "session_cookie_domain": cfg.CookieDomain, "trust_proxy_headers": cfg.TrustProxyHeaders, "trusted_proxy_cidrs": cfg.TrustedProxyCIDRs,
		},
		"Email": {
			"enabled": cfg.EmailEnabled, "smtp_host": cfg.SMTPHost, "smtp_port": cfg.SMTPPort, "smtp_username": cfg.SMTPUsername, "smtp_password": cfg.SMTPPassword,
			"smtp_encryption": firstNonEmpty(cfg.SMTPEncryption, "starttls"), "smtp_from_address": cfg.SMTPFromAddress, "smtp_from_name": cfg.SMTPFromName, "smtp_timeout_seconds": cfg.SMTPTimeoutSeconds,
			"force_bind": cfg.EmailForceBind, "code_length": cfg.EmailCodeLength, "code_type": firstNonEmpty(cfg.EmailCodeType, "numeric"), "code_ttl_minutes": cfg.EmailCodeTTLMinutes,
			"resend_cooldown_seconds": cfg.EmailResendCooldownSeconds, "max_attempts": cfg.EmailMaxAttempts, "subject_template": cfg.EmailSubjectTemplate, "body_template": cfg.EmailBodyTemplate,
			"email_validation_mode": cfg.EmailValidationMode, "email_whitelist": cfg.EmailWhitelist, "email_blacklist": cfg.EmailBlacklist,
		},
		"Security":     {"forgot_password_enabled": cfg.ForgotPasswordEnabled, "forgot_password_emby_enabled": cfg.ForgotPasswordEmbyEnabled, "forgot_password_email_enabled": cfg.ForgotPasswordEmailEnabled, "bot_internal_secret": cfg.BotInternalSecret},
		"Scheduler":    {"enabled": cfg.SchedulerEnabled, "tick_interval_seconds": cfg.SchedulerTickIntervalSeconds, "expired_check_time": cfg.SchedulerExpiredCheckTime, "expiring_check_time": cfg.SchedulerExpiringCheckTime, "daily_stats_time": cfg.SchedulerDailyStatsTime, "session_cleanup_interval": cfg.SchedulerSessionCleanupInterval, "cleanup_no_emby_time": cfg.SchedulerCleanupNoEmbyTime, "cleanup_pending_emby_time": cfg.SchedulerCleanupPendingEmbyTime, "cleanup_unused_uploads_time": cfg.SchedulerCleanupUnusedUploadsTime, "cleanup_audit_logs_time": cfg.SchedulerCleanupAuditLogsTime},
		"SystemUpdate": {"auto_update_enabled": cfg.SystemUpdateEnabled, "repo_url": cfg.SystemUpdateRepoURL, "branch": cfg.SystemUpdateBranch, "restart_services": cfg.SystemUpdateRestartServices, "auto_update_trigger_type": cfg.SystemUpdateTriggerType, "auto_update_interval_hours": cfg.SystemUpdateIntervalHours, "auto_update_time": cfg.SystemUpdateTime},
		"Notification": {"enabled": cfg.NotificationEnabled, "expiry_remind_days": cfg.NotificationExpiryRemindDays},
		"BangumiSync":  {"enabled": cfg.BangumiEnabled, "webhook_secret": cfg.BangumiWebhookSecret},
		"Ticket":       {"enabled": cfg.TicketSystemEnabled, "types": cfg.TicketTypes},
		"AuditLog": {
			"enabled": cfg.AuditLogEnabled, "auto_cleanup_enabled": cfg.AuditLogAutoCleanupEnabled,
			"retention_days": cfg.AuditLogRetentionDays, "max_entries": cfg.AuditLogMaxEntries,
			"preserve_admin": cfg.AuditLogPreserveAdmin, "cleanup_check_time": cfg.AuditLogCleanupCheckTime,
		},
	}
}

func normalizeConfigField(field configFieldDef, value any) any {
	switch field.Type {
	case "bool":
		return boolish(value)
	case "int":
		return int(numeric(value))
	case "list":
		if items, ok := value.([]any); ok {
			out := make([]any, 0, len(items))
			for _, item := range items {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
					if (field.Key == "admin_id" || listFieldWantsInts(field.Key)) && isIntegerString(text) {
						out = append(out, int(numeric(text)))
					} else {
						out = append(out, text)
					}
				}
			}
			return out
		}
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
			if listFieldWantsInts(field.Key) && isIntegerString(text) {
				return []any{int(numeric(text))}
			}
			return []any{text}
		}
		return []any{}
	case "command_map":
		items, _ := value.([]any)
		out := make([]any, 0, len(items))
		seen := map[string]bool{}
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			command := normalizeTelegramCustomCommand(fmt.Sprint(m["command"]))
			reply := strings.TrimSpace(fmt.Sprint(m["reply"]))
			if command == "" || reply == "" || seen[command] {
				continue
			}
			seen[command] = true
			out = append(out, command+" = "+reply)
		}
		return out
	default:
		return fmt.Sprint(value)
	}
}

func renderConfigTOML(values map[string]map[string]any) string {
	var b strings.Builder
	for _, section := range configSectionDefs() {
		b.WriteString("[")
		b.WriteString(section.Key)
		b.WriteString("]\n")
		for _, field := range section.Fields {
			b.WriteString(field.Key)
			b.WriteString(" = ")
			b.WriteString(tomlValue(values[section.Key][field.Key]))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func tomlValue(value any) string {
	switch typed := value.(type) {
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []string:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return tomlArray(values)
	case []any:
		return tomlArray(typed)
	default:
		return strconv.Quote(fmt.Sprint(value))
	}
}

func tomlArray(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			parts = append(parts, strconv.Itoa(typed))
		case int64:
			parts = append(parts, strconv.FormatInt(typed, 10))
		case float64:
			if typed == float64(int64(typed)) {
				parts = append(parts, strconv.FormatInt(int64(typed), 10))
			} else {
				parts = append(parts, strconv.FormatFloat(typed, 'f', -1, 64))
			}
		case bool:
			parts = append(parts, strconv.FormatBool(typed))
		case map[string]any:
			parts = append(parts, strconv.Quote(formatCommandReplyMap(typed)))
		default:
			parts = append(parts, strconv.Quote(fmt.Sprint(value)))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func linesToStrings(lines []config.Line) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Name != "" {
			out = append(out, line.Name+" : "+line.URL)
		} else {
			out = append(out, line.URL)
		}
	}
	return out
}

func commandRepliesToAny(values []config.TelegramCommandReply) []any {
	out := make([]any, 0, len(values))
	for _, item := range values {
		out = append(out, map[string]any{"command": item.Command, "reply": item.Reply})
	}
	return out
}

func formatCommandReplyMap(value map[string]any) string {
	return normalizeTelegramCustomCommand(fmt.Sprint(value["command"])) + " = " + strings.TrimSpace(fmt.Sprint(value["reply"]))
}

func normalizeTelegramCustomCommand(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "/")
	if value == "" || len(value) > 32 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return ""
	}
	return "/" + value
}

func int64sToAny(values []int64) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func intsToAny(values []int) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func listFieldWantsInts(key string) bool {
	return key == "streak_bonus_days" || key == "streak_bonus_points"
}

func isIntegerString(value string) bool {
	if value == "" {
		return false
	}
	if value[0] == '-' {
		value = value[1:]
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
