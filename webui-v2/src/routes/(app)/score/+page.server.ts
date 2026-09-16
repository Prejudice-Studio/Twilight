import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { SigninPageData } from "$lib/types";

type FormState = { action?: string; error?: string };

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

async function mutate(
  event: Parameters<NonNullable<Actions["signin"]>>[0],
  path: string,
  method: "POST" | "PUT",
  body: Record<string, unknown>,
  action: string,
  fallback: string
): Promise<never> {
  const result = await apiJSONWithResponse(event, path, {
    method,
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body)
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, {
      action,
      error: envelope?.message || fallback
    } satisfies FormState) as never;
  }
  throw redirect(303, `/score?result=${encodeURIComponent(action)}`);
}

export const load: PageServerLoad = async (event) => {
  const result = await apiJSON<SigninPageData>(event, "/api/v2/signin/summary", { cache: "no-store" });
  return {
    payload: result?.success ? result.data || null : null,
    loadError: result?.success ? null : "签到信息暂时无法读取，请刷新后重试",
    result: event.url.searchParams.get("result") || ""
  };
};

export const actions: Actions = {
  signin: async (event) => mutate(event, "/api/v2/signin", "POST", {}, "signin", "签到失败，请稍后重试"),
  renew: async (event) => mutate(event, "/api/v2/signin/renew", "POST", {}, "renew", "积分续期失败，请稍后重试"),
  autoRenewal: async (event) => {
    const form = await event.request.formData();
    const enabled = text(form, "enabled") === "true";
    return mutate(event, "/api/v2/signin/preferences", "PUT", { signin_auto_renewal: enabled }, "auto-renewal", "自动续期设置失败，请稍后重试");
  }
};
