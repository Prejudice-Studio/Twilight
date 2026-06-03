// 前端字段校验集中入口。所有规则必须与后端 internal/validate/validate.go
// 保持镜像同步，任何修改都需要：
//   1. 同步改后端常量 / 错误码
//   2. 在 docs/AUDIT_BATCH_06.md §30.1 / §30.3 追加变更记录
//   3. 更新 webui/src/lib/__tests__/validators.test.ts（若已落地 vitest）
// 设计原则：
//   - 函数纯粹：仅做规则判断，不抛 toast 也不调 API；调用方决定如何展示
//   - 返回 { ok, message } 一致结构，便于 form 抽象统一处理
//   - 用户名 / regcode 等业务字段的 message 文案与后端 validate.go 错误对齐，
//     方便后端 i18n 切换时前端只需改 catalog 不改逻辑

export interface ValidationResult {
  ok: boolean;
  message: string;
}

// === 用户名 ===
// 后端 validate.go: 3-32 字符；禁止 / \ @ : NUL < > " ' &
// 前端在后端基础上额外要求"以字母或下划线开头 + 只允许字母数字下划线"，
// 这是更严格的子集，便于路由 / 文件名 / Emby 同步等下游使用。
// 注意：先前 register/page.tsx 用的 {2,19} 把上限锁死在 20，与后端 32 不符，
// 导致 21-32 字符合法用户名静默被拒。
export const USERNAME_MIN_LEN = 3;
export const USERNAME_MAX_LEN = 32;
const USERNAME_REGEX = new RegExp(
  `^[A-Za-z_][A-Za-z0-9_]{${USERNAME_MIN_LEN - 1},${USERNAME_MAX_LEN - 1}}$`,
);

export function validateUsername(username: string): ValidationResult {
  const value = username.trim();
  if (!value) {
    return { ok: false, message: "请填写用户名" };
  }
  if (value.length < USERNAME_MIN_LEN || value.length > USERNAME_MAX_LEN) {
    return {
      ok: false,
      message: `用户名长度需为 ${USERNAME_MIN_LEN}-${USERNAME_MAX_LEN} 位`,
    };
  }
  if (!USERNAME_REGEX.test(value)) {
    return {
      ok: false,
      message: "用户名格式不正确：仅允许字母 / 数字 / 下划线，且不能以数字开头",
    };
  }
  return { ok: true, message: "" };
}

// === Emby 用户名 ===
// 后端没有显式约束，但出于 UI 防御性目的保留长度上限 + 排除控制字符。
const EMBY_USERNAME_MAX_LEN = 64;

export function validateEmbyUsername(username: string): ValidationResult {
  const value = username.trim();
  if (!value) {
    return { ok: false, message: "请填写 Emby 用户名" };
  }
  if (value.length > EMBY_USERNAME_MAX_LEN) {
    return { ok: false, message: `Emby 用户名长度不能超过 ${EMBY_USERNAME_MAX_LEN} 位` };
  }
  // U+0000-U+001F 为控制字符，剔除以避免 emby header 注入。
  // eslint-disable-next-line no-control-regex
  if (/[\x00-\x1f\x7f]/.test(value)) {
    return { ok: false, message: "Emby 用户名包含不可见字符" };
  }
  return { ok: true, message: "" };
}

// === Email（可选字段；register/admin create 用） ===
const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function validateEmailOptional(email: string): ValidationResult {
  const value = email.trim();
  if (!value) return { ok: true, message: "" };
  if (value.length > 254) {
    return { ok: false, message: "邮箱长度不能超过 254 位" };
  }
  if (!EMAIL_REGEX.test(value)) {
    return { ok: false, message: "邮箱格式不正确" };
  }
  return { ok: true, message: "" };
}

// === 注册码 / 邀请码长度 ===
// 后端 generators 默认 12 字符，但接受 6-32（见 internal/api 系列 handler）。
const CODE_MIN_LEN = 4;
const CODE_MAX_LEN = 64;

export function validateRegcodeFormat(code: string): ValidationResult {
  const value = code.trim();
  if (!value) {
    return { ok: false, message: "请填写注册码 / 邀请码" };
  }
  if (value.length < CODE_MIN_LEN || value.length > CODE_MAX_LEN) {
    return {
      ok: false,
      message: `注册码长度需为 ${CODE_MIN_LEN}-${CODE_MAX_LEN} 位`,
    };
  }
  if (!/^[A-Za-z0-9_-]+$/.test(value)) {
    return { ok: false, message: "注册码只允许字母 / 数字 / 下划线 / 短横线" };
  }
  return { ok: true, message: "" };
}

