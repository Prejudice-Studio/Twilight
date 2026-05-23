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

func (a *App) handleConfigTOMLPutSafe(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	info, status, message := a.saveConfigContent(stringValue(payload, "content"))
	if status != http.StatusOK {
		fail(w, status, message)
		return
	}
	ok(w, "配置已保存并热重载", info)
}

func (a *App) handleConfigBackups(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, err := listConfigBackups(a.configBackupDir())
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取配置备份列表失败")
		return
	}
	ok(w, "OK", map[string]any{"backups": backups, "config_file": a.configFilePath(), "backup_dir": a.configBackupDir()})
}

func (a *App) handleConfigBackup(w http.ResponseWriter, r *http.Request, _ Params) {
	info, err := a.createConfigBackup()
	if err != nil {
		if err == store.ErrNotFound {
			fail(w, http.StatusNotFound, "配置文件不存在")
			return
		}
		fail(w, http.StatusInternalServerError, "配置备份失败")
		return
	}
	ok(w, "配置备份已创建", map[string]any{"backup": info})
}

func (a *App) handleConfigBackupInspect(w http.ResponseWriter, r *http.Request, params Params) {
	backup, content, err := a.configBackupContent(params["name"])
	if err != nil {
		fail(w, http.StatusBadRequest, "配置备份无效")
		return
	}
	ok(w, "OK", map[string]any{"backup": backup, "content": string(content), "config_file": a.configFilePath()})
}

func (a *App) handleConfigRestore(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	name := firstNonEmpty(stringValue(payload, "name"), stringValue(payload, "backup"))
	backup, content, err := a.configBackupContent(name)
	if err != nil {
		fail(w, http.StatusBadRequest, "配置备份无效")
		return
	}
	if err := validateConfigContent(a.configFilePath(), content); err != nil {
		fail(w, http.StatusBadRequest, "配置备份校验失败: "+err.Error())
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
		fail(w, status, message)
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
		fail(w, http.StatusBadRequest, "配置备份无效")
		return
	}
	info, err := configBackupInfo(path)
	if err != nil {
		fail(w, http.StatusBadRequest, "配置备份无效")
		return
	}
	if err := os.Remove(path); err != nil {
		fail(w, http.StatusInternalServerError, "删除配置备份失败")
		return
	}
	ok(w, "配置备份已删除", map[string]any{"backup": info})
}

func (a *App) handleConfigSchemaFull(w http.ResponseWriter, r *http.Request, _ Params) {
	values := configValues(a.cfg)
	sections := make([]map[string]any, 0, len(configSectionDefs()))
	for _, def := range configSectionDefs() {
		fields := make([]map[string]any, 0, len(def.Fields))
		for _, field := range def.Fields {
			item := map[string]any{
				"key":         field.Key,
				"label":       field.Label,
				"type":        field.Type,
				"description": field.Description,
				"value":       values[def.Key][field.Key],
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
			{"key": "ops", "title": "运维"},
		},
		"sections": sections,
	})
}

