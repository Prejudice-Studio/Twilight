import { fail, redirect } from "@sveltejs/kit";
import { apiJSONWithResponse, copySetCookies } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { LoginPayload, UserInfo } from "$lib/types";

export const load: PageServerLoad = ({ locals }) => {
  if (locals.user) throw redirect(303, "/dashboard");
  return {};
};

export const actions: Actions = {
  default: async (event) => {
    const form = await event.request.formData();
    const username = String(form.get("username") || "").trim();
    const password = String(form.get("password") || "");
    if (!username || !password) return fail(400, { username, error: "请输入用户名和密码" });

    const result = await apiJSONWithResponse<UserInfo>(event, "/api/v1/auth/login", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ username, password } satisfies LoginPayload)
    });
    const response = result?.response;
    const envelope = result?.envelope;
    if (!response || !envelope || !response.ok || !envelope.success) {
      return fail(response?.status || 503, {
        username,
        error: envelope?.message || "登录失败，请检查账号信息"
      });
    }
    copySetCookies(event, response);
    throw redirect(303, "/dashboard");
  }
};
