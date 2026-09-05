<script lang="ts">
  import { t } from "$lib/i18n";
  import type { AdminRegcodesPageData, Regcode, RegcodeUsageUser } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string; codes?: string[] });

  const typeLabels: Record<number, string> = {
    1: t.adminRegcodesRegister,
    2: t.adminRegcodesRenew,
    3: t.adminRegcodesWhitelist
  };
  const statusLabels: Record<string, string> = {
    available: t.adminRegcodesAvailable,
    disabled: t.adminRegcodesDisabled,
    used_up: t.adminRegcodesUsedUp,
    expired: t.adminRegcodesExpired
  };

  function dateLabel(value: number | undefined): string {
    if (!value || value <= 0) return "-";
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function typeLabel(value: number): string {
    return typeLabels[value] || `Type ${value}`;
  }

  function statusLabel(value: string | undefined): string {
    return statusLabels[value || ""] || value || t.adminRegcodesDisabled;
  }

  function daysLabel(value: number | undefined): string {
    return value === undefined || value < 0 ? "永久" : `${value} 天`;
  }

  function hoursLabel(value: number | undefined): string {
    return value === undefined || value < 0 ? "永久" : `${value} 小时`;
  }

  function totalPages(): number {
    return Math.max(1, Math.ceil((data.payload?.total || 0) / Math.max(1, data.payload?.per_page || data.query.per_page)));
  }

  function queryHref(page: number, usage = ""): string {
    const query = data.query;
    const params = new URLSearchParams();
    params.set("page", String(Math.max(1, page)));
    params.set("per_page", String(query.per_page));
    if (query.type && query.type !== "all") params.set("type", query.type);
    if (query.status && query.status !== "all") params.set("status", query.status);
    if (query.source && query.source !== "all") params.set("source", query.source);
    if (query.search) params.set("search", query.search);
    if (query.sort !== "created_time") params.set("sort", query.sort);
    if (query.order !== "desc") params.set("order", query.order);
    if (usage) params.set("usage", usage);
    return `/admin/regcodes?${params.toString()}`;
  }

  function filterEntries(): Array<[string, string | number]> {
    const query = data.query;
    return [
      ["page", query.page], ["per_page", query.per_page], ["type", query.type], ["status", query.status],
      ["source", query.source], ["search", query.search], ["sort", query.sort], ["order", query.order],
      ["filter_type", query.type], ["filter_status", query.status], ["filter_source", query.source], ["filter_search", query.search]
    ];
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function usageName(user: RegcodeUsageUser): string {
    return user.found && user.username ? user.username : t.adminRegcodesUsageUnknown;
  }
</script>

<svelte:head><title>{t.adminRegcodesTitle} - {t.siteName}</title></svelte:head>

<section class="regcodes-page" aria-labelledby="regcodes-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="regcodes-title">{t.adminRegcodesTitle}</h1>
      <p class="muted">{t.adminRegcodesDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="button secondary" href={queryHref(data.query.page)}>{t.adminRegcodesRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if data.usageError}<p class="notice error" role="alert">{data.usageError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "created" || action.action === "created"}<p class="notice success" role="status">{t.adminRegcodesCreated}</p>{/if}
  {#if data.notice === "updated"}<p class="notice success" role="status">{t.adminRegcodesUpdated}</p>{/if}
  {#if data.notice === "deleted" || data.notice === "batch_deleted"}<p class="notice success" role="status">{t.adminRegcodesDeleted}</p>{/if}
  {#if data.notice === "usage_cleared"}<p class="notice success" role="status">{t.adminRegcodesUsageCleared}</p>{/if}

  {#if action.action === "created" && action.codes?.length}
    <section class="panel generated-panel" aria-labelledby="generated-title">
      <header class="panel-heading"><h2 id="generated-title">{t.adminRegcodesGeneratedCodes}</h2></header>
      <div class="code-output">{#each action.codes as code}<code>{code}</code>{/each}</div>
      <p class="muted">{t.adminRegcodesCreateDescription}</p>
    </section>
  {/if}

  <section class="panel" aria-labelledby="create-title">
    <header class="panel-heading"><div><h2 id="create-title">{t.adminRegcodesCreate}</h2><p class="muted">{t.adminRegcodesCreateDescription}</p></div></header>
    <form method="POST" action="?/create" class="create-form">
      <label>{t.adminRegcodesType}<select name="type"><option value="1">{t.adminRegcodesRegister}</option><option value="2">{t.adminRegcodesRenew}</option><option value="3">{t.adminRegcodesWhitelist}</option></select></label>
      <label>{t.adminRegcodesDays}<input name="days" type="number" min="-1" max="36500" value="30" /></label>
      <label>{t.adminRegcodesValidity}<input name="validity_time" type="number" min="-1" max="876000" value="-1" /></label>
      <label>{t.adminRegcodesUseLimit}<input name="use_count_limit" type="number" min="-1" max="1000000" value="1" /></label>
      <label>{t.adminRegcodesCount}<input name="count" type="number" min="1" max="100" value="1" /></label>
      <label>{t.adminRegcodesFormat}<input name="format" maxlength="120" placeholder="TW-&#123;type&#125;-&#123;random&#125;" /></label>
      <label>{t.adminRegcodesAlgorithm}<input name="random_algorithm" maxlength="48" placeholder="base32-20" /></label>
      <label>{t.adminRegcodesNote}<input name="note" maxlength="120" /></label>
      <label>{t.adminRegcodesTargetUsername}<input name="target_username" maxlength="32" /></label>
      <label>{t.adminRegcodesTargetTelegram}<input name="target_telegram_username" maxlength="32" /></label>
      <label>{t.adminRegcodesTargetTelegramID}<input name="target_telegram_id" inputmode="numeric" maxlength="24" /></label>
      <label>{t.adminRegcodesTargetUID}<input name="target_uid" inputmode="numeric" maxlength="24" /></label>
      <label class="check-field"><input type="checkbox" name="decoy" value="true" />{t.adminRegcodesDecoy}</label>
      <button class="button primary" type="submit">{t.adminRegcodesSubmit}</button>
    </form>
  </section>

  <section class="panel" aria-labelledby="filter-title">
    <header class="panel-heading"><div><h2 id="filter-title">{t.adminRegcodesFilter}</h2><p class="muted">{t.adminRegcodesTotal.replace("{count}", String(data.payload?.total || 0))}</p></div></header>
    <form method="GET" action="/admin/regcodes" class="filter-form">
      <label>{t.adminRegcodesType}<select name="type"><option value="" selected={!data.query.type || data.query.type === "all"}>{t.adminRegcodesAllTypes}</option><option value="1" selected={data.query.type === "1"}>{t.adminRegcodesRegister}</option><option value="2" selected={data.query.type === "2"}>{t.adminRegcodesRenew}</option><option value="3" selected={data.query.type === "3"}>{t.adminRegcodesWhitelist}</option></select></label>
      <label>{t.adminRegcodesSource}<select name="source"><option value="" selected={!data.query.source || data.query.source === "all"}>{t.adminRegcodesAllSources}</option><option value="admin" selected={data.query.source === "admin"}>{t.adminRegcodesAdminSource}</option><option value="invite" selected={data.query.source === "invite"}>{t.adminRegcodesInviteSource}</option></select></label>
      <label>{t.adminRegcodesStatus}<select name="status"><option value="" selected={!data.query.status || data.query.status === "all"}>{t.adminRegcodesAllStatuses}</option><option value="available" selected={data.query.status === "available"}>{t.adminRegcodesAvailable}</option><option value="disabled" selected={data.query.status === "disabled"}>{t.adminRegcodesDisabled}</option><option value="used_up" selected={data.query.status === "used_up"}>{t.adminRegcodesUsedUp}</option><option value="expired" selected={data.query.status === "expired"}>{t.adminRegcodesExpired}</option><option value="decoy" selected={data.query.status === "decoy"}>{t.adminRegcodesDecoy}</option></select></label>
      <label>{t.adminRegcodesSort}<select name="sort"><option value="created_time" selected={data.query.sort === "created_time"}>{t.adminRegcodesNewest}</option><option value="code" selected={data.query.sort === "code"}>{t.adminRegcodesCodeAsc}</option><option value="days" selected={data.query.sort === "days"}>{t.adminRegcodesDaysAsc}</option></select></label>
      <label>{t.adminRegcodesSearch}<input name="search" maxlength="120" value={data.query.search} /></label>
      <label>{t.adminAuditLogPerPage}<select name="per_page">{#each [20, 50, 100] as value}<option value={value} selected={data.query.per_page === value}>{value}</option>{/each}</select></label>
      <label>{t.adminAuditLogSort}<select name="order"><option value="desc" selected={data.query.order === "desc"}>{t.adminRegcodesNewest}</option><option value="asc" selected={data.query.order === "asc"}>{t.adminRegcodesOldest}</option></select></label>
      <button class="button primary" type="submit">{t.adminRegcodesApply}</button>
      <a class="button secondary" href="/admin/regcodes">{t.adminRegcodesReset}</a>
    </form>
  </section>

  <form id="batch-delete" method="POST" action="?/batchDelete" onsubmit={(event) => confirmSubmit(event, t.adminRegcodesBatchDeleteConfirm)}>
    {#each filterEntries() as [name, value]}<input type="hidden" name={name} value={value} />{/each}
    <input type="hidden" name="confirm" value="BATCH_DELETE_REGCODES" />
    <div class="batch-toolbar">
      <label class="check-field"><input type="checkbox" name="select_all" value="true" />{t.adminRegcodesSelectAllMatched}</label>
      <button class="button danger" type="submit">{t.adminRegcodesBatchDelete}</button>
    </div>
  </form>

  <section class="panel list-panel" aria-labelledby="list-title">
    <header class="panel-heading"><div><h2 id="list-title">{t.adminRegcodesTitle}</h2><p class="muted">{t.adminRegcodesPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(totalPages()))}</p></div></header>
    {#if data.payload?.regcodes?.length}
      <div class="regcode-list">
        {#each data.payload.regcodes as code (code.code)}
          <article class="regcode-entry">
            <input class="row-check" type="checkbox" name="codes" value={code.code} form="batch-delete" aria-label={`${t.adminRegcodesBatchDelete}: ${code.code}`} />
            <div class="regcode-main">
              <header class="entry-header"><div class="identity"><code class="code-value">{code.code}</code><span class="badge">{typeLabel(code.type)}</span>{#if code.is_decoy}<span class="badge warning-badge">{t.adminRegcodesDecoy}</span>{/if}<span class="badge">{statusLabel(code.status)}</span></div><time>{dateLabel(code.created_time)}</time></header>
              <dl class="metadata">
                <div><dt>{t.adminRegcodesAccountDays}</dt><dd>{daysLabel(code.days)}</dd></div>
                <div><dt>{t.adminRegcodesValidityShort}</dt><dd>{hoursLabel(code.validity_time)}</dd></div>
                <div><dt>{t.adminRegcodesUsage}</dt><dd>{code.use_count || 0} / {code.use_count_limit === -1 ? "∞" : (code.use_count_limit ?? 1)}</dd></div>
                {#if code.source}<div><dt>{t.adminRegcodesSource}</dt><dd>{code.source === "invite" ? t.adminRegcodesInviteSource : t.adminRegcodesAdminSource}</dd></div>{/if}
                {#if code.target_username || code.target_telegram_username || code.target_telegram_id || code.target_uid}<div><dt>{t.adminRegcodesTarget}</dt><dd>{code.target_resolved_username || code.target_username || code.target_telegram_username || code.target_telegram_id || code.target_uid}</dd></div>{/if}
                {#if code.note}<div class="wide-field"><dt>{t.adminRegcodesNote}</dt><dd>{code.note}</dd></div>{/if}
              </dl>
              <div class="entry-actions">
                <a class="button secondary compact" href={queryHref(data.query.page, code.code)}>{t.adminRegcodesUsageLink}</a>
                <details class="edit-details"><summary class="button secondary compact">{t.adminRegcodesEdit}</summary><form method="POST" action="?/update" class="edit-form">
                  <input type="hidden" name="code" value={code.code} /><input type="hidden" name="active" value="false" />
                  <label class="check-field"><input type="checkbox" name="active" value="true" checked={code.active !== false} />{t.adminRegcodesEnable}</label>
                  <label>{t.adminRegcodesDays}<input name="days" type="number" min="-1" max="36500" value={code.days} /></label>
                  <label>{t.adminRegcodesValidity}<input name="validity_time" type="number" min="-1" max="876000" value={code.validity_time ?? -1} /></label>
                  <label>{t.adminRegcodesUseLimit}<input name="use_count_limit" type="number" min="-1" max="1000000" value={code.use_count_limit ?? 1} /></label>
                  <label>{t.adminRegcodesNote}<input name="note" maxlength="120" value={code.note || ""} /></label>
                  <button class="button primary compact" type="submit">{t.adminRegcodesSave}</button>
                </form></details>
                <form method="POST" action="?/delete" onsubmit={(event) => confirmSubmit(event, t.adminRegcodesDeleteConfirm)}><input type="hidden" name="code" value={code.code} />{#each filterEntries().slice(0, 8) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button danger compact" type="submit">{t.adminRegcodesDelete}</button></form>
              </div>
            </div>
          </article>
        {/each}
      </div>
    {:else}<p class="empty">{t.adminRegcodesEmpty}</p>{/if}
    {#if totalPages() > 1}<nav class="pagination" aria-label={t.adminRegcodesTitle}>{#if (data.payload?.page || data.query.page) > 1}<a class="button secondary" href={queryHref((data.payload?.page || data.query.page) - 1)}>{t.adminAnnouncementsPrevious}</a>{:else}<span></span>{/if}<span>{t.adminRegcodesPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(totalPages()))}</span>{#if (data.payload?.page || data.query.page) < totalPages()}<a class="button secondary" href={queryHref((data.payload?.page || data.query.page) + 1)}>{t.adminAnnouncementsNext}</a>{:else}<span></span>{/if}</nav>{/if}
  </section>

  {#if data.query.usage}
    <section class="panel usage-panel" aria-labelledby="usage-title">
      <header class="panel-heading"><div><h2 id="usage-title">{t.adminRegcodesUsageTitle}</h2><p class="muted">{t.adminRegcodesUsageFor.replace("{code}", data.query.usage).replace("{count}", String(data.usage?.use_count || 0))}</p></div><a class="button secondary" href={queryHref(data.query.page)}>{t.adminRegcodesCloseUsage}</a></header>
      {#if data.usage?.users?.length || data.usage?.telegram_only?.length}
        <div class="usage-list">{#each data.usage.users as user}<div class="usage-row"><strong>{usageName(user)}</strong><span class="badge">{user.source === "uid" ? t.adminRegcodesUsageSourceUID : t.adminRegcodesUsageSourceTelegram}</span>{#if user.uid}<span>UID {user.uid}</span>{/if}{#if user.telegram_id}<span>TG {user.telegram_id}</span>{/if}</div>{/each}{#each data.usage.telegram_only || [] as user}<div class="usage-row"><strong>{t.adminRegcodesUsageUnknown}</strong><span class="badge">{t.adminRegcodesUsageSourceTelegram}</span><span>TG {user.telegram_id}</span></div>{/each}</div>
      {:else}<p class="empty">{t.adminRegcodesUsageEmpty}</p>{/if}
      <form method="POST" action="?/clearUsage" class="usage-clear-form" onsubmit={(event) => confirmSubmit(event, t.adminRegcodesClearUsageConfirm)}><input type="hidden" name="code" value={data.query.usage} />{#each filterEntries().slice(0, 8) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button danger" type="submit">{t.adminRegcodesClearUsage}</button></form>
    </section>
  {/if}
</section>

<style>
  .regcodes-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .entry-header, .identity, .metadata, .entry-actions, .batch-toolbar, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .entry-header, .pagination { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; } .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; } .muted { color: #52606d; margin: .4rem 0 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; } .notice { border: 1px solid; border-radius: .35rem; margin: 0; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .create-form { display: grid; gap: .75rem; grid-template-columns: repeat(4, minmax(0, 1fr)); } .filter-form { align-items: end; display: grid; gap: .75rem; grid-template-columns: repeat(4, minmax(0, 1fr)); } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input[type="checkbox"] { min-height: 1rem; width: 1rem; } input:focus, select:focus, button:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; } .check-field { align-items: center; display: flex; font-weight: 500; }
  .generated-panel { border-color: #8ab69a; } .code-output { background: #f4f7f8; border: 1px solid #d7dee5; display: grid; gap: .4rem; max-height: 14rem; overflow: auto; overscroll-behavior: contain; padding: .75rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .3rem; }
  .batch-toolbar { align-items: center; background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; justify-content: space-between; padding: .75rem 1rem; } .regcode-list, .usage-list { display: grid; gap: .7rem; } .list-panel { min-width: 0; } .regcode-list { max-height: 70dvh; min-width: 0; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .regcode-entry { align-items: start; border: 1px solid #d7dee5; display: flex; gap: .7rem; min-width: 0; padding: .9rem; } .row-check { flex: 0 0 auto; margin-top: .4rem; } .regcode-main { flex: 1; min-width: 0; } .entry-header { align-items: center; } .identity { align-items: center; flex-wrap: wrap; min-width: 0; } .code-value { color: #16394a; font-size: .9rem; } time { color: #52606d; flex: 0 0 auto; font-size: .78rem; max-width: 14rem; text-align: right; } .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .72rem; max-width: 100%; overflow-wrap: anywhere; padding: .2rem .45rem; white-space: normal; } .warning-badge { background: #fff5df; border-color: #e0a458; color: #855f20; }
  .metadata { flex-wrap: wrap; margin: .7rem 0 0; } .metadata div { align-items: baseline; display: flex; gap: .35rem; min-width: 0; } .metadata .wide-field { flex-basis: 100%; } dt { color: #52606d; font-size: .76rem; } dd { margin: 0; overflow-wrap: anywhere; } .entry-actions { align-items: center; border-top: 1px solid #e1e8ed; flex-wrap: wrap; margin-top: .7rem; padding-top: .7rem; } .entry-actions form { display: flex; } .edit-details { display: grid; gap: .7rem; min-width: min(100%, 30rem); } .edit-details summary { list-style: none; } .edit-details summary::-webkit-details-marker { display: none; } .edit-form { border: 1px solid #d7dee5; display: grid; gap: .7rem; grid-template-columns: repeat(2, minmax(0, 1fr)); padding: .75rem; } .edit-form .check-field, .edit-form > button { grid-column: 1 / -1; }
  .usage-panel { border-color: #8ab69a; } .usage-list { max-height: 30dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .usage-row { align-items: center; border-bottom: 1px solid #e1e8ed; display: flex; flex-wrap: wrap; gap: .7rem; min-width: 0; padding: .55rem 0; } .usage-row strong { overflow-wrap: anywhere; } .usage-clear-form { border-top: 1px solid #e1e8ed; padding-top: .8rem; } .empty { color: #52606d; padding: 1.5rem; text-align: center; } .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination span { color: #52606d; font-size: .84rem; }
  @media (max-width: 900px) { .create-form, .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .filter-form > .button, .filter-form > a { width: 100%; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .panel-heading, .entry-header { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button { width: 100%; } .create-form, .filter-form, .edit-form { grid-template-columns: 1fr; } .filter-form > .button, .filter-form > a { width: 100%; } .batch-toolbar { align-items: stretch; flex-direction: column; } .batch-toolbar .button { width: 100%; } .regcode-entry { padding: .75rem; } .entry-header time { max-width: none; text-align: left; } .entry-actions { align-items: stretch; flex-direction: column; } .entry-actions > *, .entry-actions form, .entry-actions .button, .edit-details { width: 100%; } .edit-form .check-field, .edit-form > button { grid-column: auto; } .panel { padding: .85rem; } h1 { font-size: 1.65rem; } .pagination { align-items: stretch; } }
</style>
