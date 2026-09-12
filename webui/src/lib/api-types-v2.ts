/**
 * V2 API 类型定义
 *
 * 此文件包含所有 V2 API 的 TypeScript 类型定义。
 * V2 API 与 V1 保持兼容，但提供了更好的缓存策略、更清晰的响应结构。
 */

import type { User, ApiResponse } from "./api-types";

// ==================== 认证模块 ====================

/**
 * V2 登录请求
 */
export interface V2LoginRequest {
  username: string;
  password: string;
  device_id?: string;
  device_name?: string;
  remember?: boolean;
}

/**
 * V2 登录响应
 */
export interface V2LoginResponse {
  token: string;
  user: User;
  expires_at?: number;
}

/**
 * V2 当前用户响应（GET /api/v2/auth/me）
 */
export interface V2CurrentUserResponse {
  user: User;
}

/**
 * V2 刷新会话响应
 */
export interface V2RefreshSessionResponse {
  token: string;
  user: User;
  expires_at?: number;
}

/**
 * V2 注册请求
 */
export interface V2RegisterRequest {
  username: string;
  password: string;
  email?: string;
  regcode: string;
  emby_username?: string;
  device_id?: string;
  device_name?: string;
}

/**
 * V2 注册响应
 */
export interface V2RegisterResponse {
  token: string;
  user: User;
  message?: string;
}

/**
 * V2 注册可用性检查请求
 */
export interface V2RegistrationAvailabilityRequest {
  regcode: string;
}

/**
 * V2 注册可用性检查响应
 */
export interface V2RegistrationAvailabilityResponse {
  available: boolean;
  requires_emby_username?: boolean;
  message?: string;
}

/**
 * V2 忘记密码请求（通过 Emby）
 */
export interface V2ForgotPasswordByEmbyRequest {
  emby_username: string;
}

/**
 * V2 忘记密码响应
 */
export interface V2ForgotPasswordResponse {
  success: boolean;
  message: string;
}

/**
 * V2 邮箱密码重置请求
 */
export interface V2EmailPasswordResetRequest {
  email: string;
}

/**
 * V2 邮箱密码重置响应
 */
export interface V2EmailPasswordResetResponse {
  success: boolean;
  message: string;
  verification_id?: string;
}

/**
 * V2 邮箱密码重置确认请求
 */
export interface V2EmailPasswordResetConfirmRequest {
  verification_id: string;
  code: string;
  new_password: string;
}

/**
 * V2 邮箱密码重置确认响应
 */
export interface V2EmailPasswordResetConfirmResponse {
  success: boolean;
  message: string;
}

/**
 * V2 发送邮箱验证码请求
 */
export interface V2SendEmailCodeRequest {
  purpose: "bind" | "change_password" | "change_emby_password";
  email?: string; // purpose=bind 时必填
}

/**
 * V2 发送邮箱验证码响应
 */
export interface V2SendEmailCodeResponse {
  verification_id: string;
  email: string; // 遮蔽后的邮箱
  expires_in: number;
  resend_after: number;
}

/**
 * V2 验证邮箱验证码请求
 */
export interface V2VerifyEmailCodeRequest {
  verification_id: string;
  code: string;
}

/**
 * V2 验证邮箱验证码响应
 */
export interface V2VerifyEmailCodeResponse {
  success: boolean;
  message: string;
}

/**
 * V2 修改系统密码请求
 */
export interface V2ChangePasswordRequest {
  old_password: string;
  new_password: string;
  verification_id?: string;
  code?: string;
}

/**
 * V2 修改系统密码响应
 */
export interface V2ChangePasswordResponse {
  token: string;
  message?: string;
}

/**
 * V2 修改 Emby 密码请求
 */
export interface V2ChangeEmbyPasswordRequest {
  new_password: string;
  old_password?: string; // 启用旧密码验证时必填
  verification_id?: string;
  code?: string;
}

/**
 * V2 修改 Emby 密码响应
 */
export interface V2ChangeEmbyPasswordResponse {
  success: boolean;
  message?: string;
}

/**
 * V2 API Key 登录请求
 */
export interface V2LoginByAPIKeyRequest {
  apikey: string;
  device_id?: string;
  device_name?: string;
}

/**
 * V2 Telegram 登录请求
 */
export interface V2TelegramLoginRequest {
  telegram_id: number;
  auth_date: number;
  hash: string;
  first_name?: string;
  last_name?: string;
  username?: string;
  photo_url?: string;
}

