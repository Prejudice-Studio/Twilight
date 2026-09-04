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
  expires_at: number;
  created_at: number;
  updated_at: number;
}

export interface UserAnnouncements {
  announcements: Announcement[];
  total: number;
  unseen_force_read: Announcement[];
  unseen_force_read_ids: number[];
}