func (a *App) handleConfigSchemaUpdateSafe(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	rawSections, _ := payload["sections"].(map[string]any)
	values := configValues(a.cfg)
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
			values[sectionKey][fieldKey] = normalizeConfigField(field, value)
		}
	}
	info, status, message := a.saveConfigContent(renderConfigTOML(values))
	if status != http.StatusOK {
		fail(w, status, message)
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
	if err := validateConfigContent(configFile, []byte(content)); err != nil {
		return nil, http.StatusBadRequest, "配置校验失败: " + err.Error()
	}
	existing, readErr := os.ReadFile(configFile)
	hadExisting := readErr == nil
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
	if hadExisting {
		_ = os.Remove(configFile)
	}
	if err := os.Rename(tmpPath, configFile); err != nil {
		if hadExisting {
			_ = os.WriteFile(configFile, existing, 0o600)
		}
		_ = os.Remove(tmpPath)
		return nil, http.StatusInternalServerError, "替换配置失败"
	}
	reloadInfo, err := a.reloadConfig()
	if err != nil {
		if hadExisting {
			_ = os.Remove(configFile)
			_ = os.WriteFile(configFile, existing, 0o600)
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

func (a *App) configBackupDir() string {
	dir := strings.TrimSpace(a.cfg.DatabaseBackupDir)
	if dir == "" {
		dir = filepath.Join(firstNonEmpty(a.cfg.DatabaseDir, "db"), "backups")
	}
	return filepath.Join(dir, "config")
}

func (a *App) configFilePath() string {
	return firstNonEmpty(a.cfg.ConfigFile, "config.toml")
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
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) || filepath.Base(name) != name || strings.Contains(name, "..") || strings.ToLower(filepath.Ext(name)) != ".toml" {
		return "", store.ErrNotFound
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
		return "", store.ErrNotFound
	}
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
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

func configSectionDefs() []configSectionDef {
	selectDriver := []map[string]any{{"label": "PostgreSQL（推荐）", "value": "postgres"}, {"label": "Go JSON 文件（兼容）", "value": "json"}}
	selectUpdate := []map[string]any{{"label": "按间隔", "value": "interval"}, {"label": "每日固定时间", "value": "daily"}, {"label": "手动", "value": "manual"}}
	return []configSectionDef{
		{Key: "Global", Title: "全局", Description: "基础运行参数", Category: "runtime", Fields: []configFieldDef{
			{Key: "databases_dir", Label: "数据目录", Type: "string", Description: "JSON 状态、备份和迁移文件目录"},
			{Key: "log_level", Label: "日志等级", Type: "select", Description: "后端运行日志等级；兼容旧值 10/20/30/40", Options: []map[string]any{{"label": "DEBUG", "value": "debug"}, {"label": "INFO", "value": "info"}, {"label": "WARN", "value": "warn"}, {"label": "ERROR", "value": "error"}}},
			{Key: "runtime_log_limit", Label: "实时日志保留行数", Type: "int", Description: "后台实时日志缓冲区行数，热重载生效"},
			{Key: "redis_url", Label: "Redis URL", Type: "secret", Description: "会话和限流 Redis，留空使用进程内存"},
			{Key: "telegram_mode", Label: "启用 Telegram", Type: "bool", Description: "启用 Bot 和 Telegram 绑定能力"},
			{Key: "force_bind_telegram", Label: "强制绑定 Telegram", Type: "bool", Description: "登录/注册流程要求绑定 Telegram"},
			{Key: "tmdb_api_key", Label: "TMDB API Key", Type: "secret", Description: "媒体搜索使用的 TMDB Key"},
			{Key: "tmdb_api_url", Label: "TMDB API URL", Type: "string", Description: "TMDB API 基础地址"},
			{Key: "tmdb_image_url", Label: "TMDB 图片 URL", Type: "string", Description: "TMDB 图片 CDN 地址"},
			{Key: "bangumi_token", Label: "Bangumi Token", Type: "secret", Description: "Bangumi 全局 Token"},
			{Key: "bangumi_api_url", Label: "Bangumi API URL", Type: "string", Description: "Bangumi API 基础地址"},
		}},
		{Key: "Database", Title: "数据库", Description: "JSON/PostgreSQL 存储和备份配置", Category: "ops", Fields: []configFieldDef{
			{Key: "driver", Label: "存储后端", Type: "select", Description: "可视化配置仅提供 PostgreSQL 与 Go JSON；旧 SQLite 仅作为手动迁移源，不作为前端可选运行后端", Options: selectDriver},
			{Key: "state_file", Label: "JSON 状态文件", Type: "string", Description: "Go JSON 状态文件路径，留空使用数据目录下 twilight_go_state.json"},
			{Key: "backup_dir", Label: "备份目录", Type: "string", Description: "数据库快照备份目录"},
			{Key: "migration_panel_enabled", Label: "启用数据库迁移", Type: "bool", Description: "默认关闭；开启后显示数据库迁移面板并允许管理员调用迁移 API"},
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
		{Key: "Emby", Title: "Emby", Description: "Emby 连接、线路和媒体库", Category: "integration", Fields: []configFieldDef{
			{Key: "emby_url", Label: "Emby URL", Type: "string", Description: "后端访问的 Emby/Jellyfin 地址"},
			{Key: "emby_token", Label: "Emby Token", Type: "secret", Description: "Emby API Key"},
			{Key: "emby_username", Label: "管理员用户名", Type: "string", Description: "备用鉴权用户名"},
			{Key: "emby_password", Label: "管理员密码", Type: "secret", Description: "备用鉴权密码"},
			{Key: "emby_url_list", Label: "普通线路", Type: "list", Description: "格式：名称 : URL"},
			{Key: "emby_url_list_for_whitelist", Label: "白名单线路", Type: "list", Description: "管理员/白名单用户可见线路"},
			{Key: "emby_default_hidden_libraries", Label: "默认隐藏媒体库", Type: "list", Description: "新建 Emby 用户默认隐藏的媒体库名"},
			{Key: "emby_self_service_libraries", Label: "自助媒体库", Type: "list", Description: "允许用户自行显隐的媒体库名"},
		}},
		{Key: "Telegram", Title: "Telegram", Description: "Bot、订阅校验和群组管理", Category: "integration", Fields: []configFieldDef{
			{Key: "telegram_api_url", Label: "Bot API URL", Type: "string", Description: "Telegram Bot API 基础地址"},
			{Key: "bot_token", Label: "Bot Token", Type: "secret", Description: "Telegram Bot Token"},
			{Key: "admin_id", Label: "管理员 Telegram ID", Type: "list", Description: "Bot 管理员 ID 列表"},
			{Key: "group_id", Label: "群组 ID", Type: "list", Description: "强制订阅/巡检群组"},
			{Key: "channel_id", Label: "频道 ID", Type: "list", Description: "强制订阅频道"},
			{Key: "force_subscribe", Label: "强制订阅", Type: "bool", Description: "要求订阅指定群组/频道"},
			{Key: "enable_tg_panel", Label: "启用 Bot 面板", Type: "bool", Description: "启用更多 Bot 查询命令和管理查询入口"},
			{Key: "require_group_membership", Label: "强制群成员", Type: "bool", Description: "巡检发现退群时禁用本地/Emby"},
			{Key: "ban_on_leave", Label: "退群封禁", Type: "bool", Description: "退群后在群组永久封禁"},
			{Key: "group_check_concurrency", Label: "巡检并发", Type: "int", Description: "getChatMember 并发数"},
			{Key: "group_action_concurrency", Label: "写操作并发", Type: "int", Description: "踢出/封禁等动作并发数"},
			{Key: "bot_start_text", Label: "Bot 开始文案", Type: "textarea", Description: "覆盖私聊 /start 文案，支持换行；保存到 TOML 时会自动转义为 \\n"},
			{Key: "bot_group_start_text", Label: "群聊开始文案", Type: "textarea", Description: "覆盖群聊 /start 提示，支持换行；留空使用内置文案"},
			{Key: "bot_start_title", Label: "开始标题", Type: "string", Description: "内置 /start 文案的标题"},
			{Key: "bot_start_intro", Label: "开始简介", Type: "textarea", Description: "内置 /start 文案的简介，支持换行"},
			{Key: "bot_bind_prompt_text", Label: "绑定提示文案", Type: "textarea", Description: "/bind 无参数时的提示文本，支持换行"},
			{Key: "bot_help_text", Label: "用户帮助文案", Type: "textarea", Description: "覆盖 /help 和 /twihelp 文案，支持换行"},
			{Key: "bot_admin_help_text", Label: "管理员帮助文案", Type: "textarea", Description: "覆盖 /twishelp 文案，支持换行"},
			{Key: "bot_help_header", Label: "帮助页前缀", Type: "textarea", Description: "追加到内置用户帮助顶部，支持换行"},
			{Key: "bot_help_footer", Label: "帮助页后缀", Type: "textarea", Description: "追加到内置用户帮助底部，支持换行"},
			{Key: "bot_about", Label: "Bot 关于文案", Type: "textarea", Description: "/about 的服务说明，支持换行"},
			{Key: "bot_custom_commands", Label: "Bot 自定义指令回复", Type: "command_map", Description: "自定义 /command 与回复内容的映射，回复支持换行；内置指令优先级更高"},
		}},
		{Key: "SAR", Title: "注册/邀请", Description: "注册、卡码、邀请树和求片", Category: "policy", Fields: []configFieldDef{
			{Key: "register_mode", Label: "开放注册", Type: "bool", Description: "是否允许注册系统账号"},
			{Key: "register_code_limit", Label: "注册必须用码", Type: "bool", Description: "注册时必须提供注册码"},
			{Key: "allow_pending_register", Label: "允许待补建", Type: "bool", Description: "允许无 Emby 账号先注册"},
			{Key: "emby_direct_register_enabled", Label: "Emby 自助注册", Type: "bool", Description: "用户可自助创建 Emby 账号"},
			{Key: "emby_direct_register_days", Label: "自助注册天数", Type: "int", Description: "Emby 自助注册默认有效期"},
			{Key: "emby_user_limit", Label: "Emby 用户上限", Type: "int", Description: "-1 表示不限"},
			{Key: "media_request_enabled", Label: "启用求片", Type: "bool", Description: "允许用户提交媒体请求"},
			{Key: "max_concurrent_requests_per_user", Label: "每用户并发求片", Type: "int", Description: "-1 表示不限"},
			{Key: "invite_enabled", Label: "启用邀请树", Type: "bool", Description: "允许用户生成邀请码/续期码"},
			{Key: "invite_limit", Label: "邀请码数量", Type: "int", Description: "每用户可持有邀请码数量"},
			{Key: "invite_root_user_limit", Label: "根邀请上限", Type: "int", Description: "根节点邀请数量限制"},
			{Key: "invite_max_depth", Label: "邀请最大深度", Type: "int", Description: "邀请关系最大层级"},
			{Key: "invite_require_emby", Label: "邀请要求 Emby", Type: "bool", Description: "已绑定 Emby 才能邀请"},
			{Key: "invite_code_default_days", Label: "邀请码默认天数", Type: "int", Description: "新邀请码默认续期/开通天数"},
			{Key: "permanent_invite_max_days", Label: "永久码最大天数", Type: "int", Description: "永久邀请可授予最大天数"},
			{Key: "auto_cleanup_no_emby", Label: "清理无 Emby 用户", Type: "bool", Description: "定期清理长期未绑定 Emby 的用户"},
			{Key: "auto_cleanup_no_emby_days", Label: "无 Emby 清理天数", Type: "int", Description: "超过该天数后可清理"},
		}},
		{Key: "DeviceLimit", Title: "设备限制", Description: "设备和并发播放限制", Category: "policy", Fields: []configFieldDef{
			{Key: "device_limit_enabled", Label: "启用设备限制", Type: "bool", Description: "限制设备数量"},
			{Key: "max_devices", Label: "最大设备数", Type: "int", Description: "每用户最大设备数"},
			{Key: "max_streams", Label: "最大播放流", Type: "int", Description: "每用户最大并发流"},
		}},
		{Key: "API", Title: "API", Description: "监听、跨域、上传和 Cookie", Category: "runtime", Fields: []configFieldDef{
			{Key: "host", Label: "监听地址", Type: "string", Description: "修改后需重启监听器"},
			{Key: "port", Label: "监听端口", Type: "int", Description: "修改后需重启监听器"},
			{Key: "cors_origins", Label: "CORS Origins", Type: "list", Description: "允许的前端 Origin；凭据接口不会接受 *，生产环境必须显式填写 HTTPS 域名"},
			{Key: "upload_folder", Label: "上传目录", Type: "string", Description: "头像/背景上传目录"},
			{Key: "max_upload_size", Label: "上传上限", Type: "int", Description: "单文件最大字节数"},
			{Key: "session_cookie_name", Label: "Cookie 名称", Type: "string", Description: "会话 Cookie 名称"},
			{Key: "session_cookie_secure", Label: "Secure Cookie", Type: "bool", Description: "HTTPS 部署应开启"},
			{Key: "session_cookie_samesite", Label: "SameSite", Type: "string", Description: "lax/strict/none"},
			{Key: "trust_proxy_headers", Label: "信任代理 IP", Type: "bool", Description: "仅在可信反代后开启"},
		}},
		{Key: "Security", Title: "安全", Description: "内部密钥和安全开关", Category: "ops", Fields: []configFieldDef{
			{Key: "bot_internal_secret", Label: "Bot 内部密钥", Type: "secret", Description: "外部更新回调共享密钥"},
		}},
		{Key: "Scheduler", Title: "调度器", Description: "后台任务计划", Category: "ops", Fields: []configFieldDef{
			{Key: "enabled", Label: "启用调度", Type: "bool", Description: "启用后台任务"},
			{Key: "expired_check_time", Label: "过期检查", Type: "string", Description: "每日 HH:MM"},
			{Key: "expiring_check_time", Label: "到期提醒检查", Type: "string", Description: "每日 HH:MM"},
			{Key: "daily_stats_time", Label: "每日统计", Type: "string", Description: "每日 HH:MM"},
			{Key: "session_cleanup_interval", Label: "会话检查间隔", Type: "int", Description: "小时"},
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
	}
}

func configValues(cfg config.Config) map[string]map[string]any {
	return map[string]map[string]any{
		"Global": {
			"databases_dir": cfg.DatabaseDir, "redis_url": cfg.RedisURL, "telegram_mode": cfg.TelegramMode, "force_bind_telegram": cfg.ForceBindTelegram,
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
			"emby_default_hidden_libraries": cfg.EmbyDefaultHiddenLibraries, "emby_self_service_libraries": cfg.EmbySelfServiceLibraries,
		},
		"Telegram": {
			"telegram_api_url": cfg.TelegramAPIURL, "bot_token": cfg.TelegramBotToken, "admin_id": int64sToAny(cfg.TelegramAdminIDs), "group_id": cfg.TelegramGroupIDs,
			"channel_id": cfg.TelegramChannelIDs, "force_subscribe": cfg.TelegramForceSubscribe, "require_group_membership": cfg.TelegramRequireMembership,
			"enable_tg_panel": cfg.TelegramEnablePanel, "ban_on_leave": cfg.TelegramBanOnLeave, "group_check_concurrency": cfg.TelegramGroupCheckConcurrency, "group_action_concurrency": cfg.TelegramGroupActionConcurrency,
			"bot_start_text": cfg.TelegramBotStartText, "bot_group_start_text": cfg.TelegramBotGroupStartText, "bot_start_title": cfg.TelegramBotStartTitle,
			"bot_start_intro": cfg.TelegramBotStartIntro, "bot_bind_prompt_text": cfg.TelegramBotBindPromptText, "bot_help_text": cfg.TelegramBotHelpText,
			"bot_admin_help_text": cfg.TelegramBotAdminHelpText, "bot_help_header": cfg.TelegramBotHelpHeader, "bot_help_footer": cfg.TelegramBotHelpFooter,
			"bot_about": cfg.TelegramBotAbout, "bot_custom_commands": commandRepliesToAny(cfg.TelegramCustomCommands),
		},
		"SAR": {
			"register_mode": cfg.RegisterEnabled, "register_code_limit": cfg.RegisterCodeLimit, "allow_pending_register": cfg.AllowPendingRegister,
			"emby_direct_register_enabled": cfg.EmbyDirectRegisterEnabled, "emby_direct_register_days": cfg.EmbyDirectRegisterDays, "emby_user_limit": cfg.EmbyUserLimit,
			"media_request_enabled": cfg.MediaRequestEnabled, "max_concurrent_requests_per_user": cfg.MaxConcurrentRequestsPerUser, "invite_enabled": cfg.InviteEnabled,
			"invite_limit": cfg.InviteLimit, "invite_root_user_limit": cfg.InviteRootUserLimit, "invite_max_depth": cfg.InviteMaxDepth, "invite_require_emby": cfg.InviteRequireEmby,
			"invite_code_default_days": cfg.InviteDefaultDays, "permanent_invite_max_days": cfg.PermanentInviteMaxDays, "auto_cleanup_no_emby": cfg.AutoCleanupNoEmby,
			"auto_cleanup_no_emby_days": cfg.AutoCleanupNoEmbyDays,
		},
		"DeviceLimit": {"device_limit_enabled": cfg.DeviceLimitEnabled, "max_devices": cfg.MaxDevices, "max_streams": cfg.MaxStreams},
		"API": {
			"host": cfg.Host, "port": cfg.Port, "cors_origins": cfg.CORSOrigins, "upload_folder": cfg.UploadDir, "max_upload_size": cfg.MaxUploadSize,
			"session_cookie_name": cfg.SessionCookie, "session_cookie_secure": cfg.CookieSecure, "session_cookie_samesite": cfg.CookieSameSite, "trust_proxy_headers": cfg.TrustProxyHeaders,
		},
		"Security":     {"bot_internal_secret": cfg.BotInternalSecret},
		"Scheduler":    {"enabled": cfg.SchedulerEnabled, "expired_check_time": cfg.SchedulerExpiredCheckTime, "expiring_check_time": cfg.SchedulerExpiringCheckTime, "daily_stats_time": cfg.SchedulerDailyStatsTime, "session_cleanup_interval": cfg.SchedulerSessionCleanupInterval},
		"SystemUpdate": {"auto_update_enabled": cfg.SystemUpdateEnabled, "repo_url": cfg.SystemUpdateRepoURL, "branch": cfg.SystemUpdateBranch, "restart_services": cfg.SystemUpdateRestartServices, "auto_update_trigger_type": cfg.SystemUpdateTriggerType, "auto_update_interval_hours": cfg.SystemUpdateIntervalHours, "auto_update_time": cfg.SystemUpdateTime},
		"Notification": {"enabled": cfg.NotificationEnabled, "expiry_remind_days": cfg.NotificationExpiryRemindDays},
		"BangumiSync":  {"enabled": cfg.BangumiEnabled, "webhook_secret": cfg.BangumiWebhookSecret},
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
					if field.Key == "admin_id" && isIntegerString(text) {
						out = append(out, int(numeric(text)))
					} else {
						out = append(out, text)
					}
				}
			}
			return out
		}
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
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
