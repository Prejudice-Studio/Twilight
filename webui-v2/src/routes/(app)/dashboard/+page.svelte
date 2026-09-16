<script lang="ts">
  import PageHeader from "$lib/components/PageHeader.svelte";
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();
</script>

<svelte:head><title>{t.dashboard} - {t.siteName}</title></svelte:head>

<section class="dashboard" aria-labelledby="dashboard-title">
  <PageHeader id="dashboard-title" eyebrow={t.siteName} title={t.dashboard}>
    {#snippet actions()}
      <p class="version">API {data.capabilities?.api_version || "v1"}</p>
    {/snippet}
  </PageHeader>

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
  .version { color: var(--tw-text-muted); font-family: ui-monospace, monospace; margin: 0; overflow-wrap: anywhere; }
  .metrics { display: grid; gap: 1rem; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .metric { background: var(--tw-surface); border: 1px solid var(--tw-border); border-radius: .45rem; display: grid; gap: .6rem; min-width: 0; padding: 1.25rem; }
  .metric span { color: var(--tw-text-muted); font-size: .9rem; }
  .metric strong { font-size: clamp(1.15rem, 5vw, 1.8rem); overflow-wrap: anywhere; }
  @media (max-width: 640px) { .metrics { grid-template-columns: 1fr; } .metric { min-height: 5.5rem; } }
</style>
