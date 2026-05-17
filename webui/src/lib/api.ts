// 后端 API 地址
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:5000";

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
    if (/^(https?:)?\/\//i.test(url) || url.startsWith("data:") || url.startsWith("blob:")) {
      return url;
    }
    if (url.startsWith("/")) {
      return `${API_BASE}${url}`;
    }
    return `${API_BASE}/${url}`;
  }

  private normalizeCssUrlValue(value?: string | null): string {
    if (!value) return "";
    return value.replace(/url\((['"]?)(.*?)\1\)/g, (_match, quote, rawUrl: string) => {
      const normalized = this.toAbsoluteAssetUrl(rawUrl.trim()) || rawUrl.trim();
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
      ...((options.headers as Record<string, string>) || {}),
    };

    const url = `${API_BASE}/api/v1${endpoint}`;
    
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
      throw new Error("网络连接失败，请检查后端服务是否启动");
    }

    // 检查响应内容类型
    const contentType = response.headers.get("content-type");

    let data: ApiResponse<T>;
    
    // 尝试解析JSON，即使content-type不匹配
    try {
      data = await response.json();
    } catch (error) {
      // 如果不是JSON，检查状态码
      if (response.status === 404) {
        throw new Error("接口不存在，请检查后端服务是否已重启并包含最新路由");
      }
      if (response.status === 403) {
        throw new Error("权限不足，请确认您有访问此接口的权限");
      }
      if (response.status === 401) {
        throw new Error("未授权，请重新登录");
      }
      console.error("JSON parse error:", error);
      throw new Error(`服务器响应解析失败 (${response.status})`);
    }

    if (!response.ok) {
      // 如果后端返回了错误信息，使用后端的错误信息
      if (data && data.message) {
        // 对于404错误，提供更详细的提示
        if (response.status === 404) {
          throw new Error(`接口不存在: ${data.message}。请确认后端服务已重启并包含最新代码。`);
        }
        throw new Error(data.message);
      }
      // 否则根据状态码提供友好的错误信息
      if (response.status === 404) {
        throw new Error(`接口不存在 (${endpoint})。请检查后端服务是否已重启并包含最新路由。`);
      }
      if (response.status === 403) {
        throw new Error("权限不足，请确认您有访问此接口的权限");
      }
      if (response.status === 401) {
        throw new Error("未授权，请重新登录");
      }
      throw new Error(`请求失败 (${response.status})`);
    }

    return data;
  }

  private async requestForm<T>(
    endpoint: string,
    formData: FormData,
    method: "POST" | "PUT" = "POST"
  ): Promise<ApiResponse<T>> {
    const headers: Record<string, string> = {};

    const url = `${API_BASE}/api/v1${endpoint}`;

    let response: Response;
    try {
      response = await fetch(url, {
        method,
        headers,
        body: formData,
        credentials: "include",
      });
    } catch (error) {
      console.error("Network error:", error);
      throw new Error("网络连接失败，请检查后端服务是否启动");
    }

    let data: ApiResponse<T>;
    try {
      data = await response.json();
    } catch {
      throw new Error(`服务器响应解析失败 (${response.status})`);
    }

    if (!response.ok) {
      throw new Error(data?.message || `请求失败 (${response.status})`);
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
    return this.request<SystemInfo>("/system/info");
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
    return this.request<{ code: string; confirmed: boolean; expires_in: number }>(
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
    }>(`/system/emby-urls`);
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

  async useCode(regCode: string, options?: { embyUsername?: string; embyPassword?: string }) {
    const payload: Record<string, string> = { reg_code: regCode };
    if (options?.embyUsername) {
      payload.emby_username = options.embyUsername;
    }
    if (typeof options?.embyPassword === "string") {
      payload.emby_password = options.embyPassword;
    }

    return this.request<{
      emby_password?: string;
      expire_status: string;
      expired_at: string | number;
      role: number;
      role_name: string;
    }>("/users/me/use-code", {
      method: "POST",
      body: JSON.stringify(payload),
    });
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
    return this.request(`/users/me/devices/${deviceId}`, {
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

  async renewUser(uid: number, days: number) {
    return this.request(`/admin/users/${uid}/renew`, {
      method: "POST",
      body: JSON.stringify({ days }),
    });
  }

  async resetPassword(uid: number) {
    return this.request<{ new_password: string }>(`/admin/users/${uid}/reset-password`, {
      method: "POST",
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

  async getSystemStats() {
    return this.request<SystemStats>("/system/admin/stats");
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

  async getAllApis() {
    return this.request<{ apis: Array<{ method: string; path: string; endpoint: string; full_path: string }>; total: number }>("/system/admin/apis");
  }

  // ==================== 定时任务管理 ====================

  async listSchedulerJobs() {
    return this.request<{ jobs: SchedulerJobItem[] }>(`/admin/scheduler/jobs`);
  }

  async triggerSchedulerJob(jobId: string) {
    return this.request<{ job_id: string; last_run: SchedulerJobRun | null }>(
      `/admin/scheduler/jobs/${encodeURIComponent(jobId)}/run`,
      { method: "POST" },
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
        config.lightBgImage = this.normalizeCssUrlValue(config.lightBgImage);
        config.darkBgImage = this.normalizeCssUrlValue(config.darkBgImage);
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
    return this.request<{ background: string }>('/users/me/background', {
      method: 'PUT',
      body: JSON.stringify(payload),
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
    if (res.success && res.data?.url) {
      res.data.url = this.toAbsoluteAssetUrl(res.data.url) || res.data.url;
    }
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

  async getRegcodes(page = 1) {
    return this.request<{ regcodes: Regcode[]; total: number }>(
      `/admin/regcodes?page=${page}`
    );
  }

  async createRegcode(data: CreateRegcodeData) {
    return this.request<{ codes: string[]; count: number }>("/admin/regcodes", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteRegcode(code: string) {
    return this.request(`/admin/regcodes/${code}`, {
      method: "DELETE",
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

  async updateMediaRequest(id: number, status: string, note?: string) {
    const normalizedStatus = this.normalizeRequestStatus(status, "admin");
    const normalizedNote = (note || "").trim().slice(0, 1000);
    return this.request(`/admin/media-requests/${id}`, {
      method: "PUT",
      body: JSON.stringify({ status: normalizedStatus, note: normalizedNote }),
    });
  }

  async deleteMediaRequest(id: number) {
    return this.request(`/media/request/${id}`, {
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

  // 扩展用户删除：支持级联层级
  async deleteUserCascade(uid: number, options?: { deleteEmby?: boolean; cascadeDepth?: number }) {
    const deleteEmby = options?.deleteEmby ?? true;
    const cascadeDepth = Math.max(1, options?.cascadeDepth ?? 1);
    return this.request<{ deleted: number[]; failed: Array<{ uid: number; reason: string }>; cascade_depth: number }>(
      `/admin/users/${uid}`,
      {
        method: "DELETE",
        body: JSON.stringify({ delete_emby: deleteEmby, cascade_depth: cascadeDepth }),
      },
    );
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
  avatar?: string;
  bgm_mode: boolean;
  created_at?: string | number;
  register_time?: number;
  is_pending?: boolean;  // 是否待激活
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
  media_id: number;
  status: string; // UNHANDLED, ACCEPTED, REJECTED, COMPLETED
  timestamp: number;
  title: string;
  media_type: string;
  season?: number;
  require_key?: string;
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
  registration_target?: "system" | "emby";
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
  emby_direct_register_days: number;
}

export interface EmbyRegisterStatus {
  request_id: string;
  status: "queued" | "processing" | "success" | "failed";
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

export interface Regcode {
  code: string;
  type: number;
  type_name: string;
  days: number;
  validity_time?: number; // 注册码有效期（小时），-1 表示永久
  use_count?: number;
  use_count_limit?: number;
  active?: boolean;
  used: boolean;
  used_by?: number;
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
}

export interface ConfigFieldOption {
  label: string;
  value: number | string;
}

export interface ConfigField {
  key: string;
  label: string;
  type: 'string' | 'int' | 'float' | 'bool' | 'secret' | 'list' | 'select';
  description: string;
  value: unknown;
  options?: ConfigFieldOption[];
}

export interface ConfigSection {
  key: string;
  title: string;
  description: string;
  fields: ConfigField[];
}

export interface ConfigSchema {
  sections: ConfigSection[];
}


export interface SchedulerJobRun {
  id?: number;
  job_id?: string;
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
  | { type: "interval"; seconds: number };

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

export interface InviteMyStatus {
  enabled: boolean;
  is_root: boolean;
  parent: { uid: number; username: string } | null;
  children: Array<{ uid: number; username: string; active: boolean; has_emby: boolean }>;
  depth: number;
  max_depth: number;
  can_invite: boolean;
  invite_block_reason?: string;
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

