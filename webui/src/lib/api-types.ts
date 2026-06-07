import type { ErrCode } from "./errcode";

export interface ApiResponse<T = unknown> {
  success: boolean;
  code?: number;
  /**
   * 后端业务错误码。`ErrCode` 是 internal/api/errcode.go 的镜像联合
   * ；后端如果先行下发新增码会落入 `(string & {})`
   * 分支，前端通过 `isKnownErrCode()` 窄化后再消费，避免 TS 编译期硬卡。
   */
  error_code?: ErrCode | (string & {});
  message: string;
  data?: T;
}

export interface SystemInfo {
  name: string;
  icon: string;
  version: string;
  /** 后端 API 大版本（如 "v1"），便于前端将来做接口契约校验 */
  api_version?: string;
  /**
   * 会话 cookie 名（默认 `twilight_session`，运维可改）。
   * 前端不直接读 HttpOnly 的 session cookie，仅用于显示 / 调试。
   */
  session_cookie_name?: string;
  features: Record<string, boolean>;
  limits: Record<string, number | null>;
  telegram_bot?: {
    username: string | null;
    url: string | null;
    enabled?: boolean;
    configured?: boolean;
    ok?: boolean;
    error?: string;
  };
  telegram_links?: {
    groups: Array<{ label: string; url: string }>;
    channels: Array<{ label: string; url: string }>;
  };
  required_telegram_links?: {
    groups: Array<{ label: string; url: string }>;
    channels: Array<{ label: string; url: string }>;
  };
}

export interface SystemHealth {
  api: boolean;
  database: boolean;
  emby: boolean;
}

export interface User {
  uid: number;
  username: string;
  role: number;
  role_name: string;
}

export interface UserInfo {
  uid: number;
  username: string;
  email?: string;
  telegram_id?: number;
  telegram_username?: string;  // Telegram 用户名
  role: number;
  role_name: string;
  active: boolean;
  expire_status?: string;  // 后端计算的状态文本（"永不过期"/"已过期"/"剩余 x天"）
  expired_at?: string | number;  // 可能是时间戳或字符串，-1 表示永久
  emby_id?: string;
  emby_username?: string;  // 绑定的 Emby 用户名（与系统用户名独立）
  emby_bound?: boolean;  // 后端判定的「已绑定 Emby」：EMBYID 非空
  avatar?: string;
  bgm_mode: boolean;
  bgm_token_set?: boolean;
  bgm_sync_ready?: boolean;
  created_at?: string | number;
  register_time?: number;
  emby_grant_locked?: boolean;
  registration_source?: string;
  registration_source_name?: string;
  registration_code?: string;
  is_pending?: boolean;  // 是否待激活
  pending_emby?: boolean;  // 系统账号已建但待补建 Emby
  pending_emby_days?: number | null;  // 注册码授予的开通天数（待 Emby 补建）
  emby_disabled_by_expiry?: boolean;  // 到期后仅禁用 Emby，系统账号仍可登录
  rebinding_in_progress?: boolean;  // 是否处于强制换绑流程中
}

export interface BatchUserResult {
  total: number;
  success: number;
  failed: number;
  errors: Array<{ uid: number; error: string }>;
  selected_all?: boolean;
  emby_grant_locked?: boolean;
  skipped_no_emby?: number;
  emby_grant_cleared?: boolean;
  cleared?: number;
  skipped_has_emby?: number;
  skipped_pending?: number;
  regcode_refs_removed?: number;
  invite_refs_removed?: number;
}

export interface CodeUsePreview {
  source: "regcode" | "invite";
  type: number;
  type_name: string;
  days: number;
  valid: boolean;
  inviter?: string | null;
  requires_emby_credentials: boolean;
  confirm_title: string;
  description: string;
  duration_label: string;
  submit_label?: string;
}

export interface CodeUseResponse extends Partial<CodeUsePreview> {
  pending?: boolean;
  request_id?: string;
  status_token?: string;
  status?: "queued" | "processing" | "success" | "failed";
  queue_position?: number;
  reused?: boolean;
  emby_password?: string;
  expire_status?: string;
  expired_at?: string | number;
  role?: number;
  role_name?: string;
  user?: UserInfo;
}

export interface ApiKeyItem {
  id: number;
  name: string;
  key: string;            // masked, e.g. "key-xxxxxxxx…yyyyyyyy"
  key_prefix: string;
  key_suffix: string;
  enabled: boolean;
  allow_query: boolean;
  permissions?: string[];
  rate_limit: number;
  request_count: number;
  last_used: number | null;
  created_at: number;
  expired_at: number | null;
}

