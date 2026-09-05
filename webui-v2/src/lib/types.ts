export interface ApiEnvelope<T> {
  success: boolean;
  code: number;
  error_code?: string;
  message: string;
  data?: T;
  timestamp: number;
}

export interface UserInfo {
  uid: number;
  username: string;
  email?: string;
  email_verified?: boolean;
  telegram_username?: string;
  role: number;
  role_name?: string;
  active: boolean;
  emby_id?: string;
  emby_username?: string;
  emby_bound?: boolean;
  emby_disabled?: boolean;
  emby_disabled_by_expiry?: boolean;
  pending_emby?: boolean;
  pending_emby_days?: number | null;
  registration_source_name?: string;
  admin_action_state?: AdminUserActionState;
  telegram_id?: number;
  expired_at?: number;
  avatar?: string | null;
  background?: string | null;
}

export interface BackgroundConfig {
  lightBg: string;
  darkBg: string;
  lightBgImage: string;
  darkBgImage: string;
  lightFlow: boolean;
  darkFlow: boolean;
  lightBlur: number;
  darkBlur: number;
  lightOpacity: number;
  darkOpacity: number;
}

export interface AdminUserActionState {
  has_emby: boolean;
  protected_role: boolean;
  can_enable_emby: boolean;
  can_disable_emby: boolean;
  can_grant_registration_entitlement: boolean;
  can_clear_registration_queue: boolean;
  can_delete: boolean;
  reasons?: Record<string, string>;
}

