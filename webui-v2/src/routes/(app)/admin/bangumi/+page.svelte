<script lang="ts">
  import { t } from "$lib/i18n";
  import type { AdminBangumiPageData } from "$lib/types";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();

  function href(overrides: Partial<AdminBangumiPageData["query"]> = {}): string {
    const query = { ...data.query, ...overrides };
    const params = new URLSearchParams();
    if (query.page > 1) params.set("page", String(query.page));
    if (query.per_page !== 20) params.set("per_page", String(query.per_page));
    if (query.search) params.set("search", query.search);
    if (query.detail && query.uid > 0) {
      params.set("detail", query.detail);
      params.set("uid", String(query.uid));
    }
    const value = params.toString();
    return value ? `/admin/bangumi?${value}` : "/admin/bangumi";
  }

  function formatTime(value: number): string {
    if (!value || !Number.isFinite(value)) return "-";
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function formatDuration(seconds: number): string {
    if (!seconds || seconds < 1) return "-";
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const rest = seconds % 60;
    if (hours > 0) return `${hours} ${t.adminBangumiHours}${minutes ? ` ${minutes} ${t.adminBangumiMinutes}` : ""}`;
    if (minutes > 0) return `${minutes} ${t.adminBangumiMinutes}`;
    return `${rest} ${t.adminBangumiSeconds}`;
  }

  function replace(template: string, key: string, value: string | number): string {
    return template.replace(`{${key}}`, String(value));
  }

  function logStatus(status: string): string {
    if (status === "success") return t.adminBangumiSuccess;
    if (status === "failed") return t.adminBangumiFailure;
    if (status === "pending") return t.adminBangumiPending;
    return t.adminBangumiUnknown;
  }

  function detailTitle(): string {
    if (!data.detail) return "";
    const name = data.detail.user?.username || t.adminBangumiUnknown;
    return replace(replace(t.adminBangumiDetailFor, "username", name), "uid", data.detail.uid);
  }

</script>

<svelte:head><title>{t.adminBangumiTitle} - {t.siteName}</title></svelte:head>

<section class="page" aria-labelledby="bangumi-admin-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="bangumi-admin-title">{t.adminBangumiTitle}</h1>
      <p class="muted">{t.adminBangumiDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="button secondary" href="/admin">{t.adminBangumiBack}</a>
      <a class="button secondary" href="/admin/config">{t.adminBangumiOpenConfig}</a>
      <a class="button secondary" href={href({})}>{t.adminBangumiRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if data.notice === "synced"}<p class="notice success" role="status">{t.adminBangumiSyncDone}</p>{/if}
  {#if data.notice === "logs_cleared"}<p class="notice success" role="status">{t.adminBangumiLogsCleared}</p>{/if}

  <section class="panel config-panel" aria-labelledby="config-title">
    <header class="panel-heading">
      <div><h2 id="config-title">{t.adminBangumiConfigTitle}</h2><p class="muted">{t.adminBangumiConfigDescription}</p></div>
    </header>
    {#if data.info?.features}
      <div class="config-grid">
        <div><span>{t.adminBangumiSyncFeature}</span><strong class:enabled={data.info.features.bangumi_sync}>{data.info.features.bangumi_sync ? t.adminBangumiEnabled : t.adminBangumiDisabled}</strong></div>
        <div><span>{t.adminBangumiManageFeature}</span><strong class:enabled={data.info.features.bangumi_manage}>{data.info.features.bangumi_manage ? t.adminBangumiEnabled : t.adminBangumiDisabled}</strong></div>
      </div>
    {:else}<p class="empty">{t.adminBangumiConfigUnavailable}</p>{/if}
  </section>

  <section class="panel" aria-labelledby="users-title">
    <header class="panel-heading">
      <div><h2 id="users-title">{t.adminBangumiTitle}</h2>{#if data.users}<p class="muted">{replace(t.adminBangumiUserCount, "count", data.users.total)}</p>{/if}</div>
    </header>
    <form class="search-form" method="GET" action="/admin/bangumi">
      <input name="search" value={data.query.search} maxlength="100" placeholder={t.adminBangumiSearchPlaceholder} aria-label={t.adminBangumiSearch} />
      <input type="hidden" name="page" value="1" />
      <input type="hidden" name="per_page" value={data.query.per_page} />
      <button class="button primary" type="submit">{t.adminBangumiApply}</button>
      {#if data.query.search}<a class="button secondary" href={href({ search: "", page: 1 })}>{t.adminBangumiClearSearch}</a>{/if}
    </form>

    {#if data.users?.users.length}
      <div class="user-list">
        {#each data.users.users as user (user.uid)}
          <article class="user-card">
            <header class="user-heading">
              <div class="user-identity"><strong>{user.username}</strong><span>{replace(t.adminBangumiUserID, "uid", user.uid)}</span></div>
              <div class="badges">
                <span class="badge">{user.bgm_mode ? t.adminBangumiSyncOn : t.adminBangumiSyncOff}</span>
                <span class="badge">{user.bgm_manage_mode ? t.adminBangumiManageOn : t.adminBangumiManageOff}</span>
                <span class="badge">{user.token_set ? t.adminBangumiTokenSet : t.adminBangumiTokenMissing}</span>
                <span class:ready={user.sync_ready} class="badge">{user.sync_ready ? t.adminBangumiReady : t.adminBangumiNotReady}</span>
              </div>
            </header>
            <div class="user-meta"><span>{replace(t.adminBangumiRecordCount, "count", user.record_count)}</span><span>{replace(t.adminBangumiSyncedCount, "count", user.sync_count)}</span></div>
            <div class="row-actions">
              <a class="button small" href={href({ detail: "records", uid: user.uid })}>{t.adminBangumiViewRecords}</a>
              <a class="button small" href={href({ detail: "logs", uid: user.uid })}>{t.adminBangumiViewLogs}</a>
              <form method="POST" action="?/sync" onsubmit={(event) => { if (!window.confirm(t.adminBangumiSyncConfirm)) event.preventDefault(); }}>
                <input type="hidden" name="uid" value={user.uid} />
                <input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} />
                <button class="button small primary" type="submit" disabled={!user.sync_ready}>{t.adminBangumiSyncNow}</button>
              </form>
              <form method="POST" action="?/clearLogs" onsubmit={(event) => { if (!window.confirm(t.adminBangumiClearLogsConfirm)) event.preventDefault(); }}>
                <input type="hidden" name="uid" value={user.uid} />
                <input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="detail" value="logs" />
                <button class="button small danger" type="submit" disabled={user.sync_count === 0}>{t.adminBangumiClearLogs}</button>
              </form>
            </div>
          </article>
        {/each}
      </div>
    {:else}<p class="empty">{t.adminBangumiNoUsers}</p>{/if}

    {#if data.users && data.users.pages > 1}
      <nav class="pagination" aria-label={t.adminBangumiTitle}>
        {#if data.users.page > 1}<a class="button secondary" href={href({ page: data.users.page - 1 })}>{t.adminBangumiPrevious}</a>{:else}<span></span>{/if}
        <span>{replace(replace(t.adminBangumiPageOf, "page", data.users.page), "pages", data.users.pages)}</span>
        {#if data.users.page < data.users.pages}<a class="button secondary" href={href({ page: data.users.page + 1 })}>{t.adminBangumiNext}</a>{:else}<span></span>{/if}
      </nav>
    {/if}
  </section>

  {#if data.detail}
    <section class="panel detail-panel" aria-labelledby="detail-title">
      <header class="panel-heading">
        <div><h2 id="detail-title">{data.detail.kind === "records" ? t.adminBangumiDetailRecords : t.adminBangumiDetailLogs}</h2><p class="muted">{detailTitle()}</p></div>
        <a class="button secondary" href={href({ detail: "", uid: 0 })}>{t.adminBangumiCloseDetail}</a>
      </header>
      {#if data.detail.error}<p class="notice error" role="alert">{data.detail.error}</p>
      {:else if data.detail.kind === "records"}
        {#if data.detail.records.length}
          <div class="table-region"><table><thead><tr><th>{t.adminBangumiItem}</th><th>{t.adminBangumiSeries}</th><th>{t.adminBangumiType}</th><th>{t.adminBangumiEpisode}</th><th>{t.adminBangumiPlayedAt}</th><th>{t.adminBangumiDuration}</th></tr></thead><tbody>
            {#each data.detail.records as record (record.item_id + record.played_at)}<tr><td><strong>{record.title || t.adminBangumiUnknown}</strong>{#if record.synced_name}<small>{replace(t.adminBangumiSyncedSubject, "name", record.synced_name)}</small>{/if}</td><td>{record.series_name || "-"}</td><td>{record.media_type || "-"}</td><td>{record.index_number || "-"}</td><td>{formatTime(record.played_at)}</td><td>{formatDuration(record.duration)}</td></tr>{/each}
          </tbody></table></div>
        {:else}<p class="empty">{t.adminBangumiNoRecords}</p>{/if}
      {:else if data.detail.logs.length}
        <div class="table-region"><table><thead><tr><th>{t.adminBangumiLogStatus}</th><th>{t.adminBangumiLogSubject}</th><th>{t.adminBangumiLogMessage}</th><th>{t.adminBangumiLogTime}</th></tr></thead><tbody>
          {#each data.detail.logs as log (log.id)}<tr><td><span class:good={log.status === "success"} class:bad={log.status === "failed"}>{logStatus(log.status)}</span></td><td>{log.subject_name || "-"}{#if log.episode}<small>#{log.episode}</small>{/if}</td><td>{log.message || "-"}</td><td>{formatTime(log.created_at)}</td></tr>{/each}
        </tbody></table></div>
      {:else}<p class="empty">{t.adminBangumiNoLogs}</p>{/if}
    </section>
  {/if}
</section>

<style>
  .page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .user-heading, .row-actions, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .user-heading, .pagination { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions, .row-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p, strong, span, small { overflow-wrap: anywhere; } h1, h2, p { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; }
  .muted { color: #52606d; line-height: 1.5; margin-top: .35rem; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .8rem; text-align: center; text-decoration: none; white-space: normal; }
  .button:hover { background: #d6e1e7; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.danger { background: #a63d40; color: #fff; } .button.danger:disabled, .button:disabled { cursor: not-allowed; opacity: .55; }
  .button.small { min-height: 2.25rem; padding: .4rem .6rem; }
  .panel, .notice { border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .panel { background: #fff; display: grid; gap: .85rem; } .notice { display: grid; gap: .35rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .config-grid { display: grid; gap: .65rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .config-grid div { border: 1px solid #d7dee5; display: flex; gap: .6rem; justify-content: space-between; min-width: 0; padding: .7rem; } .config-grid span { color: #52606d; } .config-grid strong { color: #a63d40; } .config-grid strong.enabled { color: #276749; }
  .search-form { align-items: stretch; display: grid; gap: .65rem; grid-template-columns: minmax(0, 1fr) auto auto; } input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .user-list { display: grid; gap: .7rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .user-card { border: 1px solid #d7dee5; display: grid; gap: .7rem; min-width: 0; padding: .85rem; } .user-heading { align-items: center; } .user-identity { display: grid; gap: .2rem; min-width: 0; } .user-identity span, .user-meta, small { color: #52606d; font-size: .78rem; } .user-meta { display: flex; flex-wrap: wrap; gap: 1rem; }
  .badges { display: flex; flex-wrap: wrap; gap: .35rem; justify-content: flex-end; min-width: 0; } .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .72rem; padding: .2rem .45rem; white-space: nowrap; } .badge.ready { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .row-actions form { display: inline-flex; max-width: 100%; } .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination > span { color: #52606d; font-size: .84rem; }
  .table-region { max-height: 62dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } table { border-collapse: collapse; min-width: 48rem; width: 100%; } th, td { border-bottom: 1px solid #e1e8ed; padding: .65rem; text-align: left; vertical-align: top; } th { background: #f4f7f8; color: #52606d; font-size: .8rem; position: sticky; top: 0; z-index: 1; } td { overflow-wrap: anywhere; } td strong, td small { display: block; } td small { margin-top: .2rem; }
  .good { color: #276749; } .bad { color: #a61b1b; } .empty { color: #52606d; padding: 1.5rem; text-align: center; }
  @media (max-width: 700px) { .page-heading, .heading-actions, .panel-heading, .user-heading { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button, .panel-heading .button { width: 100%; } .search-form { grid-template-columns: 1fr; } .search-form .button { width: 100%; } .config-grid { grid-template-columns: 1fr; } .badges { justify-content: flex-start; } .user-list { max-height: none; } .row-actions .button, .row-actions form { flex: 1 1 10rem; } .row-actions form .button { width: 100%; } .pagination { align-items: stretch; flex-wrap: wrap; } .pagination > span { flex: 1 1 100%; order: -1; text-align: center; } .panel, .notice { padding: .85rem; } h1 { font-size: 1.65rem; } }
</style>
