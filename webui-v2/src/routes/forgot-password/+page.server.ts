import { fail } from "@sveltejs/kit";
import type { Actions, RequestEvent } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { SystemInfo } from "$lib/types";

type ForgotForm = {
  mode?: "email-reset" | "email-done" | "emby-result";
  email?: string;
  emby_username?: string;
  resend_after?: number;
  emby_result?: { username: string; new_password: string };
  notice?: string;
  error?: string;
};

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

function genericFailure(): ReturnType<typeof fail<ForgotForm>> {
  return fail(400, { error: "操作失败，请稍后重试" });
}

async function post<T>(event: RequestEvent, path: string, payload: Record<string, unknown>) {
  return apiJSONWithResponse<T>(event, path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload)
  });
}

export const load: PageServerLoad = async (event) => {
  const response = await apiJSON<SystemInfo>(event, "/api/v1/system/info", { cache: "no-store" });
  const features = response?.success ? response.data?.features || {} : {};
  return {
    forgotPasswordEnabled: Boolean(features.forgot_password_enabled),
    embyAvailable: Boolean(features.forgot_password_emby_enabled),
    emailAvailable: Boolean(features.email_enabled && features.forgot_password_email_enabled)
  };
};

export const actions: Actions = {
  emby: async (event) => {
    const form = await event.request.formData();
    const username = text(form, "emby_username");
    const password = String(form.get("emby_password") ?? "");
    if (!username || !password) return fail(400, { emby_username: username, error: "请填写 Emby 用户名和密码" } satisfies ForgotForm);

    const result = await post<{ username: string; new_password: string }>(event, "/api/v1/auth/forgot-password/emby", {
      emby_username: username,
      emby_password: password
    });
    if (!result?.response.ok || !result.envelope?.success || !result.envelope.data?.new_password) {
      return fail(400, { emby_username: username, error: "操作失败，请检查账号信息后重试" } satisfies ForgotForm);
    }
    return {
      mode: "emby-result",
      emby_result: result.envelope.data,
      notice: "密码已重置。临时密码只显示这一次，请立即复制。"
    } satisfies ForgotForm;
  },

  requestEmail: async (event) => {
    const form = await event.request.formData();
    const email = text(form, "email");
    if (!email) return fail(400, { email, error: "请输入邮箱地址" } satisfies ForgotForm);
    const result = await post<{ resend_after?: number }>(event, "/api/v1/auth/password/email/request", { email });
    if (!result?.response.ok || !result.envelope?.success) return fail(400, { email, error: "操作失败，请稍后重试" } satisfies ForgotForm);
    return {
      mode: "email-reset",
      email,
      resend_after: result.envelope.data?.resend_after || 60,
      notice: "如果该邮箱已验证，验证码已经发送，请查收邮件。"
    } satisfies ForgotForm;
  },

  resetEmail: async (event) => {
    const form = await event.request.formData();
    const email = text(form, "email");
    const code = text(form, "code");
    const password = String(form.get("new_password") ?? "");
    const confirm = String(form.get("confirm_password") ?? "");
    if (!email || !code || !password) return fail(400, { email, mode: "email-reset", error: "请填写邮箱、验证码和新密码" } satisfies ForgotForm);
    if (password !== confirm) return fail(400, { email, mode: "email-reset", error: "两次输入的新密码不一致" } satisfies ForgotForm);
    if (password.length < 12) return fail(400, { email, mode: "email-reset", error: "新密码至少需要 12 位" } satisfies ForgotForm);

    const result = await post<{ username: string }>(event, "/api/v1/auth/password/email/reset", {
      email,
      code,
      new_password: password
    });
    if (!result?.response.ok || !result.envelope?.success) return fail(400, { email, mode: "email-reset", error: "操作失败，请检查验证码和密码后重试" } satisfies ForgotForm);
    return { mode: "email-done", notice: "密码已重置，请使用新密码登录。" } satisfies ForgotForm;
  }
};
