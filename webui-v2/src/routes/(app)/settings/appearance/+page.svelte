<script lang="ts">
  import { t } from "$lib/i18n";
  import type { BackgroundConfig } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let state = $derived((form ?? {}) as { action?: string; error?: string });

  function inputValue(value: string | number | boolean): string {
    return String(value);
  }

  function backgroundStyle(value: string): string {
    return value ? `background-image: ${value};` : "background: #eef2f4;";
  }

  function resultMessage(): string {
    const result = data.result;
    if (result === "background_saved") return t.appearanceBackgroundSaved;
    if (result === "background_reset") return t.appearanceBackgroundReset;
    if (result === "background_uploaded") return t.appearanceBackgroundUploaded;
    if (result === "avatar_uploaded") return t.appearanceAvatarUploaded;
    if (result === "avatar_deleted") return t.appearanceAvatarDeleted;
    return "";
  }

  function preview(config: BackgroundConfig, theme: "light" | "dark"): string {
    const image = theme === "light" ? config.lightBgImage : config.darkBgImage;
    const gradient = theme === "light" ? config.lightBg : config.darkBg;
    return image || gradient;
  }

  function submitConfirm(message: string): (event: SubmitEvent) => void {
    return (event) => {
      if (!window.confirm(message)) event.preventDefault();
    };
  }
</script>

<svelte:head><title>{t.appearanceTitle} - {t.siteName}</title></svelte:head>

