<script lang="ts">
  import { t } from "$lib/i18n";
  import type { HealthProbe, SystemHealthDetail } from "$lib/types";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();

  type ProbeKind = "api" | "database" | "emby";

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

  function number(value: number | null | undefined): string {
    return value === null || value === undefined ? "-" : value.toLocaleString("zh-CN");
  }

  function uptime(value: number | null | undefined): string {
    if (!value || value < 1) return "-";
    const days = Math.floor(value / 86400);
    const hours = Math.floor((value % 86400) / 3600);
    const minutes = Math.floor((value % 3600) / 60);
    const seconds = value % 60;
    return [days ? `${days} 天` : "", hours ? `${hours} 小时` : "", minutes ? `${minutes} 分钟` : "", `${seconds} 秒`]
      .filter(Boolean)
      .join(" ");
  }

  function healthState(probe: HealthProbe<SystemHealthDetail>, kind: ProbeKind): "healthy" | "unhealthy" | "unavailable" | "not-configured" {
    if (!probe.available || !probe.data) return "unavailable";
    if (kind === "emby" && probe.data.configured === false) return "not-configured";
    if (kind === "emby") return probe.data.online === true ? "healthy" : "unhealthy";
    if (kind === "database") {
      return probe.data.ok === true && probe.data.ping_ok !== false && probe.data.storage_mismatch !== true ? "healthy" : "unhealthy";
    }
    return probe.data.ok === true ? "healthy" : "unhealthy";
  }

  function stateLabel(state: ReturnType<typeof healthState>): string {
    switch (state) {
      case "healthy": return t.adminStatusHealthy;
      case "unhealthy": return t.adminStatusUnhealthy;
      case "not-configured": return t.adminStatusNotConfigured;
      default: return t.adminStatusUnavailable;
    }
  }

  function stateClass(state: ReturnType<typeof healthState>): string {
    return `state-${state}`;
  }

  function valueLabel(value: string | number | boolean | null | undefined): string {
    if (value === null || value === undefined || value === "") return "-";
    if (typeof value === "boolean") return value ? t.adminStatusOn : t.adminStatusOff;
    return String(value);
  }

  function probeTitle(kind: ProbeKind): string {
    return kind === "api" ? t.adminStatusApi : kind === "database" ? t.adminStatusDatabase : t.adminStatusEmby;
  }

  function probeHint(kind: ProbeKind): string {
    return kind === "api" ? t.adminStatusApiHint : kind === "database" ? t.adminStatusDatabaseHint : t.adminStatusEmbyHint;
  }
</script>

<svelte:head><title>{t.adminStatusTitle} - {t.siteName}</title></svelte:head>

