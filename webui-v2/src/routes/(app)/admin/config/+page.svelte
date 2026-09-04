<script lang="ts">
  import { t } from "$lib/i18n";
  import type { ConfigBackupView, ConfigField, ConfigRestoreResult, ConfigSection } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = {
    action?: string;
    error?: string;
    backupView?: ConfigBackupView;
    restorePreview?: ConfigRestoreResult;
  };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let activeTab = $state<"schema" | "toml" | "backups">("schema");
  let activeSection = $state("");
  let search = $state("");
  let sections = $state<ConfigSection[]>([]);
  let tomlContent = $state("");
  let visibleSecrets = $state<Record<string, boolean>>({});
  let initialized = $state(false);

  function cloneSections(source: ConfigSection[]): ConfigSection[] {
    return source.map((section) => ({
      ...section,
      fields: section.fields.map((field) => ({ ...field, value: cloneValue(field.value) }))
    }));
  }

  function cloneValue(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(cloneValue);
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, cloneValue(item)]));
    return value;
  }

  function updateField(sectionKey: string, fieldKey: string, value: unknown): void {
    sections = sections.map((section) => section.key !== sectionKey ? section : {
      ...section,
      fields: section.fields.map((field) => field.key === fieldKey ? { ...field, value } : field)
    });
  }

  function resetField(sectionKey: string, field: ConfigField): void {
    const original = data.schema?.sections.find((section) => section.key === sectionKey)?.fields.find((item) => item.key === field.key);
    updateField(sectionKey, field.key, cloneValue(original?.value));
  }

  function isChanged(sectionKey: string, field: ConfigField): boolean {
    const original = data.schema?.sections.find((section) => section.key === sectionKey)?.fields.find((item) => item.key === field.key)?.value;
    return JSON.stringify(field.value) !== JSON.stringify(original);
  }

  function sectionMatches(section: ConfigSection): boolean {
    const term = search.trim().toLowerCase();
    if (!term) return true;
    return `${section.title} ${section.key} ${section.description} ${section.fields.map((field) => `${field.label} ${field.key} ${field.description}`).join(" ")}`.toLowerCase().includes(term);
  }

  function visibleFields(section: ConfigSection): ConfigField[] {
    const term = search.trim().toLowerCase();
    if (!term) return section.fields;
    return section.fields.filter((field) => `${field.label} ${field.key} ${field.description}`.toLowerCase().includes(term));
  }

  function schemaPayload(): Record<string, Record<string, unknown>> {
    return Object.fromEntries(sections.map((section) => [section.key, Object.fromEntries(section.fields.map((field) => [field.key, field.value]))]));
  }

  function listValue(field: ConfigField): string[] {
    return Array.isArray(field.value) ? field.value.map((value) => String(value ?? "")) : field.value ? [String(field.value)] : [];
  }

  function updateList(sectionKey: string, field: ConfigField, index: number, value: string): void {
    const next = listValue(field);
    next[index] = value;
    updateField(sectionKey, field.key, next);
  }

  function commandRows(field: ConfigField): Array<{ command: string; reply: string }> {
    if (!Array.isArray(field.value)) return [];
    return field.value.map((item) => {
      if (item && typeof item === "object") {
        const row = item as Record<string, unknown>;
        return { command: String(row.command ?? ""), reply: String(row.reply ?? "") };
      }
      const [command, ...reply] = String(item ?? "").split(" = ");
      return { command, reply: reply.join(" = ") };
    });
  }

  function updateCommand(sectionKey: string, field: ConfigField, index: number, key: "command" | "reply", value: string): void {
    const next = commandRows(field);
    next[index] = { ...next[index], [key]: value };
    updateField(sectionKey, field.key, next);
  }

  function formatBytes(value: number): string {
    if (value < 1024) return `${value} B`;
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
    return `${(value / 1024 / 1024).toFixed(2)} MB`;
  }

  function dateLabel(value: number): string {
    return value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : "-";
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  $effect(() => {
    if (!initialized) {
      sections = cloneSections(data.schema?.sections || []);
      activeSection = data.schema?.sections[0]?.key || "";
      tomlContent = data.toml?.content || "";
      initialized = true;
    }
    if (!sections.some((section) => section.key === activeSection)) activeSection = sections[0]?.key || "";
  });
</script>

<svelte:head><title>{t.adminConfigTitle} - {t.siteName}</title></svelte:head>

<section class="config-page" aria-labelledby="config-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="config-title">{t.adminConfigTitle}</h1><p class="muted">{t.adminConfigDescription}</p></div>
    <div class="heading-actions"><a class="back-link" href="/admin/status">{t.adminStatusTitle}</a><a class="button secondary" href="/admin/config">{t.adminConfigRefresh}</a></div>
  </header>

  {#if data.errors.length}<div class="notice error" role="alert">{data.errors.join("；")}</div>{/if}
  {#if action.error}<div class="notice error" role="alert">{action.error}</div>{/if}

  <nav class="tabs" aria-label={t.adminConfigTitle}>
    <button class:active={activeTab === "schema"} type="button" onclick={() => activeTab = "schema"}>{t.adminConfigSchema}</button>
    <button class:active={activeTab === "toml"} type="button" onclick={() => activeTab = "toml"}>{t.adminConfigToml}</button>
    <button class:active={activeTab === "backups"} type="button" onclick={() => activeTab = "backups"}>{t.adminConfigBackups}</button>
  </nav>

  {#if activeTab === "schema"}
    <section class="workspace" aria-labelledby="schema-heading">
      <aside class="section-nav">
        <label class="search-label" for="config-search">{t.adminConfigSearch}</label>
        <input id="config-search" type="search" bind:value={search} placeholder={t.adminConfigSearch} />
        <div class="section-list">
          {#each sections.filter(sectionMatches) as section}
            <button type="button" class:current={section.key === activeSection} onclick={() => activeSection = section.key}>
              <strong>{section.title}</strong><span>{section.fields.length}{t.adminConfigFieldKey}</span>
            </button>
          {/each}
        </div>
      </aside>

      <div class="editor-column">
        {#if data.schema && sections.length}
          {@const current = sections.find((section) => section.key === activeSection) || sections[0]}
          <header class="editor-heading"><div><p class="eyebrow">{current.key}</p><h2 id="schema-heading">{current.title}</h2><p class="muted">{current.description}</p></div><span class="field-count">{visibleFields(current).length} / {current.fields.length}</span></header>
          <form method="POST" action="?/saveSchema" class="schema-form">
            <input type="hidden" name="sections" value={JSON.stringify(schemaPayload())} />
            {#if visibleFields(current).length}
              {#each visibleFields(current) as field (field.key)}
                <article class:changed={isChanged(current.key, field)} class="field-row">
                  <div class="field-heading"><div><label for={`${current.key}-${field.key}`}>{field.label}</label><code>{field.key}</code></div>{#if isChanged(current.key, field)}<button type="button" class="reset-link" onclick={() => resetField(current.key, field)}>{t.adminConfigReset}</button>{/if}</div>
                  <p class="field-description">{field.description}</p>
                  {#if field.type === "bool"}
                    <label class="switch"><input id={`${current.key}-${field.key}`} type="checkbox" checked={Boolean(field.value)} onchange={(event) => updateField(current.key, field.key, (event.currentTarget as HTMLInputElement).checked)} /><span>{field.value ? "开启" : "关闭"}</span></label>
                  {:else if field.type === "secret"}
                    <div class="secret-control"><input id={`${current.key}-${field.key}`} type={visibleSecrets[`${current.key}.${field.key}`] ? "text" : "password"} value={String(field.value ?? "")} oninput={(event) => updateField(current.key, field.key, (event.currentTarget as HTMLInputElement).value)} placeholder={field.value ? t.adminConfigSecretSet : t.adminConfigSecretEmpty} /><button type="button" class="button compact" onclick={() => visibleSecrets[`${current.key}.${field.key}`] = !visibleSecrets[`${current.key}.${field.key}`]}>{visibleSecrets[`${current.key}.${field.key}`] ? t.adminConfigHideSecret : t.adminConfigShowSecret}</button></div>
                  {:else if field.type === "textarea"}
                    <textarea id={`${current.key}-${field.key}`} rows="6" oninput={(event) => updateField(current.key, field.key, (event.currentTarget as HTMLTextAreaElement).value)}>{String(field.value ?? "")}</textarea>
                  {:else if field.type === "list"}
                    <div class="list-editor">{#each listValue(field) as item, index}<div class="list-row"><input value={item} oninput={(event) => updateList(current.key, field, index, (event.currentTarget as HTMLInputElement).value)} /><button type="button" class="icon-button" aria-label={t.adminConfigRemoveItem} onclick={() => updateField(current.key, field.key, listValue(field).filter((_, itemIndex) => itemIndex !== index))}>×</button></div>{/each}<button type="button" class="button compact secondary" onclick={() => updateField(current.key, field.key, [...listValue(field), ""])}>{t.adminConfigAddItem}</button></div>
                  {:else if field.type === "command_map"}
                    <div class="command-editor">{#each commandRows(field) as row, index}<div class="command-row"><input value={row.command} placeholder={t.adminConfigCommand} oninput={(event) => updateCommand(current.key, field, index, "command", (event.currentTarget as HTMLInputElement).value)} /><textarea rows="3" placeholder={t.adminConfigReply} oninput={(event) => updateCommand(current.key, field, index, "reply", (event.currentTarget as HTMLTextAreaElement).value)}>{row.reply}</textarea><button type="button" class="icon-button" aria-label={t.adminConfigRemoveItem} onclick={() => updateField(current.key, field.key, commandRows(field).filter((_, itemIndex) => itemIndex !== index))}>×</button></div>{/each}<button type="button" class="button compact secondary" onclick={() => updateField(current.key, field.key, [...commandRows(field), { command: "/", reply: "" }])}>{t.adminConfigAddCommand}</button></div>
                  {:else if field.type === "select"}
                    <select id={`${current.key}-${field.key}`} value={String(field.value ?? "")} onchange={(event) => { const option = (event.currentTarget as HTMLSelectElement).selectedOptions[0]; updateField(current.key, field.key, option?.dataset.valueType === "number" ? Number(option.value) : option?.value || ""); }}>
                      {#each field.options || [] as option}<option value={String(option.value)} data-value-type={typeof option.value === "number" ? "number" : "string"} selected={String(field.value ?? "") === String(option.value)}>{option.label}</option>{/each}
                    </select>
                  {:else}
                    <input id={`${current.key}-${field.key}`} type={field.type === "int" || field.type === "float" ? "number" : "text"} step={field.type === "float" ? "0.01" : undefined} value={String(field.value ?? "")} oninput={(event) => { const raw = (event.currentTarget as HTMLInputElement).value; updateField(current.key, field.key, field.type === "int" ? Number.parseInt(raw || "0", 10) : field.type === "float" ? Number.parseFloat(raw || "0") : raw); }} />
                  {/if}
                  {#if field.placeholder_hints?.length}<p class="hints">占位符：{field.placeholder_hints.join("、")}</p>{/if}
                </article>
              {/each}
            {:else}<p class="empty">{t.adminConfigAllSections}</p>{/if}
            <div class="form-footer"><span class="muted">{t.adminConfigSchemaHelp}</span><button class="button primary" type="submit">{t.adminConfigSave}</button></div>
          </form>
        {:else}<p class="empty">{t.adminConfigNoSchema}</p>{/if}
      </div>
    </section>
  {:else if activeTab === "toml"}
    <section class="panel" aria-labelledby="toml-heading">
      <header class="panel-heading"><div><h2 id="toml-heading">{t.adminConfigToml}</h2><p class="muted">{t.adminConfigTomlHelp}</p></div><span class="path">{data.toml?.path || "-"}</span></header>
      <form method="POST" action="?/saveToml" class="toml-form"><textarea name="content" bind:value={tomlContent} rows="28" spellcheck="false" aria-label={t.adminConfigToml}></textarea><div class="form-footer"><span class="muted">{t.adminConfigTomlHelp}</span><button class="button primary" type="submit">{t.adminConfigSaveToml}</button></div></form>
    </section>
  {:else}
    <section class="panel backups-panel" aria-labelledby="backups-heading">
      <header class="panel-heading"><div><h2 id="backups-heading">{t.adminConfigBackups}</h2><p class="muted">{t.adminConfigBackupHelp}</p></div><div class="toolbar"><form method="POST" action="?/createBackup"><button class="button secondary" type="submit">{t.adminConfigBackup}</button></form><form method="POST" action="?/sweep" onsubmit={(event) => confirmSubmit(event, t.adminConfigSweepHelp)}><button class="button secondary" type="submit">{t.adminConfigSweep}</button></form></div></header>
      <div class="upload-panel"><div><strong>{t.adminConfigUploadBackground}</strong><p class="muted">{t.adminConfigUploadHelp}</p></div><form method="POST" action="?/uploadBackground" enctype="multipart/form-data" class="upload-form"><input type="file" name="file" accept="image/jpeg,image/png,image/gif,image/webp,image/bmp" required /><button class="button secondary" type="submit">{t.adminConfigUpload}</button></form></div>
      {#if action.backupView}<section class="preview-panel"><div class="panel-heading"><h3>{action.backupView.backup.name}</h3><span>{formatBytes(action.backupView.backup.size)}</span></div><pre>{action.backupView.content}</pre></section>{/if}
      {#if action.restorePreview}<section class="restore-panel"><h3>{t.adminConfigRestorePreview}</h3><p>{action.restorePreview.restored} · {action.restorePreview.content_bytes || 0} bytes</p>{#if action.restorePreview.warnings?.length}<ul>{#each action.restorePreview.warnings as warning}<li>{warning}</li>{/each}</ul>{/if}<form method="POST" action="?/restore" onsubmit={(event) => confirmSubmit(event, t.adminConfigRestoreConfirm)}><input type="hidden" name="name" value={action.restorePreview.restored} /><input type="hidden" name="confirm" value="RESTORE_CONFIG_BACKUP" /><button class="button danger" type="submit">{t.adminConfigRestore}</button></form></section>{/if}
      {#if data.backups?.backups?.length}<div class="backup-list">{#each data.backups.backups as backup (backup.name)}<article class="backup-row"><div class="backup-info"><strong>{backup.name}</strong><span>{t.adminConfigBytes}: {formatBytes(backup.size)}</span><span>{t.adminConfigCreatedAt}: {dateLabel(backup.created_at)}</span></div><div class="backup-actions"><form method="POST" action="?/inspectBackup"><input type="hidden" name="name" value={backup.name} /><button class="button compact secondary" type="submit">{t.adminConfigInspect}</button></form><form method="POST" action="?/restorePreview"><input type="hidden" name="name" value={backup.name} /><button class="button compact secondary" type="submit">{t.adminConfigRestorePreview}</button></form><form method="POST" action="?/deleteBackup" onsubmit={(event) => confirmSubmit(event, t.adminConfigDeleteConfirm)}><input type="hidden" name="name" value={backup.name} /><button class="button compact danger" type="submit">{t.adminConfigDelete}</button></form></div></article>{/each}</div>{:else}<p class="empty">{t.adminConfigNoBackups}</p>{/if}
    </section>
  {/if}
</section>

<style>
  .config-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .form-footer, .toolbar, .upload-form, .backup-actions { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.15rem; }
  .heading-actions, .toolbar, .backup-actions { align-items: center; flex-wrap: wrap; } .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; } h3 { font-size: 1rem; } .muted { color: #52606d; margin: .4rem 0 0; } .back-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.5rem; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .65rem; }
  .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .tabs { border-bottom: 1px solid #c8d2da; display: flex; gap: .25rem; overflow-x: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .tabs button { background: transparent; border: 0; border-bottom: 3px solid transparent; color: #52606d; cursor: pointer; font: inherit; min-height: 2.75rem; padding: .55rem .85rem; white-space: nowrap; } .tabs button.active { border-bottom-color: #245b75; color: #16394a; font-weight: 700; }
  .workspace { align-items: start; display: grid; gap: 1rem; grid-template-columns: minmax(13rem, 17rem) minmax(0, 1fr); min-width: 0; } .section-nav, .editor-column, .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .section-nav { display: grid; gap: .65rem; position: sticky; top: 1rem; } .search-label { color: #243b53; font-size: .84rem; font-weight: 650; } input, select, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } textarea { line-height: 1.5; resize: vertical; } input:focus, select:focus, textarea:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .section-list { display: grid; gap: .35rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .section-list button { background: #f4f7f8; border: 1px solid transparent; border-radius: .3rem; color: #243b53; cursor: pointer; display: grid; gap: .2rem; min-width: 0; padding: .65rem; text-align: left; } .section-list button.current { background: #edf5f8; border-color: #9fb3c8; } .section-list strong, .section-list span { overflow-wrap: anywhere; } .section-list span { color: #52606d; font-size: .75rem; }
  .editor-column { display: grid; gap: 1rem; } .editor-heading { align-items: flex-start; border-bottom: 1px solid #e1e8ed; display: flex; gap: .75rem; justify-content: space-between; padding-bottom: .9rem; } .field-count, .path { color: #52606d; font-size: .78rem; overflow-wrap: anywhere; text-align: right; } .schema-form { display: grid; gap: .65rem; } .field-row { border: 1px solid transparent; border-radius: .35rem; display: grid; gap: .45rem; min-width: 0; padding: .75rem; } .field-row:hover { background: #f8fafb; } .field-row.changed { background: #fff8e6; border-color: #e9c46a; } .field-heading { align-items: start; display: flex; gap: .75rem; justify-content: space-between; } .field-heading > div { align-items: center; display: flex; flex-wrap: wrap; gap: .45rem; min-width: 0; } .field-heading label { color: #243b53; font-weight: 700; overflow-wrap: anywhere; } code { background: #eef2f4; color: #52606d; font-size: .72rem; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .3rem; } .field-description, .hints { color: #52606d; font-size: .8rem; line-height: 1.45; margin: 0; white-space: pre-wrap; } .hints { color: #7b8794; } .reset-link { background: transparent; border: 0; color: #245b75; cursor: pointer; font: inherit; font-size: .8rem; min-height: 2rem; padding: .25rem; white-space: nowrap; }
  .switch { align-items: center; color: #243b53; display: flex; gap: .55rem; min-height: 2.5rem; } .switch input { accent-color: #245b75; height: 1.15rem; min-height: 1.15rem; width: 1.15rem; } .secret-control { align-items: center; display: flex; gap: .5rem; min-width: 0; } .secret-control input { flex: 1; } .list-editor, .command-editor { display: grid; gap: .45rem; min-width: 0; } .list-row { align-items: center; display: flex; gap: .45rem; min-width: 0; } .list-row input { flex: 1; } .icon-button { background: #f4f6f8; border: 1px solid #c8d2da; border-radius: .3rem; color: #a63d40; cursor: pointer; font-size: 1.2rem; min-height: 2.5rem; min-width: 2.5rem; } .command-row { border: 1px solid #d7dee5; display: grid; gap: .45rem; grid-template-columns: minmax(8rem, 15rem) minmax(0, 1fr) auto; padding: .55rem; } .form-footer { align-items: center; border-top: 1px solid #e1e8ed; justify-content: space-between; padding-top: .85rem; } .form-footer .muted { flex: 1; }
  .toml-form { display: grid; gap: .75rem; } .toml-form textarea { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; min-height: 60dvh; } .upload-panel, .preview-panel, .restore-panel { border: 1px solid #d7dee5; display: grid; gap: .65rem; margin-top: 1rem; padding: .8rem; } .upload-panel { align-items: center; grid-template-columns: minmax(0, 1fr) minmax(18rem, 1fr); } .upload-form { align-items: center; justify-content: end; } .preview-panel pre { background: #f4f7f8; border: 1px solid #e1e8ed; font: .78rem/1.5 ui-monospace, SFMono-Regular, Consolas, monospace; max-height: 55dvh; overflow: auto; overscroll-behavior: contain; padding: .75rem; white-space: pre-wrap; word-break: break-word; } .restore-panel { background: #fff8e6; border-color: #e9c46a; } .restore-panel ul { margin: 0; padding-left: 1.2rem; } .backup-list { display: grid; gap: .55rem; margin-top: 1rem; max-height: 65dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .backup-row { align-items: center; border: 1px solid #d7dee5; display: flex; gap: .8rem; justify-content: space-between; min-width: 0; padding: .75rem; } .backup-info { display: grid; gap: .25rem; min-width: 0; } .backup-info span { color: #52606d; font-size: .78rem; } .backup-info strong { overflow-wrap: anywhere; }
  .empty { color: #52606d; padding: 2rem 1rem; text-align: center; }
  @media (max-width: 800px) { .workspace { grid-template-columns: 1fr; } .section-nav { position: static; } .section-list { display: flex; max-height: none; overflow-x: auto; } .section-list button { flex: 0 0 10rem; } .upload-panel { grid-template-columns: 1fr; } }
  @media (max-width: 560px) { .page-heading, .heading-actions, .panel-heading, .form-footer, .upload-form, .backup-row { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .form-footer .button, .upload-form .button { width: 100%; } .workspace .section-nav, .editor-column, .panel { padding: .85rem; } .field-heading { align-items: stretch; flex-direction: column; } .secret-control, .backup-actions { align-items: stretch; flex-direction: column; } .secret-control .button, .backup-actions form, .backup-actions .button { width: 100%; } .command-row { grid-template-columns: 1fr; } .icon-button { width: 100%; } }
</style>
