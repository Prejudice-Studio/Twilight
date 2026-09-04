<script lang="ts">
  import { t } from "$lib/i18n";
  import type { Ticket, TicketPriority, TicketStatus } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let payload = $derived(data.payload);
  let previewSrc = $state<string | null>(null);

  const statusLabels: Record<TicketStatus, string> = {
    open: t.ticketStatusOpen,
    in_progress: t.ticketStatusInProgress,
    resolved: t.ticketStatusResolved,
    closed: t.ticketStatusClosed
  };
  const priorityLabels: Record<TicketPriority, string> = {
    low: t.ticketPriorityLow,
    medium: t.ticketPriorityMedium,
    high: t.ticketPriorityHigh,
    urgent: t.ticketPriorityUrgent
  };
  const statusOptions: TicketStatus[] = ["open", "in_progress", "resolved", "closed"];
  const priorityOptions: TicketPriority[] = ["low", "medium", "high", "urgent"];

  function dateLabel(value?: number): string {
    if (!value || value <= 0) return "-";
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function isAdminReply(reply: { author?: string; role: number }): boolean {
    return reply.author === "admin" || reply.role === 0;
  }

  function safeImageURL(url: string): string {
    return /^\/api\/v1\/tickets\/\d+\/images\/[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$/.test(url) ? url : "";
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function statusLabel(value: string): string {
    return statusLabels[value as TicketStatus] || value || "-";
  }

  function priorityLabel(value: string): string {
    return priorityLabels[value as TicketPriority] || value || "-";
  }
</script>

<svelte:head><title>{payload?.ticket.title || t.adminTicketsTitle} - {t.siteName}</title></svelte:head>

<section class="detail-page" aria-labelledby="ticket-title">
  <header class="page-heading">
    <div class="heading-main">
      <a class="back-link" href="/admin/tickets">{t.adminTicketsBackToList}</a>
      {#if payload}
        {@const ticket = payload.ticket}
        <div class="ticket-badges"><span class={`status ${ticket.status}`}>{statusLabel(ticket.status)}</span><span class={`priority ${ticket.priority}`}>{priorityLabel(ticket.priority)}</span><span class="ticket-id">#{ticket.id}</span></div>
        <h1 id="ticket-title">{ticket.title}</h1>
        <p class="muted">{ticket.username} · UID {ticket.uid} · {t.adminTicketsCreatedAt.replace("{date}", dateLabel(ticket.created_at))}</p>
      {:else}<h1 id="ticket-title">{t.adminTicketsTitle}</h1>{/if}
    </div>
    {#if payload}<a class="button secondary" href={`/admin/tickets/${data.ticketID}`}>{t.adminTicketsRefresh}</a>{/if}
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}

  {#if payload}
    {@const ticket = payload.ticket}
    {@const typeOptions = payload.ticket_types.includes(ticket.type) ? payload.ticket_types : [...payload.ticket_types, ticket.type]}
    <div class="detail-grid">
      <article class="conversation-panel" aria-labelledby="conversation-title">
        <div class="panel-heading"><h2 id="conversation-title">{t.ticketConversation}</h2><span class="muted">{t.adminTicketsConversationHelp}</span></div>
        <div class="conversation-scroll">
          <section class="message original">
            <div class="message-meta"><strong>{ticket.username}</strong><time datetime={new Date(ticket.created_at * 1000).toISOString()}>{dateLabel(ticket.created_at)}</time></div>
            <p>{ticket.content}</p>
          </section>
          {#if ticket.attachments?.length}
            <section class="attachment-section" aria-labelledby="attachments-title">
              <h3 id="attachments-title">{t.adminTicketsAttachmentsTitle}</h3>
              <div class="attachment-grid">
                {#each ticket.attachments as attachment (attachment.filename)}
                  {@const src = safeImageURL(attachment.url)}
                  {#if src}
                    <figure class="attachment-item">
                      <button type="button" class="image-button" onclick={() => previewSrc = src} aria-label={`${t.adminTicketsPreviewImage}: ${attachment.filename}`}>
                        <img src={src} alt={attachment.filename} loading="lazy" />
                      </button>
                      <figcaption><span>{Math.max(1, Math.round(attachment.size / 1024))} KB</span><form method="POST" action="?/deleteImage" onsubmit={(event) => confirmSubmit(event, t.adminTicketsDeleteImageConfirm)}><input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="filename" value={attachment.filename} /><button class="link-button danger-link" type="submit">{t.adminTicketsDeleteImage}</button></form></figcaption>
                    </figure>
                  {/if}
                {/each}
              </div>
            </section>
          {/if}
          {#each ticket.replies || [] as reply, index (`${reply.created_at}-${reply.uid}-${index}`)}
            <section class:admin-message={isAdminReply(reply)} class="message">
              <div class="message-meta"><strong>{isAdminReply(reply) ? t.ticketAdmin : reply.username}</strong><span>{reply.username}</span><time datetime={new Date(reply.created_at * 1000).toISOString()}>{dateLabel(reply.created_at)}</time></div>
              <p>{reply.content}</p>
            </section>
          {/each}
        </div>

        <div class="composer">
          <form method="POST" action="?/reply" class="reply-form">
            <input type="hidden" name="ticket_id" value={ticket.id} />
            <label for="reply-content">{t.adminTicketsReplyLabel}</label>
            <textarea id="reply-content" name="content" rows="5" maxlength="5000" required placeholder={t.adminTicketsReplyPlaceholder}></textarea>
            <div class="form-actions"><span class="muted small">{t.adminTicketsReplyHint}</span><button class="button primary" type="submit">{t.adminTicketsSendReply}</button></div>
          </form>
          <form method="POST" action="?/uploadImage" enctype="multipart/form-data" class="upload-form">
            <input type="hidden" name="ticket_id" value={ticket.id} />
            <label for="ticket-image">{t.adminTicketsUploadImage}</label>
            <div class="upload-controls"><input id="ticket-image" name="file" type="file" accept="image/jpeg,image/png,image/gif,image/webp,image/bmp" required /><button class="button secondary" type="submit">{t.adminTicketsUpload}</button></div>
            <p class="muted small">{t.adminTicketsUploadHelp}</p>
          </form>
        </div>
      </article>

      <aside class="side-column">
        <section class="panel metadata-panel" aria-labelledby="metadata-title">
          <div class="panel-heading"><h2 id="metadata-title">{t.adminTicketsMetadata}</h2><span class="muted">{t.adminTicketsMetadataHelp}</span></div>
          <form method="POST" action="?/updateMeta" class="field-form"><input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="field" value="status" /><label for="ticket-status">{t.adminTicketsStatus}</label><div class="field-action"><select id="ticket-status" name="value">{#each statusOptions as status}<option value={status} selected={ticket.status === status}>{statusLabels[status]}</option>{/each}</select><button class="button compact secondary" type="submit">{t.adminTicketsSave}</button></div></form>
          <form method="POST" action="?/updateMeta" class="field-form"><input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="field" value="priority" /><label for="ticket-priority">{t.ticketPriority}</label><div class="field-action"><select id="ticket-priority" name="value">{#each priorityOptions as priority}<option value={priority} selected={ticket.priority === priority}>{priorityLabels[priority]}</option>{/each}</select><button class="button compact secondary" type="submit">{t.adminTicketsSave}</button></div></form>
          <form method="POST" action="?/updateMeta" class="field-form"><input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="field" value="type" /><label for="ticket-type">{t.ticketType}</label><div class="field-action"><select id="ticket-type" name="value">{#each typeOptions as type}<option value={type} selected={ticket.type === type}>{type}</option>{/each}</select><button class="button compact secondary" type="submit">{t.adminTicketsSave}</button></div></form>
          <form method="POST" action="?/updateMeta" class="field-form"><input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="field" value="admin_note" /><label for="admin-note">{t.adminTicketsAdminNote}</label><textarea id="admin-note" name="value" rows="5" maxlength="5000" placeholder={t.adminTicketsAdminNotePlaceholder}>{ticket.admin_note || ""}</textarea><button class="button secondary" type="submit">{t.adminTicketsSaveNote}</button></form>
        </section>

        <section class="panel facts-panel" aria-labelledby="facts-title">
          <h2 id="facts-title">{t.adminTicketsFacts}</h2>
          <dl class="facts"><div><dt>{t.adminTicketsCreatedAtLabel}</dt><dd>{dateLabel(ticket.created_at)}</dd></div><div><dt>{t.adminTicketsUpdatedAtLabel}</dt><dd>{dateLabel(ticket.updated_at)}</dd></div><div><dt>{t.adminTicketsReplyCount}</dt><dd>{ticket.replies?.length || 0}</dd></div><div><dt>{t.adminTicketsAttachmentCount}</dt><dd>{ticket.attachments?.length || 0}</dd></div></dl>
          <form method="POST" action="?/delete" onsubmit={(event) => confirmSubmit(event, t.adminTicketsDeleteConfirm.replace("{id}", String(ticket.id)))}><input type="hidden" name="ticket_id" value={ticket.id} /><button class="button danger full" type="submit">{t.adminTicketsDelete}</button></form>
        </section>
      </aside>
    </div>
  {:else}<div class="empty"><strong>{t.adminTicketsNoDetail}</strong><a class="button secondary" href="/admin/tickets">{t.adminTicketsBackToList}</a></div>{/if}
</section>

{#if previewSrc}
  <div class="image-modal" role="dialog" aria-modal="true" aria-label={t.adminTicketsPreviewImage} tabindex="-1">
    <button class="modal-close" type="button" onclick={(event) => { event.stopPropagation(); previewSrc = null; }} aria-label={t.commonClose}>{t.commonClose}</button>
    <img src={previewSrc} alt="" />
  </div>
{/if}

<style>
  .detail-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-main, .panel-heading, .ticket-badges, .message-meta, .form-actions, .upload-controls, figcaption, .field-action { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading { justify-content: space-between; } .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.15rem; } .heading-main { flex-direction: column; min-width: 0; }
  .back-link { min-height: 2.25rem; padding: .45rem 0; } h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.1rem; } h3 { font-size: .95rem; } .muted { color: #52606d; margin: .35rem 0 0; } .small { font-size: .78rem; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; padding: .5rem .85rem; text-decoration: none; white-space: normal; } .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.warning { background: #fff0d2; color: #7b4f00; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; } .button.full { width: 100%; }
  .ticket-badges { align-items: center; flex-wrap: wrap; } .status, .priority, .ticket-id { border: 1px solid #c8d2da; border-radius: 999px; font-size: .74rem; padding: .2rem .5rem; white-space: nowrap; } .status.open { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .status.in_progress { background: #edf5f8; border-color: #a9c8d5; color: #245b75; } .status.resolved { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .status.closed { background: #eef2f4; color: #52606d; } .priority { background: #f4f6f8; color: #52606d; } .priority.high { background: #fff8e6; color: #7b4f00; } .priority.urgent { background: #fff1f0; color: #a61b1b; } .ticket-id { color: #52606d; font-family: ui-monospace, monospace; }
  .panel, .conversation-panel, .notice { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .detail-grid { align-items: stretch; display: grid; gap: 1rem; grid-template-columns: minmax(0, 1fr) minmax(18rem, 22rem); min-width: 0; } .conversation-panel { display: flex; flex-direction: column; min-height: 70dvh; padding: 0; } .conversation-panel > .panel-heading { border-bottom: 1px solid #e1e8ed; padding: 1rem; } .conversation-scroll { display: grid; align-content: start; gap: .8rem; min-height: 0; overflow: auto; overscroll-behavior: contain; padding: 1rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .message { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .45rem; margin-left: 2rem; padding: .8rem; } .message.original, .message.admin-message { margin-left: 0; margin-right: 2rem; } .message.admin-message { background: #edf5f8; border-color: #b8d0da; } .message-meta { color: #52606d; flex-wrap: wrap; font-size: .76rem; } .message-meta time { margin-left: auto; } .message p { margin: .55rem 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .attachment-section { border: 1px solid #d7dee5; border-radius: .4rem; padding: .75rem; } .attachment-section h3 { margin-bottom: .65rem; } .attachment-grid { display: grid; gap: .65rem; grid-template-columns: repeat(auto-fill, minmax(8rem, 1fr)); } .attachment-item { border: 1px solid #c8d2da; margin: 0; min-width: 0; padding: .35rem; } .image-button { background: #f4f6f8; border: 0; cursor: pointer; display: block; height: 8rem; padding: 0; width: 100%; } .image-button img { height: 100%; object-fit: contain; width: 100%; } figcaption { align-items: center; color: #52606d; flex-wrap: wrap; font-size: .72rem; justify-content: space-between; margin-top: .35rem; } .link-button { background: none; border: 0; cursor: pointer; font: inherit; padding: 0; } .danger-link { color: #a63d40; }
  .composer { border-top: 1px solid #e1e8ed; display: grid; gap: .9rem; padding: 1rem; } .reply-form, .upload-form, .field-form { display: grid; gap: .5rem; } label { color: #243b53; font-size: .85rem; font-weight: 650; } input, select, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } textarea { resize: vertical; } input:focus, select:focus, textarea:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; } .form-actions { align-items: center; justify-content: space-between; } .form-actions .muted { flex: 1; } .upload-form { border-top: 1px solid #e1e8ed; padding-top: .8rem; } .upload-controls { align-items: center; flex-wrap: wrap; } .upload-controls input { flex: 1 1 12rem; }
  .side-column { display: grid; align-content: start; gap: 1rem; min-width: 0; } .metadata-panel, .facts-panel { display: grid; gap: .9rem; } .metadata-panel .panel-heading { display: grid; gap: .2rem; justify-content: initial; } .field-form { border-top: 1px solid #e1e8ed; padding-top: .75rem; } .field-action { align-items: center; } .field-action select { flex: 1; } .field-form > textarea { width: 100%; } .facts { display: grid; gap: .55rem; margin: 0; } .facts div { align-items: baseline; border-bottom: 1px solid #e1e8ed; display: flex; gap: .75rem; justify-content: space-between; padding-bottom: .5rem; } .facts div:last-child { border-bottom: 0; padding-bottom: 0; } dt { color: #52606d; font-size: .82rem; } dd { font-weight: 650; margin: 0; max-width: 65%; overflow-wrap: anywhere; text-align: right; }
  .empty { align-items: center; color: #52606d; display: grid; gap: .75rem; justify-items: center; padding: 3rem 1rem; text-align: center; } .image-modal { align-items: center; background: rgb(13 24 33 / 88%); display: flex; inset: 0; justify-content: center; padding: 3rem; position: fixed; z-index: 10; } .image-modal img { max-height: 90dvh; max-width: min(96vw, 72rem); object-fit: contain; } .modal-close { background: #fff; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; min-height: 2.5rem; padding: .5rem .75rem; position: fixed; right: 1rem; top: 1rem; }
  @media (max-width: 900px) { .detail-grid { grid-template-columns: 1fr; } .conversation-panel { min-height: 65dvh; } .side-column { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 600px) { .page-heading, .panel-heading, .form-actions, .field-action { align-items: stretch; flex-direction: column; } .page-heading > .button, .form-actions .button, .field-action .button { width: 100%; } .detail-grid { gap: .75rem; } .side-column { grid-template-columns: 1fr; } .message { margin-left: .5rem; } .message.original, .message.admin-message { margin-right: .5rem; } .composer, .conversation-panel > .panel-heading, .panel { padding: .85rem; } .upload-controls { align-items: stretch; flex-direction: column; } .upload-controls .button { width: 100%; } .image-modal { padding: 1rem; } }
</style>
