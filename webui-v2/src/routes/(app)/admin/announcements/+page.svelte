<script lang="ts">
  import { t } from "$lib/i18n";
  import PageHeader from "$lib/components/PageHeader.svelte";
  import Panel from "$lib/components/Panel.svelte";
  import type { Announcement, AnnouncementLevel } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string });

  const levelLabels: Record<AnnouncementLevel, string> = {
    info: t.adminAnnouncementsInfo,
    notice: t.adminAnnouncementsNotice,
    warning: t.adminAnnouncementsWarning,
    critical: t.adminAnnouncementsCritical
  };
  const levels: AnnouncementLevel[] = ["info", "notice", "warning", "critical"];
  const renderModes = [
    ["plain", t.adminAnnouncementsRenderPlain],
    ["markdown", t.adminAnnouncementsRenderMarkdown],
    ["bbcode", t.adminAnnouncementsRenderBBCode]
  ];

  function expiryOf(announcement: Announcement): number {
    return announcement.expired_at ?? announcement.expires_at ?? 0;
  }

  function isExpired(announcement: Announcement): boolean {
    const expiry = expiryOf(announcement);
    return expiry > 0 && expiry <= Math.floor(Date.now() / 1000);
  }

  function dateLabel(value: number | undefined): string {
    return value && value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : "-";
  }

  function datetimeValue(value: number | undefined): string {
    if (!value || value <= 0) return "";
    const date = new Date(value * 1000);
    const pad = (number: number) => String(number).padStart(2, "0");
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }

  function labelFor(level: string): string {
    return levelLabels[level as AnnouncementLevel] || level || t.adminAuditLogUnknown;
  }

  function pageHref(page: number): string {
    const params = new URLSearchParams({
      page: String(Math.max(1, page)),
      per_page: String(data.query.per_page),
      include_invisible: String(data.query.include_invisible),
      include_expired: String(data.query.include_expired)
    });
    return `/admin/announcements?${params}`;
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }
</script>

<svelte:head><title>{t.adminAnnouncementsTitle} - {t.siteName}</title></svelte:head>

