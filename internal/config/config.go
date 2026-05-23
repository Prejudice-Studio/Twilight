package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Line struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type TelegramCommandReply struct {
	Command string `json:"command"`
	Reply   string `json:"reply"`
}

type Config struct {
	AppName                       string
	Version                       string
	Host                          string
	Port                          int
	ServerIcon                    string
	DatabaseDir                   string
	DatabaseDriver                string
	DatabaseURL                   string
	DatabaseBackupDir             string
	DatabaseMigrationPanelEnabled bool
	PostgresHost                  string
	PostgresPort                  int
	PostgresUser                  string
	PostgresPassword              string
	PostgresDatabase              string
	PostgresSSLMode               string
	PostgresMaxOpenConns          int
	PostgresMaxIdleConns          int
	StateFile                     string
	UploadDir                     string
	MaxUploadSize                 int64
	RedisURL                      string
	LogLevel                      string
	RuntimeLogLimit               int

	CORSOrigins       []string
	AllowCredential   bool
	TrustProxyHeaders bool

	SessionCookie  string
	SessionTTL     time.Duration
	CookieSecure   bool
	CookieSameSite string

	EmbyURL                        string
	EmbyToken                      string
	EmbyUsername                   string
	EmbyPassword                   string
	EmbyURLList                    []Line
	EmbyWhitelistURLList           []Line
	EmbyDefaultHiddenLibraries     []string
	EmbySelfServiceLibraries       []string
	EmbyPublicURL                  string
	EmbyWhitelistURL               string
	TelegramMode                   bool
	ForceBindTelegram              bool
	TelegramBotToken               string
	TelegramAPIURL                 string
	TelegramAdminIDs               []int64
	TelegramGroupIDs               []string
	TelegramChannelIDs             []string
	TelegramForceSubscribe         bool
	TelegramRequireMembership      bool
	TelegramGroupCheckConcurrency  int
	TelegramGroupActionConcurrency int
	TelegramBanOnLeave             bool
	TelegramAutoEnableRejoined     bool
	TelegramEnablePanel            bool
	TelegramBotStartText           string
	TelegramBotGroupStartText      string
	TelegramBotStartTitle          string
	TelegramBotStartIntro          string
	TelegramBotBindPromptText      string
	TelegramBotHelpText            string
	TelegramBotAdminHelpText       string
	TelegramBotHelpHeader          string
	TelegramBotHelpFooter          string
	TelegramBotAbout               string
	TelegramCustomCommands         []TelegramCommandReply
	BotInternalSecret              string
	BangumiEnabled                 bool
	BangumiWebhookSecret           string
	TMDBAPIKey                     string
	TMDBAPIURL                     string
	TMDBImageURL                   string
	BangumiToken                   string
	BangumiAPIURL                  string
	BangumiAppID                   string

	RegisterEnabled              bool
	RegisterCodeLimit            bool
	AllowPendingRegister         bool
	EmbyDirectRegisterEnabled    bool
	EmbyDirectRegisterDays       int
	EmbyUserLimit                int
	DecoyAction                  string
	NotificationEnabled          bool
	NotificationExpiryRemindDays int
	AutoCleanupNoEmby            bool
	AutoCleanupNoEmbyDays        int

	SchedulerEnabled                bool
	SchedulerExpiredCheckTime       string
	SchedulerExpiringCheckTime      string
	SchedulerDailyStatsTime         string
	SchedulerSessionCleanupInterval int
	SystemUpdateEnabled             bool
	SystemUpdateRepoURL             string
	SystemUpdateBranch              string
	SystemUpdateRestartServices     bool
	SystemUpdateTriggerType         string
	SystemUpdateIntervalHours       int
	SystemUpdateTime                string

	MediaRequestEnabled          bool
	MaxConcurrentRequestsPerUser int
	InviteEnabled                bool
	InviteMaxDepth               int
	InviteLimit                  int
	InviteRootUserLimit          int
	InviteRequireEmby            bool
	InviteDefaultDays            int
	PermanentInviteMaxDays       int
	UserLimit                    int
	DeviceLimitEnabled           bool
	MaxDevices                   int
	MaxStreams                   int

	ConfigFile string
}

