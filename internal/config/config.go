package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
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
	AdminUIDs                     []int64
	AdminUsernames                []string

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

	reader := newViperConfigReader()
	if err := reader.mergeFile(path); err != nil {
		return cfg, err
	}
	local := os.Getenv("TWILIGHT_CONFIG_LOCAL_FILE")
	if local == "" {
		local = strings.TrimSuffix(path, filepath.Ext(path)) + ".local" + filepath.Ext(path)
	}
	if err := reader.mergeFile(local); err != nil {
		return cfg, err
	}

	cfg.AppName = reader.stringValue(cfg.AppName, "Global.server_name", "server_name")
	cfg.ServerIcon = reader.stringValue(cfg.ServerIcon, "Global.server_icon", "server_icon")
	cfg.RedisURL = reader.stringValue(cfg.RedisURL, "Global.redis_url", "redis_url")
	cfg.LogLevel = normalizeLogLevel(reader.stringValue(cfg.LogLevel, "Global.log_level", "log_level"))
	cfg.RuntimeLogLimit = reader.intValue(cfg.RuntimeLogLimit, "Global.runtime_log_limit", "runtime_log_limit")
	cfg.AdminUIDs = reader.int64ListValue(cfg.AdminUIDs, "Admin.uids", "Admin.admin_uids", "SAR.admin_uids", "admin_uids")
	cfg.AdminUsernames = reader.stringListValue(cfg.AdminUsernames, "Admin.usernames", "Admin.admin_usernames", "Admin.users", "SAR.admin_usernames", "admin_usernames")
	cfg.DatabaseDir = reader.stringValue(cfg.DatabaseDir, "Global.databases_dir", "databases_dir")
	cfg.DatabaseDriver = strings.ToLower(reader.stringValue(cfg.DatabaseDriver, "Database.driver", "Global.database_driver", "database_driver"))
	cfg.DatabaseURL = reader.stringValue(cfg.DatabaseURL, "Database.url", "Database.database_url", "Global.database_url", "database_url")
	cfg.DatabaseBackupDir = reader.stringValue(cfg.DatabaseBackupDir, "Database.backup_dir", "database_backup_dir")
	cfg.DatabaseMigrationPanelEnabled = reader.boolValue(cfg.DatabaseMigrationPanelEnabled, "Database.migration_panel_enabled", "database_migration_panel_enabled")
	cfg.StateFile = reader.stringValue(cfg.StateFile, "Database.state_file", "Global.state_file", "state_file")
	cfg.PostgresHost = reader.stringValue(cfg.PostgresHost, "Database.postgres_host", "PostgreSQL.host", "postgres_host")
	cfg.PostgresPort = reader.intValue(cfg.PostgresPort, "Database.postgres_port", "PostgreSQL.port", "postgres_port")
	cfg.PostgresUser = reader.stringValue(cfg.PostgresUser, "Database.postgres_user", "PostgreSQL.user", "postgres_user")
	cfg.PostgresPassword = reader.stringValue(cfg.PostgresPassword, "Database.postgres_password", "PostgreSQL.password", "postgres_password")
	cfg.PostgresDatabase = reader.stringValue(cfg.PostgresDatabase, "Database.postgres_database", "PostgreSQL.database", "postgres_database")
	cfg.PostgresSSLMode = reader.stringValue(cfg.PostgresSSLMode, "Database.postgres_sslmode", "PostgreSQL.sslmode", "postgres_sslmode")
	cfg.PostgresMaxOpenConns = reader.intValue(cfg.PostgresMaxOpenConns, "Database.postgres_max_open_conns", "PostgreSQL.max_open_conns", "postgres_max_open_conns")
	cfg.PostgresMaxIdleConns = reader.intValue(cfg.PostgresMaxIdleConns, "Database.postgres_max_idle_conns", "PostgreSQL.max_idle_conns", "postgres_max_idle_conns")
	cfg.UploadDir = reader.stringValue(cfg.UploadDir, "API.upload_folder", "upload_folder")
	cfg.Host = reader.stringValue(cfg.Host, "API.host", "host")
	cfg.Port = reader.intValue(cfg.Port, "API.port", "port")
	cfg.MaxUploadSize = reader.int64Value(cfg.MaxUploadSize, "API.max_upload_size", "max_upload_size")
	cfg.CORSOrigins = reader.stringListValue(cfg.CORSOrigins, "API.cors_origins", "cors_origins")
	cfg.TrustProxyHeaders = reader.boolValue(cfg.TrustProxyHeaders, "API.trust_proxy_headers", "trust_proxy_headers")
	cfg.SessionCookie = reader.stringValue(cfg.SessionCookie, "Security.session_cookie_name", "API.session_cookie_name", "session_cookie_name")
	cfg.BotInternalSecret = reader.stringValue(cfg.BotInternalSecret, "Security.bot_internal_secret", "bot_internal_secret")
	cfg.EmbyURL = reader.stringValue(cfg.EmbyURL, "Emby.emby_url", "emby_url")
	cfg.EmbyToken = reader.stringValue(cfg.EmbyToken, "Emby.emby_token", "emby_token")
	cfg.EmbyUsername = reader.stringValue(cfg.EmbyUsername, "Emby.emby_username", "emby_username")
	cfg.EmbyPassword = reader.stringValue(cfg.EmbyPassword, "Emby.emby_password", "emby_password")
	cfg.EmbyPublicURL = reader.stringValue(cfg.EmbyPublicURL, "Emby.emby_public_url", "emby_public_url")
	cfg.EmbyWhitelistURL = reader.stringValue(cfg.EmbyWhitelistURL, "Emby.emby_whitelist_url", "emby_whitelist_url")
	cfg.EmbyURLList = parseLinesList(reader.stringListValue(nil, "Emby.emby_url_list", "emby_url_list"))
	cfg.EmbyWhitelistURLList = parseLinesList(reader.stringListValue(nil, "Emby.emby_url_list_for_whitelist", "emby_url_list_for_whitelist"))
	cfg.EmbyDefaultHiddenLibraries = reader.stringListValue(nil, "Emby.emby_default_hidden_libraries", "emby_default_hidden_libraries")
	cfg.EmbySelfServiceLibraries = reader.stringListValue(nil, "Emby.emby_self_service_libraries", "emby_self_service_libraries")
	cfg.TelegramMode = reader.boolValue(cfg.TelegramMode, "Global.telegram_mode", "telegram_mode")
	cfg.ForceBindTelegram = reader.boolValue(cfg.ForceBindTelegram, "Global.force_bind_telegram", "force_bind_telegram")
	cfg.TelegramBotToken = reader.stringValue(cfg.TelegramBotToken, "Telegram.bot_token", "bot_token")
	cfg.TelegramAPIURL = reader.stringValue(cfg.TelegramAPIURL, "Telegram.telegram_api_url", "telegram_api_url")
	cfg.TelegramAdminIDs = reader.int64ListValue(cfg.TelegramAdminIDs, "Telegram.admin_id", "admin_id")
	cfg.TelegramGroupIDs = reader.stringListValue(cfg.TelegramGroupIDs, "Telegram.group_id", "group_id")
	cfg.TelegramChannelIDs = reader.stringListValue(cfg.TelegramChannelIDs, "Telegram.channel_id", "channel_id")
	cfg.TelegramForceSubscribe = reader.boolValue(cfg.TelegramForceSubscribe, "Telegram.force_subscribe", "force_subscribe")
	cfg.TelegramRequireMembership = reader.boolValue(cfg.TelegramRequireMembership, "Telegram.require_group_membership", "require_group_membership")
	cfg.TelegramGroupCheckConcurrency = reader.intValue(cfg.TelegramGroupCheckConcurrency, "Telegram.group_check_concurrency", "group_check_concurrency")
	cfg.TelegramGroupActionConcurrency = reader.intValue(cfg.TelegramGroupActionConcurrency, "Telegram.group_action_concurrency", "group_action_concurrency")
	cfg.TelegramBanOnLeave = reader.boolValue(cfg.TelegramBanOnLeave, "Telegram.ban_on_leave", "ban_on_leave")
	cfg.TelegramAutoEnableRejoined = reader.boolValue(cfg.TelegramAutoEnableRejoined, "Telegram.auto_enable_rejoined", "auto_enable_rejoined")
	cfg.TelegramEnablePanel = reader.boolValue(cfg.TelegramEnablePanel, "Telegram.enable_tg_panel", "enable_tg_panel")
	cfg.TelegramBotStartText = reader.stringValue(cfg.TelegramBotStartText, "Telegram.bot_start_text", "bot_start_text")
	cfg.TelegramBotGroupStartText = reader.stringValue(cfg.TelegramBotGroupStartText, "Telegram.bot_group_start_text", "bot_group_start_text")
	cfg.TelegramBotStartTitle = reader.stringValue(cfg.TelegramBotStartTitle, "Telegram.bot_start_title", "bot_start_title")
	cfg.TelegramBotStartIntro = reader.stringValue(cfg.TelegramBotStartIntro, "Telegram.bot_start_intro", "bot_start_intro")
	cfg.TelegramBotBindPromptText = reader.stringValue(cfg.TelegramBotBindPromptText, "Telegram.bot_bind_prompt_text", "bot_bind_prompt_text")
	cfg.TelegramBotHelpText = reader.stringValue(cfg.TelegramBotHelpText, "Telegram.bot_help_text", "bot_help_text")
	cfg.TelegramBotAdminHelpText = reader.stringValue(cfg.TelegramBotAdminHelpText, "Telegram.bot_admin_help_text", "bot_admin_help_text")
	cfg.TelegramBotHelpHeader = reader.stringValue(cfg.TelegramBotHelpHeader, "Telegram.bot_help_header", "bot_help_header")
	cfg.TelegramBotHelpFooter = reader.stringValue(cfg.TelegramBotHelpFooter, "Telegram.bot_help_footer", "bot_help_footer")
	cfg.TelegramBotAbout = reader.stringValue(cfg.TelegramBotAbout, "Telegram.bot_about", "bot_about")
	cfg.TelegramCustomCommands = parseTelegramCommandRepliesList(reader.stringListValue(nil, "Telegram.bot_custom_commands", "bot_custom_commands"))
	cfg.BangumiEnabled = reader.boolValue(cfg.BangumiEnabled, "BangumiSync.enabled", "bangumi_sync_enabled")
	cfg.BangumiWebhookSecret = reader.stringValue(cfg.BangumiWebhookSecret, "BangumiSync.webhook_secret", "webhook_secret")
	cfg.TMDBAPIKey = reader.stringValue(cfg.TMDBAPIKey, "Global.tmdb_api_key", "tmdb_api_key")
	cfg.TMDBAPIURL = reader.stringValue(cfg.TMDBAPIURL, "Global.tmdb_api_url", "tmdb_api_url")
	cfg.TMDBImageURL = reader.stringValue(cfg.TMDBImageURL, "Global.tmdb_image_url", "tmdb_image_url")
	cfg.BangumiToken = reader.stringValue(cfg.BangumiToken, "Global.bangumi_token", "bangumi_token")
	cfg.BangumiAPIURL = reader.stringValue(cfg.BangumiAPIURL, "Global.bangumi_api_url", "bangumi_api_url")
	cfg.BangumiAppID = reader.stringValue(cfg.BangumiAppID, "Global.bangumi_app_id", "bangumi_app_id")
	cfg.RegisterEnabled = reader.boolValue(cfg.RegisterEnabled, "SAR.register_mode", "Register.register_mode", "register_mode")
	cfg.RegisterCodeLimit = reader.boolValue(cfg.RegisterCodeLimit, "SAR.register_code_limit", "Register.register_code_limit", "register_code_limit")
	cfg.AllowPendingRegister = reader.boolValue(cfg.AllowPendingRegister, "SAR.allow_pending_register", "Register.allow_pending_register", "allow_pending_register")
	cfg.EmbyDirectRegisterEnabled = reader.boolValue(cfg.EmbyDirectRegisterEnabled, "SAR.emby_direct_register_enabled", "Register.emby_direct_register_enabled", "emby_direct_register_enabled")
	cfg.EmbyDirectRegisterDays = reader.intValue(cfg.EmbyDirectRegisterDays, "SAR.emby_direct_register_days", "Register.emby_direct_register_days", "emby_direct_register_days")
	cfg.EmbyUserLimit = reader.intValue(cfg.EmbyUserLimit, "SAR.emby_user_limit", "Register.emby_user_limit", "emby_user_limit")
	cfg.DecoyAction = reader.stringValue(cfg.DecoyAction, "SAR.regcode_decoy_action", "Register.regcode_decoy_action", "regcode_decoy_action")
	cfg.MediaRequestEnabled = reader.boolValue(cfg.MediaRequestEnabled, "SAR.media_request_enabled", "Register.media_request_enabled", "media_request_enabled")
	cfg.MaxConcurrentRequestsPerUser = reader.intValue(cfg.MaxConcurrentRequestsPerUser, "SAR.max_concurrent_requests_per_user", "Register.max_concurrent_requests_per_user", "max_concurrent_requests_per_user")
	cfg.InviteEnabled = reader.boolValue(cfg.InviteEnabled, "SAR.invite_enabled", "Register.invite_enabled", "invite_enabled")
	cfg.InviteMaxDepth = reader.intValue(cfg.InviteMaxDepth, "SAR.invite_max_depth", "Register.invite_max_depth", "invite_max_depth")
	cfg.InviteLimit = reader.intValue(cfg.InviteLimit, "SAR.invite_limit", "Register.invite_limit", "invite_limit")
	cfg.InviteRootUserLimit = reader.intValue(cfg.InviteRootUserLimit, "SAR.invite_root_user_limit", "Register.invite_root_user_limit", "invite_root_user_limit")
	cfg.InviteRequireEmby = reader.boolValue(cfg.InviteRequireEmby, "SAR.invite_require_emby", "Register.invite_require_emby", "invite_require_emby")
	cfg.InviteDefaultDays = reader.intValue(cfg.InviteDefaultDays, "SAR.invite_code_default_days", "SAR.invite_default_days", "Register.invite_default_days", "invite_default_days")
	cfg.PermanentInviteMaxDays = reader.intValue(cfg.PermanentInviteMaxDays, "SAR.permanent_invite_max_days", "Register.permanent_invite_max_days", "permanent_invite_max_days")
	cfg.UserLimit = reader.intValue(cfg.UserLimit, "SAR.user_limit", "Register.user_limit", "user_limit")
	cfg.AutoCleanupNoEmby = reader.boolValue(cfg.AutoCleanupNoEmby, "SAR.auto_cleanup_no_emby", "Register.auto_cleanup_no_emby", "auto_cleanup_no_emby")
	cfg.AutoCleanupNoEmbyDays = reader.intValue(cfg.AutoCleanupNoEmbyDays, "SAR.auto_cleanup_no_emby_days", "Register.auto_cleanup_no_emby_days", "auto_cleanup_no_emby_days")
	cfg.NotificationEnabled = reader.boolValue(cfg.NotificationEnabled, "Notification.enabled", "notification_enabled")
	cfg.NotificationExpiryRemindDays = reader.intValue(cfg.NotificationExpiryRemindDays, "Notification.expiry_remind_days", "expiry_remind_days")
	cfg.SchedulerEnabled = reader.boolValue(cfg.SchedulerEnabled, "Scheduler.enabled", "scheduler_enabled")
	cfg.SchedulerExpiredCheckTime = reader.stringValue(cfg.SchedulerExpiredCheckTime, "Scheduler.expired_check_time", "expired_check_time")
	cfg.SchedulerExpiringCheckTime = reader.stringValue(cfg.SchedulerExpiringCheckTime, "Scheduler.expiring_check_time", "expiring_check_time")
	cfg.SchedulerDailyStatsTime = reader.stringValue(cfg.SchedulerDailyStatsTime, "Scheduler.daily_stats_time", "daily_stats_time")
	cfg.SchedulerSessionCleanupInterval = reader.intValue(cfg.SchedulerSessionCleanupInterval, "Scheduler.session_cleanup_interval", "session_cleanup_interval")
	cfg.SystemUpdateEnabled = reader.boolValue(cfg.SystemUpdateEnabled, "SystemUpdate.auto_update_enabled", "auto_update_enabled")
	cfg.SystemUpdateRepoURL = reader.stringValue(cfg.SystemUpdateRepoURL, "SystemUpdate.repo_url", "repo_url")
	cfg.SystemUpdateBranch = reader.stringValue(cfg.SystemUpdateBranch, "SystemUpdate.branch", "branch")
	cfg.SystemUpdateRestartServices = reader.boolValue(cfg.SystemUpdateRestartServices, "SystemUpdate.restart_services", "restart_services")
	cfg.SystemUpdateTriggerType = reader.stringValue(cfg.SystemUpdateTriggerType, "SystemUpdate.auto_update_trigger_type", "auto_update_trigger_type")
	cfg.SystemUpdateIntervalHours = reader.intValue(cfg.SystemUpdateIntervalHours, "SystemUpdate.auto_update_interval_hours", "auto_update_interval_hours")
	cfg.SystemUpdateTime = reader.stringValue(cfg.SystemUpdateTime, "SystemUpdate.auto_update_time", "auto_update_time")
	cfg.DeviceLimitEnabled = reader.boolValue(cfg.DeviceLimitEnabled, "DeviceLimit.enabled", "DeviceLimit.device_limit_enabled", "device_limit_enabled")
	cfg.MaxDevices = reader.intValue(cfg.MaxDevices, "DeviceLimit.max_devices", "max_devices")
	cfg.MaxStreams = reader.intValue(cfg.MaxStreams, "DeviceLimit.max_streams", "max_streams")

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
	if v := os.Getenv("TWILIGHT_ADMIN_UIDS"); v != "" {
		cfg.AdminUIDs = int64ListValue(v)
	}
	if v := os.Getenv("TWILIGHT_ADMIN_USERNAMES"); v != "" {
		cfg.AdminUsernames = listValue(v)
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

type viperConfigReader struct {
	v *viper.Viper
}

func newViperConfigReader() viperConfigReader {
	v := viper.New()
	v.SetConfigType("toml")
	return viperConfigReader{v: v}
}

func (r viperConfigReader) mergeFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return r.v.MergeConfig(bytes.NewReader(data))
}

