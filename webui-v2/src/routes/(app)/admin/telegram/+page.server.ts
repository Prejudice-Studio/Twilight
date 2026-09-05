import { fail, redirect } from "@sveltejs/kit";
import type { Actions, RequestEvent } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type {
  AdminTelegramPageData,
  ApiEnvelope,
  ConfigField,
  ConfigSchema,
  ConfigSection,
  TelegramBotRuntime,
  TelegramBotTestResult,
  TelegramCommandCatalog,
  TelegramRosterStats
} from "$lib/types";

type FormState = {
  action?: "save" | "botTest";
  error?: string;
  result?: {
    results: TelegramBotTestResult[];
    runtime: TelegramBotRuntime | null;
  };
};

const maxSettingsBytes = 256 * 1024;
const maxListItems = 100;
const maxListItemBytes = 512;
const maxCommandReplyBytes = 16 * 1024;
const telegramParseModes = new Set(["", "HTML", "Markdown", "MarkdownV2"]);

const editableTelegramFields = new Set([
  "admin_id",
  "group_id",
  "channel_id",
  "force_bind_group",
  "force_bind_channel",
  "enable_tg_panel",
  "group_user_panel_template",
  "require_group_membership",
  "ban_on_leave",
  "auto_enable_rejoined",
  "group_check_concurrency",
  "group_action_concurrency",
  "bot_start_text",
  "bot_group_start_text",
  "bot_start_title",
  "bot_start_intro",
  "bot_bind_prompt_text",
  "bot_help_text",
  "bot_admin_help_text",
  "bot_help_header",
  "bot_help_footer",
  "bot_about",
  "parse_mode",
  "disabled_commands",
  "bot_custom_commands"
]);

const booleanFields = new Set([
  "force_bind_group",
  "force_bind_channel",
  "enable_tg_panel",
  "require_group_membership",
  "ban_on_leave",
  "auto_enable_rejoined"
]);

const listFields = new Set(["admin_id", "group_id", "channel_id", "disabled_commands"]);
const numericFields = new Set(["group_check_concurrency", "group_action_concurrency"]);
const textFields = new Set([
  "group_user_panel_template",
  "bot_start_text",
  "bot_group_start_text",
  "bot_start_title",
  "bot_start_intro",
  "bot_bind_prompt_text",
  "bot_help_text",
  "bot_admin_help_text",
  "bot_help_header",
  "bot_help_footer",
  "bot_about",
  "parse_mode"
]);

function bytes(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function cloneField(field: ConfigField): ConfigField {
  return { ...field, value: Array.isArray(field.value) ? [...field.value] : field.value };
}

function telegramSection(schema: ConfigSchema | null): ConfigSection | null {
  const section = schema?.sections.find((item) => item.key === "Telegram");
  if (!section) return null;
  return {
    ...section,
    fields: section.fields.filter((field) => editableTelegramFields.has(field.key)).map(cloneField)
  };
}

function readEnvelope<T>(result: PromiseSettledResult<ApiEnvelope<T>>): T | null {
  return result.status === "fulfilled" && result.value.success ? result.value.data || null : null;
}

function normalizeList(field: string, value: unknown): unknown[] | null {
  if (!Array.isArray(value) || value.length > maxListItems) return null;
  const result: unknown[] = [];
  for (const item of value) {
    if (typeof item !== "string" && typeof item !== "number") return null;
    const text = String(item).trim();
    if (bytes(text) > maxListItemBytes) return null;
    if (!text) continue;
    if (field === "admin_id") {
      if (!/^-?\d+$/.test(text)) return null;
      const id = Number(text);
      if (!Number.isSafeInteger(id) || id === 0) return null;
      result.push(id);
    } else {
      result.push(text);
    }
  }
  return result;
}

function normalizeCommands(value: unknown): Array<{ command: string; reply: string }> | null {
  if (!Array.isArray(value) || value.length > maxListItems) return null;
  const result: Array<{ command: string; reply: string }> = [];
  const seen = new Set<string>();
  for (const item of value) {
    const row = asRecord(item);
    if (!row || typeof row.command !== "string" || typeof row.reply !== "string") return null;
    if (bytes(row.command) > 64 || bytes(row.reply) > maxCommandReplyBytes) return null;
    const command = row.command.trim().replace(/^\/+/, "").toLowerCase();
    if (!/^[a-z0-9_]{1,32}$/.test(command) || seen.has(command)) return null;
    seen.add(command);
    result.push({ command: `/${command}`, reply: row.reply });
  }
  return result;
}

function parseSettings(form: FormData): { value?: Record<string, unknown>; error?: string } {
  const raw = form.get("settings");
  if (typeof raw !== "string" || !raw || bytes(raw) > maxSettingsBytes) return { error: "Telegram 配置内容无效" };
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { error: "Telegram 配置内容无效" };
  }
  const record = asRecord(parsed);
  if (!record) return { error: "Telegram 配置内容无效" };
  const value: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(record)) {
    if (!editableTelegramFields.has(key)) return { error: "Telegram 配置字段无效" };
    if (booleanFields.has(key)) {
      if (typeof item !== "boolean") return { error: "Telegram 开关值无效" };
      value[key] = item;
      continue;
    }
    if (listFields.has(key)) {
      const normalized = normalizeList(key, item);
      if (!normalized) return { error: "Telegram 列表配置无效" };
      value[key] = normalized;
      continue;
    }
    if (key === "bot_custom_commands") {
      const normalized = normalizeCommands(item);
      if (!normalized) return { error: "Telegram 自定义指令无效" };
      value[key] = normalized;
      continue;
    }
    if (numericFields.has(key)) {
      if (typeof item !== "number" || !Number.isSafeInteger(item) || item < 1 || item > 100) return { error: "Telegram 并发值无效" };
      value[key] = item;
      continue;
    }
    if (textFields.has(key)) {
      if (typeof item !== "string" || bytes(item) > maxCommandReplyBytes) return { error: "Telegram 文案长度无效" };
      if (key === "parse_mode" && !telegramParseModes.has(item)) return { error: "Telegram 消息解析模式无效" };
      value[key] = item;
      continue;
    }
  }
  return { value };
}

