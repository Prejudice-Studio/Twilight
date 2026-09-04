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
