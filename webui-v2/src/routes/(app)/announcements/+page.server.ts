import { fail } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { Actions, PageServerLoad } from "./$types";
import type { UserAnnouncements } from "$lib/types";

type FormState = { error?: string; success?: boolean; message?: string };

export const load: PageServerLoad = async (event) => {
  const result = await apiJSON<UserAnnouncements>(event, "/api/v1/users/me/announcements", { cache: "no-store" });
  return {
    announcements: result?.success ? result.data || null : null,
    loadError: result?.success ? null : "公告暂时无法读取，请刷新后重试",
    now: Math.floor(Date.now() / 1000)
  };
};

export const actions: Actions = {
  acknowledge: async (event) => {
    const form = await event.request.formData();
    const ids = form.getAll("id")
      .map((value) => Number(value))
      .filter((value) => Number.isSafeInteger(value) && value > 0);
    if (ids.length === 0) return fail(400, { error: "没有需要确认的公告" } satisfies FormState);

    const result = await apiJSONWithResponse<{ acknowledged: number }>(event, "/api/v1/users/me/announcements/ack", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ ids: [...new Set(ids)] })
    });
    const response = result?.response;
    const envelope = result?.envelope;
    if (!response || !envelope || !response.ok || !envelope.success) {
      return fail(response?.status || 503, { error: envelope?.message || "公告确认失败，请稍后重试" } satisfies FormState);
    }
    return { success: true, message: envelope.message || "公告已确认" } satisfies FormState;
  }
};