/**
 * V2 Telegram 登录响应
 */
export interface V2TelegramLoginResponse {
  token: string;
  user: User;
  expires_at?: number;
}

/**
 * V2 创建 Telegram 绑定码请求（注册用）
 */
export interface V2CreateRegistrationBindCodeRequest {
  regcode?: string;
}

/**
 * V2 创建 Telegram 绑定码响应
 */
export interface V2CreateRegistrationBindCodeResponse {
  bind_code: string;
  expires_in: number;
}

// ==================== 用户管理模块 ====================

/**
 * V2 用户列表查询参数
 */
export interface V2UserListParams {
  page?: number;
  per_page?: number;
  cursor?: string;
  search?: string;
  filter_emby?: boolean;
  filter_disabled?: boolean;
  filter_email_status?: "verified" | "unverified" | "no_email";
  sort?: "uid" | "username" | "created_at" | "expire_at";
  order?: "asc" | "desc";
}

/**
 * V2 用户列表响应
 */
export interface V2UserListResponse {
  users: User[];
  pagination: {
    total: number;
    page: number;
    per_page: number;
    next_cursor?: string;
    has_next: boolean;
  };
}

/**
 * V2 用户详情响应
 */
export interface V2UserDetailResponse {
  user: User;
}

/**
 * V2 用户更新请求
 */
export interface V2UserUpdateRequest {
  username?: string;
  email?: string;
  expire_at?: number;
  is_disabled?: boolean;
  is_permanent?: boolean;
  note?: string;
}

/**
 * V2 用户更新响应
 */
export interface V2UserUpdateResponse {
  user: User;
  message?: string;
  emby_sync_failed?: boolean;
}

/**
 * V2 创建用户请求
 */
export interface V2CreateUserRequest {
  username: string;
  password?: string;
  email?: string;
  role?: number;
  expired_at?: number;
  days?: number;
}

/**
 * V2 创建用户响应
 */
export interface V2CreateUserResponse {
  user: User;
  password: string;
  auto_generated: boolean;
}

/**
 * V2 删除用户选项
 */
export interface V2DeleteUserOptions {
  delete_emby?: boolean;
}

/**
 * V2 删除用户响应
 */
export interface V2DeleteUserResponse {
  success: boolean;
  message?: string;
}

/**
 * V2 用户强制解绑请求
 */
export interface V2ForceUnbindRequest {
  scope?: "telegram" | "emby" | "both";
}

/**
 * V2 用户强制解绑响应
 */
export interface V2ForceUnbindResponse {
  changed: string[];
  old: {
    telegram_id?: number | null;
    emby_id?: string | null;
  };
}

/**
 * V2 管理员绑定 Telegram 请求
 */
export interface V2AdminBindTelegramRequest {
  telegram_id: number;
}

/**
 * V2 管理员绑定 Telegram 响应
 */
export interface V2AdminBindTelegramResponse {
  uid: number;
  username: string;
  telegram_id: number;
  old_telegram_id: number | null;
}

// ==================== Telegram 模块 ====================

/**
 * V2 Telegram 命令目录响应
 */
export interface V2TelegramCommandCatalogResponse {
  commands: Array<{
    command: string;
    description: string;
    admin_only: boolean;
    group_only: boolean;
  }>;
  disabled_commands: string[];
}

/**
 * V2 Telegram 换绑请求列表参数
 */
export interface V2TelegramRebindRequestListParams {
  status?: "pending" | "approved" | "rejected";
  page?: number;
  per_page?: number;
}

/**
 * V2 Telegram 换绑请求
 */
export interface V2TelegramRebindRequest {
  id: number;
  uid: number;
  username: string;
  old_telegram_id: number;
  old_telegram_username?: string;
  new_telegram_id: number;
  new_telegram_username?: string;
  status: "pending" | "approved" | "rejected";
  reason?: string;
  admin_note?: string;
  created_at: number;
  updated_at: number;
}

/**
 * V2 Telegram 换绑请求列表响应
 */
export interface V2TelegramRebindRequestListResponse {
  requests: V2TelegramRebindRequest[];
  total: number;
}

// ==================== 工单系统模块 ====================

/**
 * V2 用户工单列表查询参数
 */
export interface V2UserTicketListParams {
  page?: number;
  per_page?: number;
}

/**
 * V2 用户工单列表响应
 */
