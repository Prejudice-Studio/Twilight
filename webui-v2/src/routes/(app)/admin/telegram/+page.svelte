<script lang="ts">
  import { t } from "$lib/i18n";
  import type {
    ConfigField,
    TelegramBotTestResult,
    TelegramCommandCatalogItem,
    TelegramRosterStats
  } from "$lib/types";
  import type { PageData } from "./$types";

  type CommandRow = { command: string; reply: string };
  type FormState = {
    action?: "save" | "botTest";
    error?: string;
    result?: { results: TelegramBotTestResult[]; runtime: { polling?: boolean; last_ok_at?: number | null; last_error_at?: number | null } | null };
  };

  let { data, form }: { data: PageData; form: FormState | null } = $props();
  let settings = $state<Record<string, unknown>>({});
  let initializedSeed = $state("");
  let focusedTarget = $state<{ type: "panel" } | { type: "reply"; index: number } | null>(null);
  let search = $state("");

  const panelPlaceholders = [
    "{server_name}", "{username}", "{uid}", "{role}", "{web_status}",
    "{telegram_username}", "{telegram_userid}", "{emby_status}", "{emby_username}",
    "{expired_at}"
  ];

  function sectionSeed(): string {
    return JSON.stringify(data.telegram?.fields.map((field) => [field.key, field.value]) || []);
  }

  function cloneValue(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(cloneValue);
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, cloneValue(item)]));
    return value;
  }

  function readInitialSettings(): Record<string, unknown> {
    return Object.fromEntries((data.telegram?.fields || []).map((field) => [field.key, cloneValue(field.value)]));
  }

  $effect(() => {
    const seed = sectionSeed();
    if (seed !== initializedSeed) {
      settings = readInitialSettings();
      initializedSeed = seed;
    }
  });

  function field(key: string): ConfigField | undefined {
    return data.telegram?.fields.find((item) => item.key === key);
  }

  function textValue(key: string): string {
    const value = settings[key];
    return typeof value === "string" ? value : value == null ? "" : String(value);
  }

  function boolValue(key: string): boolean {
    return settings[key] === true;
  }

  function numberValue(key: string, fallback: number): number {
    const value = settings[key];
    return typeof value === "number" && Number.isFinite(value) ? value : fallback;
  }

  function listValue(key: string): string[] {
    const value = settings[key];
    return Array.isArray(value) ? value.map((item) => String(item ?? "")).filter(Boolean) : [];
  }

  function setValue(key: string, value: unknown): void {
    settings = { ...settings, [key]: value };
  }

  function listText(key: string): string {
    return listValue(key).join("\n");
  }

  function updateList(key: string, value: string): void {
    setValue(key, value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean).slice(0, 100));
  }

  function commands(): CommandRow[] {
    const value = settings.bot_custom_commands;
    if (!Array.isArray(value)) return [];
    return value.flatMap((item) => {
      if (!item || typeof item !== "object") return [];
      const row = item as Record<string, unknown>;
      return [{ command: String(row.command ?? ""), reply: String(row.reply ?? "") }];
    });
  }

  function updateCommand(index: number, key: keyof CommandRow, value: string): void {
    const next = commands();
    next[index] = { ...next[index], [key]: value };
    setValue("bot_custom_commands", next);
  }

  function addCommand(): void {
    setValue("bot_custom_commands", [...commands(), { command: "/", reply: "" }]);
  }

  function removeCommand(index: number): void {
    setValue("bot_custom_commands", commands().filter((_, itemIndex) => itemIndex !== index));
    if (focusedTarget?.type === "reply") {
      if (focusedTarget.index === index) focusedTarget = null;
      else if (focusedTarget.index > index) focusedTarget = { type: "reply", index: focusedTarget.index - 1 };
    }
  }

  function commandName(value: string): string {
    return value.trim().toLowerCase().replace(/^\//, "");
  }

  function commandType(row: CommandRow): "text" | "js" {
    return row.reply.trimStart().toLowerCase().startsWith("js:") ? "js" : "text";
  }

  function changeCommandType(index: number, type: "text" | "js"): void {
    const row = commands()[index];
    if (!row) return;
    const reply = row.reply.trimStart().toLowerCase().startsWith("js:") ? row.reply.trimStart().slice(3).trimStart() : row.reply;
    updateCommand(index, "reply", type === "js" ? `js: ${reply}` : reply);
  }

  function commandError(index: number): string {
    const rows = commands();
    const row = rows[index];
    if (!row || !commandName(row.command)) return t.adminTelegramCommandNameRequired;
    const name = commandName(row.command);
    if (data.commands?.commands.some((item) => commandName(item.command) === name)) return t.adminTelegramCommandBuiltinConflict;
    if (rows.some((item, itemIndex) => itemIndex !== index && commandName(item.command) === name)) return t.adminTelegramCommandDuplicate;
    if (!row.reply.trim()) return t.adminTelegramReplyRequired;
    return "";
  }

  function filteredCommands(): TelegramCommandCatalogItem[] {
    const term = search.trim().toLowerCase();
    const items = data.commands?.commands || [];
    if (!term) return items;
    return items.filter((item) => `${item.command} ${item.description} ${item.usage} ${item.category}`.toLowerCase().includes(term));
  }

  function disabled(command: TelegramCommandCatalogItem): boolean {
    const name = commandName(command.command);
    return listValue("disabled_commands").some((item) => commandName(item) === name);
  }

  function toggleDisabled(command: TelegramCommandCatalogItem): void {
    const name = commandName(command.command);
    if (!name || !command.disableable) return;
    const current = listValue("disabled_commands").map(commandName).filter(Boolean);
    setValue("disabled_commands", disabled(command) ? current.filter((item) => item !== name) : [...new Set([...current, name])]);
  }

  function insertPanelPlaceholder(value: string): void {
    if (focusedTarget?.type === "panel") setValue("group_user_panel_template", `${textValue("group_user_panel_template")}${value}`);
    else if (focusedTarget?.type === "reply") {
      const rows = commands();
      if (rows[focusedTarget.index]) updateCommand(focusedTarget.index, "reply", `${rows[focusedTarget.index].reply}${value}`);
    }
  }

  function dateLabel(value: number | null | undefined): string {
    return value ? new Date(value * 1000).toLocaleString("zh-CN") : "-";
  }

  function rosterLabel(roster: TelegramRosterStats | null): string {
    if (!roster) return "-";
    if (!roster.available) return t.adminTelegramRosterUnavailable;
    return `${roster.active || 0} ${t.adminTelegramRosterActive} / ${roster.inactive || 0} ${t.adminTelegramRosterInactive}`;
  }

  function categoryLabel(category: string): string {
    return ({ user: t.adminTelegramCategoryUser, admin: t.adminTelegramCategoryAdmin, system: t.adminTelegramCategorySystem, group: t.adminTelegramCategoryGroup } as Record<string, string>)[category] || category;
  }
</script>

<svelte:head><title>{t.adminTelegramTitle} - {t.siteName}</title></svelte:head>

<section class="telegram-page" aria-labelledby="telegram-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="telegram-title">{t.adminTelegramTitle}</h1><p class="muted">{t.adminTelegramDescription}</p></div>
    <div class="heading-actions"><a class="text-link" href="/admin/status">{t.adminStatusTitle}</a><a class="button secondary" href="/admin/telegram">{t.adminTelegramRefresh}</a></div>
  </header>

  {#if data.errors.length}<div class="notice warning" role="alert">{data.errors.join("；")}</div>{/if}
  {#if form?.error}<div class="notice error" role="alert">{form.error}</div>{/if}
  {#if data.notice === "saved"}<div class="notice success" role="status">{t.adminTelegramSaved}</div>{/if}

  <div class="summary-grid">
    <section class="summary-card"><span>{t.adminTelegramBotStatus}</span><strong>{form?.action === "botTest" && form.result ? (form.result.results.some((item) => item.success) ? t.adminTelegramTestPassed : t.adminTelegramTestFailed) : t.adminTelegramManualTest}</strong><form method="POST" action="?/botTest"><button class="button secondary" type="submit">{t.adminTelegramTestBot}</button></form></section>
    <section class="summary-card"><span>{t.adminTelegramRoster}</span><strong>{rosterLabel(data.roster)}</strong>{#if data.roster?.last_seen_at}<small>{t.adminTelegramLastSeen.replace("{date}", dateLabel(data.roster.last_seen_at))}</small>{/if}</section>
    <section class="summary-card"><span>{t.adminTelegramCommands}</span><strong>{data.commands?.commands.length || 0}</strong><small>{t.adminTelegramDisabledCount.replace("{count}", String(listValue("disabled_commands").length))}</small></section>
  </div>

  {#if form?.action === "botTest" && form.result}
    <section class="panel test-panel" aria-labelledby="telegram-test-title">
      <header class="panel-heading"><div><h2 id="telegram-test-title">{t.adminTelegramTestResult}</h2><p class="muted">{t.adminTelegramTestResultHelp}</p></div></header>
      <div class="test-grid">{#each form.result.results as result, index (result.target + index)}<article class:passed={result.success} class="test-item"><strong>{result.success ? t.adminTelegramPassed : t.adminTelegramFailed}</strong><span>{result.target}</span>{#if result.username}<small>@{result.username}</small>{/if}{#if result.title}<small>{result.title}</small>{/if}{#if result.bot_status}<small>{result.bot_status}</small>{/if}{#if result.error}<small>{result.error}</small>{/if}</article>{/each}</div>
    </section>
  {/if}

  <form method="POST" action="?/save" class="settings-form">
    <input type="hidden" name="settings" value={JSON.stringify(settings)} />

    <section class="panel" aria-labelledby="connection-title">
      <header class="panel-heading"><div><h2 id="connection-title">{t.adminTelegramIdentityTitle}</h2><p class="muted">{t.adminTelegramIdentityDescription}</p></div><a class="text-link" href="/admin/config">{t.adminTelegramOpenFullConfig}</a></header>
      <div class="field-grid">
        <label>{t.adminTelegramAdminIDs}<textarea rows="3" value={listText("admin_id")} oninput={(event) => updateList("admin_id", event.currentTarget.value)} placeholder={t.adminTelegramIDPlaceholder}></textarea></label>
        <label>{t.adminTelegramGroupIDs}<textarea rows="3" value={listText("group_id")} oninput={(event) => updateList("group_id", event.currentTarget.value)} placeholder={t.adminTelegramGroupIDPlaceholder}></textarea></label>
        <label>{t.adminTelegramChannelIDs}<textarea rows="3" value={listText("channel_id")} oninput={(event) => updateList("channel_id", event.currentTarget.value)} placeholder={t.adminTelegramChannelIDPlaceholder}></textarea></label>
        <label>{t.adminTelegramParseMode}<select value={textValue("parse_mode")} onchange={(event) => setValue("parse_mode", event.currentTarget.value)}><option value="">{t.adminTelegramPlainText}</option><option value="HTML">{t.adminTelegramHTML}</option><option value="Markdown">{t.adminTelegramMarkdown}</option><option value="MarkdownV2">{t.adminTelegramMarkdownV2}</option></select></label>
      </div>
      <div class="check-grid">
        {#each [["force_bind_group", t.adminTelegramForceGroup], ["force_bind_channel", t.adminTelegramForceChannel], ["enable_tg_panel", t.adminTelegramEnablePanel], ["require_group_membership", t.adminTelegramRequireMembership], ["ban_on_leave", t.adminTelegramBanOnLeave], ["auto_enable_rejoined", t.adminTelegramAutoRejoin]] as [key, label]}
          <label class="check"><input type="checkbox" checked={boolValue(key)} onchange={(event) => setValue(key, event.currentTarget.checked)} />{label}</label>
        {/each}
      </div>
      <div class="field-grid narrow"><label>{t.adminTelegramCheckConcurrency}<input type="number" min="1" max="100" value={numberValue("group_check_concurrency", 24)} oninput={(event) => setValue("group_check_concurrency", Number(event.currentTarget.value))} /></label><label>{t.adminTelegramActionConcurrency}<input type="number" min="1" max="100" value={numberValue("group_action_concurrency", 8)} oninput={(event) => setValue("group_action_concurrency", Number(event.currentTarget.value))} /></label></div>
    </section>

    <section class="panel" aria-labelledby="copy-title">
      <header class="panel-heading"><div><h2 id="copy-title">{t.adminTelegramCopyTitle}</h2><p class="muted">{t.adminTelegramCopyDescription}</p></div></header>
      <div class="copy-grid">
        <label>{t.adminTelegramStartText}<textarea rows="5" value={textValue("bot_start_text")} oninput={(event) => setValue("bot_start_text", event.currentTarget.value)}></textarea></label>
        <label>{t.adminTelegramGroupStartText}<textarea rows="5" value={textValue("bot_group_start_text")} oninput={(event) => setValue("bot_group_start_text", event.currentTarget.value)}></textarea></label>
        <label>{t.adminTelegramHelpText}<textarea rows="5" value={textValue("bot_help_text")} oninput={(event) => setValue("bot_help_text", event.currentTarget.value)}></textarea></label>
        <label>{t.adminTelegramAdminHelpText}<textarea rows="5" value={textValue("bot_admin_help_text")} oninput={(event) => setValue("bot_admin_help_text", event.currentTarget.value)}></textarea></label>
      </div>
    </section>

    <section class="panel" aria-labelledby="panel-template-title">
      <header class="panel-heading"><div><h2 id="panel-template-title">{t.adminTelegramPanelTemplate}</h2><p class="muted">{t.adminTelegramPanelTemplateDescription}</p></div></header>
      <textarea rows="12" value={textValue("group_user_panel_template")} onfocus={() => focusedTarget = { type: "panel" }} oninput={(event) => setValue("group_user_panel_template", event.currentTarget.value)}></textarea>
      <div class="placeholder-list" aria-label={t.adminTelegramPlaceholders}>{#each panelPlaceholders as placeholder}<button class="chip" type="button" onclick={() => insertPanelPlaceholder(placeholder)}>{placeholder}</button>{/each}</div>
    </section>

    <section class="panel" aria-labelledby="commands-title">
      <header class="panel-heading"><div><h2 id="commands-title">{t.adminTelegramCommandsEditor}</h2><p class="muted">{t.adminTelegramCommandsDescription}</p></div><button class="button secondary" type="button" onclick={addCommand}>{t.adminTelegramAddCommand}</button></header>
      {#if commands().length === 0}<p class="empty">{t.adminTelegramNoCustomCommands}</p>{:else}<div class="command-list">{#each commands() as row, index (index)}<article class:error-row={Boolean(commandError(index))} class="command-row"><div class="command-head"><label>{t.adminTelegramCommandName}<input value={row.command} maxlength="32" oninput={(event) => updateCommand(index, "command", event.currentTarget.value)} placeholder={t.adminTelegramCommandPlaceholder} /></label><label>{t.adminTelegramCommandType}<select value={commandType(row)} onchange={(event) => changeCommandType(index, event.currentTarget.value as "text" | "js")}><option value="text">{t.adminTelegramTextReply}</option><option value="js">{t.adminTelegramJavaScript}</option></select></label><button class="icon-button" type="button" aria-label={t.adminTelegramRemoveCommand} onclick={() => removeCommand(index)}>×</button></div><label>{commandType(row) === "js" ? t.adminTelegramJavaScript : t.adminTelegramReply}<textarea rows="5" onfocus={() => focusedTarget = { type: "reply", index }} value={commandType(row) === "js" ? row.reply.replace(/^\s*js:\s*/i, "") : row.reply} oninput={(event) => updateCommand(index, "reply", commandType(row) === "js" ? `js: ${event.currentTarget.value}` : event.currentTarget.value)}></textarea></label>{#if commandType(row) === "js"}<p class="muted">{t.adminTelegramJavaScriptHelp}</p>{/if}{#if commandError(index)}<p class="field-error" role="alert">{commandError(index)}</p>{/if}</article>{/each}</div>{/if}
      <div class="placeholder-list" aria-label={t.adminTelegramPlaceholders}>{#each panelPlaceholders as placeholder}<button class="chip" type="button" onclick={() => insertPanelPlaceholder(placeholder)}>{placeholder}</button>{/each}</div>
    </section>

    <section class="panel" aria-labelledby="builtin-title">
      <header class="panel-heading"><div><h2 id="builtin-title">{t.adminTelegramBuiltinCommands}</h2><p class="muted">{t.adminTelegramBuiltinDescription}</p></div><label class="search-control">{t.adminTelegramSearch}<input type="search" value={search} oninput={(event) => search = event.currentTarget.value} /></label></header>
      {#if filteredCommands().length === 0}<p class="empty">{t.adminTelegramNoCommands}</p>{:else}<div class="builtin-list">{#each filteredCommands() as command (command.command)}<label class="builtin-row"><input type="checkbox" checked={disabled(command)} disabled={!command.disableable} onchange={() => toggleDisabled(command)} /><span><strong>{command.label}</strong><small>{categoryLabel(command.category)} · {command.description}</small></span><code>{command.usage}</code></label>{/each}</div>{/if}
    </section>

    <footer class="form-footer"><p class="muted">{t.adminTelegramSecretsHelp}</p><button class="button primary" type="submit" disabled={!data.telegram}>{t.adminTelegramSave}</button></footer>
  </form>
</section>

<style>
  .telegram-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .form-footer { align-items: flex-start; display: flex; gap: .8rem; }
  .page-heading, .panel-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; } .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; } .muted { color: #52606d; margin: .35rem 0 0; }
  .text-link { min-height: 2.45rem; padding: .5rem 0; } .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.45rem; padding: .5rem .8rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } button:disabled { cursor: not-allowed; opacity: .55; }
  .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .summary-grid { display: grid; gap: .75rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .summary-card, .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .summary-card { display: grid; gap: .35rem; } .summary-card span, .summary-card small { color: #52606d; font-size: .8rem; } .summary-card strong { overflow-wrap: anywhere; }
  .panel { display: grid; gap: 1rem; } .field-grid, .copy-grid { display: grid; gap: .8rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .field-grid.narrow { grid-template-columns: repeat(2, minmax(8rem, 18rem)); } label { color: #243b53; display: grid; gap: .35rem; font-weight: 650; min-width: 0; } input, select, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.45rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } textarea { line-height: 1.5; resize: vertical; } input:focus, select:focus, textarea:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .check-grid { display: grid; gap: .55rem .8rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .check { align-items: center; display: flex; font-weight: 500; grid-template-columns: auto 1fr; min-height: 2.45rem; } .check input, .builtin-row input { accent-color: #245b75; min-height: 1.1rem; width: 1.1rem; }
  .test-grid { display: grid; gap: .6rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .test-item { border: 1px solid #f1a7a0; display: grid; gap: .25rem; min-width: 0; padding: .7rem; } .test-item.passed { border-color: #a9d5b4; } .test-item strong { color: #a61b1b; } .test-item.passed strong { color: #276749; } .test-item span, .test-item small { overflow-wrap: anywhere; }
  .placeholder-list { display: flex; flex-wrap: wrap; gap: .4rem; } .chip { background: #eef2f4; border: 1px solid #c8d2da; border-radius: .3rem; color: #243b53; cursor: pointer; min-height: 2rem; padding: .3rem .5rem; } .chip:hover { background: #e1e8ed; }
  .command-list, .builtin-list { display: grid; gap: .65rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .command-row { border: 1px solid #d7dee5; display: grid; gap: .65rem; padding: .8rem; } .command-row.error-row { background: #fff8e6; border-color: #e9c46a; } .command-head { align-items: end; display: grid; gap: .65rem; grid-template-columns: minmax(8rem, 15rem) minmax(8rem, 13rem) auto; } .icon-button { background: #f4f6f8; border: 1px solid #c8d2da; border-radius: .3rem; color: #a61b1b; cursor: pointer; font-size: 1.15rem; min-height: 2.45rem; min-width: 2.45rem; } .field-error { color: #a61b1b; font-size: .82rem; margin: 0; }
  .search-control { align-items: center; display: flex; flex-wrap: wrap; font-size: .85rem; } .search-control input { width: min(18rem, 100%); } .builtin-row { align-items: start; border-bottom: 1px solid #e1e8ed; display: grid; gap: .7rem; grid-template-columns: auto minmax(0, 1fr) minmax(8rem, 18rem); padding: .65rem .2rem; } .builtin-row span { display: grid; gap: .2rem; min-width: 0; } .builtin-row small, .builtin-row code { color: #52606d; overflow-wrap: anywhere; } code { background: #eef2f4; font-size: .76rem; max-width: 100%; padding: .15rem .3rem; }
  .form-footer { align-items: center; border-top: 1px solid #d7dee5; justify-content: space-between; padding-top: .85rem; } .form-footer .muted { flex: 1; } .empty { color: #52606d; margin: 0; padding: 1rem; text-align: center; }
  @media (max-width: 900px) { .summary-grid, .check-grid, .test-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .builtin-row { grid-template-columns: auto minmax(0, 1fr); } .builtin-row code { grid-column: 2; } }
  @media (max-width: 650px) { .field-grid, .copy-grid, .summary-grid, .check-grid, .test-grid, .field-grid.narrow { grid-template-columns: 1fr; } .page-heading, .heading-actions, .panel-heading, .form-footer { align-items: stretch; flex-direction: column; } .heading-actions > *, .form-footer .button, .summary-card form, .summary-card form .button { width: 100%; } .command-head { align-items: stretch; grid-template-columns: 1fr; } .icon-button { width: 100%; } .panel, .summary-card { padding: .85rem; } .search-control { align-items: stretch; } .search-control input { width: 100%; } }
</style>
