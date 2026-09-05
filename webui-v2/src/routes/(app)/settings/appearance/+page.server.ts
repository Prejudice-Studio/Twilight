import { fail, redirect } from "@sveltejs/kit";
import type { Actions, PageServerLoad, RequestEvent } from "./$types";
import { apiJSONWithResponse, copySetCookies } from "$lib/server/api";
import { t } from "$lib/i18n";
import type { BackgroundConfig } from "$lib/types";

type FormState = { action?: string; error?: string; success?: string };

const maxBackgroundBytes = 5 * 1024 * 1024;
const maxAvatarBytes = 2 * 1024 * 1024;
const allowedImageTypes = new Set(["image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp"]);
const gradientPattern = /^(?:linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|repeating-radial-gradient)\s*\(/i;
const assetPattern = /^url\(\s*["']?\/?api\/v1\/users\/assets\/background\/[a-f0-9]{16}\.(?:jpg|png|gif|webp|bmp)["']?\s*\)$/i;

function text(form: FormData, name: string, max = 2000): string {
  const value = form.get(name);
  return typeof value === "string" ? value.trim().slice(0, max) : "";
}

function boolean(form: FormData, name: string): boolean {
  return form.get(name) === "true";
}

function number(form: FormData, name: string, fallback: number, min: number, max: number): number {
  const value = Number(text(form, name, 16));
  return Number.isFinite(value) ? Math.min(max, Math.max(min, Math.round(value))) : fallback;
}

function safeGradient(value: string): string {
  if (!value || value.length > 2000 || !gradientPattern.test(value)) return "";
  if (/[\u0000-\u001f\u007f<>;{}@]|url\s*\(/i.test(value)) return "";
  return value;
}

function safeAsset(value: string): string {
  if (!value || value.length > 180 || !assetPattern.test(value)) return "";
  return value;
}

function backgroundPayload(form: FormData): BackgroundConfig {
  return {
    lightBg: safeGradient(text(form, "lightBg")),
    darkBg: safeGradient(text(form, "darkBg")),
    lightBgImage: safeAsset(text(form, "lightBgImage", 180)),
    darkBgImage: safeAsset(text(form, "darkBgImage", 180)),
    lightFlow: boolean(form, "lightFlow"),
    darkFlow: boolean(form, "darkFlow"),
    lightBlur: number(form, "lightBlur", 0, 0, 30),
    darkBlur: number(form, "darkBlur", 0, 0, 30),
    lightOpacity: number(form, "lightOpacity", 100, 10, 100),
    darkOpacity: number(form, "darkOpacity", 100, 10, 100)
  };
}

function error(action: string, message: string): ReturnType<typeof fail<FormState>> {
  return fail(400, { action, error: message } satisfies FormState);
}

async function mutation(event: RequestEvent, path: string, init: RequestInit, action: string, success: string) {
  const result = await apiJSONWithResponse(event, path, init);
  const response = result?.response;
  const envelope = result?.envelope;
  if (!response || !envelope || !response.ok || !envelope.success) {
    return error(action, t.appearanceUploadFailed);
  }
  copySetCookies(event, response);
  throw redirect(303, `/settings/appearance?result=${encodeURIComponent(success)}`);
}

function readAppearance(value: string | null | undefined): BackgroundConfig {
  const defaults: BackgroundConfig = {
    lightBg: "", darkBg: "", lightBgImage: "", darkBgImage: "",
    lightFlow: false, darkFlow: false, lightBlur: 0, darkBlur: 0,
    lightOpacity: 100, darkOpacity: 100
  };
  if (!value || value.length > 10000) return defaults;
  try {
    const raw = JSON.parse(value) as Record<string, unknown>;
    return {
      lightBg: safeGradient(typeof raw.lightBg === "string" ? raw.lightBg : ""),
      darkBg: safeGradient(typeof raw.darkBg === "string" ? raw.darkBg : ""),
      lightBgImage: safeAsset(typeof raw.lightBgImage === "string" ? raw.lightBgImage : ""),
      darkBgImage: safeAsset(typeof raw.darkBgImage === "string" ? raw.darkBgImage : ""),
      lightFlow: raw.lightFlow === true,
      darkFlow: raw.darkFlow === true,
      lightBlur: Number.isFinite(Number(raw.lightBlur)) ? Math.min(30, Math.max(0, Number(raw.lightBlur))) : 0,
      darkBlur: Number.isFinite(Number(raw.darkBlur)) ? Math.min(30, Math.max(0, Number(raw.darkBlur))) : 0,
      lightOpacity: Number.isFinite(Number(raw.lightOpacity)) ? Math.min(100, Math.max(10, Number(raw.lightOpacity))) : 100,
      darkOpacity: Number.isFinite(Number(raw.darkOpacity)) ? Math.min(100, Math.max(10, Number(raw.darkOpacity))) : 100
    };
  } catch {
    return defaults;
  }
}

export const load: PageServerLoad = ({ locals, url }) => {
  const rawResult = url.searchParams.get("result") || "";
  const result = new Set(["background_saved", "background_reset", "background_uploaded", "avatar_uploaded", "avatar_deleted"]).has(rawResult)
    ? rawResult
    : "";
  return {
    background: readAppearance(locals.user?.background),
    avatar: locals.user?.avatar || null,
    result
  };
};

export const actions: Actions = {
  saveBackground: async (event) => {
    const form = await event.request.formData();
    const payload = backgroundPayload(form);
    if (!payload.lightBg && !payload.darkBg && !payload.lightBgImage && !payload.darkBgImage) {
      return error("saveBackground", t.appearanceBackgroundInvalid);
    }
    return mutation(event, "/api/v1/users/me/background", {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(payload)
    }, "saveBackground", "background_saved");
  },

  resetBackground: async (event) => mutation(event, "/api/v1/users/me/background", { method: "DELETE" }, "resetBackground", "background_reset"),

  uploadBackground: async (event) => {
    const form = await event.request.formData();
    const file = form.get("file");
    const type = text(form, "type", 10);
    if (typeof file === "string" || !file || typeof file.arrayBuffer !== "function") return error("uploadBackground", t.appearanceUploadRequired);
    if (!allowedImageTypes.has(file.type)) return error("uploadBackground", t.appearanceUploadTypeInvalid);
    if (file.size <= 0 || file.size > maxBackgroundBytes) return error("uploadBackground", t.appearanceUploadTooLarge);
    if (type !== "light" && type !== "dark") return error("uploadBackground", t.appearanceUploadFailed);
    const body = new FormData();
    body.set("file", file, file.name || "background");
    body.set("type", type);
    return mutation(event, "/api/v1/users/me/background/upload", { method: "POST", body }, "uploadBackground", "background_uploaded");
  },

  uploadAvatar: async (event) => {
    const form = await event.request.formData();
    const file = form.get("file");
    if (typeof file === "string" || !file || typeof file.arrayBuffer !== "function") return error("uploadAvatar", t.appearanceUploadRequired);
    if (!allowedImageTypes.has(file.type)) return error("uploadAvatar", t.appearanceUploadTypeInvalid);
    if (file.size <= 0 || file.size > maxAvatarBytes) return error("uploadAvatar", t.appearanceUploadTooLarge);
    const body = new FormData();
    body.set("file", file, file.name || "avatar");
    return mutation(event, "/api/v1/users/me/avatar/upload", { method: "POST", body }, "uploadAvatar", "avatar_uploaded");
  },

  deleteAvatar: async (event) => mutation(event, "/api/v1/users/me/avatar", { method: "DELETE" }, "deleteAvatar", "avatar_deleted")
};
