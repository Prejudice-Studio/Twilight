// 后端 API 地址
const API_BASE = (process.env.NEXT_PUBLIC_API_URL || "").replace(/\/$/, "");

interface ApiResponse<T = unknown> {
  success: boolean;
  message: string;
  data?: T;
}

function isAbortError(error: unknown): boolean {
  if (error instanceof DOMException && error.name === "AbortError") {
    return true;
  }
  if (error instanceof Error && error.name === "AbortError") {
    return true;
  }
  return false;
}

function requestMethod(options: RequestInit, fallback = "GET"): string {
  return (options.method || fallback).toString().toUpperCase();
}

function describeApiTarget(endpoint: string, method: string): string {
  return `${method} /api/v1${endpoint}`;
}

function buildHttpErrorMessage(
  status: number,
  endpoint: string,
  method: string,
  backendMessage?: string,
): string {
  const target = describeApiTarget(endpoint, method);
  const detail = backendMessage && backendMessage !== "接口不存在" ? `后端返回：${backendMessage}` : "";

  if (status === 404) {
    if (backendMessage && backendMessage !== "接口不存在") {
      return backendMessage;
    }
    return [
      `接口不存在：${target}`,
      detail,
      "常见原因：后端未更新/未重启、前后端版本不一致，或当前功能在后端尚未实现。",
    ].filter(Boolean).join("\n");
  }
  if (status === 405) {
    return [
      `请求方法不允许：${target}`,
      detail,
      "请确认前端调用的方法与后端路由一致，例如 GET/POST/PUT/DELETE 是否写反。",
    ].filter(Boolean).join("\n");
  }
  if (status === 401) {
    return backendMessage || "登录状态已失效，请重新登录。";
  }
  if (status === 403) {
    return backendMessage || `权限不足：当前账号无权访问 ${target}。`;
  }
  if (status === 413) {
    return backendMessage || "上传内容过大，请压缩文件或联系管理员调整上传上限。";
  }
  if (status === 429) {
    return backendMessage || "请求过于频繁，请稍后再试。";
  }
  if (status === 500) {
    return [
      `后端接口执行失败：${target}`,
      detail || "服务器内部错误，请查看后端日志。",
    ].filter(Boolean).join("\n");
  }
  if (status === 502 || status === 503 || status === 504) {
    return `后端服务暂不可用 (${status})：${target}\n请确认 API 服务、反向代理或网关正在运行。`;
  }
  return backendMessage || `请求失败 (${status})：${target}`;
}

function buildParseErrorMessage(status: number, endpoint: string, method: string): string {
  const target = describeApiTarget(endpoint, method);
  if (status === 404) {
    return `接口不存在：${target}\n服务器没有返回标准 JSON，可能命中了前端页面 404、反向代理路径错误，或后端缺少该路由。`;
  }
  if (status >= 500) {
    return `后端响应格式异常：${target}\nHTTP ${status} 未返回标准 JSON，请查看后端或网关日志。`;
  }
  return `服务器响应解析失败 (${status})：${target}\n接口没有返回标准 JSON。`;
}

async function parseApiResponse<T>(
  response: Response,
  endpoint: string,
  method: string,
): Promise<ApiResponse<T>> {
  if (response.status === 204) {
    return { success: true, message: "OK" };
  }

  const text = await response.text();
  if (!text) {
    return { success: response.ok, message: response.ok ? "OK" : response.statusText };
  }

  try {
    return JSON.parse(text) as ApiResponse<T>;
  } catch (error) {
    if (!response.ok) {
      return { success: false, message: response.statusText || `HTTP ${response.status}` };
    }
    console.error("JSON parse error:", error);
    throw new Error(buildParseErrorMessage(response.status, endpoint, method));
  }
}

