<script lang="ts">
  import type { ActionData, PageData } from "./$types";
  import { t } from "$lib/i18n";

  let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<svelte:head><title>{t.setupTitle} - {t.siteName}</title></svelte:head>

<section class="auth-panel setup-panel" aria-labelledby="setup-title">
  <header>
    <p class="eyebrow">{t.siteName}</p>
    <h1 id="setup-title">{t.setupTitle}</h1>
    <p class="muted">{t.setupDescription}</p>
  </header>

  {#if !data.status?.available}
    <p class="notice warning" role="status">{t.setupUnavailable}</p>
  {:else}
    <div class="notice warning"><strong>{t.setupSecurityTitle}</strong><span>{t.setupSecurityDescription}</span></div>
    {#if form?.error}<p class="notice error" role="alert">{form.error}</p>{/if}

    <form method="POST" class="setup-form">
      <fieldset>
        <legend>{t.setupAccountTitle}</legend>
        <label for="site-name">{t.setupSiteName}</label>
        <input id="site-name" name="site_name" value={data.system?.name || "Twilight"} maxlength="100" />
        <label for="username">{t.setupAdminUsername}</label>
        <input id="username" name="username" value={form?.username || ""} autocomplete="username" required />
        <label for="admin-email">{t.setupAdminEmail}</label>
        <input id="admin-email" name="email" type="email" value={form?.email || ""} autocomplete="email" />
        <div class="two-column">
          <div><label for="password">{t.setupPassword}</label><input id="password" name="password" type="password" autocomplete="new-password" minlength="12" required /></div>
          <div><label for="confirm-password">{t.setupConfirmPassword}</label><input id="confirm-password" name="confirm_password" type="password" autocomplete="new-password" minlength="12" required /></div>
        </div>
      </fieldset>

      <fieldset>
        <legend>{t.setupEmbyTitle}</legend>
        <label for="emby-url">{t.setupEmbyURL}</label>
        <input id="emby-url" name="emby_url" placeholder="http://emby:8096" inputmode="url" />
        <label for="emby-token">{t.setupEmbyToken}</label>
        <input id="emby-token" name="emby_token" type="password" autocomplete="off" />
        <label for="emby-lines">{t.setupEmbyLines}</label>
        <textarea id="emby-lines" name="emby_lines" rows="3" placeholder="LAN=http://192.168.1.10:8096"></textarea>
        <small>{t.setupEmbyLinesHelp}</small>
      </fieldset>

      <fieldset>
        <legend>{t.setupTelegramTitle}</legend>
        <label class="check"><input type="checkbox" name="telegram_enabled" value="true" />{t.telegram}</label>
        <label for="telegram-token">{t.setupTelegramToken}</label>
        <input id="telegram-token" name="telegram_token" type="password" autocomplete="off" />
        <label for="telegram-admins">{t.setupTelegramAdmins}</label>
        <textarea id="telegram-admins" name="telegram_admins" rows="2"></textarea>
        <small>{t.setupTelegramAdminsHelp}</small>
      </fieldset>

      <fieldset>
        <legend>{t.setupEmailTitle}</legend>
        <label class="check"><input type="checkbox" name="email_enabled" value="true" />{t.email}</label>
        <div class="two-column">
          <div><label for="smtp-host">{t.setupSMTPHost}</label><input id="smtp-host" name="smtp_host" /></div>
          <div><label for="smtp-port">{t.setupSMTPPort}</label><input id="smtp-port" name="smtp_port" type="number" min="1" max="65535" value="587" /></div>
          <div><label for="smtp-username">{t.setupSMTPUsername}</label><input id="smtp-username" name="smtp_username" autocomplete="username" /></div>
          <div><label for="smtp-password">{t.setupSMTPPassword}</label><input id="smtp-password" name="smtp_password" type="password" autocomplete="new-password" /></div>
        </div>
        <label for="smtp-from">{t.setupSMTPFrom}</label>
        <input id="smtp-from" name="smtp_from" type="email" />
      </fieldset>

      <fieldset>
        <legend>{t.setupPolicyTitle}</legend>
        <label class="check"><input type="checkbox" name="register_open" value="true" />{t.setupOpenRegistration}</label>
        <label class="check"><input type="checkbox" name="register_code_limit" value="true" checked />{t.setupRequireRegCode}</label>
        <label class="check"><input type="checkbox" name="allow_pending_register" value="true" />{t.setupAllowPending}</label>
      </fieldset>

      <button class="button primary" type="submit">{t.setupSubmit}</button>
    </form>
  {/if}

  <a class="back-link" href="/login">{t.setupBackLogin}</a>
</section>

<style>
  .auth-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .5rem; margin: clamp(1rem, 5vh, 3rem) auto 0; max-width: 48rem; padding: clamp(1.1rem, 4vw, 2rem); }
  .setup-panel { display: grid; gap: 1rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1, p, legend, label, small { overflow-wrap: anywhere; } h1 { font-size: clamp(1.6rem, 6vw, 2.25rem); margin: 0; } .muted { color: #52606d; margin: .45rem 0 0; }
  .notice { border: 1px solid; border-radius: .35rem; display: grid; gap: .25rem; padding: .7rem .8rem; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .setup-form { display: grid; gap: 1rem; min-width: 0; } fieldset { border: 1px solid #d7dee5; border-radius: .35rem; display: grid; gap: .55rem; min-width: 0; padding: .85rem; } legend { color: #243b53; font-size: 1rem; font-weight: 700; padding: 0 .3rem; } label { color: #243b53; font-size: .9rem; font-weight: 650; } small { color: #52606d; }
  input, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.6rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; resize: vertical; } textarea { min-height: 5rem; } input:focus, textarea:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .two-column { display: grid; gap: .75rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .two-column > div { display: grid; gap: .55rem; min-width: 0; } .check { align-items: center; display: flex; gap: .55rem; } .check input { min-height: 1rem; width: 1rem; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.7rem; padding: .55rem 1rem; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .back-link { color: #245b75; text-align: center; }
  @media (max-width: 620px) { .auth-panel { border-left: 0; border-right: 0; border-radius: 0; margin-top: 0; } .two-column { grid-template-columns: 1fr; } .button { width: 100%; } }
</style>

