<script lang="ts">
  import { t } from "$lib/i18n";
  import type { AdminAuditLogsPageData, AuditLogEntry } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string });

  const actionLabels: Record<string, string> = {
    create_regcode: t.adminAuditLogActionCreateRegcode,
    update_regcode: t.adminAuditLogActionUpdateRegcode,
    delete_regcode: t.adminAuditLogActionDeleteRegcode,
    batch_delete_regcode: t.adminAuditLogActionBatchDeleteRegcode,
    clear_regcode_usage: t.adminAuditLogActionClearRegcodeUsage,
    create_invite_code: t.adminAuditLogActionCreateInviteCode,
    create_renew_code: t.adminAuditLogActionCreateRenewCode,
    use_code: t.adminAuditLogActionUseCode,
    update_user: t.adminAuditLogActionUpdateUser,
    set_role: t.adminAuditLogActionSetRole,
    enable_user: t.adminAuditLogActionEnableUser,
    disable_user: t.adminAuditLogActionDisableUser,
    delete_user: t.adminAuditLogActionDeleteUser,
    batch_enable_users: t.adminAuditLogActionBatchEnableUsers,
    batch_disable_users: t.adminAuditLogActionBatchDisableUsers,
    batch_renew_users: t.adminAuditLogActionBatchRenewUsers,
    batch_delete_users: t.adminAuditLogActionBatchDeleteUsers
  };

  const categoryLabels: Record<string, string> = {
    admin: t.adminAuditLogCategoryAdmin,
    user: t.adminAuditLogCategoryUser,
    system: t.adminAuditLogCategorySystem
  };

  const sourceLabels: Record<string, string> = {
    http: "HTTP",
    telegram: "Telegram",
    scheduler: "定时任务",
    system: t.adminAuditLogCategorySystem
  };

  const sortOptions = [
    ["created_desc", t.adminAuditLogSortNewest],
    ["created_asc", t.adminAuditLogSortOldest],
    ["action_asc", t.adminAuditLogSortActionAsc],
    ["action_desc", t.adminAuditLogSortActionDesc],
    ["user_asc", t.adminAuditLogSortUserAsc],
    ["user_desc", t.adminAuditLogSortUserDesc],
    ["category_asc", t.adminAuditLogSortCategoryAsc],
    ["uid_asc", t.adminAuditLogSortUIDAsc]
  ];

  function dateLabel(value: number): string {
    return value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : "-";
  }

  function actionLabel(value: string): string {
    return actionLabels[value] || value || t.adminAuditLogUnknown;
  }

  function categoryLabel(value: string): string {
    return categoryLabels[value] || value || t.adminAuditLogUnknown;
  }

  function sourceLabel(value: string | undefined): string {
    return sourceLabels[value || ""] || value || t.adminAuditLogUnknown;
  }

  function detailLabel(value: AuditLogEntry["detail"]): string {
    if (!value || Object.keys(value).length === 0) return "";
    try {
      return JSON.stringify(value) || "";
    } catch {
      return t.adminAuditLogUnknown;
    }
  }

  function totalPages(): number {
    return Math.max(1, Math.ceil((data.payload?.total || 0) / Math.max(1, data.payload?.per_page || data.query.per_page)));
  }

  function queryHref(page: number): string {
    const params = new URLSearchParams();
    const query = data.query;
    params.set("page", String(Math.max(1, page)));
    params.set("per_page", String(query.per_page));
    for (const key of ["preset", "category", "action", "time", "sort", "order", "search"] as const) {
      if (query[key]) params.set(key, query[key]);
    }
    if (query.uid > 0) params.set("uid", String(query.uid));
    if (query.target_uid > 0) params.set("target_uid", String(query.target_uid));
    return `/admin/audit-logs?${params.toString()}`;
  }

  function queryEntries(query: AdminAuditLogsPageData["query"]): Array<[string, string | number]> {
    return [
      ["page", query.page], ["per_page", query.per_page], ["preset", query.preset], ["category", query.category],
      ["action", query.action], ["time", query.time], ["sort", query.sort], ["order", query.order],
      ["uid", query.uid], ["target_uid", query.target_uid], ["search", query.search]
    ];
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }
</script>

