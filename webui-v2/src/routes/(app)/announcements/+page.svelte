<script lang="ts">
  import { t } from "$lib/i18n";
  import type { Announcement, AnnouncementLevel } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { error?: string; success?: boolean; message?: string };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let unseen = $derived(data.announcements?.unseen_force_read || []);
  let announcements = $derived(data.announcements?.announcements || []);

  const levelLabels: Record<AnnouncementLevel, string> = {
    info: t.announcementInfo,
    notice: t.announcementNotice,
    warning: t.announcementWarning,
    critical: t.announcementCritical
  };

  function dateLabel(value: number): string {
    if (!value) return "-";
    return new Date(value * 1000).toISOString().slice(0, 16).replace("T", " ");
  }

  function isExpired(announcement: Announcement): boolean {
    return announcement.expires_at > 0 && announcement.expires_at <= data.now;
  }
</script>

<svelte:head><title>{t.announcements} - {t.siteName}</title></svelte:head>

<section class="announcements-page" aria-labelledby="announcements-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.siteName}</p>
      <h1 id="announcements-title">{t.announcements}</h1>
      <p class="muted">{t.announcementsIntro}</p>
    </div>
    <a class="back-link" href="/dashboard">{t.backDashboard}</a>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if action.success}<p class="notice success" role="status">{action.message || "公告已确认"}</p>{/if}

  {#if unseen.length}
    <section class="required-panel" aria-labelledby="required-title">
      <div class="section-heading">
        <div><h2 id="required-title">{t.requiredAnnouncements}</h2><p class="muted">{t.requiredAnnouncementsHelp}</p></div>
        <form method="POST" action="?/acknowledge">
          {#each unseen as announcement}<input type="hidden" name="id" value={announcement.id} />{/each}
          <button class="button primary" type="submit">{t.acknowledgeAll}</button>
        </form>
      </div>
      <div class="required-list">
        {#each unseen as announcement}
          <article class={`announcement required ${announcement.level}`}>
            <div class="announcement-meta"><span class="level">{levelLabels[announcement.level]}</span><time datetime={new Date(announcement.created_at * 1000).toISOString()}>{dateLabel(announcement.created_at)}</time></div>
            <h3>{announcement.title || t.announcementDefault}</h3>
            <p>{announcement.content}</p>
          </article>
        {/each}
      </div>
    </section>
  {/if}

  {#if announcements.length}
    <div class="announcement-list">
      {#each announcements as announcement}
        <article class={`announcement ${announcement.level}`}>
          <div class="announcement-meta"><span class="level">{levelLabels[announcement.level]}</span>{#if announcement.pinned}<span class="pinned">{t.announcementPinned}</span>{/if}<time datetime={new Date(announcement.created_at * 1000).toISOString()}>{dateLabel(announcement.created_at)}</time></div>
          <h2>{announcement.title || t.announcementDefault}</h2>
          <p>{announcement.content}</p>
          <footer>{t.announcementUpdatedAt} {dateLabel(announcement.updated_at)}{#if isExpired(announcement)} · {t.announcementExpired}{/if}</footer>
        </article>
      {/each}
    </div>
  {:else if !data.loadError}
    <div class="empty"><strong>{t.noAnnouncements}</strong><p>{t.noAnnouncementsHelp}</p></div>
  {/if}
</section>

<style>
  .announcements-page { display: grid; gap: 1rem; }
  .page-heading, .section-heading, .announcement-meta { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .section-heading { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.25rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1, h2, h3 { margin: 0; overflow-wrap: anywhere; }
  h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); }
  h2 { font-size: 1.15rem; }
  h3 { font-size: 1rem; }
  .muted { color: #52606d; margin: .45rem 0 0; }
  .back-link { align-self: center; min-height: 2.5rem; padding: .55rem 0; }
  .required-panel { background: #fffdf5; border: 1px solid #e9c46a; border-radius: .45rem; padding: 1rem; }
  .required-list, .announcement-list { display: grid; gap: .75rem; }
  .required-list { margin-top: 1rem; }
  .announcement-list { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .announcement { background: #fff; border: 1px solid #d7dee5; border-left: 4px solid #9fb3c8; border-radius: .45rem; display: grid; gap: .6rem; min-width: 0; padding: 1rem; }
  .announcement.info { border-left-color: #4f86a1; }
  .announcement.notice { border-left-color: #e0a458; }
  .announcement.warning { border-left-color: #c9822b; }
  .announcement.critical { border-left-color: #a63d40; }
  .announcement-meta { align-items: center; color: #52606d; flex-wrap: wrap; font-size: .78rem; }
  .announcement-meta time { margin-left: auto; }
  .level, .pinned { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; padding: .15rem .45rem; }
  .pinned { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .announcement p { line-height: 1.65; margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .announcement footer { border-top: 1px solid #e1e8ed; color: #7b8794; font-size: .78rem; padding-top: .55rem; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; padding: .5rem .85rem; }
  .button.primary { background: #245b75; color: #fff; }
  .button.primary:hover { background: #1c465a; }
  .notice { border: 1px solid; border-radius: .3rem; padding: .65rem .75rem; }
  .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .empty { color: #52606d; display: grid; gap: .35rem; padding: 2rem 1rem; place-content: center; text-align: center; }
  .empty p { margin: 0; }
  button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 700px) { .announcement-list { grid-template-columns: 1fr; } }
  @media (max-width: 560px) { .page-heading, .section-heading { align-items: stretch; flex-direction: column; } .back-link { align-self: flex-start; } .section-heading form, .section-heading .button { width: 100%; } }
</style>
