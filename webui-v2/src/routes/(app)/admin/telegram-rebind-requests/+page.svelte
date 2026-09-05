<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: { action?: string; error?: string } | null } = $props();

  const filters = [
    ["all", t.adminTelegramRebindFilterAll],
    ["pending", t.adminTelegramRebindFilterPending],
    ["approved", t.adminTelegramRebindFilterApproved],
    ["rejected", t.adminTelegramRebindFilterRejected],
    ["revoked", t.adminTelegramRebindFilterRevoked]
  ] as const;

  function dateLabel(value: number | null | undefined): string {
    return value ? new Date(value * 1000).toLocaleString("zh-CN") : t.adminTelegramRebindNone;
  }

  function statusLabel(value: string): string {
    return ({
      pending: t.adminTelegramRebindPending,
      approved: t.adminTelegramRebindApproved,
      rejected: t.adminTelegramRebindRejected,
      revoked: t.adminTelegramRebindStatusRevoked
    } as Record<string, string>)[value] || value;
  }

  function pageCount(): number {
    return Math.max(1, Math.ceil((data.payload?.total || 0) / data.perPage));
  }

  function pageHref(page: number): string {
    return `?status=${encodeURIComponent(data.status)}&page=${page}`;
  }

  function filterHref(status: string): string {
    return `?status=${encodeURIComponent(status)}&page=1`;
  }
</script>

<svelte:head><title>{t.adminTelegramRebindTitle} - {t.siteName}</title></svelte:head>

