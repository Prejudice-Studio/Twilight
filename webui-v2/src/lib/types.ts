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
  role: number;
  role_name?: string;
  active: boolean;
  emby_id?: string;
  emby_username?: string;
  telegram_id?: number;
  expired_at?: number;
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

export interface Ticket extends TicketSummary {
  uid: number;
  username: string;
  content: string;
  replies?: TicketReply[];
  attachments?: TicketAttachment[];
}

export interface SystemInfo {
  name?: string;
  version?: string;
  features?: Record<string, boolean>;
  telegram_bot?: {
    username?: string | null;
    url?: string | null;
    enabled?: boolean;
    configured?: boolean;
  };
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
