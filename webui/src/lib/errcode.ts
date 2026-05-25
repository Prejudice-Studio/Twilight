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
  | "AUTH_SESSION_CREATE_FAILED"
  | "AUTH_APIKEY_EMPTY"
  | "AUTH_APIKEY_INVALID"
  | "AUTH_DIRECT_LOGIN_DISABLED"
  | "AUTH_PASSWORD_RESET_TOO_MANY"
  | "AUTH_PASSWORD_OLD_MISMATCH"
  | "AUTH_PASSWORD_WEAK"
  | "AUTH_PASSWORD_HASH_FAILED"
  | "AUTH_CSRF_MISSING"
  | "AUTH_CSRF_MISMATCH"
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
  | "INVITE_NOT_FOUND"
  | "INVITE_SELF_GENERATE"
  | "INVITE_ALREADY_HAS_PARENT"
  | "INVITE_TARGET_MISMATCH"
  | "INVITER_UNAVAILABLE"
  | "INVITE_DEPTH_EXCEEDED"
  | "INVITE_ROOT_FULL"
  | "INVITER_DAYS_SHORT"
  | "REGCODE_NOT_FOUND"
  // === defaultErrorCode 兜底（response.go HTTP status → 通用码） ===
  | "BAD_REQUEST"
  | "UNAUTHORIZED"
  | "FORBIDDEN"
  | "NOT_FOUND"
  | "CONFLICT"
  | "RATE_LIMITED";

/**
 * 与 ErrCode 联合一一对应的运行时常量。前端业务分支建议优先消费这些常量
 * 而非裸写字符串，重命名时 TS 会同步报错。
 */
export const ErrCodes = {
  // 鉴权 / 会话
  LoginRateLimited: "AUTH_LOGIN_RATE_LIMITED",
  LoginInvalid: "AUTH_LOGIN_INVALID",
  AccountDisabled: "AUTH_ACCOUNT_DISABLED",
  SessionCreateFailed: "AUTH_SESSION_CREATE_FAILED",
  APIKeyEmpty: "AUTH_APIKEY_EMPTY",
  APIKeyInvalid: "AUTH_APIKEY_INVALID",
  DirectLoginDisabled: "AUTH_DIRECT_LOGIN_DISABLED",
  PasswordResetTooMany: "AUTH_PASSWORD_RESET_TOO_MANY",
  PasswordOldMismatch: "AUTH_PASSWORD_OLD_MISMATCH",
  PasswordWeak: "AUTH_PASSWORD_WEAK",
  PasswordHashFailed: "AUTH_PASSWORD_HASH_FAILED",
  CSRFMissing: "AUTH_CSRF_MISSING",
  CSRFMismatch: "AUTH_CSRF_MISMATCH",
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
  InviteNotFound: "INVITE_NOT_FOUND",
  InviteSelfGenerate: "INVITE_SELF_GENERATE",
  InviteAlreadyHasParent: "INVITE_ALREADY_HAS_PARENT",
  InviteTargetMismatch: "INVITE_TARGET_MISMATCH",
  InviterUnavailable: "INVITER_UNAVAILABLE",
  InviteDepthExceeded: "INVITE_DEPTH_EXCEEDED",
  InviteRootFull: "INVITE_ROOT_FULL",
  InviterDaysShort: "INVITER_DAYS_SHORT",
  RegcodeNotFound: "REGCODE_NOT_FOUND",
  BadRequest: "BAD_REQUEST",
  Unauthorized: "UNAUTHORIZED",
  Forbidden: "FORBIDDEN",
  NotFound: "NOT_FOUND",
  Conflict: "CONFLICT",
  RateLimited: "RATE_LIMITED",
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
