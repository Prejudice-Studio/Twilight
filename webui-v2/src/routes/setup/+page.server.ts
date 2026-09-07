import { fail, redirect, type Actions, type RequestEvent } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse, copySetCookies } from "$lib/server/api";
import type { SetupPayload, SetupResult, SetupStatus, SystemInfo } from "$lib/types";

type SetupForm = { error?: string; username?: string; email?: string };

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

function list(value: string): string[] {
  return value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean);
}

function lines(value: string): Array<{ name?: string; url: string }> {
  return list(value).map((item) => {
    const separator = item.indexOf("=");
    return separator > 0
      ? { name: item.slice(0, separator).trim(), url: item.slice(separator + 1).trim() }
      : { url: item };
  }).filter((item) => item.url);
}

function checked(form: FormData, name: string): boolean {
  return form.getAll(name).some((value) => String(value) === "true");
}

async function complete(event: RequestEvent, payload: SetupPayload) {
  return apiJSONWithResponse<SetupResult>(event, "/api/v1/setup/complete", {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-Twilight-Client": "webui",
      "X-Twilight-Intent": "complete-setup"
    },
    body: JSON.stringify(payload)
  });
}

export const load: PageServerLoad = async (event) => {
  if (event.locals.user) throw redirect(303, "/dashboard");
  const [status, system] = await Promise.all([
    apiJSON<SetupStatus>(event, "/api/v1/setup/status", { cache: "no-store" }),
    apiJSON<SystemInfo>(event, "/api/v2/system/info", { cache: "no-store" })
  ]);
  return {
    status: status?.success ? status.data || null : null,
    system: system?.success ? system.data || null : null
  };
};

export const actions: Actions = {
  default: async (event) => {
    const form = await event.request.formData();
    const username = text(form, "username");
    const email = text(form, "email");
    const password = String(form.get("password") ?? "");
    const confirm = String(form.get("confirm_password") ?? "");
    if (!username || !password) return fail(400, { username, email, error: "请填写管理员用户名和密码" } satisfies SetupForm);
    if (password !== confirm) return fail(400, { username, email, error: "两次输入的管理员密码不一致" } satisfies SetupForm);
    if (password.length < 12) return fail(400, { username, email, error: "管理员密码至少需要 12 位" } satisfies SetupForm);

    const smtpPort = Number(text(form, "smtp_port"));
    const payload: SetupPayload = {
      admin: { username, password, ...(email ? { email } : {}) },
      global: { server_name: text(form, "site_name") || "Twilight" },
      emby: {
        ...(text(form, "emby_url") ? { emby_url: text(form, "emby_url") } : {}),
        ...(String(form.get("emby_token") ?? "") ? { emby_token: String(form.get("emby_token") ?? "") } : {}),
        emby_url_list: lines(text(form, "emby_lines"))
      },
      telegram: {
        enabled: checked(form, "telegram_enabled"),
        ...(String(form.get("telegram_token") ?? "") ? { bot_token: String(form.get("telegram_token") ?? "") } : {}),
        admin_id: list(text(form, "telegram_admins"))
      },
      email: {
        enabled: checked(form, "email_enabled"),
        smtp_host: text(form, "smtp_host") || undefined,
        smtp_port: Number.isFinite(smtpPort) && smtpPort > 0 ? smtpPort : undefined,
        smtp_username: text(form, "smtp_username") || undefined,
        smtp_password: String(form.get("smtp_password") ?? "") || undefined,
        smtp_from_address: text(form, "smtp_from") || undefined,
        smtp_encryption: "starttls"
      },
      policy: {
        register_mode: checked(form, "register_open"),
        register_code_limit: checked(form, "register_code_limit"),
        allow_pending_register: checked(form, "allow_pending_register")
      }
    };

    const result = await complete(event, payload);
    if (!result?.response.ok || !result.envelope?.success) {
      return fail(result?.response.status || 503, { username, email, error: "初始化失败，请检查填写内容后重试" } satisfies SetupForm);
    }
    copySetCookies(event, result.response);
    throw redirect(303, "/admin/status");
  }
};
