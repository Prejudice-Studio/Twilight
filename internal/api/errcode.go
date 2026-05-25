package api

// ErrCode 是后端业务级错误码，前端 / Bot / 第三方集成方靠它做语义判断
// （HTTP status 仅描述协议层错误，业务错误码描述领域语义）。
// 命名规范：
//   - 全大写 + 下划线
//   - 前缀按业务域分组：USER_ / EMBY_ / REGCODE_ / INVITE_ / MEDIA_ /
//     APIKEY_ / TG_ / CONFIG_ / SCHEDULER_ / SYSTEM_ / RATE_
//   - 通用错误延用 response.go:defaultErrorCode 自动推导（BAD_REQUEST 等）
// 新增错误码时：
//   1. 在本文件追加常量
//   2. 在前端 webui/src/lib/api-types.ts 镜像枚举
type ErrCode = string

const (
	// === 鉴权 / 会话 ===
	ErrLoginRateLimited      ErrCode = "AUTH_LOGIN_RATE_LIMITED"
	ErrLoginInvalid          ErrCode = "AUTH_LOGIN_INVALID"
	ErrAccountDisabled       ErrCode = "AUTH_ACCOUNT_DISABLED"
	ErrSessionCreateFailed   ErrCode = "AUTH_SESSION_CREATE_FAILED"
	ErrAPIKeyEmpty           ErrCode = "AUTH_APIKEY_EMPTY"
	ErrAPIKeyInvalid         ErrCode = "AUTH_APIKEY_INVALID"
	ErrDirectLoginDisabled   ErrCode = "AUTH_DIRECT_LOGIN_DISABLED"
	ErrPasswordResetTooMany  ErrCode = "AUTH_PASSWORD_RESET_TOO_MANY"
	ErrPasswordOldMismatch   ErrCode = "AUTH_PASSWORD_OLD_MISMATCH"
	ErrPasswordWeak          ErrCode = "AUTH_PASSWORD_WEAK"
	ErrPasswordHashFailed    ErrCode = "AUTH_PASSWORD_HASH_FAILED"
	ErrCSRFMissing           ErrCode = "AUTH_CSRF_MISSING"
	ErrCSRFMismatch          ErrCode = "AUTH_CSRF_MISMATCH"

	// === 用户 / 注册 ===
	ErrRegisterRateLimited ErrCode = "USER_REGISTER_RATE_LIMITED"
	ErrRegisterDisabled    ErrCode = "USER_REGISTER_DISABLED"
	ErrUsernameInvalid     ErrCode = "USER_USERNAME_INVALID"
	ErrUsernameTaken       ErrCode = "USER_USERNAME_TAKEN"
	ErrUserNotFound        ErrCode = "USER_NOT_FOUND"
	ErrUserLimitReached    ErrCode = "USER_LIMIT_REACHED"
	ErrUserProtected       ErrCode = "USER_PROTECTED"

	// === Telegram 绑定 ===
	ErrTGBindRequired       ErrCode = "TG_BIND_REQUIRED"
	ErrTGBindCodeFormat     ErrCode = "TG_BIND_CODE_FORMAT_INVALID"
	ErrTGBindCodeExpired    ErrCode = "TG_BIND_CODE_EXPIRED"
	ErrTGBindCodeNotConfirm ErrCode = "TG_BIND_CODE_NOT_CONFIRMED"
	ErrTGBindCodeSceneBad   ErrCode = "TG_BIND_CODE_SCENE_INVALID"
	ErrTGAlreadyBound       ErrCode = "TG_ALREADY_BOUND"

	// === Emby ===
	ErrEmbyAuthFailed      ErrCode = "EMBY_AUTH_FAILED"
	ErrEmbyAccountUnlinked ErrCode = "EMBY_ACCOUNT_UNLINKED"
	ErrEmbyCapacityReached ErrCode = "EMBY_CAPACITY_REACHED"
	ErrEmbyMissingCreds    ErrCode = "EMBY_MISSING_CREDENTIALS"
	ErrEmbyInputTooLong    ErrCode = "EMBY_INPUT_TOO_LONG"

	// === Bangumi ===
	ErrBangumiSyncDisabled ErrCode = "BANGUMI_SYNC_DISABLED"
	ErrBangumiTokenTooLong ErrCode = "BANGUMI_TOKEN_TOO_LONG"
	ErrBangumiTokenMissing ErrCode = "BANGUMI_TOKEN_MISSING"

	// === 调度器 ===
	ErrSchedulerJobNotFound ErrCode = "SCHEDULER_JOB_NOT_FOUND"
	ErrSchedulerJobRunning  ErrCode = "SCHEDULER_JOB_RUNNING"
	ErrSchedulerJobFailed   ErrCode = "SCHEDULER_JOB_FAILED"

	// === 系统更新（Git 拉取 / Systemd 重启） ===
	ErrUpdateRepoInvalid    ErrCode = "UPDATE_REPO_INVALID"
	ErrUpdateBranchInvalid  ErrCode = "UPDATE_BRANCH_INVALID"
	ErrUpdateNotGitRepo     ErrCode = "UPDATE_NOT_GIT_REPO"
	ErrUpdateGitMissing     ErrCode = "UPDATE_GIT_MISSING"
	ErrUpdateInspectFailed  ErrCode = "UPDATE_INSPECT_FAILED"
	ErrUpdateGitFailed      ErrCode = "UPDATE_GIT_FAILED"
	ErrUpdateRestartFailed  ErrCode = "UPDATE_RESTART_FAILED"

	// === 通用业务 ===
	ErrInvalidPayload ErrCode = "INVALID_PAYLOAD"
	ErrInternal       ErrCode = "INTERNAL_ERROR"

	// === 注册码 / 邀请码 / 卡码使用流 ===
	// 这些错误在 code_use_handlers.go 高频出现，前端需基于稳定码做差异化
	// UI 行为（"不能使用自己生成的邀请码"应跳"前往个人主页"，"邀请树人数
	// 已达上限"应跳"申请提升上限"等），不能再依赖中文 message 正则。
	ErrCodeEmpty             ErrCode = "CODE_EMPTY"
	ErrCodeInvalid           ErrCode = "CODE_INVALID"
	ErrCodeAlreadyEmbyBound  ErrCode = "CODE_ALREADY_EMBY_BOUND"
	ErrInviteNotFound        ErrCode = "INVITE_NOT_FOUND"
	ErrInviteSelfGenerate    ErrCode = "INVITE_SELF_GENERATE"
	ErrInviteAlreadyHasParent ErrCode = "INVITE_ALREADY_HAS_PARENT"
	ErrInviteTargetMismatch  ErrCode = "INVITE_TARGET_MISMATCH"
	ErrInviterUnavailable    ErrCode = "INVITER_UNAVAILABLE"
	ErrInviteDepthExceeded   ErrCode = "INVITE_DEPTH_EXCEEDED"
	ErrInviteRootFull        ErrCode = "INVITE_ROOT_FULL"
	ErrInviterDaysShort      ErrCode = "INVITER_DAYS_SHORT"
	ErrRegcodeNotFound       ErrCode = "REGCODE_NOT_FOUND"
)
