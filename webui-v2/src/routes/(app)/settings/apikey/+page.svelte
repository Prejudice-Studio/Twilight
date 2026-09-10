<script lang="ts">
  import { t } from "$lib/i18n";
  import type { ApiKeyItem } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string; created?: { id: number; name: string; key: string } };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);

  function dateLabel(value: number | null | undefined): string { return value && value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : t.apiKeyNeverUsed; }
  function permissionLabel(value: string): string {
    return ({ "account:read": t.apiKeyRead, "account:write": t.apiKeyWrite, "emby:read": t.apiKeyEmbyRead, "emby:write": t.apiKeyEmbyWrite } as Record<string, string>)[value] || value;
  }
  function confirmDelete(event: SubmitEvent): void { if (!window.confirm(t.apiKeyDeleteConfirm)) event.preventDefault(); }
  function permissions(key: ApiKeyItem): string { return (key.permissions || []).map(permissionLabel).join("、") || t.apiKeyNone; }
</script>

<svelte:head><title>{t.apiKeyTitle} - {t.siteName}</title></svelte:head>

<section class="api-key-page" aria-labelledby="api-key-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.settings}</p><h1 id="api-key-title">{t.apiKeyTitle}</h1><p class="muted">{t.apiKeyDescription}</p></div>
    <div class="heading-actions"><a class="text-link" href="/settings">{t.apiKeyBackSettings}</a><a class="button secondary" href="/settings/apikey">{t.apiKeyRefresh}</a></div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "updated"}<p class="notice success" role="status">{t.apiKeyUpdated}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.apiKeyDeleted}</p>{/if}

  {#if action.created}
    <section class="created-panel" aria-labelledby="created-title">
      <h2 id="created-title">{t.apiKeyCreated}: {action.created.name}</h2>
      <p>{t.apiKeyCreatedHelp}</p>
      <label>{t.apiKeyPlaintext}<textarea readonly rows="3" spellcheck="false">{action.created.key}</textarea></label>
    </section>
  {/if}

  <section class="panel usage-panel" aria-labelledby="usage-title"><h2 id="usage-title">{t.apiKeyUsageTitle}</h2><p class="muted">{t.apiKeyUsageDescription}</p></section>

  <section class="panel create-panel" aria-labelledby="create-title">
    <h2 id="create-title">{t.apiKeyCreate}</h2>
    <form method="POST" action="?/create" class="create-form">
      <label>{t.apiKeyCreateName}<input name="name" maxlength="120" required placeholder={t.apiKeyCreateNamePlaceholder} /></label>
      <label>{t.apiKeyCreateRateLimit}<input name="rate_limit" type="number" min="0" max="100000" step="1" value="0" required /><span class="hint">{t.apiKeyCreateRateLimitHelp}</span></label>
      <label class="check-field"><input type="hidden" name="allow_query" value="false" /><input type="checkbox" name="allow_query" value="true" /><span><strong>{t.apiKeyAllowQuery}</strong><small>{t.apiKeyAllowQueryHelp}</small></span></label>
      <button class="button primary" type="submit">{t.apiKeyCreate}</button>
    </form>
  </section>

  <section class="key-list" aria-label={t.apiKeyTitle}>
    {#if data.keys.length}
      {#each data.keys as key (key.id)}
        <article class="key-row">
          <header class="key-header"><div><h2>{key.name}</h2><p class="masked"><code>{key.key}</code></p></div><span class:good={key.enabled} class:warning={!key.enabled} class="status">{key.enabled ? t.apiKeyEnabled : t.apiKeyDisabled}</span></header>
          <dl class="facts"><div><dt>{t.apiKeyPermissions}</dt><dd>{permissions(key)}</dd></div><div><dt>{t.apiKeyRequests.replace("{count}", "")}</dt><dd>{key.request_count}</dd></div><div><dt>{t.apiKeyRateLimit.replace("{limit}", "")}</dt><dd>{key.rate_limit} / 分钟</dd></div><div><dt>{t.apiKeyCreatedAt.replace("{time}", "")}</dt><dd>{dateLabel(key.created_at)}</dd></div><div><dt>{t.apiKeyLastUsed.replace("{time}", "")}</dt><dd>{key.last_used ? dateLabel(key.last_used) : t.apiKeyNeverUsed}</dd></div></dl>
          <details class="edit-details"><summary>{t.apiKeyEdit}</summary>
            <form method="POST" action="?/update" class="edit-form">
              <input type="hidden" name="key_id" value={key.id} />
              <label>{t.apiKeyCreateName}<input name="name" maxlength="120" required value={key.name} /></label>
              <label>{t.apiKeyCreateRateLimit}<input name="rate_limit" type="number" min="0" max="100000" step="1" value={key.rate_limit} required /></label>
              <label class="check-field"><input type="hidden" name="enabled" value="false" /><input type="checkbox" name="enabled" value="true" checked={key.enabled} /><span>{t.apiKeyEnabled}</span></label>
              <label class="check-field"><input type="hidden" name="allow_query" value="false" /><input type="checkbox" name="allow_query" value="true" checked={key.allow_query} /><span>{t.apiKeyAllowQuery}</span></label>
              <button class="button secondary" type="submit">{t.apiKeyEdit}</button>
            </form>
          </details>
          <form method="POST" action="?/delete" onsubmit={confirmDelete}><input type="hidden" name="key_id" value={key.id} /><button class="button danger" type="submit">{t.apiKeyDelete}</button></form>
        </article>
      {/each}
    {:else}<div class="empty"><strong>{t.apiKeyEmpty}</strong><p>{t.apiKeyEmptyHelp}</p></div>{/if}
  </section>
</section>

<style>
  .api-key-page { display: grid; gap: 1rem; min-width: 0; } .page-heading, .heading-actions, .key-header { align-items: flex-start; display: flex; gap: .8rem; } .page-heading, .key-header { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; } .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; } h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.1rem; } .muted { color: #52606d; line-height: 1.5; margin: .4rem 0 0; } .text-link { min-height: 2.45rem; padding: .5rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.45rem; padding: .5rem .8rem; text-decoration: none; white-space: normal; } .button:hover { background: #d6e1e7; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.danger { background: #a63d40; color: #fff; } .button.danger:hover { background: #843336; }
  .notice, .created-panel, .panel { border: 1px solid #d7dee5; border-radius: .45rem; padding: .9rem 1rem; } .notice { margin: 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a9d5b4; color: #276749; } .created-panel { background: #edf7f0; border-color: #a9d5b4; display: grid; gap: .55rem; } .created-panel p { color: #276749; margin: 0; } .created-panel label { color: #245b37; display: grid; gap: .3rem; font-weight: 700; }
  textarea, input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.45rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } textarea { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; resize: vertical; } input:focus, textarea:focus, button:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .usage-panel, .create-panel { display: grid; gap: .55rem; } .create-form, .edit-form { align-items: end; display: grid; gap: .7rem; grid-template-columns: minmax(0, 1fr) minmax(10rem, 1fr) minmax(0, 1.5fr) auto; } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .3rem; min-width: 0; } .hint, label small { color: #52606d; font-size: .76rem; font-weight: 400; line-height: 1.4; } .check-field { align-items: start; display: flex; gap: .5rem; } .check-field input { accent-color: #245b75; flex: 0 0 auto; height: 1.15rem; min-height: 1.15rem; width: 1.15rem; } .check-field span { display: grid; gap: .2rem; }
  .key-list { display: grid; gap: .7rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .key-row { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: .8rem; min-width: 0; padding: .9rem; } .key-header { align-items: center; } .key-header h2 { overflow-wrap: anywhere; } .masked { margin: .3rem 0 0; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .2rem .35rem; } .status { background: #eef2f4; border-radius: 999px; color: #52606d; flex: 0 0 auto; font-size: .78rem; padding: .3rem .5rem; } .status.good { background: #edf7f0; color: #276749; } .status.warning { background: #fff8e6; color: #7b4f00; }
  .facts { display: flex; flex-wrap: wrap; gap: .55rem 1rem; margin: 0; } .facts div { min-width: 9rem; } dt { color: #52606d; font-size: .76rem; } dd { margin: .2rem 0 0; overflow-wrap: anywhere; } .edit-details { border-top: 1px solid #e1e8ed; padding-top: .7rem; } summary { color: #245b75; cursor: pointer; font-weight: 700; } .edit-form { grid-template-columns: minmax(0, 1fr) minmax(10rem, 1fr) auto auto auto; margin-top: .7rem; } .key-row > form:last-child { justify-self: end; } .empty { background: #fff; border: 1px dashed #c8d2da; color: #52606d; padding: 2rem 1rem; text-align: center; } .empty p { margin: .35rem 0 0; }
  @media (max-width: 900px) { .create-form, .edit-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .create-form .button, .edit-form .button { width: 100%; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .key-header { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button, .key-row > form:last-child, .key-row > form:last-child .button { width: 100%; } .create-form, .edit-form { grid-template-columns: 1fr; } .key-list { max-height: none; overflow: visible; } .panel, .key-row, .created-panel { padding: .85rem; } h1 { font-size: 1.65rem; } }
</style>
