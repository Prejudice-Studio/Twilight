<script lang="ts">
  import { t } from "$lib/i18n";
  import type { AdminViolationsPageData, ViolationLog } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string });

  const typeLabels: Record<string, string> = {
    regcode_decoy: t.adminViolationsTypeDecoy,
    regcode_target_mismatch: t.adminViolationsTypeTargetMismatch
  };

  const actionLabels: Record<string, string> = {
    disable_user: t.adminViolationsActionDisableUser,
    disable_emby: t.adminViolationsActionDisableEmby,
    log_only: t.adminViolationsActionLogOnly
  };

  function dateLabel(value: number): string {
    if (!Number.isFinite(value) || value <= 0) return t.adminViolationsUnknown;
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function totalPages(): number {
    return Math.max(1, Math.ceil((data.payload?.total || 0) / Math.max(1, data.payload?.per_page || data.query.per_page)));
  }

  function typeLabel(value: string): string {
    return typeLabels[value] || value || t.adminViolationsUnknown;
  }

  function actionLabel(value: string): string {
    return actionLabels[value] || value || t.adminViolationsUnknown;
  }

  function queryHref(page: number): string {
    const query = data.query;
    const params = new URLSearchParams();
    params.set("page", String(Math.max(1, page)));
    params.set("per_page", String(query.per_page));
    if (query.type && query.type !== "all") params.set("type", query.type);
    if (query.search) params.set("search", query.search);
    return `/admin/violations?${params.toString()}`;
  }

  function queryEntries(query: AdminViolationsPageData["query"]): Array<[string, string | number]> {
    return [["page", query.page], ["per_page", query.per_page], ["type", query.type], ["search", query.search]];
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function codeLabel(log: ViolationLog): string {
    return log.code || t.adminViolationsUnknown;
  }
</script>

<svelte:head><title>{t.adminViolationsTitle} - {t.siteName}</title></svelte:head>

<section class="violations-page" aria-labelledby="violations-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="violations-title">{t.adminViolationsTitle}</h1>
      <p class="muted">{t.adminViolationsDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/admin/audit-logs">{t.adminAuditLogTitle}</a>
      <a class="button secondary" href={queryHref(data.query.page)}>{t.adminViolationsRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.adminViolationsDeleted}</p>{/if}
  {#if data.notice === "cleared"}<p class="notice success" role="status">{t.adminViolationsCleared}</p>{/if}

  <section class="panel" aria-labelledby="filter-title">
    <header class="panel-heading">
      <div>
        <h2 id="filter-title">{t.adminViolationsFilter}</h2>
        <p class="muted">{t.adminViolationsTotal.replace("{count}", String(data.payload?.total || 0))}</p>
      </div>
      {#if (data.payload?.total || 0) > 0}
        <form method="POST" action="?/clearViolations" class="clear-form" onsubmit={(event) => confirmSubmit(event, t.adminViolationsClearConfirm)}>
          {#each queryEntries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
          <input type="hidden" name="confirm" value="CLEAR_VIOLATIONS" />
          <button class="button danger" type="submit">{t.adminViolationsClearAll}</button>
        </form>
      {/if}
    </header>
    <form method="GET" action="/admin/violations" class="filter-form">
      <label>{t.adminViolationsType}
        <select name="type">
          <option value="" selected={data.query.type === "" || data.query.type === "all"}>{t.adminViolationsAllTypes}</option>
          <option value="regcode_decoy" selected={data.query.type === "regcode_decoy"}>{t.adminViolationsTypeDecoy}</option>
          <option value="regcode_target_mismatch" selected={data.query.type === "regcode_target_mismatch"}>{t.adminViolationsTypeTargetMismatch}</option>
        </select>
      </label>
      <label class="search-field">{t.adminViolationsSearch}
        <input name="search" maxlength="200" value={data.query.search} />
      </label>
      <label>{t.adminAuditLogPerPage}
        <select name="per_page">
          {#each [20, 50, 100] as value}<option value={value} selected={data.query.per_page === value}>{value}</option>{/each}
        </select>
      </label>
      <button class="button primary" type="submit">{t.adminViolationsApply}</button>
      <a class="button secondary" href="/admin/violations">{t.adminViolationsReset}</a>
    </form>
  </section>

  <section class="panel list-panel" aria-labelledby="list-title">
    <header class="panel-heading">
      <div>
        <h2 id="list-title">{t.adminViolationsTitle}</h2>
        <p class="muted">{t.adminViolationsPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(totalPages()))}</p>
      </div>
    </header>
    {#if data.payload?.violations?.length}
      <div class="violation-list">
        {#each data.payload.violations as log (log.id)}
          <article class="violation-entry">
            <div class="violation-main">
              <header class="entry-header">
                <div class="identity">
                  <strong>{log.username || t.adminViolationsUnknown}</strong>
                  <span class="badge">{t.adminViolationsUID} {log.uid}</span>
                  {#if log.telegram_id}<span class="badge">{t.adminViolationsTelegram} {log.telegram_id}</span>{/if}
                </div>
                <time datetime={log.created_at > 0 ? new Date(log.created_at * 1000).toISOString() : undefined}>{dateLabel(log.created_at)}</time>
              </header>
              <dl class="metadata">
                <div><dt>{t.adminViolationsCode}</dt><dd><code>{codeLabel(log)}</code></dd></div>
                <div><dt>{t.adminViolationsType}</dt><dd>{typeLabel(log.code_type)}</dd></div>
                <div><dt>{t.adminViolationsAction}</dt><dd><span class:danger-badge={log.action !== "log_only"} class="badge">{actionLabel(log.action)}</span></dd></div>
                {#if log.ip}<div><dt>{t.adminViolationsIP}</dt><dd><code>{log.ip}</code></dd></div>{/if}
              </dl>
              {#if log.reason}<p class="reason"><strong>{t.adminViolationsReason}：</strong>{log.reason}</p>{/if}
            </div>
            <form method="POST" action="?/deleteViolation" class="delete-form" onsubmit={(event) => confirmSubmit(event, t.adminViolationsDeleteConfirm)}>
              {#each queryEntries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
              <input type="hidden" name="violation_id" value={log.id} />
              <button class="button danger compact" type="submit">{t.adminViolationsDelete}</button>
            </form>
          </article>
        {/each}
      </div>
    {:else}
      <p class="empty">{t.adminViolationsEmpty}</p>
    {/if}
    {#if totalPages() > 1}
      <nav class="pagination" aria-label={t.adminViolationsTitle}>
        {#if (data.payload?.page || data.query.page) > 1}<a class="button secondary" href={queryHref((data.payload?.page || data.query.page) - 1)}>{t.adminAnnouncementsPrevious}</a>{:else}<span></span>{/if}
        <span>{t.adminViolationsPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(totalPages()))}</span>
        {#if (data.payload?.page || data.query.page) < totalPages()}<a class="button secondary" href={queryHref((data.payload?.page || data.query.page) + 1)}>{t.adminAnnouncementsNext}</a>{:else}<span></span>{/if}
      </nav>
    {/if}
  </section>
</section>

<style>
  .violations-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .entry-header, .identity, .metadata, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .entry-header, .pagination { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; }
  .muted { color: #52606d; margin: .4rem 0 0; } .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; white-space: normal; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; }
  .notice { border: 1px solid; border-radius: .35rem; margin: 0; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .clear-form { flex: 0 0 auto; }
  .filter-form { align-items: end; display: grid; gap: .75rem; grid-template-columns: minmax(0, 1fr) minmax(0, 2fr) minmax(7rem, .7fr) auto auto; }
  label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .violation-list { display: grid; gap: .7rem; max-height: 70dvh; min-width: 0; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .violation-entry { align-items: start; border: 1px solid #d7dee5; display: flex; gap: 1rem; justify-content: space-between; min-width: 0; padding: .9rem; }
  .violation-main { flex: 1; min-width: 0; } .entry-header { align-items: center; } .identity { align-items: center; flex-wrap: wrap; min-width: 0; } .identity strong { overflow-wrap: anywhere; } time { color: #52606d; flex: 0 0 auto; font-size: .78rem; max-width: 14rem; text-align: right; }
  .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .72rem; max-width: 100%; overflow-wrap: anywhere; padding: .2rem .45rem; white-space: normal; } .danger-badge { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .metadata { flex-wrap: wrap; margin: .7rem 0 0; } .metadata div { align-items: baseline; display: flex; gap: .35rem; min-width: 0; } dt { color: #52606d; font-size: .76rem; } dd { margin: 0; overflow-wrap: anywhere; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .3rem; }
  .reason { border-top: 1px solid #e1e8ed; color: #52606d; margin: .7rem 0 0; padding-top: .6rem; } .reason strong { color: #243b53; } .delete-form { flex: 0 0 auto; } .empty { color: #52606d; padding: 1.5rem; text-align: center; } .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination span { color: #52606d; font-size: .84rem; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .search-field { grid-column: span 2; } .filter-form > .button, .filter-form > a { width: 100%; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .panel-heading, .entry-header { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button, .clear-form, .clear-form .button { width: 100%; } .filter-form { grid-template-columns: 1fr; } .search-field { grid-column: auto; } .violation-entry { flex-direction: column; } .delete-form, .delete-form .button { width: 100%; } .entry-header time { max-width: none; text-align: left; } .panel { padding: .85rem; } h1 { font-size: 1.65rem; } .pagination { align-items: stretch; } .pagination .button { min-width: 0; } }
</style>
