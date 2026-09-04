import { fail, redirect } from "@sveltejs/kit";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { Actions, PageServerLoad } from "./$types";
import type { BangumiCollectionPage } from "$lib/types";

type FormState = { action?: string; error?: string };

const validTypes = new Set([1, 2, 3, 4, 5]);

function positiveInt(value: string | null, fallback: number, max: number): number {
  const parsed = Number(value || "");
  return Number.isSafeInteger(parsed) && parsed > 0 ? Math.min(parsed, max) : fallback;
}

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

async function updateCollection(event: Parameters<NonNullable<Actions["update"]>>[0]) {
  const form = await event.request.formData();
  const subjectID = text(form, "subject_id");
  const type = Number(text(form, "type"));
  const rate = Number(text(form, "rate"));
  const episode = Number(text(form, "ep_status"));
  const page = positiveInt(text(form, "page"), 1, 1000000);
  const perPage = positiveInt(text(form, "per_page"), 24, 100);
  if (!/^\d+$/.test(subjectID) || Number(subjectID) <= 0 || !validTypes.has(type) || !Number.isInteger(rate) || rate < 0 || rate > 10 || (type === 3 && (!Number.isInteger(episode) || episode < 0))) {
    return fail(400, { action: "update", error: t.bangumiCollectionUpdateFailed } satisfies FormState);
  }
  const payload: Record<string, unknown> = { type, rate };
  if (type === 3) payload.ep_status = episode;
  const result = await apiJSONWithResponse(event, `/api/v1/bangumi/collections/${encodeURIComponent(subjectID)}`, {
    method: "PATCH",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload)
  });
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return fail(response?.status || 503, { action: "update", error: envelope?.message || t.bangumiCollectionUpdateFailed } satisfies FormState);
  }
  throw redirect(303, `/bangumi/collections/${type}?page=${page}&per_page=${perPage}&result=updated`);
}

export const load: PageServerLoad = async (event) => {
  const type = Number(event.params.type || 3);
  if (!validTypes.has(type)) {
    return { collectionType: 3, pageData: null, loadError: t.bangumiCollectionLoadFailed, filters: { tag: "", sort: "default" }, result: "" };
  }
  const page = positiveInt(event.url.searchParams.get("page"), 1, 1000000);
  const perPage = positiveInt(event.url.searchParams.get("per_page"), 24, 100);
  const offset = (page - 1) * perPage;
  const query = new URLSearchParams({ type: String(type), limit: String(perPage), offset: String(offset) });
  if (["1", "true"].includes(event.url.searchParams.get("refresh") || "")) query.set("refresh", "1");
  const result = await apiJSON<BangumiCollectionPage>(event, `/api/v1/bangumi/collections?${query.toString()}`, { cache: "no-store" });
  return {
    collectionType: type,
    pageData: result?.success ? result.data || null : null,
    loadError: result?.success ? null : t.bangumiCollectionLoadFailed,
    filters: {
      tag: event.url.searchParams.get("tag")?.trim() || "",
      sort: event.url.searchParams.get("sort") || "default"
    },
    result: event.url.searchParams.get("result") || "",
    page,
    perPage
  };
};

export const actions: Actions = { update: updateCollection };
