// Frontend password strength rule. Keep this in sync with the Go backend
// password validation policy.

export interface PasswordStrengthResult {
  ok: boolean;
  message: string;
  /** 0-4 rough strength score for visual indicator */
  score: number;
}

export function validatePasswordStrength(
  password: string,
  label = "新密码"
): PasswordStrengthResult {
  if (!password) {
    return { ok: false, message: `请提供${label}`, score: 0 };
  }
  if (password.length > 128) {
    return { ok: false, message: `${label}过长，最多 128 位`, score: 0 };
  }

  const hasLower = /[a-z]/.test(password);
  const hasUpper = /[A-Z]/.test(password);
  const hasDigit = /[0-9]/.test(password);
  const hasSymbol = /[^A-Za-z0-9]/.test(password);
  const longEnough = password.length >= 8;

  const score =
    (longEnough ? 1 : 0) +
    (hasLower ? 1 : 0) +
    (hasUpper ? 1 : 0) +
    (hasDigit ? 1 : 0) +
    (hasSymbol ? 1 : 0);

  if (!longEnough) {
    return {
      ok: false,
      message: `${label}强度不足：至少 8 位，且包含大小写字母和数字`,
      score,
    };
  }
  if (!hasLower) {
    return { ok: false, message: `${label}强度不足：至少包含一个小写字母`, score };
  }
  if (!hasUpper) {
    return { ok: false, message: `${label}强度不足：至少包含一个大写字母`, score };
  }
  if (!hasDigit) {
    return { ok: false, message: `${label}强度不足：至少包含一个数字`, score };
  }

  return { ok: true, message: "强度合格", score };
}

export function passwordStrengthLabel(score: number): {
  label: string;
  className: string;
} {
  if (score <= 1) return { label: "弱", className: "text-destructive" };
  if (score === 2) return { label: "一般", className: "text-amber-500" };
  if (score === 3) return { label: "良好", className: "text-emerald-500" };
  return { label: "强", className: "text-emerald-600" };
}
