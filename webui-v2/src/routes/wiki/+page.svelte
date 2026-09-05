<script lang="ts">
  import { t } from "$lib/i18n";

  type GuideSection = { title: string; items: string[] };

  function items(value: string): string[] { return value.split("|"); }

  const userFlows: GuideSection[] = [
    { title: t.wikiRegisterTitle, items: items(t.wikiRegisterItems) },
    { title: t.wikiEmbyTitle, items: items(t.wikiEmbyItems) },
    { title: t.wikiRenewTitle, items: items(t.wikiRenewItems) },
    { title: t.wikiEmailTitle, items: items(t.wikiEmailItems) }
  ];
  const adminFeatures = items(t.wikiAdminItems);
  const concepts = items(t.wikiConceptItems);
  const workflows = items(t.wikiWorkflowItems);
  const safety = items(t.wikiSafetyItems);
  const extensions = items(t.wikiExtensionItems);
  const faq = items(t.wikiFaqItems);

  function pair(value: string): [string, string] { const split = value.indexOf("："); return split > 0 ? [value.slice(0, split), value.slice(split + 1)] : ["", value]; }
</script>

<svelte:head><title>{t.wikiTitle} - {t.siteName}</title></svelte:head>

<section class="wiki-page" aria-labelledby="wiki-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.siteName} Wiki</p><h1 id="wiki-title">{t.wikiTitle}</h1><p class="muted">{t.wikiDescription}</p></div>
    <a class="button secondary" href="/dashboard">{t.wikiBackDashboard}</a>
  </header>

  <nav class="quick-links" aria-label={t.wikiQuickLinks}>
    <strong>{t.wikiQuickLinks}</strong>
    <a href="/dashboard">{t.wikiBackDashboard}</a><a href="/settings">{t.wikiBackSettings}</a><a href="/invite">{t.wikiBackInvite}</a><a href="/tickets">{t.wikiBackTickets}</a><a href="/announcements">{t.wikiBackAnnouncements}</a><a href="/api/v1/docs">{t.wikiOpenApiDocs}</a>
  </nav>

  <section class="guide-section" aria-labelledby="user-title"><h2 id="user-title">{t.wikiUserTitle}</h2><div class="card-grid">{#each userFlows as flow}<article class="guide-card"><h3>{flow.title}</h3><ul>{#each flow.items as item}<li>{item}</li>{/each}</ul></article>{/each}</div></section>

  <section class="guide-section" aria-labelledby="admin-title"><h2 id="admin-title">{t.wikiAdminTitle}</h2><div class="card-grid three">{#each adminFeatures as feature}{@const [title, text] = pair(feature)}<article class="guide-card"><h3>{title || t.wikiAdminTitle}</h3><p>{text}</p></article>{/each}</div></section>

  <section class="guide-section" aria-labelledby="concept-title"><h2 id="concept-title">{t.wikiConceptTitle}</h2><div class="card-grid">{#each concepts as concept}{@const [title, text] = pair(concept)}<article class="guide-card"><h3>{title}</h3><p>{text}</p></article>{/each}</div></section>

  <section class="guide-section" aria-labelledby="workflow-title"><h2 id="workflow-title">{t.wikiWorkflowTitle}</h2><div class="card-grid">{#each workflows as workflow}{@const [title, text] = pair(workflow)}<article class="guide-card"><h3>{title}</h3><p>{text}</p></article>{/each}</div></section>

  <section class="guide-section" aria-labelledby="safety-title"><h2 id="safety-title">{t.wikiSafetyTitle}</h2><article class="guide-card safety"><ul>{#each safety as note}<li>{note}</li>{/each}</ul></article></section>

  <section class="guide-section" aria-labelledby="extension-title"><h2 id="extension-title">{t.wikiExtensionTitle}</h2><div class="card-grid three">{#each extensions as extension}{@const [title, text] = pair(extension)}<article class="guide-card"><h3>{title}</h3><p>{text}</p></article>{/each}</div></section>

  <section class="guide-section" aria-labelledby="faq-title"><h2 id="faq-title">{t.wikiFaqTitle}</h2><div class="faq-list">{#each faq as value, index}{#if index % 2 === 0}{@const question = value}{@const answer = faq[index + 1] || ""}<details class="guide-card"><summary>{question}</summary><p>{answer}</p></details>{/if}{/each}</div></section>
</section>

<style>
  .wiki-page { display: grid; gap: 1.25rem; min-width: 0; }
  .page-heading { align-items: flex-start; border-bottom: 1px solid #d7dee5; display: flex; gap: 1rem; justify-content: space-between; padding-bottom: 1.2rem; } .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; } h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; } h3 { font-size: 1rem; } .muted { color: #52606d; line-height: 1.55; margin: .45rem 0 0; max-width: 60rem; }
  .button, .quick-links a { align-items: center; background: #e8eef2; border-radius: .3rem; color: #16394a; display: inline-flex; font: inherit; font-weight: 650; min-height: 2.45rem; padding: .5rem .75rem; text-decoration: none; } .button:hover, .quick-links a:hover { background: #d6e1e7; } .quick-links { align-items: center; display: flex; flex-wrap: wrap; gap: .45rem; } .quick-links strong { color: #52606d; margin-right: .25rem; }
  .guide-section { display: grid; gap: .7rem; } .card-grid { display: grid; gap: .7rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .card-grid.three { grid-template-columns: repeat(3, minmax(0, 1fr)); } .guide-card { background: #fff; border: 1px solid #d7dee5; border-radius: .4rem; display: grid; gap: .55rem; min-width: 0; padding: .9rem; } .guide-card p { color: #52606d; line-height: 1.6; margin: 0; white-space: pre-wrap; } .guide-card ul { color: #52606d; display: grid; gap: .45rem; line-height: 1.55; margin: 0; padding-left: 1.2rem; } .guide-card li { overflow-wrap: anywhere; } .safety { border-left: 4px solid #e0a458; }
  .faq-list { display: grid; gap: .6rem; } details summary { color: #16394a; cursor: pointer; font-weight: 700; list-style-position: inside; } details p { margin: .7rem 0 0; }
  a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 800px) { .card-grid.three { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 600px) { .page-heading { align-items: stretch; flex-direction: column; } .page-heading .button { width: 100%; } .card-grid, .card-grid.three { grid-template-columns: 1fr; } h1 { font-size: 1.65rem; } .quick-links { align-items: stretch; } .quick-links strong { flex: 1 0 100%; } .quick-links a { justify-content: center; } }
</style>
