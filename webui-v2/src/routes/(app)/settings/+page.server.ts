import { fail } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse, copySetCookies } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { EmailCodeSent, UserInfo, UserSettings } from "$lib/types";

type FormResult = {
  section?: string;
  purpose?: string;
  success?: boolean;
  message?: string;
  error?: string;
  verification?: EmailCodeSent;
};

const preferenceFields = [
  "notify_on_login_telegram",
  "notify_on_login_email",
  "notify_on_ticket_telegram",
  "signin_auto_renewal",
  "password_change_email_required",
  "emby_password_email_required",
  "emby_password_old_password_required"
] as const;

function formBoolean(form: FormData, name: string): boolean {
  return form.getAll(name).some((value) => value === "true" || value === "on");
}

function formString(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

async function postJSON<T>(
  event: Parameters<NonNullable<Actions["preferences"]>>[0],
  path: string,
  section: string,
  payload: Record<string, unknown>,
  fallback: string
): Promise<FormResult | ReturnType<typeof fail>> {
  const result = await apiJSONWithResponse<T>(event, path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload)
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, {
      section,
      error: envelope?.message || fallback
    } satisfies FormResult);
  }
  copySetCookies(event, response);
  return { section, success: true, message: envelope.message || "操作成功" };
}

export const load: PageServerLoad = async (event) => {
  const result = await apiJSON<UserSettings>(event, "/api/v1/users/me/settings", { cache: "no-store" });
  return {
    settings: result?.success ? result.data || null : null,
    loadError: result?.success ? null : "设置暂时无法读取，请刷新后重试"
  };
};

export const actions: Actions = {
  preferences: async (event) => {
    const form = await event.request.formData();
    const payload: Record<string, unknown> = {};
    for (const field of preferenceFields) {
      if (form.has(field)) payload[field] = formBoolean(form, field);
    }
    return postJSON<UserInfo>(event, "/api/v1/users/me", "preferences", payload, "设置保存失败，请刷新后重试");
  },

  sendEmail: async (event) => {
    const form = await event.request.formData();
    const purpose = formString(form, "purpose") || "bind";
    const email = formString(form, "email");
    const result = await apiJSONWithResponse<EmailCodeSent>(event, "/api/v1/users/me/email/send-code", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ purpose, ...(email ? { email } : {}) })
    });
    const response = result?.response;
    const envelope = result?.envelope;
    if (!response || !envelope || !response.ok || !envelope.success) {
      return fail(response?.status || 503, {
        section: "email",
        error: envelope?.message || "邮件发送失败，请稍后重试"
      } satisfies FormResult);
    }
    return {
      section: "email",
      purpose,
      success: true,
      message: "验证码已发送，请查收邮件",
      verification: envelope.data
    } satisfies FormResult;
  },

  verifyEmail: async (event) => {
    const form = await event.request.formData();
    return postJSON<UserInfo>(event, "/api/v1/users/me/email/verify", "email", {
      verification_id: formString(form, "verification_id"),
      code: formString(form, "code")
    }, "验证码无效或已失效，请重新获取");
  },

  changePassword: async (event) => {
    const form = await event.request.formData();
    const newPassword = formString(form, "new_password");
    const confirmPassword = formString(form, "confirm_password");
    if (!newPassword || !confirmPassword) return fail(400, { section: "password", error: "请填写新密码" } satisfies FormResult);
    if (newPassword !== confirmPassword) return fail(400, { section: "password", error: "两次输入的新密码不一致" } satisfies FormResult);
    return postJSON<{ token?: string }>(event, "/api/v1/users/me/password/system", "password", {
      old_password: formString(form, "old_password"),
      new_password: newPassword,
      ...(formString(form, "verification_id") && formString(form, "email_code")
        ? { verification_id: formString(form, "verification_id"), email_code: formString(form, "email_code") }
        : {})
    }, "密码修改失败，请检查输入后重试");
  },

  changeEmbyPassword: async (event) => {
    const form = await event.request.formData();
    const newPassword = formString(form, "new_password");
    const confirmPassword = formString(form, "confirm_password");
    if (!newPassword || !confirmPassword) return fail(400, { section: "emby", error: "请填写新密码" } satisfies FormResult);
    if (newPassword !== confirmPassword) return fail(400, { section: "emby", error: "两次输入的新密码不一致" } satisfies FormResult);
    return postJSON<null>(event, "/api/v1/users/me/password/emby", "emby", {
      new_password: newPassword,
      ...(formString(form, "old_password") ? { old_password: formString(form, "old_password") } : {}),
      ...(formString(form, "verification_id") && formString(form, "email_code")
        ? { verification_id: formString(form, "verification_id"), email_code: formString(form, "email_code") }
        : {})
    }, "Emby 密码修改失败，请检查输入后重试");
  },

  bindEmby: async (event) => {
    const form = await event.request.formData();
    return postJSON<{ user?: UserInfo }>(event, "/api/v1/users/me/emby/bind", "emby", {
      emby_username: formString(form, "emby_username"),
      emby_password: formString(form, "emby_password")
    }, "Emby 绑定失败，请稍后重试");
  },

  registerEmby: async (event) => {
    const form = await event.request.formData();
    return postJSON<{ user?: UserInfo }>(event, "/api/v1/users/me/emby/register", "emby", {
      emby_username: formString(form, "emby_username"),
      emby_password: formString(form, "emby_password")
    }, "Emby 开通失败，请稍后重试");
  },

  unbindEmby: async (event) => postJSON<UserInfo>(event, "/api/v1/users/me/emby/unbind", "emby", {}, "解除 Emby 绑定失败，请稍后重试")
};