func Load(path string) (Config, error) {
	cfg := defaults()
	if path == "" {
		path = defaultConfigPath()
	}
	cfg.ConfigFile = path

	values := map[string]string{}
	if err := readTOML(path, values); err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	local := os.Getenv("TWILIGHT_CONFIG_LOCAL_FILE")
	if local == "" {
		local = strings.TrimSuffix(path, filepath.Ext(path)) + ".local" + filepath.Ext(path)
	}
	if err := readTOML(local, values); err != nil && !os.IsNotExist(err) {
		return cfg, err
	}

	cfg.AppName = first(values, "Global.server_name", "server_name", cfg.AppName)
	cfg.ServerIcon = first(values, "Global.server_icon", "server_icon", cfg.ServerIcon)
	cfg.RedisURL = first(values, "Global.redis_url", "redis_url", cfg.RedisURL)
	cfg.LogLevel = normalizeLogLevel(first(values, "Global.log_level", "log_level", cfg.LogLevel))
	cfg.RuntimeLogLimit = intValue(first(values, "Global.runtime_log_limit", "runtime_log_limit", strconv.Itoa(cfg.RuntimeLogLimit)), cfg.RuntimeLogLimit)
	cfg.DatabaseDir = first(values, "Global.databases_dir", "databases_dir", cfg.DatabaseDir)
	cfg.DatabaseDriver = strings.ToLower(first(values, "Database.driver", "Global.database_driver", "database_driver", cfg.DatabaseDriver))
	cfg.DatabaseURL = first(values, "Database.url", "Database.database_url", "Global.database_url", "database_url", cfg.DatabaseURL)
	cfg.DatabaseBackupDir = first(values, "Database.backup_dir", "database_backup_dir", cfg.DatabaseBackupDir)
	cfg.DatabaseMigrationPanelEnabled = boolValue(first(values, "Database.migration_panel_enabled", "database_migration_panel_enabled", strconv.FormatBool(cfg.DatabaseMigrationPanelEnabled)), cfg.DatabaseMigrationPanelEnabled)
	cfg.StateFile = first(values, "Database.state_file", "Global.state_file", "state_file", cfg.StateFile)
	cfg.PostgresHost = first(values, "Database.postgres_host", "PostgreSQL.host", "postgres_host", cfg.PostgresHost)
	cfg.PostgresPort = intValue(first(values, "Database.postgres_port", "PostgreSQL.port", "postgres_port", strconv.Itoa(cfg.PostgresPort)), cfg.PostgresPort)
	cfg.PostgresUser = first(values, "Database.postgres_user", "PostgreSQL.user", "postgres_user", cfg.PostgresUser)
	cfg.PostgresPassword = first(values, "Database.postgres_password", "PostgreSQL.password", "postgres_password", cfg.PostgresPassword)
	cfg.PostgresDatabase = first(values, "Database.postgres_database", "PostgreSQL.database", "postgres_database", cfg.PostgresDatabase)
	cfg.PostgresSSLMode = first(values, "Database.postgres_sslmode", "PostgreSQL.sslmode", "postgres_sslmode", cfg.PostgresSSLMode)
	cfg.PostgresMaxOpenConns = intValue(first(values, "Database.postgres_max_open_conns", "PostgreSQL.max_open_conns", "postgres_max_open_conns", strconv.Itoa(cfg.PostgresMaxOpenConns)), cfg.PostgresMaxOpenConns)
	cfg.PostgresMaxIdleConns = intValue(first(values, "Database.postgres_max_idle_conns", "PostgreSQL.max_idle_conns", "postgres_max_idle_conns", strconv.Itoa(cfg.PostgresMaxIdleConns)), cfg.PostgresMaxIdleConns)
	cfg.UploadDir = first(values, "API.upload_folder", "upload_folder", cfg.UploadDir)
	cfg.Host = first(values, "API.host", "host", cfg.Host)
	cfg.Port = intValue(first(values, "API.port", "port", strconv.Itoa(cfg.Port)), cfg.Port)
	cfg.MaxUploadSize = int64Value(first(values, "API.max_upload_size", "max_upload_size", strconv.FormatInt(cfg.MaxUploadSize, 10)), cfg.MaxUploadSize)
	cfg.CORSOrigins = listValue(first(values, "API.cors_origins", "cors_origins", strings.Join(cfg.CORSOrigins, ",")))
	cfg.TrustProxyHeaders = boolValue(first(values, "API.trust_proxy_headers", "trust_proxy_headers", strconv.FormatBool(cfg.TrustProxyHeaders)), cfg.TrustProxyHeaders)
	cfg.SessionCookie = first(values, "Security.session_cookie_name", "session_cookie_name", cfg.SessionCookie)
	cfg.BotInternalSecret = first(values, "Security.bot_internal_secret", "bot_internal_secret", cfg.BotInternalSecret)
	cfg.EmbyURL = first(values, "Emby.emby_url", "emby_url", cfg.EmbyURL)
	cfg.EmbyToken = first(values, "Emby.emby_token", "emby_token", cfg.EmbyToken)
	cfg.EmbyUsername = first(values, "Emby.emby_username", "emby_username", cfg.EmbyUsername)
	cfg.EmbyPassword = first(values, "Emby.emby_password", "emby_password", cfg.EmbyPassword)
	cfg.EmbyPublicURL = first(values, "Emby.emby_public_url", "emby_public_url", cfg.EmbyPublicURL)
	cfg.EmbyWhitelistURL = first(values, "Emby.emby_whitelist_url", "emby_whitelist_url", cfg.EmbyWhitelistURL)
	cfg.EmbyURLList = parseLines(first(values, "Emby.emby_url_list", "emby_url_list", ""))
	cfg.EmbyWhitelistURLList = parseLines(first(values, "Emby.emby_url_list_for_whitelist", "emby_url_list_for_whitelist", ""))
	cfg.EmbyDefaultHiddenLibraries = listValue(first(values, "Emby.emby_default_hidden_libraries", "emby_default_hidden_libraries", ""))
	cfg.EmbySelfServiceLibraries = listValue(first(values, "Emby.emby_self_service_libraries", "emby_self_service_libraries", ""))
	cfg.TelegramMode = boolValue(first(values, "Global.telegram_mode", "telegram_mode", strconv.FormatBool(cfg.TelegramMode)), cfg.TelegramMode)
	cfg.ForceBindTelegram = boolValue(first(values, "Global.force_bind_telegram", "force_bind_telegram", strconv.FormatBool(cfg.ForceBindTelegram)), cfg.ForceBindTelegram)
	cfg.TelegramBotToken = first(values, "Telegram.bot_token", "bot_token", cfg.TelegramBotToken)
	cfg.TelegramAPIURL = first(values, "Telegram.telegram_api_url", "telegram_api_url", cfg.TelegramAPIURL)
	cfg.TelegramAdminIDs = int64ListValue(first(values, "Telegram.admin_id", "admin_id", strings.Join(int64ListToStrings(cfg.TelegramAdminIDs), ",")))
	cfg.TelegramGroupIDs = listValue(first(values, "Telegram.group_id", "group_id", strings.Join(cfg.TelegramGroupIDs, ",")))
	cfg.TelegramChannelIDs = listValue(first(values, "Telegram.channel_id", "channel_id", strings.Join(cfg.TelegramChannelIDs, ",")))
	cfg.TelegramForceSubscribe = boolValue(first(values, "Telegram.force_subscribe", "force_subscribe", strconv.FormatBool(cfg.TelegramForceSubscribe)), cfg.TelegramForceSubscribe)
	cfg.TelegramRequireMembership = boolValue(first(values, "Telegram.require_group_membership", "require_group_membership", strconv.FormatBool(cfg.TelegramRequireMembership)), cfg.TelegramRequireMembership)
	cfg.TelegramGroupCheckConcurrency = intValue(first(values, "Telegram.group_check_concurrency", "group_check_concurrency", strconv.Itoa(cfg.TelegramGroupCheckConcurrency)), cfg.TelegramGroupCheckConcurrency)
	cfg.TelegramGroupActionConcurrency = intValue(first(values, "Telegram.group_action_concurrency", "group_action_concurrency", strconv.Itoa(cfg.TelegramGroupActionConcurrency)), cfg.TelegramGroupActionConcurrency)
	cfg.TelegramBanOnLeave = boolValue(first(values, "Telegram.ban_on_leave", "ban_on_leave", strconv.FormatBool(cfg.TelegramBanOnLeave)), cfg.TelegramBanOnLeave)
	cfg.TelegramAutoEnableRejoined = boolValue(first(values, "Telegram.auto_enable_rejoined", "auto_enable_rejoined", strconv.FormatBool(cfg.TelegramAutoEnableRejoined)), cfg.TelegramAutoEnableRejoined)
	cfg.TelegramEnablePanel = boolValue(first(values, "Telegram.enable_tg_panel", "enable_tg_panel", strconv.FormatBool(cfg.TelegramEnablePanel)), cfg.TelegramEnablePanel)
	cfg.TelegramBotStartText = first(values, "Telegram.bot_start_text", "bot_start_text", cfg.TelegramBotStartText)
	cfg.TelegramBotGroupStartText = first(values, "Telegram.bot_group_start_text", "bot_group_start_text", cfg.TelegramBotGroupStartText)
	cfg.TelegramBotStartTitle = first(values, "Telegram.bot_start_title", "bot_start_title", cfg.TelegramBotStartTitle)
	cfg.TelegramBotStartIntro = first(values, "Telegram.bot_start_intro", "bot_start_intro", cfg.TelegramBotStartIntro)
	cfg.TelegramBotBindPromptText = first(values, "Telegram.bot_bind_prompt_text", "bot_bind_prompt_text", cfg.TelegramBotBindPromptText)
	cfg.TelegramBotHelpText = first(values, "Telegram.bot_help_text", "bot_help_text", cfg.TelegramBotHelpText)
	cfg.TelegramBotAdminHelpText = first(values, "Telegram.bot_admin_help_text", "bot_admin_help_text", cfg.TelegramBotAdminHelpText)
	cfg.TelegramBotHelpHeader = first(values, "Telegram.bot_help_header", "bot_help_header", cfg.TelegramBotHelpHeader)
	cfg.TelegramBotHelpFooter = first(values, "Telegram.bot_help_footer", "bot_help_footer", cfg.TelegramBotHelpFooter)
	cfg.TelegramBotAbout = first(values, "Telegram.bot_about", "bot_about", cfg.TelegramBotAbout)
	cfg.TelegramCustomCommands = parseTelegramCommandReplies(first(values, "Telegram.bot_custom_commands", "bot_custom_commands", ""))
	cfg.BangumiEnabled = boolValue(first(values, "BangumiSync.enabled", "bangumi_sync_enabled", strconv.FormatBool(cfg.BangumiEnabled)), cfg.BangumiEnabled)
	cfg.BangumiWebhookSecret = first(values, "BangumiSync.webhook_secret", "webhook_secret", cfg.BangumiWebhookSecret)
	cfg.TMDBAPIKey = first(values, "Global.tmdb_api_key", "tmdb_api_key", cfg.TMDBAPIKey)
	cfg.TMDBAPIURL = first(values, "Global.tmdb_api_url", "tmdb_api_url", cfg.TMDBAPIURL)
	cfg.TMDBImageURL = first(values, "Global.tmdb_image_url", "tmdb_image_url", cfg.TMDBImageURL)
	cfg.BangumiToken = first(values, "Global.bangumi_token", "bangumi_token", cfg.BangumiToken)
	cfg.BangumiAPIURL = first(values, "Global.bangumi_api_url", "bangumi_api_url", cfg.BangumiAPIURL)
	cfg.BangumiAppID = first(values, "Global.bangumi_app_id", "bangumi_app_id", cfg.BangumiAppID)
	cfg.RegisterEnabled = boolValue(first(values, "SAR.register_mode", "Register.register_mode", "register_mode", strconv.FormatBool(cfg.RegisterEnabled)), cfg.RegisterEnabled)
	cfg.RegisterCodeLimit = boolValue(first(values, "SAR.register_code_limit", "Register.register_code_limit", "register_code_limit", strconv.FormatBool(cfg.RegisterCodeLimit)), cfg.RegisterCodeLimit)
	cfg.AllowPendingRegister = boolValue(first(values, "SAR.allow_pending_register", "Register.allow_pending_register", "allow_pending_register", strconv.FormatBool(cfg.AllowPendingRegister)), cfg.AllowPendingRegister)
	cfg.EmbyDirectRegisterEnabled = boolValue(first(values, "SAR.emby_direct_register_enabled", "Register.emby_direct_register_enabled", "emby_direct_register_enabled", strconv.FormatBool(cfg.EmbyDirectRegisterEnabled)), cfg.EmbyDirectRegisterEnabled)
	cfg.EmbyDirectRegisterDays = intValue(first(values, "SAR.emby_direct_register_days", "Register.emby_direct_register_days", "emby_direct_register_days", strconv.Itoa(cfg.EmbyDirectRegisterDays)), cfg.EmbyDirectRegisterDays)
	cfg.EmbyUserLimit = intValue(first(values, "SAR.emby_user_limit", "Register.emby_user_limit", "emby_user_limit", strconv.Itoa(cfg.EmbyUserLimit)), cfg.EmbyUserLimit)
	cfg.DecoyAction = first(values, "SAR.regcode_decoy_action", "Register.regcode_decoy_action", "regcode_decoy_action", cfg.DecoyAction)
	cfg.MediaRequestEnabled = boolValue(first(values, "SAR.media_request_enabled", "Register.media_request_enabled", "media_request_enabled", strconv.FormatBool(cfg.MediaRequestEnabled)), cfg.MediaRequestEnabled)
	cfg.MaxConcurrentRequestsPerUser = intValue(first(values, "SAR.max_concurrent_requests_per_user", "Register.max_concurrent_requests_per_user", "max_concurrent_requests_per_user", strconv.Itoa(cfg.MaxConcurrentRequestsPerUser)), cfg.MaxConcurrentRequestsPerUser)
	cfg.InviteEnabled = boolValue(first(values, "SAR.invite_enabled", "Register.invite_enabled", "invite_enabled", strconv.FormatBool(cfg.InviteEnabled)), cfg.InviteEnabled)
	cfg.InviteMaxDepth = intValue(first(values, "SAR.invite_max_depth", "Register.invite_max_depth", "invite_max_depth", strconv.Itoa(cfg.InviteMaxDepth)), cfg.InviteMaxDepth)
	cfg.InviteLimit = intValue(first(values, "SAR.invite_limit", "Register.invite_limit", "invite_limit", strconv.Itoa(cfg.InviteLimit)), cfg.InviteLimit)
	cfg.InviteRootUserLimit = intValue(first(values, "SAR.invite_root_user_limit", "Register.invite_root_user_limit", "invite_root_user_limit", strconv.Itoa(cfg.InviteRootUserLimit)), cfg.InviteRootUserLimit)
	cfg.InviteRequireEmby = boolValue(first(values, "SAR.invite_require_emby", "Register.invite_require_emby", "invite_require_emby", strconv.FormatBool(cfg.InviteRequireEmby)), cfg.InviteRequireEmby)
	cfg.InviteDefaultDays = intValue(first(values, "SAR.invite_code_default_days", "SAR.invite_default_days", "Register.invite_default_days", "invite_default_days", strconv.Itoa(cfg.InviteDefaultDays)), cfg.InviteDefaultDays)
	cfg.PermanentInviteMaxDays = intValue(first(values, "SAR.permanent_invite_max_days", "Register.permanent_invite_max_days", "permanent_invite_max_days", strconv.Itoa(cfg.PermanentInviteMaxDays)), cfg.PermanentInviteMaxDays)
	cfg.UserLimit = intValue(first(values, "SAR.user_limit", "Register.user_limit", "user_limit", strconv.Itoa(cfg.UserLimit)), cfg.UserLimit)
	cfg.AutoCleanupNoEmby = boolValue(first(values, "SAR.auto_cleanup_no_emby", "Register.auto_cleanup_no_emby", "auto_cleanup_no_emby", strconv.FormatBool(cfg.AutoCleanupNoEmby)), cfg.AutoCleanupNoEmby)
	cfg.AutoCleanupNoEmbyDays = intValue(first(values, "SAR.auto_cleanup_no_emby_days", "Register.auto_cleanup_no_emby_days", "auto_cleanup_no_emby_days", strconv.Itoa(cfg.AutoCleanupNoEmbyDays)), cfg.AutoCleanupNoEmbyDays)
	cfg.NotificationEnabled = boolValue(first(values, "Notification.enabled", "notification_enabled", strconv.FormatBool(cfg.NotificationEnabled)), cfg.NotificationEnabled)
	cfg.NotificationExpiryRemindDays = intValue(first(values, "Notification.expiry_remind_days", "expiry_remind_days", strconv.Itoa(cfg.NotificationExpiryRemindDays)), cfg.NotificationExpiryRemindDays)
	cfg.SchedulerEnabled = boolValue(first(values, "Scheduler.enabled", "scheduler_enabled", strconv.FormatBool(cfg.SchedulerEnabled)), cfg.SchedulerEnabled)
	cfg.SchedulerExpiredCheckTime = first(values, "Scheduler.expired_check_time", "expired_check_time", cfg.SchedulerExpiredCheckTime)
	cfg.SchedulerExpiringCheckTime = first(values, "Scheduler.expiring_check_time", "expiring_check_time", cfg.SchedulerExpiringCheckTime)
	cfg.SchedulerDailyStatsTime = first(values, "Scheduler.daily_stats_time", "daily_stats_time", cfg.SchedulerDailyStatsTime)
	cfg.SchedulerSessionCleanupInterval = intValue(first(values, "Scheduler.session_cleanup_interval", "session_cleanup_interval", strconv.Itoa(cfg.SchedulerSessionCleanupInterval)), cfg.SchedulerSessionCleanupInterval)
	cfg.SystemUpdateEnabled = boolValue(first(values, "SystemUpdate.auto_update_enabled", "auto_update_enabled", strconv.FormatBool(cfg.SystemUpdateEnabled)), cfg.SystemUpdateEnabled)
	cfg.SystemUpdateRepoURL = first(values, "SystemUpdate.repo_url", "repo_url", cfg.SystemUpdateRepoURL)
	cfg.SystemUpdateBranch = first(values, "SystemUpdate.branch", "branch", cfg.SystemUpdateBranch)
	cfg.SystemUpdateRestartServices = boolValue(first(values, "SystemUpdate.restart_services", "restart_services", strconv.FormatBool(cfg.SystemUpdateRestartServices)), cfg.SystemUpdateRestartServices)
	cfg.SystemUpdateTriggerType = first(values, "SystemUpdate.auto_update_trigger_type", "auto_update_trigger_type", cfg.SystemUpdateTriggerType)
	cfg.SystemUpdateIntervalHours = intValue(first(values, "SystemUpdate.auto_update_interval_hours", "auto_update_interval_hours", strconv.Itoa(cfg.SystemUpdateIntervalHours)), cfg.SystemUpdateIntervalHours)
	cfg.SystemUpdateTime = first(values, "SystemUpdate.auto_update_time", "auto_update_time", cfg.SystemUpdateTime)
	cfg.DeviceLimitEnabled = boolValue(first(values, "DeviceLimit.enabled", "device_limit_enabled", strconv.FormatBool(cfg.DeviceLimitEnabled)), cfg.DeviceLimitEnabled)
	cfg.MaxDevices = intValue(first(values, "DeviceLimit.max_devices", "max_devices", strconv.Itoa(cfg.MaxDevices)), cfg.MaxDevices)
	cfg.MaxStreams = intValue(first(values, "DeviceLimit.max_streams", "max_streams", strconv.Itoa(cfg.MaxStreams)), cfg.MaxStreams)

	applyEnv(&cfg)
	if cfg.StateFile == "" {
		cfg.StateFile = filepath.Join(cfg.DatabaseDir, "twilight_go_state.json")
	}
	if cfg.DatabaseBackupDir == "" {
		cfg.DatabaseBackupDir = filepath.Join(cfg.DatabaseDir, "backups")
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = "uploads"
	}
	return cfg, nil
}

