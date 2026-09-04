import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { RegisterAvailability, RegisterResponse, SystemInfo } from "$lib/types";

type FormState = {
  username?: string;
  email?: string;
  reg_code?: string;
  telegram_bind_code?: string;
  error?: string;
  bind_code?: string;
  bind_expires_in?: number;
  bind_message?: string;
};

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

async function jsonAction<T>(event: Parameters<NonNullable<Actions["default"]>>[0], payload: Record<string, unknown>) {
  return apiJSONWithResponse<T>(event, "/api/v1/users/register", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload)
  });
}

export const load: PageServerLoad = async (event) => {
  if (event.locals.user) throw redirect(303, "/dashboard");
  const [availability, system] = await Promise.all([
    apiJSON<RegisterAvailability>(event, "/api/v1/users/check-available", { cache: "no-store" }),
    apiJSON<SystemInfo>(event, "/api/v1/system/info", { cache: "no-store" })
  ]);
  return {
    availability: availability?.success ? availability.data || null : null,
    system: system?.success ? system.data || null : null
  };
};

export const actions: Actions = {
  createBindCode: async (event) => {
    const response = await apiJSONWithResponse<{ bind_code: string; expires_in: number }>(event, "/api/v1/users/telegram/register/bind-code", {
      headers: {
        "X-Twilight-Client": "webui",
        "X-Twilight-Intent": "create-bind-code"
      },
      cache: "no-store"
    });
    const envelope = response?.envelope;
    if (!response?.response || !envelope?.success || !envelope.data?.bind_code) {
      return fail(response?.response.status || 503, {
        error: envelope?.message || "绑定码暂时无法生成，请稍后重试"
      } satisfies FormState);
    }
    return {
      bind_code: envelope.data.bind_code,
      bind_expires_in: envelope.data.expires_in,
      bind_message: "绑定码已生成，请在 Telegram Bot 中完成绑定后再提交注册。"
    } satisfies FormState;
  },

  default: async (event) => {
    const form = await event.request.formData();
    const username = text(form, "username");
    const email = text(form, "email");
    const password = String(form.get("password") ?? "");
    const confirmPassword = String(form.get("confirm_password") ?? "");
    const regCode = text(form, "reg_code");
    const telegramBindCode = text(form, "telegram_bind_code");
    const draft = { username, email, reg_code: regCode, telegram_bind_code: telegramBindCode };

    if (!username || !password) return fail(400, { ...draft, error: "请填写用户名和密码" } satisfies FormState);
    if (password !== confirmPassword) return fail(400, { ...draft, error: "两次输入的密码不一致" } satisfies FormState);

    const result = await jsonAction<RegisterResponse>(event, {
      username,
      password,
      ...(email ? { email } : {}),
      ...(regCode ? { reg_code: regCode } : {}),
      ...(telegramBindCode ? { telegram_bind_code: telegramBindCode } : {})
    });
    const response = result?.response;
    const envelope = result?.envelope;
    if (!response || !envelope || !response.ok || !envelope.success) {
      return fail(response?.status || 503, {
        ...draft,
        error: envelope?.message || "注册失败，请检查填写内容后重试"
      } satisfies FormState);
    }
    throw redirect(303, "/login?registered=1");
  }
};