<svelte:head><title>{t.adminAuditLogTitle} - {t.siteName}</title></svelte:head>

<section class="audit-page" aria-labelledby="audit-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="audit-title">{t.adminAuditLogTitle}</h1>
      <p class="muted">{t.adminAuditLogDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/admin/status">{t.adminStatusTitle}</a>
      <a class="button secondary" href={queryHref(data.query.page)}>{t.adminAuditLogRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.adminAuditLogDeleted}</p>{/if}
  {#if data.notice === "cleared"}<p class="notice success" role="status">{t.adminAuditLogClearDone}</p>{/if}
  {#if data.notice === "pruned"}<p class="notice success" role="status">{t.adminAuditLogPruneDone}</p>{/if}

  <section class="panel" aria-labelledby="filter-title">
    <header class="panel-heading"><div><h2 id="filter-title">{t.adminAuditLogFilter}</h2><p class="muted">{t.adminAuditLogTotal.replace("{count}", String(data.payload?.total || 0))}</p></div></header>
    <form method="GET" action="/admin/audit-logs" class="filter-form">
      <label>{t.adminAuditLogPreset}<select name="preset">
        <option value="" selected={data.query.preset === "" || data.query.preset === "all"}>{t.adminAuditLogPresetAll}</option>
        <option value="admin" selected={data.query.preset === "admin"}>{t.adminAuditLogPresetAdmin}</option>
        <option value="user" selected={data.query.preset === "user"}>{t.adminAuditLogPresetUser}</option>
        <option value="system" selected={data.query.preset === "system"}>{t.adminAuditLogPresetSystem}</option>
        <option value="destructive" selected={data.query.preset === "destructive"}>{t.adminAuditLogPresetDestructive}</option>
        <option value="security" selected={data.query.preset === "security"}>{t.adminAuditLogPresetSecurity}</option>
      </select></label>
      <label>{t.adminAuditLogCategory}<select name="category">
        <option value="" selected={data.query.category === "" || data.query.category === "all"}>{t.adminAuditLogAllCategories}</option>
        <option value="admin" selected={data.query.category === "admin"}>{t.adminAuditLogCategoryAdmin}</option>
        <option value="user" selected={data.query.category === "user"}>{t.adminAuditLogCategoryUser}</option>
        <option value="system" selected={data.query.category === "system"}>{t.adminAuditLogCategorySystem}</option>
      </select></label>
      <label>{t.adminAuditLogAction}<select name="action">
        <option value="" selected={data.query.action === "" || data.query.action === "all"}>{t.adminAuditLogAllActions}</option>
        {#each Object.entries(actionLabels) as [value, label]}<option value={value} selected={data.query.action === value}>{label}</option>{/each}
      </select></label>
      <label>{t.adminAuditLogTime}<select name="time">
        <option value="" selected={data.query.time === "" || data.query.time === "all"}>{t.adminAuditLogTimeAll}</option>
        <option value="today" selected={data.query.time === "today"}>{t.adminAuditLogTimeToday}</option>
        <option value="24h" selected={data.query.time === "24h"}>{t.adminAuditLogTime24h}</option>
        <option value="7d" selected={data.query.time === "7d"}>{t.adminAuditLogTime7d}</option>
        <option value="30d" selected={data.query.time === "30d"}>{t.adminAuditLogTime30d}</option>
      </select></label>
      <label>{t.adminAuditLogSort}<select name="sort">
        {#each sortOptions as [value, label]}<option value={value} selected={data.query.sort === value}>{label}</option>{/each}
      </select></label>
      <label>{t.adminAuditLogPerPage}<select name="per_page">
        {#each [25, 50, 100, 200] as value}<option value={value} selected={data.query.per_page === value}>{value}</option>{/each}
      </select></label>
      <label>{t.adminAuditLogUID}<input name="uid" inputmode="numeric" pattern="[0-9]*" value={data.query.uid || ""} /></label>
      <label>{t.adminAuditLogTargetUID}<input name="target_uid" inputmode="numeric" pattern="[0-9]*" value={data.query.target_uid || ""} /></label>
      <label class="search-field">{t.adminAuditLogSearch}<input name="search" maxlength="200" value={data.query.search} /></label>
      <button class="button primary" type="submit">{t.adminAuditLogApply}</button>
      <a class="button secondary" href="/admin/audit-logs">{t.adminAuditLogReset}</a>
    </form>
  </section>

  <section class="panel maintenance" aria-labelledby="maintenance-title">
    <header class="panel-heading"><div><h2 id="maintenance-title">{t.adminAuditLogPrune}</h2><p class="muted">{t.adminAuditLogPruneDescription}</p></div></header>
    <div class="maintenance-grid">
      <form method="POST" action="?/prune" class="maintenance-form" onsubmit={(event) => confirmSubmit(event, t.adminAuditLogPruneConfirm)}>
        {#each queryEntries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
        <label>{t.adminAuditLogPruneDays}<input name="retention_days" type="number" min="1" max="3650" placeholder="90" /></label>
        <label>{t.adminAuditLogPruneEntries}<input name="max_entries" type="number" min="1" max="100000" placeholder="1000" /></label>
        <label class="check-field"><input type="hidden" name="preserve_admin" value="false" /><input type="checkbox" name="preserve_admin" value="true" checked />{t.adminAuditLogPrunePreserveAdmin}</label>
        <button class="button secondary" type="submit">{t.adminAuditLogPruneApply}</button>
      </form>
      <form method="POST" action="?/clear" class="maintenance-form danger-form" onsubmit={(event) => confirmSubmit(event, t.adminAuditLogClearConfirm)}>
        {#each queryEntries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
        <p>{t.adminAuditLogClearDescription}</p>
        <button class="button danger" type="submit">{t.adminAuditLogClear}</button>
      </form>
    </div>
  </section>

  <section class="panel log-panel" aria-labelledby="list-title">
    <header class="panel-heading"><div><h2 id="list-title">{t.adminAuditLogTitle}</h2><p class="muted">{t.adminAuditLogPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(totalPages()))}</p></div></header>
    {#if data.payload?.logs?.length}
      <div class="log-list">
        {#each data.payload.logs as log (log.id)}
          <article class="log-entry">
            <div class="log-main">
              <header class="log-header">
                <div class="identity"><strong>{log.username || t.adminAuditLogUnknown}</strong><span class="badge">UID {log.uid}</span><span class="badge category-{log.category}">{categoryLabel(log.category)}</span></div>
                <time datetime={new Date(log.created_at * 1000).toISOString()}>{dateLabel(log.created_at)}</time>
              </header>
              <p class="action-line"><strong>{actionLabel(log.action)}</strong>{#if log.target_uid && log.target_uid > 0}<span>{t.adminAuditLogTarget} UID {log.target_uid}</span>{/if}</p>
              <dl class="metadata">
                <div><dt>{t.adminAuditLogSource}</dt><dd>{sourceLabel(log.source)}</dd></div>
                {#if log.method}<div><dt>{t.adminAuditLogMethod}</dt><dd>{log.method}</dd></div>{/if}
                {#if log.ip}<div><dt>{t.adminAuditLogIP}</dt><dd><code>{log.ip}</code></dd></div>{/if}
              </dl>
              {#if detailLabel(log.detail)}<details class="detail"><summary>{t.adminAuditLogDetail}</summary><pre>{detailLabel(log.detail)}</pre></details>{/if}
            </div>
            <form method="POST" action="?/deleteLog" class="delete-form" onsubmit={(event) => confirmSubmit(event, t.adminAuditLogDelete)}>
              {#each queryEntries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
              <input type="hidden" name="log_id" value={log.id} />
              <button class="button danger compact" type="submit">{t.adminAuditLogDelete}</button>
            </form>
          </article>
        {/each}
      </div>
    {:else}
      <p class="empty">{t.adminAuditLogEmpty}</p>
    {/if}
    {#if totalPages() > 1}
      <nav class="pagination" aria-label={t.adminAuditLogTitle}>
        {#if (data.payload?.page || data.query.page) > 1}<a class="button secondary" href={queryHref((data.payload?.page || data.query.page) - 1)}>{t.adminTicketsPrevious}</a>{:else}<span></span>{/if}
        <span>{t.adminAuditLogPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(totalPages()))}</span>
        {#if (data.payload?.page || data.query.page) < totalPages()}<a class="button secondary" href={queryHref((data.payload?.page || data.query.page) + 1)}>{t.adminTicketsNext}</a>{:else}<span></span>{/if}
      </nav>
    {/if}
  </section>
</section>

<style>
  .audit-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .filter-form, .maintenance-grid, .maintenance-form, .log-header, .identity, .metadata, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .log-header, .pagination { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; }
  .muted { color: #52606d; margin: .4rem 0 0; } .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; white-space: normal; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; }
  .notice { border: 1px solid; border-radius: .35rem; margin: 0; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .filter-form { align-items: end; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); }
  label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input[type="checkbox"] { min-height: 1rem; width: 1rem; } input:focus, select:focus, button:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .search-field { grid-column: span 2; } .filter-form > .button { width: 100%; }
  .maintenance-grid { align-items: stretch; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); } .maintenance-form { align-items: end; border: 1px solid #d7dee5; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); padding: .85rem; } .maintenance-form > p { grid-column: 1 / -1; margin: 0; } .maintenance-form > .button { width: 100%; } .check-field { align-items: center; display: flex; font-weight: 500; grid-column: 1 / -1; } .danger-form { border-color: #f1a7a0; }
  .log-panel { min-width: 0; } .log-list { display: grid; gap: .7rem; max-height: 70dvh; min-width: 0; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .log-entry { align-items: start; border: 1px solid #d7dee5; display: flex; gap: 1rem; justify-content: space-between; min-width: 0; padding: .9rem; } .log-main { min-width: 0; flex: 1; } .log-header { align-items: center; } .identity { align-items: center; flex-wrap: wrap; min-width: 0; } .identity strong { overflow-wrap: anywhere; } time { color: #52606d; flex: 0 0 auto; font-size: .78rem; max-width: 14rem; text-align: right; } .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .72rem; padding: .2rem .45rem; white-space: nowrap; } .category-admin { background: #edf4f8; color: #245b75; } .category-system { background: #f4f7f8; color: #52606d; }
  .action-line { align-items: baseline; display: flex; flex-wrap: wrap; gap: .65rem; margin: .65rem 0; } .action-line span { color: #52606d; font-size: .82rem; } .metadata { flex-wrap: wrap; margin: 0; } .metadata div { align-items: baseline; display: flex; gap: .35rem; min-width: 0; } dt { color: #52606d; font-size: .76rem; } dd { margin: 0; overflow-wrap: anywhere; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .3rem; }
  .detail { border-top: 1px solid #e1e8ed; margin-top: .7rem; padding-top: .6rem; } summary { color: #245b75; cursor: pointer; font-size: .8rem; } pre { background: #f4f7f8; max-height: 12rem; overflow: auto; padding: .6rem; white-space: pre-wrap; overflow-wrap: anywhere; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .delete-form { flex: 0 0 auto; } .empty { color: #52606d; padding: 1.5rem; text-align: center; } .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination span { color: #52606d; font-size: .84rem; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .search-field { grid-column: 1 / -1; } .maintenance-grid { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .panel-heading, .log-header { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button { width: 100%; } .filter-form, .maintenance-form { grid-template-columns: 1fr; } .search-field, .check-field, .maintenance-form > p { grid-column: auto; } .filter-form > .button { width: 100%; } .log-entry { flex-direction: column; } .delete-form, .delete-form .button { width: 100%; } .log-header time { max-width: none; text-align: left; } .panel { padding: .85rem; } h1 { font-size: 1.65rem; } }
</style>
