/** Resolve the public background resource without duplicating its API prefix. */
export function resolveAuthBackgroundUrl(raw?: string | null, apiBase = ""): string | undefined {
  const value = raw?.trim();
  if (!value || /[\\\u0000-\u001f\u007f]/.test(value)) return undefined;
  const internal = value.match(/^(?:\/api\/v[12])?\/system\/auth-background([?#].*)?$/);
  if (internal) {
    return `${apiBase.replace(/\/$/, "")}/api/v2/system/auth-background${internal[1] || ""}`;
  }
  try {
    const url = new URL(value);
    if (!/^https?:$/.test(url.protocol) || !url.hostname || url.username || url.password) return undefined;
    return url.href;
  } catch {
    return undefined;
  }
}
