<script lang="ts">
  import { enhance } from "$app/forms";
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  type FormState = {
    username?: string;
    email?: string;
    reg_code?: string;
    telegram_bind_code?: string;
    error?: string;
    bind_code?: string;
    bind_expires_in?: number;
    bind_message?: string;
  };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let bindCode = $state("");
  $effect(() => {
    const next = action.bind_code || action.telegram_bind_code;
    if (next) bindCode = next;
  });

  const registrationOpen = $derived(data.availability?.can_register ?? data.availability?.available ?? false);
  const requiresRegCode = $derived(Boolean(data.availability?.requires_reg_code && (data.availability?.current_users ?? 0) > 0));
  const hasTelegram = $derived(Boolean(data.system?.features?.telegram || data.system?.features?.force_bind_telegram));
  const forceTelegram = $derived(Boolean(data.system?.features?.force_bind_telegram));

  function createBindCodeResult() {
    return async ({ update, result }: { update: (options?: { reset?: boolean }) => Promise<void>; result: { type: string; data?: Record<string, unknown> } }) => {
      if (result.type === "success" && result.data?.bind_code) {
        bindCode = String(result.data.bind_code);
      }
      await update({ reset: false });
    };
  }
</script>

<svelte:head><title>{t.register} - {t.siteName}</title></svelte:head>

<section class="auth-panel register-panel" aria-labelledby="register-title">
  <header>
    <p class="eyebrow">{t.siteName}</p>
    <h1 id="register-title">{t.registerIntro}</h1>
    <p class="muted">{t.registerHint}</p>
  </header>

  {#if !registrationOpen}
    <div class="notice warning" role="status">
      <strong>{t.registerUnavailable}</strong>
      <span>{data.availability?.message || t.registerUnavailableHelp}</span>
    </div>
  {:else}
    {#if hasTelegram}
      <fieldset>
        <legend>{t.telegram}</legend>
        <p class="muted">{t.telegramBindCodeHelp}</p>
        {#if bindCode}<output class="bind-code">{bindCode}</output>{/if}
        <form method="POST" action="?/createBindCode" use:enhance={createBindCodeResult} class="bind-code-form">
          <button class="button secondary" type="submit">{t.getTelegramBindCode}</button>
        </form>
        {#if forceTelegram}<small class="required-note">{t.telegramRequired}</small>{/if}
      </fieldset>
    {/if}
    <form method="POST" class="register-form">
      <label for="username">{t.username}</label>
      <input id="username" name="username" value={action.username || ""} autocomplete="username" required />

      <label for="email">{t.registerEmailOptional}</label>
      <input id="email" name="email" type="email" value={action.email || ""} autocomplete="email" />

      <label for="password">{t.password}</label>
      <input id="password" name="password" type="password" autocomplete="new-password" minlength="12" required />
      <small class="muted">{t.registerPasswordHelp}</small>

      <label for="confirm-password">{t.registerConfirmPassword}</label>
      <input id="confirm-password" name="confirm_password" type="password" autocomplete="new-password" minlength="12" required />

      {#if requiresRegCode}
        <label for="reg-code">{t.registerCode}</label>
        <input id="reg-code" name="reg_code" value={action.reg_code || ""} autocomplete="off" required />
        <small class="muted">{t.registerCodeHelp}</small>
      {/if}

      {#if hasTelegram}
        <input type="hidden" name="telegram_bind_code" value={bindCode} />
      {/if}

      {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
      {#if action.bind_message}<p class="notice success" role="status">{action.bind_message}</p>{/if}
      <button class="button primary" type="submit">{t.registerSubmit}</button>
    </form>
  {/if}

  <a class="back-link" href="/login">{t.alreadyHaveAccount}</a>
</section>

<style>
  .auth-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .5rem; margin: clamp(1.5rem, 8vh, 5rem) auto 0; max-width: 34rem; padding: clamp(1.15rem, 5vw, 2rem); }
  .register-panel { display: grid; gap: 1.25rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1 { font-size: clamp(1.55rem, 7vw, 2.15rem); margin: 0; overflow-wrap: anywhere; }
  .muted { color: #52606d; margin: .5rem 0 0; }
  .register-form { display: grid; gap: .55rem; }
  label, legend { color: #243b53; font-size: .9rem; font-weight: 650; }
  input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.6rem; min-width: 0; padding: .5rem .65rem; }
  input:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  fieldset { border: 1px solid #d7dee5; border-radius: .35rem; display: grid; gap: .55rem; margin: .5rem 0; min-width: 0; padding: .8rem; }
  fieldset p { margin: 0; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.6rem; padding: .5rem .9rem; }
  .button.primary { background: #245b75; color: #fff; }
  .button.primary:hover { background: #1c465a; }
  .button.secondary { background: #e8eef2; color: #16394a; }
  .button.secondary:hover { background: #d6e1e7; }
  .bind-code-form { display: flex; flex-wrap: wrap; }
  .bind-code { background: #f4f6f8; border: 1px solid #c8d2da; border-radius: .3rem; font: 1.05rem ui-monospace, monospace; letter-spacing: .08em; overflow-wrap: anywhere; padding: .55rem .7rem; }
  .required-note { color: #7b341e; }
  .notice { border: 1px solid; border-radius: .3rem; display: grid; gap: .2rem; padding: .65rem .75rem; }
  .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .back-link { color: #245b75; text-align: center; }
  @media (max-width: 560px) { .auth-panel { border-left: 0; border-right: 0; border-radius: 0; margin-top: 1rem; } .button { width: 100%; } }
</style>
