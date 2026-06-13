// 前端错误码镜像表 —— 对齐 internal/api/errcode.go。
// 设计目标：
//   1. 单一真源在后端；前端镜像负责把 ApiResponse.error_code 收成判别字面量
//      联合，新增 / 改名时 TS 编译器立刻报错，不再靠中文 message 做正则。
//   2. 后端不在表里的码视为"未知 / 未来兼容"——ApiResponse.error_code 仍可
//      是宽松 string，避免老接口下发新码时前端崩；isKnownErrCode() 提供运行
//      时窄化以让 friendlyError / 业务分支安全消费。
//   3. 与 ERROR_CODE_FRIENDLY（webui/src/lib/validators.ts）配合：友好映射
//      用 Partial<Record<ErrCode, string>> 类型，新增码若漏配 friendly 文案，
//      falls back 到后端 message 而非编译失败，保留增量落地空间。
// 同步规则（每次后端改 errcode.go 必走）：
//   1. 在本文件追加常量 + 字面量
//   2. 视情况把文案补到 webui/src/lib/validators.ts ERROR_CODE_FRIENDLY
//   3. CI（脚本未来可加）通过 `grep -E '^\s*Err[A-Z][a-zA-Z]+\s+ErrCode'`
//      校对两侧条目数一致

/**
 * 后端业务错误码字面量联合。
 * 与 internal/api/errcode.go 的 const 块严格 1:1。
 */
