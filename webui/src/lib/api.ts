// 后端 API 地址 - 确保设置了 NEXT_PUBLIC_API_URL 环境变量
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
  private token: string | null = null;

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
    this.token = token;
    if (typeof window !== "undefined") {
      if (token) {
        localStorage.setItem("twilight_token", token);
      } else {
        localStorage.removeItem("twilight_token");
      }
    }
  }

  getToken(): string | null {
    if (this.token) return this.token;
    if (typeof window !== "undefined") {
      this.token = localStorage.getItem("twilight_token");
    }
    return this.token;
  }

  hasToken(): boolean {
    return !!this.getToken();
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<ApiResponse<T>> {
    const token = this.getToken();
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...((options.headers as Record<string, string>) || {}),
    };

    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }

    const url = `${API_BASE}/api/v1${endpoint}`;
    
    let response: Response;
    try {
      response = await fetch(url, {
        ...options,
        headers,
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
    const token = this.getToken();
    const headers: Record<string, string> = {};

    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }

    const url = `${API_BASE}/api/v1${endpoint}`;

    let response: Response;
    try {
      response = await fetch(url, {
        method,
        headers,
        body: formData,
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
    const res = await this.request<{ token: string; user: Partial<UserInfo> }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    if (res.success && res.data?.user?.avatar) {
      res.data.user.avatar = this.toAbsoluteAssetUrl(res.data.user.avatar) || undefined;
    }
    if (res.success && res.data?.token) {
      this.setToken(res.data.token);
    }
    return res;
  }

  async register(data: RegisterData) {
    return this.request<{ uid: number }>("/users/register", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async logout() {
    this.setToken(null);
  }

  // User
  async getMe() {
    const res = await this.request<UserInfo>("/users/me");
    if (res.success && res.data?.avatar) {
      res.data.avatar = this.toAbsoluteAssetUrl(res.data.avatar) || undefined;
    }
    return res;
  }

  async updateMe(data: { email?: string; username?: string }) {
    return this.request<UserInfo>("/users/me", {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async getMySettings() {
    return this.request<UserSettings>("/users/me/settings");
  }

  async updateAutoRenew(enabled: boolean) {
    return this.request("/users/me/auto-renew", {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    });
  }

  async getTelegramStatus() {
    return this.request<TelegramStatus>("/users/me/telegram");
  }

  async bindTelegram(telegramId: number) {
    return this.request("/users/me/telegram/bind", {
      method: "POST",
      body: JSON.stringify({ telegram_id: telegramId }),
    });
  }

  async unbindTelegram() {
    return this.request("/users/me/telegram/unbind", {
      method: "POST",
    });
  }

  async changeTelegram(newTelegramId: number) {
    return this.request("/users/me/telegram/change", {
      method: "POST",
      body: JSON.stringify({ new_telegram_id: newTelegramId }),
    });
  }

  async getNsfwStatus() {
    return this.request<NsfwStatus>("/users/me/nsfw");
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

  async toggleNsfw(enable: boolean) {
    return this.request("/users/me/nsfw", {
      method: "PUT",
      body: JSON.stringify({ enable }),
    });
  }

  // Score
  async getScoreInfo() {
    return this.request<ScoreInfo>("/score/info");
  }

  async checkin() {
    return this.request<CheckinResult>("/score/checkin", {
      method: "POST",
    });
  }

  async getScoreHistory(page = 1, perPage = 20) {
    return this.request<{ records: ScoreRecord[]; total: number }>(
      `/score/history?page=${page}&per_page=${perPage}`
    );
  }

  async transferScore(toUid: number, amount: number, note?: string) {
    return this.request("/score/transfer", {
      method: "POST",
      body: JSON.stringify({ to_uid: toUid, amount, note }),
    });
  }

  async getScoreRanking(limit = 10) {
    return this.request<{ ranking: ScoreRankingItem[]; score_name: string }>(
      `/score/ranking?limit=${limit}`
    );
  }

  async getScoreConfig() {
    return this.request<ScoreConfig>("/score/config");
  }

  async createRedPacket(amount: number, count: number, type: number) {
    return this.request<{ rp_key: string; amount: number; count: number; type: string }>(
      "/score/redpacket",
      {
        method: "POST",
        body: JSON.stringify({ amount, count, type }),
      }
    );
  }

  async grabRedPacket(rpKey: string) {
    return this.request<{ amount: number; total_score: number }>(
      `/score/redpacket/${rpKey}/grab`,
      {
        method: "POST",
      }
    );
  }

  async withdrawRedPacket(rpKey: string) {
    return this.request(`/score/redpacket/${rpKey}/withdraw`, {
      method: "POST",
    });
  }

  async renewWithScore(days: number) {
    return this.request("/users/me/renew-by-score", {
      method: "POST",
      body: JSON.stringify({ days }),
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
    return this.request<MediaRequest[]>(
      "/media/request/my",
      { signal }
    );
  }

  // Emby
  async getEmbyInfo() {
    return this.request<EmbyInfo>("/emby/info");
  }

  async getMySessions() {
    return this.request<EmbySession[]>("/emby/sessions/my");
  }

  async getMyDevices() {
    return this.request<EmbyDevice[]>("/emby/devices/my");
  }

  async removeDevice(deviceId: string) {
    return this.request(`/emby/devices/${deviceId}`, {
      method: "DELETE",
    });
  }

  // Stats
  async getMyStats() {
    return this.request<PlaybackStats>("/stats/playback/my");
  }

  async getTopMedia(period = "week", limit = 10) {
    return this.request<{ ranking: TopMediaItem[]; period: string }>(
      `/stats/ranking/media?period=${period}&limit=${limit}`
    );
  }

  // Admin
  async getUsers(params: AdminUserListParams = {}, signal?: AbortSignal) {
    const query = new URLSearchParams();
    if (params.page) query.set("page", String(params.page));
    if (params.per_page) query.set("per_page", String(params.per_page));
    if (params.role !== undefined) query.set("role", String(params.role));
    if (params.active !== undefined) query.set("active", String(params.active));
    if (params.search) query.set("search", params.search);
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

  async setUserNsfwPermission(uid: number, grant: boolean) {
    return this.request(`/admin/users/${uid}/nsfw`, {
      method: "PUT",
      body: JSON.stringify({ grant }),
    });
  }

  async updateMyAdminInfo(data: { score?: number }) {
    return this.request("/admin/me/update", {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteUser(uid: number) {
    return this.request(`/admin/users/${uid}`, {
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

  async getAllApis() {
    return this.request<{ apis: Array<{ method: string; path: string; endpoint: string; full_path: string }>; total: number }>("/system/admin/apis");
  }

  async getEmbyLibraries() {
    return this.request<Array<{ id: string; name: string; type: string; is_nsfw: boolean }>>("/system/admin/emby/libraries");
  }

  async updateNsfwLibrary(libraryId: string) {
    return this.request<{ nsfw_library_id: string }>("/system/admin/emby/nsfw", {
      method: "PUT",
      body: JSON.stringify({ library_id: libraryId }),
    });
  }

  async getApiKeyStatus() {
    return this.request<{ enabled: boolean; apikey: string | null }>("/auth/apikey");
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
    allow_checkin: boolean;
    allow_transfer: boolean;
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
      allow_checkin: boolean;
      allow_transfer: boolean;
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
    return this.request<{ requests: MediaRequest[]; total: number }>(
      `/admin/media-requests?${query}`,
      { signal }
    );
  }

  async updateMediaRequest(id: number, status: string, note?: string) {
    return this.request(`/admin/media-requests/${id}`, {
      method: "PUT",
      body: JSON.stringify({ status, note }),
    });
  }

  async deleteMediaRequest(id: number) {
    return this.request(`/media/request/${id}`, {
      method: "DELETE",
    });
  }
}

export const api = new ApiClient();

// Types
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
  expired_at?: string | number;  // 可能是时间戳或字符串，-1 表示永久
  emby_id?: string;
  avatar?: string;
  score: number;
  auto_renew: boolean;
  bgm_mode: boolean;
  nsfw: boolean | {  // 可能是布尔值（列表）或对象（详情）
    enabled: boolean;
    has_permission: boolean;
    nsfw_library_id?: string;
  };
  created_at: string;
  is_pending?: boolean;  // 是否待激活
}

export interface ApiKeyItem {
  id: number;
  name: string;
  key: string;
  key_full: string;
  enabled: boolean;
  allow_checkin: boolean;
  allow_transfer: boolean;
  allow_query: boolean;
  rate_limit: number;
  request_count: number;
  last_used: number | null;
  created_at: number;
  expired_at: number | null;
}

export interface UserSettings {
  auto_renew: boolean;
  nsfw_enabled: boolean;
  nsfw_can_toggle: boolean;
  bgm_mode: boolean;
  api_key_enabled: boolean;
  telegram: {
    bound: boolean;
    force_bind: boolean;
    can_unbind: boolean;
    can_change: boolean;
  };
  system_config: {
    auto_renew_enabled: boolean;
    auto_renew_cost: number;
    auto_renew_days: number;
    device_limit_enabled: boolean;
    max_devices: number;
    max_streams: number;
    nsfw_library_configured: boolean;
  };
}

export interface TelegramStatus {
  bound: boolean;
  telegram_id?: string;
  telegram_id_full?: number;
  telegram_username?: string;  // Telegram 用户名
  force_bind: boolean;
  can_unbind: boolean;
  can_change: boolean;
}

export interface NsfwStatus {
  enabled: boolean;
  has_permission: boolean;
  nsfw_library_id?: string;
  can_toggle: boolean;
  message: string;
}

export interface ScoreInfo {
  balance: number;
  score_name: string;
  today_checkin: boolean;
  checkin_streak: number;
  total_earned: number;
  total_spent: number;
}

export interface ScoreRankingItem {
  rank: number;
  uid: number;
  username: string;
  score: number;
  checkin_days: number;
}

export interface ScoreConfig {
  score_name: string;
  checkin: {
    base_score: number;
    streak_bonus: number;
    max_streak_bonus: number;
    random_range: [number, number];
  };
  transfer: {
    enabled: boolean;
    min_amount: number;
    max_amount: number;
    fee_rate: number;
  };
  red_packet: {
    enabled: boolean;
    min_amount: number;
    max_amount: number;
    min_count: number;
    max_count: number;
  };
  auto_renew: {
    enabled: boolean;
    days: number;
    cost: number;
  };
}

export interface CheckinResult {
  score: number;
  balance: number;
  streak: number;
  message: string;
}

export interface ScoreRecord {
  id: number;
  type: string;
  amount: number;
  balance_after: number;
  note?: string;
  related_uid?: number;
  created_at: number;  // 时间戳
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
  server_name: string;
  version: string;
  user_id?: string;
  user_name?: string;
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

export interface PlaybackStats {
  total_plays: number;
  total_time: number;
  favorite_genres?: string[];
  recent_items?: {
    name: string;
    type: string;
    played_at: string;
  }[];
}

export interface TopMediaItem {
  item_id: string;
  item_name: string;
  item_type: string;
  play_count: number;
  total_duration: number;
}

export interface RegisterData {
  telegram_id?: number;
  username: string;
  password: string;
  email?: string;
  reg_code?: string;
}

export interface AdminUserListParams {
  page?: number;
  per_page?: number;
  role?: number;
  active?: boolean;
  search?: string;
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
  score?: number;
  expired_at?: string;
}

export interface SystemStats {
  total_users: number;
  active_users: number;
  expired_users: number;
  total_score: number;
  active_regcodes: number;
  pending_requests: number;
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

