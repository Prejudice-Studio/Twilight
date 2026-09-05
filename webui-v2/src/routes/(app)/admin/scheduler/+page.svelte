<script lang="ts">
  import { t } from "$lib/i18n";
  import type { SchedulerJobItem, SchedulerJobRun, SchedulerTriggerSpec } from "$lib/types";
  import type { ActionData, PageData } from "./$types";

  let { data, form }: { data: PageData; form: ActionData } = $props();

  const parameterized = new Set([
    "cleanup_no_emby", "cleanup_pending_emby_entitlements", "cleanup_audit_logs",
    "cleanup_ticket_images", "sync_emby_activity_logs", "cleanup_unlinked_emby",
    "cleanup_emby_devices", "kick_unknown_group_members", "enforce_group_membership", "emby_sync"
  ]);

  function dateLabel(value: number | null | undefined): string {
    return value ? new Date(value * 1000).toLocaleString("zh-CN") : t.adminSchedulerNeverRun;
  }

  function duration(run: SchedulerJobRun | null | undefined): string {
    if (!run?.finished_at) return t.adminSchedulerRunning;
    const seconds = Math.max(0, run.finished_at - run.started_at);
    if (seconds < 60) return `${seconds} 秒`;
    return `${Math.round(seconds / 60)} 分钟`;
  }

  function triggerLabel(spec: SchedulerTriggerSpec | null | undefined): string {
    if (!spec || spec.type === "manual") return t.adminSchedulerModeManual;
    if (spec.type === "cron_daily") return `每日 ${String(spec.hour).padStart(2, "0")}:${String(spec.minute).padStart(2, "0")}`;
    if (spec.seconds % 3600 === 0) return `每 ${spec.seconds / 3600} 小时`;
    if (spec.seconds % 60 === 0) return `每 ${spec.seconds / 60} 分钟`;
    return `每 ${spec.seconds} 秒`;
  }

  function param(job: SchedulerJobItem, key: string, fallback: string): string {
    const value = job.runtime_params?.[key];
    return value === null || value === undefined ? fallback : String(value);
  }

  function flag(job: SchedulerJobItem, key: string, fallback: boolean): boolean {
    const value = job.runtime_params?.[key];
    return value === null || value === undefined ? fallback : Boolean(value);
  }

  function intervalValue(job: SchedulerJobItem): string {
    if (job.trigger_spec.type !== "interval") return "1";
    return job.trigger_spec.seconds % 3600 === 0
      ? String(job.trigger_spec.seconds / 3600)
      : String(Math.max(1, Math.round(job.trigger_spec.seconds / 60)));
  }

  function intervalUnit(job: SchedulerJobItem): string {
    return job.trigger_spec.type === "interval" && job.trigger_spec.seconds % 3600 === 0 ? "hours" : "minutes";
  }

  function statusLabel(job: SchedulerJobItem): string {
    if (job.is_running) return t.adminSchedulerStatusRunning;
    if (!job.last_run) return t.adminSchedulerNeverRun;
    return job.last_run.status === "success" ? t.adminSchedulerStatusSuccess : t.adminSchedulerStatusFailed;
  }

  function statusClass(job: SchedulerJobItem): string {
    if (job.is_running) return "running";
    if (job.last_run?.status === "success") return "success";
    if (job.last_run?.status === "failed") return "failed";
    return "idle";
  }

  function summaryEntries(run: SchedulerJobRun | null | undefined): Array<[string, string]> {
    if (!run?.summary) return [];
    return Object.entries(run.summary)
      .filter(([, value]) => value === null || ["string", "number", "boolean"].includes(typeof value))
      .map(([key, value]) => [key, typeof value === "boolean" ? (value ? "是" : "否") : String(value)]);
  }

  function confirmSubmit(event: SubmitEvent, message: string) {
    if (!globalThis.confirm(message)) event.preventDefault();
  }

  function viewHref(view: string): string {
    return view === "all" ? "/admin/scheduler" : `/admin/scheduler?view=${encodeURIComponent(view)}`;
  }

  function logsHref(jobID: string): string {
    const query = new URLSearchParams({ logs: jobID });
    if (data.view !== "all") query.set("view", data.view);
    return `/admin/scheduler?${query}`;
  }

  function actionError(): string {
    return (form as { error?: string } | null)?.error || "";
  }
