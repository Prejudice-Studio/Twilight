const avatarPattern = /^\/api\/v1\/users\/assets\/avatar\/[a-f0-9]{16}\.(?:jpg|png|gif|webp|bmp)$/i;

// User-provided historical values are not trusted render inputs. V2 only renders
// the authenticated asset route that the Go API generated for the current user.
export function safeAvatarPath(value: unknown): string {
  return typeof value === "string" && avatarPattern.test(value) ? value : "";
}