func defaultConfigPath() string {
	return "config.toml"
}

func defaults() Config {
	return Config{
		AppName:                         "Twilight",
		Version:                         "0.0.4",
		Host:                            "0.0.0.0",
		Port:                            5000,
		DatabaseDir:                     "db",
		DatabaseDriver:                  "postgres",
		PostgresHost:                    "127.0.0.1",
		PostgresPort:                    5432,
		PostgresUser:                    "twilight",
		PostgresDatabase:                "twilight",
		PostgresSSLMode:                 "disable",
		PostgresMaxOpenConns:            8,
		PostgresMaxIdleConns:            4,
		UploadDir:                       "uploads",
		MaxUploadSize:                   5 * 1024 * 1024,
		LogLevel:                        "info",
		RuntimeLogLimit:                 5000,
		CORSOrigins:                     []string{"http://localhost:3000", "http://127.0.0.1:3000"},
		AllowCredential:                 true,
		TrustProxyHeaders:               false,
		SessionCookie:                   "twilight_session",
		SessionTTL:                      7 * 24 * time.Hour,
		CookieSameSite:                  "lax",
		TelegramAPIURL:                  "https://api.telegram.org",
		TelegramGroupCheckConcurrency:   24,
		TelegramGroupActionConcurrency:  8,
		AllowPendingRegister:            true,
		EmbyDirectRegisterDays:          30,
		EmbyUserLimit:                   -1,
		NotificationEnabled:             true,
		NotificationExpiryRemindDays:    3,
		AutoCleanupNoEmbyDays:           7,
		SchedulerEnabled:                true,
		SchedulerExpiredCheckTime:       "03:00",
		SchedulerExpiringCheckTime:      "09:00",
		SchedulerDailyStatsTime:         "00:05",
		SchedulerSessionCleanupInterval: 6,
		SystemUpdateRepoURL:             "https://github.com/Prejudice-Studio/Twilight.git",
		SystemUpdateBranch:              "main",
		SystemUpdateRestartServices:     true,
		SystemUpdateTriggerType:         "interval",
		SystemUpdateIntervalHours:       24,
		SystemUpdateTime:                "04:00",
		TMDBAPIURL:                      "https://api.themoviedb.org/3",
		TMDBImageURL:                    "https://image.tmdb.org/t/p",
		BangumiAPIURL:                   "https://api.bgm.tv/v0",
		MediaRequestEnabled:             true,
		MaxConcurrentRequestsPerUser:    3,
		InviteEnabled:                   true,
		InviteMaxDepth:                  3,
		InviteLimit:                     10,
		InviteRootUserLimit:             -1,
		InviteRequireEmby:               false,
		InviteDefaultDays:               30,
		PermanentInviteMaxDays:          365,
		UserLimit:                       -1,
		MaxDevices:                      5,
		MaxStreams:                      2,
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("TWILIGHT_API_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("TWILIGHT_GLOBAL_SERVER_NAME"); v != "" {
		cfg.AppName = v
	}
	if v := os.Getenv("TWILIGHT_SERVER_NAME"); v != "" {
		cfg.AppName = v
	}
	if v := os.Getenv("TWILIGHT_SERVER_ICON"); v != "" {
		cfg.ServerIcon = v
	}
	if v := os.Getenv("TWILIGHT_API_PORT"); v != "" {
		cfg.Port = intValue(v, cfg.Port)
	}
	if v := os.Getenv("TWILIGHT_REDIS_URL"); v != "" {
		cfg.RedisURL = v
	}
	if v := os.Getenv("TWILIGHT_LOG_LEVEL"); v != "" {
		cfg.LogLevel = normalizeLogLevel(v)
	}
	if v := os.Getenv("TWILIGHT_RUNTIME_LOG_LIMIT"); v != "" {
		cfg.RuntimeLogLimit = intValue(v, cfg.RuntimeLogLimit)
	}
	if v := os.Getenv("TWILIGHT_GLOBAL_REDIS_URL"); v != "" {
		cfg.RedisURL = v
	}
	if v := os.Getenv("TWILIGHT_DATABASES_DIR"); v != "" {
		cfg.DatabaseDir = v
	}
	if v := os.Getenv("TWILIGHT_DATABASE_DRIVER"); v != "" {
		cfg.DatabaseDriver = strings.ToLower(v)
	}
	if v := os.Getenv("TWILIGHT_DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_DSN"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("TWILIGHT_DATABASE_BACKUP_DIR"); v != "" {
		cfg.DatabaseBackupDir = v
	}
	if v := os.Getenv("TWILIGHT_DATABASE_MIGRATION_PANEL_ENABLED"); v != "" {
		cfg.DatabaseMigrationPanelEnabled = boolValue(v, cfg.DatabaseMigrationPanelEnabled)
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_HOST"); v != "" {
		cfg.PostgresHost = v
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_PORT"); v != "" {
		cfg.PostgresPort = intValue(v, cfg.PostgresPort)
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_USER"); v != "" {
		cfg.PostgresUser = v
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_PASSWORD"); v != "" {
		cfg.PostgresPassword = v
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_DATABASE"); v != "" {
		cfg.PostgresDatabase = v
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_SSLMODE"); v != "" {
		cfg.PostgresSSLMode = v
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_MAX_OPEN_CONNS"); v != "" {
		cfg.PostgresMaxOpenConns = intValue(v, cfg.PostgresMaxOpenConns)
	}
	if v := os.Getenv("TWILIGHT_POSTGRES_MAX_IDLE_CONNS"); v != "" {
		cfg.PostgresMaxIdleConns = intValue(v, cfg.PostgresMaxIdleConns)
	}
	if v := os.Getenv("TWILIGHT_STATE_FILE"); v != "" {
		cfg.StateFile = v
	}
	if v := os.Getenv("TWILIGHT_API_UPLOAD_FOLDER"); v != "" {
		cfg.UploadDir = v
	}
	if v := os.Getenv("TWILIGHT_API_MAX_UPLOAD_SIZE"); v != "" {
		cfg.MaxUploadSize = int64Value(v, cfg.MaxUploadSize)
	}
	if v := os.Getenv("TWILIGHT_API_CORS_ORIGINS"); v != "" {
		cfg.CORSOrigins = listValue(v)
	}
	if v := os.Getenv("TWILIGHT_TRUST_PROXY_HEADERS"); v != "" {
		cfg.TrustProxyHeaders = boolValue(v, cfg.TrustProxyHeaders)
	}
	if v := os.Getenv("TWILIGHT_SESSION_COOKIE_NAME"); v != "" {
		cfg.SessionCookie = v
	}
	if v := os.Getenv("TWILIGHT_SESSION_COOKIE_SECURE"); v != "" {
		cfg.CookieSecure = boolValue(v, cfg.CookieSecure)
	}
	if v := os.Getenv("TWILIGHT_SESSION_COOKIE_SAMESITE"); v != "" {
		cfg.CookieSameSite = strings.ToLower(v)
	}
	if v := os.Getenv("TWILIGHT_SESSION_TTL_SECONDS"); v != "" {
		seconds := int64Value(v, int64(cfg.SessionTTL/time.Second))
		cfg.SessionTTL = time.Duration(seconds) * time.Second
	}
	if v := os.Getenv("TWILIGHT_TMDB_API_KEY"); v != "" {
		cfg.TMDBAPIKey = v
	}
	if v := os.Getenv("TWILIGHT_BANGUMI_TOKEN"); v != "" {
		cfg.BangumiToken = v
	}
	if v := os.Getenv("TWILIGHT_EMBY_TOKEN"); v != "" {
		cfg.EmbyToken = v
	}
	if v := os.Getenv("TWILIGHT_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.TelegramBotToken = v
	}
	if v := os.Getenv("TWILIGHT_TELEGRAM_ADMIN_ID"); v != "" {
		cfg.TelegramAdminIDs = int64ListValue(v)
	}
	if v := os.Getenv("TWILIGHT_TELEGRAM_GROUP_ID"); v != "" {
		cfg.TelegramGroupIDs = listValue(v)
	}
	if v := os.Getenv("TWILIGHT_TELEGRAM_REQUIRE_GROUP_MEMBERSHIP"); v != "" {
		cfg.TelegramRequireMembership = boolValue(v, cfg.TelegramRequireMembership)
	}
	if v := os.Getenv("TWILIGHT_TELEGRAM_BAN_ON_LEAVE"); v != "" {
		cfg.TelegramBanOnLeave = boolValue(v, cfg.TelegramBanOnLeave)
	}
	if v := os.Getenv("TWILIGHT_SYSTEM_UPDATE_ENABLED"); v != "" {
		cfg.SystemUpdateEnabled = boolValue(v, cfg.SystemUpdateEnabled)
	}
	if v := os.Getenv("TWILIGHT_SYSTEM_UPDATE_REPO_URL"); v != "" {
		cfg.SystemUpdateRepoURL = v
	}
	if v := os.Getenv("TWILIGHT_SYSTEM_UPDATE_BRANCH"); v != "" {
		cfg.SystemUpdateBranch = v
	}
	if v := os.Getenv("TWILIGHT_BOT_INTERNAL_SECRET"); v != "" {
		cfg.BotInternalSecret = v
	}
	if v := os.Getenv("TWILIGHT_NOTIFICATION_ENABLED"); v != "" {
		cfg.NotificationEnabled = boolValue(v, cfg.NotificationEnabled)
	}
	if v := os.Getenv("TWILIGHT_NOTIFICATION_EXPIRY_REMIND_DAYS"); v != "" {
		cfg.NotificationExpiryRemindDays = intValue(v, cfg.NotificationExpiryRemindDays)
	}
}

func readTOML(path string, values map[string]string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	section := ""
	var pendingKey, pendingFull string
	var pending strings.Builder
	pendingDepth := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if pendingKey != "" {
			if line != "" {
				pending.WriteByte(' ')
				pending.WriteString(line)
				pendingDepth += bracketDelta(line)
			}
			if pendingDepth > 0 {
				continue
			}
			value := normalizeValue(pending.String())
			values[pendingFull] = value
			values[pendingKey] = value
			pending.Reset()
			pendingKey, pendingFull = "", ""
			pendingDepth = 0
			continue
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.Trim(line, "[]"))
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		full := key
		if section != "" {
			full = section + "." + key
		}
		if depth := bracketDelta(val); depth > 0 {
			pendingKey = key
			pendingFull = full
			pending.WriteString(val)
			pendingDepth = depth
			continue
		}
		val = normalizeValue(val)
		values[full] = val
		values[key] = val
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if pendingKey != "" {
		return fmt.Errorf("unterminated TOML array for key %s", pendingFull)
	}
	return nil
}

func normalizeValue(value string) string {
	value = strings.TrimSpace(value)
	// Strip outer quotes and unescape TOML string escape sequences
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		value = value[1 : len(value)-1]
	}
	return unescapeTomlString(value)
}

// unescapeTomlString handles common TOML escape sequences within string values.
func unescapeTomlString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '"':
				b.WriteByte('"')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func bracketDelta(line string) int {
	inQuote := rune(0)
	escaped := false
	delta := 0
	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuote != 0 {
			escaped = true
			continue
		}
		if r == '\'' || r == '"' {
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
			continue
		}
		if inQuote != 0 {
			continue
		}
		switch r {
		case '[':
			delta++
		case ']':
			delta--
		}
	}
	return delta
}

