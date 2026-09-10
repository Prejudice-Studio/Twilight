<script lang="ts">
  import type { ActionData, PageData } from "./$types";
  import { t } from "$lib/i18n";

  let { data, form }: { data: PageData; form: ActionData } = $props();

  function copyPassword(value: string) {
    void navigator.clipboard?.writeText(value);
  }
</script>

<svelte:head><title>{t.forgotPassword} - {t.siteName}</title></svelte:head>

<section class="auth-panel forgot-panel" aria-labelledby="forgot-title">
  <header>
    <p class="eyebrow">{t.siteName}</p>
    <h1 id="forgot-title">{t.forgotPassword}</h1>
    <p class="muted">{t.forgotPasswordDescription}</p>
  </header>

  {#if !data.forgotPasswordEnabled || (!data.embyAvailable && !data.emailAvailable)}
    <p class="notice warning" role="status">{t.forgotPasswordUnavailable}</p>
  {:else}
    {#if form?.error}<p class="notice error" role="alert">{form.error}</p>{/if}
    {#if form?.notice}<p class="notice success" role="status">{form.notice}</p>{/if}

    {#if data.embyAvailable}
      <section class="method" aria-labelledby="emby-method-title">
        <h2 id="emby-method-title">{t.forgotPasswordEmbyTab}</h2>
        <p class="muted">使用 Emby 账号密码验证身份并重置 Web 密码。</p>
        <form method="POST" action="?/emby" class="form-grid">
          <label for="emby-username">{t.forgotPasswordEmbyUsername}</label>
          <input id="emby-username" name="emby_username" value={form?.emby_username || ""} autocomplete="username" required />
          <label for="emby-password">{t.forgotPasswordEmbyPassword}</label>
          <input id="emby-password" name="emby_password" type="password" autocomplete="current-password" required />
          <button class="button primary" type="submit">{t.forgotPasswordEmbySubmit}</button>
        </form>
        {#if form?.mode === "emby-result" && form.emby_result}
          <div class="result" role="status">
            <strong>{t.forgotPasswordTemporaryPassword}</strong>
            <p>{t.forgotPasswordTemporaryPasswordHelp}</p>
            <div class="secret-row">
              <code>{form.emby_result.new_password}</code>
              <button class="button secondary" type="button" onclick={() => copyPassword(form.emby_result?.new_password || "")} aria-label={t.forgotPasswordCopy}>{t.forgotPasswordCopy}</button>
            </div>
          </div>
        {/if}
      </section>
    {/if}

    {#if data.emailAvailable}
      <section class="method" aria-labelledby="email-method-title">
        <h2 id="email-method-title">{t.forgotPasswordEmailTab}</h2>
        {#if form?.mode === "email-done"}
          <p class="notice success">{t.forgotPasswordResetSuccess}</p>
        {:else if form?.mode === "email-reset"}
          <p class="muted">{t.forgotPasswordCodeSent.replace("{email}", form.email || "")}</p>
          <form method="POST" action="?/resetEmail" class="form-grid">
            <input type="hidden" name="email" value={form.email || ""} />
            <label for="email-code">{t.forgotPasswordCode}</label>
            <input id="email-code" name="code" placeholder={t.forgotPasswordCodePlaceholder} inputmode="numeric" autocomplete="one-time-code" required />
            <label for="new-password">{t.forgotPasswordNewPassword}</label>
            <input id="new-password" name="new_password" type="password" placeholder={t.forgotPasswordNewPasswordPlaceholder} autocomplete="new-password" minlength="12" required />
            <label for="confirm-password">{t.confirmPassword}</label>
            <input id="confirm-password" name="confirm_password" type="password" autocomplete="new-password" minlength="12" required />
            <div class="button-row">
              <button class="button primary" type="submit">{t.forgotPasswordReset}</button>
              <span class="muted">{t.forgotPasswordResendIn.replace("{seconds}", String(form.resend_after || 60))}</span>
            </div>
          </form>
        {:else}
          <p class="muted">{t.forgotPasswordEmailHelp}</p>
          <form method="POST" action="?/requestEmail" class="form-grid">
            <label for="email">{t.forgotPasswordEmail}</label>
            <input id="email" name="email" type="email" placeholder={t.forgotPasswordEmailPlaceholder} autocomplete="email" required />
            <button class="button primary" type="submit">{t.forgotPasswordSendCode}</button>
          </form>
        {/if}
      </section>
    {/if}
  {/if}

  <a class="back-link" href="/login">{t.forgotPasswordBackLogin}</a>
</section>

<style>
  .auth-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .5rem; margin: clamp(1.25rem, 7vh, 4rem) auto 0; max-width: 38rem; padding: clamp(1.15rem, 5vw, 2rem); }
  .forgot-panel { display: grid; gap: 1.15rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); } h2 { font-size: 1.05rem; }
  .muted { color: #52606d; margin: .45rem 0 0; }
  .method { border-top: 1px solid #e1e8ed; display: grid; gap: .75rem; padding-top: 1rem; }
  .form-grid { display: grid; gap: .55rem; min-width: 0; } label { color: #243b53; font-size: .9rem; font-weight: 650; }
  input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.6rem; min-width: 0; padding: .5rem .65rem; max-width: 100%; }
  input:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .button { align-items: center; border: 0; border-radius: .3rem; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.6rem; max-width: 100%; padding: .5rem .9rem; text-decoration: none; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary { background: #e8eef2; color: #16394a; } .button.secondary:hover { background: #d6e1e7; }
  .notice, .result { border: 1px solid; border-radius: .35rem; display: grid; gap: .3rem; padding: .7rem .8rem; } .result p { margin: 0; }
  .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .result { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .secret-row, .button-row { align-items: center; display: flex; flex-wrap: wrap; gap: .6rem; min-width: 0; } code { background: #fff; border: 1px solid #d9c98c; color: #243b53; flex: 1 1 12rem; min-width: 0; overflow-wrap: anywhere; padding: .55rem .65rem; }
  .back-link { color: #245b75; text-align: center; }
  @media (max-width: 560px) { .auth-panel { border-left: 0; border-right: 0; border-radius: 0; margin-top: 1rem; } .button, .button-row .muted { width: 100%; } }
</style>
