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

export interface LoginPayload {
  username: string;
  password: string;
}