<section class="rebind-page" aria-labelledby="rebind-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="rebind-title">{t.adminTelegramRebindTitle}</h1>
      <p class="muted">{t.adminTelegramRebindDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/admin/telegram">{t.adminTelegramTitle}</a>
      <a class="button secondary" href={pageHref(data.page)}>{t.adminTelegramRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<div class="notice error" role="alert">{t.adminTelegramRebindLoadFailed}</div>{/if}
  {#if form?.error}<div class="notice error" role="alert">{form.error}</div>{/if}
  {#if data.notice === "reviewed"}<div class="notice success" role="status">{t.adminTelegramRebindReviewed}</div>{/if}
  {#if data.notice === "batch_reviewed"}<div class="notice success" role="status">{t.adminTelegramRebindBatchReviewed}</div>{/if}
  {#if data.notice === "revoked"}<div class="notice success" role="status">{t.adminTelegramRebindRevokedNotice}</div>{/if}

  <nav class="filter-tabs" aria-label={t.adminTelegramRebindTitle}>
    {#each filters as [value, label]}
      <a class:active={data.status === value} href={filterHref(value)}>{label}</a>
    {/each}
  </nav>

  <section class="panel batch-panel" aria-labelledby="batch-title">
    <header class="panel-heading">
      <div><h2 id="batch-title">{t.adminTelegramRebindBatchHint}</h2><p class="muted">{t.adminTelegramRebindTotal.replace("{count}", String(data.payload?.total || 0))}</p></div>
      <div class="batch-actions">
        <form id="batch-review-form" method="POST" action="?/batch">
          <label>{t.adminTelegramRebindNote}<input name="admin_note" maxlength="500" placeholder={t.adminTelegramRebindNotePlaceholder} /></label>
          <button class="button secondary" name="review_action" value="approve" type="submit">{t.adminTelegramRebindBatchApprove}</button>
          <button class="button danger" name="review_action" value="reject" type="submit">{t.adminTelegramRebindBatchReject}</button>
        </form>
      </div>
    </header>
  </section>

  {#if data.payload?.requests.length}
    <section class="request-list" aria-label={t.adminTelegramRebindTitle}>
      {#each data.payload.requests as request (request.id)}
        <article class="request-row">
          <div class="request-main">
            <div class="request-title">
              {#if request.status === "pending"}<input form="batch-review-form" type="checkbox" name="ids" value={request.id} aria-label={t.adminTelegramRebindSelect} />{/if}
              <strong>{request.username || `${t.adminTelegramRebindUID} ${request.uid}`}</strong>
              <span class:pending={request.status === "pending"} class="status">{statusLabel(request.status)}</span>
            </div>
            <dl>
              <div><dt>{t.adminTelegramRebindUID}</dt><dd>{request.uid}</dd></div>
              <div><dt>{t.adminTelegramRebindOldID.replace("{id}", "")}</dt><dd>{request.old_telegram_id || t.adminTelegramRebindNone}</dd></div>
              <div><dt>{t.adminTelegramRebindSubmittedAt.replace("{time}", "")}</dt><dd>{dateLabel(request.created_at)}</dd></div>
              {#if request.reviewed_at}<div><dt>{t.adminTelegramRebindReviewedAt.replace("{time}", "")}</dt><dd>{dateLabel(request.reviewed_at)}</dd></div>{/if}
            </dl>
            {#if request.reason}<p class="detail">{t.adminTelegramRebindReason.replace("{reason}", request.reason)}</p>{/if}
            {#if request.admin_note}<p class="detail note">{t.adminTelegramRebindAdminNote.replace("{note}", request.admin_note)}</p>{/if}
          </div>
          {#if request.status === "pending"}
            <form class="row-actions" method="POST" action="?/review">
              <input type="hidden" name="id" value={request.id} />
              <label>{t.adminTelegramRebindNote}<input name="admin_note" maxlength="500" placeholder={t.adminTelegramRebindNotePlaceholder} /></label>
              <button class="button secondary" name="review_action" value="approve" type="submit">{t.adminTelegramRebindApprove}</button>
              <button class="button danger" name="review_action" value="reject" type="submit">{t.adminTelegramRebindReject}</button>
            </form>
          {/if}
        </article>
      {/each}
    </section>
  {:else}
    <div class="empty">{t.adminTelegramRebindEmpty}</div>
  {/if}

  {#if pageCount() > 1}
    <nav class="pagination" aria-label={t.adminTelegramRebindTitle}>
      {#if data.page > 1}<a class="button secondary" href={pageHref(data.page - 1)}>{t.adminTelegramRebindPrevious}</a>{:else}<span></span>{/if}
      <span>{t.adminTelegramRebindPage.replace("{page}", String(data.page)).replace("{pages}", String(pageCount()))}</span>
      {#if data.page < pageCount()}<a class="button secondary" href={pageHref(data.page + 1)}>{t.adminTelegramRebindNext}</a>{:else}<span></span>{/if}
    </nav>
  {/if}

  <section class="panel revoke-panel" aria-labelledby="revoke-title">
    <header class="panel-heading"><div><h2 id="revoke-title">{t.adminTelegramRebindRevokeTitle}</h2><p class="muted">{t.adminTelegramRebindRevokeHelp}</p></div></header>
    <form method="POST" action="?/revoke" class="revoke-form">
      <label>{t.adminTelegramRebindConfirmLabel}<input name="confirm" required maxlength="64" placeholder={t.adminTelegramRebindConfirmPlaceholder} /></label>
      <label>{t.adminTelegramRebindNote}<input name="admin_note" maxlength="500" placeholder={t.adminTelegramRebindNotePlaceholder} /></label>
      <button class="button danger" type="submit">{t.adminTelegramRebindRevoke}</button>
    </form>
  </section>
</section>

<style>
  .rebind-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading { align-items: flex-start; display: flex; gap: .8rem; }
  .page-heading, .panel-heading { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1rem; }
  .heading-actions, .batch-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; }
  .muted { color: #52606d; margin: .35rem 0 0; }
  .text-link { min-height: 2.45rem; padding: .5rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.45rem; padding: .5rem .8rem; text-decoration: none; white-space: normal; }
  .button:hover { background: #d6e1e7; } .button.danger { background: #b42318; color: #fff; } .button.danger:hover { background: #8f1d14; }
  .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .panel, .request-row { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; }
  .filter-tabs { display: flex; flex-wrap: wrap; gap: .45rem; overflow-x: auto; overscroll-behavior: contain; padding-bottom: .1rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .filter-tabs a { border-bottom: 2px solid transparent; color: #243b53; min-height: 2.45rem; padding: .6rem .65rem; text-decoration: none; white-space: nowrap; } .filter-tabs a.active { border-color: #245b75; color: #16394a; font-weight: 700; }
  #batch-review-form, .row-actions, .revoke-form { align-items: end; display: flex; flex-wrap: wrap; gap: .55rem; }
  label { color: #243b53; display: grid; gap: .3rem; font-weight: 650; min-width: 10rem; } input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.45rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .request-list { display: grid; gap: .7rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .request-row { align-items: start; display: flex; gap: 1rem; justify-content: space-between; } .request-main { flex: 1; min-width: 0; } .request-title { align-items: center; display: flex; flex-wrap: wrap; gap: .55rem; } .request-title input { min-height: 1.1rem; width: 1.1rem; } .request-title strong { overflow-wrap: anywhere; } .status { background: #eef2f4; border-radius: .25rem; color: #52606d; font-size: .82rem; padding: .25rem .45rem; } .status.pending { background: #fff8e6; color: #7b4f00; }
  dl { display: flex; flex-wrap: wrap; gap: .5rem 1rem; margin: .7rem 0 0; } dl div { min-width: 8rem; } dt { color: #52606d; font-size: .78rem; } dd { margin: .15rem 0 0; overflow-wrap: anywhere; } .detail { color: #52606d; margin: .65rem 0 0; } .detail.note { color: #245b75; }
  .row-actions { flex-shrink: 0; max-width: 24rem; } .row-actions label { flex: 1 1 100%; } .empty { background: #fff; border: 1px dashed #c8d2da; color: #52606d; padding: 2rem 1rem; text-align: center; }
  .pagination { align-items: center; display: grid; gap: .8rem; grid-template-columns: minmax(5rem, 1fr) auto minmax(5rem, 1fr); text-align: center; } .pagination > :last-child { justify-self: end; }
  .revoke-panel { border-color: #e9c46a; } .revoke-form { align-items: end; } .revoke-form label { flex: 1 1 14rem; }
  @media (max-width: 760px) { .page-heading, .panel-heading, .request-row { flex-direction: column; } .heading-actions, .batch-actions, #batch-review-form, .row-actions, .revoke-form { align-items: stretch; width: 100%; } .heading-actions > *, #batch-review-form > *, .row-actions > *, .revoke-form > * { width: 100%; } .request-list { max-height: none; overflow: visible; } .panel, .request-row { padding: .85rem; } }
</style>
