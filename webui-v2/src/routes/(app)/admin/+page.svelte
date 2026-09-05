<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  type Category = "user" | "content" | "security" | "operations" | "integration";
  type Entry = { href: string; title: string; description: string; category: Category };

  let { data }: { data: PageData } = $props();

  const categories: Array<{ id: Category; title: string }> = [
    { id: "user", title: t.adminHomeCategoryUser },
    { id: "content", title: t.adminHomeCategoryContent },
    { id: "security", title: t.adminHomeCategorySecurity },
    { id: "operations", title: t.adminHomeCategoryOperations },
    { id: "integration", title: t.adminHomeCategoryIntegration }
  ];

  const entries: Entry[] = [
    { href: "/admin/users", title: t.adminUsersTitle, description: t.adminUsersDescription, category: "user" },
    { href: "/admin/invite", title: t.adminInviteTitle, description: t.adminInviteDescription, category: "user" },
    { href: "/admin/regcodes", title: t.adminRegcodesTitle, description: t.adminRegcodesDescription, category: "user" },
    { href: "/admin/requests", title: t.adminRequestsTitle, description: t.adminRequestsDescription, category: "content" },
    { href: "/admin/tickets", title: t.adminTicketsTitle, description: t.adminTicketsDescription, category: "content" },
    { href: "/admin/announcements", title: t.adminAnnouncementsTitle, description: t.adminAnnouncementsDescription, category: "content" },
    { href: "/admin/audit-logs", title: t.adminAuditLogTitle, description: t.adminAuditLogDescription, category: "security" },
    { href: "/admin/violations", title: t.adminViolationsTitle, description: t.adminViolationsDescription, category: "security" },
    { href: "/admin/status", title: t.adminStatusTitle, description: t.adminStatusDescription, category: "operations" },
    { href: "/admin/scheduler", title: t.adminSchedulerTitle, description: t.adminSchedulerDescription, category: "operations" },
    { href: "/admin/config", title: t.adminConfigTitle, description: t.adminConfigDescription, category: "operations" },
    { href: "/admin/database", title: t.adminDatabaseTitle, description: t.adminDatabaseDescription, category: "operations" },
    { href: "/admin/logs", title: t.adminRuntimeLogsTitle, description: t.adminRuntimeLogsDescription, category: "operations" },
    { href: "/admin/emby", title: t.adminEmbyTitle, description: t.adminEmbyDescription, category: "integration" },
    { href: "/admin/telegram", title: t.adminTelegramTitle, description: t.adminTelegramDescription, category: "integration" },
    { href: "/admin/telegram-rebind-requests", title: t.adminTelegramRebindTitle, description: t.adminTelegramRebindDescription, category: "integration" },
    { href: "/admin/email", title: t.adminEmailTitle, description: t.adminEmailDescription, category: "integration" }
  ];

  const featureLabels: Record<string, string> = {
    register: "开放注册",
    telegram: "Telegram Bot",
    force_bind_telegram: "强制绑定 Telegram",
    bangumi_sync: "Bangumi 同步",
    bangumi_manage: "Bangumi 收藏管理",
    media_request: "求片中心",
    signin: "签到",
    invite: "邀请系统",
    email_enabled: "邮件服务",
    ticket_system: "工单系统",
    developer_mode: "开发者模式"
  };

  function featureLabel(key: string): string { return featureLabels[key] || key; }
  function number(value: number | undefined): string { return value === undefined ? "-" : value.toLocaleString("zh-CN"); }
</script>

<svelte:head><title>{t.adminHomeTitle} - {t.siteName}</title></svelte:head>

