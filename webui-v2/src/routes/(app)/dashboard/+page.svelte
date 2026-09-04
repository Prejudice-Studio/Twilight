<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();
</script>

<svelte:head><title>{t.dashboard} - {t.siteName}</title></svelte:head>

<section class="dashboard" aria-labelledby="dashboard-title">
  <div class="heading">
    <div>
      <p class="eyebrow">{t.siteName}</p>
      <h1 id="dashboard-title">{t.dashboard}</h1>
    </div>
    <p class="version">API {data.capabilities?.api_version || "v1"}</p>
  </div>

  <div class="metrics">
    <article class="metric">
      <span>{t.onlineViewers}</span>
      <strong>{data.viewers?.available ? data.viewers.count : "-"}</strong>
    </article>
    <article class="metric">
      <span>{t.account}</span>
      <strong>{data.user?.username || "-"}</strong>
    </article>
    <article class="metric">
      <span>{t.embyBound}</span>
      <strong>{data.user?.emby_id ? t.bound : t.unbound}</strong>
    </article>
  </div>
</section>

<style>
  .dashboard { display: grid; gap: 1.25rem; }
  .heading { align-items: flex-start; display: flex; gap: 1rem; justify-content: space-between; }
  .eyebrow { color: #486581; font-size: 0.8rem; font-weight: 700; letter-spacing: 0.08em; margin: 0 0 0.5rem; text-transform: uppercase; }
  h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); margin: 0; }
  .version { color: #52606d; font-family: ui-monospace, monospace; margin: 0; }
  .metrics { display: grid; gap: 1rem; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .metric { background: #fff; border: 1px solid #d7dee5; border-radius: 0.5rem; display: grid; gap: 0.6rem; min-width: 0; padding: 1.25rem; }
  .metric span { color: #52606d; font-size: 0.9rem; }
  .metric strong { font-size: clamp(1.15rem, 5vw, 1.8rem); overflow-wrap: anywhere; }
  @media (max-width: 640px) { .heading { flex-direction: column; } .metrics { grid-template-columns: 1fr; } .metric { min-height: 5.5rem; } }
</style>
