<script lang="ts">
  import { t } from "$lib/i18n";

  type Entry = { href: string; title: string; description: string; mark: string; tone: "info" | "warning" | "danger" | "neutral" | "primary" };

  const entries: Entry[] = [
    { href: "/admin/audit-logs", title: t.adminSecurityAuditTitle, description: t.adminSecurityAuditDescription, mark: "审", tone: "primary" },
    { href: "/admin/logs", title: t.adminSecurityLogsTitle, description: t.adminSecurityLogsDescription, mark: "运", tone: "info" },
    { href: "/admin/violations", title: t.adminSecurityViolationsTitle, description: t.adminSecurityViolationsDescription, mark: "风", tone: "danger" },
    { href: "/admin/emby?tab=devices", title: t.adminSecurityDevicesTitle, description: t.adminSecurityDevicesDescription, mark: "设", tone: "warning" },
    { href: "/admin/config", title: t.adminSecurityConfigTitle, description: t.adminSecurityConfigDescription, mark: "策", tone: "neutral" }
  ];
</script>

<svelte:head><title>{t.adminSecurityTitle} - {t.siteName}</title></svelte:head>

<section class="security-page" aria-labelledby="security-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="security-title">{t.adminSecurityTitle}</h1>
      <p class="muted">{t.adminSecurityDescription}</p>
    </div>
    <a class="button secondary" href="/admin">{t.adminSecurityBack}</a>
  </header>

  <section class="notice" aria-labelledby="security-boundary-title">
    <h2 id="security-boundary-title">{t.adminSecurityBoundaryTitle}</h2>
    <p>{t.adminSecurityBoundaryDescription}</p>
  </section>

  <div class="entry-grid">
    {#each entries as entry}
      <a class="entry" href={entry.href}>
        <span class={`entry-mark ${entry.tone}`} aria-hidden="true">{entry.mark}</span>
        <span class="entry-copy"><strong>{entry.title}</strong><span>{entry.description}</span><small>{entry.href}</small></span>
        <span class="entry-arrow" aria-hidden="true">→</span>
      </a>
    {/each}
  </div>
</section>

<style>
  .security-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading { align-items: flex-start; border-bottom: 1px solid #d7dee5; display: flex; gap: .8rem; justify-content: space-between; padding-bottom: 1.1rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p, strong, span, small { overflow-wrap: anywhere; } h1, h2, p { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.05rem; }
  .muted { color: #52606d; line-height: 1.5; margin-top: .4rem; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.45rem; max-width: 100%; padding: .5rem .8rem; text-align: center; text-decoration: none; white-space: normal; }
  .button:hover { background: #d6e1e7; }
  .notice { background: #f4f8fa; border: 1px solid #c8d8e1; border-radius: .45rem; color: #243b53; display: grid; gap: .35rem; padding: .85rem 1rem; }
  .notice p { color: #486581; line-height: 1.5; }
  .entry-grid { display: grid; gap: .7rem; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .entry { align-items: center; background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; color: inherit; display: flex; gap: .75rem; min-width: 0; padding: .85rem; text-decoration: none; }
  .entry:hover { background: #f8fafb; border-color: #9fb3c8; }
  .entry-mark { align-items: center; background: #e8eef2; border: 1px solid #c8d2da; border-radius: .3rem; color: #245b75; display: flex; flex: 0 0 2.45rem; font-weight: 750; height: 2.45rem; justify-content: center; }
  .entry-mark.info { background: #e8f3f7; border-color: #b9dce8; color: #17627a; } .entry-mark.warning { background: #fff8e6; border-color: #ead398; color: #7b4f00; } .entry-mark.danger { background: #fff1f0; border-color: #f1b4ae; color: #a63d40; } .entry-mark.neutral { background: #f2f4f5; border-color: #d5dce0; color: #465865; }
  .entry-copy { display: grid; gap: .22rem; min-width: 0; } .entry-copy strong { color: #16394a; } .entry-copy span { color: #52606d; font-size: .82rem; line-height: 1.4; } .entry-copy small { color: #7b8794; font: .7rem ui-monospace, SFMono-Regular, Consolas, monospace; }
  .entry-arrow { color: #52606d; flex: 0 0 auto; font-size: 1.15rem; margin-left: auto; } a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 760px) { .entry-grid { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading { align-items: stretch; flex-direction: column; } .page-heading .button { width: 100%; } h1 { font-size: 1.65rem; } .notice, .entry { padding: .8rem; } }
</style>
