<script lang="ts">
  import { enhance } from "$app/forms";
  import { t } from "$lib/i18n";
  import type { DeveloperJSDocEntry, DeveloperJSPreviewResult, DeveloperJSPreset } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string; preview?: DeveloperJSPreviewResult };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let selectedID = $state(0);
  let name = $state("");
  let description = $state("");
  let code = $state("");
  let initialized = $state(false);
  let command = $state("/preview");
  let args = $state("");
  let privateChat = $state(true);
  let docsTab = $state<"bindings" | "functions" | "namespaces" | "native_objects" | "examples">("bindings");

  $effect(() => {
    if (initialized) return;
    const preset = data.presets[0];
    if (preset) {
      selectedID = preset.id;
      name = preset.name;
      description = preset.description || "";
      code = preset.code || "";
    }
    initialized = true;
  });

  const docsTabs: Array<{ id: typeof docsTab; label: string }> = [
    { id: "bindings", label: t.adminDeveloperBindings },
    { id: "functions", label: t.adminDeveloperFunctions },
    { id: "namespaces", label: t.adminDeveloperNamespaces },
    { id: "native_objects", label: t.adminDeveloperNativeObjects },
    { id: "examples", label: t.adminDeveloperExamples }
  ];

  function selectPreset(preset: DeveloperJSPreset): void {
    selectedID = preset.id;
    name = preset.name;
    description = preset.description || "";
    code = preset.code || "";
  }

  function newPreset(): void {
    selectedID = 0;
    name = "";
    description = "";
    code = "";
  }

  function entryList(): DeveloperJSDocEntry[] {
    if (!data.docs || docsTab === "examples") return [];
    return data.docs[docsTab] as DeveloperJSDocEntry[];
  }

  function insertExample(value: string): void { code = value; }
  function dateLabel(value: number): string { return value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : "-"; }
</script>

<svelte:head><title>{t.adminDeveloperTitle} - {t.siteName}</title></svelte:head>