class ApiClient {
  private normalizeRequestStatus(status?: string | null, mode: "user" | "admin" = "user"): string {
    const raw = (status || "").trim().toLowerCase();
    if (mode === "admin") {
      const adminMap: Record<string, string> = {
        pending: "pending",
        unhandled: "pending",
        accepted: "accepted",
        rejected: "rejected",
        completed: "completed",
        downloading: "downloading",
      };
      return adminMap[raw] || "pending";
    }

    const userMap: Record<string, string> = {
      pending: "UNHANDLED",
      unhandled: "UNHANDLED",
      accepted: "ACCEPTED",
      rejected: "REJECTED",
      completed: "COMPLETED",
      downloading: "DOWNLOADING",
    };
    return userMap[raw] || (status || "UNHANDLED").toUpperCase();
  }

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
    options: RequestInit = {}
  ): Promise<ApiResponse<T>> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "X-Twilight-Client": "webui",
      ...((options.headers as Record<string, string>) || {}),
    };

    const url = `${API_BASE}/api/v1${endpoint}`;
    const method = requestMethod(options);
    
    let response: Response;
    try {
      response = await fetch(url, {
        ...options,
        headers,
        credentials: "include",
      });
    } catch (error) {
      if (isAbortError(error)) {
        throw error;
      }
      console.error("Network error:", error);
      throw new Error(
        `无法连接后端接口：${describeApiTarget(endpoint, method)}\n请检查后端服务是否启动、API 地址是否正确、反向代理是否可达。`
      );
    }

    const data = await parseApiResponse<T>(response, endpoint, method);

    if (!response.ok) {
      throw new Error(buildHttpErrorMessage(response.status, endpoint, method, data?.message));
    }

    return data;
  }

  private async requestForm<T>(
    endpoint: string,
    formData: FormData,
    method: "POST" | "PUT" = "POST"
  ): Promise<ApiResponse<T>> {
    const headers: Record<string, string> = {
      "X-Twilight-Client": "webui",
    };

    const url = `${API_BASE}/api/v1${endpoint}`;
    const methodName = method.toUpperCase();

    let response: Response;
    try {
      response = await fetch(url, {
        method,
        headers,
        body: formData,
        credentials: "include",
      });
    } catch (error) {
      if (isAbortError(error)) {
        throw error;
      }
      console.error("Network error:", error);
      throw new Error(
        `无法连接后端接口：${describeApiTarget(endpoint, methodName)}\n请检查后端服务是否启动、API 地址是否正确、反向代理是否可达。`
      );
    }

    const data = await parseApiResponse<T>(response, endpoint, methodName);

    if (!response.ok) {
      throw new Error(buildHttpErrorMessage(response.status, endpoint, methodName, data?.message));
    }

    return data;
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
      method: "POST",
    });
  }

  async getRegisterBindCode() {
    return this.request<{ bind_code: string; expires_in: number }>("/users/telegram/register/bind-code", {
      method: "POST",
    });
  }

  async getRegisterBindCodeStatus(code: string, signal?: AbortSignal) {
    const q = new URLSearchParams({ code }).toString();
    // 后端约定：code 在 DB 不存在 / 已过期 / 已确认 都是 *终态*，
    // 通过 data.terminal === true 表示；其中 invalid 区分"不存在/过期"和"已确认"。
    // 这一端点不会再为业务无效抛 HTTP 404——上面的字段是唯一可信信号。
    return this.request<{
      code?: string;
      confirmed?: boolean;
      expires_in?: number;
      invalid?: boolean;
      terminal?: boolean;
    }>(
      `/users/telegram/register/bind-code/status?${q}`,
      { signal },
    );
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
    return this.request("/users/me/telegram/unbind", {
      method: "POST",
    });
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
    return this.request("/users/me/emby/unbind", {
      method: "POST",
    });
  }

  async changePassword(oldPassword: string, newPassword: string) {
    return this.request("/users/me/password/change", {
      method: "POST",
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    });
  }

  async changeSystemPassword(oldPassword: string, newPassword: string) {
    return this.request("/users/me/password/system", {
      method: "POST",
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    });
  }

  async changeEmbyPassword(newPassword: string) {
    return this.request("/users/me/password/emby", {
      method: "POST",
      body: JSON.stringify({ new_password: newPassword }),
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

  async getMyLibraries() {
    return this.request<EmbyLibraryAccess>("/users/me/libraries");
  }

  async updateMyLibraryVisibility(action: "show" | "hide", libraryNames?: string[]) {
    return this.request<EmbyLibraryAccess>("/users/me/libraries/visibility", {
      method: "PUT",
      body: JSON.stringify({ action, library_names: libraryNames }),
    });
  }

  async checkRegcode(regCode: string) {
    return this.request<{ type: number; type_name: string; days: number; valid: boolean }>("/users/regcode/check", {
      method: "POST",
      body: JSON.stringify({ reg_code: regCode }),
    });
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
    return this.request<{ results: MediaItem[] }>(
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
        status: this.normalizeRequestStatus(item.status, "user"),
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
    return this.request<EmbyDevice[]>("/users/me/devices");
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
    if (params.search) query.set("search", params.search);
    if (params.sort) query.set("sort", params.sort);
    return this.request<AdminUserListResponse>(`/admin/users?${query}`, { signal });
  }

  async getUser(uid: number) {
    return this.request<UserInfo>(`/admin/users/${uid}`);
  }

  async updateUser(uid: number, data: Partial<UserUpdateData>) {
    return this.request(`/admin/users/${uid}`, {
      method: "PUT",
      body: JSON.stringify(data),
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
    filter?: { role?: number; active?: boolean; emby?: "bound" | "unbound"; search?: string };
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

  async getUserLibraries(uid: number) {
    return this.request<EmbyLibraryAccess>(`/admin/users/${uid}/libraries`);
  }

  async updateUserLibraries(uid: number, payload: {
    action?: "set" | "show" | "hide" | "enable_all" | "disable_all";
    library_ids?: string[];
    library_names?: string[];
    enable_all?: boolean;
  }) {
    return this.request<EmbyLibraryAccess>(`/admin/users/${uid}/libraries`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  }

  async setUserLibrarySelfService(uid: number, enabled: boolean) {
    return this.request<{ uid: number; library_self_service: boolean }>(`/admin/users/${uid}/library-self-service`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    });
  }

  async bulkEnableLibrarySelfService() {
    return this.request<{ updated: number }>("/admin/users/library-self-service/bulk-enable", {
      method: "POST",
      body: JSON.stringify({ confirm: "ENABLE_LIBRARY_SELF_SERVICE" }),
    });
  }

  async cancelUserPermanent(uid: number, days: number) {
    return this.request<{ uid: number; days: number; expired_at: number; role: number; role_name: string; downgraded_whitelist: boolean }>(`/admin/users/${uid}/cancel-permanent`, {
      method: "POST",
      body: JSON.stringify({ days }),
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
    return this.request<{ content: string; path: string }>("/system/admin/config/toml");
  }

  async updateConfigToml(content: string) {
    return this.request<{ path: string }>("/system/admin/config/toml", {
      method: "PUT",
      body: JSON.stringify({ content }),
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

  async createDatabaseBackup() {
    return this.request<{ backup: DatabaseBackup }>("/system/admin/database/backup", {
      method: "POST",
      body: JSON.stringify({}),
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
    return this.request<{ job_id: string; last_run: SchedulerJobRun | null }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/run`,
      {
        method: "POST",
        body: JSON.stringify(params ? { params } : {}),
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

  async setSchedulerJobSchedule(jobId: string, payload: SchedulerTriggerSpec) {
    return this.request<{ job_id: string; trigger_spec: SchedulerTriggerSpec; is_custom: boolean }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/schedule`,
      { method: "PUT", body: JSON.stringify(payload) },
    );
  }

  async resetSchedulerJobSchedule(jobId: string) {
    return this.request<{ job_id: string; trigger_spec: SchedulerTriggerSpec; is_custom: boolean }>(
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

  async cleanupInvalidUsers(minDays: number = 7, dryRun: boolean = false) {
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
      body: JSON.stringify({ min_days: minDays, dry_run: dryRun }),
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
      }>;
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

  async getRegcodes(page = 1, params: { type?: string; status?: string; search?: string; sort?: string; order?: string } = {}) {
    const query = new URLSearchParams({ page: String(page) });
    if (params.type && params.type !== "all") query.set("type", params.type);
    if (params.status && params.status !== "all") query.set("status", params.status);
    if (params.search) query.set("search", params.search);
    if (params.sort) query.set("sort", params.sort);
    if (params.order) query.set("order", params.order);
    return this.request<{ regcodes: Regcode[]; total: number }>(
      `/admin/regcodes?${query.toString()}`
    );
  }

  async createRegcode(data: CreateRegcodeData) {
    return this.request<{ codes: string[]; count: number; decoy?: boolean }>("/admin/regcodes", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteRegcode(code: string) {
    return this.request(`/admin/regcodes/${encodeURIComponent(code)}`, {
      method: "DELETE",
    });
  }

  async batchDeleteRegcodes(codes: string[]) {
    return this.request<{
      deleted: number;
      deleted_codes: string[];
      missing: number;
      missing_codes: string[];
    }>("/admin/regcodes/batch-delete", {
      method: "POST",
      body: JSON.stringify({ codes }),
    });
  }

  async updateRegcode(code: string, data: { note?: string }) {
    return this.request<{ code: string; note: string }>(`/admin/regcodes/${encodeURIComponent(code)}`, {
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

  async forgotPasswordByEmby(data: { emby_username: string; emby_password: string }) {
    return this.request<{ username: string; new_password: string }>("/auth/forgot-password/emby", {
      method: "POST",
      body: JSON.stringify(data),
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
        status: this.normalizeRequestStatus(item.status, "admin"),
      }));
    }
    return res;
  }

  /**
   * 管理员更新求片状态。第一个参数现在是 require_key（全局唯一），避免
   * Bangumi/TMDB 两表数值 id 撞车把操作打到错的求片。
   */
  async updateMediaRequest(requireKey: string, status: string, note?: string) {
    const normalizedStatus = this.normalizeRequestStatus(status, "admin");
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

  async createInviteCode(payload: { days?: number; expires_at?: number; note?: string }) {
    return this.request<InviteCodeItem>("/invite/codes", {
      method: "POST",
      body: JSON.stringify(payload || {}),
    });
  }

  async createInviteRenewCode(payload: { target_uid: number; days: number; validity_hours?: number; note?: string }) {
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
    return this.request<{ days: number; inviter: string | null }>("/invite/check", {
      method: "POST",
      body: JSON.stringify({ code }),
    });
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
    }>(`/admin/users/${uid}`, {
      method: "DELETE",
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

// Types
export interface SystemInfo {
  name: string;
  icon: string;
  version: string;
  features: Record<string, boolean>;
  limits: Record<string, number | null>;
  telegram_bot?: {
    username: string | null;
    url: string | null;
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
  is_pending?: boolean;  // 是否待激活
  pending_emby?: boolean;  // 系统账号已建但待补建 Emby
  pending_emby_days?: number | null;  // 注册码授予的开通天数（待 Emby 补建）
  emby_disabled_by_expiry?: boolean;  // 到期后仅禁用 Emby，系统账号仍可登录
  library_self_service?: boolean;  // 是否允许在个人设置中自助显隐管理员开放的媒体库
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
    force_bind: boolean;
    can_unbind: boolean;
    can_change: boolean;
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
  pending_rebind_request?: boolean;
  rebind_request_status?: string | null;
  rebind_request_id?: number | null;
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

export interface EmbyLibraryItem {
  id: string;
  name: string;
  type?: string;
}

export interface EmbyLibraryAccess {
  has_emby: boolean;
  enable_all: boolean;
  enabled_ids: string[];
  blocked_names: string[];
  all_libraries: EmbyLibraryItem[];
  libraries: EmbyLibraryItem[];
  default_hidden_libraries: string[];
  self_service_libraries: string[];
  self_service_enabled: boolean;
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
}

export interface RegisterAvailability {
  available: boolean;
  message: string;
  current_users: number;
  max_users: number;
  register_mode: boolean;
  allow_pending_register: boolean;
  emby_direct_register_enabled: boolean;
  // 管理员单值固定的开通天数（-1 永久）；客户端只读
  emby_direct_register_days: number;
  // 兼容老前端：等值单项数组，恒不允许自定义
  emby_direct_register_day_options?: number[];
  emby_direct_register_allow_custom_days?: boolean;
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
  used: boolean;
  used_by?: number | string;
  used_by_uids?: number[];
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
}

export interface ConfigFieldOption {
  label: string;
  value: number | string;
}

export interface ConfigField {
  key: string;
  label: string;
  type: 'string' | 'textarea' | 'int' | 'float' | 'bool' | 'secret' | 'list' | 'select';
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
}

export interface DatabaseStatus {
  active_driver: string;
  configured_driver: string;
  state_file: string;
  backup_dir: string;
  backup_count: number;
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
  target_ready?: Record<string, unknown>;
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
  days: number;
  use_count_limit: number;
  use_count: number;
  expires_at: number;
  active: boolean;
  created_at: number;
  used_by_uid?: number | null;
  used_at?: number | null;
  note?: string | null;
}

export interface InviteTreeNode {
  uid: number;
  username: string;
  active: boolean;
  has_emby: boolean;
  expired_at?: number | null;
  expire_status?: string;
  emby_expired?: boolean;
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
}

export interface SigninActionResult {
  today_signed: boolean;
  daily_points: number;
  bonus_points: number;
  total_today: number;
  current_streak: number;
  longest_streak: number;
  current_points: number;
  currency_name: string;
}

export interface SigninHistoryRecord {
  date: string;
  daily_points: number;
  bonus_points: number;
  total: number;
  streak: number;
  created_at: number;
}
