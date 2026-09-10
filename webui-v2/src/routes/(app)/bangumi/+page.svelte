<script lang="ts">
  import { t } from "$lib/i18n";
  import type { BangumiCollectionEntry, BangumiSummary } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let payload = $derived(data.payload as BangumiSummary | null);
  let status = $derived(payload?.status);
  let account = $derived(payload?.account);
  let action = $derived((form ?? {}) as FormState);

  const categories = [
    { key: "watching", label: t.bangumiCollectionWatching, type: 3 },
    { key: "collected", label: t.bangumiCollectionCollected, type: 2 },
    { key: "wishlist", label: t.bangumiCollectionWishlist, type: 1 },
    { key: "on_hold", label: t.bangumiCollectionOnHold, type: 4 },
    { key: "dropped", label: t.bangumiCollectionDropped, type: 5 }
  ] as const;

  function dateLabel(value: number): string {
    if (!value) return "-";
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function itemTitle(item: BangumiCollectionEntry): string {
    return item.subject?.name_cn || item.subject?.name || t.bangumiUnknownItem;
  }

  function activityLabel(item: BangumiCollectionEntry): string {
    const name = itemTitle(item);
    switch (item.collection_type || item.type) {
      case 1: return t.bangumiActivityWishlist.replace("{name}", name);
      case 2: return t.bangumiActivityCollected.replace("{name}", name);
      case 3: return item.ep_status ? t.bangumiActivityProgress.replace("{name}", name).replace("{episode}", String(item.ep_status)) : t.bangumiActivityWatching.replace("{name}", name);
      case 4: return t.bangumiActivityOnHold.replace("{name}", name);
      case 5: return t.bangumiActivityDropped.replace("{name}", name);
      default: return t.bangumiActivityUpdated.replace("{name}", name);
    }
  }

  function avatar(): string {
    return account?.avatar?.large || account?.avatar?.medium || account?.avatar?.small || "";
  }

  function collectionTotal(key: string): number {
    return payload?.collections?.[key]?.total || 0;
  }
</script>

<svelte:head><title>{t.bangumiTitle} - {t.siteName}</title></svelte:head>

<section class="bangumi-page" aria-labelledby="bangumi-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.account}</p>
      <h1 id="bangumi-title">{t.bangumiTitle}</h1>
      <p class="muted">{t.bangumiDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/dashboard">{t.bangumiBackDashboard}</a>
      <a class="button secondary" href="/bangumi">{t.bangumiRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.result === "sync"}<p class="notice success" role="status">{t.bangumiSyncCompleted}</p>{/if}
  {#if data.result === "clear-history"}<p class="notice success" role="status">{t.bangumiClearHistory}</p>{/if}
  {#if data.result === "settings"}<p class="notice success" role="status">{t.bangumiSettingsSaved}</p>{/if}
  {#if data.result === "clear-token"}<p class="notice success" role="status">{t.bangumiTokenCleared}</p>{/if}

  {#if payload && status}
    {#if account?.expired}
      <section class="notice warning"><strong>{t.bangumiAccountExpired}</strong><p>{t.bangumiAccountExpiredHelp}</p></section>
    {:else if payload.account_error}
      <section class="notice warning"><strong>{t.bangumiAccountUnavailable}</strong><p>{t.systemUnavailable}</p></section>
    {/if}

    {#if account && !account.expired}
      <section class="account-grid">
        <article class="panel account-card">
          <div class="account-row">
            {#if avatar()}<img src={avatar()} alt={account.nickname || t.bangumiUnknownUser} loading="lazy" referrerpolicy="no-referrer" />{:else}<div class="avatar-fallback" aria-hidden="true">B</div>{/if}
            <div class="account-text"><strong>{account.nickname || t.bangumiUnknownUser}</strong><span>@{account.username || "-"}</span>{#if account.id}<small>{t.bangumiAccountUID.replace("{id}", String(account.id))}</small>{/if}</div>
          </div>
          {#if account.sign}<p class="account-sign">“{account.sign}”</p>{/if}
          {#if account.username}<a class="text-link" href={`https://bgm.tv/user/${encodeURIComponent(account.username)}`} target="_blank" rel="noopener noreferrer">{t.bangumiHomepage}</a>{/if}
        </article>
        <article class="panel activity-panel">
          <div class="section-heading"><h2>{t.bangumiRecentActivity}</h2></div>
          {#if payload.recent_activity?.length}
            <div class="activity-list">{#each payload.recent_activity as item (`${item.collection_type || item.type}:${item.subject_id}`)}<div class="activity-row"><span class="activity-mark" aria-hidden="true"></span><div><a href={`/bangumi/collections/${item.collection_type || item.type}`}>{activityLabel(item)}</a>{#if item.updated_at}<small>{dateLabel(item.updated_at)}</small>{/if}</div></div>{/each}</div>
          {:else}<p class="muted">{t.bangumiNoActivity}</p>{/if}
        </article>
      </section>
    {/if}

    {#if status.manage_enabled && status.bgm_manage_mode && payload.collections}
      {#if payload.collections_partial}<p class="notice warning">{t.bangumiPartialCollections}</p>{/if}
      <section class="category-grid" aria-label={t.bangumiTitle}>
        {#each categories as category}
          <a class="category-card" href={`/bangumi/collections/${category.type}`}>
            <span>{category.label}</span><strong>{collectionTotal(category.key)}</strong><small>{t.bangumiCollectionTitle.replace("{name}", category.label)}</small>
          </a>
        {/each}
      </section>
    {:else if status.token_set && !status.manage_enabled}
      <section class="notice warning"><strong>{t.bangumiManagementDisabled}</strong><p>{t.bangumiManagementDisabledHelp}</p></section>
    {/if}

    {#if status.sync_enabled}
      <section class="panel" aria-labelledby="sync-title">
        <div class="section-heading"><div><h2 id="sync-title">{t.bangumiStatus}</h2></div><div class="action-row"><form method="POST" action="?/sync"><button class="button primary" type="submit" disabled={!status.sync_ready}>{t.bangumiSyncNow}</button></form>{#if status.recent_logs.length}<form method="POST" action="?/clearHistory" onsubmit={(event) => { if (!confirm(t.bangumiClearHistoryConfirm)) event.preventDefault(); }}><button class="button secondary" type="submit">{t.bangumiClearHistory}</button></form>{/if}</div></div>
        <div class="metrics"><article><strong>{status.total_records}</strong><span>{t.bangumiTotalRecords}</span></article><article><strong>{status.synced_count}</strong><span>{t.bangumiSyncedCount}</span></article><article><strong>{status.sync_ready ? t.bangumiReady : t.bangumiNotReady}</strong><span>{t.bangumiStatus}</span></article><article><strong>{status.token_set ? t.bangumiTokenConfigured : t.bangumiTokenMissing}</strong><span>{t.bangumiAccessToken}</span></article></div>
      </section>
    {/if}

    <section class="panel" aria-labelledby="settings-title">
      <div class="section-heading"><div><h2 id="settings-title">{t.bangumiSettings}</h2></div></div>
      <div class="settings-form">
        {#if status.sync_enabled}<form method="POST" action="?/saveSettings" class="switch-row"><span><strong>{t.bangumiSyncMode}</strong><small>{t.bangumiSyncModeHelp}</small></span><input type="hidden" name="bgm_mode" value={status.bgm_mode ? "false" : "true"} /><button class="toggle" type="submit" aria-label={status.bgm_mode ? t.bangumiTurnOff : t.bangumiTurnOn}>{status.bgm_mode ? t.bangumiTurnOff : t.bangumiTurnOn}</button></form>{/if}
        {#if status.manage_enabled}<form method="POST" action="?/saveSettings" class="switch-row"><span><strong>{t.bangumiManageMode}</strong><small>{t.bangumiManageModeHelp}</small></span><input type="hidden" name="bgm_manage_mode" value={status.bgm_manage_mode ? "false" : "true"} /><button class="toggle" type="submit" aria-label={status.bgm_manage_mode ? t.bangumiTurnOff : t.bangumiTurnOn}>{status.bgm_manage_mode ? t.bangumiTurnOff : t.bangumiTurnOn}</button></form>{/if}
        <form method="POST" action="?/saveSettings" class="token-form"><label>{t.bangumiAccessToken}<input name="bgm_token" type="password" autocomplete="new-password" placeholder={status.token_set ? t.bangumiAccessTokenKeep : t.bangumiAccessTokenPlaceholder} /><small>{t.bangumiAccessTokenHelp}</small></label><button class="button primary" type="submit">{t.bangumiSaveSettings}</button></form>
        {#if status.token_set}<form method="POST" action="?/clearToken" class="action-row" onsubmit={(event) => { if (!confirm(t.bangumiClearTokenConfirm)) event.preventDefault(); }}><button class="button danger" type="submit">{t.bangumiClearToken}</button></form>{/if}
      </div>
    </section>

    {#if status.recent_logs.length}
      <section class="panel" aria-labelledby="history-title"><div class="section-heading"><h2 id="history-title">{t.bangumiHistory}</h2></div><div class="history-list">{#each status.recent_logs as log (log.id)}<div class="history-row"><span class:failure={log.status === "failed"} class="history-mark" aria-hidden="true"></span><div><strong>{log.subject_name || log.record_item_id || "-"}</strong><small>{log.status === "success" ? t.bangumiSyncSuccess : log.status === "failed" ? t.bangumiSyncFailure : t.bangumiSyncPending} · {dateLabel(log.created_at)}</small>{#if log.message}<p>{log.message}</p>{/if}</div></div>{/each}</div></section>
    {:else if status.sync_enabled}<section class="panel"><h2>{t.bangumiHistory}</h2><p class="muted">{t.bangumiHistoryEmpty}</p></section>{/if}
  {:else if !data.loadError}
    <p class="notice error" role="alert">{t.systemUnavailable}</p>
  {/if}
</section>

<style>
  .bangumi-page { display: grid; gap: 1rem; }
  .page-heading, .section-heading, .heading-actions, .action-row, .account-row, .switch-row { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .section-heading, .switch-row { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.15rem; }
  .heading-actions, .action-row { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); } h2 { font-size: 1.1rem; }
  .muted, small { color: #52606d; } .muted { margin: .4rem 0 0; } small { display: block; font-size: .78rem; margin-top: .25rem; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; } .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; padding: .5rem .85rem; } .button.primary { background: #245b75; color: #fff; } .button.secondary { background: #e8eef2; color: #16394a; } .button.danger { background: #a63d40; color: #fff; } .button:disabled { cursor: not-allowed; opacity: .55; }
  .notice, .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; padding: 1rem; } .notice { margin: 0; } .notice p { margin: .4rem 0 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .account-grid { display: grid; gap: 1rem; grid-template-columns: minmax(14rem, .8fr) minmax(0, 1.2fr); } .account-card, .activity-panel { display: grid; gap: .8rem; min-width: 0; } .account-row { align-items: center; } .account-row img, .avatar-fallback { background: #e8eef2; border: 2px solid #9fb3c8; border-radius: 50%; flex: 0 0 3.5rem; height: 3.5rem; object-fit: cover; width: 3.5rem; } .avatar-fallback { align-items: center; color: #245b75; display: flex; font-weight: 800; justify-content: center; } .account-text { display: grid; min-width: 0; } .account-text strong, .account-text span, .account-text small { overflow-wrap: anywhere; } .account-sign { background: #f4f6f8; border: 1px solid #e1e8ed; border-radius: .3rem; font-style: italic; margin: 0; padding: .6rem; }
  .activity-list, .history-list { display: grid; gap: .35rem; max-height: 18rem; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .activity-row, .history-row { align-items: flex-start; border-bottom: 1px solid #e1e8ed; display: flex; gap: .6rem; min-width: 0; padding: .55rem .1rem; } .activity-row:last-child, .history-row:last-child { border-bottom: 0; } .activity-row > div, .history-row > div { min-width: 0; } .activity-row a { overflow-wrap: anywhere; } .activity-mark, .history-mark { background: #245b75; border-radius: 50%; flex: 0 0 .55rem; height: .55rem; margin-top: .35rem; } .history-mark.failure { background: #a63d40; } .history-row p { color: #52606d; font-size: .78rem; margin: .3rem 0 0; }
  .category-grid { display: grid; gap: .75rem; grid-template-columns: repeat(5, minmax(0, 1fr)); } .category-card { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; color: #17202a; display: grid; gap: .25rem; min-width: 0; padding: .9rem; text-decoration: none; } .category-card:hover { border-color: #9fb3c8; } .category-card strong { font-size: 1.45rem; } .category-card small { min-height: 1.2em; }
  .metrics { display: grid; gap: .75rem; grid-template-columns: repeat(4, minmax(0, 1fr)); margin-top: 1rem; } .metrics article { background: #f4f6f8; border: 1px solid #e1e8ed; border-radius: .3rem; display: grid; gap: .3rem; min-width: 0; padding: .75rem; } .metrics strong { overflow-wrap: anywhere; } .metrics span { color: #52606d; font-size: .78rem; }
  .settings-form { display: grid; gap: .9rem; margin-top: .5rem; } .switch-row { align-items: center; border-bottom: 1px solid #e1e8ed; padding-bottom: .8rem; } .switch-row > span { display: grid; gap: .25rem; min-width: 0; } .switch-row input[type="hidden"] { display: none; } .toggle { background: #e8eef2; border: 1px solid #9fb3c8; border-radius: 999px; color: #16394a; cursor: pointer; min-height: 2.25rem; padding: .35rem .8rem; } .token-form { display: grid; gap: .65rem; } label { color: #243b53; display: grid; font-size: .87rem; font-weight: 650; gap: .35rem; min-width: 0; } input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } button:focus-visible, a:focus-visible, input:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 900px) { .account-grid { grid-template-columns: 1fr; } .category-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 600px) { .page-heading, .section-heading, .switch-row { align-items: stretch; flex-direction: column; } .heading-actions, .action-row { align-items: stretch; } .heading-actions .button, .action-row .button, .action-row form { width: 100%; } .category-grid, .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .panel, .notice { padding: .85rem; } .account-row { align-items: flex-start; } }
</style>
