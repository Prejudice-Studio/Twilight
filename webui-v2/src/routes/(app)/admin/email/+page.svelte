<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  type FormState = {
    action?: string;
    error?: string;
    test?: { success: boolean; email: string };
  };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);

  const purposeLabels: Record<string, string> = {
    bind: t.adminEmailPurposeBind,
    reset_password: t.adminEmailPurposeReset,
    change_password: t.adminEmailPurposeChangePassword,
    change_emby_password: t.adminEmailPurposeChangeEmby
  };

  function dateLabel(value: number | null | undefined): string {
    return value && value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : t.adminEmailNone;
  }

  function purposeLabel(value: string): string {
    return purposeLabels[value] || value || t.adminEmailNone;
  }

  function totalPages(): number {
    const pages = data.payload?.pages?.[data.view];
    return Math.max(1, pages || Math.ceil((data.payload?.total?.[data.view] || 0) / data.perPage));
  }

  function href(view: "pending" | "accounts", page = 1, search = data.view === view ? data.search : "", verified = view === "accounts" ? data.verified : "all"): string {
    const params = new URLSearchParams({ view, page: String(Math.max(1, page)), per_page: String(data.perPage) });
    if (search) params.set("search", search);
    if (view === "accounts" && verified !== "all") params.set("verified", verified);
    return `/admin/email?${params}`;
  }

  function submitConfirmation(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function hiddenQuery(): Array<[string, string | number]> {
    return [
      ["view", data.view],
      ["page", data.page],
      ["per_page", data.perPage],
      ["search", data.search],
      ["verified", data.verified]
    ];
  }
</script>

<svelte:head><title>{t.adminEmailTitle} - {t.siteName}</title></svelte:head>

<section class="email-page" aria-labelledby="email-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="email-title">{t.adminEmailTitle}</h1>
      <p class="muted">{t.adminEmailDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/admin/config">{t.adminEmailOpenConfig}</a>
      <a class="button secondary" href={href(data.view, data.page)}>{t.adminEmailRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "revoked"}<p class="notice success" role="status">{t.adminEmailRevokeDone}</p>{/if}
  {#if data.notice === "cleaned"}<p class="notice success" role="status">{t.adminEmailCleanupNotice}</p>{/if}
  {#if data.notice === "cleared"}<p class="notice success" role="status">{t.adminEmailClearUnverifiedNotice}</p>{/if}

  {#if data.payload}
    <section class="summary-panel" aria-label={t.adminEmailTitle}>
      <div class:good={data.payload.smtp_configured} class:warning={!data.payload.smtp_configured} class="summary-chip">{data.payload.smtp_configured ? t.adminEmailSMTPConfigured : t.adminEmailSMTPNotConfigured}</div>
      <div class="summary-chip">{data.payload.force_bind ? t.adminEmailForceBindOn : t.adminEmailForceBindOff}</div>
      <div class="summary-chip">{t.adminEmailSummaryPending.replace("{count}", String(data.payload.summary.total_pending))}</div>
      <div class:warning={data.payload.summary.expired_pending > 0} class="summary-chip">{t.adminEmailSummaryExpired.replace("{count}", String(data.payload.summary.expired_pending))}</div>
      <div class="summary-chip">{t.adminEmailSummaryWithEmail.replace("{count}", String(data.payload.summary.total_with_email))}</div>
      <div class:good={data.payload.summary.verified > 0} class="summary-chip">{t.adminEmailSummaryVerified.replace("{count}", String(data.payload.summary.verified))}</div>
      <div class:warning={data.payload.summary.unverified > 0} class="summary-chip">{t.adminEmailSummaryUnverified.replace("{count}", String(data.payload.summary.unverified))}</div>
    </section>
  {/if}

  <nav class="tabs" aria-label={t.adminEmailTitle}>
    <a class:active={data.view === "pending"} href={href("pending")}>{t.adminEmailTabPending}{#if data.payload}<span>{data.payload.summary.total_pending}</span>{/if}</a>
    <a class:active={data.view === "accounts"} href={href("accounts")}>{t.adminEmailTabAccounts}{#if data.payload}<span>{data.payload.summary.total_with_email}</span>{/if}</a>
  </nav>

  <section class="panel" aria-labelledby="list-title">
    <header class="panel-heading">
      <div>
        <h2 id="list-title">{data.view === "pending" ? t.adminEmailTabPending : t.adminEmailTabAccounts}</h2>
        <p class="muted">{t.adminEmailPageOf.replace("{page}", String(data.page)).replace("{pages}", String(totalPages()))}</p>
      </div>
    </header>

    <form method="GET" action="/admin/email" class="filter-form">
      <input type="hidden" name="view" value={data.view} />
      <input type="hidden" name="page" value="1" />
      <label class="search-label">{t.adminEmailEmail}
        <input name="search" maxlength="120" value={data.search} placeholder={data.view === "pending" ? t.adminEmailSearchPending : t.adminEmailSearchAccounts} />
      </label>
      {#if data.view === "accounts"}
        <label>{t.adminEmailStatus}
          <select name="verified">
            <option value="all" selected={data.verified === "all"}>{t.adminEmailFilterAll}</option>
            <option value="verified" selected={data.verified === "verified"}>{t.adminEmailFilterVerified}</option>
            <option value="unverified" selected={data.verified === "unverified"}>{t.adminEmailFilterUnverified}</option>
          </select>
        </label>
      {/if}
      <label>{t.adminAuditLogPerPage}
        <select name="per_page">
          {#each [25, 50, 100] as size}<option value={size} selected={data.perPage === size}>{size}</option>{/each}
        </select>
      </label>
      <button class="button primary" type="submit">{t.adminEmailApply}</button>
      <a class="button secondary" href={href(data.view)}>{t.adminEmailRefresh}</a>
    </form>

    {#if data.view === "pending"}
      {#if data.payload?.pending.length}
        <div class="record-list">
          {#each data.payload.pending as record (record.id)}
            <article class="record-row">
              <div class="record-main">
                <header class="record-header">
                  <div class="identity"><strong>{purposeLabel(record.purpose)}</strong><span class="badge">{record.expired ? t.adminEmailExpired : t.adminEmailActive}</span></div>
                  <time datetime={record.created_at > 0 ? new Date(record.created_at * 1000).toISOString() : undefined}>{dateLabel(record.created_at)}</time>
                </header>
                <dl class="metadata">
                  <div><dt>{t.adminEmailEmail}</dt><dd>{record.email_masked || record.email}</dd></div>
                  <div><dt>{t.adminEmailUser}</dt><dd>{record.username || t.adminEmailNone}{#if record.uid} · {t.adminEmailUID} {record.uid}{/if}</dd></div>
                  <div><dt>{t.adminEmailAttempts}</dt><dd>{record.attempts} / {record.max_attempts}</dd></div>
                  <div><dt>{t.adminEmailExpires}</dt><dd>{dateLabel(record.expires_at)}</dd></div>
                </dl>
              </div>
              <form method="POST" action="?/revoke" class="row-action" onsubmit={(event) => submitConfirmation(event, t.adminEmailRevokeConfirm)}>
                {#each hiddenQuery() as [name, value]}<input type="hidden" name={name} value={value} />{/each}
                <input type="hidden" name="verification_id" value={record.id} />
                <button class="button danger" type="submit">{t.adminEmailRevoke}</button>
              </form>
            </article>
          {/each}
        </div>
      {:else}<p class="empty">{t.adminEmailEmptyPending}</p>{/if}
    {:else if data.payload?.accounts.length}
      <div class="account-list">
        {#each data.payload.accounts as account (account.uid)}
          <article class="account-row">
            <header class="record-header">
              <div class="identity"><strong>{account.username}</strong>{#if account.role === 0}<span class="badge info">{t.adminEmailAdmin}</span>{/if}</div>
              <span class:good={account.email_verified} class:warning={!account.email_verified} class="status">{account.email_verified ? t.adminEmailVerified : t.adminEmailUnverified}</span>
            </header>
            <dl class="metadata">
              <div><dt>{t.adminEmailUID}</dt><dd>{account.uid}</dd></div>
              <div><dt>{t.adminEmailEmail}</dt><dd>{account.email}</dd></div>
              <div><dt>{t.adminEmailTelegram}</dt><dd>{account.telegram_username ? `@${account.telegram_username}` : account.telegram_id || t.adminEmailNotBound}</dd></div>
              <div><dt>{t.adminEmailStatus}</dt><dd>{account.active ? t.adminEmailEnabled : t.adminEmailDisabled}</dd></div>
              {#if account.email_verified_at}<div><dt>{t.adminEmailVerified}</dt><dd>{dateLabel(account.email_verified_at)}</dd></div>{/if}
            </dl>
          </article>
        {/each}
      </div>
    {:else}<p class="empty">{t.adminEmailEmptyAccounts}</p>{/if}

    {#if totalPages() > 1}
      <nav class="pagination" aria-label={t.adminEmailTitle}>
        {#if data.page > 1}<a class="button secondary" href={href(data.view, data.page - 1)}>{t.adminEmailPrevious}</a>{:else}<span></span>{/if}
        <span>{t.adminEmailPageOf.replace("{page}", String(data.page)).replace("{pages}", String(totalPages()))}</span>
        {#if data.page < totalPages()}<a class="button secondary" href={href(data.view, data.page + 1)}>{t.adminEmailNext}</a>{:else}<span></span>{/if}
      </nav>
    {/if}
  </section>

  <section class="maintenance-grid">
    <section class="panel" aria-labelledby="maintenance-title">
      <h2 id="maintenance-title">{t.adminEmailCleanup}</h2>
      <p class="muted">{t.adminEmailCleanupHelp}</p>
      <form method="POST" action="?/cleanup" onsubmit={(event) => submitConfirmation(event, t.adminEmailCleanupConfirm)}>
        {#each hiddenQuery() as [name, value]}<input type="hidden" name={name} value={value} />{/each}
        <input type="hidden" name="confirm" value="CLEANUP_EXPIRED_EMAILS" />
        <button class="button secondary" type="submit">{t.adminEmailCleanup}</button>
      </form>
    </section>
    <section class="panel" aria-labelledby="clear-title">
      <h2 id="clear-title">{t.adminEmailClearUnverified}</h2>
      <p class="muted">{t.adminEmailClearUnverifiedHelp}</p>
      <form method="POST" action="?/clearUnverified" onsubmit={(event) => submitConfirmation(event, t.adminEmailClearUnverifiedConfirm)}>
        {#each hiddenQuery() as [name, value]}<input type="hidden" name={name} value={value} />{/each}
        <input type="hidden" name="confirm" value="CLEAR_UNVERIFIED_EMAILS" />
        <button class="button danger" type="submit">{t.adminEmailClearUnverified}</button>
      </form>
    </section>
    <section class="panel" aria-labelledby="test-title">
      <h2 id="test-title">{t.adminEmailSMTPTest}</h2>
      <p class="muted">{t.adminEmailSMTPTestHelp}</p>
      <form method="POST" action="?/testEmail" class="test-form">
        <label>{t.adminEmailSMTPTestAddress}<input type="email" name="to" maxlength="254" placeholder={t.adminEmailSMTPTestAddressPlaceholder} /></label>
        <button class="button secondary" type="submit">{t.adminEmailSMTPTestSubmit}</button>
      </form>
      {#if action.action === "testEmail" && action.test}
        <p class:success={action.test.success} class:failure={!action.test.success} class="test-result">{action.test.success ? t.adminEmailSMTPTestSuccess.replace("{email}", action.test.email) : t.adminEmailSMTPTestFailed}</p>
      {/if}
    </section>
  </section>
</section>

<style>
  .email-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .record-header, .identity, .metadata, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .record-header, .pagination { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; } .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; } .muted { color: #52606d; margin: .35rem 0 0; }
  .text-link { min-height: 2.45rem; padding: .5rem 0; } .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.45rem; padding: .5rem .8rem; text-decoration: none; white-space: normal; } .button:hover { background: #d6e1e7; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.danger { background: #a63d40; color: #fff; } .button.danger:hover { background: #843336; }
  .notice { border: 1px solid; border-radius: .35rem; margin: 0; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .summary-panel { display: flex; flex-wrap: wrap; gap: .45rem; } .summary-chip, .badge, .status { background: #eef2f4; border: 1px solid #c8d2da; border-radius: .3rem; color: #36566a; font-size: .78rem; max-width: 100%; overflow-wrap: anywhere; padding: .35rem .5rem; } .summary-chip.good, .status.good { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .summary-chip.warning, .status.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .tabs { border-bottom: 1px solid #c8d2da; display: flex; gap: .35rem; overflow-x: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .tabs a { border-bottom: 3px solid transparent; color: #52606d; min-height: 2.7rem; padding: .6rem .75rem; text-decoration: none; white-space: nowrap; } .tabs a.active { border-bottom-color: #245b75; color: #16394a; font-weight: 700; } .tabs a span { color: #7b8794; font-size: .78rem; margin-left: .35rem; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: .9rem; min-width: 0; padding: 1rem; } .filter-form { align-items: end; display: grid; gap: .7rem; grid-template-columns: minmax(0, 2fr) minmax(8rem, 1fr) auto auto; } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.45rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .record-list, .account-list { display: grid; gap: .65rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .record-row, .account-row { align-items: start; border: 1px solid #d7dee5; display: flex; gap: 1rem; justify-content: space-between; min-width: 0; padding: .85rem; } .record-main { flex: 1; min-width: 0; } .record-header { align-items: center; min-width: 0; } .identity { align-items: center; flex-wrap: wrap; min-width: 0; } .identity strong { overflow-wrap: anywhere; } time { color: #52606d; flex: 0 0 auto; font-size: .78rem; max-width: 14rem; text-align: right; } .badge { border-radius: 999px; padding: .25rem .45rem; white-space: normal; } .badge.info { background: #edf4f8; color: #245b75; }
  .metadata { flex-wrap: wrap; margin: .7rem 0 0; } .metadata div { align-items: baseline; display: flex; gap: .35rem; min-width: 9rem; max-width: 100%; } dt { color: #52606d; font-size: .76rem; } dd { margin: 0; max-width: 100%; overflow-wrap: anywhere; } .status { border-radius: 999px; flex: 0 0 auto; } .row-action { flex: 0 0 auto; } .empty { color: #52606d; padding: 1.5rem; text-align: center; }
  .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination > span { color: #52606d; font-size: .84rem; }
  .maintenance-grid { display: grid; gap: 1rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .maintenance-grid .panel { align-content: start; } .maintenance-grid form { margin-top: .25rem; } .test-form { display: grid; gap: .65rem; } .test-result { border-top: 1px solid #e1e8ed; font-size: .84rem; margin: .1rem 0 0; padding-top: .65rem; } .test-result.success { color: #276749; } .test-result.failure { color: #a61b1b; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: minmax(0, 1fr) minmax(8rem, 1fr); } .filter-form .button, .filter-form > a { width: 100%; } .maintenance-grid { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .panel-heading, .record-header, .record-row, .account-row { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button, .record-row .row-action, .record-row .row-action .button, .maintenance-grid form, .maintenance-grid form .button { width: 100%; } .filter-form { grid-template-columns: 1fr; } .metadata { display: grid; grid-template-columns: 1fr; } .metadata div { min-width: 0; } time { max-width: none; text-align: left; } .record-list, .account-list { max-height: none; overflow: visible; } .panel { padding: .85rem; } h1 { font-size: 1.65rem; } .pagination { align-items: stretch; } .pagination .button { min-width: 0; } }
</style>
