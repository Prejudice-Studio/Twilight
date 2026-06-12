import type {
  AdminUserListParams,
  AdminUserListResponse,
  Announcement,
  AnnouncementRenderMode,
  ApiKeyItem,
  ApiResponse,
  AuditLog,
  BatchUserSelection,
  BatchUserResult,
  CodeUsePreview,
  CodeUseResponse,
  ConfigBackup,
  ConfigBackupView,
  ConfigCategory,
  ConfigField,
  ConfigFieldOption,
  ConfigRestoreResult,
  ConfigSchema,
  ConfigSection,
  CreateRegcodeData,
  EmailAdminData,
  EmailCodeProof,
  EmailCodeSent,
  EmailTestResult,
  DatabaseBackup,
  DatabaseBackupInspectResult,
  DatabaseMigrationResult,
  DatabaseOperationResult,
  DatabaseRestoreResult,
  DatabaseStatus,
  EmbyDevice,
  EmbyDeviceAuditData,
  EmbyInfo,
  EmbyRegisterStatus,
  EmbySession,
  LoginDevice,
  EmbyStatus,
  InventoryCheckRequest,
  InventoryCheckResult,
  InviteCodeItem,
  InviteConfig,
  InviteForest,
  InviteForestEdge,
  InviteForestNode,
  InviteMyStatus,
  InviteTreeNode,
  MediaDetail,
  MediaItem,
  MediaRequest,
  MediaRequestData,
  Regcode,
  RegisterAvailability,
  RegisterData,
  RegisterResponse,
  RuntimeLogEntry,
  RuntimeLogsResponse,
  RuntimeStatus,
  SchedulerJobItem,
  SchedulerJobRun,
  SchedulerSchedulePayload,
  SchedulerTriggerSpec,
  SigninActionResult,
  SigninBonusRule,
  SigninHistoryRecord,
  SigninPublicConfig,
  SigninRenewalResult,
  SigninSummary,
  SystemHealth,
  SystemInfo,
  SystemStats,
  TelegramRebindRequest,
  TelegramStatus,
  User,
  UserInfo,
  UserSettings,
  UserUpdateData,
  ViolationLog,
} from "./api-types";
import { confirmPhrases } from "./confirm-phrases";
import { API_BASE, ApiError, apiRequest, apiRequestForm, type ApiRequestExtraOptions } from "./api-request";
import { normalizeMediaRequestStatus } from "./media-status";

const BIND_CODE_CREATE_HEADERS = {
  "X-Twilight-Client": "webui",
  "X-Twilight-Intent": "create-bind-code",
};

// emailCodeBody 把可选的邮箱验证码凭据展开进改密请求体。强制邮箱验证开启时后端
// 要求 verification_id + email_code；未开启时省略，保持向后兼容。
function emailCodeBody(proof?: EmailCodeProof): Record<string, string> {
  if (!proof?.verification_id || !proof?.code) return {};
  return { verification_id: proof.verification_id, email_code: proof.code };
}

class ApiClient {
  private toAbsoluteAssetUrl(url?: string | null): string | null {
    if (!url) return null;
    const value = url.trim();
    if (!value) return null;
    if (/^(https?:)?\/\//i.test(value) || value.startsWith("blob:")) {
      return value;
    }
    if (/^data:image\/(png|jpe?g|gif|webp|avif|bmp)(;|,)/i.test(value)) {
      return value;
    }
    if (/^[a-z][a-z0-9+.-]*:/i.test(value)) {
      return null;
    }
    if (value.startsWith("/")) {
      return `${API_BASE}${value}`;
    }
    return `${API_BASE}/${value}`;
  }