<section class="status-page" aria-labelledby="status-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="status-title">{t.adminStatusTitle}</h1>
      <p class="muted">{t.adminStatusDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/dashboard">{t.backDashboard}</a>
      <form method="GET" action="/admin/status">
        <button class="button primary" type="submit">{t.adminStatusRefresh}</button>
      </form>
    </div>
  </header>

  <p class="updated" role="status">{t.adminStatusUpdatedAt.replace("{date}", new Date(data.refreshed_at * 1000).toLocaleString("zh-CN"))}</p>

  <section class="health-grid" aria-labelledby="health-title">
    <div class="section-heading"><div><h2 id="health-title">{t.adminStatusHealthTitle}</h2><p class="muted">{t.adminStatusHealthDescription}</p></div></div>
    <div class="health-cards">
      {#each ["api", "database", "emby"] as rawKind}
        {@const kind = rawKind as ProbeKind}
        {@const probe = data.health[kind]}
        {@const state = healthState(probe, kind)}
        <article class="health-card" class:unavailable={!probe.available}>
          <div class="health-card-heading"><div><h3>{probeTitle(kind)}</h3><p>{probeHint(kind)}</p></div><strong class={stateClass(state)}>{stateLabel(state)}</strong></div>
          {#if state === "unavailable"}<p class="detail-error">{t.adminStatusReadFailed}</p>{/if}
          {#if kind === "api" && probe.data}
            <dl class="facts"><div><dt>{t.adminStatusRoutes}</dt><dd>{number(probe.data.routes)}</dd></div><div><dt>{t.adminStatusUptime}</dt><dd>{uptime(probe.data.uptime)}</dd></div></dl>
          {:else if kind === "database" && probe.data}
            <dl class="facts"><div><dt>{t.adminStatusDatabaseBackend}</dt><dd>{valueLabel(probe.data.backend)}</dd></div><div><dt>{t.adminStatusUsers}</dt><dd>{number(probe.data.user_count)}</dd></div><div><dt>{t.adminStatusConnections}</dt><dd>{number(probe.data.open_connections)}</dd></div></dl>
          {:else if kind === "emby" && probe.data}
            <dl class="facts"><div><dt>{t.adminStatusServerName}</dt><dd>{valueLabel(probe.data.server_name)}</dd></div><div><dt>{t.adminStatusVersion}</dt><dd>{valueLabel(probe.data.version)}</dd></div><div><dt>{t.adminStatusActiveSessions}</dt><dd>{number(probe.data.active_sessions)}</dd></div></dl>
          {/if}
        </article>
      {/each}
    </div>
  </section>

  <div class="content-grid">
    <section class="panel" aria-labelledby="system-title">
      <div class="section-heading"><div><h2 id="system-title">{t.adminStatusSystemTitle}</h2><p class="muted">{t.adminStatusSystemDescription}</p></div></div>
      {#if data.info}
        <dl class="facts large"><div><dt>{t.adminStatusServiceName}</dt><dd>{valueLabel(data.info.name)}</dd></div><div><dt>{t.adminStatusVersion}</dt><dd>{valueLabel(data.info.version)}</dd></div></dl>
        <div class="subsection"><h3>{t.adminStatusFeatures}</h3><div class="feature-list">{#each Object.entries(data.info.features || {}) as [name, enabled]}<div><span>{featureLabels[name] || name}</span><strong class:enabled={enabled}>{enabled ? t.adminStatusOn : t.adminStatusOff}</strong></div>{/each}</div></div>
      {:else}<p class="muted">{t.adminStatusReadFailed}</p>{/if}
    </section>

    <section class="panel" aria-labelledby="stats-title">
      <div class="section-heading"><div><h2 id="stats-title">{t.adminStatusStatsTitle}</h2><p class="muted">{t.adminStatusStatsDescription}</p></div></div>
      {#if data.stats}
        <div class="stat-group"><h3>{t.adminStatusUserStats}</h3><dl class="facts"><div><dt>{t.adminStatusActiveUsers}</dt><dd>{number(data.stats.users?.active)}</dd></div><div><dt>{t.adminStatusTotalUsers}</dt><dd>{number(data.stats.users?.total)}</dd></div><div><dt>{t.adminStatusUserLimit}</dt><dd>{data.stats.users?.limit == null ? t.adminStatusUnlimited : number(data.stats.users.limit)}</dd></div><div><dt>{t.adminStatusUsage}</dt><dd>{data.stats.users?.usage_percent === undefined ? "-" : `${data.stats.users.usage_percent}%`}</dd></div></dl></div>
        <div class="stat-group"><h3>{t.adminStatusRegcodeStats}</h3><dl class="facts"><div><dt>{t.adminStatusActive}</dt><dd>{number(data.stats.regcodes?.active)}</dd></div><div><dt>{t.adminStatusTotal}</dt><dd>{number(data.stats.regcodes?.total)}</dd></div></dl></div>
        <div class="stat-group"><h3>{t.adminStatusRuntime}</h3><dl class="facts"><div><dt>{t.adminStatusRedis}</dt><dd>{valueLabel(data.stats.redis_enabled)}</dd></div><div><dt>{t.adminStatusRoutes}</dt><dd>{number(data.stats.routes)}</dd></div><div><dt>{t.adminStatusUptime}</dt><dd>{uptime(data.stats.uptime)}</dd></div></dl></div>
      {:else}<p class="muted">{t.adminStatusReadFailed}</p>{/if}
    </section>
  </div>
</section>

<style>
  .status-page { display: grid; gap: 1.1rem; min-width: 0; }
  .page-heading, .heading-actions, .section-heading, .health-card-heading { align-items: flex-start; display: flex; gap: .8rem; }
  .page-heading, .section-heading, .health-card-heading { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.15rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; }
  .heading-actions form { display: flex; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; } h3 { font-size: .98rem; }
  .muted, .updated { color: #52606d; margin: .4rem 0 0; } .updated { font-size: .82rem; margin: 0; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; }
  .health-grid, .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; }
  .health-grid { display: grid; gap: 1rem; } .health-cards { display: grid; gap: .8rem; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .health-card { border: 1px solid #c8d2da; border-left: 4px solid #2f855a; display: grid; gap: .75rem; min-width: 0; padding: .9rem; } .health-card.unavailable { border-left-color: #a0aec0; }
  .health-card-heading { align-items: center; } .health-card-heading p { color: #52606d; font-size: .8rem; margin: .25rem 0 0; } .health-card-heading strong { border-radius: 999px; flex: 0 0 auto; font-size: .78rem; padding: .25rem .5rem; white-space: nowrap; }
  .state-healthy { background: #edf7f0; color: #276749; } .state-unhealthy { background: #fff1f0; color: #a61b1b; } .state-unavailable, .state-not-configured { background: #eef2f4; color: #52606d; }
  .facts { display: grid; gap: .55rem; margin: 0; } .facts div { align-items: baseline; border-bottom: 1px solid #e1e8ed; display: flex; gap: .8rem; justify-content: space-between; min-width: 0; padding-bottom: .5rem; } .facts div:last-child { border-bottom: 0; padding-bottom: 0; } dt { color: #52606d; font-size: .82rem; } dd { font-weight: 650; margin: 0; max-width: 70%; overflow-wrap: anywhere; text-align: right; } .facts.large { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .detail-error { background: #fff8e6; border: 1px solid #e9c46a; color: #7b4f00; margin: 0; padding: .55rem .65rem; }
  .content-grid { display: grid; gap: 1rem; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); } .panel { display: grid; gap: 1rem; }
  .subsection, .stat-group { border-top: 1px solid #e1e8ed; display: grid; gap: .65rem; padding-top: .85rem; } .feature-list { display: grid; gap: .45rem; } .feature-list div { align-items: center; border: 1px solid #e1e8ed; display: flex; gap: .75rem; justify-content: space-between; min-width: 0; padding: .55rem .65rem; } .feature-list span { overflow-wrap: anywhere; } .feature-list strong { color: #a63d40; flex: 0 0 auto; font-size: .8rem; } .feature-list strong.enabled { color: #276749; }
  button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 900px) { .health-cards, .content-grid { grid-template-columns: 1fr; } }
  @media (max-width: 560px) { .page-heading, .heading-actions, .section-heading, .health-card-heading { align-items: stretch; flex-direction: column; } .heading-actions { width: 100%; } .heading-actions .text-link, .heading-actions form, .heading-actions .button { width: 100%; } .facts.large { grid-template-columns: 1fr; } .health-card-heading strong { align-self: flex-start; } .health-grid, .panel { padding: .85rem; } }
</style>
