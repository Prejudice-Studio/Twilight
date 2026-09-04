<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  type FormState = { action?: string; error?: string };
  let action = $derived((form ?? {}) as FormState);
  let payload = $derived(data.payload);
  let summary = $derived(payload?.summary);
  let config = $derived(payload?.config);
  let renewal = $derived(summary?.renewal || config?.renewal);

  function dateLabel(value: number): string {
    if (!value) return "-";
    return new Date(value * 1000).toISOString().slice(0, 16).replace("T", " ");
  }

  function rewardRange(): string {
    if (!config) return "-";
    return config.daily_min === config.daily_max ? String(config.daily_min) : `${config.daily_min} - ${config.daily_max}`;
  }
</script>

<svelte:head><title>{t.signin} - {t.siteName}</title></svelte:head>

<section class="score-page" aria-labelledby="score-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.account}</p>
      <h1 id="score-title">{t.signin}</h1>
      <p class="muted">{t.signinIntro}</p>
    </div>
    <div class="heading-actions">
      <a class="back-link" href="/dashboard">{t.backDashboard}</a>
      <a class="refresh-link" href="/score">{t.signinRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.result === "signin"}<p class="notice success" role="status">{t.signinSignSuccessHelp.replace("{days}", String(summary?.current_streak || 0)).replace("{bonus}", "")}</p>{/if}
  {#if data.result === "renew"}<p class="notice success" role="status">{t.signinRenewSuccess}</p>{/if}
  {#if data.result === "auto-renewal"}<p class="notice success" role="status">{t.saved}</p>{/if}

  {#if payload && summary && config}
    {#if !summary.enabled}
      <section class="disabled-panel"><h2>{t.signinDisabled}</h2><p>{t.signinDisabledHelp}</p></section>
    {:else}
      <section class="balance-panel" aria-labelledby="balance-title">
        <div>
          <p class="eyebrow">{t.signinBalance}</p>
          <h2 id="balance-title"><strong>{summary.current_points}</strong> <span>{summary.currency_name}</span></h2>
          <p class="muted">{t.signinTotalEarned.replace("{points}", String(summary.total_points)).replace("{currency}", summary.currency_name)}</p>
        </div>
        <div class="primary-actions">
          <form method="POST" action="?/signin"><button class="button primary" type="submit" disabled={summary.today_signed}>{summary.today_signed ? t.signinToday : t.signinNow}</button></form>
          {#if renewal?.enabled}
            <form method="POST" action="?/renew"><button class="button secondary" type="submit" disabled={!renewal.affordable || !data.user?.emby_id}>
              {t.signinRenew.replace("{cost}", String(renewal.cost)).replace("{currency}", summary.currency_name).replace("{days}", String(renewal.days))}
            </button></form>
            {#if renewal.auto_renewal_available === false}<small class="muted">{t.signinNoEmby}</small>{/if}
          {/if}
        </div>
      </section>

      {#if renewal?.auto_renewal_enabled}
        <section class="auto-panel" aria-labelledby="auto-title">
          <div><h2 id="auto-title">{t.signinAutoRenew}</h2><p class="muted">{renewal.auto_renewal_available ? t.signinAutoRenewHelp : t.signinAutoRenewUnavailable}</p></div>
          <form method="POST" action="?/autoRenewal">
            <input type="hidden" name="enabled" value={renewal.auto_renewal_user_enabled ? "false" : "true"} />
            <button class="button secondary" type="submit" disabled={!renewal.auto_renewal_available}>{renewal.auto_renewal_user_enabled ? t.signinAutoDisabled : t.signinAutoEnabled}</button>
          </form>
        </section>
      {/if}

      <div class="summary-grid">
        <article class="panel"><span>{t.signinCurrentStreak}</span><strong>{t.signinDays.replace("{days}", String(summary.current_streak))}</strong>{#if summary.next_bonus_in_days && summary.next_bonus_points}<small>{t.signinNextBonus.replace("{days}", String(summary.next_bonus_in_days)).replace("{points}", String(summary.next_bonus_points)).replace("{currency}", summary.currency_name)}</small>{:else}<small>{config.streak_bonus_enabled ? t.signinNoMoreBonus : t.signinBonusClosed}</small>{/if}</article>
        <article class="panel"><span>{t.signinLongestStreak}</span><strong>{t.signinDays.replace("{days}", String(summary.longest_streak))}</strong><small>{t.signinLast.replace("{date}", summary.last_signin_date || "-")}</small></article>
        <article class="panel"><span>{t.signinDailyReward}</span><strong>{rewardRange()} {summary.currency_name}</strong><small>{config.reset_after_miss ? t.signinResetAfterMiss : t.signinKeepAfterMiss}</small></article>
      </div>

      <div class="content-grid">
        <section class="panel" aria-labelledby="bonus-title">
          <div class="section-heading"><h2 id="bonus-title">{t.signinBonusRules}</h2></div>
          {#if !config.streak_bonus_enabled}<p class="muted">{t.signinBonusClosed}</p>{:else if !config.bonus_table.length}<p class="muted">{t.signinNoMoreBonus}</p>{:else}<div class="rules">{#each config.bonus_table as rule}<div><span>{t.signinDays.replace("{days}", String(rule.streak_days))}</span><strong>+{rule.bonus_points} {summary.currency_name}</strong></div>{/each}</div>{/if}
        </section>
        <section class="panel history-panel" aria-labelledby="history-title">
          <div class="section-heading"><h2 id="history-title">{t.signinHistory}</h2><span class="muted">{t.signinHistoryCount.replace("{count}", String(payload.history.length))}</span></div>
          {#if payload.history.length}<div class="history-scroll">{#each payload.history as row}<div class="history-row"><div><strong>{row.date}</strong><small>{dateLabel(row.created_at)}</small></div><div><span>{t.signinDays.replace("{days}", String(row.streak))}</span><strong>+{row.total} {summary.currency_name}</strong></div></div>{/each}</div>{:else}<p class="muted">{t.signinHistoryEmpty}</p>{/if}
        </section>
      </div>
    {/if}
  {/if}
</section>

<style>
  .score-page { display: grid; gap: 1rem; }
  .page-heading, .heading-actions, .section-heading, .primary-actions, .auto-panel, .history-row { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .section-heading, .auto-panel, .history-row { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.25rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1, h2 { margin: 0; overflow-wrap: anywhere; } h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); } h2 { font-size: 1.15rem; }
  .muted { color: #52606d; margin: .45rem 0 0; } .back-link, .refresh-link { min-height: 2.5rem; padding: .55rem 0; }
  .balance-panel, .auto-panel, .panel, .disabled-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1.15rem; }
  .balance-panel { align-items: center; display: flex; gap: 1rem; justify-content: space-between; } .balance-panel h2 { font-size: 1rem; margin-top: .2rem; } .balance-panel h2 strong { font-size: 2.5rem; } .balance-panel h2 span { color: #52606d; font-size: 1rem; }
  .primary-actions { align-items: flex-end; flex-direction: column; } .primary-actions form, .primary-actions .button { width: 100%; } .primary-actions form { max-width: 20rem; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; padding: .5rem .85rem; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary { background: #e8eef2; color: #16394a; } .button.secondary:hover { background: #d6e1e7; } .button:disabled { cursor: not-allowed; opacity: .55; }
  .auto-panel { align-items: center; } .auto-panel form, .auto-panel .button { flex: 0 0 auto; }
  .summary-grid, .content-grid { display: grid; gap: 1rem; } .summary-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } .content-grid { grid-template-columns: minmax(14rem, .8fr) minmax(0, 1.2fr); }
  .panel { display: grid; gap: .55rem; } .summary-grid .panel > span { color: #52606d; font-size: .85rem; } .summary-grid .panel > strong { font-size: 1.35rem; } .summary-grid small { color: #52606d; overflow-wrap: anywhere; }
  .rules, .history-scroll { display: grid; gap: .5rem; } .rules div { align-items: center; border: 1px solid #e1e8ed; border-radius: .3rem; display: flex; justify-content: space-between; padding: .6rem .7rem; } .rules strong { color: #7b4f00; }
  .history-panel { min-height: 12rem; } .history-scroll { max-height: 24rem; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .history-row { align-items: center; border-bottom: 1px solid #e1e8ed; padding: .6rem .1rem; } .history-row:last-child { border-bottom: 0; } .history-row div { display: grid; gap: .2rem; } .history-row div:last-child { align-items: end; } .history-row small, .history-row span { color: #52606d; font-size: .78rem; } .history-row div:last-child strong { color: #7b4f00; }
  .disabled-panel { color: #52606d; display: grid; gap: .4rem; padding: 2rem; } .disabled-panel p { margin: 0; }
  .notice { border: 1px solid; border-radius: .3rem; padding: .65rem .75rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 760px) { .balance-panel, .auto-panel { align-items: stretch; flex-direction: column; } .primary-actions { align-items: stretch; } .primary-actions form, .primary-actions .button { max-width: none; } .summary-grid, .content-grid { grid-template-columns: 1fr; } }
  @media (max-width: 560px) { .page-heading, .heading-actions, .section-heading { align-items: stretch; flex-direction: column; } .back-link, .refresh-link { align-self: flex-start; } .auto-panel form, .auto-panel .button { width: 100%; } .panel, .balance-panel, .auto-panel { padding: .9rem; } }
</style>