  private normalizeCssUrlValue(value?: string | null): string {
    if (!value) return "";
    return value.replace(/url\((['"]?)(.*?)\1\)/g, (_match, quote, rawUrl: string) => {
      const normalized = this.toAbsoluteAssetUrl(rawUrl.trim());
      if (!normalized) return "none";
      const q = quote || '"';
      return `url(${q}${normalized}${q})`;
    });
  }

  private normalizeBackgroundAssetCssUrlValue(value?: string | null): string {
    if (!value) return "";
    return value.replace(/url\((['"]?)(.*?)\1\)/g, (_match, quote, rawUrl: string) => {
      const raw = rawUrl.trim();
      let path = raw;
      if (/^https?:\/\//i.test(raw)) {
        try {
          const parsed = new URL(raw);
          const allowedOrigin = API_BASE
            ? new URL(API_BASE, typeof window === "undefined" ? "http://localhost" : window.location.origin).origin
            : (typeof window === "undefined" ? "" : window.location.origin);
          if (!allowedOrigin) return "none";
          if (parsed.origin !== allowedOrigin) return "none";
          path = parsed.pathname;
        } catch {
          return "none";
        }
      }
      if (!/^\/api\/v1\/users\/assets\/background\/[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$/i.test(path)) return "none";
      const normalized = this.toAbsoluteAssetUrl(path);
      if (!normalized) return "none";
      const q = quote || '"';
      return `url(${q}${normalized}${q})`;
    });
  }

  setToken(token: string | null) {
    void token;
  }

  getToken(): string | null {
    return null;
  }

  hasToken(): boolean {
    return true;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
    extra: ApiRequestExtraOptions = {},
  ): Promise<ApiResponse<T>> {
    return apiRequest<T>(endpoint, options, extra);
  }

  private async requestForm<T>(
    endpoint: string,
    formData: FormData,
    method: "POST" | "PUT" = "POST"
  ): Promise<ApiResponse<T>> {
    return apiRequestForm<T>(endpoint, formData, method);
  }

  // Auth
  async login(username: string, password: string) {
    const res = await this.request<{ user: Partial<UserInfo> }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    if (res.success && res.data?.user?.avatar) {
      res.data.user.avatar = this.toAbsoluteAssetUrl(res.data.user.avatar) || undefined;
    }
    return res;
  }

  async register(data: RegisterData) {
    return this.request<RegisterResponse>("/users/register", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async logout() {
    try {
      await this.request("/auth/logout", { method: "POST" });
    } catch {
      // 忽略网络异常，前端仍会清理本地状态
    }
  }

  // System
  async getSystemInfo() {
    const res = await this.request<SystemInfo>("/system/info");
    if (res.success && res.data?.icon) {
      res.data.icon = this.toAbsoluteAssetUrl(res.data.icon) || "";
    }
    return res;
  }

  private toApiRelativeAssetValue(value?: string | null): string {
    if (!value || !API_BASE) return value || "";
    return value.replace(API_BASE, "");
  }

  async getSystemHealth() {
    return this.request<SystemHealth>("/system/health");
  }

  // User
  async getMe() {
    const res = await this.request<UserInfo>("/users/me");
    if (res.success && res.data?.avatar) {
      res.data.avatar = this.toAbsoluteAssetUrl(res.data.avatar) || undefined;
    }
    return res;
  }

  async updateMe(data: { email?: string; username?: string; bgm_mode?: boolean; bgm_token?: string }) {
    return this.request<UserInfo>("/users/me", {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async getMySettings() {
    return this.request<UserSettings>("/users/me/settings");
  }

  async updateMySettings(data: { bgm_mode?: boolean; bgm_token?: string; email?: string }) {
    return this.request<UserInfo>("/users/me", {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async getTelegramStatus() {
    return this.request<TelegramStatus>("/users/me/telegram");
  }

  async getBindCode() {
    return this.request<{ bind_code: string; expires_in: number }>("/users/me/telegram/bind-code", {
      headers: BIND_CODE_CREATE_HEADERS,
    });
  }

  async getRegisterBindCode() {
    return this.request<{ bind_code: string; expires_in: number }>("/users/telegram/register/bind-code", {
      headers: BIND_CODE_CREATE_HEADERS,
    });
  }

  async getRegisterBindCodeStatus(code: string, signal?: AbortSignal) {
    const q = new URLSearchParams({ code }).toString();
    // 后端约定：code 在 DB 不存在 / 已过期 / 已确认 都是 *终态*，
    // 通过 data.terminal === true 表示；其中 invalid 区分"不存在/过期"和"已确认"。
    // 这一端点不会再为业务无效抛 HTTP 404——上面的字段是唯一可信信号。
    return apiRequest<{
      code?: string;
      status?: string;
      error_code?: string;
      message?: string;
      confirmed?: boolean;
      expires_in?: number;
      invalid?: boolean;
      terminal?: boolean;
      telegram_bound?: boolean;
      telegram_id?: number;
      telegram_username?: string;
    }>(
      `/users/telegram/register/bind-code/status?${q}`,
      { signal },
      { timeoutMs: 10_000 },
    );
  }

  getRegisterBindCodeStatusWebSocketUrl(code: string) {
    const base = API_BASE || (typeof window !== "undefined" ? window.location.origin : "http://localhost");
    const url = new URL("/api/v1/users/telegram/register/bind-code/ws", base);
    url.searchParams.set("code", code);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return url.toString();
  }

  async getBindCodeStatus(code: string, signal?: AbortSignal) {
    const q = new URLSearchParams({ code }).toString();
    return apiRequest<{
      code?: string;
      status?: string;
      error_code?: string;
      message?: string;
      confirmed?: boolean;
      expires_in?: number;
      invalid?: boolean;
      terminal?: boolean;
      telegram_bound?: boolean;
      telegram_id?: number;
      telegram_username?: string;
    }>(
      `/users/me/telegram/bind-code/status?${q}`,
      { signal },
      { timeoutMs: 10_000 },
    );
  }

  getBindCodeStatusWebSocketUrl(code: string) {
    const base = API_BASE || (typeof window !== "undefined" ? window.location.origin : "http://localhost");
    const url = new URL("/api/v1/users/me/telegram/bind-code/ws", base);
    url.searchParams.set("code", code);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return url.toString();
  }

  async getRegisterAvailability() {
    return this.request<RegisterAvailability>("/users/check-available");
  }

  async getEmbyRegisterStatus(requestId: string, statusToken: string) {
    const query = new URLSearchParams({
      request_id: requestId,
      status_token: statusToken,
    });
    return this.request<EmbyRegisterStatus>(`/users/register/emby/status?${query.toString()}`);
  }

  async requestTelegramRebind(reason?: string) {
    return this.request("/users/me/telegram/rebind-request", {
      method: "POST",
      body: JSON.stringify({ reason }),
    });
  }

  async unbindTelegram() {
    return this.request("/users/me/telegram/unbind", { method: "POST" });
  }

  async completeRebind() {
    return this.request("/users/me/telegram/rebind-complete", { method: "POST" });
  }

  async bindEmbyAccount(embyUsername: string, embyPassword: string) {
    return this.request<{ emby_id: string; emby_username: string }>("/users/me/emby/bind", {
      method: "POST",
      body: JSON.stringify({
        emby_username: embyUsername,
        emby_password: embyPassword,
      }),
    });
  }

  async completeEmbyRegistration(embyUsername: string, embyPassword: string) {
    // 自由注册的开通天数由管理员在配置里固定（[SAR].emby_direct_register_days），
    // 客户端不再上传 days；老调用方传值也由后端静默丢弃。
    return this.request<{ user?: UserInfo; pending?: boolean; request_id?: string; status_token?: string; status?: string; queue_position?: number }>("/users/me/emby/register", {
      method: "POST",
      body: JSON.stringify({
        emby_username: embyUsername,
        emby_password: embyPassword,
      }),
    });
  }

  async unbindEmbyAccount() {
    return this.request<UserInfo & { remote_emby_disabled?: boolean; old_emby_id?: string }>("/users/me/emby/unbind", {
      method: "POST",
    });
  }

  async changePassword(oldPassword: string, newPassword: string, emailCode?: EmailCodeProof) {
    return this.request("/users/me/password/change", {
      method: "POST",
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword, ...emailCodeBody(emailCode) }),
    });
  }

  async changeSystemPassword(oldPassword: string, newPassword: string, emailCode?: EmailCodeProof) {
    return this.request("/users/me/password/system", {
      method: "POST",
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword, ...emailCodeBody(emailCode) }),
    });
  }

  async changeEmbyPassword(newPassword: string, emailCode?: EmailCodeProof) {
    return this.request("/users/me/password/emby", {
      method: "POST",
      body: JSON.stringify({ new_password: newPassword, ...emailCodeBody(emailCode) }),
    });
  }

  // === 邮箱验证 ===
  // 登录态：绑定/换绑邮箱 或 改密前获取验证码（purpose 决定收件邮箱）。
  async sendEmailCode(data: { purpose: "bind" | "change_password" | "change_emby_password"; email?: string }) {
    return this.request<EmailCodeSent>("/users/me/email/send-code", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // 登录态：校验 bind 验证码并完成邮箱绑定（返回更新后的用户）。
  async verifyEmailCode(data: { verification_id: string; code: string }) {
    return this.request<UserInfo>("/users/me/email/verify", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // 登出态找回：第一步请求验证码（防枚举，统一成功）。
  async requestPasswordResetEmail(email: string) {
    return this.request<{ resend_after: number; expires_in: number }>("/auth/password/email/request", {
      method: "POST",
      body: JSON.stringify({ email }),
    });
  }

  // 登出态找回：第二步校验验证码并重置系统密码。
  async resetPasswordByEmail(data: { email: string; code: string; new_password: string }) {
    return this.request<{ username: string }>("/auth/password/email/reset", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // 管理员：强制把用户绑定到指定邮箱（默认直接标记已验证）。
  async adminBindUserEmail(uid: number, data: { email: string; mark_verified?: boolean; force?: boolean }) {
    return this.request<UserInfo>(`/admin/users/${uid}/bind-email`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // 管理员：手动置/撤销某用户的邮箱验证状态。
  async adminSetUserEmailVerified(uid: number, data: { verified: boolean; force?: boolean }) {
    return this.request<UserInfo>(`/admin/users/${uid}/email/verified`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // 管理员：邮箱管理审查——在用验证码记录（脱敏）+ 已绑邮箱账号 + 统计。
  async adminGetEmailVerifications() {
    return this.request<EmailAdminData>("/admin/email/verifications");
  }

  // 管理员：撤销一条在用验证码记录（让该验证码立即失效）。
  async adminRevokeEmailVerification(id: string) {
    return this.request<{ id: string }>(`/admin/email/verifications/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
  }

  // 管理员：手动清理所有已过期的在用验证码。
  async adminCleanupEmailVerifications() {
    return this.request<{ deleted: number }>("/admin/email/verifications/cleanup", {
      method: "POST",
    });
  }

  // 管理员：发送测试邮件验证 SMTP 配置。
  async adminTestEmail(to?: string) {
    return this.request<{ results: EmailTestResult[] }>("/admin/email/test", {
      method: "POST",
      body: JSON.stringify({ to: to || "" }),
    });
  }

  async getEmbyUrls() {
    return this.request<{
      lines: Array<{ name: string; url: string }>;
      whitelist_lines?: Array<{ name: string; url: string }>;
      requires_emby_account?: boolean;
      requires_renewal?: boolean;
      emby_disabled_by_expiry?: boolean;
    }>(`/system/emby-urls`);
  }

  async probeEmbyUrl(url: string) {
    return this.request<{
      status: "ok" | "timeout" | "error";
      latency_ms: number;
      http_status?: number;
    }>(`/system/emby-urls/probe`, {
      method: "POST",
      body: JSON.stringify({ url }),
    });
  }

  async checkRegcode(regCode: string) {
    const query = new URLSearchParams({ reg_code: regCode });
    return this.request<{ type: number; type_name: string; days: number; valid: boolean }>(`/users/regcode/check?${query.toString()}`);
  }

  async renewWithRegcode(regCode: string) {
    return this.request<{ expire_status: string; expired_at: string | number }>("/users/me/renew", {
      method: "POST",
      body: JSON.stringify({ reg_code: regCode }),
    });
  }

  // Signin (签到 / 积分)
  async getSigninSummary() {
    return this.request<SigninSummary>("/signin/me");
  }

  async getSigninPublicConfig() {
    return this.request<SigninPublicConfig>("/signin/config");
  }

  async signinNow() {
    return this.request<SigninActionResult>("/signin", { method: "POST" });
  }

  async renewWithSigninCurrency() {
    const res = await this.request<SigninRenewalResult>("/signin/renew", { method: "POST" });
    if (res.success && res.data?.user?.avatar) {
      res.data.user.avatar = this.toAbsoluteAssetUrl(res.data.user.avatar) || undefined;
    }
    return res;
  }

  async getSigninHistory(limit = 30) {
    return this.request<{ records: SigninHistoryRecord[]; currency_name: string }>(
      `/signin/history?limit=${limit}`,
    );
  }

  async useCode(regCode: string, options?: { embyUsername?: string; embyPassword?: string; checkOnly?: boolean }) {
    const payload: Record<string, string | boolean> = { reg_code: regCode };
    if (options?.checkOnly) {
      payload.check_only = true;
    }
    if (options?.embyUsername) {
      payload.emby_username = options.embyUsername;
    }
    if (typeof options?.embyPassword === "string") {
      payload.emby_password = options.embyPassword;
    }

    return this.request<CodeUseResponse>("/users/me/use-code", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async getUseCodeStatus(requestId: string, statusToken: string) {
    const query = new URLSearchParams({ request_id: requestId, status_token: statusToken });
    return this.request<{
      request_id: string;
      uid: number;
      status: "queued" | "processing" | "success" | "failed";
      message?: string;
      queue_position?: number | null;
      created_at?: number;
      updated_at?: number;
      finished_at?: number;
      data?: {
        emby_password?: string;
        expire_status: string;
        expired_at: string | number;
        role: number;
        role_name: string;
      };
    }>(`/users/me/use-code/status?${query.toString()}`);
  }

  // Media
  async searchMedia(query: string, source = "all", signal?: AbortSignal) {
    return this.request<{ results: MediaItem[]; total?: number; warnings?: Record<string, string> }>(
      `/media/search?q=${encodeURIComponent(query)}&source=${source}`,
      { signal }
    );
  }

  async getMediaDetail(source: string, mediaId: number, mediaType: string, signal?: AbortSignal) {
    return this.request<MediaDetail>(
      `/media/detail?source=${source}&media_id=${mediaId}&media_type=${mediaType}`,
      { signal }
    );
  }

  async getMediaByTmdbId(tmdbId: number, type: "movie" | "tv" = "movie", includeDetails = true, signal?: AbortSignal) {
    return this.request<MediaDetail>(
      `/media/tmdb/${tmdbId}?type=${type}&include_details=${includeDetails}`,
      { signal }
    );
  }

  async getMediaByBangumiId(bgmId: number, includeDetails = true, signal?: AbortSignal) {
    return this.request<MediaDetail>(
      `/media/bangumi/${bgmId}?include_details=${includeDetails}`,
      { signal }
    );
  }

  async getMediaById(source: "tmdb" | "bangumi" | "bgm", mediaId: number, type: "movie" | "tv" = "movie", includeDetails = true) {
    return this.request<MediaDetail>(
      `/media/search/id/${source}/${mediaId}?type=${type}&include_details=${includeDetails}`
    );
  }

  async checkInventory(data: InventoryCheckRequest, signal?: AbortSignal) {
    return this.request<InventoryCheckResult>("/media/inventory/check", {
      method: "POST",
      body: JSON.stringify(data),
      signal,
    });
  }

  async createMediaRequest(data: MediaRequestData) {
    return this.request("/media/request", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async getMyRequests(signal?: AbortSignal) {
    const res = await this.request<MediaRequest[]>(
      "/media/request/my",
      { signal }
    );
    if (res.success && Array.isArray(res.data)) {
      res.data = res.data.map((item) => ({
        ...item,
        status: normalizeMediaRequestStatus(item.status, "user"),
      }));
    }
    return res;
  }

  // Emby
  async getEmbyInfo() {
    return this.request<EmbyInfo>("/emby/status");
  }

  async getMySessions() {
    return this.request<EmbySession[]>("/users/me/sessions");
  }

  async getMyDevices() {
    return this.request<LoginDevice[]>("/users/me/devices");
  }

  async removeDevice(deviceId: string) {
    return this.request(`/users/me/devices/${encodeURIComponent(deviceId)}`, {
      method: "DELETE",
    });
  }


  // Admin
  async getUsers(params: AdminUserListParams = {}, signal?: AbortSignal) {
    const query = new URLSearchParams();
    if (params.page) query.set("page", String(params.page));
    if (params.per_page) query.set("per_page", String(params.per_page));
    if (params.role !== undefined && params.role !== null) query.set("role", String(params.role));
    if (params.active !== undefined && params.active !== null) query.set("active", String(params.active));
    if (params.emby) query.set("emby", params.emby);
    if (params.emby_status) query.set("emby_status", params.emby_status);
    if (params.email_status) query.set("email_status", params.email_status);
    if (params.search) query.set("search", params.search);
    if (params.sort) query.set("sort", params.sort);
    return this.request<AdminUserListResponse>(`/admin/users?${query}`, { signal });
  }

  async getUser(uid: number) {
    return this.request<UserInfo>(`/admin/users/${uid}`);
  }

  async updateUser(uid: number, data: Partial<UserUpdateData>) {
    // 改成禁用时 Emby 远端关停失败会附带 emby_sync_failed=true（其余字段为 publicUser）。
    return this.request<UserInfo & { emby_sync_failed?: boolean }>(`/admin/users/${uid}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async adminCreateUser(payload: { username: string; password?: string; email?: string; role?: number; expired_at?: number; days?: number }) {
    return this.request<{ user: UserInfo; password: string; auto_generated: boolean }>("/admin/users", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async deleteUser(uid: number, options?: { deleteEmby?: boolean }) {
    const deleteEmby = options?.deleteEmby ?? true;
    return this.request(`/admin/users/${uid}?delete_emby=${deleteEmby}`, {
      method: "DELETE",
    });
  }

  async deleteUserEmby(uid: number) {
    return this.request(`/admin/users/${uid}/emby`, {
      method: "DELETE",
    });
  }

  async forceUnbindUser(uid: number, scope: "telegram" | "emby" | "both" = "both") {
    return this.request<{ changed: string[]; old: { telegram_id?: number | null; emby_id?: string | null } }>(
      `/admin/users/${uid}/force-unbind`,
      { method: "POST", body: JSON.stringify({ scope }) },
    );
  }

  async adminBindTelegramToUser(uid: number, telegramId: number) {
    return this.request<{ uid: number; username: string; telegram_id: number; old_telegram_id: number | null }>(
      `/admin/users/${uid}/bind-telegram`,
      { method: "POST", body: JSON.stringify({ telegram_id: telegramId }) },
    );
  }

  async clearUserRegistrationQueue(uid: number) {
    return this.request<{
      uid: number;
      username: string;
      cleared: boolean;
      emby_register_queue: Record<string, unknown>;
      regcode_use_queue: Record<string, unknown>;
    }>(`/admin/users/${uid}/registration-queue/clear`, { method: "POST" });
  }

  async grantUserRegistrationEntitlement(uid: number, days?: number) {
    return this.request<{
      uid: number;
      username: string;
      pending_emby: boolean;
      pending_emby_days: number;
      queue_cleared: boolean;
      emby_register_queue: Record<string, unknown>;
      regcode_use_queue: Record<string, unknown>;
    }>(`/admin/users/${uid}/registration-entitlement`, {
      method: "POST",
      body: JSON.stringify({ days }),
    });
  }

  async grantUserRegistrationEntitlementAndDequeue(uid: number, days?: number) {
    return this.request<{
      uid: number;
      username: string;
      pending_emby: boolean;
      pending_emby_days: number;
      dequeued: boolean;
      processing_blocked: string[];
      emby_register_queue: Record<string, unknown>;
      regcode_use_queue: Record<string, unknown>;
    }>(`/admin/users/${uid}/registration-entitlement/dequeue`, {
      method: "POST",
      body: JSON.stringify({ days }),
    });
  }

  async previewRegistrationQueueUsers() {
    return this.request<{ dry_run: true; count: number; uids: number[] }>(
      "/admin/users/registration-queue/clear",
      { method: "POST", body: JSON.stringify({ dry_run: true }) },
    );
  }

  async clearRegistrationQueueUsers() {
    return this.request<{
      dry_run: false;
      matched: number;
      cleared: number;
      blocked: number;
      results: Array<{ uid: number; cleared: boolean; blocked: boolean }>;
    }>("/admin/users/registration-queue/clear", {
      method: "POST",
      body: JSON.stringify({ dry_run: false, confirm: "CLEAR_REGISTRATION_QUEUE" }),
    });
  }

  async previewGrantRegistrationQueueUsersEntitlement(days?: number) {
    return this.request<{
      dry_run: true;
      days: number;
      matched: number;
      eligible: number;
      skipped: Array<{ uid: number; username?: string; reason: string }>;
      users: Array<{ uid: number; username: string }>;
    }>("/admin/users/registration-queue/grant-entitlement-and-clear", {
      method: "POST",
      body: JSON.stringify({ dry_run: true, days }),
    });
  }

  async grantRegistrationQueueUsersEntitlementAndClear(days?: number) {
    return this.request<{
      dry_run: false;
      days: number;
      matched: number;
      eligible: number;
      granted: number;
      dequeued: number;
      blocked: number;
      skipped: Array<{ uid: number; username?: string; reason: string }>;
      failed: Array<{ uid: number; username?: string; reason: string }>;
    }>("/admin/users/registration-queue/grant-entitlement-and-clear", {
      method: "POST",
      body: JSON.stringify({ dry_run: false, days, confirm: "GRANT_AND_CLEAR_REGISTRATION_QUEUE" }),
    });
  }

  async syncUserBindings(payload: {
    scope?: "telegram" | "emby" | "both";
    uid?: number;
    filter?: { role?: number; active?: boolean; emby?: "bound" | "unbound"; email_status?: "verified" | "unverified" | "bound" | "none"; search?: string };
    repair?: boolean;
  }) {
    return this.request<{
      matched: number;
      telegram_checked: number;
      telegram_repaired: number;
      emby_checked: number;
      emby_repaired: number;
      synced: number;
      failed: Array<{ uid: number; scope: string; reason: string }>;
      details: Array<Record<string, unknown>>;
    }>(`/admin/users/sync-bindings`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  // 强制从 Telegram / Emby 拉取单个用户的当前状态并回写：刷新 telegram_username、
  // emby_username，并把「Web 已禁用/过期但 Emby 仍启用」的越权漂移关停。返回刷新后的
  // 用户对象（UserInfo 同口径）+ refresh 同步差异摘要。
  // scope: telegram 仅刷新 TG 用户名、emby 仅核对 Emby、both 两者都刷（默认）。
  async refreshUserStatus(uid: number, scope: "telegram" | "emby" | "both" = "both") {
    return this.request<
      UserInfo & {
        refresh?: {
          telegram_checked?: boolean;
          telegram_username?: string;
          telegram_username_updated?: boolean;
          telegram_error?: string;
          emby_checked?: boolean;
          emby_username?: string;
          emby_username_updated?: boolean;
          emby_missing?: boolean;
          emby_remote_disabled?: boolean;
          emby_disabled_synced?: boolean;
          emby_error?: string;
        };
      }
    >(`/admin/users/${uid}/refresh-status`, { method: "POST", body: JSON.stringify({ scope }) });
  }

  async adminCreateStandaloneEmby(payload: { username: string; password: string; email?: string }) {
    return this.request<{ emby_id: string; emby_username: string }>("/admin/emby/create-standalone", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async adminBindEmbyToUser(uid: number, payload: { emby_username?: string; emby_id?: string; force?: boolean }) {
    return this.request<{
      uid: number;
      emby_id: string;
      emby_username: string;
      force_taken: boolean;
      previous_uid: number | null;
      conflict?: boolean;
      conflict_uid?: number;
      conflict_username?: string;
    }>(`/admin/users/${uid}/bind-emby`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async renewUser(uid: number, days: number) {
    return this.request(`/admin/users/${uid}/renew`, {
      method: "POST",
      body: JSON.stringify({ days }),
    });
  }

  // 单独启停用户的 Emby 账号，不改动 Web 账号状态（与「禁用 Web 顺带关停 Emby」相对）。
  async setUserEmbyEnabled(uid: number, enable: boolean) {
    return this.request<{ uid: number; emby_enabled: boolean }>(
      `/admin/users/${uid}/emby/${enable ? "enable" : "disable"}`,
      { method: "POST" },
    );
  }

  // 批量单独启停 Emby 账号（保留 Web）。
  async batchToggleEmby(selection: number[] | BatchUserSelection, enable: boolean) {
    return this.request<BatchUserResult & { skipped_no_emby?: number; emby_enabled?: boolean }>(
      `/batch/users/emby/${enable ? "enable" : "disable"}`,
      {
        method: "POST",
        body: JSON.stringify({
          ...this.batchUserSelectionBody(selection),
          confirm: enable ? confirmPhrases.batchEmbyEnable : confirmPhrases.batchEmbyDisable,
        }),
      },
      { timeoutMs: 600_000 },
    );
  }

  // 批量强制刷新外部状态。scope 控制只刷 TG / 只刷 Emby / 两者。非破坏性、无需确认短语。
  async batchRefreshStatus(selection: number[] | BatchUserSelection, scope: "telegram" | "emby" | "both" = "both") {
    return this.request<BatchUserResult & { telegram_updated?: number; emby_disabled?: number }>(
      `/batch/users/refresh-status`,
      {
        method: "POST",
        body: JSON.stringify({ ...this.batchUserSelectionBody(selection), scope }),
      },
      { timeoutMs: 600_000 },
    );
  }

  private batchUserSelectionBody(selection: number[] | BatchUserSelection) {
    return Array.isArray(selection) ? { uids: selection } : selection;
  }

  async batchToggleUsers(selection: number[] | BatchUserSelection, enable: boolean) {
    return this.request<BatchUserResult>(`/batch/users/${enable ? "enable" : "disable"}`, {
      method: "POST",
      body: JSON.stringify({
        ...this.batchUserSelectionBody(selection),
        confirm: enable ? confirmPhrases.batchEnableUsers : confirmPhrases.batchDisableUsers,
      }),
    });
  }

  async batchDeleteUsers(selection: number[] | BatchUserSelection, deleteEmby: boolean) {
    return this.request<BatchUserResult>("/batch/users/delete", {
      method: "POST",
      body: JSON.stringify({
        ...this.batchUserSelectionBody(selection),
        delete_emby: deleteEmby,
        confirm: confirmPhrases.batchDeleteUsers,
      }),
    });
  }

  async batchLockEmbyUnbind(selection: number[] | BatchUserSelection) {
    return this.request<BatchUserResult>("/batch/users/emby-unbind-lock", {
      method: "POST",
      body: JSON.stringify({
        ...this.batchUserSelectionBody(selection),
        confirm: confirmPhrases.batchLockEmbyUnbind,
      }),
    }, {
      timeoutMs: 600_000,
    });
  }

  async batchClearEmbyGrant(selection: number[] | BatchUserSelection) {
    return this.request<BatchUserResult>("/batch/users/emby-grant-clear", {
      method: "POST",
      body: JSON.stringify({
        ...this.batchUserSelectionBody(selection),
        confirm: confirmPhrases.batchClearEmbyGrant,
      }),
    }, {
      timeoutMs: 600_000,
    });
  }

  async cancelUserPermanent(uid: number, days: number) {
    return this.request<{ uid: number; days: number; expired_at: number; role: number; role_name: string; downgraded_whitelist: boolean }>(`/admin/users/${uid}/cancel-permanent`, {
      method: "POST",
      body: JSON.stringify({ days }),
    });
  }

  /**
   * 绝对式设置用户到期时间（覆盖现有，不叠加续期）。
   * - permanent=true：设为永久；
   * - 否则 days 必须为正整数，表示从现在起 N 天后到期。
   * 后端路由 /admin/users/:uid/set-expiry。
   */
  async adminSetUserExpiry(uid: number, opts: { days?: number; permanent?: boolean }) {
    const body: Record<string, unknown> = {};
    if (opts.permanent) body.permanent = true;
    else body.days = opts.days;
    return this.request(`/admin/users/${uid}/set-expiry`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async resetPassword(
    uid: number,
    opts?: { scope?: "system" | "emby" | "both"; password?: string },
  ) {
    const body: Record<string, unknown> = {
      scope: opts?.scope || "both",
    };
    if (opts?.password) body.password = opts.password;
    return this.request<{
      scope: "system" | "emby" | "both";
      new_password: string;
      auto_generated: boolean;
    }>(`/admin/users/${uid}/reset-password`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  /**
   * 管理员凭 Emby 用户名强制重置 Emby 密码（即使没有绑定本地账号）。
   * @param embyUsername 目标 Emby 用户名
   * @param newPassword 可选；省略时后端生成 12 位强密码并在响应里返回
   */
  async adminForceSetEmbyPassword(embyUsername: string, newPassword?: string) {
    return this.request<{
      emby_id: string;
      emby_username: string;
      linked_local_user: boolean;
      new_password: string;
    }>(`/admin/emby/force-set-password`, {
      method: "POST",
      body: JSON.stringify({
        emby_username: embyUsername,
        new_password: newPassword || undefined,
      }),
    });
  }

  async getTelegramRebindRequests(params: { page?: number; per_page?: number; status?: string } = {}, signal?: AbortSignal) {
    const query = new URLSearchParams();
    if (params.page) query.set('page', String(params.page));
    if (params.per_page) query.set('per_page', String(params.per_page));
    if (params.status) query.set('status', params.status);
    return this.request<{ requests: TelegramRebindRequest[]; total: number }>(`/admin/telegram/rebind-requests?${query}`, { signal });
  }

  async approveTelegramRebindRequest(id: number, admin_note?: string) {
    return this.request(`/admin/telegram/rebind-requests/${id}/approve`, {
      method: "POST",
      body: JSON.stringify({ admin_note }),
    });
  }

  async rejectTelegramRebindRequest(id: number, admin_note?: string) {
    return this.request(`/admin/telegram/rebind-requests/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ admin_note }),
    });
  }

  async batchReviewTelegramRebindRequests(ids: number[], action: "approve" | "reject", admin_note?: string) {
    return this.request<{
      action: "approve" | "reject";
      total: number;
      success: number;
      failed: number;
      results: Array<{ id: number; success: boolean; message: string }>;
    }>(`/admin/telegram/rebind-requests/batch`, {
      method: "POST",
      body: JSON.stringify({ ids, action, admin_note }),
    });
  }

  // 一键撤销所有"已批准"的换绑许可：所有人立即变为不可换绑（但仍可重新申请）。
  async revokeAllTelegramRebindApprovals(admin_note?: string) {
    return this.request<{ revoked: number }>(`/admin/telegram/rebind-requests/revoke-approved`, {
      method: "POST",
      body: JSON.stringify({ admin_note }),
    });
  }

  async getSystemStats() {
    return this.request<SystemStats>("/system/admin/stats");
  }

  async getRuntimeStatus() {
    return this.request<RuntimeStatus>("/system/admin/runtime/status");
  }

  async getRuntimeLogs(limit = 200, after?: number) {
    const query = new URLSearchParams({ limit: String(limit) });
    if (after && after > 0) query.set("after", String(after));
    return this.request<RuntimeLogsResponse>(`/system/admin/runtime/logs?${query}`);
  }

  runtimeLogStreamURL(limit = 100, after?: number) {
    const query = new URLSearchParams({ limit: String(limit) });
    if (after && after > 0) query.set("after", String(after));
    return `${API_BASE}/api/v1/system/admin/runtime/logs/stream?${query}`;
  }

  async getConfigToml() {
    return this.request<{ content: string; path: string; raw_content?: string; completed?: boolean }>("/system/admin/config/toml");
  }

  async updateConfigToml(content: string) {
    return this.request<{ path: string }>("/system/admin/config/toml", {
      method: "PUT",
      body: JSON.stringify({ content }),
    });
  }

  async listConfigBackups() {
    return this.request<{ backups: ConfigBackup[]; config_file: string; backup_dir: string }>("/system/admin/config/backups");
  }

  async createConfigBackup() {
    return this.request<{ backup: ConfigBackup }>("/system/admin/config/backup", {
      method: "POST",
      body: JSON.stringify({}),
    });
  }

  async getConfigBackup(name: string) {
    return this.request<ConfigBackupView>(`/system/admin/config/backups/${encodeURIComponent(name)}`);
  }

  async restoreConfigBackup(name: string, options?: { dry_run?: boolean; preview?: boolean; confirm?: string }) {
    return this.request<ConfigRestoreResult>("/system/admin/config/restore", {
      method: "POST",
      body: JSON.stringify({ name, ...(options || {}) }),
    });
  }

  async deleteConfigBackup(name: string) {
    return this.request<{ backup: ConfigBackup }>(`/system/admin/config/backups/${encodeURIComponent(name)}`, {
      method: "DELETE",
    });
  }

  async getConfigSchema() {
    return this.request<ConfigSchema>("/system/admin/config/schema");
  }

  async updateConfigBySchema(sections: Record<string, Record<string, unknown>>) {
    return this.request("/system/admin/config/schema", {
      method: "PUT",
      body: JSON.stringify({ sections }),
    });
  }

  async getDatabaseStatus() {
    return this.request<DatabaseStatus>("/system/admin/database/status");
  }

  async listDatabaseBackups() {
    return this.request<{ backups: DatabaseBackup[] }>("/system/admin/database/backups");
  }

  async inspectDatabaseBackup(name: string) {
    return this.request<DatabaseBackupInspectResult>(`/system/admin/database/backups/${encodeURIComponent(name)}`);
  }

  async deleteDatabaseBackup(name: string) {
    return this.request<{ backup: DatabaseBackup }>(`/system/admin/database/backups/${encodeURIComponent(name)}`, {
      method: "DELETE",
    });
  }

  async createDatabaseBackup(note?: string) {
    return this.request<{ backup: DatabaseBackup }>("/system/admin/database/backup", {
      method: "POST",
      body: JSON.stringify({ note: note?.trim() || undefined }),
    });
  }

  async restoreDatabaseBackup(
    name: string,
    options?: { dry_run?: boolean; preview?: boolean; confirm?: string }
  ) {
    return this.request<DatabaseRestoreResult>("/system/admin/database/restore", {
      method: "POST",
      body: JSON.stringify({ name, ...(options || {}) }),
    });
  }

  async previewDatabaseRestore(name: string) {
    return this.restoreDatabaseBackup(name, { dry_run: true });
  }

  async migrateDatabase(payload: {
    source_driver?: "json" | "postgres";
    target_driver: "json" | "postgres";
    dry_run?: boolean;
    preview?: boolean;
    confirm?: string;
    database_url?: string;
    postgres_dsn?: string;
    state_file?: string;
  }) {
    return this.request<DatabaseMigrationResult>("/system/admin/database/migrate", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async updateFromGit(payload: {
    repo_url: string;
    branch?: string;
    restart_services?: boolean;
    dry_run?: boolean;
    allow_dirty?: boolean;
  }) {
    return this.request<{
      project_root: string;
      repo_url: string;
      branch: string;
      dry_run?: boolean;
      updated?: boolean;
      restart_scheduled?: boolean;
      restart_available?: boolean;
      services?: string[];
      before?: {
        branch: string;
        commit: string;
        remote_url: string;
        dirty: boolean;
        dirty_count: number;
        dirty_files: string[];
      };
      after?: {
        branch: string;
        commit: string;
        remote_url: string;
        dirty: boolean;
        dirty_count: number;
        dirty_files: string[];
      };
      results: Array<{
        command: string;
        returncode: number;
        stdout: string;
        stderr: string;
        duration_ms: number;
      }>;
    }>("/system/admin/update", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async getAllApis() {
    return this.request<{ apis: Array<{ method: string; path: string; endpoint: string; full_path: string }>; total: number }>("/system/admin/apis");
  }

  // ==================== 定时任务管理 ====================

  async listSchedulerJobs() {
    return this.request<{ jobs: SchedulerJobItem[] }>(`/admin/scheduler/jobs`);
  }

  async triggerSchedulerJob(
    jobId: string,
    params?: Record<string, unknown>,
  ) {
    // Body 必须用 `runtime_params` 键名 — 后端 schedulerRuntimeParamsFromPayload
    // (internal/api/scheduler_handlers.go:160) 优先读 runtime_params；fallback
    // 到整个 payload 是兼容路径，但管理员 dry_run / max_per_run /
    // preserve_pending_register 这些键如果包到 `params` 下，fallback 也会把
    // {params:{...}} 整体当作 runtime params dict，子键全部丢失，最终按默认值
    // 跑。与 setSchedulerJobSchedule 的 payload 命名拉齐。
    return this.request<{ job_id: string; last_run: SchedulerJobRun | null }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/run`,
      {
        method: "POST",
        body: JSON.stringify({ runtime_params: params ?? {} }),
      },
    );
  }

  async getSchedulerJobLastRun(jobId: string) {
    return this.request<{ job_id: string; last_run: SchedulerJobRun | null }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/last-run`,
    );
  }

  async getSchedulerJobHistory(jobId: string, limit = 20) {
    const q = new URLSearchParams({ limit: String(limit) });
    return this.request<{ job_id: string; history: SchedulerJobRun[]; total: number }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/history?${q}`,
    );
  }

  async setSchedulerJobSchedule(jobId: string, payload: SchedulerSchedulePayload) {
    return this.request<{ job_id: string; trigger_spec: SchedulerTriggerSpec; runtime_params?: Record<string, unknown> | null; is_custom: boolean }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/schedule`,
      { method: "PUT", body: JSON.stringify(payload) },
    );
  }

  async resetSchedulerJobSchedule(jobId: string) {
    return this.request<{ job_id: string; trigger_spec: SchedulerTriggerSpec; runtime_params?: Record<string, unknown> | null; is_custom: boolean }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/schedule`,
      { method: "DELETE" },
    );
  }

  async syncAllEmbyUsers() {
    return this.request<{ success: number; failed: number; errors: string[] }>("/admin/emby/sync", {
      method: "POST",
    });
  }

  // Emby 管理
  async testEmbyConnectivity() {
    return this.request<{
      emby_url: string;
      tests: Array<{ name: string; success: boolean; latency_ms?: number; message: string }>;
      overall: boolean;
      server_info?: { name: string; version: string; os: string; id: string };
    }>("/admin/emby/test", { method: "POST" });
  }

  async listEmbyUsers() {
    return this.request<{
      emby_users: Array<{
        emby_id: string; emby_name: string; has_password: boolean;
        is_admin: boolean; is_disabled: boolean; is_hidden: boolean;
        last_login: string | null; last_activity: string | null;
        local_user: { uid: number; username: string; telegram_id: number | null; active: boolean; role: number } | null;
        sync_status: 'synced' | 'name_mismatch' | 'unlinked';
      }>;
      orphans: Array<{ uid: number; username: string; emby_id: string; telegram_id: number | null }>;
      total_emby: number; total_linked: number; total_orphans: number;
    }>("/admin/emby/users");
  }

  // Emby 登录用户的设备 / IP 审查（按用户聚合）：
  // /Devices 设备清单 + 实时 /Sessions IP + 活动日志历史登录 IP，映射完整本地账号。
  async adminGetEmbyDeviceAudit() {
    return this.request<EmbyDeviceAuditData>("/admin/emby/device-audit");
  }

  // 设备/IP 审查页的快速处置：按 Emby 用户 ID 单独启停 Emby（已关联本地用户时后端会
  // 沿用本地保护/有效期约束并同步镜像；未关联也可直接处置可疑 Emby 账号）。
  async setEmbyUserEnabledById(embyId: string, enable: boolean) {
    return this.request<{ emby_user_id: string; emby_enabled: boolean }>(
      `/admin/emby/users/${encodeURIComponent(embyId)}/${enable ? "enable" : "disable"}`,
      { method: "POST" },
    );
  }

  // 按 Emby 用户 ID 踢出其全部在线会话。
  async kickEmbyUserById(embyId: string) {
    return this.request<{ emby_user_id: string; kicked_count: number }>(
      `/admin/emby/users/${encodeURIComponent(embyId)}/kick`,
      { method: "POST" },
    );
  }

  async cleanupOrphanEmbyIds() {
    return this.request<{
      cleaned: Array<{ uid: number; username: string; old_emby_id: string }>;
      count: number;
    }>("/admin/emby/cleanup-orphans", { method: "POST" });
  }

  async importEmbyUsers(embyIds?: string[]) {
    return this.request<{
      unlinked: Array<{ emby_id: string; emby_name: string; is_disabled: boolean; is_hidden: boolean }>;
      skipped: Array<{ emby_id: string; name: string; reason: string }>;
      unlinked_count: number; skipped_count: number;
    }>("/admin/emby/import-users", {
      method: "POST",
      body: JSON.stringify(embyIds ? { emby_ids: embyIds } : {}),
    });
  }

  async deleteUnlinkedEmbyUsers(dryRun: boolean = false) {
    return this.request<{
      candidates: Array<{ emby_id: string; emby_name: string; is_disabled: boolean; is_hidden: boolean }>;
      deleted: Array<{ emby_id: string; emby_name: string; is_disabled: boolean; is_hidden: boolean }>;
      failed: Array<{ emby_id: string; emby_name: string; reason: string }>;
      count: number;
      dry_run: boolean;
    }>("/admin/emby/delete-unlinked", {
      method: "POST",
      body: JSON.stringify({ dry_run: dryRun }),
    });
  }

  async resetAllEmbyBindings() {
    return this.request<{ count: number }>("/admin/emby/reset-bindings", {
      method: "POST",
      body: JSON.stringify({ confirm: "RESET_ALL_EMBY" }),
    });
  }

  /**
   * 批量一键调控用户到期时间（按筛选条件覆盖普通用户）。
   * 后端需要 confirm="BULK_EXPIRE_OK"；前端这里强制带上。
   */
  async adminBulkSetExpire(payload: {
    expired_at?: number;          // -1 永久；正数 unix 秒
    days?: number;                // 与 expired_at 二选一，正数 = 从现在起 N 天；<=0 视为永久
    filter?: {
      role?: number;
      active?: boolean;
      emby?: "bound" | "unbound";
      email_status?: "verified" | "unverified" | "bound" | "none";
    };
    include_admin?: boolean;
    include_whitelist?: boolean;
    // 未绑定 Emby 的账号一律由后端强制跳过，无法通过此参数覆盖
  }) {
    return this.request<{
      matched: number;
      updated: number;
      expired_at: number;
      skipped_admins: number;
      skipped_whitelist: number;
      skipped_pending_emby: number;
      skipped_unrecognized?: number;
    }>("/admin/users/bulk-expire", {
      method: "POST",
      body: JSON.stringify({ ...payload, confirm: "BULK_EXPIRE_OK" }),
    });
  }

  /** 按当前筛选或指定 UID 批量启用已禁用账号。 */
  async adminBulkEnableDisabledUsers(payload: {
    uids?: number[];
    filter?: {
      role?: number;
      active?: boolean;
      emby?: "bound" | "unbound";
      email_status?: "verified" | "unverified" | "bound" | "none";
      search?: string;
    };
    include_admin?: boolean;
    include_whitelist?: boolean;
  }) {
    return this.request<{
      matched: number;
      eligible: number;
      enabled: number;
      failed: Array<{ uid: number; username?: string | null; reason: string }>;
      skipped: Array<{ uid: number; reason: string }>;
      skipped_admins: number;
      skipped_whitelist: number;
      skipped_unrecognized: number;
      skipped_active: number;
      enabled_users: Array<{ uid: number; username?: string | null }>;
    }>("/admin/users/bulk-enable-disabled", {
      method: "POST",
      body: JSON.stringify({ ...payload, confirm: "BULK_ENABLE_DISABLED_OK" }),
    });
  }

  /** 重新校验并启用已经回到 Telegram 群聊的禁用用户。 */
  async enableRejoinedTelegramUsers(maxPerRun = 500) {
    return this.request<{
      scanned: number;
      valid_telegram_users: number;
      invalid_telegram_id: number;
      candidates: number;
      eligible: number;
      enabled: number;
      failed: Array<{ uid: number; username?: string | null; reason: string }>;
      skipped: Array<{ uid: number; username?: string | null; reason: string }>;
      enabled_users: Array<{ uid: number; username?: string | null; telegram_id: number }>;
      max_per_run: number;
      limited: boolean;
    }>("/admin/telegram/rejoined-users/enable", {
      method: "POST",
      body: JSON.stringify({ confirm: "ENABLE_REJOINED_OK", max_per_run: maxPerRun }),
    });
  }

  /** 拉取 Bot 被动观察到的 TG 群花名册概况（用于踢人前的弹窗 / 状态展示）。 */
  async getTelegramRosterStats() {
    return this.request<{
      available: boolean;
      reason?: string;
      chat_id?: string;
      active?: number;
      inactive?: number;
      bots?: number;
      first_seen_at?: number | null;
      last_seen_at?: number | null;
    }>("/admin/telegram/roster/stats");
  }

  /** 一键踢出群里未绑定 Web 账号的成员。``dryRun=true`` 时只统计目标，不实际踢。 */
  async kickUnboundGroupMembers(opts: { dryRun?: boolean; maxPerRun?: number } = {}) {
    const body: Record<string, unknown> = {
      dry_run: !!opts.dryRun,
      max_per_run: opts.maxPerRun ?? 200,
    };
    if (!opts.dryRun) body.confirm = "KICK_UNBOUND_OK";
    return this.request<{
      chat_id: string;
      roster_size: number;
      bots_in_roster: number;
      preserved_bound: number;
      admins_excluded: number;
      excluded_total: number;
      targets: number;
      reason_no_account: number;
      reason_no_emby: number;
      reason_disabled: number;
      dry_run: boolean;
      max_per_run: number;
      kicked: number;
      skipped: number;
      failed: number;
      not_in_group: number;
      scanned: number;
      preview_targets?: Array<{ tg_id: number; reason: string }>;
    }>("/admin/telegram/kick-unbound", {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async cleanupInvalidUsers(minDays: number = 7, dryRun: boolean = false, confirm?: string) {
    const body: Record<string, unknown> = { min_days: minDays, dry_run: dryRun };
    if (!dryRun) {
      body.confirm = confirm || confirmPhrases.cleanupInvalidUsers;
    }
    return this.request<{
      users: Array<{
        uid: number;
        username: string;
        role: number;
        active: boolean;
        register_time: number | null;
      }>;
      count: number;
      dry_run: boolean;
    }>("/admin/users/cleanup-invalid", {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async clearAllUserEmails(opts: { dryRun?: boolean; confirm?: string } = {}) {
    const dryRun = Boolean(opts.dryRun);
    const body: Record<string, unknown> = { dry_run: dryRun };
    if (!dryRun) body.confirm = opts.confirm || confirmPhrases.clearAllUserEmails;
    return this.request<{
      users: Array<{ uid: number; username: string; role: number; active: boolean; register_time: number | null }>;
      count: number;
      cleared: number;
      total_users: number;
      dry_run: boolean;
    }>("/admin/users/clear-emails", {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async kickNoEmbyUsers(opts?: {
    dryRun?: boolean;
    confirm?: string;
    minDays?: number;
    preservePendingRegister?: boolean | null;
  }) {
    const dryRun = Boolean(opts?.dryRun);
    const body: Record<string, unknown> = {
      dry_run: dryRun,
      min_days: opts?.minDays ?? 0,
    };
    if (opts?.preservePendingRegister !== undefined) {
      body.preserve_pending_register = opts.preservePendingRegister;
    }
    if (!dryRun) {
      body.confirm = opts?.confirm || "KICK_NO_EMBY_OK";
    }
    return this.request<{
      candidates: Array<{
        uid: number;
        username: string;
        role: number;
        register_time: number | null;
        has_telegram: boolean;
        pending_emby: boolean;
      }>;
      candidate_count: number;
      deleted_count: number;
      failed: Array<{ uid: number; username: string; error: string }>;
      skipped_admins: number;
      skipped_whitelist: number;
      skipped_unrecognized: number;
      skipped_pending_register: number;
      skipped_too_recent: number;
      skipped_in_queue: number;
      min_days: number;
      preserve_pending_register: boolean;
      dry_run: boolean;
    }>("/admin/users/kick-no-emby", {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async clearStalePendingEmbyUsers(opts: { dryRun?: boolean } = {}) {
    const dryRun = Boolean(opts.dryRun);
    const body: Record<string, unknown> = { dry_run: dryRun };
    if (!dryRun) body.confirm = "CLEAR_PENDING_EMBY_OK";
    return this.request<{
      users: Array<{
        uid: number;
        username: string;
        telegram_id: number | null;
        register_time: number | null;
        created_at: number | null;
      }>;
      count: number;
      cleared: number;
      failed: Array<{ uid: number; username: string; error: string }>;
      dry_run: boolean;
    }>("/admin/users/clear-stale-pending-emby", {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async testBotConnectivity(target?: string) {
    return this.request<{
      results: Array<{
        target: string;
        success: boolean;
        error: string | null;
        username?: string;
        bot_id?: number;
        title?: string;
        bot_status?: string;
      }>;
      runtime?: {
        polling?: boolean;
        last_ok_at?: number | null;
        last_error_at?: number | null;
        last_error?: string;
      };
    }>("/system/admin/bot/test", {
      method: "POST",
      body: JSON.stringify(target ? { target } : {}),
    });
  }

  async getApiKeyStatus() {
    return this.request<{ enabled: boolean; has_key: boolean }>("/auth/apikey");
  }

  async generateApiKey() {
    return this.request<{ apikey: string; enabled: boolean }>("/auth/apikey", {
      method: "POST",
    });
  }

  async disableApiKey() {
    return this.request("/auth/apikey", {
      method: "DELETE",
    });
  }

  async enableApiKey() {
    return this.request<{ apikey: string; enabled: boolean }>("/auth/apikey/enable", {
      method: "POST",
    });
  }

  async refreshApiKey() {
    return this.request<{ apikey: string; enabled: boolean }>("/auth/apikey", {
      method: "POST",
    });
  }

  async getApiKeyPermissions() {
    return this.request<{ permissions: string[] }>("/auth/apikey/permissions");
  }

  async updateApiKeyPermissions(permissions: string[]) {
    return this.request<{ permissions: string[] }>("/auth/apikey/permissions", {
      method: "PUT",
      body: JSON.stringify({ permissions }),
    });
  }

  // Appearance
  async getUserBackground(uid: number) {
    const res = await this.request<{ background: string | null }>(`/users/${uid}/background`);
    if (res.success && res.data?.background) {
      try {
        const config = JSON.parse(res.data.background);
        config.lightBgImage = this.normalizeBackgroundAssetCssUrlValue(config.lightBgImage);
        config.darkBgImage = this.normalizeBackgroundAssetCssUrlValue(config.darkBgImage);
        res.data.background = JSON.stringify(config);
      } catch {
        // ignore invalid legacy format
      }
    }
    return res;
  }

  async updateUserBackground(payload: {
    lightBg: string;
    darkBg: string;
    lightBgImage: string;
    darkBgImage: string;
    lightFlow?: boolean;
    darkFlow?: boolean;
    lightBlur?: number;
    darkBlur?: number;
    lightOpacity?: number;
    darkOpacity?: number;
  }) {
    const body = {
      ...payload,
      lightBgImage: this.toApiRelativeAssetValue(payload.lightBgImage),
      darkBgImage: this.toApiRelativeAssetValue(payload.darkBgImage),
    };
    return this.request<{ background: string }>('/users/me/background', {
      method: 'PUT',
      body: JSON.stringify(body),
    });
  }

  async deleteUserBackground() {
    return this.request('/users/me/background', {
      method: 'DELETE',
    });
  }

  async uploadBackgroundImage(file: File, type: 'light' | 'dark') {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('type', type);
    const res = await this.requestForm<{ url: string; type: string; filename: string }>(
      '/users/me/background/upload',
      formData,
      'POST'
    );
    return res;
  }

  async getUserAvatar(uid: number) {
    const res = await this.request<{ avatar: string | null; uid: number; username: string }>(`/users/${uid}/avatar`);
    if (res.success && res.data?.avatar) {
      res.data.avatar = this.toAbsoluteAssetUrl(res.data.avatar);
    }
    return res;
  }

  async uploadAvatar(file: File) {
    const formData = new FormData();
    formData.append('file', file);
    const res = await this.requestForm<{ avatar_url: string }>('/users/me/avatar/upload', formData, 'POST');
    if (res.success && res.data?.avatar_url) {
      res.data.avatar_url = this.toAbsoluteAssetUrl(res.data.avatar_url) || res.data.avatar_url;
    }
    return res;
  }

  async uploadServerIcon(file: File) {
    const formData = new FormData();
    formData.append('file', file);
    return this.requestForm<{ url: string; server_icon: string; filename: string; reload?: unknown }>(
      '/system/admin/server-icon/upload',
      formData,
      'POST'
    );
  }

  async terminateSchedulerJob(jobId: string) {
    return this.request<{ job_id: string; terminated: boolean }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/terminate`,
      { method: "POST" },
    );
  }

  async deleteAvatar() {
    return this.request('/users/me/avatar', {
      method: 'DELETE',
    });
  }

  // Multi API Keys
  async getMyApiKeys() {
    return this.request<{ keys: ApiKeyItem[]; total: number }>('/users/me/apikeys');
  }

  async createMyApiKey(payload: {
    name: string;
    allow_query: boolean;
    rate_limit: number;
  }) {
    return this.request<{ id: number; key: string; name: string; created_at: number }>('/users/me/apikeys', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async updateMyApiKey(
    keyId: number,
    payload: {
      name: string;
      enabled: boolean;
      allow_query: boolean;
      rate_limit: number;
    }
  ) {
    return this.request<{ id: number; name: string; enabled: boolean }>(`/users/me/apikeys/${keyId}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
  }

  async deleteMyApiKey(keyId: number) {
    return this.request(`/users/me/apikeys/${keyId}`, {
      method: 'DELETE',
    });
  }

  async getRegcodes(page = 1, params: { type?: string; status?: string; source?: string; search?: string; sort?: string; order?: string; per_page?: number } = {}) {
    const query = new URLSearchParams({ page: String(page) });
    if (params.type && params.type !== "all") query.set("type", params.type);
    if (params.status && params.status !== "all") query.set("status", params.status);
    if (params.source && params.source !== "all") query.set("source", params.source);
    if (params.search) query.set("search", params.search);
    if (params.sort) query.set("sort", params.sort);
    if (params.order) query.set("order", params.order);
    if (params.per_page) query.set("per_page", String(params.per_page));
    return this.request<{ regcodes: Regcode[]; total: number }>(
      `/admin/regcodes?${query.toString()}`
    );
  }

  async createRegcode(data: CreateRegcodeData) {
    return this.request<{ codes: string[]; count: number; decoy?: boolean; target_username?: string; target_telegram_username?: string; target_telegram_id?: number }>("/admin/regcodes", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteRegcode(code: string) {
    return this.batchDeleteRegcodes([code]);
  }

  async batchDeleteRegcodes(codes: string[]) {
    return this.request<{
      deleted: number;
      deleted_codes: string[];
      missing: number;
      missing_codes: string[];
    }>("/admin/regcodes/batch-delete", {
      method: "POST",
      body: JSON.stringify({ codes, confirm: confirmPhrases.batchDeleteRegcodes }),
    });
  }

  // 部分更新注册码：仅传入需要改动的字段。active 控制停用/启用，
  // validity_time（小时，-1 永久）/ days / use_count_limit 用于编辑有效期与额度。
  async updateRegcode(
    code: string,
    data: { note?: string; active?: boolean; validity_time?: number; days?: number; use_count_limit?: number },
  ) {
    return this.request<Regcode>(`/admin/regcodes/${encodeURIComponent(code)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async getRegcodeUsers(code: string) {
    return this.request<{
      code: string;
      use_count: number;
      users: Array<(Partial<UserInfo> & { found: boolean; source: "uid" | "telegram" })>;
      telegram_only: Array<{ telegram_id: number; found: false; source: "telegram" }>;
    }>(`/admin/regcodes/${encodeURIComponent(code)}/users`);
  }

  async clearRegcodeUsage(code: string) {
    return this.request<{
      code: string;
      cleared_use_count: number;
      cleared_used_by_uids: number[] | null;
      cleared_used_by_telegram: number[] | null;
    }>(`/admin/regcodes/${encodeURIComponent(code)}/clear-usage`, {
      method: "POST",
      body: JSON.stringify({ confirm: "CLEAR_REGCODE_USAGE" }),
    });
  }

  async getAdminInviteCodes() {
    return this.request<{
      codes: InviteCodeItem[];
      total: number;
    }>("/admin/invite/codes");
  }

  async forgotPasswordByEmby(data: { emby_username: string; emby_password: string }) {
    return this.request<{ username: string; new_password: string }>("/auth/forgot-password/emby", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // Violations audit
  async getViolations(page = 1, params: { type?: string; search?: string } = {}) {
    const query = new URLSearchParams({ page: String(page) });
    if (params.type && params.type !== "all") query.set("type", params.type);
    if (params.search) query.set("search", params.search);
    return this.request<{ violations: ViolationLog[]; total: number; page: number; per_page: number }>(
      `/admin/violations?${query.toString()}`
    );
  }

  async deleteViolation(id: number) {
    return this.request(`/admin/violations/${id}`, { method: "DELETE" });
  }

  async clearViolations() {
    return this.request("/admin/violations/clear", {
      method: "POST",
      body: JSON.stringify({ confirm: confirmPhrases.clearViolations }),
    });
  }

  // Audit logs
  async getAuditLogs(page = 1, params: { category?: string; action?: string; uid?: string; search?: string; per_page?: number; signal?: AbortSignal } = {}) {
    const query = new URLSearchParams({ page: String(page), per_page: String(params.per_page ?? 50) });
    if (params.category && params.category !== "all") query.set("category", params.category);
    if (params.action && params.action !== "all") query.set("action", params.action);
    if (params.uid) query.set("uid", params.uid);
    if (params.search) query.set("search", params.search);
    return this.request<{ logs: AuditLog[]; total: number; page: number; per_page: number }>(
      `/admin/audit-logs?${query.toString()}`,
      { signal: params.signal }
    );
  }

  async deleteAuditLog(id: number) {
    return this.request(`/admin/audit-logs/${id}`, { method: "DELETE" });
  }

  async clearAuditLogs() {
    return this.request("/admin/audit-logs/clear", {
      method: "POST",
      body: JSON.stringify({ confirm: confirmPhrases.clearAuditLogs }),
    });
  }

  async getMediaRequests(params: { page?: number; status?: string } = {}, signal?: AbortSignal) {
    const query = new URLSearchParams();
    if (params.page) query.set("page", String(params.page));
    if (params.status) query.set("status", params.status);
    const res = await this.request<{ requests: MediaRequest[]; total: number }>(
      `/admin/media-requests?${query}`,
      { signal }
    );
    if (res.success && res.data?.requests) {
      res.data.requests = res.data.requests.map((item) => ({
        ...item,
        status: normalizeMediaRequestStatus(item.status, "admin"),
      }));
    }
    return res;
  }

  /**
   * 管理员更新求片状态。第一个参数现在是 require_key（全局唯一），避免
   * Bangumi/TMDB 两表数值 id 撞车把操作打到错的求片。
   */
  async updateMediaRequest(requireKey: string, status: string, note?: string) {
    const normalizedStatus = normalizeMediaRequestStatus(status, "admin");
    const normalizedNote = (note || "").trim().slice(0, 1000);
    return this.request(`/admin/media-requests/by-key/${encodeURIComponent(requireKey)}`, {
      method: "PUT",
      body: JSON.stringify({ status: normalizedStatus, note: normalizedNote }),
    });
  }

  /** 管理员删除任意求片，按 require_key。 */
  async deleteMediaRequest(requireKey: string) {
    return this.request(`/admin/media-requests/by-key/${encodeURIComponent(requireKey)}`, {
      method: "DELETE",
    });
  }

  /** 用户删除自己的求片（也允许管理员），按 require_key。 */
  async deleteMyMediaRequest(requireKey: string) {
    return this.request(`/media/request/by-key/${encodeURIComponent(requireKey)}`, {
      method: "DELETE",
    });
  }

  // ==================== Announcements ====================

  /** 公开列表：登录页 / 主页等场景可直接调用。 */
  async getActiveAnnouncements(limit: number = 50) {
    return this.request<{ announcements: Announcement[]; total: number }>(
      `/announcements?limit=${limit}`
    );
  }

  /** 管理员视角列表，含历史与隐藏条目。 */
  async adminListAnnouncements(params: {
    page?: number;
    per_page?: number;
    include_invisible?: boolean;
    include_expired?: boolean;
  } = {}) {
    const query = new URLSearchParams();
    if (params.page) query.set('page', String(params.page));
    if (params.per_page) query.set('per_page', String(params.per_page));
    if (params.include_invisible !== undefined) query.set('include_invisible', String(params.include_invisible));
    if (params.include_expired !== undefined) query.set('include_expired', String(params.include_expired));
    return this.request<{
      announcements: Announcement[];
      total: number;
      page: number;
      per_page: number;
      pages: number;
    }>(`/admin/announcements?${query.toString()}`);
  }

  async adminCreateAnnouncement(payload: {
    title?: string;
    content: string;
    level?: 'info' | 'notice' | 'warning' | 'critical';
    render_mode?: AnnouncementRenderMode;
    pinned?: boolean;
    visible?: boolean;
    expires_at?: number;
  }) {
    return this.request<Announcement>(`/admin/announcements`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async adminUpdateAnnouncement(id: number, payload: {
    title?: string;
    content?: string;
    level?: 'info' | 'notice' | 'warning' | 'critical';
    render_mode?: AnnouncementRenderMode;
    pinned?: boolean;
    visible?: boolean;
    expires_at?: number;
  }) {
    return this.request<Announcement>(`/admin/announcements/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
  }

  async adminDeleteAnnouncement(id: number) {
    return this.request(`/admin/announcements/${id}`, {
      method: 'DELETE',
    });
  }

  // ==================== 邀请树 ====================
  async getInviteConfig() {
    return this.request<InviteConfig>("/invite/config");
  }

  async getMyInviteStatus() {
    return this.request<InviteMyStatus>("/invite/me");
  }

  async getMyInviteCodes() {
    return this.request<{ codes: InviteCodeItem[]; total: number }>("/invite/codes");
  }

  async createInviteCode(payload: { days?: number; expires_at?: number; note?: string; target_username?: string }) {
    return this.request<InviteCodeItem>("/invite/codes", {
      method: "POST",
      body: JSON.stringify(payload || {}),
    });
  }

  async createInviteRenewCode(payload: { target_uid: number; days: number; validity_hours?: number; note?: string; format?: string; random_algorithm?: string }) {
    return this.request<{
      code: string;
      target_uid: number;
      target_username: string;
      days: number;
      validity_hours: number;
      max_code_days: number;
    }>("/invite/renew-codes", {
      method: "POST",
      body: JSON.stringify(payload || {}),
    });
  }

  async revokeInviteCode(code: string) {
    return this.request(`/invite/codes/${encodeURIComponent(code)}`, {
      method: "DELETE",
    });
  }

  async detachExpiredInviteChild(uid: number) {
    return this.request<{ uid: number; detached: boolean; deleted_emby: boolean }>(
      `/invite/children/${uid}/detach-expired`,
      { method: "POST" },
    );
  }

  async useInviteCode(payload: { code: string; emby_username: string; emby_password: string }) {
    return this.request<{
      emby_id: string;
      emby_username: string;
      expired_at: number;
      inviter_uid: number | null;
      days: number;
    }>("/invite/use", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async checkInviteCode(code: string) {
    const query = new URLSearchParams({ code });
    return this.request<{ days: number; inviter: string | null }>(`/invite/check?${query.toString()}`);
  }

  // 管理员：邀请森林
  async adminGetInviteTree() {
    return this.request<InviteForest>("/admin/invite/tree");
  }

  async adminDetachInviteUser(uid: number) {
    return this.request<{ uid: number; is_root: boolean }>(`/admin/invite/users/${uid}/detach`, {
      method: "POST",
    });
  }

  /**
   * 邀请树级联的禁用/启用。
   * cascadeDepth：1=仅本人；N=本人+下 N-1 层；0=整棵子树。
   * - 不会翻动其他管理员账号
   * - 不会改邀请关系，只改 ACTIVE_STATUS + 同步 Emby
   */
  async toggleUserActive(uid: number, options: {
    enable: boolean;
    cascadeDepth?: number;
    reason?: string;
  }) {
    const cascadeDepth = Math.max(0, Math.floor(options.cascadeDepth ?? 1));
    const path = options.enable ? "enable" : "disable";
    return this.request<{
      affected: number[];
      skipped: Array<{ uid: number; reason: string }>;
      failed: Array<{ uid: number; reason: string }>;
      // 本地禁用成功但 Emby 远端关停失败的用户（仅禁用方向返回）。
      emby_failed?: Array<{ uid: number; reason: string }>;
      cascade_depth: number | string;
      enable: boolean;
    }>(`/admin/users/${uid}/${path}`, {
      method: "POST",
      body: JSON.stringify({
        cascade_depth: cascadeDepth,
        ...(options.reason ? { reason: options.reason } : {}),
      }),
    });
  }

  /**
   * 扩展用户删除：支持邀请树级联，且三种 mode 均可级联。
   * - mode = with_emby：本地 + Emby
   * - mode = local_only：仅本地（保留 Emby）
   * - mode = emby_only：仅 Emby（保留本地与树关系）
   * - cascadeDepth：1 = 仅本人；2 = 本人+直接下级；... 传 0 表示整棵子树
   */
  async deleteUserScoped(
    uid: number,
    options: {
      mode: "with_emby" | "local_only" | "emby_only";
      cascadeDepth?: number;
    },
  ) {
    const cascadeDepth = Math.max(0, Math.floor(options.cascadeDepth ?? 1));
    return this.request<{
      deleted: number[];
      skipped: Array<{ uid: number; reason: string }>;
      failed: Array<{ uid: number; reason: string }>;
      mode: string;
      cascade_depth: number | string;
    }>(`/admin/users/${uid}/delete`, {
      method: "POST",
      body: JSON.stringify({ mode: options.mode, cascade_depth: cascadeDepth }),
    });
  }

  /** @deprecated 用 deleteUserScoped 代替。保留旧 API 兼容。 */
  async deleteUserCascade(uid: number, options?: { deleteEmby?: boolean; cascadeDepth?: number }) {
    return this.deleteUserScoped(uid, {
      mode: options?.deleteEmby === false ? "local_only" : "with_emby",
      cascadeDepth: options?.cascadeDepth ?? 1,
    });
  }
}

export const api = new ApiClient();

export { ApiError } from "./api-request";

export type * from "./api-types";