<section class="announcements-page" aria-labelledby="announcements-title">
  <PageHeader id="announcements-title" eyebrow={t.adminArea} title={t.adminAnnouncementsTitle} description={t.adminAnnouncementsDescription}>
    {#snippet actions()}
      <a class="text-link" href="/admin/status">{t.adminStatusTitle}</a>
      <a class="button secondary" href={pageHref(data.query.page)}>{t.adminAnnouncementsRefresh}</a>
    {/snippet}
  </PageHeader>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "created"}<p class="notice success" role="status">{t.adminAnnouncementsCreated}</p>{/if}
  {#if data.notice === "updated"}<p class="notice success" role="status">{t.adminAnnouncementsUpdated}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.adminAnnouncementsDeleted}</p>{/if}
  {#if data.notice === "hidden"}<p class="notice success" role="status">{t.adminAnnouncementsHidden}</p>{/if}
  {#if data.notice === "shown"}<p class="notice success" role="status">{t.adminAnnouncementsShown}</p>{/if}
  {#if data.notice === "pinned"}<p class="notice success" role="status">{t.adminAnnouncementsPinnedDone}</p>{/if}
  {#if data.notice === "unpinned"}<p class="notice success" role="status">{t.adminAnnouncementsUnpinnedDone}</p>{/if}

  <Panel id="filter-title" className="filters">
    <header class="panel-heading"><div><h2 id="filter-title">{t.adminAnnouncementsFilter}</h2><p class="muted">{t.adminAnnouncementsTotal.replace("{count}", String(data.payload?.total || 0))}</p></div></header>
    <form method="GET" action="/admin/announcements" class="filter-form">
      <label>{t.adminAnnouncementsShowHidden}<select name="include_invisible"><option value="true" selected={data.query.include_invisible}>是</option><option value="false" selected={!data.query.include_invisible}>否</option></select></label>
      <label>{t.adminAnnouncementsShowExpired}<select name="include_expired"><option value="true" selected={data.query.include_expired}>是</option><option value="false" selected={!data.query.include_expired}>否</option></select></label>
      <label>{t.adminAuditLogPerPage}<select name="per_page">{#each [20, 50, 100] as value}<option value={value} selected={data.query.per_page === value}>{value}</option>{/each}</select></label>
      <input type="hidden" name="page" value="1" />
      <button class="button primary" type="submit">{t.adminAuditLogApply}</button>
      <a class="button secondary" href="/admin/announcements">{t.adminAuditLogReset}</a>
    </form>
  </Panel>

  <details class="editor-panel" open>
    <summary>{t.adminAnnouncementsCreate}</summary>
    <p class="muted">{t.adminAnnouncementsDescription}</p>
    <form method="POST" action="?/save" class="editor-form">
      <input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} />
      <input type="hidden" name="include_invisible" value={data.query.include_invisible} /><input type="hidden" name="include_expired" value={data.query.include_expired} />
      <label>{t.adminAnnouncementsTitleLabel}<input name="title" maxlength="200" placeholder={t.adminAnnouncementsTitlePlaceholder} /></label>
      <label class="wide-field">{t.adminAnnouncementsContent}<textarea name="content" maxlength="10000" required placeholder={t.adminAnnouncementsContentPlaceholder}></textarea></label>
      <div class="editor-grid">
        <label>{t.adminAnnouncementsLevel}<select name="level">{#each levels as level}<option value={level}>{labelFor(level)}</option>{/each}</select></label>
        <label>{t.adminAnnouncementsRenderMode}<select name="render_mode">{#each renderModes as [value, label]}<option value={value}>{label}</option>{/each}</select></label>
        <label>{t.adminAnnouncementsExpires}<input name="expires_at" type="datetime-local" /></label>
        <label>{t.adminAnnouncementsForceReadSeconds}<input name="force_read_seconds" type="number" min="0" max="2678400" value="0" /></label>
      </div>
      <p class="help">{t.adminAnnouncementsRenderHelp}</p>
      <div class="switch-grid">
        <label class="check-field"><input type="hidden" name="pinned" value="false" /><input type="checkbox" name="pinned" value="true" />{t.adminAnnouncementsPinned}</label>
        <label class="check-field"><input type="hidden" name="visible" value="false" /><input type="checkbox" name="visible" value="true" checked />{t.adminAnnouncementsVisible}</label>
        <label class="check-field"><input type="hidden" name="force_read" value="false" /><input type="checkbox" name="force_read" value="true" />{t.adminAnnouncementsForceRead}</label>
      </div>
      <div class="preview"><strong>{t.adminAnnouncementsPreview}</strong><p>{t.adminAnnouncementsRenderHelp}</p></div>
      <button class="button primary" type="submit">{t.adminAnnouncementsCreate}</button>
    </form>
  </details>

  <Panel id="list-title" className="list-panel">
    <header class="panel-heading"><div><h2 id="list-title">{t.adminAnnouncementsList}</h2><p class="muted">{t.adminAnnouncementsPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(data.payload?.pages || 1))}</p></div></header>
    {#if data.payload?.announcements?.length}
      <div class="announcement-list">
        {#each data.payload.announcements as announcement (announcement.id)}
          {@const expired = isExpired(announcement)}
          {@const expiry = expiryOf(announcement)}
          <article class="announcement" class:dimmed={!announcement.visible || expired}>
            <header class="announcement-header">
              <div class="announcement-title">
                <div class="badges"><span class={`badge level-${announcement.level}`}>{labelFor(announcement.level)}</span>{#if announcement.pinned}<span class="badge pinned">{t.adminAnnouncementsPinned}</span>{/if}{#if !announcement.visible}<span class="badge">{t.adminAnnouncementsHiddenState}</span>{/if}{#if expired}<span class="badge expired">{t.adminAnnouncementsExpiredState}</span>{/if}{#if announcement.force_read}<span class="badge">{t.adminAnnouncementsForceReadState}</span>{/if}</div>
                <h3>{announcement.title || t.announcementDefault}</h3>
                <p class="meta">#{announcement.id} · {t.adminAnnouncementsPublishedAt.replace("{date}", dateLabel(announcement.created_at))}{#if announcement.updated_at && announcement.updated_at !== announcement.created_at} · {t.adminAnnouncementsUpdatedAt.replace("{date}", dateLabel(announcement.updated_at))}{/if}{#if expiry > 0} · {t.adminAnnouncementsExpiresAt.replace("{date}", dateLabel(expiry))}{/if}</p>
              </div>
              <div class="row-actions">
                <form method="POST" action="?/togglePinned"><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="include_invisible" value={data.query.include_invisible} /><input type="hidden" name="include_expired" value={data.query.include_expired} /><input type="hidden" name="announcement_id" value={announcement.id} /><input type="hidden" name="pinned" value={!announcement.pinned} /><button class="button compact secondary" type="submit">{announcement.pinned ? t.adminAnnouncementsUnpinnedDone : t.adminAnnouncementsPinned}</button></form>
                <form method="POST" action="?/toggleVisible"><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="include_invisible" value={data.query.include_invisible} /><input type="hidden" name="include_expired" value={data.query.include_expired} /><input type="hidden" name="announcement_id" value={announcement.id} /><input type="hidden" name="visible" value={!announcement.visible} /><button class="button compact secondary" type="submit">{announcement.visible ? t.adminAnnouncementsHiddenState : t.adminAnnouncementsVisible}</button></form>
              </div>
            </header>
            <p class="content">{announcement.content}</p>
            <details class="editor-panel inline-editor"><summary>{t.adminAnnouncementsEdit}</summary>
              <form method="POST" action="?/save" class="editor-form">
                <input type="hidden" name="announcement_id" value={announcement.id} /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="include_invisible" value={data.query.include_invisible} /><input type="hidden" name="include_expired" value={data.query.include_expired} />
                <label>{t.adminAnnouncementsTitleLabel}<input name="title" maxlength="200" value={announcement.title || ""} /></label>
                <label class="wide-field">{t.adminAnnouncementsContent}<textarea name="content" maxlength="10000" required>{announcement.content}</textarea></label>
                <div class="editor-grid"><label>{t.adminAnnouncementsLevel}<select name="level">{#each levels as level}<option value={level} selected={announcement.level === level}>{labelFor(level)}</option>{/each}</select></label><label>{t.adminAnnouncementsRenderMode}<select name="render_mode">{#each renderModes as [value, label]}<option value={value} selected={(announcement.render_mode || "plain") === value}>{label}</option>{/each}</select></label><label>{t.adminAnnouncementsExpires}<input name="expires_at" type="datetime-local" value={datetimeValue(expiry)} /></label><label>{t.adminAnnouncementsForceReadSeconds}<input name="force_read_seconds" type="number" min="0" max="2678400" value={announcement.force_read_seconds || 0} /></label></div>
                <p class="help">{t.adminAnnouncementsRenderHelp}</p>
                <div class="switch-grid"><label class="check-field"><input type="hidden" name="pinned" value="false" /><input type="checkbox" name="pinned" value="true" checked={announcement.pinned} />{t.adminAnnouncementsPinned}</label><label class="check-field"><input type="hidden" name="visible" value="false" /><input type="checkbox" name="visible" value="true" checked={announcement.visible} />{t.adminAnnouncementsVisible}</label><label class="check-field"><input type="hidden" name="force_read" value="false" /><input type="checkbox" name="force_read" value="true" checked={announcement.force_read} />{t.adminAnnouncementsForceRead}</label></div>
                <button class="button primary" type="submit">{t.adminAnnouncementsSave}</button>
              </form>
            </details>
            <form method="POST" action="?/delete" class="delete-form" onsubmit={(event) => confirmSubmit(event, t.adminAnnouncementsDeleteConfirm)}><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="include_invisible" value={data.query.include_invisible} /><input type="hidden" name="include_expired" value={data.query.include_expired} /><input type="hidden" name="announcement_id" value={announcement.id} /><button class="button danger compact" type="submit">{t.adminAnnouncementsDelete}</button></form>
          </article>
        {/each}
      </div>
    {:else}<p class="empty">{t.adminAnnouncementsEmpty}</p>{/if}
    {#if (data.payload?.pages || 1) > 1}<nav class="pagination" aria-label={t.adminAnnouncementsTitle}>{#if (data.payload?.page || data.query.page) > 1}<a class="button secondary" href={pageHref((data.payload?.page || data.query.page) - 1)}>{t.adminAnnouncementsPrevious}</a>{:else}<span></span>{/if}<span>{t.adminAnnouncementsPageOf.replace("{page}", String(data.payload?.page || data.query.page)).replace("{pages}", String(data.payload?.pages || 1))}</span>{#if (data.payload?.page || data.query.page) < (data.payload?.pages || 1)}<a class="button secondary" href={pageHref((data.payload?.page || data.query.page) + 1)}>{t.adminAnnouncementsNext}</a>{:else}<span></span>{/if}</nav>{/if}
  </Panel>
</section>

<style>
  .announcements-page { display: grid; gap: 1rem; min-width: 0; }
  .panel-heading, .filter-form, .editor-form, .editor-grid, .switch-grid, .announcement-header, .badges, .row-actions, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .panel-heading, .announcement-header, .pagination { justify-content: space-between; }
  h2, h3, p { overflow-wrap: anywhere; } h2, h3 { margin: 0; } h2 { font-size: 1.15rem; } h3 { font-size: 1.05rem; } .muted, .meta, .help { color: #52606d; margin: .4rem 0 0; } .meta, .help { font-size: .78rem; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; } .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.2rem; padding: .35rem .6rem; }
  .editor-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; } .notice { border: 1px solid; border-radius: .35rem; margin: 0; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .filter-form { align-items: end; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto auto; } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } textarea { min-height: 8rem; resize: vertical; } input[type="checkbox"] { min-height: 1rem; width: 1rem; } input:focus, select:focus, textarea:focus, button:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .editor-panel > summary, .inline-editor > summary { color: #245b75; cursor: pointer; font-weight: 700; } .editor-form { display: grid; grid-template-columns: minmax(0, 1fr); } .wide-field { grid-column: 1 / -1; } .editor-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); } .switch-grid { flex-wrap: wrap; } .check-field { align-items: center; display: flex; font-weight: 500; } .check-field input[type="checkbox"] { margin-right: .35rem; } .help { border-left: 3px solid #9fb3c8; padding-left: .6rem; }
  .preview { background: #f4f7f8; border: 1px solid #d7dee5; display: grid; gap: .3rem; padding: .7rem; } .preview p { margin: 0; } .announcement-list { display: grid; gap: .8rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .announcement { border: 1px solid #d7dee5; display: grid; gap: .8rem; min-width: 0; padding: .9rem; } .announcement.dimmed { opacity: .7; } .announcement-title { min-width: 0; } .badges { flex-wrap: wrap; } .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .72rem; padding: .2rem .45rem; white-space: nowrap; } .level-info { background: #edf4f8; color: #245b75; } .level-notice { background: #eef8f1; color: #276749; } .level-warning { background: #fff8e6; color: #7b4f00; } .level-critical, .badge.expired { background: #fff1f0; color: #a61b1b; } .announcement-title h3 { margin-top: .45rem; } .meta { margin-top: .3rem; } .content { line-height: 1.65; margin: 0; white-space: pre-wrap; } .inline-editor { border-top: 1px solid #e1e8ed; padding-top: .7rem; } .delete-form { justify-self: end; }
  .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination span { color: #52606d; font-size: .84rem; } .empty { color: #52606d; padding: 1.5rem; text-align: center; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .editor-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 600px) { .panel-heading, .announcement-header { align-items: stretch; flex-direction: column; } .filter-form, .editor-grid { grid-template-columns: 1fr; } .filter-form .button { width: 100%; } .row-actions, .row-actions form, .row-actions .button, .delete-form, .delete-form .button { width: 100%; } .row-actions { flex-direction: column; } .delete-form { justify-self: stretch; } .editor-panel { padding: .85rem; } }
</style>
