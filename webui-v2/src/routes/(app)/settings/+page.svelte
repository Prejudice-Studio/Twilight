<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  type FormState = {
    section?: string;
    purpose?: string;
    success?: boolean;
    message?: string;
    error?: string;
    verification?: { verification_id: string };
  };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let settings = $derived(data.settings);
  let action = $derived((form ?? {}) as FormState);

  function isSection(section: string): boolean {
    return action.section === section;
  }
</script>

<svelte:head><title>{t.settings} - {t.siteName}</title></svelte:head>

<section class="settings-page" aria-labelledby="settings-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.account}</p>
      <h1 id="settings-title">{t.settings}</h1>
      <p class="muted">{t.settingsIntro}</p>
    </div>
    <a class="back-link" href="/dashboard">{t.backDashboard}</a>
  </header>

  {#if data.loadError}
    <p class="notice error" role="alert">{data.loadError}</p>
  {:else if settings}
    <div class="settings-grid">
      <article class="panel profile-panel">
        <div class="panel-heading"><h2>{t.profile}</h2><span class="status-chip">{t.userID} {data.user.uid}</span></div>
        <dl class="facts">
          <div><dt>{t.username}</dt><dd>{data.user.username}</dd></div>
          <div><dt>{t.email}</dt><dd>{data.user.email || t.notSet}</dd></div>
          <div><dt>{t.emailStatus}</dt><dd>{data.user.email_verified ? t.emailVerified : t.emailUnverified}</dd></div>
          <div><dt>{t.telegram}</dt><dd>{settings.telegram.bound ? `${t.telegramBoundAs} @${settings.telegram.telegram_username || settings.telegram.telegram_id || ""}` : t.telegramNotBound}</dd></div>
          <div><dt>{t.emby}</dt><dd>{settings.emby_status.is_synced ? `${data.user.emby_username || data.user.emby_id}` : t.embyNotBound}</dd></div>
        </dl>
      </article>

      <article class="panel api-key-link-panel">
        <div class="panel-heading"><h2>{t.apiKeyTitle}</h2></div>
        <p class="muted">{t.apiKeyDescription}</p>
        <a class="button secondary" href="/settings/apikey">{t.settingsOpenApiKeys}</a>
      </article>

      <article class="panel">
        <div class="panel-heading"><h2>{t.notifications}</h2></div>
        <form method="POST" action="?/preferences" class="stack-form">
          <label class="switch-row"><span><strong>{t.loginTelegramNotice}</strong></span><input type="hidden" name="notify_on_login_telegram" value="false" /><input type="checkbox" name="notify_on_login_telegram" value="true" checked={settings.notify_on_login_telegram} /></label>
          <label class="switch-row"><span><strong>{t.loginEmailNotice}</strong></span><input type="hidden" name="notify_on_login_email" value="false" /><input type="checkbox" name="notify_on_login_email" value="true" checked={settings.notify_on_login_email} /></label>
          <label class="switch-row"><span><strong>{t.ticketTelegramNotice}</strong></span><input type="hidden" name="notify_on_ticket_telegram" value="false" /><input type="checkbox" name="notify_on_ticket_telegram" value="true" checked={settings.notify_on_ticket_telegram} /></label>
          <label class="switch-row"><span><strong>{t.autoRenewal}</strong><small>{t.autoRenewalHelp}</small></span><input type="hidden" name="signin_auto_renewal" value="false" /><input type="checkbox" name="signin_auto_renewal" value="true" checked={settings.signin_auto_renewal} /></label>
          {#if isSection("preferences") && action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
          {#if isSection("preferences") && action.success}<p class="notice success" role="status">{action.message || t.saved}</p>{/if}
          <button class="button primary" type="submit">{t.save}</button>
        </form>
      </article>

      <article class="panel">
        <div class="panel-heading"><h2>{t.email}</h2><span class:ok={data.user.email_verified} class="status-chip">{data.user.email_verified ? t.emailVerified : t.emailUnverified}</span></div>
        <p class="muted">{data.user.email || t.notSet}</p>
        <form method="POST" action="?/sendEmail" class="stack-form compact-form">
          <input type="hidden" name="purpose" value="bind" />
          <label for="bind-email">{t.emailAddress}</label>
          <input id="bind-email" name="email" type="email" autocomplete="email" value={data.user.email || ""} required />
          <small class="muted">{t.emailBindHelp}</small>
          <button class="button secondary" type="submit">{t.sendCode}</button>
        </form>
        {#if isSection("email") && action.purpose === "bind" && action.verification}
          <form method="POST" action="?/verifyEmail" class="stack-form compact-form verification-form">
            <input type="hidden" name="verification_id" value={action.verification.verification_id} />
            <label for="bind-code">{t.verificationCode}</label>
            <input id="bind-code" name="code" inputmode="numeric" autocomplete="one-time-code" maxlength="12" required />
            <button class="button primary" type="submit">{t.verify}</button>
          </form>
        {/if}
        {#if isSection("email") && action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
        {#if isSection("email") && action.success}<p class="notice success" role="status">{action.message || t.verificationSent}</p>{/if}
      </article>

      <article class="panel security-panel">
        <div class="panel-heading"><h2>{t.security}</h2></div>
        <form method="POST" action="?/preferences" class="stack-form security-options">
          {#if !settings.password_change_email_forced}<label class="switch-row"><span><strong>{t.passwordSecurityEmail}</strong><small>{t.passwordSecurityEmailHelp}</small></span><input type="hidden" name="password_change_email_required" value="false" /><input type="checkbox" name="password_change_email_required" value="true" checked={settings.password_change_email_required} /></label>{:else}<p class="muted small">{t.passwordSecurityEmail}: {t.emailSecurityForced}</p>{/if}
          {#if !settings.emby_password_email_forced}<label class="switch-row"><span><strong>{t.embyPasswordSecurityEmail}</strong><small>{t.embyPasswordSecurityEmailHelp}</small></span><input type="hidden" name="emby_password_email_required" value="false" /><input type="checkbox" name="emby_password_email_required" value="true" checked={settings.emby_password_email_required} /></label>{:else}<p class="muted small">{t.embyPasswordSecurityEmail}: {t.emailSecurityForced}</p>{/if}
          <label class="switch-row"><span><strong>{t.embyPasswordSecurityWeb}</strong><small>{t.embyPasswordSecurityWebHelp}</small></span><input type="hidden" name="emby_password_old_password_required" value="false" /><input type="checkbox" name="emby_password_old_password_required" value="true" checked={settings.emby_password_old_password_required} /></label>
          {#if isSection("preferences") && action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
          <button class="button secondary" type="submit">{t.save}</button>
        </form>

        <div class="subsection">
          <h3>{t.systemPassword}</h3>
          {#if settings.password_change_email_required || settings.password_change_email_forced}<form method="POST" action="?/sendEmail" class="inline-form"><input type="hidden" name="purpose" value="change_password" /><button class="button secondary" type="submit">{t.sendCode}</button></form>{/if}
          <form method="POST" action="?/changePassword" class="stack-form">
            <label for="web-old-password">{t.currentPassword}</label><input id="web-old-password" name="old_password" type="password" autocomplete="current-password" required />
            <label for="web-new-password">{t.newPassword}</label><input id="web-new-password" name="new_password" type="password" autocomplete="new-password" minlength="12" required />
            <label for="web-confirm-password">{t.confirmPassword}</label><input id="web-confirm-password" name="confirm_password" type="password" autocomplete="new-password" minlength="12" required />
            {#if settings.password_change_email_required || settings.password_change_email_forced}
              {#if action.purpose === "change_password" && action.verification}<label for="web-verification-id">{t.verificationId}</label><input id="web-verification-id" name="verification_id" value={action.verification.verification_id} readonly autocomplete="off" />{/if}
              <label for="web-email-code">{t.verificationCode}</label><input id="web-email-code" name="email_code" inputmode="numeric" autocomplete="one-time-code" />
              <p class="muted small">{t.sendCodeFirst}</p>
            {/if}
            {#if isSection("password") && action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
            {#if isSection("password") && action.success}<p class="notice success" role="status">{action.message}</p>{/if}
            <button class="button primary" type="submit">{t.changePassword}</button>
          </form>
        </div>
      </article>

      <article class="panel emby-panel">
        <div class="panel-heading"><h2>{t.integrations}</h2><span class:ok={settings.emby_status.is_active} class="status-chip">{settings.emby_status.is_active ? t.embyActive : t.embyDisabled}</span></div>
        {#if settings.emby_status.is_synced}
          <p class="muted">{t.boundAccount}: {data.user.emby_username || data.user.emby_id}</p>
          <div class="subsection">
            <h3>{t.changeEmbyPassword}</h3>
            {#if settings.emby_password_email_required || settings.emby_password_email_forced}<form method="POST" action="?/sendEmail" class="inline-form"><input type="hidden" name="purpose" value="change_emby_password" /><button class="button secondary" type="submit">{t.sendCode}</button></form>{/if}
            <form method="POST" action="?/changeEmbyPassword" class="stack-form">
              {#if settings.emby_password_old_password_required}<label for="emby-old-password">{t.oldPasswordOptional}</label><input id="emby-old-password" name="old_password" type="password" autocomplete="current-password" required />{/if}
              <label for="emby-new-password">{t.newPassword}</label><input id="emby-new-password" name="new_password" type="password" autocomplete="new-password" minlength="12" required />
              <label for="emby-confirm-password">{t.confirmPassword}</label><input id="emby-confirm-password" name="confirm_password" type="password" autocomplete="new-password" minlength="12" required />
              {#if settings.emby_password_email_required || settings.emby_password_email_forced}{#if action.purpose === "change_emby_password" && action.verification}<label for="emby-verification-id">{t.verificationId}</label><input id="emby-verification-id" name="verification_id" value={action.verification.verification_id} readonly autocomplete="off" />{/if}<label for="emby-email-code">{t.verificationCode}</label><input id="emby-email-code" name="email_code" inputmode="numeric" autocomplete="one-time-code" />{/if}
              {#if isSection("emby") && action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
              {#if isSection("emby") && action.success}<p class="notice success" role="status">{action.message}</p>{/if}
              <button class="button secondary" type="submit">{t.changeEmbyPassword}</button>
            </form>
          </div>
          {#if settings.emby_status.can_unbind}
            <form method="POST" action="?/unbindEmby" class="danger-action"><button class="button danger" type="submit">{t.unbindEmby}</button><small>{t.unbindEmbyHelp}</small></form>
          {/if}
        {:else}
          <div class="subsection two-columns">
            <form method="POST" action="?/bindEmby" class="stack-form">
              <h3>{t.bindEmby}</h3>
              <label for="bind-emby-username">{t.embyUsername}</label><input id="bind-emby-username" name="emby_username" autocomplete="username" required />
              <label for="bind-emby-password">{t.embyPasswordInput}</label><input id="bind-emby-password" name="emby_password" type="password" autocomplete="current-password" required />
              {#if isSection("emby") && action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
              <button class="button secondary" type="submit">{t.bindEmby}</button>
            </form>
            <form method="POST" action="?/registerEmby" class="stack-form">
              <h3>{t.registerEmby}</h3>
              <label for="register-emby-username">{t.embyUsername}</label><input id="register-emby-username" name="emby_username" autocomplete="username" required />
              <label for="register-emby-password">{t.embyPasswordInput}</label><input id="register-emby-password" name="emby_password" type="password" autocomplete="new-password" minlength="12" required />
              <button class="button primary" type="submit">{t.registerEmby}</button>
            </form>
          </div>
        {/if}
      </article>
    </div>
  {/if}
</section>

<style>
  .settings-page { display: grid; gap: 1.25rem; }
  .page-heading, .panel-heading { align-items: flex-start; display: flex; gap: 1rem; justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.25rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1, h2, h3 { margin: 0; }
  h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); }
  h2 { font-size: 1.15rem; }
  h3 { font-size: 1rem; }
  .muted { color: #52606d; margin: .6rem 0 0; }
  .small { font-size: .85rem; }
  .back-link { align-self: center; min-height: 2.5rem; padding: .55rem 0; }
  .settings-grid { align-items: start; display: grid; gap: 1rem; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1.15rem; }
  .profile-panel { grid-column: 1 / -1; }
  .status-chip { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #486581; font-size: .8rem; padding: .25rem .55rem; white-space: nowrap; }
  .status-chip.ok { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .facts { display: grid; gap: .75rem; grid-template-columns: repeat(5, minmax(0, 1fr)); margin: 1rem 0 0; }
  .facts div { min-width: 0; }
  dt { color: #52606d; font-size: .82rem; }
  dd { margin: .25rem 0 0; overflow-wrap: anywhere; }
  .stack-form { display: grid; gap: .55rem; margin-top: 1rem; }
  .inline-form { display: flex; flex-wrap: wrap; gap: .5rem; }
  .compact-form { max-width: 28rem; }
  label { color: #243b53; font-size: .9rem; font-weight: 650; }
  input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; }
  input:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .switch-row { align-items: center; display: flex; gap: 1rem; justify-content: space-between; min-height: 3.1rem; }
  .switch-row span { display: grid; gap: .2rem; min-width: 0; }
  .switch-row small { color: #52606d; font-size: .8rem; font-weight: 400; line-height: 1.4; }
  .switch-row input[type="checkbox"] { accent-color: #245b75; flex: 0 0 auto; height: 1.15rem; min-height: 1.15rem; width: 1.15rem; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font-weight: 650; padding: .5rem .9rem; width: fit-content; }
  .button.primary { background: #245b75; color: #fff; }
  .button.primary:hover { background: #1c465a; }
  .button.secondary { background: #e8eef2; color: #16394a; }
  .button.secondary:hover { background: #d6e1e7; }
  .button.danger { background: #a63d40; color: #fff; }
  .button.danger:hover { background: #893337; }
  .notice { border: 1px solid; border-radius: .3rem; margin: .7rem 0 0; padding: .65rem .75rem; }
  .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .subsection { border-top: 1px solid #e1e8ed; margin-top: 1rem; padding-top: 1rem; }
  .verification-form { border-left: 3px solid #7b9aaa; padding-left: .8rem; }
  .danger-action { align-items: center; border-top: 1px solid #e1e8ed; display: flex; flex-wrap: wrap; gap: .75rem; margin-top: 1rem; padding-top: 1rem; }
  .danger-action small { color: #7b341e; }
  .two-columns { display: grid; gap: 1rem; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  @media (max-width: 800px) { .settings-grid { grid-template-columns: 1fr; } .profile-panel { grid-column: auto; } .facts { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 560px) { .page-heading { flex-direction: column; } .back-link { align-self: flex-start; } .facts, .two-columns { grid-template-columns: 1fr; } .panel { padding: .9rem; } .panel-heading { flex-wrap: wrap; } .switch-row { align-items: flex-start; } .switch-row input[type="checkbox"] { margin-top: .2rem; } .button { width: 100%; } .danger-action .button { width: auto; } }
</style>