export interface V2UserTicketListResponse {
  items: Array<{
    id: number;
    uid: number;
    username: string;
    title: string;
    status: string;
    priority: string;
    type: string;
    created_at: number;
    updated_at: number;
    last_reply_at: number;
    unread_admin_replies: number;
  }>;
  pagination: {
    page: number;
    per_page: number;
    total: number;
    total_pages: number;
  };
  ticket_types: string[];
}

/**
 * V2 用户工单详情响应
 */
export interface V2UserTicketDetailResponse {
  item: {
    id: number;
    uid: number;
    username: string;
    title: string;
    content: string;
    status: string;
    priority: string;
    type: string;
    notify_telegram: boolean;
    created_at: number;
    updated_at: number;
    last_reply_at: number;
    unread_admin_replies: number;
    replies: Array<{
      id: number;
      ticket_id: number;
      uid: number;
      username: string;
      content: string;
      is_admin: boolean;
      created_at: number;
    }>;
    attachments: Array<{
      filename: string;
      url: string;
      content_type: string;
      size: number;
      uploaded_uid: number;
      created_at: number;
    }>;
  };
  ticket_types: string[];
}

/**
 * V2 管理员工单列表查询参数
 */
export interface V2AdminTicketListParams {
  uid?: number;
  status?: string;
  type?: string;
  priority?: string;
  all?: boolean;
  page?: number;
  per_page?: number;
}

/**
 * V2 管理员工单列表响应
 */
export interface V2AdminTicketListResponse {
  items: Array<{
    id: number;
    uid: number;
    username: string;
    title: string;
    status: string;
    priority: string;
    type: string;
    admin_note: string;
    created_at: number;
    updated_at: number;
    last_reply_at: number;
    unread_admin_replies: number;
    unread_user_replies: number;
  }>;
  pagination: {
    page: number;
    per_page: number;
    total: number;
    total_pages: number;
  };
  ticket_types: string[];
}

/**
 * V2 管理员工单详情响应
 */
export interface V2AdminTicketDetailResponse {
  item: {
    id: number;
    uid: number;
    username: string;
    title: string;
    content: string;
    status: string;
    priority: string;
    type: string;
    admin_note: string;
    notify_telegram: boolean;
    created_at: number;
    updated_at: number;
    last_reply_at: number;
    unread_admin_replies: number;
    unread_user_replies: number;
    replies: Array<{
      id: number;
      ticket_id: number;
      uid: number;
      username: string;
      content: string;
      is_admin: boolean;
      created_at: number;
    }>;
    attachments: Array<{
      filename: string;
      url: string;
      content_type: string;
      size: number;
      uploaded_uid: number;
      created_at: number;
    }>;
  };
  ticket_types: string[];
}

/**
 * V2 工单回复响应
 */
export interface V2TicketReplyResponse {
  ticket_id: number;
  ticket?: any;
  item?: any;
  replies: Array<{
    id: number;
    ticket_id: number;
    uid: number;
    username: string;
    content: string;
    is_admin: boolean;
    created_at: number;
  }>;
}

// ==================== 公告系统模块 ====================

/**
 * V2 公告列表查询参数
 */
export interface V2AnnouncementListParams {
  page?: number;
  per_page?: number;
  published_only?: boolean;
}

/**
 * V2 公告列表响应
 */
export interface V2AnnouncementListResponse {
  announcements: Array<{
    id: number;
    title: string;
    content: string;
    render_mode: "plain" | "markdown" | "bbcode";
    published: boolean;
    created_at: number;
    updated_at: number;
  }>;
  pagination: {
    total: number;
    page: number;
    per_page: number;
  };
}

/**
 * V2 创建公告请求
 */
export interface V2CreateAnnouncementRequest {
  title: string;
  content: string;
  render_mode?: "plain" | "markdown" | "bbcode";
  published?: boolean;
}

/**
 * V2 创建公告响应
 */
export interface V2CreateAnnouncementResponse {
  announcement_id: number;
  message?: string;
}

// ==================== 通用类型 ====================

/**
 * V2 分页参数（通用）
 */
export interface V2PaginationParams {
  page?: number;
  per_page?: number;
  cursor?: string;
}

/**
 * V2 分页响应（通用）
 */
export interface V2PaginationMeta {
  total: number;
  page: number;
  per_page: number;
  next_cursor?: string;
  has_next: boolean;
}

/**
 * V2 API 响应包装（通用）
 * V2 使用与 V1 相同的 ApiResponse 格式
 */
export type V2ApiResponse<T> = ApiResponse<T>;