export type ErrCode =
  // === 鉴权 / 会话 ===
  | "AUTH_LOGIN_RATE_LIMITED"
  | "AUTH_LOGIN_INVALID"
  | "AUTH_ACCOUNT_DISABLED"
  | "AUTH_ACCOUNT_EXPIRED"
  | "AUTH_SESSION_CREATE_FAILED"
  | "AUTH_APIKEY_EMPTY"
  | "AUTH_APIKEY_INVALID"
  | "AUTH_DIRECT_LOGIN_DISABLED"
  | "AUTH_PASSWORD_RESET_TOO_MANY"
  | "AUTH_PASSWORD_OLD_MISMATCH"
  | "AUTH_PASSWORD_WEAK"
  | "AUTH_PASSWORD_HASH_FAILED"
  | "AUTH_FORGOT_PASSWORD_DISABLED"
  // === 工单 ===
  | "TICKET_DISABLED"
  | "TICKET_NOT_FOUND"
  | "TICKET_RATE_LIMITED"
  // === 操作日志 ===
  | "AUDIT_LOG_NOT_FOUND"
  // === 用户 / 注册 ===
  | "USER_REGISTER_RATE_LIMITED"
  | "USER_REGISTER_DISABLED"
  | "USER_USERNAME_INVALID"
  | "USER_USERNAME_TAKEN"
  | "USER_NOT_FOUND"
  | "USER_LIMIT_REACHED"
  | "USER_PROTECTED"
  // === Telegram 绑定 ===
  | "TG_BIND_REQUIRED"
  | "TG_BIND_CODE_FORMAT_INVALID"
  | "TG_BIND_CODE_EXPIRED"
  | "TG_BIND_CODE_NOT_CONFIRMED"
  | "TG_BIND_CODE_SCENE_INVALID"
  | "TG_ALREADY_BOUND"
  // === Emby ===
  | "EMBY_AUTH_FAILED"
  | "EMBY_ACCOUNT_UNLINKED"
  | "EMBY_UNBIND_FORBIDDEN"
  | "EMBY_DISABLE_FAILED"
  | "EMBY_ACCOUNT_CONFLICT"
  | "EMBY_CAPACITY_REACHED"
  | "EMBY_MISSING_CREDENTIALS"
  | "EMBY_INPUT_TOO_LONG"
  // === Bangumi ===
  | "BANGUMI_SYNC_DISABLED"
  | "BANGUMI_TOKEN_TOO_LONG"
  | "BANGUMI_TOKEN_MISSING"
  // === 调度器 ===
  | "SCHEDULER_JOB_NOT_FOUND"
  | "SCHEDULER_JOB_RUNNING"
  | "SCHEDULER_JOB_FAILED"
  // === 系统更新（Git 拉取 / Systemd 重启） ===
  | "UPDATE_REPO_INVALID"
  | "UPDATE_BRANCH_INVALID"
  | "UPDATE_NOT_GIT_REPO"
  | "UPDATE_GIT_MISSING"
  | "UPDATE_INSPECT_FAILED"
  | "UPDATE_GIT_FAILED"
  | "UPDATE_RESTART_FAILED"
  // === 通用业务 ===
  | "INVALID_PAYLOAD"
  | "INTERNAL_ERROR"
  // === 注册码 / 邀请码 / 卡码使用流 ===
  | "CODE_EMPTY"
  | "CODE_INVALID"
  | "CODE_ALREADY_EMBY_BOUND"
  | "CODE_REGISTRATION_GRANT_ALREADY_USED"
  | "INVITE_NOT_FOUND"
  | "INVITE_SELF_GENERATE"
  | "INVITE_ALREADY_HAS_PARENT"
  | "INVITE_TARGET_MISMATCH"
  | "INVITER_UNAVAILABLE"
  | "INVITE_DEPTH_EXCEEDED"
  | "INVITE_ROOT_FULL"
  | "INVITER_DAYS_SHORT"
  | "REGCODE_NOT_FOUND"
  // === 邀请域补充（与 errcode.go 第 95+ 行对齐） ===
  | "INVITE_DISABLED"
  | "INVITE_CANNOT_INVITE"
  | "INVITE_DAYS_OUT_OF_RANGE"
  | "INVITE_EXPIRES_BEFORE_NOW"
  | "INVITE_TARGET_USERNAME_INVALID"
  | "INVITE_GENERATION_CONFLICT"
  | "INVITE_RENEW_USER_DISABLED"
  | "INVITE_RENEW_REQUIRES_EMBY"
  | "INVITE_RENEW_BAD_TARGET"
  | "INVITE_RENEW_NOT_DIRECT_CHILD"
  | "INVITE_RENEW_TARGET_MISSING"
  | "INVITE_RENEW_DAYS_OUT_OF_RANGE"
  | "INVITE_DETACH_NOT_DIRECT_CHILD"
  | "INVITE_DETACH_NOT_EXPIRED"
  | "INVITE_EMBY_ALREADY_BOUND"
  | "EMBY_DELETE_FAILED"
  // === 注册码（admin 写流程） ===
  | "REGCODE_TYPE_INVALID"
  | "REGCODE_TARGET_USERNAME_INVALID"
  | "REGCODE_GENERATE_CONFLICT"
  | "REGCODE_BATCH_CONFIRM_REQUIRED"
  | "REGCODE_BATCH_EMPTY"
  | "REGCODE_BATCH_TOO_LARGE"
  | "REGCODE_BATCH_DELETE_FAILED"
  // === 签到 ===
  | "SIGNIN_DISABLED"
  | "SIGNIN_RENEWAL_DISABLED"
  | "SIGNIN_INSUFFICIENT_POINTS"
  // === 数据库迁移 / 备份 ===
  | "DB_BACKUP_LIST_FAILED"
  | "DB_BACKUP_INVALID"
  | "DB_BACKUP_READ_FAILED"
  | "DB_BACKUP_SNAPSHOT_INVALID"
  | "DB_BACKUP_DELETE_FAILED"
  | "DB_BACKUP_CREATE_FAILED"
  | "DB_SNAPSHOT_FAILED"
  | "DB_SNAPSHOT_VERIFY_FAILED"
  | "DB_RESTORE_BACKUP_FAILED"
  | "DB_RESTORE_FAILED"
  | "DB_MIGRATION_DISABLED"
  | "DB_SQLITE_DISABLED"
  | "DB_POSTGRES_DSN_MISSING"
  | "DB_POSTGRES_CONNECT_FAILED"
  | "DB_POSTGRES_WRITE_FAILED"
  | "DB_STATE_FILE_BAD_PATH"
  | "DB_STATE_FILE_MKDIR_FAILED"
  | "DB_STATE_FILE_WRITE_FAILED"
  // === Emby 远端调用 / Admin Emby 操作 ===
  | "EMBY_NOT_CONFIGURED"
  | "EMBY_REMOTE_USERS_FAILED"
  | "EMBY_REMOTE_ACTIVITY_FAILED"
  | "EMBY_REMOTE_SESSIONS_FAILED"
  | "EMBY_BROADCAST_TEXT_EMPTY"
  | "EMBY_USERNAME_INVALID"
  | "EMBY_PASSWORD_TOO_SHORT"
  | "EMBY_CREATE_FAILED"
  | "EMBY_CREATE_NO_ID"
  | "EMBY_SET_PASSWORD_FAILED"
  // === 管理员批量 / 危险操作 confirm 短语保护 ===
  | "ADMIN_EMBY_RESET_CONFIRM_REQUIRED"
  | "ADMIN_BULK_EXPIRE_CONFIRM_REQUIRED"
  | "ADMIN_BULK_EXPIRE_DAYS_TOO_LARGE"
  | "ADMIN_BULK_EXPIRE_INVALID"
  | "ADMIN_BULK_ENABLE_CONFIRM_REQUIRED"
  | "ADMIN_CLEAR_PENDING_EMBY_CONFIRM_REQUIRED"
  | "ADMIN_KICK_NO_EMBY_CONFIRM_REQUIRED"
  | "ADMIN_ENABLE_REJOINED_CONFIRM_REQUIRED"
  | "ADMIN_KICK_UNBOUND_CONFIRM_REQUIRED"
  | "ADMIN_CLEAR_EMAILS_CONFIRM_REQUIRED"
  | "ADMIN_WHITELIST_USERNAME_REQUIRED"
  // === Rebind 申请审核 ===
  | "REBIND_STATUS_INVALID"
  | "REBIND_ACTION_INVALID"
  | "REBIND_BATCH_SIZE_INVALID"
  // === Telegram 配置 ===
  | "TG_NOT_CONFIGURED"
  // === handlers.go 历史遗留：登录 / 资料 / 绑定 / 上传 / 管理员维护 ===
  | "AUTH_CREDENTIALS_EMPTY"
  | "AUTH_SESSION_REFRESH_FAILED"
  | "USER_NEW_USERNAME_REQUIRED"
  | "USER_BACKGROUND_INVALID"
  | "EMBY_ALREADY_LINKED"
  | "EMBY_NO_REGISTRATION_ENTITLEMENT"
  | "EMBY_USERNAME_TAKEN"
  | "EMBY_USERNAME_LOOKUP_FAILED"
  | "EMBY_ADMIN_LINK_FORBIDDEN"
  | "EMBY_LINKED_OTHER_USER"
  | "EMBY_PASSWORD_UPDATE_FAILED"
  | "EMBY_CONNECT_FAILED"
  | "EMBY_USER_LOOKUP_FAILED"
  | "EMBY_USER_NOT_FOUND"
  | "EMBY_LATEST_FAILED"
  | "RENEW_CODE_REQUIRED"
  | "RENEW_CODE_INVALID"
  | "REGCODE_INVALID"
  | "BIND_CODE_RATE_LIMITED"
  | "BIND_CODE_CONFLICT"
  | "BIND_CODE_SAVE_FAILED"
  | "BIND_CODE_NOT_FOUND"
  | "TG_UNBIND_FORBIDDEN"
  | "TG_NOT_BOUND"
  | "TG_ID_INVALID"
  | "TG_ID_TAKEN"
  | "DEVICE_ID_REQUIRED"
  | "IP_REQUIRED"
  | "IP_BLACKLIST_DURATION_INVALID"
  | "UPLOAD_RATE_LIMITED"
  | "UPLOAD_INVALID_PAYLOAD"
  | "UPLOAD_FILE_MISSING"
  | "UPLOAD_FILE_TOO_LARGE"
  | "UPLOAD_TYPE_NOT_ALLOWED"
  | "UPLOAD_DIR_INVALID"
  | "UPLOAD_DIR_CREATE_FAILED"
  | "UPLOAD_SAVE_FAILED"
  | "ASSET_NOT_FOUND"
  | "CONFIG_FILE_NOT_FOUND"
  | "API_DEPRECATED"
  | "ADMIN_QUEUE_CLEAR_PARTIAL"
  | "ADMIN_DAYS_OUT_OF_RANGE"
  | "ADMIN_ENTITLEMENT_PARTIAL"
  | "ADMIN_BULK_LIBRARY_CONFIRM_REQUIRED"
  | "ADMIN_PASSWORD_RESET_SCOPE_INVALID"
  | "ADMIN_EMBY_PASSWORD_RESET_FAILED"
  | "ADMIN_LAST_ADMIN_PROTECTED"
  | "API_KEY_SELF_PERMISSION_FORBIDDEN"
  | "WATCH_STATS_FORBIDDEN"
  // === 求片 / 库存 / 媒体（media_request_handlers.go） ===
  | "MEDIA_REQUEST_DISABLED"
  | "MEDIA_REQUEST_TG_REQUIRED"
  | "MEDIA_REQUEST_PENDING_LIMIT"
  | "MEDIA_REQUEST_GLOBAL_LIMIT"
  | "MEDIA_REQUEST_ALREADY_EXISTS"
  | "MEDIA_REQUEST_STATUS_INVALID"
  | "MEDIA_REQUEST_NOT_FOUND"
  | "MEDIA_REQUEST_ACCESS_DENIED"
  | "MEDIA_REQUEST_DELETE_DENIED"
  | "MEDIA_REQUEST_QUERY_REQUIRED"
  | "MEDIA_REQUEST_PAYLOAD_EMPTY"
  | "MEDIA_SEARCH_SOURCE_FAILED"
  | "MEDIA_INVENTORY_SEARCH_FAILED"
  | "MEDIA_ADMIN_ROLE_REQUIRED"
  | "INTERNAL_SECRET_INVALID"
  // === 配置备份 / 恢复（config_admin.go） ===
  | "CONFIG_BACKUP_LIST_FAILED"
  | "CONFIG_BACKUP_CREATE_FAILED"
  | "CONFIG_BACKUP_NOT_FOUND"
  | "CONFIG_BACKUP_INVALID"
  | "CONFIG_BACKUP_VERIFY_FAILED"
  | "CONFIG_BACKUP_DELETE_FAILED"
  // === 违规 / 黑名单（violation_handlers.go / batch_helpers.go） ===
  | "VIOLATION_ID_INVALID"
  | "VIOLATION_CONFIRM_REQUIRED"
  | "VIOLATION_CLEAR_FAILED"
  | "BATCH_CONFIRM_REQUIRED"
  | "BATCH_UIDS_REQUIRED"
  | "BATCH_TOO_MANY_TARGETS"
  // === Telegram 内部绑定（telegram_bind_secure.go） ===
  | "TG_BIND_CODE_NOT_FOUND"
  | "TG_BIND_TGID_INVALID"
  | "TG_BIND_TARGET_TAKEN"
  | "TG_BIND_GROUP_CHECK_FAILED"
  | "TG_BIND_GROUP_MEMBERSHIP_REQUIRED"
  // === 鉴权中间件 / Emby 越权拦截 ===
  | "EMBY_ADMIN_BLOCKED"
  | "EMBY_ADMIN_RESTRICTED"
  // === 批量 / 求片 / 上传 / 运行时日志补码 ===
  | "BATCH_DAYS_INVALID"
  | "BATCH_LIBRARY_ACTION_INVALID"
  | "BATCH_SELF_TARGET"
  | "USER_NO_EMBY"
  | "REGCODE_STORAGE_MISMATCH"
  | "RUNTIME_LOG_STREAM_UNSUPPORTED"
  | "CONFIG_SAVE_FAILED"
  | "AUTH_APIKEY_LOGIN_RATE_LIMITED"
  // === 邮箱验证 / 验证码 / 强制绑定 ===
  | "EMAIL_DISABLED"
  | "EMAIL_NOT_BOUND"
  | "EMAIL_CODE_INVALID"
  | "EMAIL_CODE_EXPIRED"
  | "EMAIL_CODE_TOO_MANY"
  | "EMAIL_CODE_REQUIRED"
  | "EMAIL_SEND_FAILED"
  | "EMAIL_RESEND_COOLDOWN"
  | "EMAIL_RATE_LIMITED"
  | "EMAIL_PURPOSE_INVALID"
  | "USER_EMAIL_VERIFICATION_REQUIRED"
  | "USER_EMAIL_CONFLICT"
  | "USER_EMAIL_ALREADY_VERIFIED"
  // === 中间件层（IP 黑名单 / 全局限流 / 路由分发） ===
  // 比通用 FORBIDDEN / RATE_LIMITED / METHOD_NOT_ALLOWED / NOT_FOUND 更精确，
  // UI 可以分别给出"联系管理员解封"、"稍后重试"、"接口已下线" 等不同 CTA。
  | "SECURITY_IP_BLACKLISTED"
  | "RATE_GLOBAL_LIMITED"
  | "ROUTE_METHOD_NOT_ALLOWED"
  | "ROUTE_NOT_FOUND"
  // === defaultErrorCode 兜底（response.go HTTP status → 通用码） ===
  // 完整覆盖 response.go 全部 13 个 fallback 字面量，任何 isKnownErrCode
  // 命中失败都意味着 errcode.go 又增了新协议码。
  | "BAD_REQUEST"
  | "UNAUTHORIZED"
  | "FORBIDDEN"
  | "NOT_FOUND"
  | "METHOD_NOT_ALLOWED"
  | "CONFLICT"
  | "GONE"
  | "PAYLOAD_TOO_LARGE"
  | "RATE_LIMITED"
  | "UPSTREAM_ERROR"
  | "SERVICE_UNAVAILABLE"
  | "REQUEST_FAILED";

