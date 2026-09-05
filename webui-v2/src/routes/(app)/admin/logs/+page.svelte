<script lang="ts">
  import { t } from "$lib/i18n";
  import type { RuntimeLogEntry, RuntimeStatus } from "$lib/types";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();

  function dateLabel(value: number | undefined): string {
    return value ? new Date(value * 1000).toLocaleString("zh-CN") : t.adminRuntimeLogsUnknown;
  }

  function duration(total: number | undefined): string {
    if (!total || total < 0) return t.adminRuntimeLogsSeconds.replace("{value}", "0");
    const days = Math.floor(total / 86400);
    const hours = Math.floor((total % 86400) / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const seconds = total % 60;
    return [
      days ? t.adminRuntimeLogsDays.replace("{value}", String(days)) : "",
      hours ? t.adminRuntimeLogsHours.replace("{value}", String(hours)) : "",
      minutes ? t.adminRuntimeLogsMinutes.replace("{value}", String(minutes)) : "",
      !days && !hours ? t.adminRuntimeLogsSeconds.replace("{value}", String(seconds)) : ""
    ].filter(Boolean).join(" ");
  }

  function bytes(value: number | undefined): string {
    if (!value || value < 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let size = value;
    let index = 0;
    while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1; }
    return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
  }

  function levelClass(level: string): string {
    const value = level.toLowerCase();
    if (value.includes("error")) return "error";
    if (value.includes("warn")) return "warning";
    if (value.includes("info")) return "info";
    return "default";
  }

  function message(entry: RuntimeLogEntry): string {
    const attrs = entry.attrs || {};
    const suffix = Object.entries(attrs).map(([key, value]) => `${key}=${value}`).join(" ");
    return suffix ? `${entry.message} ${suffix}` : entry.message;
  }

  function moreHref(): string {
    const max = data.status?.runtime_log_limit || data.limit;
    const next = Math.min(max, data.limit + 500);
    return `/admin/logs?limit=${next}`;
  }

  function statValue(value: string | number | boolean | undefined | null): string {
    if (value === undefined || value === null || value === "") return "-";
    if (typeof value === "boolean") return value ? t.adminRuntimeLogsEnabled : t.adminRuntimeLogsDisabled;
    return String(value);
  }

  function latestStatus(status: RuntimeStatus): Array<[string, string]> {
    return [
      [t.adminRuntimeLogsHost, status.hostname || t.adminRuntimeLogsUnknown],
      [t.adminRuntimeLogsProcessUptime, duration(status.uptime_seconds)],
      [t.adminRuntimeLogsHeapMemory, bytes(status.memory?.heap_alloc)],
      [t.adminRuntimeLogsDatabase, t.adminRuntimeLogsDatabaseUsers.replace("{database}", status.active_database || "-").replace("{users}", String(status.users))]
    ];
  }
</script>

<svelte:head><title>{t.adminRuntimeLogsTitle} - {t.siteName}</title></svelte:head>

<section class="logs-page" aria-labelledby="logs-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="logs-title">{t.adminRuntimeLogsTitle}</h1><p class="muted">{t.adminRuntimeLogsDescription}</p></div>
    <div class="heading-actions"><a class="text-link" href="/admin/status">{t.adminStatusTitle}</a><a class="button primary" href={`/admin/logs?limit=${data.limit}`}>{t.adminRuntimeLogsRefresh}</a></div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}

  {#if data.status}
    <div class="status-grid">{#each latestStatus(data.status) as [label, value]}<div class="stat"><span>{label}</span><strong>{value}</strong></div>{/each}</div>
  {/if}

  <section class="panel log-panel" aria-labelledby="stream-title">
    <header class="panel-heading"><div><h2 id="stream-title">{t.adminRuntimeLogsTitle}</h2><p class="muted">{t.adminRuntimeLogsCurrent.replace("{current}", String(data.logs?.entries?.length || 0)).replace("{limit}", String(data.logs?.limit || data.limit))}</p></div><div class="heading-actions">{#if data.status?.runtime_log_limit && data.limit < data.status.runtime_log_limit}<a class="button secondary" href={moreHref()}>{t.adminRuntimeLogsMore}</a>{/if}</div></header>
    {#if data.logs?.entries?.length}
      <div class="log-list">{#each data.logs.entries as entry (entry.id)}<article class="log-row"><time>{dateLabel(entry.time)}</time><span class={`level ${levelClass(entry.level)}`}>{entry.level || t.adminRuntimeLogsUnknown}</span><p>{message(entry)}</p></article>{/each}</div>
    {:else}<p class="empty">{t.adminRuntimeLogsEmpty}</p>{/if}
  </section>

  {#if data.status}
    <div class="detail-grid">
      <section class="panel" aria-labelledby="runtime-title"><h2 id="runtime-title">{t.adminRuntimeLogsGoRuntime}</h2><dl class="facts"><div><dt>{t.adminRuntimeLogsVersion}</dt><dd>{statValue(data.status.go_version)}</dd></div><div><dt>{t.adminRuntimeLogsPlatform}</dt><dd>{statValue(`${data.status.goos}/${data.status.goarch}`)}</dd></div><div><dt>{t.adminRuntimeLogsGoroutines}</dt><dd>{statValue(data.status.goroutines)}</dd></div><div><dt>{t.adminRuntimeLogsCPU}</dt><dd>{statValue(data.status.cpu_count)}</dd></div></dl></section>
      <section class="panel" aria-labelledby="service-title"><h2 id="service-title">{t.adminRuntimeLogsServiceStatus}</h2><dl class="facts"><div><dt>{t.adminRuntimeLogsStartedAt}</dt><dd>{dateLabel(data.status.started_at)}</dd></div><div><dt>{t.adminRuntimeLogsRedis}</dt><dd>{statValue(data.status.redis_enabled)}</dd></div><div><dt>{t.adminRuntimeLogsLevel}</dt><dd>{statValue(data.status.log_level)}</dd></div><div><dt>{t.adminRuntimeLogsBackend}</dt><dd>{statValue(data.status.runtime_log_backend || data.status.active_database)}</dd></div><div><dt>{t.adminRuntimeLogsBuffer}</dt><dd>{String(data.status.runtime_log_entries ?? 0)} / {String(data.status.runtime_log_limit ?? data.limit)}</dd></div><div><dt>{t.adminRuntimeLogsRoutes}</dt><dd>{statValue(data.status.routes)}</dd></div><div><dt>{t.adminRuntimeLogsHostUptime}</dt><dd>{duration(data.status.host_uptime_seconds)}</dd></div></dl></section>
      <section class="panel" aria-labelledby="load-title"><h2 id="load-title">{t.adminRuntimeLogsHostLoad}</h2><dl class="facts"><div><dt>{t.adminRuntimeLogsLoad}</dt><dd>{data.status.load_average?.join(" / ") || t.adminRuntimeLogsUnavailable}</dd></div><div><dt>{t.adminRuntimeLogsTotalMemory}</dt><dd>{bytes((data.status.host_memory?.total_kb || 0) * 1024)}</dd></div><div><dt>{t.adminRuntimeLogsAvailableMemory}</dt><dd>{bytes((data.status.host_memory?.available_kb || 0) * 1024)}</dd></div><div><dt>{t.adminRuntimeLogsCachedMemory}</dt><dd>{bytes((data.status.host_memory?.cached_kb || 0) * 1024)}</dd></div></dl></section>
    </div>
  {/if}
</section>

<style>
  .logs-page { display: grid; gap: 1rem; min-width: 0; } .page-heading, .heading-actions, .panel-heading, .log-row { align-items: flex-start; display: flex; gap: .75rem; } .page-heading, .panel-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; } .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; } h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; } .muted { color: #52606d; margin: .4rem 0 0; } .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; } .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .status-grid, .detail-grid { display: grid; gap: .8rem; grid-template-columns: repeat(4, minmax(0, 1fr)); } .detail-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } .stat { background: #fff; border: 1px solid #d7dee5; display: grid; gap: .3rem; min-width: 0; padding: .8rem; } .stat span, dt { color: #52606d; font-size: .8rem; } .stat strong { overflow-wrap: anywhere; }
  .log-list { background: #111820; border: 1px solid #263442; color: #edf2f7; display: grid; gap: .25rem; max-height: 68dvh; min-width: 0; overflow: auto; overscroll-behavior: contain; padding: .7rem; scrollbar-color: #718096 #1f2d3a; scrollbar-width: thin; } .log-row { align-items: start; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; min-width: 0; padding: .35rem .4rem; } .log-row:hover { background: #1c2935; } .log-row time { color: #8fa3b8; flex: 0 0 10rem; } .log-row p { flex: 1; margin: 0; min-width: 0; overflow-wrap: anywhere; white-space: pre-wrap; } .level { border: 1px solid #52606d; border-radius: .25rem; flex: 0 0 auto; font-size: .68rem; min-width: 4rem; padding: .12rem .3rem; text-align: center; text-transform: uppercase; } .level.info { border-color: #7cb6cf; color: #b8e5f5; } .level.warning { border-color: #e0a458; color: #ffe1a6; } .level.error { border-color: #e58b8b; color: #ffb8b8; } .level.default { color: #cbd5e0; }
  .facts { display: grid; gap: .55rem; margin: 0; } .facts div { align-items: baseline; border-bottom: 1px solid #e1e8ed; display: flex; gap: .8rem; justify-content: space-between; min-width: 0; padding-bottom: .5rem; } .facts div:last-child { border-bottom: 0; padding-bottom: 0; } dd { font-weight: 650; margin: 0; max-width: 70%; overflow-wrap: anywhere; text-align: right; } .empty { color: #52606d; padding: 1.5rem; text-align: center; }
  @media (max-width: 900px) { .status-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .detail-grid { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .panel-heading { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button { width: 100%; } .status-grid { grid-template-columns: 1fr; } .log-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; } .log-row time { grid-column: 1 / -1; } .log-row p { grid-column: 1 / -1; } .panel { padding: .85rem; } h1 { font-size: 1.65rem; } }
</style>