<section class="developer-page" aria-labelledby="developer-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="developer-title">{t.adminDeveloperTitle}</h1><p class="muted">{t.adminDeveloperDescription}</p></div>
    <div class="heading-actions"><a class="button secondary" href="/admin">{t.adminDeveloperBack}</a><a class="button secondary" href="/admin/telegram">{t.adminTelegramTitle}</a></div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "saved"}<p class="notice success" role="status">{t.adminDeveloperSaved}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.adminDeveloperDeleted}</p>{/if}

  {#if !data.developerModeEnabled}
    <section class="disabled-panel" aria-labelledby="developer-disabled-title">
      <h2 id="developer-disabled-title">{t.adminDeveloperDisabledTitle}</h2><p>{t.adminDeveloperDisabledDescription}</p><a class="button primary" href="/dashboard">{t.adminDeveloperOpenDashboard}</a>
    </section>
  {:else}
    <section class="notice warning" aria-labelledby="developer-boundary-title"><h2 id="developer-boundary-title">{t.adminDeveloperBoundaryTitle}</h2><p>{t.adminDeveloperBoundaryDescription}</p></section>

    <div class="workspace-grid">
      <aside class="panel preset-panel" aria-labelledby="preset-title">
        <header class="panel-heading"><div><h2 id="preset-title">{t.adminDeveloperPresetsTitle}</h2><p class="muted">{t.adminDeveloperPresetsDescription}</p></div><button class="button secondary" type="button" onclick={newPreset}>{t.adminDeveloperNewPreset}</button></header>
        <div class="preset-list">
          {#if data.presets.length}
            {#each data.presets as preset (preset.id)}
              <button class:selected={selectedID === preset.id} class="preset" type="button" onclick={() => selectPreset(preset)}>
                <strong>{preset.name}</strong><span>{preset.description || t.adminDeveloperNone}</span><small>{dateLabel(preset.updated_at)}</small>
              </button>
            {/each}
          {:else}<p class="empty">{t.adminDeveloperNoPresets}</p>{/if}
        </div>
      </aside>

      <section class="panel editor-panel" aria-labelledby="editor-title">
        <header class="panel-heading"><div><h2 id="editor-title">{t.adminDeveloperEditorTitle}</h2><p class="muted">{selectedID ? t.adminDeveloperUpdatePreset : t.adminDeveloperSavePreset}</p></div></header>
        <form method="POST" action="?/save" use:enhance class="editor-form">
          <input type="hidden" name="preset_id" value={selectedID} />
          <label>{t.adminDeveloperName}<input name="name" maxlength="80" required bind:value={name} placeholder={t.adminDeveloperNamePlaceholder} /></label>
          <label>{t.adminDeveloperDescriptionLabel}<input name="description" maxlength="500" bind:value={description} placeholder={t.adminDeveloperDescriptionPlaceholder} /></label>
          <label>{t.adminDeveloperCode}<textarea name="code" maxlength="8000" rows="16" required bind:value={code} spellcheck="false" placeholder={t.adminDeveloperCodePlaceholder}></textarea></label>
          <div class="editor-actions"><button class="button primary" type="submit">{selectedID ? t.adminDeveloperUpdatePreset : t.adminDeveloperSavePreset}</button>{#if selectedID}<button class="button danger" formaction="?/delete" type="submit" onclick={(event) => { if (!window.confirm(t.adminDeveloperDeleteConfirm)) event.preventDefault(); }}>{t.adminDeveloperDeletePreset}</button>{/if}</div>
        </form>
      </section>

      <section class="panel preview-panel" aria-labelledby="preview-title">
        <header class="panel-heading"><div><h2 id="preview-title">{t.adminDeveloperPreview}</h2><p class="muted">{t.adminDeveloperBoundaryDescription}</p></div></header>
        <form method="POST" action="?/preview" use:enhance class="preview-form">
          <textarea class="hidden-code" name="code" aria-label={t.adminDeveloperCode} bind:value={code}></textarea>
          <label>{t.adminDeveloperCommand}<input name="command" maxlength="80" bind:value={command} placeholder={t.adminDeveloperCommandPlaceholder} /></label>
          <label>{t.adminDeveloperArgs}<input name="args_text" maxlength="2400" bind:value={args} placeholder={t.adminDeveloperArgsPlaceholder} /></label>
          <label class="check-field"><input type="hidden" name="private_chat" value="false" /><input type="checkbox" name="private_chat" value="true" bind:checked={privateChat} /><span>{t.adminDeveloperPrivateChat}</span></label>
          <button class="button primary" type="submit">{t.adminDeveloperPreview}</button>
        </form>
        {#if action.preview}
          <section class="result" aria-labelledby="preview-result-title"><h3 id="preview-result-title">{t.adminDeveloperPreviewResult}: {action.preview.ok ? t.adminDeveloperPreviewPassed : t.adminDeveloperPreviewFailed}</h3>{#if action.preview.output}<div><h4>{t.adminDeveloperPreviewOutput}</h4><pre>{action.preview.output}</pre></div>{/if}{#if action.preview.logs?.length}<div><h4>{t.adminDeveloperPreviewLogs}</h4><pre>{action.preview.logs.join("\n")}</pre></div>{/if}<dl class="facts">{#if action.preview.duration_ms !== undefined}<div><dt>{t.adminDeveloperDuration}</dt><dd>{action.preview.duration_ms} ms</dd></div>{/if}{#if action.preview.preview_context}<div><dt>{t.adminDeveloperPreviewContext}</dt><dd>{action.preview.preview_context.command} {action.preview.preview_context.args.join(" ")}</dd></div>{/if}</dl>{#if action.preview.errors?.length}<ul class="errors">{#each action.preview.errors as error}<li>{error}</li>{/each}</ul>{/if}</section>
        {/if}
      </section>
    </div>

    <section class="panel docs-panel" aria-labelledby="docs-title">
      <header class="panel-heading"><div><h2 id="docs-title">{t.adminDeveloperDocsTitle}</h2><p class="muted">{t.adminDeveloperDocsDescription}</p></div></header>
      {#if data.docs}
        <div class="docs-tabs" role="tablist">{#each docsTabs as tab}<button class:selected={docsTab === tab.id} type="button" role="tab" aria-selected={docsTab === tab.id} onclick={() => docsTab = tab.id}>{tab.label}</button>{/each}</div>
        <div class="docs-content">
          {#if docsTab === "examples"}
            {#each data.docs.examples as example}<article class="doc-entry"><h3>{example.title}</h3><p>{example.description}</p><pre>{example.code}</pre><button class="button secondary" type="button" onclick={() => insertExample(example.code)}>{t.adminDeveloperInsert}</button></article>{/each}
          {:else if entryList().length}
            {#each entryList() as entry (entry.name)}<details class="doc-entry"><summary><code>{entry.name}</code>{#if entry.type}<span>{entry.type}</span>{/if}</summary><p>{entry.description}</p>{#if entry.returns}<p><strong>{t.adminDeveloperReturns}</strong>{entry.returns}</p>{/if}{#if entry.fields?.length}<p><strong>{t.adminDeveloperFields}</strong>{entry.fields.join("、")}</p>{/if}{#if entry.example}<pre>{entry.example}</pre>{/if}</details>{/each}
          {:else}<p class="empty">{t.adminDeveloperDocsUnavailable}</p>{/if}
        </div>
        <div class="allowlist"><div><strong>{t.adminDeveloperConfigKeys}</strong><code>{data.docs.config_keys.join("、") || t.adminDeveloperNone}</code></div><div><strong>{t.adminDeveloperEnvKeys}</strong><code>{data.docs.env_keys.join("、") || t.adminDeveloperNone}</code></div></div>
      {:else}<p class="empty">{t.adminDeveloperDocsUnavailable}</p>{/if}
    </section>
  {/if}
</section>

<style>
  .developer-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .editor-actions { align-items: flex-start; display: flex; gap: .8rem; } .page-heading, .panel-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; } .heading-actions, .editor-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; } h1, h2, h3, h4, p, strong, span, small { overflow-wrap: anywhere; } h1, h2, h3, h4, p { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.1rem; } h3 { font-size: 1rem; } h4 { font-size: .85rem; } .muted { color: #52606d; line-height: 1.5; margin-top: .4rem; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.45rem; max-width: 100%; padding: .5rem .8rem; text-align: center; text-decoration: none; white-space: normal; } .button:hover { background: #d6e1e7; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.danger { background: #a63d40; color: #fff; } .button.danger:hover { background: #843336; }
  .notice, .disabled-panel, .panel { border: 1px solid #d7dee5; border-radius: .45rem; padding: .9rem 1rem; } .notice { display: grid; gap: .35rem; } .notice p { color: #486581; line-height: 1.5; } .notice.warning { background: #fff8e6; border-color: #ead398; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a9d5b4; color: #276749; } .disabled-panel { display: grid; gap: .65rem; justify-items: start; background: #f4f8fa; } .disabled-panel p { color: #52606d; line-height: 1.5; }
  .workspace-grid { display: grid; gap: 1rem; grid-template-columns: minmax(13rem, 18rem) minmax(0, 1.25fr) minmax(16rem, .9fr); align-items: start; } .panel { background: #fff; min-width: 0; } .panel-heading { margin-bottom: .8rem; } .preset-panel, .editor-panel, .preview-panel, .docs-panel { min-width: 0; } .preset-panel { display: grid; min-height: 24rem; } .preset-list { display: grid; gap: .45rem; max-height: 60dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .preset { background: #f8fafb; border: 1px solid #d7dee5; border-radius: .3rem; color: inherit; cursor: pointer; display: grid; gap: .25rem; min-width: 0; padding: .65rem; text-align: left; } .preset:hover, .preset.selected { background: #edf4f8; border-color: #7c9caf; } .preset span, .preset small { color: #52606d; font-size: .78rem; } .preset small { font-size: .7rem; }
  .editor-form, .preview-form { display: grid; gap: .7rem; } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .3rem; min-width: 0; } input, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.45rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } textarea { resize: vertical; } textarea[name="code"], .result pre, .doc-entry pre { font: .78rem/1.5 ui-monospace, SFMono-Regular, Consolas, monospace; } textarea[name="code"] { min-height: 22rem; } input:focus, textarea:focus, button:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; } .check-field { align-items: center; display: flex; gap: .5rem; } .check-field input { accent-color: #245b75; flex: 0 0 auto; height: 1.1rem; min-height: 1.1rem; width: 1.1rem; }
  .hidden-code { display: none; } .result { border-top: 1px solid #e1e8ed; display: grid; gap: .55rem; margin-top: .9rem; padding-top: .8rem; } .result pre, .doc-entry pre { background: #f4f7f8; border: 1px solid #d7dee5; max-height: 18rem; overflow: auto; overscroll-behavior: contain; padding: .65rem; white-space: pre-wrap; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .facts { display: grid; gap: .35rem; margin: 0; } .facts div { display: flex; gap: .5rem; justify-content: space-between; } .facts dt { color: #52606d; } .facts dd { margin: 0; overflow-wrap: anywhere; text-align: right; } .errors { color: #a61b1b; margin: 0; padding-left: 1.2rem; }
  .docs-panel { display: grid; gap: .8rem; } .docs-tabs { display: flex; gap: .35rem; overflow-x: auto; overscroll-behavior: contain; padding-bottom: .2rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .docs-tabs button { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .3rem; color: #243b53; cursor: pointer; flex: 0 0 auto; min-height: 2.35rem; padding: .45rem .65rem; } .docs-tabs button.selected { background: #edf4f8; border-color: #7c9caf; color: #16394a; font-weight: 700; } .docs-content { display: grid; gap: .45rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .doc-entry { border: 1px solid #d7dee5; border-radius: .3rem; display: grid; gap: .45rem; padding: .65rem; } .doc-entry p { color: #52606d; font-size: .82rem; line-height: 1.45; } .doc-entry summary { align-items: center; cursor: pointer; display: flex; flex-wrap: wrap; gap: .5rem; } .doc-entry summary span { color: #52606d; font-size: .75rem; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .3rem; } .allowlist { border-top: 1px solid #e1e8ed; display: grid; gap: .45rem; padding-top: .7rem; } .allowlist div { display: grid; gap: .3rem; } .empty { color: #52606d; padding: 1rem; text-align: center; }
  @media (max-width: 1100px) { .workspace-grid { grid-template-columns: minmax(12rem, 16rem) minmax(0, 1fr); } .preview-panel { grid-column: 1 / -1; } }
  @media (max-width: 700px) { .page-heading, .heading-actions, .panel-heading { align-items: stretch; flex-direction: column; } .heading-actions .button { width: 100%; } .workspace-grid { grid-template-columns: 1fr; } .preview-panel { grid-column: auto; } .preset-list, .docs-content { max-height: none; } .editor-actions .button { flex: 1 1 10rem; } h1 { font-size: 1.65rem; } .panel, .notice, .disabled-panel { padding: .8rem; } }
</style>