/**
 * 与 ErrCode 联合一一对应的运行时常量。前端业务分支建议优先消费这些常量
 * 而非裸写字符串，重命名时 TS 会同步报错。
 */
export const ErrCodes = {
  // 鉴权 / 会话
  LoginRateLimited: "AUTH_LOGIN_RATE_LIMITED",
  LoginInvalid: "AUTH_LOGIN_INVALID",
  AccountDisabled: "AUTH_ACCOUNT_DISABLED",
  AccountExpired: "AUTH_ACCOUNT_EXPIRED",
  SessionCreateFailed: "AUTH_SESSION_CREATE_FAILED",
  APIKeyEmpty: "AUTH_APIKEY_EMPTY",
  APIKeyInvalid: "AUTH_APIKEY_INVALID",
  DirectLoginDisabled: "AUTH_DIRECT_LOGIN_DISABLED",
  PasswordResetTooMany: "AUTH_PASSWORD_RESET_TOO_MANY",
  PasswordOldMismatch: "AUTH_PASSWORD_OLD_MISMATCH",
  PasswordWeak: "AUTH_PASSWORD_WEAK",
  PasswordHashFailed: "AUTH_PASSWORD_HASH_FAILED",
  // 用户 / 注册
  RegisterRateLimited: "USER_REGISTER_RATE_LIMITED",
  RegisterDisabled: "USER_REGISTER_DISABLED",
  UsernameInvalid: "USER_USERNAME_INVALID",
  UsernameTaken: "USER_USERNAME_TAKEN",
  UserNotFound: "USER_NOT_FOUND",
  UserLimitReached: "USER_LIMIT_REACHED",
  UserProtected: "USER_PROTECTED",
  // Telegram 绑定
  TGBindRequired: "TG_BIND_REQUIRED",
  TGBindCodeFormat: "TG_BIND_CODE_FORMAT_INVALID",
  TGBindCodeExpired: "TG_BIND_CODE_EXPIRED",
  TGBindCodeNotConfirm: "TG_BIND_CODE_NOT_CONFIRMED",
  TGBindCodeSceneBad: "TG_BIND_CODE_SCENE_INVALID",
  TGAlreadyBound: "TG_ALREADY_BOUND",
  // Emby
  EmbyAuthFailed: "EMBY_AUTH_FAILED",
  EmbyAccountUnlinked: "EMBY_ACCOUNT_UNLINKED",
  EmbyUnbindForbidden: "EMBY_UNBIND_FORBIDDEN",
  EmbyDisableFailed: "EMBY_DISABLE_FAILED",
  EmbyAccountConflict: "EMBY_ACCOUNT_CONFLICT",
  EmbyCapacityReached: "EMBY_CAPACITY_REACHED",
  EmbyMissingCreds: "EMBY_MISSING_CREDENTIALS",
  EmbyInputTooLong: "EMBY_INPUT_TOO_LONG",
  // Bangumi
  BangumiSyncDisabled: "BANGUMI_SYNC_DISABLED",
  BangumiTokenTooLong: "BANGUMI_TOKEN_TOO_LONG",
  BangumiTokenMissing: "BANGUMI_TOKEN_MISSING",
  // 调度器
  SchedulerJobNotFound: "SCHEDULER_JOB_NOT_FOUND",
  SchedulerJobRunning: "SCHEDULER_JOB_RUNNING",
  SchedulerJobFailed: "SCHEDULER_JOB_FAILED",
  // 系统更新
  UpdateRepoInvalid: "UPDATE_REPO_INVALID",
  UpdateBranchInvalid: "UPDATE_BRANCH_INVALID",
  UpdateNotGitRepo: "UPDATE_NOT_GIT_REPO",
  UpdateGitMissing: "UPDATE_GIT_MISSING",
  UpdateInspectFailed: "UPDATE_INSPECT_FAILED",
  UpdateGitFailed: "UPDATE_GIT_FAILED",
  UpdateRestartFailed: "UPDATE_RESTART_FAILED",
  // 通用业务 + 兜底
  InvalidPayload: "INVALID_PAYLOAD",
  Internal: "INTERNAL_ERROR",
  // 注册码 / 邀请码 / 卡码使用流
  CodeEmpty: "CODE_EMPTY",
  CodeInvalid: "CODE_INVALID",
  CodeAlreadyEmbyBound: "CODE_ALREADY_EMBY_BOUND",
  CodeRegistrationGrantAlreadyUsed: "CODE_REGISTRATION_GRANT_ALREADY_USED",
  InviteNotFound: "INVITE_NOT_FOUND",
  InviteSelfGenerate: "INVITE_SELF_GENERATE",
  InviteAlreadyHasParent: "INVITE_ALREADY_HAS_PARENT",
  InviteTargetMismatch: "INVITE_TARGET_MISMATCH",
  InviterUnavailable: "INVITER_UNAVAILABLE",
  InviteDepthExceeded: "INVITE_DEPTH_EXCEEDED",
  InviteRootFull: "INVITE_ROOT_FULL",
  InviterDaysShort: "INVITER_DAYS_SHORT",
  RegcodeNotFound: "REGCODE_NOT_FOUND",
  // 邀请域补充
  InviteDisabled: "INVITE_DISABLED",
  InviteCannotInvite: "INVITE_CANNOT_INVITE",
  InviteDaysOutOfRange: "INVITE_DAYS_OUT_OF_RANGE",
  InviteExpiresBeforeNow: "INVITE_EXPIRES_BEFORE_NOW",
  InviteTargetUsernameBad: "INVITE_TARGET_USERNAME_INVALID",
  InviteGenerationConflict: "INVITE_GENERATION_CONFLICT",
  InviteRenewUserDisabled: "INVITE_RENEW_USER_DISABLED",
  InviteRenewRequiresEmby: "INVITE_RENEW_REQUIRES_EMBY",
  InviteRenewBadTarget: "INVITE_RENEW_BAD_TARGET",
  InviteRenewNotDirectChild: "INVITE_RENEW_NOT_DIRECT_CHILD",
  InviteRenewTargetMissing: "INVITE_RENEW_TARGET_MISSING",
  InviteRenewDaysOutOfRange: "INVITE_RENEW_DAYS_OUT_OF_RANGE",
  InviteDetachNotDirect: "INVITE_DETACH_NOT_DIRECT_CHILD",
  InviteDetachNotExpired: "INVITE_DETACH_NOT_EXPIRED",
  InviteEmbyBound: "INVITE_EMBY_ALREADY_BOUND",
  EmbyDeleteFailed: "EMBY_DELETE_FAILED",
  // 注册码（admin 写流程）
  RegcodeTypeInvalid: "REGCODE_TYPE_INVALID",
  RegcodeTargetBad: "REGCODE_TARGET_USERNAME_INVALID",
  RegcodeGenerateConflict: "REGCODE_GENERATE_CONFLICT",
  RegcodeBatchConfirm: "REGCODE_BATCH_CONFIRM_REQUIRED",
  RegcodeBatchEmpty: "REGCODE_BATCH_EMPTY",
  RegcodeBatchTooLarge: "REGCODE_BATCH_TOO_LARGE",
  RegcodeBatchFailed: "REGCODE_BATCH_DELETE_FAILED",
  // 签到
  SigninDisabled: "SIGNIN_DISABLED",
  SigninRenewalDisabled: "SIGNIN_RENEWAL_DISABLED",
  SigninInsufficientPoints: "SIGNIN_INSUFFICIENT_POINTS",
  // 数据库迁移 / 备份
  DBBackupListFailed: "DB_BACKUP_LIST_FAILED",
  DBBackupInvalid: "DB_BACKUP_INVALID",
  DBBackupReadFailed: "DB_BACKUP_READ_FAILED",
  DBBackupSnapshotBad: "DB_BACKUP_SNAPSHOT_INVALID",
  DBBackupDeleteFailed: "DB_BACKUP_DELETE_FAILED",
  DBBackupCreateFailed: "DB_BACKUP_CREATE_FAILED",
  DBSnapshotFailed: "DB_SNAPSHOT_FAILED",
  DBSnapshotVerifyBad: "DB_SNAPSHOT_VERIFY_FAILED",
  DBRestoreBackupFail: "DB_RESTORE_BACKUP_FAILED",
  DBRestoreFailed: "DB_RESTORE_FAILED",
  DBMigrationDisabled: "DB_MIGRATION_DISABLED",
  DBSQLiteDisabled: "DB_SQLITE_DISABLED",
  DBPostgresMissing: "DB_POSTGRES_DSN_MISSING",
  DBPostgresConnect: "DB_POSTGRES_CONNECT_FAILED",
  DBPostgresWriteFail: "DB_POSTGRES_WRITE_FAILED",
  DBStateFileBadPath: "DB_STATE_FILE_BAD_PATH",
  DBStateFileMkdirBad: "DB_STATE_FILE_MKDIR_FAILED",
  DBStateFileWriteBad: "DB_STATE_FILE_WRITE_FAILED",
  // Emby 远端调用 / Admin Emby 操作
  EmbyNotConfigured: "EMBY_NOT_CONFIGURED",
  EmbyRemoteUsersFailed: "EMBY_REMOTE_USERS_FAILED",
  EmbyRemoteActivityFail: "EMBY_REMOTE_ACTIVITY_FAILED",
  EmbyRemoteSessionsFail: "EMBY_REMOTE_SESSIONS_FAILED",
  EmbyBroadcastTextEmpty: "EMBY_BROADCAST_TEXT_EMPTY",
  EmbyUsernameInvalid: "EMBY_USERNAME_INVALID",
  EmbyPasswordTooShort: "EMBY_PASSWORD_TOO_SHORT",
  EmbyCreateFailed: "EMBY_CREATE_FAILED",
  EmbyCreateNoID: "EMBY_CREATE_NO_ID",
  EmbySetPasswordFailed: "EMBY_SET_PASSWORD_FAILED",
  // 管理员批量 / 危险操作 confirm 短语保护
  AdminEmbyResetConfirm: "ADMIN_EMBY_RESET_CONFIRM_REQUIRED",
  AdminBulkExpireConfirm: "ADMIN_BULK_EXPIRE_CONFIRM_REQUIRED",
  AdminBulkExpireDaysTooLarge: "ADMIN_BULK_EXPIRE_DAYS_TOO_LARGE",
  AdminBulkExpireInvalid: "ADMIN_BULK_EXPIRE_INVALID",
  AdminBulkEnableConfirm: "ADMIN_BULK_ENABLE_CONFIRM_REQUIRED",
  AdminClearPendingEmbyConfirm: "ADMIN_CLEAR_PENDING_EMBY_CONFIRM_REQUIRED",
  AdminKickNoEmbyConfirm: "ADMIN_KICK_NO_EMBY_CONFIRM_REQUIRED",
  AdminEnableRejoinedConfirm: "ADMIN_ENABLE_REJOINED_CONFIRM_REQUIRED",
  AdminKickUnboundConfirm: "ADMIN_KICK_UNBOUND_CONFIRM_REQUIRED",
  AdminClearEmailsConfirm: "ADMIN_CLEAR_EMAILS_CONFIRM_REQUIRED",
  AdminWhitelistUsernameEmpty: "ADMIN_WHITELIST_USERNAME_REQUIRED",
  // Rebind 申请审核
  RebindStatusInvalid: "REBIND_STATUS_INVALID",
  RebindActionInvalid: "REBIND_ACTION_INVALID",
  RebindBatchSizeInvalid: "REBIND_BATCH_SIZE_INVALID",
  // Telegram 配置
  TGNotConfigured: "TG_NOT_CONFIGURED",
  // handlers.go 历史遗留
  AuthCredentialsEmpty: "AUTH_CREDENTIALS_EMPTY",
  AuthSessionRefreshFailed: "AUTH_SESSION_REFRESH_FAILED",
  UserNewUsernameRequired: "USER_NEW_USERNAME_REQUIRED",
  UserBackgroundInvalid: "USER_BACKGROUND_INVALID",
  EmbyAlreadyLinked: "EMBY_ALREADY_LINKED",
  EmbyNoRegistrationGrant: "EMBY_NO_REGISTRATION_ENTITLEMENT",
  EmbyUsernameTaken: "EMBY_USERNAME_TAKEN",
  EmbyUsernameLookupFailed: "EMBY_USERNAME_LOOKUP_FAILED",
  EmbyAdminLinkForbidden: "EMBY_ADMIN_LINK_FORBIDDEN",
  EmbyLinkedOtherUser: "EMBY_LINKED_OTHER_USER",
  EmbyPasswordUpdateFailed: "EMBY_PASSWORD_UPDATE_FAILED",
  EmbyConnectFailed: "EMBY_CONNECT_FAILED",
  EmbyUserLookupFailed: "EMBY_USER_LOOKUP_FAILED",
  EmbyUserNotFound: "EMBY_USER_NOT_FOUND",
  EmbyLatestFailed: "EMBY_LATEST_FAILED",
  RenewCodeRequired: "RENEW_CODE_REQUIRED",
  RenewCodeInvalid: "RENEW_CODE_INVALID",
  RegcodeInvalid: "REGCODE_INVALID",
  BindCodeRateLimited: "BIND_CODE_RATE_LIMITED",
  BindCodeConflict: "BIND_CODE_CONFLICT",
  BindCodeSaveFailed: "BIND_CODE_SAVE_FAILED",
  BindCodeNotFound: "BIND_CODE_NOT_FOUND",
  TGUnbindForbidden: "TG_UNBIND_FORBIDDEN",
  TGNotBound: "TG_NOT_BOUND",
  TGIDInvalid: "TG_ID_INVALID",
  TGIDTaken: "TG_ID_TAKEN",
  DeviceIDRequired: "DEVICE_ID_REQUIRED",
  IPRequired: "IP_REQUIRED",
  IPBlacklistDurationInvalid: "IP_BLACKLIST_DURATION_INVALID",
  UploadRateLimited: "UPLOAD_RATE_LIMITED",
  UploadInvalidPayload: "UPLOAD_INVALID_PAYLOAD",
  UploadFileMissing: "UPLOAD_FILE_MISSING",
  UploadFileTooLarge: "UPLOAD_FILE_TOO_LARGE",
  UploadTypeNotAllowed: "UPLOAD_TYPE_NOT_ALLOWED",
  UploadDirInvalid: "UPLOAD_DIR_INVALID",
  UploadDirCreateFailed: "UPLOAD_DIR_CREATE_FAILED",
  UploadSaveFailed: "UPLOAD_SAVE_FAILED",
  AssetNotFound: "ASSET_NOT_FOUND",
  ConfigFileNotFound: "CONFIG_FILE_NOT_FOUND",
  APIDeprecated: "API_DEPRECATED",
  AdminQueueClearPartial: "ADMIN_QUEUE_CLEAR_PARTIAL",
  AdminDaysOutOfRange: "ADMIN_DAYS_OUT_OF_RANGE",
  AdminEntitlementPartial: "ADMIN_ENTITLEMENT_PARTIAL",
  AdminBulkLibraryConfirm: "ADMIN_BULK_LIBRARY_CONFIRM_REQUIRED",
  AdminPasswordResetScope: "ADMIN_PASSWORD_RESET_SCOPE_INVALID",
  AdminEmbyPasswordReset: "ADMIN_EMBY_PASSWORD_RESET_FAILED",
  AdminLastAdminProtected: "ADMIN_LAST_ADMIN_PROTECTED",
  APIKeySelfPermForbidden: "API_KEY_SELF_PERMISSION_FORBIDDEN",
  WatchStatsForbidden: "WATCH_STATS_FORBIDDEN",
  // 求片 / 库存 / 媒体
  MediaRequestDisabled: "MEDIA_REQUEST_DISABLED",
  MediaRequestTGRequired: "MEDIA_REQUEST_TG_REQUIRED",
  MediaRequestPendingLimit: "MEDIA_REQUEST_PENDING_LIMIT",
  MediaRequestGlobalLimit: "MEDIA_REQUEST_GLOBAL_LIMIT",
  MediaRequestExists: "MEDIA_REQUEST_ALREADY_EXISTS",
  MediaRequestStatusInvalid: "MEDIA_REQUEST_STATUS_INVALID",
  MediaRequestNotFound: "MEDIA_REQUEST_NOT_FOUND",
  MediaRequestAccessDenied: "MEDIA_REQUEST_ACCESS_DENIED",
  MediaRequestDeleteDenied: "MEDIA_REQUEST_DELETE_DENIED",
  MediaRequestQueryRequired: "MEDIA_REQUEST_QUERY_REQUIRED",
  MediaRequestPayloadEmpty: "MEDIA_REQUEST_PAYLOAD_EMPTY",
  MediaSearchSourceFailed: "MEDIA_SEARCH_SOURCE_FAILED",
  MediaInventorySearchFailed: "MEDIA_INVENTORY_SEARCH_FAILED",
  MediaAdminRoleRequired: "MEDIA_ADMIN_ROLE_REQUIRED",
  InternalSecretInvalid: "INTERNAL_SECRET_INVALID",
  // 配置备份 / 恢复
  ConfigBackupListFailed: "CONFIG_BACKUP_LIST_FAILED",
  ConfigBackupCreateFailed: "CONFIG_BACKUP_CREATE_FAILED",
  ConfigBackupNotFound: "CONFIG_BACKUP_NOT_FOUND",
  ConfigBackupInvalid: "CONFIG_BACKUP_INVALID",
  ConfigBackupVerifyFailed: "CONFIG_BACKUP_VERIFY_FAILED",
  ConfigBackupDeleteFailed: "CONFIG_BACKUP_DELETE_FAILED",
  // 违规 / 黑名单 / 批量
  ViolationIDInvalid: "VIOLATION_ID_INVALID",
  ViolationConfirmRequired: "VIOLATION_CONFIRM_REQUIRED",
  ViolationClearFailed: "VIOLATION_CLEAR_FAILED",
  BatchConfirmRequired: "BATCH_CONFIRM_REQUIRED",
  BatchUIDsRequired: "BATCH_UIDS_REQUIRED",
  BatchTooManyTargets: "BATCH_TOO_MANY_TARGETS",
  // Telegram 内部绑定
  TGBindCodeNotFound: "TG_BIND_CODE_NOT_FOUND",
  TGBindTGIDInvalid: "TG_BIND_TGID_INVALID",
  TGBindTargetTaken: "TG_BIND_TARGET_TAKEN",
  TGBindGroupCheckFailed: "TG_BIND_GROUP_CHECK_FAILED",
  TGBindGroupMembershipRequired: "TG_BIND_GROUP_MEMBERSHIP_REQUIRED",
  // 鉴权中间件 / Emby 越权拦截
  EmbyAdminBlocked: "EMBY_ADMIN_BLOCKED",
  EmbyAdminRestricted: "EMBY_ADMIN_RESTRICTED",
  // 批量 / 求片 / 上传 / 运行时日志补码
  BatchDaysInvalid: "BATCH_DAYS_INVALID",
  BatchLibraryActionInvalid: "BATCH_LIBRARY_ACTION_INVALID",
  BatchSelfTarget: "BATCH_SELF_TARGET",
  UserHasNoEmby: "USER_NO_EMBY",
  RegcodeStorageMismatch: "REGCODE_STORAGE_MISMATCH",
  RuntimeLogStreamUnsupported: "RUNTIME_LOG_STREAM_UNSUPPORTED",
  ConfigSaveFailed: "CONFIG_SAVE_FAILED",
  APIKeyLoginRateLimited: "AUTH_APIKEY_LOGIN_RATE_LIMITED",
  // 邮箱验证 / 验证码 / 强制绑定
  EmailDisabled: "EMAIL_DISABLED",
  EmailNotBound: "EMAIL_NOT_BOUND",
  EmailCodeInvalid: "EMAIL_CODE_INVALID",
  EmailCodeExpired: "EMAIL_CODE_EXPIRED",
  EmailCodeTooMany: "EMAIL_CODE_TOO_MANY",
  EmailCodeRequired: "EMAIL_CODE_REQUIRED",
  EmailSendFailed: "EMAIL_SEND_FAILED",
  EmailResendCooldown: "EMAIL_RESEND_COOLDOWN",
  EmailRateLimited: "EMAIL_RATE_LIMITED",
  EmailPurposeInvalid: "EMAIL_PURPOSE_INVALID",
  EmailVerificationRequired: "USER_EMAIL_VERIFICATION_REQUIRED",
  EmailConflict: "USER_EMAIL_CONFLICT",
  EmailAlreadyVerified: "USER_EMAIL_ALREADY_VERIFIED",
  // 中间件层（IP 黑名单 / 全局限流 / 路由分发）
  IPBlacklisted: "SECURITY_IP_BLACKLISTED",
  GlobalRateLimited: "RATE_GLOBAL_LIMITED",
  RouteMethodNotAllowed: "ROUTE_METHOD_NOT_ALLOWED",
  RouteNotFound: "ROUTE_NOT_FOUND",
  BadRequest: "BAD_REQUEST",
  Unauthorized: "UNAUTHORIZED",
  Forbidden: "FORBIDDEN",
  NotFound: "NOT_FOUND",
  MethodNotAllowed: "METHOD_NOT_ALLOWED",
  Conflict: "CONFLICT",
  Gone: "GONE",
  PayloadTooLarge: "PAYLOAD_TOO_LARGE",
  RateLimited: "RATE_LIMITED",
  UpstreamError: "UPSTREAM_ERROR",
  ServiceUnavailable: "SERVICE_UNAVAILABLE",
  RequestFailed: "REQUEST_FAILED",
} as const satisfies Record<string, ErrCode>;

/**
 * 全部已知错误码的运行时清单。用于 isKnownErrCode 窄化 + 单元测试覆盖率
 * 校对（逐字符串与后端 errcode.go 对照）。
 */
export const KNOWN_ERR_CODES: ReadonlySet<ErrCode> = new Set<ErrCode>(
  Object.values(ErrCodes),
);

/**
 * 类型守卫：宽松 string 收紧到 ErrCode。
 * 未知码（后端先行下发的新增 / 第三方代理改写）走 false 分支，调用方应
 * 退回到 friendly 文案默认值或 backend message。
 */
export function isKnownErrCode(code: string | undefined | null): code is ErrCode {
  if (!code) return false;
  return KNOWN_ERR_CODES.has(code as ErrCode);
}
