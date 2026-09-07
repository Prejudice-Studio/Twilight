<script lang="ts">
  import { t } from "$lib/i18n";
  import PageHeader from "$lib/components/PageHeader.svelte";
  import Panel from "$lib/components/Panel.svelte";
  import type { DatabaseBackupInspectResult, DatabaseOperationResult, DatabaseStatus } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = {
    action?: string;
    error?: string;
    backupPreview?: DatabaseBackupInspectResult;
    restorePreview?: DatabaseOperationResult;
    migrationPreview?: DatabaseOperationResult;
  };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let targetDriver = $state<"postgres" | "json">("json");
  let stateFile = $state("");

  function formatBytes(value: number | undefined): string {
    const size = Number(value || 0);
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(2)} MB`;
    return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
  }

  function dateLabel(value: number | undefined): string {
    return value ? new Date(value * 1000).toLocaleString("zh-CN") : "-";
  }

  function valueLabel(value: unknown): string {
    if (value === null || value === undefined || value === "") return "-";
    if (typeof value === "boolean") return value ? t.adminDatabaseEnabled : t.adminDatabaseDisabled;
    return String(value);
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function counts(result: DatabaseOperationResult | DatabaseBackupInspectResult | null | undefined): Array<[string, number]> {
    return Object.entries(result?.counts || {}).filter(([, value]) => Number(value) > 0).map(([key, value]) => [key, Number(value)]);
  }

  function statusState(status: DatabaseStatus | null): string {
    if (!status) return t.adminDatabaseNoStatus;
    if (status.storage_mismatch) return t.adminDatabaseMismatch;
    return t.adminDatabaseRuntime;
  }
</script>

<svelte:head><title>{t.adminDatabaseTitle} - {t.siteName}</title></svelte:head>

<section class="database-page" aria-labelledby="database-title">
  <PageHeader id="database-title" eyebrow={t.adminArea} title={t.adminDatabaseTitle} description={t.adminDatabaseDescription}>
    {#snippet actions()}
      <a class="back-link" href="/admin/status">{t.adminStatusTitle}</a>
      <a class="button secondary" href="/admin/database">{t.adminDatabaseRefresh}</a>
    {/snippet}
  </PageHeader>

  {#if data.errors.length}<div class="notice error" role="alert">{data.errors.join("；")}</div>{/if}
  {#if action.error}<div class="notice error" role="alert">{action.error}</div>{/if}

  <Panel id="status-heading" className="status-panel">
    <header class="panel-heading"><div><h2 id="status-heading">{t.adminDatabaseStatus}</h2><p class="muted">{statusState(data.status)}</p></div><span class="status-chip">{data.status?.active_label || data.status?.active_driver || "-"}</span></header>
    {#if data.status}
      <dl class="facts"><div><dt>{t.adminDatabaseRuntime}</dt><dd>{valueLabel(data.status.active_label || data.status.active_driver)}</dd></div><div><dt>{t.adminDatabaseConfigured}</dt><dd>{valueLabel(data.status.configured_label || data.status.configured_driver)}</dd></div><div><dt>{t.adminDatabaseUserCount}</dt><dd>{data.status.user_count.toLocaleString("zh-CN")}</dd></div><div><dt>{t.adminDatabaseBackupCount}</dt><dd>{data.status.backup_count.toLocaleString("zh-CN")}</dd></div><div><dt>{t.adminDatabaseRedis}</dt><dd>{valueLabel(data.status.redis_enabled)}</dd></div></dl>
      {#if data.status.storage_mismatch}<p class="notice warning">{data.status.storage_warning || t.adminDatabaseMismatch}</p>{/if}
    {:else}<p class="empty">{t.adminDatabaseNoStatus}</p>{/if}
  </Panel>

  <Panel id="backups-heading" className="backups-panel">
    <header class="panel-heading"><div><h2 id="backups-heading">{t.adminDatabaseBackups}</h2><p class="muted">{t.adminDatabaseRestoreConfirm}</p></div></header>
    <form method="POST" action="?/createBackup" class="backup-create"><label>{t.adminDatabaseBackupNote}<input name="note" maxlength="200" /></label><button class="button primary" type="submit">{t.adminDatabaseCreateBackup}</button></form>
    {#if action.backupPreview}<section class="result-panel"><header class="result-heading"><h3>{t.adminDatabaseBackupPreview}</h3><span>{action.backupPreview.backup.name}</span></header><p>{t.adminDatabaseSnapshotBytes}: {formatBytes(action.backupPreview.snapshot_bytes)}</p><div class="count-grid">{#each counts(action.backupPreview) as [key, value]}<span><b>{key}</b>{value}</span>{/each}</div></section>{/if}
    {#if action.restorePreview}<section class="result-panel warning-panel"><h3>{t.adminDatabaseRestorePreview}</h3><p>{action.restorePreview.restored || action.restorePreview.backup?.name || "-"} · {formatBytes(action.restorePreview.target_snapshot_bytes)}</p><div class="count-grid">{#each counts(action.restorePreview) as [key, value]}<span><b>{key}</b>{value}</span>{/each}</div>{#if action.restorePreview.warnings?.length}<ul>{#each action.restorePreview.warnings as warning}<li>{warning}</li>{/each}</ul>{/if}<form method="POST" action="?/restore" onsubmit={(event) => confirmSubmit(event, t.adminDatabaseRestoreConfirm)}><input type="hidden" name="name" value={action.restorePreview.restored || ""} /><input type="hidden" name="confirm" value="RESTORE_DATABASE_BACKUP" /><button class="button danger" type="submit">{t.adminDatabaseRestore}</button></form></section>{/if}
    {#if data.backups.length}<div class="backup-list">{#each data.backups as backup (backup.name)}<article class="backup-row"><div class="backup-meta"><strong>{backup.name}</strong><span>{formatBytes(backup.size)} · {dateLabel(backup.created_at)}</span>{#if backup.note}<span>{backup.note}</span>{/if}</div><div class="backup-actions"><form method="POST" action="?/inspectBackup"><input type="hidden" name="name" value={backup.name} /><button class="button compact secondary" type="submit">{t.adminDatabaseInspect}</button></form><form method="POST" action="?/restorePreview"><input type="hidden" name="name" value={backup.name} /><button class="button compact secondary" type="submit">{t.adminDatabaseRestorePreview}</button></form><form method="POST" action="?/deleteBackup" onsubmit={(event) => confirmSubmit(event, t.adminDatabaseDeleteConfirm)}><input type="hidden" name="name" value={backup.name} /><button class="button compact danger" type="submit">{t.adminDatabaseDelete}</button></form></div></article>{/each}</div>{:else}<p class="empty">{t.adminDatabaseNoBackups}</p>{/if}
  </Panel>

  <Panel id="migration-heading" className="migration-panel">
    <header class="panel-heading"><div><h2 id="migration-heading">{t.adminDatabaseMigration}</h2><p class="muted">{t.adminDatabaseMigrationHelp}</p></div><span class="status-chip">{data.status?.migration_panel_enabled ? t.adminDatabaseEnabled : t.adminDatabaseMigrationDisabled}</span></header>
    {#if data.status?.migration_panel_enabled}
      <form method="POST" action="?/migrationPreview" class="migration-form"><label>{t.adminDatabaseTarget}<select name="target_driver" bind:value={targetDriver}><option value="json">{t.adminDatabaseJSON}</option><option value="postgres" disabled={!data.status.postgres_configured}>{t.adminDatabasePostgres}</option></select></label><label>{t.adminDatabaseStateFile}<input name="state_file" bind:value={stateFile} maxlength="240" placeholder={t.adminDatabaseStateFilePlaceholder} /></label><button class="button primary" type="submit">{t.adminDatabasePreflight}</button></form>
      {#if action.migrationPreview}<section class="result-panel"><h3>{t.adminDatabaseMigrationPreview}</h3><dl class="facts"><div><dt>{t.adminDatabaseSource}</dt><dd>{action.migrationPreview.source_driver || "-"}</dd></div><div><dt>{t.adminDatabaseTarget}</dt><dd>{action.migrationPreview.target_driver || "-"}</dd></div><div><dt>{t.adminDatabaseSnapshotBytes}</dt><dd>{formatBytes(action.migrationPreview.snapshot_bytes)}</dd></div></dl>{#if action.migrationPreview.warnings?.length}<ul>{#each action.migrationPreview.warnings as warning}<li>{warning}</li>{/each}</ul>{/if}<form method="POST" action="?/migrate" onsubmit={(event) => confirmSubmit(event, t.adminDatabaseMigrationHelp)}><input type="hidden" name="target_driver" value={action.migrationPreview.target_driver || targetDriver} /><input type="hidden" name="state_file" value={stateFile} /><input type="hidden" name="confirm" value="MIGRATE_DATABASE" /><button class="button danger" type="submit">{t.adminDatabaseExecuteMigration}</button></form></section>{:else}<p class="empty">{t.adminDatabaseNoPreview}</p>{/if}
    {:else}<p class="notice warning">{t.adminDatabaseMigrationDisabled}</p>{/if}
  </Panel>
</section>

<style>
  .database-page { display: grid; gap: 1rem; min-width: 0; }
  .panel-heading, .backup-create, .backup-row, .backup-actions, .migration-form, .result-heading { align-items: flex-start; display: flex; gap: .75rem; }
  .panel-heading, .backup-row, .result-heading { justify-content: space-between; } .backup-actions { align-items: center; flex-wrap: wrap; }
  h2, h3, p { overflow-wrap: anywhere; } h2, h3 { margin: 0; } h2 { font-size: 1.2rem; } h3 { font-size: 1rem; } .muted { color: #52606d; margin: .4rem 0 0; } .back-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .65rem; }
  .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.warning, .warning-panel { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .status-chip { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #486581; flex: 0 0 auto; font-size: .8rem; padding: .25rem .55rem; white-space: nowrap; }
  .facts { display: grid; gap: .6rem; grid-template-columns: repeat(3, minmax(0, 1fr)); margin: 1rem 0 0; } .facts div { border-bottom: 1px solid #e1e8ed; min-width: 0; padding-bottom: .5rem; } dt { color: #52606d; font-size: .8rem; } dd { font-weight: 650; margin: .25rem 0 0; overflow-wrap: anywhere; }
  label { color: #243b53; display: grid; font-size: .85rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } input:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .backup-create { align-items: end; margin-top: 1rem; } .backup-create label { flex: 1; max-width: 32rem; } .backup-list { display: grid; gap: .55rem; margin-top: 1rem; max-height: 65dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .backup-row { align-items: center; border: 1px solid #d7dee5; min-width: 0; padding: .75rem; } .backup-meta { display: grid; gap: .25rem; min-width: 0; } .backup-meta strong, .backup-meta span { overflow-wrap: anywhere; } .backup-meta span { color: #52606d; font-size: .78rem; }
  .backup-actions { justify-content: end; } .result-panel { border: 1px solid #d7dee5; display: grid; gap: .65rem; margin-top: 1rem; padding: .8rem; } .result-heading span { color: #52606d; font-size: .8rem; overflow-wrap: anywhere; } .count-grid { display: grid; gap: .45rem; grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr)); } .count-grid span { background: #f4f7f8; border: 1px solid #e1e8ed; display: flex; justify-content: space-between; gap: .5rem; padding: .45rem .55rem; } .count-grid b { color: #52606d; font-size: .78rem; overflow-wrap: anywhere; } .result-panel ul { margin: 0; padding-left: 1.25rem; } .migration-form { align-items: end; } .migration-form label { flex: 1; } .empty { color: #52606d; padding: 1.5rem .75rem; text-align: center; }
  @media (max-width: 700px) { .facts { grid-template-columns: repeat(2, minmax(0, 1fr)); } .backup-create, .migration-form { align-items: stretch; flex-direction: column; } .backup-create label, .migration-form label, .backup-create .button, .migration-form .button { max-width: none; width: 100%; } .backup-row { align-items: stretch; flex-direction: column; } .backup-actions { align-items: stretch; } .backup-actions form, .backup-actions .button { flex: 1 1 8rem; } }
  @media (max-width: 560px) { .panel-heading, .result-heading { align-items: stretch; flex-direction: column; } .facts { grid-template-columns: 1fr; } .backup-actions { flex-direction: column; } .backup-actions .button { width: 100%; } }
</style>