func stripComment(line string) string {
	inQuote := rune(0)
	for i, r := range line {
		if r == '\'' || r == '"' {
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
		}
		if r == '#' && inQuote == 0 {
			return line[:i]
		}
	}
	return line
}

func first(values map[string]string, keys ...string) string {
	def := ""
	if len(keys) > 0 {
		def = keys[len(keys)-1]
	}
	for _, key := range keys[:len(keys)-1] {
		if value, ok := values[key]; ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return def
}

func intValue(value string, fallback int) int {
	i, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return i
}

func int64Value(value string, fallback int64) int64 {
	i, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	return i
}

func boolValue(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func normalizeLogLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "10", "debug":
		return "debug"
	case "20", "info", "":
		return "info"
	case "30", "warn", "warning":
		return "warn"
	case "40", "error":
		return "error"
	default:
		return "info"
	}
}

func (c Config) SlogLevel() slog.Level {
	switch normalizeLogLevel(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func listValue(value string) []string {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if value == "" {
		return nil
	}
	parts := splitRespectingQuotes(value)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// Strip surrounding quotes and unescape
		if (strings.HasPrefix(part, "\"") && strings.HasSuffix(part, "\"")) ||
			(strings.HasPrefix(part, "'") && strings.HasSuffix(part, "'")) {
			part = unescapeTomlString(part[1 : len(part)-1])
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// splitRespectingQuotes splits a string by commas but respects quoted segments.
func splitRespectingQuotes(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := rune(0)
	escaped := false
	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && inQuote == '"' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if (r == '"' || r == '\'') && inQuote == 0 {
			inQuote = r
			current.WriteRune(r)
			continue
		}
		if r == inQuote {
			inQuote = 0
			current.WriteRune(r)
			continue
		}
		if r == ',' && inQuote == 0 {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func int64ListValue(value string) []int64 {
	parts := listValue(value)
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && parsed != 0 {
			out = append(out, parsed)
		}
	}
	return out
}

func int64ListToStrings(values []int64) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != 0 {
			out = append(out, strconv.FormatInt(value, 10))
		}
	}
	return out
}

func parseLines(value string) []Line {
	parts := listValue(value)
	lines := make([]Line, 0, len(parts))
	for _, part := range parts {
		name := "默认线路"
		rawURL := part
		if left, right, ok := strings.Cut(part, " : "); ok {
			name = left
			rawURL = right
		} else if left, right, ok := strings.Cut(part, "| "); ok {
			name = left
			rawURL = right
		}
		if !strings.Contains(rawURL, "://") {
			name = "默认线路"
			rawURL = part
		}
		name = strings.TrimSpace(name)
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		lines = append(lines, Line{Name: name, URL: rawURL})
	}
	return lines
}

func parseTelegramCommandReplies(value string) []TelegramCommandReply {
	parts := listValue(value)
	out := make([]TelegramCommandReply, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		command, reply, ok := strings.Cut(part, " = ")
		if !ok {
			command, reply, ok = strings.Cut(part, "=")
		}
		if !ok {
			command, reply, ok = strings.Cut(part, " : ")
		}
		if !ok {
			command, reply, ok = strings.Cut(part, "| ")
		}
		if !ok {
			continue
		}
		command = normalizeTelegramCommand(command)
		reply = strings.TrimSpace(reply)
		if command == "" || reply == "" || seen[command] {
			continue
		}
		seen[command] = true
		out = append(out, TelegramCommandReply{Command: command, Reply: reply})
	}
	return out
}

func normalizeTelegramCommand(value string) string {
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

func (c Config) PostgresDSN() string {
	if strings.TrimSpace(c.DatabaseURL) != "" {
		return strings.TrimSpace(c.DatabaseURL)
	}
	if c.PostgresHost == "" || c.PostgresDatabase == "" || c.PostgresUser == "" {
		return ""
	}
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(c.PostgresHost, strconv.Itoa(c.PostgresPort)),
		Path:   "/" + c.PostgresDatabase,
	}
	if c.PostgresPassword == "" {
		u.User = url.User(c.PostgresUser)
	} else {
		u.User = url.UserPassword(c.PostgresUser, c.PostgresPassword)
	}
	q := u.Query()
	if c.PostgresSSLMode != "" {
		q.Set("sslmode", c.PostgresSSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
