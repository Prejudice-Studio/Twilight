<script lang="ts">
  import PageHeader from "$lib/components/PageHeader.svelte";
  import Panel from "$lib/components/Panel.svelte";
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";

  let { data }: { data: PageData } = $props();

  function href(overrides: Record<string, string> = {}): string {
    const params = new URLSearchParams();
    const values = { q: data.query.search, method: data.query.method, auth: data.query.auth, version: data.query.version, ...overrides };
    for (const [key, value] of Object.entries(values)) if (value) params.set(key, value);
    const query = params.toString();
    return query ? `/api-docs?${query}` : "/api-docs";
  }

  function authClass(auth: string): string { return `auth-${auth.toLowerCase().replace(/[^a-z]+/g, "-")}`; }
</script>

<svelte:head>
  <title>{t.apiDocsTitle} - {t.siteName}</title>
  <meta name="description" content={t.apiDocsDescription} />
</svelte:head>

<section class="docs-page" aria-labelledby="api-docs-title">
  <PageHeader id="api-docs-title" eyebrow={t.apiDocsEyebrow} title={t.apiDocsTitle} description={t.apiDocsDescription}>
    {#snippet actions()}
      <a class="button secondary" href="/wiki">{t.apiDocsBackWiki}</a>
      <a class="button secondary" href="/api/v2/openapi.json">{t.apiDocsRawSpec}</a>
    {/snippet}
  </PageHeader>

  <Panel title={t.apiDocsFilterTitle} description={t.apiDocsFilterDescription}>
    <form class="filter-grid" method="GET" action="/api-docs">
      <label class="search-field">{t.apiDocsSearch}<input name="q" value={data.query.search} maxlength="120" placeholder={t.apiDocsSearchPlaceholder} /></label>
      <label>{t.apiDocsMethod}<select name="method"><option value="">{t.apiDocsAll}</option>{#each ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"] as value}<option value={value} selected={data.query.method === value}>{value}</option>{/each}</select></label>
      <label>{t.apiDocsAuth}<select name="auth"><option value="">{t.apiDocsAll}</option>{#each ["Public", "User", "Admin", "API Key"] as value}<option value={value} selected={data.query.auth === value}>{value}</option>{/each}</select></label>
      <label>{t.apiDocsVersion}<select name="version"><option value="">{t.apiDocsAll}</option><option value="v2" selected={data.query.version === "v2"}>V2</option><option value="v1" selected={data.query.version === "v1"}>V1</option></select></label>
      <div class="filter-actions"><button class="button primary" type="submit">{t.apiDocsApply}</button><a class="button secondary" href={href({ q: "", method: "", auth: "", version: "" })}>{t.apiDocsClear}</a></div>
    </form>
  </Panel>

  {#if data.loadError}<p class="notice" role="status">{data.loadError}</p>{/if}

  <Panel title={t.apiDocsListTitle} description={t.apiDocsListDescription.replace("{count}", data.routes.length.toLocaleString("zh-CN")).replace("{total}", data.total.toLocaleString("zh-CN"))}>
    <div class="source-line"><span class="source-badge">{data.source === "admin" ? t.apiDocsAdminSource : t.apiDocsPublicSource}</span><span>{t.apiDocsNoSecrets}</span></div>
    {#if data.routes.length}
      <div class="table-region">
        <table>
          <thead><tr><th>{t.apiDocsMethod}</th><th>{t.apiDocsPath}</th><th>{t.apiDocsAuth}</th><th>{t.apiDocsVersion}</th></tr></thead>
          <tbody>{#each data.routes as route (route.method + route.path)}<tr><td><code class={`method ${route.method.toLowerCase()}`}>{route.method}</code></td><td><code class="path">{route.path}</code></td><td><span class={`auth ${authClass(route.auth)}`}>{route.auth}</span></td><td>{route.version.toUpperCase()}</td></tr>{/each}</tbody>
        </table>
      </div>
    {:else}<p class="empty">{t.apiDocsEmpty}</p>{/if}
  </Panel>
</section>

<style>
  .docs-page { display: grid; gap: 1rem; min-width: 0; }
  .button { align-items: center; display: inline-flex; justify-content: center; max-width: 100%; min-height: var(--tw-control-height); padding: .5rem .8rem; text-decoration: none; }
  .filter-grid { align-items: end; display: grid; gap: .75rem; grid-template-columns: minmax(12rem, 2fr) repeat(3, minmax(8rem, 1fr)) auto; }
  label { color: var(--tw-text-muted); display: grid; font-size: .82rem; gap: .3rem; min-width: 0; }
  .search-field { min-width: 0; }
  input, select { background: var(--tw-surface); border: 1px solid var(--tw-border); color: var(--tw-text); min-width: 0; padding: .55rem .65rem; }
  .filter-actions { align-items: end; display: flex; flex-wrap: wrap; gap: .5rem; }
  .primary { background: var(--tw-accent); color: #fff; }
  .secondary { background: var(--tw-accent-soft); color: var(--tw-text-strong); }
  .notice { background: var(--tw-warning-soft); border: 1px solid #e9c46a; color: var(--tw-warning); margin: 0; padding: .7rem .85rem; }
  .source-line { align-items: center; color: var(--tw-text-muted); display: flex; flex-wrap: wrap; gap: .6rem; }
  .source-badge, .auth { border: 1px solid var(--tw-border); border-radius: 999px; display: inline-flex; font-size: .78rem; font-weight: 700; padding: .2rem .55rem; }
  .source-badge { background: var(--tw-accent-soft); color: var(--tw-text-strong); }
  .table-region { border: 1px solid var(--tw-border); max-height: min(68dvh, 48rem); min-width: 0; overflow: auto; overscroll-behavior: contain; scrollbar-color: var(--tw-border-strong) var(--tw-surface-muted); scrollbar-width: thin; }
  table { border-collapse: collapse; min-width: 42rem; width: 100%; }
  th, td { border-bottom: 1px solid var(--tw-border); padding: .6rem .7rem; text-align: left; vertical-align: middle; }
  th { background: var(--tw-surface-muted); color: var(--tw-text-muted); font-size: .78rem; position: sticky; top: 0; z-index: 1; }
  td { overflow-wrap: anywhere; }
  tr:last-child td { border-bottom: 0; }
  code { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; }
  .method { border-radius: .25rem; display: inline-flex; font-size: .75rem; font-weight: 800; justify-content: center; min-width: 4.4rem; padding: .2rem .35rem; }
  .get { background: var(--tw-success-soft); color: var(--tw-success); } .head { background: var(--tw-success-soft); color: var(--tw-success); } .post { background: #e8f3f7; color: #17627a; } .put, .patch { background: var(--tw-warning-soft); color: var(--tw-warning); } .delete { background: var(--tw-danger-soft); color: var(--tw-danger); } .options { background: #f2f4f5; color: #465865; }
  .path { color: var(--tw-text-strong); overflow-wrap: anywhere; }
  .auth-public { background: var(--tw-success-soft); color: var(--tw-success); } .auth-user { background: #e8f3f7; color: #17627a; } .auth-admin { background: var(--tw-warning-soft); color: var(--tw-warning); } .auth-api-key { background: #f2f4f5; color: #465865; }
  .empty { color: var(--tw-text-muted); margin: 0; padding: 2rem 0; text-align: center; }
  @media (max-width: 900px) { .filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .search-field { grid-column: 1 / -1; } .filter-actions { grid-column: 1 / -1; } }
  @media (max-width: 600px) { .filter-grid { grid-template-columns: 1fr; } .search-field, .filter-actions { grid-column: auto; } .filter-actions, .filter-actions :global(a), .filter-actions :global(button) { width: 100%; } .source-line { align-items: flex-start; flex-direction: column; } table { min-width: 35rem; } }
</style>