// === 数值范围（管理员 regcode / invite 创建表单用） ===
export function validatePositiveInt(
  value: number | string,
  label: string,
  opts: { min?: number; max?: number; allowMinusOne?: boolean } = {},
): ValidationResult {
  const n = typeof value === "string" ? Number(value) : value;
  if (!Number.isFinite(n) || !Number.isInteger(n)) {
    return { ok: false, message: `${label} 必须是整数` };
  }
  if (opts.allowMinusOne && n === -1) {
    return { ok: true, message: "" };
  }
  const min = opts.min ?? 1;
  if (n < min) {
    return { ok: false, message: `${label} 不能小于 ${min}` };
  }
  if (typeof opts.max === "number" && n > opts.max) {
    return { ok: false, message: `${label} 不能大于 ${opts.max}` };
  }
  return { ok: true, message: "" };
}

// === 后端错误码 → 前端友好文案映射 ===
// 见 internal/api/errcode.go + webui/src/lib/errcode.ts。
// 前端不再依赖 backend 的中文 message 做关键字匹配，而是基于稳定的 error_code
// 决定 UI 行为（与 ApiError.errorCode 配合，整改 46 后将在 admin / auth 页全
// 量铺开）。
// 类型选择 Partial<Record<ErrCode, string>>：所有 key 必须是已知 ErrCode，
// 重命名后端常量时 TS 立刻报错；但允许漏配 friendly 文案（不强制，
// 漏配的码 falls back 到 backend message，不阻塞增量迁移）。
import type { ErrCode } from "./errcode";