function safeBotResult(value: unknown): { results: TelegramBotTestResult[]; runtime: TelegramBotRuntime | null } {
  const record = asRecord(value);
  const rawResults = Array.isArray(record?.results) ? record.results : [];
  const results: TelegramBotTestResult[] = rawResults.slice(0, 20).flatMap((item) => {
    const row = asRecord(item);
    if (!row || typeof row.target !== "string") return [];
    const result: TelegramBotTestResult = {
      target: row.target.slice(0, 128),
      success: row.success === true
    };
    if (typeof row.username === "string") result.username = row.username.slice(0, 128);
    if (typeof row.title === "string") result.title = row.title.slice(0, 256);
    if (typeof row.bot_status === "string") result.bot_status = row.bot_status.slice(0, 64);
    if (typeof row.bot_id === "number" && Number.isSafeInteger(row.bot_id)) result.bot_id = row.bot_id;
    if (!result.success) result.error = "连接测试失败，请检查 Telegram 配置";
    return [result];
  });
  const runtimeValue = asRecord(record?.runtime);
  const runtime: TelegramBotRuntime | null = runtimeValue ? {
    polling: runtimeValue.polling === true,
    last_ok_at: typeof runtimeValue.last_ok_at === "number" ? runtimeValue.last_ok_at : null,
    last_error_at: typeof runtimeValue.last_error_at === "number" ? runtimeValue.last_error_at : null
  } : null;
  return { results, runtime };
}

async function saveTelegramSettings(event: RequestEvent, settings: Record<string, unknown>) {
  return apiJSONWithResponse(event, "/api/v1/system/admin/config/schema", {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ sections: { Telegram: settings } })
  });
}

export const load: PageServerLoad = async (event): Promise<AdminTelegramPageData> => {
  const results = await Promise.allSettled([
    apiJSON<ConfigSchema>(event, "/api/v1/system/admin/config/schema", { cache: "no-store" }),
    apiJSON<TelegramCommandCatalog>(event, "/api/v1/admin/telegram/commands/catalog", { cache: "no-store" }),
    apiJSON<TelegramRosterStats>(event, "/api/v1/admin/telegram/roster/stats", { cache: "no-store" })
  ]);
  const schema = readEnvelope(results[0] as PromiseSettledResult<ApiEnvelope<ConfigSchema>>);
  const commands = readEnvelope(results[1] as PromiseSettledResult<ApiEnvelope<TelegramCommandCatalog>>);
  const roster = readEnvelope(results[2] as PromiseSettledResult<ApiEnvelope<TelegramRosterStats>>);
  const errors: string[] = [];
  if (!schema) errors.push("Telegram 配置暂时无法读取");
  if (!commands) errors.push("Telegram 指令目录暂时无法读取");
  if (!roster) errors.push("Telegram 花名册摘要暂时无法读取");
  return {
    telegram: telegramSection(schema),
    commands,
    roster,
    errors,
    notice: event.url.searchParams.get("notice") || ""
  };
};

export const actions: Actions = {
  save: async (event) => {
    const parsed = parseSettings(await event.request.formData());
    if (parsed.error || !parsed.value) return fail(400, { action: "save", error: parsed.error || "Telegram 配置无效" } satisfies FormState);
    const result = await saveTelegramSettings(event, parsed.value);
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 503, { action: "save", error: "Telegram 配置保存失败，请稍后重试" } satisfies FormState);
    throw redirect(303, "/admin/telegram?notice=saved");
  },

  botTest: async (event) => {
    const result = await apiJSONWithResponse(event, "/api/v1/system/admin/bot/test", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{}"
    });
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 503, { action: "botTest", error: "Telegram 测试失败，请稍后重试" } satisfies FormState);
    return { action: "botTest", result: safeBotResult(result.envelope.data) } satisfies FormState;
  }
};