export interface UserSettings {
  bgm_mode: boolean;
  bgm_token_set: boolean;
  api_key_enabled: boolean;
  telegram: {
    bound: boolean;
    telegram_id?: string;
    telegram_id_full?: number;
    telegram_username?: string;
    force_bind: boolean;
    can_unbind: boolean;
    can_change: boolean;
    rebind_approved?: boolean;
    pending_rebind_request?: boolean;
    rebind_request_status?: string | null;
    rebind_request_id?: number | null;
  };
  emby_status: {
    is_synced: boolean;
    is_active: boolean;
    active_sessions: number;
    message: string;
  };
  system_config: {
    device_limit_enabled: boolean;
    max_devices: number;
    max_streams: number;
    bangumi_sync_enabled?: boolean;
  };
}

export interface EmbyStatus {
  is_synced: boolean;
  is_active: boolean;
  can_unbind?: boolean;
  active_sessions: number;
  message: string;
}

export interface TelegramStatus {
  bound: boolean;
  telegram_id?: string;
  telegram_id_full?: number;
  telegram_username?: string;  // Telegram 用户名
  force_bind: boolean;
  can_unbind: boolean;
  can_change: boolean;
  rebind_approved?: boolean;
  pending_rebind_request?: boolean;
  rebind_request_status?: string | null;
  rebind_request_id?: number | null;
  rebinding_in_progress?: boolean;
}

export interface TelegramRebindRequest {
  id: number;
  uid: number;
  username?: string | null;
  old_telegram_id?: number | null;
  status: string;
  reason?: string | null;
  admin_note?: string | null;
  reviewer_uid?: number | null;
  created_at: number;
  reviewed_at?: number | null;
}

export interface MediaItem {
  id: number;
  title: string;
  original_title?: string;
  overview?: string;
  poster?: string;
  poster_url?: string;
  year?: number;
  release_date?: string;
  source: string;
  source_url?: string;
  media_type: string;
  rating?: number;
  vote_average?: number;
}

export interface MediaDetail extends MediaItem {
  backdrop?: string;
  genres?: string[];
  runtime?: number;
  seasons?: number;
  episodes?: number;
  status?: string;
}

export interface InventoryCheckRequest {
  source: string;
  media_id: number;
  media_type: string;
  title?: string;
  original_title?: string;
  year?: number;
  season?: number;
}

export interface InventoryCheckResult {
  exists: boolean;
  message: string;
  media_item?: {
    id: string;
    name: string;
    year?: number;
  };
  seasons_available?: number[];
  season_requested?: number;
}

export interface MediaRequestData {
  source: string;
  media_id: number;
  media_type: string;
  title?: string;
  original_title?: string;
  poster?: string;
  poster_url?: string;
  overview?: string;
  season?: number;
  note?: string;
  year?: number;  // 年份限制
}

export interface MediaRequest {
  id: number;
  source: string;
  // Bangumi 端是 int，TMDB 端是 str（"12345" 或 "tv:12345"），所以这里宽放一些类型
  media_id: number | string;
  status: string; // UNHANDLED, ACCEPTED, REJECTED, COMPLETED
  timestamp: number;
  title: string;
  media_type: string;
  season?: number;
  // 后端始终下发；用作前端 React key 与 PUT/DELETE 的路由参数。
  require_key: string;
  media_info?: {
    title: string;
    media_type: string;
    season?: number;
    year?: number;
    note?: string;
    overview?: string;
    poster?: string;
    poster_url?: string;
    vote_average?: number;
    rating?: number;
    [key: string]: any;
  };
  admin_note?: string;
  user?: {
    telegram_id: number;
    username?: string;
    uid?: number;
  };
}

export interface EmbyInfo {
  server_name?: string;
  version?: string;
  user_id?: string;
  user_name?: string;
  online?: boolean;
  active_sessions?: number;
  total_sessions?: number;
  operating_system?: string;
  message?: string;
}

export interface EmbySession {
  id: string;
  device_name: string;
  client: string;
  now_playing?: string;
  last_activity: string;
}

export interface EmbyDevice {
  id: string;
  name: string;
  app_name: string;
  last_user?: string;
  last_used: string;
}

export interface RegisterData {
  telegram_bind_code?: string;
  username: string;
  password?: string;
  email?: string;
  reg_code?: string;
}