func (r viperConfigReader) rawValue(keys ...string) (any, bool) {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if r.v.IsSet(key) {
			return r.v.Get(key), true
		}
		if !strings.Contains(key, ".") {
			if value, ok := findBareSetting(r.v.AllSettings(), key); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func findBareSetting(settings map[string]any, key string) (any, bool) {
	needle := strings.ToLower(strings.TrimSpace(key))
	if needle == "" {
		return nil, false
	}
	var found any
	ok := false
	var walk func(map[string]any)
	walk = func(m map[string]any) {
		for k, v := range m {
			if strings.EqualFold(k, needle) {
				if _, nested := v.(map[string]any); !nested {
					found = v
					ok = true
				}
			}
			if child, nested := v.(map[string]any); nested {
				walk(child)
			}
		}
	}
	walk(settings)
	return found, ok
}

func (r viperConfigReader) stringValue(fallback string, keys ...string) string {
	raw, ok := r.rawValue(keys...)
	if !ok {
		return fallback
	}
	text, ok := rawToString(raw)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return strings.TrimSpace(text)
}

func rawToString(raw any) (string, bool) {
	switch value := raw.(type) {
	case nil:
		return "", false
	case string:
		return strings.TrimSpace(value), true
	case fmt.Stringer:
		return strings.TrimSpace(value.String()), true
	default:
		return strings.TrimSpace(fmt.Sprint(value)), true
	}
}

func (r viperConfigReader) intValue(fallback int, keys ...string) int {
	raw, ok := r.rawValue(keys...)
	if !ok {
		return fallback
	}
	switch value := raw.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	}
	text, ok := rawToString(raw)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return intValue(text, fallback)
}

func (r viperConfigReader) int64Value(fallback int64, keys ...string) int64 {
	raw, ok := r.rawValue(keys...)
	if !ok {
		return fallback
	}
	switch value := raw.(type) {
	case int:
		return int64(value)
	case int8:
		return int64(value)
	case int16:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case uint:
		return int64(value)
	case uint8:
		return int64(value)
	case uint16:
		return int64(value)
	case uint32:
		return int64(value)
	case uint64:
		return int64(value)
	case float32:
		return int64(value)
	case float64:
		return int64(value)
	}
	text, ok := rawToString(raw)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return int64Value(text, fallback)
}

func (r viperConfigReader) boolValue(fallback bool, keys ...string) bool {
	raw, ok := r.rawValue(keys...)
	if !ok {
		return fallback
	}
	switch value := raw.(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	}
	text, ok := rawToString(raw)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return boolValue(text, fallback)
}

func (r viperConfigReader) stringListValue(fallback []string, keys ...string) []string {
	raw, ok := r.rawValue(keys...)
	if !ok {
		return cloneStringSlice(fallback)
	}
	list, ok := rawToStringList(raw)
	if !ok {
		return cloneStringSlice(fallback)
	}
	return list
}

func rawToStringList(raw any) ([]string, bool) {
	switch value := raw.(type) {
	case nil:
		return nil, false
	case []string:
		return cleanStringList(value), true
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		if len(out) == 0 {
			return nil, true
		}
		return out, true
	case []int:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, strconv.Itoa(item))
		}
		return out, true
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, false
		}
		return listValue(value), true
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil, false
		}
		return listValue(text), true
	}
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func (r viperConfigReader) int64ListValue(fallback []int64, keys ...string) []int64 {
	raw, ok := r.rawValue(keys...)
	if !ok {
		return cloneInt64Slice(fallback)
	}
	items, ok := rawToStringList(raw)
	if !ok {
		return cloneInt64Slice(fallback)
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		parsed, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
		if err == nil && parsed != 0 {
			out = append(out, parsed)
		}
	}
	return out
}

func cloneInt64Slice(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]int64, len(values))
	copy(out, values)
	return out
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

func (c Config) ZapLevel() zapcore.Level {
	switch normalizeLogLevel(c.LogLevel) {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// unescapeTomlString keeps legacy comma-list parsing compatible with TOML string escapes.
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
	return parseLinesList(listValue(value))
}

func parseLinesList(parts []string) []Line {
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
	return parseTelegramCommandRepliesList(listValue(value))
}

func parseTelegramCommandRepliesList(parts []string) []TelegramCommandReply {
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