</script>

<svelte:head><title>{t.adminSchedulerTitle} - {t.siteName}</title></svelte:head>

<section class="scheduler-page" aria-labelledby="scheduler-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="scheduler-title">{t.adminSchedulerTitle}</h1><p class="muted">{t.adminSchedulerDescription}</p></div>
    <div class="heading-actions"><a class="text-link" href="/admin/status">{t.adminStatusTitle}</a><a class="button primary" href={viewHref(data.view)}>{t.adminSchedulerRefresh}</a></div>
  </header>

  {#if data.load_error}<p class="notice error" role="alert">{data.load_error}</p>{/if}
  {#if actionError()}<p class="notice error" role="alert">{actionError()}</p>{/if}
  {#if data.notice === "run"}<p class="notice success" role="status">{t.adminSchedulerRunStarted}。{t.adminSchedulerRunStartedHelp}</p>{/if}
  {#if data.notice === "terminate"}<p class="notice success" role="status">{t.adminSchedulerTerminateDone}</p>{/if}
  {#if data.notice === "schedule"}<p class="notice success" role="status">{t.adminSchedulerScheduleSaved}</p>{/if}
  {#if data.notice === "reset"}<p class="notice success" role="status">{t.adminSchedulerScheduleReset}</p>{/if}

  <div class="metric-grid">
    <div class="metric"><span>{t.adminSchedulerRunning}</span><strong>{data.jobs.filter((job) => job.is_running).length}</strong></div>
    <div class="metric"><span>{t.adminSchedulerTimed}</span><strong>{data.jobs.filter((job) => !job.manual_only && job.trigger_spec.type !== "manual").length}</strong></div>
    <div class="metric"><span>{t.adminSchedulerFailed}</span><strong>{data.jobs.filter((job) => job.last_run?.status === "failed").length}</strong><small>{t.adminSchedulerFailedHint}</small></div>
    <div class="metric"><span>{t.adminSchedulerNext}</span><strong>{dateLabel(data.jobs.filter((job) => job.next_run_at).sort((a, b) => (a.next_run_at || 0) - (b.next_run_at || 0))[0]?.next_run_at)}</strong><small>{t.adminSchedulerNoSchedule}</small></div>
  </div>

  <section class="panel filter-panel" aria-labelledby="scheduler-filter-title">
    <div><h2 id="scheduler-filter-title">{t.adminSchedulerFilter}</h2><p class="muted">{t.adminSchedulerJobList}</p></div>
    <nav class="filter-list" aria-label={t.adminSchedulerFilter}>
      {#each [["all", t.adminSchedulerAll], ["running", t.adminSchedulerFilterRunning], ["failed", t.adminSchedulerFilterFailed], ["custom", t.adminSchedulerFilterCustom], ["manual", t.adminSchedulerFilterManual]] as [value, label]}
        <a class:active={data.view === value} class="filter-link" href={viewHref(value)}>{label}</a>
      {/each}
    </nav>
  </section>

  {#if data.jobs.length === 0}
    <section class="panel empty">{data.load_error ? t.adminSchedulerLoadFailed : t.adminSchedulerNoFilterJobs}</section>
  {:else}
    <div class="job-list">
      {#each data.jobs as job (job.id)}
        <article class="job-card">
          <header class="job-heading">
            <div class="job-title"><h2>{job.name}</h2><code>{job.id}</code><span class={`status ${statusClass(job)}`}>{statusLabel(job)}</span></div>
            <span class="job-trigger">{t.adminSchedulerTrigger.replace("{value}", triggerLabel(job.trigger_spec))}</span>
          </header>
          <p class="job-description">{job.description}</p>
          <dl class="job-facts">
            <div><dt>{t.adminSchedulerNext.replace("{value}", "").replace("：", "")}</dt><dd>{dateLabel(job.next_run_at)}</dd></div>
            <div><dt>{t.adminSchedulerLastAuto.replace("{value}", "").replace("：", "")}</dt><dd>{dateLabel(job.last_auto_run_at)}</dd></div>
            <div><dt>{t.adminSchedulerLastManual.replace("{value}", "").replace("：", "")}</dt><dd>{dateLabel(job.last_manual_run_at)}</dd></div>
            <div><dt>{t.adminSchedulerCustom}</dt><dd>{job.manual_only ? t.adminSchedulerManualOnly : job.is_custom ? t.adminSchedulerCustom : t.adminSchedulerNoSchedule}</dd></div>
          </dl>

          <div class="action-layout">
            <form method="POST" action="?/run" class="action-panel">
              <input type="hidden" name="job_id" value={job.id} />
              <h3>{t.adminSchedulerRunNow}</h3>
              {#if parameterized.has(job.id)}
                <details class="params"><summary>{t.adminSchedulerRunParams}</summary>
                  <p class="muted">{t.adminSchedulerParametersHelp}</p>
                  {#if job.id === "cleanup_no_emby"}<label>{t.adminSchedulerDays}<input name="days" type="number" min="1" max="3650" value={param(job, "days", "7")} /></label><label class="check"><input type="hidden" name="preserve_tg_bound" value="false" /><input type="checkbox" name="preserve_tg_bound" value="true" checked={flag(job, "preserve_tg_bound", true)} />{t.adminSchedulerPreserveTelegram}</label><label class="check"><input type="hidden" name="ignore_enabled_flag" value="false" /><input type="checkbox" name="ignore_enabled_flag" value="true" checked />{t.adminSchedulerIgnoreEnabled}</label>
                  {:else if job.id === "cleanup_pending_emby_entitlements"}<label class="check"><input type="hidden" name="ignore_enabled_flag" value="false" /><input type="checkbox" name="ignore_enabled_flag" value="true" checked />{t.adminSchedulerIgnoreEnabled}</label>
                  {:else if job.id === "cleanup_audit_logs"}<label>{t.adminSchedulerDays}<input name="retention_days" type="number" min="0" max="3650" value={param(job, "retention_days", "30")} /></label><label>{t.adminSchedulerMaxPerRun}<input name="max_entries" type="number" min="0" max="100000" value={param(job, "max_entries", "0")} /></label><label class="check"><input type="hidden" name="preserve_admin" value="false" /><input type="checkbox" name="preserve_admin" value="true" checked={flag(job, "preserve_admin", true)} />保留管理员日志</label><label class="check"><input type="checkbox" name="ignore_enabled_flag" value="true" checked />{t.adminSchedulerIgnoreEnabled}</label>
                  {:else if job.id === "cleanup_ticket_images"}<label>{t.adminSchedulerDays}<input name="retention_days" type="number" min="0" max="3650" value={param(job, "retention_days", "30")} /></label>
                  {:else if job.id === "sync_emby_activity_logs"}<label>{t.adminSchedulerSinceHours}<input name="since_hours" type="number" min="1" max="720" value="24" /></label>
                  {:else if job.id === "cleanup_unlinked_emby"}<label class="check"><input type="hidden" name="dry_run" value="false" /><input type="checkbox" name="dry_run" value="true" checked />{t.adminSchedulerDryRun}</label><label class="check"><input type="checkbox" name="allow_delete" value="true" />{t.adminSchedulerAllowDelete}</label>
                  {:else if job.id === "cleanup_emby_devices"}<label class="check"><input type="hidden" name="dry_run" value="false" /><input type="checkbox" name="dry_run" value="true" checked />{t.adminSchedulerDryRun}</label><label>{t.adminSchedulerMaxWorkers}<input name="max_workers" type="number" min="1" max="10" value={param(job, "max_workers", "10")} /></label><label>{t.adminSchedulerSkipUsernames}<textarea name="skip_usernames" rows="2" placeholder="每行一个用户名"></textarea></label>
                  {:else if job.id === "kick_unknown_group_members"}<label class="check"><input type="hidden" name="dry_run" value="false" /><input type="checkbox" name="dry_run" value="true" checked />{t.adminSchedulerDryRun}</label><label>{t.adminSchedulerMaxPerRun}<input name="max_per_run" type="number" min="1" max="500" value="200" /></label>
                  {:else if job.id === "enforce_group_membership"}<label class="check"><input type="checkbox" name="auto_enable_rejoined" value="true" checked={flag(job, "auto_enable_rejoined", false)} />{t.adminSchedulerAutoRejoin}</label>
                  {:else if job.id === "emby_sync"}<label>{t.adminSchedulerMaxPerRun}<input name="max_users" type="number" min="1" max="50000" value="1000" /></label>{/if}
                </details>
              {/if}
              <button class="button primary" type="submit" disabled={job.is_running}>{job.is_running ? t.adminSchedulerRunning : t.adminSchedulerRunNow}</button>
            </form>

            <section class="action-panel">
              <h3>{t.adminSchedulerEdit}</h3>
              {#if job.manual_only}<p class="muted">{t.adminSchedulerModeManual}</p>{:else}<form method="POST" action="?/schedule" class="schedule-form"><input type="hidden" name="job_id" value={job.id} /><label>{t.adminSchedulerTriggerMode}<select name="schedule_type" value={job.trigger_spec.type}><option value="manual">{t.adminSchedulerModeManual}</option><option value="cron_daily">{t.adminSchedulerModeDaily}</option><option value="interval">{t.adminSchedulerModeInterval}</option></select></label><div class="schedule-grid"><label>{t.adminSchedulerHour}<input name="hour" type="number" min="0" max="23" value={job.trigger_spec.type === "cron_daily" ? job.trigger_spec.hour : 0} /></label><label>{t.adminSchedulerMinute}<input name="minute" type="number" min="0" max="59" value={job.trigger_spec.type === "cron_daily" ? job.trigger_spec.minute : 0} /></label><label>{t.adminSchedulerEvery}<input name="interval_value" type="number" min="1" max="168" value={intervalValue(job)} /></label><label>{t.adminSchedulerUnit}<select name="interval_unit" value={intervalUnit(job)}><option value="minutes">{t.adminSchedulerMinutes}</option><option value="hours">{t.adminSchedulerHours}</option></select></label></div>{#if parameterized.has(job.id)}<div class="schedule-runtime"><p class="muted">{t.adminSchedulerRunParams}</p>{#if job.id === "cleanup_no_emby"}<label>{t.adminSchedulerDays}<input name="days" type="number" min="1" max="3650" value={param(job, "days", "7")} /></label><label class="check"><input type="checkbox" name="enabled" value="true" checked={flag(job, "enabled", true)} />{t.adminSchedulerAutoRejoin}</label>{:else if job.id === "cleanup_audit_logs"}<label>{t.adminSchedulerDays}<input name="retention_days" type="number" min="0" max="3650" value={param(job, "retention_days", "30")} /></label><label>{t.adminSchedulerMaxPerRun}<input name="max_entries" type="number" min="0" max="100000" value={param(job, "max_entries", "0")} /></label><label class="check"><input type="checkbox" name="preserve_admin" value="true" checked={flag(job, "preserve_admin", true)} />保留管理员日志</label>{:else if job.id === "sync_emby_activity_logs"}<label>{t.adminSchedulerSinceHours}<input name="since_hours" type="number" min="1" max="720" value="24" /></label>{:else if job.id === "enforce_group_membership"}<label class="check"><input type="checkbox" name="auto_enable_rejoined" value="true" checked={flag(job, "auto_enable_rejoined", false)} />{t.adminSchedulerAutoRejoin}</label>{/if}</div>{/if}<button class="button secondary" type="submit">{t.adminSchedulerEdit}</button></form>{/if}
              <div class="secondary-actions"><a class="button secondary" href={logsHref(job.id)}>{t.adminSchedulerViewLogs}</a>{#if job.is_custom}<form method="POST" action="?/resetSchedule" onsubmit={(event) => confirmSubmit(event, "确认恢复该任务的默认计划？")}><input type="hidden" name="job_id" value={job.id} /><button class="button secondary" type="submit">{t.adminSchedulerResetDefault}</button></form>{/if}{#if job.is_running}<form method="POST" action="?/terminate" onsubmit={(event) => confirmSubmit(event, t.adminSchedulerConfirmTerminate)}><input type="hidden" name="job_id" value={job.id} /><button class="button danger" type="submit">{t.adminSchedulerTerminate}</button></form>{/if}</div>
            </section>
          </div>
        </article>
      {/each}
    </div>
  {/if}

  {#if data.selected_job}
    <section class="panel logs-panel" aria-labelledby="logs-title">
      <header class="panel-heading"><div><h2 id="logs-title">{data.selected_job.name} · {t.adminSchedulerLastRunDetail}</h2><p class="muted">{data.selected_job.description}</p></div><a class="button secondary" href={viewHref(data.view)}>{t.commonClose}</a></header>
      {#if !data.logs}<p class="notice warning">{t.adminSchedulerNoDetail}</p>{:else if !data.logs.last_run}<p class="empty">{t.adminSchedulerNoRun}</p>{:else}
        {@const run = data.logs.last_run}
        <div class="run-detail"><p>{t.adminSchedulerStart.replace("{value}", dateLabel(run.started_at))}</p><p>{t.adminSchedulerEnd.replace("{value}", dateLabel(run.finished_at))}</p><p>{t.adminSchedulerDuration.replace("{value}", duration(run))}</p><p>{t.adminSchedulerType.replace("{value}", run.type === "manual" ? t.adminSchedulerRunTypeManual : t.adminSchedulerRunTypeAuto)}</p><p>{t.adminSchedulerStatus.replace("{value}", run.status)}</p>{#if run.error}<p class="error-text">{t.adminSchedulerError.replace("{value}", run.error)}</p>{/if}</div>
        {#if summaryEntries(run).length}<div class="summary"><h3>{t.adminSchedulerSummary}</h3>{#each summaryEntries(run) as [key, value]}<span><b>{key}</b>{value}</span>{/each}</div>{/if}
        {#if run.logs?.length}<pre class="log-output">{run.logs.join("\n")}</pre>{:else}<p class="muted">{t.adminSchedulerNoLogs}</p>{/if}
        {#if data.logs.history.length}<section class="history"><h3>{t.adminSchedulerHistory.replace("{count}", String(data.logs.history.length))}</h3>{#each data.logs.history as history (history.id || history.started_at)}<details><summary><span>{dateLabel(history.started_at)}</span><span>{history.status} · {duration(history)}</span></summary>{#if history.error}<p class="error-text">{history.error}</p>{/if}{#if history.logs?.length}<pre class="log-output">{history.logs.join("\n")}</pre>{/if}</details>{/each}</section>{/if}
      {/if}
    </section>
  {/if}
</section>

<style>
  .scheduler-page { display: grid; gap: 1rem; min-width: 0; } .page-heading, .heading-actions, .panel-heading, .job-heading, .job-title, .secondary-actions { align-items: flex-start; display: flex; gap: .75rem; } .page-heading, .panel-heading, .job-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1rem; } .heading-actions, .secondary-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; } h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.1rem; } h3 { font-size: .95rem; } .muted { color: #52606d; margin: .35rem 0 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.45rem; max-width: 100%; padding: .5rem .8rem; text-decoration: none; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.danger { background: #a63d40; color: #fff; } .button.secondary:hover { background: #d6e1e7; } button:disabled { cursor: not-allowed; opacity: .6; }
  .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .metric-grid { display: grid; gap: .75rem; grid-template-columns: repeat(4, minmax(0, 1fr)); } .metric, .panel, .job-card { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; } .metric { display: grid; gap: .3rem; padding: .8rem; } .metric span, .metric small { color: #52606d; font-size: .8rem; } .metric strong { font-size: 1.2rem; min-width: 0; overflow-wrap: anywhere; }
  .panel { padding: 1rem; } .filter-panel { align-items: center; display: flex; gap: 1rem; justify-content: space-between; } .filter-list { display: flex; flex-wrap: wrap; gap: .35rem; } .filter-link { border: 1px solid #c8d2da; border-radius: .3rem; color: #243b53; min-height: 2.2rem; padding: .45rem .7rem; text-decoration: none; } .filter-link.active, .filter-link:hover { background: #e8eef2; border-color: #78909c; }
  .job-list { display: grid; gap: .9rem; } .job-card { display: grid; gap: .85rem; padding: 1rem; } .job-title { align-items: center; flex-wrap: wrap; min-width: 0; } .job-title code { background: #eef2f4; color: #52606d; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .35rem; } .status { border-radius: 999px; font-size: .75rem; padding: .25rem .5rem; white-space: nowrap; } .status.running { background: #e5f2f7; color: #14516a; } .status.success { background: #edf7f0; color: #276749; } .status.failed { background: #fff1f0; color: #a61b1b; } .status.idle { background: #eef2f4; color: #52606d; } .job-trigger { color: #52606d; flex: 0 0 auto; font-size: .82rem; } .job-description { color: #52606d; margin: 0; }
  .job-facts { display: grid; gap: .45rem; grid-template-columns: repeat(4, minmax(0, 1fr)); margin: 0; } .job-facts div { border-left: 3px solid #c8d2da; min-width: 0; padding-left: .6rem; } dt { color: #52606d; font-size: .75rem; } dd { font-weight: 650; margin: .2rem 0 0; overflow-wrap: anywhere; }
  .action-layout { display: grid; gap: .75rem; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); } .action-panel { border: 1px solid #e1e8ed; display: grid; gap: .65rem; min-width: 0; padding: .8rem; } .action-panel h3 { border-bottom: 1px solid #e1e8ed; padding-bottom: .5rem; } .params { border: 1px solid #e1e8ed; min-width: 0; padding: .5rem; } .params summary { cursor: pointer; font-weight: 650; } label { color: #243b53; display: grid; font-size: .82rem; gap: .3rem; min-width: 0; } input, textarea, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.35rem; min-width: 0; max-width: 100%; padding: .4rem .55rem; } textarea { resize: vertical; } .check { align-items: center; display: flex; grid-template-columns: auto 1fr; } .check input[type="checkbox"] { min-height: 1rem; width: 1rem; } .schedule-form, .schedule-runtime { display: grid; gap: .55rem; } .schedule-grid { display: grid; gap: .5rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .schedule-runtime { border-top: 1px solid #e1e8ed; padding-top: .55rem; }
  .logs-panel { display: grid; gap: .8rem; } .run-detail, .summary { display: flex; flex-wrap: wrap; gap: .5rem .8rem; } .run-detail p { margin: 0; } .summary { border-top: 1px solid #e1e8ed; padding-top: .7rem; } .summary h3 { flex-basis: 100%; } .summary span { background: #eef2f4; border-radius: .25rem; font-size: .78rem; padding: .3rem .45rem; } .summary b { margin-right: .25rem; } .log-output { background: #111820; border: 1px solid #263442; color: #edf2f7; max-height: 40dvh; margin: 0; overflow: auto; overscroll-behavior: contain; padding: .75rem; scrollbar-color: #718096 #1f2d3a; scrollbar-width: thin; white-space: pre-wrap; overflow-wrap: anywhere; } .history { border-top: 1px solid #e1e8ed; display: grid; gap: .45rem; padding-top: .75rem; } .history details { border: 1px solid #e1e8ed; padding: .55rem; } .history summary { cursor: pointer; display: flex; flex-wrap: wrap; gap: .5rem 1rem; justify-content: space-between; } .error-text { color: #a61b1b; overflow-wrap: anywhere; } .empty { color: #52606d; text-align: center; }
  a:focus-visible, button:focus-visible, input:focus-visible, textarea:focus-visible, select:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 900px) { .metric-grid, .job-facts { grid-template-columns: repeat(2, minmax(0, 1fr)); } .action-layout { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .filter-panel, .panel-heading, .job-heading { align-items: stretch; flex-direction: column; } .heading-actions > *, .heading-actions .button { width: 100%; } .metric-grid, .job-facts { grid-template-columns: 1fr; } .job-title { align-items: flex-start; flex-direction: column; } .job-trigger { align-self: flex-start; } .schedule-grid { grid-template-columns: 1fr; } .secondary-actions > *, .secondary-actions form, .secondary-actions .button { width: 100%; } .panel, .job-card { padding: .8rem; } h1 { font-size: 1.65rem; } }
</style>