export interface RegisterResponse {
  registration_target?: "system" | "emby";
  uid?: number;
  username?: string;
  password?: string;
  user?: UserInfo;
  request_id?: string;
  status_token?: string;
  status?: "queued" | "processing" | "success" | "failed";
  queue_position?: number;
  reused?: boolean;
  reg_code_used?: string;
}

export interface RegisterAvailability {
  enabled?: boolean;
  can_register?: boolean;
  requires_reg_code?: boolean;
  available: boolean;
  message: string;
  current_users: number;
  max_users: number;
  register_mode: boolean;
  allow_pending_register: boolean;
  emby_direct_register_enabled: boolean;
  // 管理员单值固定的开通天数（-1 永久）；客户端只读
  emby_direct_register_days: number;
  emby_user_limit?: number;
  emby_bound_users?: number;
}

export interface EmbyRegisterStatus {
  request_id: string;
  status: "queued" | "processing" | "success" | "failed" | "rejected";
  queue_position?: number;
  message?: string;
  created_at?: number;
  updated_at?: number;
  finished_at?: number;
  data?: {
    uid?: number;
    username?: string;
    emby_password?: string;
  };
}

export interface AdminUserListParams {
  page?: number;
  per_page?: number;
  role?: number | null;
  active?: boolean | null;
  emby?: "bound" | "unbound" | null;
  search?: string;
  sort?: string;
}

