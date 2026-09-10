<script lang="ts">
  import { page } from "$app/state";
  import { t } from "$lib/i18n";

  const statusText: Record<number, string> = {
    403: t.errorForbidden,
    404: t.errorNotFound,
    500: t.errorServer
  };

  let status = $derived(page.status || 500);
  let description = $derived(statusText[status] || t.errorGeneric);
</script>

<svelte:head><title>{status} - {t.siteName}</title></svelte:head>

<main class="error-page">
  <p class="code">{status}</p>
  <h1>{t.errorTitle}</h1>
  <p>{description}</p>
  <div class="actions">
    <a href="/">{t.errorBackHome}</a>
    <a href="/login">{t.errorBackLogin}</a>
  </div>
</main>

<style>
  .error-page { align-items: center; display: grid; gap: .8rem; justify-items: center; margin: 0 auto; max-width: 34rem; min-height: 70dvh; padding: 2rem 1rem; text-align: center; }
  .code { color: #486581; font: 700 1rem ui-monospace, monospace; margin: 0; }
  h1 { font-size: clamp(1.6rem, 7vw, 2.2rem); margin: 0; }
  p:not(.code) { color: #52606d; line-height: 1.6; margin: 0; max-width: 30rem; }
  .actions { display: flex; flex-wrap: wrap; gap: .75rem; justify-content: center; margin-top: .5rem; }
  .actions a { background: #e8eef2; border-radius: .3rem; color: #16394a; min-height: 2.5rem; padding: .6rem .85rem; text-decoration: none; }
  .actions a:first-child { background: #245b75; color: #fff; }
  @media (max-width: 420px) { .actions, .actions a { width: 100%; } }
</style>
