<script lang="ts">
  import { t } from "$lib/i18n";
  import type { MigrationSummary } from "$lib/types";
  import type { ActionData, PageData } from "./$types";

  let { data, form }: { data: PageData; form: ActionData } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string; summary?: MigrationSummary });

  function formatBytes(value: number | undefined): string {
    const bytes = Number(value || 0);
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
  }

  function dateLabel(value: string | undefined): string {
    if (!value) return "-";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString("zh-CN");
  }
  function yesNo(value: boolean | undefined): string { return value ? t.adminMigrationYes : t.adminMigrationNo; }
  function confirmSubmit(event: SubmitEvent): void { if (!window.confirm(t.adminMigrationDangerConfirm)) event.preventDefault(); }
</script>

<svelte:head><title>{t.adminMigrationTitle} - {t.siteName}</title></svelte:head>

<section class="migration-page" aria-labelledby="migration-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="migration-title">{t.adminMigrationTitle}</h1><p class="muted">{t.adminMigrationDescription}</p></div>
    <div class="heading-actions"><a class="button secondary" href="/admin/database">{t.adminMigrationBackDatabase}</a><a class="button secondary" href="/admin/migration">{t.adminMigrationRefresh}</a></div>
  </header>

  {#if data.error}<p class="notice error" role="alert">{data.error}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}

  {#if data.status?.enabled}
    <section class="panel facts-panel" aria-labelledby="status-title">
      <header class="panel-heading"><div><h2 id="status-title">{t.adminMigrationTitle}</h2><p class="muted">{t.adminMigrationExportHelp}</p></div><span class="status-chip">{data.status.format_version}</span></header>
      <dl class="facts"><div><dt>{t.adminMigrationFormat}</dt><dd>{data.status.format_version}</dd></div><div><dt>{t.adminMigrationSchema}</dt><dd>{data.status.database_schema_version}</dd></div><div><dt>{t.adminMigrationLimit}</dt><dd>{formatBytes(data.status.max_archive_bytes)}</dd></div><div><dt>{t.adminMigrationNamespaces}</dt><dd>{data.status.resource_namespaces.length}</dd></div></dl>
      {#if data.status.resource_namespaces.length}<ul class="namespace-list">{#each data.status.resource_namespaces as namespace}<li>{namespace}</li>{/each}</ul>{:else}<p class="empty">{t.adminMigrationNoNamespaces}</p>{/if}
    </section>

    <section class="panel" aria-labelledby="export-title">
      <header class="panel-heading"><div><h2 id="export-title">{t.adminMigrationExportTitle}</h2><p class="muted">{t.adminMigrationExportHelp}</p></div></header>
      <form method="POST" action="/admin/migration/export-download" class="export-form"><label>{t.adminMigrationPassword}<input type="password" name="password" maxlength="1024" autocomplete="new-password" placeholder={t.adminMigrationPasswordPlaceholder} /></label><button class="button primary" type="submit">{t.adminMigrationExport}</button></form>
    </section>

    <section class="panel" aria-labelledby="import-title">
      <header class="panel-heading"><div><h2 id="import-title">{t.adminMigrationImportTitle}</h2><p class="muted">{t.adminMigrationImportHelp}</p></div></header>
      <form method="POST" action="?/preview" enctype="multipart/form-data" class="import-form"><label>{t.adminMigrationArchive}<input type="file" name="archive" accept="application/zip,.zip" required /></label><label>{t.adminMigrationImportPassword}<input type="password" name="password" maxlength="1024" autocomplete="off" /></label><label>{t.adminMigrationResourceMode}<select name="resource_mode"><option value="preserve">{t.adminMigrationPreserve}</option><option value="replace">{t.adminMigrationReplace}</option></select></label><label class="check"><input type="checkbox" name="apply_config" value="true" /><span><strong>{t.adminMigrationApplyConfig}</strong><small>{t.adminMigrationApplyConfigHelp}</small></span></label><button class="button primary" type="submit">{t.adminMigrationPreview}</button></form>
      {#if action.summary}
        <section class="preview" aria-labelledby="preview-title"><header class="preview-heading"><div><h3 id="preview-title">{t.adminMigrationPreviewTitle}</h3><p class="muted">{action.summary.dry_run ? t.adminMigrationPreviewOnly : t.adminMigrationImportSuccess}</p></div><span class:encrypted={action.summary.encrypted} class="status-chip">{action.summary.encrypted ? t.adminMigrationEncrypted : t.adminMigrationPlain}</span></header>
          <dl class="facts compact-facts"><div><dt>{t.adminMigrationFormat}</dt><dd>{action.summary.format_version || "-"}</dd></div><div><dt>{t.adminMigrationFiles}</dt><dd>{action.summary.file_count || 0}</dd></div><div><dt>{t.adminMigrationUncompressed}</dt><dd>{formatBytes(action.summary.total_uncompressed_bytes)}</dd></div><div><dt>{t.adminMigrationResources}</dt><dd>{action.summary.resource_count || 0}</dd></div><div><dt>{t.adminMigrationConflicts}</dt><dd>{action.summary.resource_conflict_count || 0}</dd></div><div><dt>{t.adminMigrationHasConfig}</dt><dd>{yesNo(action.summary.has_config)}</dd></div><div><dt>{t.adminMigrationSchema}</dt><dd>{action.summary.database_schema_version || "-"}</dd></div><div><dt>{t.adminMigrationExportTitle}</dt><dd>{dateLabel(action.summary.exported_at)}</dd></div></dl>
          {#if action.summary.resource_conflicts?.length}<p class="notice warning">{t.adminMigrationConflictHelp}</p><ul class="conflicts">{#each action.summary.resource_conflicts as conflict}<li>{conflict}</li>{/each}</ul>{/if}
          {#if action.summary.dry_run}<form method="POST" action="?/import" enctype="multipart/form-data" class="confirm-form" onsubmit={confirmSubmit}><input type="file" name="archive" accept="application/zip,.zip" required /><label>{t.adminMigrationImportPassword}<input type="password" name="password" maxlength="1024" autocomplete="off" /></label><input type="hidden" name="resource_mode" value={action.summary.resource_mode || "preserve"} />{#if action.summary.apply_config_requested}<input type="hidden" name="apply_config" value="true" />{/if}<label>{t.adminMigrationConfirmPhrase}<input name="confirm" maxlength="64" required placeholder={t.adminMigrationConfirmPlaceholder} /></label><p class="muted">{t.adminMigrationConfirmHelp}</p><button class="button danger" type="submit">{t.adminMigrationConfirm}</button></form>{/if}
        </section>
      {:else}<p class="empty">{t.adminMigrationNoPreview}</p>{/if}
    </section>
  {:else}<section class="panel"><p class="notice warning">{t.adminMigrationDisabled}</p></section>{/if}
</section>

<style>
  .migration-page { display: grid; gap: 1rem; min-width: 0; } .page-heading, .heading-actions, .panel-heading, .export-form, .import-form, .preview-heading, .confirm-form { align-items: flex-start; display: flex; gap: .8rem; min-width: 0; } .page-heading, .panel-heading, .preview-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; } .heading-actions { align-items: center; flex-wrap: wrap; justify-content: flex-end; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .4rem; text-transform: uppercase; } h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; } h3 { font-size: 1rem; } .muted { color: #52606d; line-height: 1.5; margin: .35rem 0 0; }
  .panel, .notice, .preview { border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .panel, .preview { background: #fff; display: grid; gap: .9rem; } .notice { margin: 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .button { align-items: center; border: 0; border-radius: .3rem; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.5rem; padding: .5rem .8rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary { background: #e8eef2; color: #16394a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; }
  .status-chip { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #486581; flex: 0 0 auto; font-size: .8rem; padding: .25rem .55rem; white-space: nowrap; } .status-chip.encrypted { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .facts { display: grid; gap: .6rem; grid-template-columns: repeat(4, minmax(0, 1fr)); margin: 0; } .facts div { border-bottom: 1px solid #e1e8ed; min-width: 0; padding-bottom: .5rem; } dt { color: #52606d; font-size: .8rem; } dd { font-weight: 650; margin: .25rem 0 0; overflow-wrap: anywhere; }
  .namespace-list, .conflicts { background: #f4f7f8; border: 1px solid #e1e8ed; columns: 2; column-gap: 2rem; margin: 0; max-height: 16rem; overflow: auto; padding: .7rem 1.8rem; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .conflicts { color: #7b4f00; }
  label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; max-width: 100%; min-height: 2.5rem; min-width: 0; padding: .5rem .6rem; } input:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .export-form label { flex: 1; max-width: 34rem; } .import-form { align-items: end; flex-wrap: wrap; } .import-form label { flex: 1 1 14rem; } .import-form .button { flex: 0 0 auto; } .check { align-items: flex-start; display: flex; gap: .55rem; } .check input { flex: 0 0 auto; min-height: 1rem; margin-top: .2rem; width: 1rem; } .check span { display: grid; gap: .15rem; } small { color: #52606d; font-weight: 400; line-height: 1.4; }
  .preview { background: #fbfcfd; } .preview-heading { align-items: flex-start; } .confirm-form { align-items: end; flex-wrap: wrap; border-top: 1px solid #e1e8ed; padding-top: .9rem; } .confirm-form > input[type="file"] { flex: 1 1 15rem; } .confirm-form label { flex: 1 1 14rem; } .confirm-form .muted { flex: 1 1 100%; } .empty { color: #52606d; padding: 1rem 0; text-align: center; }
  @media (max-width: 800px) { .facts { grid-template-columns: repeat(2, minmax(0, 1fr)); } .namespace-list, .conflicts { columns: 1; } } @media (max-width: 620px) { .page-heading, .heading-actions, .panel-heading, .export-form, .import-form, .preview-heading, .confirm-form { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .export-form .button, .import-form .button, .confirm-form .button { width: 100%; } .export-form label, .import-form label, .confirm-form label { max-width: none; width: 100%; } .facts { grid-template-columns: 1fr; } .panel, .preview { padding: .85rem; } .confirm-form > input[type="file"] { width: 100%; } }
</style>