export interface AdminUserListResponse {
  users: UserInfo[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface UserUpdateData {
  role?: number;
  active?: boolean;
  expired_at?: string;
}

export interface SystemStats {
  timestamp: number;
  cpu_count: number | null;
  cpu_percent?: number | null;
  memory?: {
    total: number;
    available: number;
    percent: number;
    used: number;
  } | null;
  disk?: {
    total: number;
    free: number;
    percent: number;
  } | null;
}

export interface RuntimeLogEntry {
  id: number;
  time: number;
  level: string;
  message: string;
  attrs?: Record<string, string>;
}

export interface RuntimeLogsResponse {
  entries: RuntimeLogEntry[];
  next_cursor: number;
  limit: number;
}

export interface RuntimeStatus {
  started_at: number;
  uptime_seconds: number;
  host_uptime_seconds?: number;
  hostname?: string;
  go_version: string;
  goos: string;
  goarch: string;
  goroutines: number;
  cpu_count: number;
  redis_enabled: boolean;
  routes: number;
  active_database: string;
  config_database: string;
  users: number;
  log_level?: string;
  runtime_log_limit?: number;
  runtime_log_entries?: number;
  runtime_log_backend?: string;
  load_average?: number[];
  memory?: Record<string, number>;
  host_memory?: Record<string, number>;
}

export interface Regcode {
  code: string;
  type: number;
  type_name: string;
  is_decoy?: boolean;
  days: number;
  validity_time?: number; // 注册码有效期（小时），-1 表示永久
  use_count?: number;
  use_count_limit?: number;
  active?: boolean;
  status?: "available" | "disabled" | "used_up" | "expired";
  note?: string;
  target_username?: string;
  target_telegram_username?: string;
  target_telegram_id?: number;
  target_uid?: number;
  target_resolved_username?: string;
  used: boolean;
  used_by?: number | string;
  used_by_uids?: number[];
  used_by_usernames?: string[];
  used_by_telegram_ids?: number[];
  created_at: string;
  created_time?: number; // 创建时间戳（兼容字段）
  used_at?: string;
}

export interface CreateRegcodeData {
  type: number;
  days: number;
  validity_time?: number; // 注册码有效期（小时），-1 表示永久
  use_count_limit?: number; // 使用次数限制，-1 表示无限
  count?: number;
  decoy?: boolean;
  format?: string;
  random_algorithm?: string;
  target_username?: string;
  target_telegram_username?: string;
  target_telegram_id?: number;
}

export interface BatchUserSelection {
  uids?: number[];
  select_all?: boolean;
  filter?: Pick<AdminUserListParams, "role" | "active" | "emby" | "search">;
}

export interface ConfigFieldOption {
  label: string;
  value: number | string;
}

export interface ConfigField {
  key: string;
  label: string;
  type: 'string' | 'textarea' | 'int' | 'float' | 'bool' | 'secret' | 'list' | 'select' | 'command_map';
  description: string;
  value: unknown;
  options?: ConfigFieldOption[];
}

export interface ConfigSection {
  key: string;
  title: string;
  description: string;
  fields: ConfigField[];
  /** 类别 key，与 ConfigSchema.categories 中的 key 对应。后端可缺省。 */
  category?: string;
}

export interface ConfigCategory {
  key: string;
  title: string;
}

export interface ConfigSchema {
  sections: ConfigSection[];
  /** 类别声明（顺序即渲染顺序）；后端可缺省，前端会回落为单一类别 */
  categories?: ConfigCategory[];
}


export interface DatabaseBackup {
  name: string;
  path: string;
  size: number;
  created_at: number;
  note?: string;
}

export type ConfigBackup = DatabaseBackup;

export interface ConfigBackupView {
  backup: ConfigBackup;
  content: string;
  config_file: string;
}

export interface ConfigRestoreResult {
  operation: "restore_config" | string;
  dry_run: boolean;
  requires_confirmation?: boolean;
  confirm?: string;
  restored: string;
  backup: ConfigBackup;
  config_file: string;
  content_bytes: number;
  warnings?: string[];
  pre_restore_backup?: ConfigBackup;
  pre_operation_backup?: ConfigBackup;
  reload?: Record<string, unknown>;
}

export interface DatabaseBackupInspectResult {
  backup: DatabaseBackup;
  snapshot_bytes: number;
  counts: Record<string, number>;
  users: number;
  api_keys: number;
  regcodes: number;
  invite_codes: number;
  media_requests: number;
  announcements: number;
}

export interface DatabaseStatus {
  active_driver: string;
  configured_driver: string;
  active_label?: string;
  configured_label?: string;
  supported_drivers?: Array<{ driver: string; label: string; role: string }>;
  state_file: string;
  backup_dir: string;
  backup_count: number;
  migration_panel_enabled?: boolean;
  postgres_configured: boolean;
  redis_enabled: boolean;
  user_count: number;
}

export interface DatabaseOperationResult {
  operation?: "restore" | "migrate" | string;
  source_driver?: string;
  configured_driver?: string;
  target_driver?: string;
  dry_run: boolean;
  requires_confirmation?: boolean;
  confirm?: string;
  snapshot_bytes?: number;
  target_snapshot_bytes?: number;
  current_snapshot_bytes?: number;
  source_ready?: Record<string, unknown>;
  target_ready?: Record<string, unknown>;
  backup_ready?: Record<string, unknown>;
  warnings?: string[];
  counts?: Record<string, number>;
  current_counts?: Record<string, number>;
  users: number;
  api_keys: number;
  regcodes: number;
  invite_codes: number;
  media_requests: number;
  announcements: number;
  state_file?: string;
  backup?: DatabaseBackup;
  restored?: string;
  pre_restore_backup?: DatabaseBackup;
  pre_migration_backup?: DatabaseBackup;
  pre_operation_backup?: DatabaseBackup;
}

export type DatabaseMigrationResult = DatabaseOperationResult & {
  target_driver: string;
};

export type DatabaseRestoreResult = DatabaseOperationResult & {
  restored: string;
  backup?: DatabaseBackup;
};

export interface SchedulerJobRun {
  id?: number;
  job_id?: string;
  type?: "auto" | "manual";
  trigger?: string;
  status: "running" | "success" | "failed";
  started_at: number;
  finished_at: number | null;
  error: string | null;
  summary?: Record<string, unknown> | null;
  logs?: string[];
}

export type SchedulerTriggerSpec =
  | { type: "cron_daily"; hour: number; minute: number }
  | { type: "interval"; seconds: number }
  | { type: "manual" };

export type SchedulerSchedulePayload = SchedulerTriggerSpec & {
  runtime_params?: Record<string, unknown> | null;
};

export interface SchedulerJobItem {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  schedule: string | null;
  next_run_at: number | null;
  last_run: SchedulerJobRun | null;
  is_running: boolean;
  trigger_spec: SchedulerTriggerSpec;
  default_trigger_spec: SchedulerTriggerSpec;
  is_custom: boolean;
  auto_disabled?: boolean;
  last_auto_run_at?: number | null;
  last_manual_run_at?: number | null;
  persisted_info?: Record<string, unknown> | null;
  /**
   * 手动专属任务：不接受定时触发器，仅能手动触发。
   * 后端在 JOB_DEFINITIONS 上打的标记，下发到前端用于隐藏"编辑触发器"按钮。
   */
  manual_only?: boolean;
  runtime_params?: Record<string, unknown> | null;
}


export type AnnouncementRenderMode = "plain" | "markdown" | "bbcode";

export interface Announcement {
  id: number;
  title: string | null;
  content: string;
  level: "info" | "notice" | "warning" | "critical";
  render_mode?: AnnouncementRenderMode;
  pinned: boolean;
  visible: boolean;
  expires_at: number; // -1 = 永不过期
  created_at: number;
  updated_at: number;
  created_by_uid?: number | null;
}

// ==================== 邀请树 ====================
export interface InviteConfig {
  enabled: boolean;
  max_depth: number;
  invite_limit: number;
  require_emby: boolean;
  default_days: number;
  code_format?: string;
  permanent_invite_max_days?: number;
}

export interface InviteCodeItem {
  code: string;
  inviter_uid: number;
  inviter_username?: string;
  days: number;
  use_count_limit: number;
  use_count: number;
  expires_at: number | null;
  active: boolean;
  created_at: number;
  used_by_uid?: number | null;
  used_by_username?: string;
  used_at?: number | null;
  note?: string | null;
  target_username?: string;
  target_uid?: number;
}

export interface InviteTreeNode {
  uid: number;
  username: string;
  active: boolean;
  has_emby: boolean;
  expired_at?: number | null;
  expire_status?: string;
  emby_expired?: boolean;
  can_delete_emby_and_detach?: boolean;
  depth: number;
  children?: InviteTreeNode[];
}

export interface InviteMyStatus {
  enabled: boolean;
  is_root: boolean;
  parent: { uid: number; username: string } | null;
  children: Array<{
    uid: number;
    username: string;
    active: boolean;
    has_emby: boolean;
    expired_at?: number | null;
    expire_status?: string;
    emby_expired?: boolean;
    can_generate_renew_code?: boolean;
    can_delete_emby_and_detach?: boolean;
  }>;
  tree?: {
    self: InviteTreeNode;
    descendants: InviteTreeNode[];
    descendant_count: number;
  };
  depth: number;
  max_depth: number;
  can_invite: boolean;
  invite_block_reason?: string;
  max_code_days?: number;
  max_code_days_reason?: string;
}

export interface InviteForestNode {
  uid: number;
  username: string;
  role: number;
  emby_id?: string | null;
  active: boolean;
  telegram_id?: number | null;
  register_time?: number | null;
  expired_at?: number | null;
  is_root: boolean;
}

export interface InviteForestEdge {
  parent: number;
  child: number;
}

export interface InviteForest {
  nodes: InviteForestNode[];
  edges: InviteForestEdge[];
  roots: number[];
  max_depth: number;
  config: {
    enabled: boolean;
    max_depth: number;
    invite_limit: number;
    require_emby: boolean;
  };
}

// ==================== 签到 / 积分 ====================
export interface SigninRenewalConfig {
  enabled: boolean;
  cost: number;
  days: number;
  affordable?: boolean;
}

export interface SigninSummary {
  enabled: boolean;
  currency_name: string;
  current_points: number;
  current_streak: number;
  longest_streak: number;
  total_points: number;
  last_signin_date: string | null;
  today_signed: boolean;
  next_bonus_in_days: number | null;
  next_bonus_points: number | null;
  renewal?: SigninRenewalConfig;
}

export interface SigninBonusRule {
  streak_days: number;
  bonus_points: number;
}

export interface SigninPublicConfig {
  enabled: boolean;
  currency_name: string;
  daily_min: number;
  daily_max: number;
  streak_bonus_enabled: boolean;
  bonus_table: SigninBonusRule[];
  reset_after_miss: boolean;
  renewal?: SigninRenewalConfig;
}

export interface SigninActionResult {
  created?: boolean;
  today_signed: boolean;
  daily_points: number;
  bonus_points: number;
  total_today: number;
  current_streak: number;
  longest_streak: number;
  current_points: number;
  currency_name: string;
  renewal?: SigninRenewalConfig;
}

export interface SigninRenewalResult {
  currency_name: string;
  spent_points: number;
  remaining_points: number;
  renewal: SigninRenewalConfig;
  expire_status: string;
  expired_at: string | number;
  user?: UserInfo;
}

export interface SigninHistoryRecord {
  date: string;
  daily_points: number;
  bonus_points: number;
  total: number;
  streak: number;
  created_at: number;
}

// ==================== 违规审计 ====================
export interface ViolationLog {
  id: number;
  uid: number;
  username: string;
  code: string;
  code_type: string;
  reason: string;
  action: string;
  ip: string | null;
  telegram_id: number | null;
  created_at: number;
}
