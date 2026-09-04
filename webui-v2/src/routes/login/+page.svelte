<script lang="ts">
  import { t } from "$lib/i18n";
  import type { ActionData } from "./$types";

  let { data, form }: { data: import("./$types").PageData; form: ActionData } = $props();
</script>

<svelte:head><title>{t.login} - {t.siteName}</title></svelte:head>

<section class="auth-panel" aria-labelledby="login-title">
  <div>
    <p class="eyebrow">{t.siteName}</p>
    <h1 id="login-title">{t.login}</h1>
    <p class="muted">使用已有 Web 账号继续。</p>
  </div>
  {#if data.registered}<p class="success" role="status">{t.registerSuccess}</p>{/if}
  <form method="POST" class="login-form">
    <label for="username">{t.username}</label>
    <input id="username" name="username" value={form?.username || ""} autocomplete="username" required />
    <label for="password">{t.password}</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required />
    {#if form?.error}<p class="error" role="alert">{form.error}</p>{/if}
    <button type="submit">{t.submit}</button>
    <a class="register-link" href="/register">{t.register}</a>
  </form>
</section>

<style>
  .auth-panel { background: #fff; border: 1px solid #d7dee5; border-radius: 0.5rem; margin: clamp(2rem, 12vh, 8rem) auto 0; max-width: 28rem; padding: clamp(1.25rem, 5vw, 2rem); }
  .eyebrow { color: #486581; font-size: 0.8rem; font-weight: 700; letter-spacing: 0.08em; margin: 0 0 0.5rem; text-transform: uppercase; }
  h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); margin: 0; }
  .muted { color: #52606d; }
  .login-form { display: grid; gap: 0.65rem; margin-top: 1.5rem; }
  label { font-size: 0.9rem; font-weight: 650; }
  input { border: 1px solid #9fb3c8; border-radius: 0.35rem; font: inherit; min-height: 2.75rem; min-width: 0; padding: 0.5rem 0.7rem; }
  input:focus, button:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  button { background: #245b75; border: 0; border-radius: 0.35rem; color: #fff; cursor: pointer; font: inherit; font-weight: 650; margin-top: 0.6rem; min-height: 2.75rem; padding: 0.5rem 1rem; }
  button:hover { background: #1c465a; }
  .error { background: #fff1f0; border: 1px solid #f1a7a0; color: #a61b1b; margin: 0.35rem 0 0; padding: 0.65rem; }
  .success { background: #edf7f0; border: 1px solid #a9d5b4; color: #276749; margin: 0; padding: 0.65rem; }
  .register-link { color: #245b75; font-size: .9rem; text-align: center; }
</style>