<section class="page" aria-labelledby="appearance-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.account}</p><h1 id="appearance-title">{t.appearanceTitle}</h1><p class="muted">{t.appearanceDescription}</p></div>
    <a class="button secondary" href="/settings">{t.appearanceBackSettings}</a>
  </header>

  {#if state.error}<p class="notice error" role="alert">{state.error}</p>{/if}
  {#if resultMessage()}<p class="notice success" role="status">{resultMessage()}</p>{/if}

  <section class="panel" aria-labelledby="background-title">
    <header class="panel-heading"><div><h2 id="background-title">{t.appearanceBackgroundTitle}</h2><p class="muted">{t.appearanceBackgroundDescription}</p></div></header>
    <div class="background-form">
      {#each [{ key: "light", label: t.appearanceLightTheme }, { key: "dark", label: t.appearanceDarkTheme }] as theme}
        {@const prefix = theme.key === "light" ? "light" : "dark"}
        <fieldset class="theme-panel">
          <legend>{theme.label}</legend>
          <div class="preview" style={backgroundStyle(preview(data.background, prefix))}><span>{t.appearancePreview}</span></div>
          <label>{t.appearanceGradient}<input form="background-save" name={`${prefix}Bg`} value={data.background[`${prefix}Bg` as "lightBg" | "darkBg"]} maxlength="2000" placeholder={t.appearanceGradientPlaceholder} /></label>
          <label>{t.appearanceImage}<input form="background-save" name={`${prefix}BgImage`} value={data.background[`${prefix}BgImage` as "lightBgImage" | "darkBgImage"]} maxlength="180" readonly placeholder={t.appearanceImageEmpty} /></label>
          <label class="check"><input form="background-save" type="checkbox" name={`${prefix}Flow`} value="true" checked={data.background[`${prefix}Flow` as "lightFlow" | "darkFlow"]} /><span><strong>{t.appearanceFlow}</strong><small>{t.appearanceFlowHelp}</small></span></label>
          <div class="range-grid">
            <label>{t.appearanceBlur}<input form="background-save" type="number" name={`${prefix}Blur`} min="0" max="30" value={inputValue(data.background[`${prefix}Blur` as "lightBlur" | "darkBlur"])} /></label>
            <label>{t.appearanceOpacity}<input form="background-save" type="number" name={`${prefix}Opacity`} min="10" max="100" value={inputValue(data.background[`${prefix}Opacity` as "lightOpacity" | "darkOpacity"])} /></label>
          </div>
          <div class="upload-row">
            <span class="muted">{t.appearanceBackgroundUploadHelp}</span>
            <form method="POST" action="?/uploadBackground" enctype="multipart/form-data">
              <input type="hidden" name="type" value={prefix} />
              <label class="file-button"><span>{t.appearanceBackgroundUpload}</span><input type="file" name="file" accept="image/jpeg,image/png,image/gif,image/webp,image/bmp" required /></label>
            </form>
          </div>
        </fieldset>
      {/each}
    </div>
    <form id="background-save" method="POST" action="?/saveBackground"></form>
    <div class="form-actions">
      <button class="button primary" form="background-save" type="submit">{t.appearanceSaveBackground}</button>
    </div>
    <form method="POST" action="?/resetBackground" onsubmit={submitConfirm(t.appearanceResetBackground)}><button class="button secondary" type="submit">{t.appearanceResetBackground}</button></form>
  </section>

  <section class="panel avatar-panel" aria-labelledby="avatar-title">
    <header class="panel-heading"><div><h2 id="avatar-title">{t.appearanceAvatarTitle}</h2><p class="muted">{t.appearanceAvatarDescription}</p></div></header>
    <div class="avatar-layout">
      <div class="avatar-preview">
        {#if data.avatar}<img src={data.avatar} alt={t.appearanceAvatarTitle} width="160" height="160" />{:else}<span>{t.appearanceAvatarEmpty}</span>{/if}
      </div>
      <div class="avatar-actions">
        <p class="muted">{t.appearanceAvatarUploadHelp}</p>
        <form method="POST" action="?/uploadAvatar" enctype="multipart/form-data" class="upload-form">
          <label class="file-button"><span>{t.appearanceAvatarUpload}</span><input type="file" name="file" accept="image/jpeg,image/png,image/gif,image/webp,image/bmp" required /></label>
        </form>
        {#if data.avatar}<form method="POST" action="?/deleteAvatar" onsubmit={submitConfirm(t.appearanceAvatarDelete)}><button class="button danger" type="submit">{t.appearanceAvatarDelete}</button></form>{/if}
      </div>
    </div>
  </section>
</section>

<style>
  .page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .panel-heading, .form-actions, .upload-row, .avatar-layout { align-items: flex-start; display: flex; gap: .85rem; justify-content: space-between; min-width: 0; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .4rem; text-transform: uppercase; }
  h1, h2, p { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; }
  .muted { color: #52606d; line-height: 1.5; margin-top: .35rem; overflow-wrap: anywhere; }
  .button { align-items: center; border: 0; border-radius: .3rem; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.5rem; padding: .5rem .8rem; text-decoration: none; white-space: normal; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary { background: #e8eef2; color: #16394a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; }
  .panel, .notice { border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .panel { background: #fff; display: grid; gap: .9rem; } .notice { margin: 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .background-form { display: grid; gap: .9rem; } .theme-panel { border: 1px solid #d7dee5; display: grid; gap: .7rem; min-width: 0; padding: .85rem; } legend { color: #243b53; font-weight: 700; padding: 0 .3rem; }
  label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; max-width: 100%; min-height: 2.45rem; min-width: 0; padding: .5rem .6rem; } input:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .preview { align-items: center; background-position: center; background-size: cover; border: 1px solid #c8d2da; color: #16394a; display: flex; justify-content: center; min-height: 8rem; overflow: hidden; padding: 1rem; } .preview span { background: rgb(255 255 255 / 80%); padding: .35rem .55rem; }
  .check { align-items: flex-start; display: flex; gap: .55rem; } .check input { flex: 0 0 auto; min-height: 1rem; margin-top: .2rem; width: 1rem; } .check span { display: grid; gap: .15rem; } small { color: #52606d; font-weight: 400; line-height: 1.4; overflow-wrap: anywhere; }
  .range-grid { display: grid; gap: .7rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .upload-row { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .7rem; } .upload-row form { flex: 0 0 auto; }
  .file-button { align-items: center; background: #e8eef2; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; min-height: 2.45rem; padding: .45rem .7rem; } .file-button input { border: 0; min-height: 0; padding: 0; width: 0; }
  .avatar-layout { align-items: center; justify-content: flex-start; } .avatar-preview { align-items: center; background: #eef2f4; border: 1px solid #c8d2da; border-radius: 50%; color: #52606d; display: flex; flex: 0 0 10rem; height: 10rem; justify-content: center; overflow: hidden; text-align: center; } .avatar-preview img { height: 100%; object-fit: cover; width: 100%; } .avatar-actions { display: grid; gap: .7rem; min-width: 0; } .upload-form { display: flex; flex-wrap: wrap; }
  @media (min-width: 760px) { .background-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 620px) { .page-heading, .panel-heading, .upload-row, .avatar-layout { flex-direction: column; } .page-heading > .button, .form-actions .button { width: 100%; } .range-grid { grid-template-columns: 1fr; } .upload-row form, .file-button { width: 100%; } .file-button { justify-content: center; } .avatar-layout { align-items: flex-start; } }
</style>
