<script lang="ts">
  import { t } from "$lib/i18n";
  import { safeImageURL } from "$lib/media";
  import type { AdminMediaRequest } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string });
  const statusOptions = ["pending", "accepted", "downloading", "rejected", "completed"] as const;
  const statusTabs = ["active", "pending", "accepted", "downloading", "rejected", "completed", "all"] as const;

  function dateLabel(value?: number): string {
    if (!value || value <= 0) return "-";
    const date = new Date(value * 1000);
    return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString("zh-CN");
  }

  function info(request: AdminMediaRequest, key: string): string {
    const value = request.media_info?.[key];
    return typeof value === "string" ? value : "";
  }

  function rating(request: AdminMediaRequest): string {
    const value = request.media_info?.["vote_average"] ?? request.media_info?.["rating"];
    const parsed = typeof value === "number" ? value : Number(value);
    return Number.isFinite(parsed) && parsed > 0 ? parsed.toFixed(1) : "";
  }

  function poster(request: AdminMediaRequest): string {
    return safeImageURL(request.media_info?.["poster_url"] || request.media_info?.["poster"]);
  }

  function title(request: AdminMediaRequest): string {
    return info(request, "title") || request.title || request.original_title || "-";
  }

  function members(request: AdminMediaRequest): AdminMediaRequest[] {
    return request.grouped_requests?.length ? request.grouped_requests : [request];
  }

  function groupKey(request: AdminMediaRequest): string {
    return request.group_key || request.require_key;
  }

  function isSplit(request: AdminMediaRequest): boolean {
    return data.query.split.includes(groupKey(request));
  }

  function statusLabel(status: string): string {
    const labels: Record<string, string> = {
      pending: t.adminRequestsStatusPending,
      accepted: t.adminRequestsStatusAccepted,
      downloading: t.adminRequestsStatusDownloading,
      rejected: t.adminRequestsStatusRejected,
      completed: t.adminRequestsStatusCompleted
    };
    return labels[status] || status || "-";
  }

  function sourceLabel(source: string): string {
    return source.toLowerCase() === "bangumi" ? "Bangumi" : "TMDB";
  }

  function statusCount(status: string): number {
    const counts = data.payload?.status_counts;
    if (!counts) return 0;
    return Number(counts[status as keyof typeof counts] || 0);
  }

  function listHref(page = data.payload?.page || data.query.page, split = data.query.split, status = data.query.status): string {
    const params = new URLSearchParams();
    params.set("status", status);
    params.set("source", data.query.source);
    params.set("page", String(Math.max(1, page)));
    params.set("per_page", String(data.query.per_page));
    if (data.query.query) params.set("q", data.query.query);
    if (split.length) params.set("split", split.map((key) => encodeURIComponent(key)).join(","));
    return `/admin/requests?${params.toString()}`;
  }

  function toggleSplit(request: AdminMediaRequest): string {
    const key = groupKey(request);
    const next = data.query.split.includes(key)
      ? data.query.split.filter((item) => item !== key)
      : [...data.query.split, key];
    return listHref(data.payload?.page || data.query.page, next);
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }
</script>

<svelte:head><title>{t.adminRequestsTitle} - {t.siteName}</title></svelte:head>