<section class="admin-home" aria-labelledby="admin-home-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="admin-home-title">{t.adminHomeTitle}</h1><p class="muted">{t.adminHomeDescription}</p></div>
    <div class="heading-actions"><a class="button secondary" href="/admin/status">{t.adminStatusTitle}</a></div>
  </header>

  {#if data.loadError}<p class="notice warning" role="status">{data.loadError}</p>{/if}

  <div class="summary-grid">
    <section class="summary-panel" aria-labelledby="system-summary-title">
      <h2 id="system-summary-title">{t.adminHomeSystemSummary}</h2><p class="muted">{t.adminHomeSystemSummaryHelp}</p>
      {#if data.info}
        <dl class="facts"><div><dt>{t.adminStatusServiceName}</dt><dd>{data.info.name || "-"}</dd></div><div><dt>{t.adminStatusVersion}</dt><dd>{data.info.version || "-"}</dd></div><div><dt>API</dt><dd>{data.info.api_version || "-"}</dd></div></dl>
        {#if data.info.features}<div class="feature-list" aria-label={t.adminHomeFeatures}>{#each Object.entries(data.info.features) as [key, enabled]}<div><span>{featureLabel(key)}</span><strong class:enabled={enabled}>{enabled ? t.adminStatusOn : t.adminStatusOff}</strong></div>{/each}</div>{/if}
      {:else}<p class="empty">{t.adminHomeNoData}</p>{/if}
    </section>
    <section class="summary-panel" aria-labelledby="stats-summary-title">
      <h2 id="stats-summary-title">{t.adminHomeStatsSummary}</h2><p class="muted">{t.adminHomeStatsSummaryHelp}</p>
      {#if data.stats}
        <dl class="facts"><div><dt>{t.adminStatusTotalUsers}</dt><dd>{number(data.stats.users?.total)}</dd></div><div><dt>{t.adminStatusActiveUsers}</dt><dd>{number(data.stats.users?.active)}</dd></div><div><dt>{t.adminStatusRegcodeStats}</dt><dd>{number(data.stats.regcodes?.active)} / {number(data.stats.regcodes?.total)}</dd></div><div><dt>{t.adminStatusRoutes}</dt><dd>{number(data.stats.routes)}</dd></div></dl>
      {:else}<p class="empty">{t.adminHomeNoData}</p>{/if}
    </section>
  </div>

  <div class="category-list">
    {#each categories as category}
      {@const categoryEntries = entries.filter((entry) => entry.category === category.id)}
      <section aria-labelledby={`category-${category.id}`}>
        <h2 id={`category-${category.id}`}>{category.title}</h2>
        <div class="entry-grid">
          {#each categoryEntries as entry}
            <a class="entry" href={entry.href}>
              <span class="entry-mark" aria-hidden="true">{entry.title.slice(0, 1)}</span>
              <span class="entry-copy"><strong>{entry.title}</strong><span>{entry.description}</span><small>{entry.href}</small></span>
              <span class="entry-arrow" aria-hidden="true">→</span>
            </a>
          {/each}
        </div>
      </section>
    {/each}
  </div>
</section>

<style>
  .admin-home { display: grid; gap: 1.1rem; min-width: 0; }
  .page-heading, .heading-actions { align-items: flex-start; display: flex; gap: .8rem; } .page-heading { border-bottom: 1px solid #d7dee5; justify-content: space-between; padding-bottom: 1.1rem; } .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; } h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.1rem; } .muted { color: #52606d; margin: .4rem 0 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.45rem; padding: .5rem .8rem; text-decoration: none; } .button:hover { background: #d6e1e7; } .notice { border: 1px solid #e9c46a; border-radius: .35rem; background: #fff8e6; color: #7b4f00; margin: 0; padding: .7rem .8rem; }
  .summary-grid { display: grid; gap: 1rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .summary-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: .75rem; min-width: 0; padding: 1rem; }
  .facts { display: grid; gap: .5rem; margin: 0; } .facts div { align-items: baseline; border-bottom: 1px solid #e1e8ed; display: flex; gap: .75rem; justify-content: space-between; min-width: 0; padding-bottom: .45rem; } .facts div:last-child { border-bottom: 0; padding-bottom: 0; } dt { color: #52606d; font-size: .8rem; } dd { font-weight: 650; margin: 0; max-width: 70%; overflow-wrap: anywhere; text-align: right; }
  .feature-list { border-top: 1px solid #e1e8ed; display: grid; gap: .35rem; padding-top: .7rem; } .feature-list div { align-items: center; display: flex; gap: .7rem; justify-content: space-between; min-width: 0; } .feature-list span { overflow-wrap: anywhere; } .feature-list strong { color: #a63d40; flex: 0 0 auto; font-size: .8rem; } .feature-list strong.enabled { color: #276749; } .empty { color: #52606d; padding: 1rem 0; text-align: center; }
  .category-list { display: grid; gap: 1.2rem; } .category-list > section { display: grid; gap: .65rem; } .entry-grid { display: grid; gap: .65rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .entry { align-items: center; background: #fff; border: 1px solid #d7dee5; border-radius: .4rem; color: inherit; display: flex; gap: .7rem; min-width: 0; padding: .75rem; text-decoration: none; } .entry:hover { background: #f8fafb; border-color: #9fb3c8; } .entry-mark { align-items: center; background: #edf4f8; border: 1px solid #c8d2da; border-radius: .3rem; color: #245b75; display: flex; flex: 0 0 2.3rem; font-weight: 750; height: 2.3rem; justify-content: center; } .entry-copy { display: grid; gap: .2rem; min-width: 0; } .entry-copy strong, .entry-copy span, .entry-copy small { overflow-wrap: anywhere; } .entry-copy strong { color: #16394a; } .entry-copy span { color: #52606d; font-size: .78rem; line-height: 1.35; } .entry-copy small { color: #7b8794; font: .7rem ui-monospace, SFMono-Regular, Consolas, monospace; } .entry-arrow { color: #52606d; flex: 0 0 auto; font-size: 1.15rem; margin-left: auto; }
  a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 900px) { .entry-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 600px) { .page-heading, .heading-actions { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions .button { width: 100%; } .summary-grid, .entry-grid { grid-template-columns: 1fr; } .summary-panel { padding: .85rem; } h1 { font-size: 1.65rem; } }
</style>