export const ERROR_CODE_FRIENDLY: Partial<Record<ErrCode, string>> = {
  AUTH_LOGIN_RATE_LIMITED: "登录过于频繁，请稍后再试",
  AUTH_LOGIN_INVALID: "用户名或密码错误",
  AUTH_ACCOUNT_DISABLED: "账号已被停用，请联系管理员",
  AUTH_ACCOUNT_EXPIRED: "账号有效期已到期，请续费后再继续",
  AUTH_DIRECT_LOGIN_DISABLED: "已禁用密码登录，请使用 API Key 或 Telegram",
  AUTH_PASSWORD_RESET_TOO_MANY: "密码重置过于频繁，请稍后再试",
  AUTH_PASSWORD_OLD_MISMATCH: "原密码不正确",
  AUTH_PASSWORD_WEAK: "密码强度不足",
  USER_REGISTER_RATE_LIMITED: "注册过于频繁，请稍后再试",
  USER_REGISTER_DISABLED: "已关闭新用户注册",
  USER_USERNAME_INVALID: "用户名格式不正确",
  USER_USERNAME_TAKEN: "该用户名已被占用",
  USER_NOT_FOUND: "用户不存在",
  USER_LIMIT_REACHED: "已达到用户上限",
  TG_BIND_REQUIRED: "请先完成 Telegram 绑定",
  TG_BIND_CODE_FORMAT_INVALID: "绑定码格式不正确",
  TG_BIND_CODE_EXPIRED: "绑定码已过期，请重新获取",
  TG_BIND_CODE_NOT_CONFIRMED: "请先在 Bot 私聊确认绑定",
  TG_BIND_CODE_SCENE_INVALID: "绑定码场景不匹配",
  TG_ALREADY_BOUND: "该 Telegram 已绑定其它账号",
  EMBY_AUTH_FAILED: "Emby 用户名或密码错误",
  EMBY_ACCOUNT_UNLINKED: "Emby 账号未关联",
  EMBY_UNBIND_FORBIDDEN: "当前账号不能自助解绑 Emby",
  EMBY_DISABLE_FAILED: "禁用远端 Emby 账号失败，已保留本地绑定",
  EMBY_ACCOUNT_CONFLICT: "目标 Emby 账号已被其它用户绑定",
  EMBY_CAPACITY_REACHED: "已达到 Emby 用户上限",
  EMBY_MISSING_CREDENTIALS: "请填写 Emby 用户名和密码",
  EMBY_INPUT_TOO_LONG: "Emby 用户名 / 密码长度超出限制",
  EMBY_DELETE_FAILED: "删除 Emby 账号失败，请稍后重试或联系管理员",
  CODE_EMPTY: "请填写卡码 / 邀请码",
  CODE_INVALID: "卡码无效或已过期",
  CODE_ALREADY_EMBY_BOUND: "当前账号已绑定 Emby，请使用续期码",
  CODE_REGISTRATION_GRANT_ALREADY_USED: "当前账号已使用过 Emby 注册资格，不能重复使用注册类卡码",
  REGCODE_NOT_FOUND: "注册码不存在",
  INVITE_DISABLED: "邀请功能未开启",
  INVITE_CANNOT_INVITE: "当前账号暂不能生成邀请码",
  INVITE_NOT_FOUND: "邀请码无效或已停用",
  INVITE_SELF_GENERATE: "不能使用自己生成的邀请码",
  INVITE_ALREADY_HAS_PARENT: "当前账号已存在邀请上级，不能重复加入邀请树",
  INVITE_TARGET_MISMATCH: "此邀请码仅限指定用户使用",
  INVITER_UNAVAILABLE: "邀请人状态不可用",
  INVITE_DEPTH_EXCEEDED: "邀请树层级已达上限",
  INVITE_ROOT_FULL: "邀请树人数已达上限",
  INVITER_DAYS_SHORT: "邀请人有效期不足",
  INVITE_DAYS_OUT_OF_RANGE: "邀请码天数超出允许范围",
  INVITE_EXPIRES_BEFORE_NOW: "邀请码过期时间必须晚于当前时间",
  INVITE_TARGET_USERNAME_INVALID: "目标用户名长度需为 3-32 字符，且不能包含特殊路径或注入字符",
  INVITE_GENERATION_CONFLICT: "邀请码 / 续期码生成冲突，请重试",
  INVITE_RENEW_USER_DISABLED: "账号已被禁用，无法生成续期码",
  INVITE_RENEW_REQUIRES_EMBY: "请先绑定 Emby 账号后再生成续期码",
  INVITE_RENEW_BAD_TARGET: "目标用户无效",
  INVITE_RENEW_NOT_DIRECT_CHILD: "只能给自己的直属下级生成续期码",
  INVITE_RENEW_TARGET_MISSING: "目标用户不存在",
  INVITE_RENEW_DAYS_OUT_OF_RANGE: "续期天数超出允许范围",
  INVITE_DETACH_NOT_DIRECT_CHILD: "只能断开自己的直属下级",
  INVITE_DETACH_NOT_EXPIRED: "只能断开已到期的下级",
  INVITE_EMBY_ALREADY_BOUND: "当前账号已绑定 Emby",
  REGCODE_TYPE_INVALID: "注册码类型无效",
  REGCODE_TARGET_USERNAME_INVALID: "目标用户名长度需为 3-32 字符，且不能包含特殊路径或注入字符",
  REGCODE_GENERATE_CONFLICT: "注册码生成冲突，请调整格式或随机算法后重试",
  REGCODE_BATCH_CONFIRM_REQUIRED: "请先输入批量删除确认短语",
  REGCODE_BATCH_EMPTY: "请选择要删除的注册码",
  REGCODE_BATCH_TOO_LARGE: "单次最多删除 200 个注册码",
  REGCODE_BATCH_DELETE_FAILED: "批量删除注册码失败，请稍后重试",
  SIGNIN_DISABLED: "签到功能未开启",
  DB_BACKUP_LIST_FAILED: "读取数据库备份列表失败",
  DB_BACKUP_INVALID: "备份文件无效",
  DB_BACKUP_READ_FAILED: "读取备份失败",
  DB_BACKUP_SNAPSHOT_INVALID: "备份内容不是有效的 Twilight 状态快照",
  DB_BACKUP_DELETE_FAILED: "删除数据库备份失败",
  DB_BACKUP_CREATE_FAILED: "数据库备份失败",
  DB_SNAPSHOT_FAILED: "生成数据库快照失败",
  DB_SNAPSHOT_VERIFY_FAILED: "数据库快照校验失败",
  DB_RESTORE_BACKUP_FAILED: "恢复 / 迁移前保护性备份失败",
  DB_RESTORE_FAILED: "数据库恢复失败",
  DB_MIGRATION_DISABLED: "数据库迁移功能未开启，请先在配置文件中启用 Database.migration_panel_enabled",
  DB_SQLITE_DISABLED: "SQLite 数据源已禁用；请使用当前运行状态或 PostgreSQL",
  DB_POSTGRES_DSN_MISSING: "未配置 PostgreSQL 连接信息",
  DB_POSTGRES_CONNECT_FAILED: "连接 PostgreSQL 失败",
  DB_POSTGRES_WRITE_FAILED: "写入 PostgreSQL 失败",
  DB_STATE_FILE_BAD_PATH: "目标状态文件路径无效",
  DB_STATE_FILE_MKDIR_FAILED: "创建数据库目录失败",
  DB_STATE_FILE_WRITE_FAILED: "写入状态文件失败",
  EMBY_NOT_CONFIGURED: "Emby 未配置，请先在系统配置中填写 Emby 服务地址",
  EMBY_REMOTE_USERS_FAILED: "读取 Emby 用户列表失败，请稍后重试或检查上游 Emby 状态",
  EMBY_REMOTE_ACTIVITY_FAILED: "读取 Emby 活动日志失败，请稍后重试或检查上游 Emby 状态",
  EMBY_REMOTE_SESSIONS_FAILED: "读取 Emby 会话失败，请稍后重试或检查上游 Emby 状态",
  EMBY_BROADCAST_TEXT_EMPTY: "请填写广播消息内容",
  EMBY_USERNAME_INVALID: "Emby 用户名不合法，长度需在 1-64 之间",
  EMBY_PASSWORD_TOO_SHORT: "密码长度需至少 8 位",
  EMBY_CREATE_FAILED: "创建 Emby 用户失败，请稍后重试",
  EMBY_CREATE_NO_ID: "Emby 未返回用户 ID，请检查上游 Emby 状态",
  EMBY_SET_PASSWORD_FAILED: "设置 Emby 用户密码失败，请稍后重试",
  ADMIN_EMBY_RESET_CONFIRM_REQUIRED: "请输入确认短语 RESET_ALL_EMBY 后再执行",
  ADMIN_BULK_EXPIRE_CONFIRM_REQUIRED: "请输入确认短语 BULK_EXPIRE_OK 后再执行",
  ADMIN_BULK_EXPIRE_DAYS_TOO_LARGE: "过期天数超出允许范围",
  ADMIN_BULK_EXPIRE_INVALID: "过期时间不合法",
  ADMIN_BULK_ENABLE_CONFIRM_REQUIRED: "请输入确认短语 BULK_ENABLE_DISABLED_OK 后再执行",
  ADMIN_CLEAR_PENDING_EMBY_CONFIRM_REQUIRED: "请输入确认短语 CLEAR_PENDING_EMBY_OK 后再执行",
  ADMIN_KICK_NO_EMBY_CONFIRM_REQUIRED: "请输入确认短语 KICK_NO_EMBY_OK 后再执行",
  ADMIN_ENABLE_REJOINED_CONFIRM_REQUIRED: "请输入确认短语 ENABLE_REJOINED_OK 后再执行",
  ADMIN_KICK_UNBOUND_CONFIRM_REQUIRED: "请输入确认短语 KICK_UNBOUND_OK 后再执行",
  ADMIN_WHITELIST_USERNAME_REQUIRED: "请填写用户名",
  REBIND_STATUS_INVALID: "无效的状态过滤值",
  REBIND_ACTION_INVALID: "操作必须是 approve 或 reject",
  REBIND_BATCH_SIZE_INVALID: "ids 数量需在 1-100 之间",
  TG_NOT_CONFIGURED: "Telegram 未配置，请先在系统配置中填写 Telegram Bot Token",
  AUTH_CREDENTIALS_EMPTY: "请填写用户名和密码",
  AUTH_SESSION_REFRESH_FAILED: "刷新登录态失败，请稍后再试",
  USER_NEW_USERNAME_REQUIRED: "请填写新用户名",
  USER_BACKGROUND_INVALID: "背景配置不合法",
  EMBY_ALREADY_LINKED: "当前账号已绑定 Emby",
  EMBY_NO_REGISTRATION_ENTITLEMENT: "当前账号尚未获得 Emby 注册资格，请联系管理员",
  EMBY_USERNAME_TAKEN: "该 Emby 用户名已被占用",
  EMBY_USERNAME_LOOKUP_FAILED: "校验 Emby 用户名失败，请稍后重试",
  EMBY_ADMIN_LINK_FORBIDDEN: "禁止绑定 Emby 管理员账号",
  EMBY_LINKED_OTHER_USER: "该 Emby 账号已绑定其它用户",
  EMBY_PASSWORD_UPDATE_FAILED: "更新 Emby 密码失败，请稍后重试",
  EMBY_CONNECT_FAILED: "Emby 调用失败，请稍后重试或检查上游 Emby 状态",
  EMBY_USER_LOOKUP_FAILED: "查询 Emby 用户失败，请稍后重试",
  EMBY_USER_NOT_FOUND: "Emby 用户不存在",
  EMBY_LATEST_FAILED: "读取 Emby 最新媒体失败，请稍后重试",
  RENEW_CODE_REQUIRED: "请填写续期码",
  RENEW_CODE_INVALID: "续期码无效或已过期",
  REGCODE_INVALID: "注册码无效或已过期",
  BIND_CODE_RATE_LIMITED: "绑定码请求过于频繁，请稍后再试",
  BIND_CODE_CONFLICT: "绑定码冲突，请稍后再试",
  BIND_CODE_NOT_FOUND: "绑定码不存在或已过期",
  TG_UNBIND_FORBIDDEN: "当前账号不允许解绑 Telegram",
  TG_NOT_BOUND: "当前账号尚未绑定 Telegram",
  TG_ID_INVALID: "Telegram ID 无效",
  TG_ID_TAKEN: "该 Telegram ID 已绑定其它账号",
  DEVICE_ID_REQUIRED: "请填写设备 ID",
  IP_REQUIRED: "请填写 IP",
  UPLOAD_RATE_LIMITED: "上传过于频繁，请稍后再试",
  UPLOAD_INVALID_PAYLOAD: "上传内容无效",
  UPLOAD_FILE_MISSING: "请选择要上传的文件",
  UPLOAD_FILE_TOO_LARGE: "文件过大，请压缩后再上传",
  UPLOAD_TYPE_NOT_ALLOWED: "不支持的文件类型",
  UPLOAD_DIR_INVALID: "上传目录无效",
  UPLOAD_DIR_CREATE_FAILED: "创建上传目录失败",
  UPLOAD_SAVE_FAILED: "保存文件失败",
  ASSET_NOT_FOUND: "资源不存在",
  CONFIG_FILE_NOT_FOUND: "配置文件不存在",
  API_DEPRECATED: "该接口已废弃，请使用最新接口",
  ADMIN_QUEUE_CLEAR_PARTIAL: "部分注册队列状态清理失败，请重试",
  ADMIN_DAYS_OUT_OF_RANGE: "天数超出允许范围",
  ADMIN_ENTITLEMENT_PARTIAL: "部分 Emby 注册资格发放失败，请重试",
  ADMIN_BULK_LIBRARY_CONFIRM_REQUIRED: "请先输入批量启用确认短语",
  ADMIN_PASSWORD_RESET_SCOPE_INVALID: "无效的密码重置范围",
  ADMIN_EMBY_PASSWORD_RESET_FAILED: "重置 Emby 密码失败，请稍后重试",
  ADMIN_LAST_ADMIN_PROTECTED: "无法移除最后一个管理员的权限，系统至少需要一个管理员",
  API_KEY_SELF_PERMISSION_FORBIDDEN: "不允许通过当前 API Key 修改自身权限",
  WATCH_STATS_FORBIDDEN: "无权查看其它用户的观看统计",
  // 中间件层（IP 黑名单 / 全局限流 / 路由分发）
  SECURITY_IP_BLACKLISTED: "当前 IP 已被封禁，请联系管理员申请解封",
  RATE_GLOBAL_LIMITED: "全局请求过于频繁，请稍后再试",
  ROUTE_METHOD_NOT_ALLOWED: "请求方法不允许",
  ROUTE_NOT_FOUND: "接口不存在",
  RATE_LIMITED: "请求过于频繁，请稍后再试",
  INVALID_PAYLOAD: "提交数据格式错误",
  INTERNAL_ERROR: "服务器内部错误，请稍后再试",
};

/**
 * 将后端 error_code 映射为友好文案；未命中时回落到后端 message。
 * 与 ApiError.errorCode 配合使用：
 *   catch (err) {
 *     if (err instanceof ApiError) {
 *       toast({ description: friendlyError(err.errorCode, err.backendMessage) });
 *     }
 *   }
 */
export function friendlyError(code: string | undefined, fallback?: string): string {
  if (code && code in ERROR_CODE_FRIENDLY) {
    const mapped = ERROR_CODE_FRIENDLY[code as ErrCode];
    if (mapped) return mapped;
  }
  return fallback || "操作失败";
}