<section class="requests-page" aria-labelledby="requests-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="requests-title">{t.adminRequestsTitle}</h1><p class="muted">{t.adminRequestsDescription}</p></div>
    <a class="button secondary" href={listHref()}>{t.adminRequestsRefresh}</a>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "updated"}<p class="notice success" role="status">{t.adminRequestsUpdated}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.adminRequestsDeleted}</p>{/if}

  <section class="panel filters" aria-labelledby="request-filter-title">
    <div class="panel-heading"><div><h2 id="request-filter-title">{t.adminRequestsFilters}</h2><p class="muted">{t.adminRequestsTotal.replace("{count}", String(data.payload?.request_total || 0))}</p></div><span class="count">{data.payload?.total || 0} {t.adminRequestsGroups}</span></div>
    <nav class="status-tabs" aria-label={t.adminRequestsFilters}>
      {#each statusTabs as status}
        <a class:active={data.query.status === status} href={listHref(1, data.query.split, status)}>{status === "active" ? t.adminRequestsStatusActive : status === "all" ? t.adminRequestsStatusAll : statusLabel(status)} <span>{statusCount(status)}</span></a>
      {/each}
    </nav>
    <form method="GET" action="/admin/requests" class="filter-form">
      <input type="hidden" name="status" value={data.query.status} />
      <label class="search-field">{t.adminRequestsSearch}<input name="q" maxlength="120" value={data.query.query} placeholder={t.adminRequestsSearchPlaceholder} /></label>
      <label>{t.adminRequestsSource}<select name="source"><option value="all" selected={data.query.source === "all"}>全部来源</option><option value="tmdb" selected={data.query.source === "tmdb"}>TMDB</option><option value="bangumi" selected={data.query.source === "bangumi"}>Bangumi</option></select></label>
      <label>{t.adminRequestsPerPage}<select name="per_page">{#each [20, 50, 100] as size}<option value={size} selected={data.query.per_page === size}>{size}</option>{/each}</select></label>
      <button class="button primary" type="submit">{t.adminRequestsApply}</button>
      <a class="button secondary" href="/admin/requests">{t.adminRequestsReset}</a>
    </form>
  </section>

  <section class="panel list-panel" aria-labelledby="request-list-title">
    <header class="panel-heading"><div><h2 id="request-list-title">{t.adminRequestsQueue}</h2><p class="muted">{t.adminRequestsPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(Math.max(1, data.payload?.total_pages || 1)))}</p></div></header>
    {#if data.payload?.requests?.length}
      <div class="request-list">
        {#each data.payload.requests as group (groupKey(group))}
          {@const groupMembers = members(group)}
          {#if groupMembers.length > 1 && isSplit(group)}
            <div class="group-banner"><strong>{t.adminRequestsSplitGroup}</strong><span>{t.adminRequestsGroupCount.replace("{count}", String(groupMembers.length))}</span><a class="button compact secondary" href={toggleSplit(group)}>{t.adminRequestsMergeGroup}</a></div>
            {#each groupMembers as request (request.require_key)}{@render requestCard(request, [request], true)}{/each}
          {:else}
            {@render requestCard(group, groupMembers, false)}
          {/if}
        {/each}
      </div>
    {:else}<p class="empty">{t.adminRequestsEmpty}</p>{/if}
    {#if (data.payload?.total_pages || 1) > 1}
      <nav class="pagination" aria-label={t.adminRequestsPagination}>
        {#if (data.payload?.page || data.query.page) > 1}<a class="button secondary" href={listHref((data.payload?.page || data.query.page) - 1)}>{t.adminRequestsPrevious}</a>{:else}<span></span>{/if}
        <span>{data.payload?.page || data.query.page} / {data.payload?.total_pages || 1}</span>
        {#if (data.payload?.page || data.query.page) < (data.payload?.total_pages || 1)}<a class="button secondary" href={listHref((data.payload?.page || data.query.page) + 1)}>{t.adminRequestsNext}</a>{:else}<span></span>{/if}
      </nav>
    {/if}
  </section>
</section>

{#snippet queryFields()}
  <input type="hidden" name="status" value={data.query.status} /><input type="hidden" name="source" value={data.query.source} /><input type="hidden" name="q" value={data.query.query} /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="split" value={data.query.split.map((key) => encodeURIComponent(key)).join(",")} />
{/snippet}

{#snippet requestCard(request: AdminMediaRequest, groupMembers: AdminMediaRequest[], split: boolean)}
  <article class:split-card={split} class="request-card">
    <div class="request-main">
      {#if poster(request)}<img class="poster" src={poster(request)} alt={title(request)} loading="lazy" />{:else}<div class="poster placeholder" aria-hidden="true">?</div>{/if}
      <div class="request-content">
        <div class="title-row"><h3>{title(request)}</h3><span class:source-bangumi={sourceLabel(request.source) === "Bangumi"} class="source-badge">{sourceLabel(request.source)}</span>{#if rating(request)}<span class="rating">★ {rating(request)}</span>{/if}{#if groupMembers.length > 1 && !split}<span class="group-badge">{t.adminRequestsGroupCount.replace("{count}", String(groupMembers.length))}</span>{/if}</div>
        <p class="meta">{request.original_title || "-"} · {request.media_type || "-"}{#if request.season} · {t.adminRequestsSeason} {request.season}{/if} · {dateLabel(request.timestamp)}</p>
        {#if request.user}<p class="meta">{t.adminRequestsRequester}: {request.user.username || `UID ${request.user.uid || request.user.telegram_id || "-"}`}</p>{/if}
        {#if info(request, "overview")}<p class="overview">{info(request, "overview")}</p>{/if}
        {#if request.note}<p class="note">{t.adminRequestsUserNote}: {request.note}</p>{/if}
        {#if request.admin_note}<p class="admin-note">{t.adminRequestsAdminNote}: {request.admin_note}</p>{/if}
        <p class="key-line"><code>{request.require_key}</code> · #{request.id} · {t.adminRequestsRevision} {request.revision}</p>
      </div>
    </div>
    <div class="request-actions">
      {#if groupMembers.length > 1 && !split}
        <form method="POST" action="?/updateGroup" class="update-form" onsubmit={(event) => confirmSubmit(event, t.adminRequestsUpdateConfirm)}>
          {@render queryFields()}
          <input type="hidden" name="items" value={JSON.stringify(groupMembers.map((item) => ({ require_key: item.require_key, revision: item.revision })))} />
          <label>{t.adminRequestsStatus}<select name="status" required>{#each statusOptions as value}<option value={value} selected={value === groupMembers[0].status}>{statusLabel(value)}</option>{/each}</select></label>
          <input name="note" maxlength="1000" placeholder={t.adminRequestsNotePlaceholder} />
          <button class="button primary" type="submit">{t.adminRequestsHandleGroup}</button>
        </form>
        <a class="button compact secondary" href={toggleSplit(request)}>{t.adminRequestsSplitGroup}</a>
      {:else}
        <form method="POST" action="?/update" class="update-form" onsubmit={(event) => confirmSubmit(event, t.adminRequestsUpdateConfirm)}>
          {@render queryFields()}
          <input type="hidden" name="require_key" value={request.require_key} /><input type="hidden" name="revision" value={request.revision} />
          <label>{t.adminRequestsStatus}<select name="status" required>{#each statusOptions as value}<option value={value} selected={value === request.status}>{statusLabel(value)}</option>{/each}</select></label>
          <input name="note" maxlength="1000" value={request.admin_note || ""} placeholder={t.adminRequestsNotePlaceholder} />
          <button class="button primary" type="submit">{t.adminRequestsHandle}</button>
        </form>
      {/if}
      <form method="POST" action="?/delete" onsubmit={(event) => confirmSubmit(event, t.adminRequestsDeleteConfirm)}>
        {@render queryFields()}
        <input type="hidden" name="require_key" value={request.require_key} /><input type="hidden" name="revision" value={request.revision} />
        <button class="button danger compact" type="submit">{t.adminRequestsDelete}</button>
      </form>
    </div>
  </article>
{/snippet}

<style>
  .requests-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .panel-heading, .title-row, .meta, .request-main, .request-actions, .update-form, .pagination, .group-banner { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .pagination { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; } h3 { font-size: 1.05rem; }
  .muted, .meta, .overview, .note, .admin-note { color: #52606d; margin: .35rem 0 0; } .meta { flex-wrap: wrap; font-size: .8rem; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.5rem; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; }
  .panel, .notice { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .notice { margin: 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; } .count, .group-badge, .rating, .source-badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; font-size: .75rem; padding: .2rem .5rem; white-space: nowrap; }
  .filters, .list-panel { display: grid; gap: .9rem; } .status-tabs { display: flex; gap: .35rem; overflow-x: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .status-tabs a { color: #36566a; min-height: 2.4rem; padding: .55rem .7rem; text-decoration: none; white-space: nowrap; } .status-tabs a.active { border-bottom: 3px solid #245b75; color: #16394a; font-weight: 700; } .status-tabs span { color: #52606d; font-size: .75rem; }
  .filter-form { align-items: end; display: grid; gap: .75rem; grid-template-columns: minmax(0, 2fr) minmax(10rem, 1fr) minmax(7rem, .7fr) auto auto; } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .request-list { display: grid; gap: .7rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding: .1rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .request-card { border: 1px solid #c8d2da; display: grid; gap: .8rem; min-width: 0; padding: .9rem; } .split-card { background: #f8fafb; border-left: 4px solid #9fb3c8; } .request-main { min-width: 0; } .poster { background: #eef2f4; border: 1px solid #d7dee5; border-radius: .3rem; flex: 0 0 4.5rem; height: 6.2rem; object-fit: contain; width: 4.5rem; } .poster.placeholder { align-items: center; color: #7b8794; display: flex; font-size: 1.5rem; justify-content: center; } .request-content { display: grid; gap: .25rem; min-width: 0; } .title-row { align-items: center; flex-wrap: wrap; } .source-bangumi { background: #fff4df; border-color: #e9c46a; color: #7b4f00; } .rating { color: #7b4f00; } .overview, .note, .admin-note { font-size: .82rem; line-height: 1.45; } .admin-note { color: #245b75; } .key-line { color: #7b8794; font-size: .75rem; margin: .3rem 0 0; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; max-width: 100%; overflow-wrap: anywhere; padding: .12rem .3rem; }
  .request-actions { align-items: end; flex-wrap: wrap; justify-content: flex-end; } .update-form { align-items: end; flex: 1 1 32rem; flex-wrap: wrap; } .update-form label { min-width: 10rem; } .update-form input { flex: 1 1 12rem; } .group-banner { align-items: center; background: #eef2f4; border: 1px solid #c8d2da; flex-wrap: wrap; justify-content: space-between; padding: .6rem .75rem; }
  .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .8rem; } .empty { color: #52606d; padding: 2rem 1rem; text-align: center; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .search-field { grid-column: span 2; } .filter-form > .button, .filter-form > a { width: 100%; } .request-actions { align-items: stretch; flex-direction: column; } .update-form { flex-basis: auto; } .request-actions > form:last-child { align-self: flex-start; } }
  @media (max-width: 600px) { .page-heading, .panel-heading, .request-main, .request-actions, .update-form, .pagination { align-items: stretch; flex-direction: column; } .page-heading > .button, .filter-form > .button, .filter-form > a, .update-form > *, .request-actions > form, .request-actions > form > .button { width: 100%; } .filter-form { grid-template-columns: 1fr; } .search-field { grid-column: auto; } .poster { align-self: flex-start; } .panel { padding: .85rem; } .page-heading { gap: .9rem; } }
</style>