export interface AdminUserListResponse {
  users: UserInfo[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface AdminUsersPageData {
  payload: AdminUserListResponse | null;
  query: {
    page: number;
    per_page: number;
    search: string;
    role: string;
    active: string;
    emby: string;
    emby_status: string;
    email_status: string;
    sort: string;
  };
  loadError: string | null;
}

export interface V2Capabilities {
  api_version: string;
  compatible_api_versions: string[];
  server_version: string;
  features: Record<string, boolean>;
  limits: Record<string, number>;
  links: Record<string, string>;
}

export interface ViewerCount {
  viewers: number;
}

export interface DashboardSummary {
  user: UserInfo;
  capabilities: V2Capabilities;
  viewers: {
    available: boolean;
    count: number;
  };
}

export interface SigninRenewal {
  enabled: boolean;
  cost: number;
  days: number;
  affordable?: boolean;
  auto_renewal_enabled?: boolean;
  auto_renewal_user_enabled?: boolean;
  auto_renewal_available?: boolean;
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
  renewal?: SigninRenewal;
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
  renewal?: SigninRenewal;
}

export interface SigninHistoryRecord {
  date: string;
  daily_points: number;
  bonus_points: number;
  total: number;
  streak: number;
  created_at: number;
}

export interface SigninPageData {
  summary: SigninSummary;
  config: SigninPublicConfig;
  history: SigninHistoryRecord[];
}

export interface EmailCodeSent {
  verification_id: string;
  email: string;
  expires_in: number;
  resend_after: number;
}

export interface EmailVerificationRecord {
  id: string;
  purpose: string;
  email: string;
  email_masked?: string;
  uid: number | null;
  username: string | null;
  attempts: number;
  max_attempts: number;
  created_at: number;
  expires_at: number;
  last_sent_at: number;
  expired: boolean;
}

export interface EmailAccountRecord {
  uid: number;
  username: string;
  email: string;
  email_verified: boolean;
  email_verified_at: number | null;
  telegram_id: number | null;
  telegram_username: string | null;
  role: number;
  active: boolean;
}

export interface EmailAdminSummary {
  total_pending: number;
  expired_pending: number;
  total_with_email: number;
  verified: number;
  unverified: number;
}

export interface EmailAdminData {
  view?: "pending" | "accounts" | "summary" | "";
  page?: number;
  per_page?: number;
  total?: { pending: number; accounts: number };
  pages?: { pending: number; accounts: number };
  smtp_configured: boolean;
  email_enabled: boolean;
  force_bind: boolean;
  pending: EmailVerificationRecord[];
  accounts: EmailAccountRecord[];
  summary: EmailAdminSummary;
}

export interface AdminEmailPageData {
  payload: EmailAdminData | null;
  view: "pending" | "accounts";
  page: number;
  perPage: number;
  search: string;
  verified: "all" | "verified" | "unverified";
  loadError: string | null;
  notice: "revoked" | "cleaned" | "cleared" | "";
}

export interface ApiKeyItem {
  id: number;
  name: string;
  key: string;
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

export interface MyApiKeysPageData {
  keys: ApiKeyItem[];
  total: number;
  loadError: string | null;
  notice: "updated" | "deleted" | "";
}

export interface DeveloperJSPreset {
  id: number;
  name: string;
  description?: string;
  code: string;
  creator_uid?: number;
  created_at: number;
  updated_at: number;
}

export interface DeveloperJSPreviewResult {
  ok: boolean;
  errors: string[];
  warnings: string[];
  risk_tokens?: string[];
  output?: string;
  logs?: string[];
  duration_ms?: number;
  metrics?: { bytes: number; chars: number; lines: number; max_bytes: number; timeout_ms: number; reply_limit: number; log_limit: number };
  diagnostics?: { severity: string; blocked_count: number; risk_count: number; requires_review: boolean };
  preview_context?: { command: string; args: string[]; private_chat: boolean };
}

export interface DeveloperJSDocParam {
  name: string;
  type?: string;
  required?: boolean;
  description: string;
  default?: string;
}

export interface DeveloperJSDocEntry {
  name: string;
  category: string;
  type?: string;
  description: string;
  example?: string;
  mutates?: boolean;
  scope?: string;
  fields?: string[];
  params?: DeveloperJSDocParam[];
  returns?: string;
}

export interface DeveloperJSDocs {
  engine: { name: string; module: string; version: string; description: string; language: string; timeout_ms: number; sandbox: string[] };
  bindings: DeveloperJSDocEntry[];
  functions: DeveloperJSDocEntry[];
  namespaces: DeveloperJSDocEntry[];
  native_objects: DeveloperJSDocEntry[];
  config_keys: string[];
  env_keys: string[];
  examples: Array<{ id: string; title: string; description: string; code: string }>;
  blocked_tokens: string[];
  risk_tokens?: string[];
}

export interface AdminDeveloperPageData {
  developerModeEnabled: boolean;
  presets: DeveloperJSPreset[];
  docs: DeveloperJSDocs | null;
  loadError: string | null;
  notice: "saved" | "deleted" | "";
}

export interface TelegramSettings {
  bound: boolean;
  telegram_id?: number;
  telegram_username?: string;
  force_bind: boolean;
  can_unbind: boolean;
  can_change: boolean;
  pending_rebind_request?: boolean;
  rebind_request_status?: string | null;
}

export interface UserSettings {
  bgm_mode: boolean;
  bgm_manage_mode?: boolean;
  bgm_token_set: boolean;
  api_key_enabled: boolean;
  notify_on_login_telegram?: boolean;
  notify_on_login_email?: boolean;
  notify_on_ticket_telegram?: boolean;
  signin_auto_renewal?: boolean;
  password_change_email_required?: boolean;
  emby_password_email_required?: boolean;
  emby_password_old_password_required?: boolean;
  password_change_email_forced?: boolean;
  emby_password_email_forced?: boolean;
  telegram: TelegramSettings;
  emby_status: {
    is_synced: boolean;
    is_active: boolean;
    can_unbind?: boolean;
    active_sessions: number;
    message: string;
  };
  system_config: {
    device_limit_enabled: boolean;
    max_devices: number;
    max_streams: number;
    bangumi_sync_enabled?: boolean;
    bangumi_manage_enabled?: boolean;
  };
}

export interface LoginPayload {
  username: string;
  password: string;
}

export interface RegisterAvailability {
  enabled?: boolean;
  can_register?: boolean;
  requires_reg_code?: boolean;
  available: boolean;
  message: string;
  current_users: number;
  max_users: number;
  allow_pending_register?: boolean;
}

export interface RegisterResponse {
  user?: UserInfo;
  first_admin?: boolean;
  reg_code_used?: string;
  email_verification_sent?: string;
}

export interface InviteConfig {
  enabled: boolean;
  max_depth: number;
  invite_limit: number;
  invite_root_user_limit: number;
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
  expires_at?: number | null;
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
  emby_disabled?: boolean;
  expired_at?: number | null;
  expire_status?: string;
  emby_expired?: boolean;
  can_delete_emby_and_detach?: boolean;
  depth: number;
  children?: InviteTreeNode[];
}

export interface InviteChild {
  uid: number;
  username: string;
  active: boolean;
  has_emby: boolean;
  emby_disabled?: boolean;
  expired_at?: number | null;
  expire_status?: string;
  emby_expired?: boolean;
  can_generate_renew_code?: boolean;
  can_delete_emby_and_detach?: boolean;
}

export interface InviteStatus {
  enabled: boolean;
  is_root: boolean;
  parent: { uid: number; username: string } | null;
  children: InviteChild[];
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
  codes: InviteCodeItem[];
  total: number;
}

export interface V2InviteSummary {
  config: InviteConfig;
  invite: InviteStatus;
}

export interface AdminInviteTreeRow {
  uid: number;
  username: string;
  role: number;
  emby_bound: boolean;
  emby_disabled?: boolean;
  active: boolean;
  telegram_id?: number | null;
  register_time?: number | null;
  expired_at?: number | null;
  is_root: boolean;
  depth: number;
  root_uid: number;
  direct_children: number;
  descendants: number;
  collapsed: boolean;
}

export interface AdminInviteTreePage {
  rows: AdminInviteTreeRow[];
  selected: AdminInviteTreeRow | null;
  roots: Array<{ uid: number; username: string }>;
  total_rows: number;
  total_nodes: number;
  total_relations: number;
  max_depth: number;
  page: number;
  per_page: number;
  pages: number;
  config: InviteConfig;
}

export interface AdminInviteCodesPage {
  codes: InviteCodeItem[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface AdminInvitePageData {
  view: "tree" | "codes" | "config";
  tree: AdminInviteTreePage | null;
  codes: AdminInviteCodesPage | null;
  config: ConfigSection | null;
  query: {
    view: "tree" | "codes" | "config";
    page: number;
    per_page: number;
    search: string;
    root: string;
    selected: number;
    collapsed: number[];
    code_page: number;
    code_per_page: number;
    code_search: string;
  };
  notice: "detached" | "deleted_emby" | "batch_detached" | "quick_maintained" | "cascade_updated" | "deleted" | "config_saved" | "";
  loadError: string | null;
}

export interface BangumiSyncLog {
  id: number;
  uid: number;
  record_item_id: string;
  subject_id?: string;
  subject_name?: string;
  episode?: number;
  status: string;
  message?: string;
  created_at: number;
}

export interface BangumiSubject {
  id?: string;
  name?: string;
  name_cn?: string;
  summary?: string;
  date?: string;
  platform?: string;
  eps?: number;
  volumes?: number;
  images?: Record<string, string>;
  rating?: { score?: number; rank?: number; total?: number };
  tags?: string[];
}

export interface BangumiCollectionEntry {
  subject_id: number;
  type: number;
  ep_status: number;
  rate: number;
  updated_at: number;
  collection_type?: number;
  subject?: BangumiSubject;
}

export interface BangumiCollectionPreview {
  entries: BangumiCollectionEntry[];
  total: number;
  cached: boolean;
  cache_updated_at?: number;
}

export interface BangumiAccount {
  id?: number;
  username?: string;
  nickname?: string;
  sign?: string;
  avatar?: Record<string, string>;
  expired?: boolean;
}

export interface BangumiStatus {
  sync_enabled: boolean;
  manage_enabled: boolean;
  bgm_mode: boolean;
  bgm_manage_mode: boolean;
  token_set: boolean;
  sync_ready: boolean;
  total_records: number;
  synced_count: number;
  recent_logs: BangumiSyncLog[];
}

export interface AdminBangumiUser {
  uid: number;
  username: string;
  bgm_mode: boolean;
  bgm_manage_mode: boolean;
  token_set: boolean;
  sync_ready: boolean;
  sync_count: number;
  record_count: number;
}

export interface PlaybackRecordWithSync {
  uid: number;
  item_id: string;
  title: string;
  series_name?: string;
  media_type: string;
  index_number?: number;
  duration: number;
  played_at: number;
  synced_name?: string;
}

export interface AdminBangumiUsersResult {
  users: AdminBangumiUser[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface AdminBangumiDetail {
  kind: "records" | "logs";
  uid: number;
  user: AdminBangumiUser | null;
  records: PlaybackRecordWithSync[];
  logs: BangumiSyncLog[];
  error: string | null;
}

export interface AdminBangumiPageData {
  info: SystemInfo | null;
  users: AdminBangumiUsersResult | null;
  detail: AdminBangumiDetail | null;
  query: { page: number; per_page: number; search: string; detail: "records" | "logs" | ""; uid: number };
  notice: "synced" | "logs_cleared" | "";
  loadError: string | null;
}

export interface BangumiSummary {
  status: BangumiStatus;
  account?: BangumiAccount;
  collections?: Record<string, BangumiCollectionPreview>;
  recent_activity?: BangumiCollectionEntry[];
  account_error?: boolean;
  collections_partial?: boolean;
}

export interface BangumiCollectionPage {
  entries: BangumiCollectionEntry[];
  total: number;
  limit: number;
  offset: number;
  cached: boolean;
  cache_updated_at?: number | null;
}

export interface MediaItem {
  id: number;
  title: string;
  original_title?: string;
  overview?: string;
  poster?: string;
  poster_url?: string;
  year?: number | string | null;
  release_date?: string;
  source: string;
  source_url?: string;
  media_type: string;
  rating?: number;
  vote_average?: number;
  media_type_label?: string;
  logo?: string;
  logo_url?: string;
  logo_language?: "zh" | "ja" | "en" | string;
}

export interface MediaDetail extends MediaItem {
  backdrop?: string;
  backdrop_url?: string;
  genres?: string[];
  runtime?: number;
  seasons?: number;
  episodes?: number;
  volumes?: number;
  status?: string;
  end_date?: string;
  tagline?: string;
  platform?: string;
  broadcast?: string;
  official_url?: string;
  trailer_url?: string;
  rank?: number;
  vote_count?: number;
  countries?: string[];
  languages?: string[];
  aliases?: string[];
  creators?: string[];
  studios?: string[];
  cast?: string[];
  extra?: Record<string, unknown>;
}

export interface InventoryCheckResult {
  exists: boolean;
  message: string;
  media_item?: { id: string; name: string; year?: number };
  item?: { id: string; name: string; year?: number };
  seasons_available?: number[];
  season_requested?: number | null;
}

export interface MediaRequest {
  id: number;
  revision: number;
  source: string;
  media_id: number | string;
  status: string;
  timestamp: number;
  updated_at?: number;
  title: string;
  original_title?: string;
  media_type: string;
  season?: number;
  require_key: string;
  media_info?: Record<string, unknown>;
  admin_note?: string;
  note?: string;
}

export interface AdminMediaRequest extends MediaRequest {
  user?: { uid?: number; username?: string; telegram_id?: number };
  group_key?: string;
  group_count?: number;
  grouped_requests?: AdminMediaRequest[];
}

export interface MediaRequestStatusCounts {
  all: number;
  active: number;
  pending: number;
  accepted: number;
  downloading: number;
  rejected: number;
  completed: number;
}

export interface AdminMediaRequestListResponse {
  requests: AdminMediaRequest[];
  total: number;
  request_total: number;
  page: number;
  per_page: number;
  total_pages: number;
  has_next: boolean;
  status_counts: MediaRequestStatusCounts;
}

export type TicketStatus = "open" | "in_progress" | "resolved" | "closed";
export type TicketPriority = "low" | "medium" | "high" | "urgent";

export interface TicketReply {
  uid: number;
  username: string;
  role: number;
  author?: "admin" | "user";
  content: string;
  created_at: number;
}

export interface TicketAttachment {
  filename: string;
  content_type: string;
  size: number;
  uploaded_uid: number;
  created_at: number;
  url: string;
}

export interface TicketSummary {
  id: number;
  title: string;
  type: string;
  status: TicketStatus;
  priority: TicketPriority;
  reply_count: number;
  attachment_count: number;
  notify_telegram: boolean;
  created_at: number;
  updated_at: number;
  resolved_at?: number;
  closed_at?: number;
}

export interface AdminTicketSummary extends TicketSummary {
  uid: number;
  username: string;
  admin_note?: string;
}

export interface AdminTicketListResponse {
  tickets: AdminTicketSummary[];
  total: number;
  page: number;
  per_page: number;
  ticket_types: string[];
}

export interface AdminTicketDetailResponse {
  ticket: Ticket;
  ticket_types: string[];
}

export interface Ticket extends TicketSummary {
  uid: number;
  username: string;
  content: string;
  admin_note?: string;
  replies?: TicketReply[];
  attachments?: TicketAttachment[];
}

export interface SystemInfo {
  name?: string;
  icon?: string;
  version?: string;
  api_version?: string;
  features?: Record<string, boolean>;
  limits?: Record<string, number | null>;
  storage_mismatch?: boolean;
  storage_warning?: string;
  telegram_bot?: {
    username?: string | null;
    url?: string | null;
    enabled?: boolean;
    configured?: boolean;
  };
  setup?: SetupStatus;
}

export interface SetupStatus {
  available: boolean;
  setup_mode?: boolean;
  reasons: string[];
  user_count: number;
  config_file_exists: boolean;
}

export interface SetupLine {
  name?: string;
  url: string;
}

export interface SetupPayload {
  admin: { username: string; password: string; email?: string };
  global?: { server_name?: string };
  emby?: { emby_url?: string; emby_token?: string; emby_url_list?: SetupLine[] };
  telegram?: { enabled?: boolean; bot_token?: string; admin_id?: string[] };
  email?: {
    enabled?: boolean;
    smtp_host?: string;
    smtp_port?: number;
    smtp_username?: string;
    smtp_password?: string;
    smtp_from_address?: string;
    smtp_encryption?: string;
  };
  policy?: { register_mode?: boolean; register_code_limit?: boolean; allow_pending_register?: boolean };
}

export interface SetupResult {
  user: UserInfo;
  setup_completed: boolean;
}

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

export interface SchedulerJobItem {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  next_run_at: number | null;
  last_run: SchedulerJobRun | null;
  is_running: boolean;
  trigger_spec: SchedulerTriggerSpec;
  default_trigger_spec: SchedulerTriggerSpec;
  is_custom: boolean;
  auto_disabled?: boolean;
  last_auto_run_at?: number | null;
  last_manual_run_at?: number | null;
  manual_only?: boolean;
  runtime_params?: Record<string, unknown> | null;
}

export interface SchedulerRunDetail {
  job_id: string;
  last_run: SchedulerJobRun | null;
  history: SchedulerJobRun[];
}

export interface AdminSchedulerPageData {
  jobs: SchedulerJobItem[];
  view: string;
  notice: string;
  selected_job_id: string;
  selected_job: SchedulerJobItem | null;
  logs: SchedulerRunDetail | null;
  refreshed_at: number;
  load_error: string | null;
}

export interface SystemHealthDetail {
  ok?: boolean;
  online?: boolean;
  configured?: boolean;
  status?: string;
  error?: string;
  warning?: string;
  backend?: string;
  configured_driver?: string;
  storage_mismatch?: boolean;
  storage_warning?: string;
  state_read_ok?: boolean;
  user_count?: number;
  ping_ok?: boolean;
  open_connections?: number;
  in_use?: number;
  idle?: number;
  server_name?: string;
  version?: string;
  operating_system?: string;
  active_sessions?: number;
  total_sessions?: number;
  sessions_error?: string;
  routes?: number;
  uptime?: number;
  timestamp?: number;
}

export interface SystemStats {
  timestamp?: number;
  users?: {
    active?: number;
    total?: number;
    limit?: number | null;
    usage_percent?: number;
  };
  regcodes?: {
    active?: number;
    total?: number;
  };
  redis_enabled?: boolean;
  redis_fallback?: {
    session?: number;
    rate?: number;
  };
  routes?: number;
  uptime?: number;
}

export interface HealthProbe<T> {
  available: boolean;
  data: T | null;
}

export interface AdminStatusPageData {
  health: {
    api: HealthProbe<SystemHealthDetail>;
    database: HealthProbe<SystemHealthDetail>;
    emby: HealthProbe<SystemHealthDetail>;
  };
  info: SystemInfo | null;
  stats: SystemStats | null;
  refreshed_at: number;
}

export interface AdminHomePageData {
  info: SystemInfo | null;
  stats: SystemStats | null;
  loadError: string | null;
}

export type ConfigFieldType = "string" | "textarea" | "int" | "float" | "bool" | "secret" | "list" | "select" | "command_map";

export interface ConfigFieldOption {
  label: string;
  value: string | number;
}

export interface ConfigField {
  key: string;
  label: string;
  type: ConfigFieldType;
  description: string;
  value: unknown;
  options?: ConfigFieldOption[];
  placeholder_hints?: string[];
}

export interface ConfigSection {
  key: string;
  title: string;
  description: string;
  category?: string;
  collapsed?: boolean;
  fields: ConfigField[];
}

export interface ConfigCategory {
  key: string;
  title: string;
}

export interface ConfigSchema {
  sections: ConfigSection[];
  categories?: ConfigCategory[];
}

export interface TelegramCommandCatalogItem {
  command: string;
  name: string;
  label: string;
  description: string;
  usage: string;
  category: "user" | "admin" | "system" | "group" | (string & {});
  private: boolean;
  admin: boolean;
  disableable: boolean;
  disabled: boolean;
}

export interface TelegramCommandCatalog {
  commands: TelegramCommandCatalogItem[];
  disabled_commands: string[];
}

export interface TelegramRosterStats {
  available: boolean;
  reason?: string;
  chat_id?: string;
  active?: number;
  inactive?: number;
  bots?: number;
  first_seen_at?: number | null;
  last_seen_at?: number | null;
}

export interface TelegramBotTestResult {
  target: string;
  success: boolean;
  error?: string;
  username?: string;
  bot_id?: number | null;
  title?: string;
  bot_status?: string;
}

export interface TelegramBotRuntime {
  polling?: boolean;
  last_ok_at?: number | null;
  last_error_at?: number | null;
}

export interface AdminTelegramPageData {
  telegram: ConfigSection | null;
  commands: TelegramCommandCatalog | null;
  roster: TelegramRosterStats | null;
  errors: string[];
  notice: string;
}

export interface TelegramRebindRequest {
  id: number;
  uid: number;
  username?: string | null;
  old_telegram_id?: number | null;
  status: "pending" | "approved" | "rejected" | "revoked" | (string & {});
  reason?: string | null;
  admin_note?: string | null;
  reviewer_uid?: number | null;
  created_at: number;
  reviewed_at?: number | null;
}

export interface TelegramRebindPage {
  requests: TelegramRebindRequest[];
  total: number;
  page: number;
  per_page: number;
}

export interface AdminTelegramRebindPageData {
  payload: TelegramRebindPage | null;
  status: "all" | "pending" | "approved" | "rejected" | "revoked";
  page: number;
  perPage: number;
  loadError: string | null;
  notice: "reviewed" | "batch_reviewed" | "revoked" | "";
}

export interface ConfigToml {
  content: string;
  raw_content?: string;
  path: string;
  completed?: boolean;
}

export interface ConfigBackup {
  name: string;
  path: string;
  size: number;
  created_at: number;
}

export interface ConfigBackupList {
  backups: ConfigBackup[];
  config_file?: string;
  backup_dir?: string;
}

export interface ConfigBackupView {
  backup: ConfigBackup;
  content: string;
  config_file?: string;
}

export interface ConfigRestoreResult {
  operation: string;
  dry_run: boolean;
  requires_confirmation?: boolean;
  confirm?: string;
  restored: string;
  backup: ConfigBackup;
  config_file?: string;
  content_bytes?: number;
  warnings?: string[];
  pre_restore_backup?: ConfigBackup;
  pre_operation_backup?: ConfigBackup;
}

export interface AdminConfigPageData {
  schema: ConfigSchema | null;
  toml: ConfigToml | null;
  backups: ConfigBackupList | null;
  errors: string[];
}

export interface DatabaseBackup extends ConfigBackup {
  note?: string;
}

export interface DatabaseStatus {
  active_driver: string;
  configured_driver: string;
  active_label?: string;
  configured_label?: string;
  supported_drivers?: Array<{ driver: string; label: string; role: string }>;
  state_file?: string;
  backup_dir?: string;
  backup_count: number;
  storage_mismatch?: boolean;
  storage_warning?: string;
  migration_panel_enabled?: boolean;
  postgres_configured: boolean;
  redis_enabled: boolean;
  user_count: number;
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

export interface DatabaseOperationResult {
  operation?: string;
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

export interface AdminDatabasePageData {
  status: DatabaseStatus | null;
  backups: DatabaseBackup[];
  errors: string[];
}

export interface EmbyConnectivityTest {
  name: string;
  success: boolean;
  latency_ms?: number;
  message: string;
}

export interface EmbyConnectivityResult {
  overall: boolean;
  tests: EmbyConnectivityTest[];
  server_info?: {
    name?: string;
    version?: string;
    os?: string;
    id?: string;
  };
}

export interface AdminEmbyLocalUser {
  uid: number;
  username: string;
  telegram_id: number | null;
  active: boolean;
  role: number;
}

export interface AdminEmbyUser {
  emby_id: string;
  emby_name: string;
  has_password: boolean;
  is_admin: boolean;
  is_disabled: boolean;
  is_hidden: boolean;
  last_login: string | null;
  last_activity: string | null;
  local_user: AdminEmbyLocalUser | null;
  sync_status: "synced" | "name_mismatch" | "unlinked";
}

export interface AdminEmbyOrphan {
  uid: number;
  username: string;
  emby_id: string;
  telegram_id: number | null;
}

export interface AdminEmbyUsersResult {
  emby_users: AdminEmbyUser[];
  orphans: AdminEmbyOrphan[];
  total?: number;
  total_emby: number;
  total_linked: number;
  total_orphans: number;
  page?: number;
  per_page?: number;
  pages?: number;
  orphan_page?: number;
  orphan_per_page?: number;
  orphan_pages?: number;
}

export interface AdminEmbyDevice {
  device_id: string;
  device_name: string;
  app_name: string;
  app_version: string;
  last_activity: string;
  ip: string;
  ip_approx: boolean;
  online: boolean;
  count?: number;
}

export interface AdminEmbyAuditLocalUser {
  uid: number;
  username: string;
  email: string | null;
  email_verified: boolean;
  telegram_id: number | null;
  telegram_username: string | null;
  emby_username: string | null;
  role: number;
  active: boolean;
  expired_at: number;
  register_time: number;
  created_at: number;
  pending_emby: boolean;
}

export interface AuditLogEntry {
  id: number;
  uid: number;
  username: string;
  action: string;
  category: "admin" | "user" | "system" | string;
  source?: "http" | "telegram" | "scheduler" | "system" | string;
  method?: string;
  target_uid?: number | null;
  detail?: Record<string, unknown> | null;
  ip?: string;
  created_at: number;
}

export interface AuditLogPage {
  logs: AuditLogEntry[];
  total: number;
  page: number;
  per_page: number;
  sort?: string;
  order?: string;
}

export interface AdminAuditLogsPageData {
  payload: AuditLogPage | null;
  notice?: "deleted" | "cleared" | "pruned" | "";
  query: {
    page: number;
    per_page: number;
    preset: string;
    category: string;
    action: string;
    time: string;
    sort: string;
    order: string;
    uid: number;
    target_uid: number;
    search: string;
  };
  loadError: string | null;
}

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

export interface ViolationLogPage {
  violations: ViolationLog[];
  total: number;
  page: number;
  per_page: number;
}

export interface AdminViolationsPageData {
  payload: ViolationLogPage | null;
  query: {
    page: number;
    per_page: number;
    type: string;
    search: string;
  };
  notice: "deleted" | "cleared" | "";
  loadError: string | null;
}

export interface Regcode {
  code: string;
  type: number;
  type_name: string;
  is_decoy?: boolean;
  days: number;
  validity_time?: number;
  expires_at?: number;
  use_count?: number;
  use_count_limit?: number;
  active?: boolean;
  status?: "available" | "disabled" | "used_up" | "expired" | string;
  note?: string;
  target_username?: string;
  target_telegram_username?: string;
  target_telegram_id?: number;
  target_uid?: number;
  target_resolved_username?: string;
  used_by_uids?: number[];
  used_by_usernames?: string[];
  used_by_telegram_ids?: number[];
  created_time?: number;
  source?: string;
  creator_uid?: number;
  creator_username?: string;
  paused_seconds?: number;
  pause_start?: number;
}

export interface RegcodePage {
  regcodes: Regcode[];
  total: number;
  page: number;
  per_page: number;
}

export interface RegcodeUsageUser {
  uid?: number;
  username?: string;
  active?: boolean;
  emby_id?: string;
  emby_bound?: boolean;
  telegram_id?: number;
  found: boolean;
  source: "uid" | "telegram";
}

export interface RegcodeUsagePage {
  code: string;
  use_count: number;
  users: RegcodeUsageUser[];
  telegram_only?: Array<{ telegram_id: number; found: false; source: "telegram" }>;
}

export interface AdminRegcodesPageData {
  payload: RegcodePage | null;
  usage: RegcodeUsagePage | null;
  query: {
    page: number;
    per_page: number;
    type: string;
    status: string;
    source: string;
    search: string;
    sort: string;
    order: string;
    usage: string;
  };
  notice: "created" | "updated" | "deleted" | "batch_deleted" | "usage_cleared" | "";
  loadError: string | null;
  usageError: string | null;
}

export interface AdminEmbyAuditUser {
  emby_user_id: string;
  emby_user_name: string;
  device_count: number;
  online_count: number;
  ip_count: number;
  ips: string[];
  last_activity: string | null;
  devices: AdminEmbyDevice[];
  local_user: AdminEmbyAuditLocalUser | null;
}

export interface AdminEmbyAuditSummary {
  total_users: number;
  linked_users: number;
  total_devices: number;
  online_devices: number;
  total_ips: number;
  sessions_available?: boolean;
  sessions_error?: string | null;
  devices_available?: boolean;
  devices_error?: string | null;
  activity_available: boolean;
  activity_error?: string | null;
  clients: Array<{ name: string; devices: number; online: number; users: number }>;
}

export interface AdminEmbyDeviceAuditResult {
  emby_configured: boolean;
  users: AdminEmbyAuditUser[];
  summary: AdminEmbyAuditSummary;
  total?: number;
  page?: number;
  per_page?: number;
  pages?: number;
}

export interface AdminEmbyActivityLog {
  id: number;
  emby_log_id: number;
  type: string;
  name: string;
  item_id?: string;
  user_id: string;
  user_name: string;
  overview?: string;
  date: number;
  created_at: number;
}

export interface AdminEmbyActivityResult {
  entries: AdminEmbyActivityLog[];
  total: number;
  refreshed?: boolean;
  new_entries?: number;
}

export interface AdminEmbyPageData {
  tab: "accounts" | "devices" | "activity";
  users: AdminEmbyUsersResult | null;
  deviceAudit: AdminEmbyDeviceAuditResult | null;
  activity: AdminEmbyActivityResult | null;
  query: {
    page: number;
    per_page: number;
    search: string;
    link: string;
    attribute: string;
    device_page: number;
    device_per_page: number;
    device_search: string;
    orphan_page: number;
    orphan_per_page: number;
  };
  errors: string[];
}

export type AnnouncementLevel = "info" | "notice" | "warning" | "critical";

export interface Announcement {
  id: number;
  title: string | null;
  content: string;
  level: AnnouncementLevel;
  render_mode?: "plain" | "markdown" | "bbcode";
  pinned: boolean;
  visible: boolean;
  force_read?: boolean;
  force_read_seconds?: number;
  expires_at?: number;
  expired_at?: number;
  created_at: number;
  updated_at: number;
}

export interface AdminAnnouncementPage {
  announcements: Announcement[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface AdminAnnouncementsPageData {
  payload: AdminAnnouncementPage | null;
  query: {
    page: number;
    per_page: number;
    include_invisible: boolean;
    include_expired: boolean;
  };
  notice?: "created" | "updated" | "deleted" | "hidden" | "shown" | "pinned" | "unpinned" | "";
  loadError: string | null;
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

export interface AdminRuntimeLogsPageData {
  status: RuntimeStatus | null;
  logs: RuntimeLogsResponse | null;
  limit: number;
  loadError: string | null;
}

export interface UserAnnouncements {
  announcements: Announcement[];
  total: number;
  unseen_force_read: Announcement[];
  unseen_force_read_ids: number[];
}
