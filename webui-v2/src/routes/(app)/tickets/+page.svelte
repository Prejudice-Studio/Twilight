<script lang="ts">
  import { t } from "$lib/i18n";
  import type { PageData } from "./$types";
  import type { Ticket, TicketStatus } from "$lib/types";

  type FormState = { action?: string; error?: string; draft?: { title?: string; content?: string; type?: string; priority?: string } };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let selected = $derived(data.detail?.ticket || null);
  let types = $derived(data.list?.ticket_types || data.detail?.ticket_types || []);
  let totalPages = $derived(Math.max(1, Math.ceil((data.list?.total || 0) / Math.max(1, data.list?.per_page || 20))));

  const statusLabels: Record<TicketStatus, string> = {
    open: t.ticketStatusOpen,
    in_progress: t.ticketStatusInProgress,
    resolved: t.ticketStatusResolved,
    closed: t.ticketStatusClosed
  };
  const priorityLabels: Record<string, string> = {
    low: t.ticketPriorityLow,
    medium: t.ticketPriorityMedium,
    high: t.ticketPriorityHigh,
    urgent: t.ticketPriorityUrgent
  };

  function timeLabel(value?: number): string {
    return value ? new Date(value * 1000).toISOString().slice(0, 16).replace("T", " ") : "-";
  }

  function isClosed(ticket: Ticket): boolean {
    return ticket.status === "closed";
  }
</script>

<svelte:head><title>{t.tickets} - {t.siteName}</title></svelte:head>

<section class="tickets-page" aria-labelledby="tickets-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.account}</p><h1 id="tickets-title">{t.tickets}</h1><p class="muted">{t.ticketsIntro}</p></div>
    <a class="back-link" href="/dashboard">{t.backDashboard}</a>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  <div class="workspace">
    <aside class="panel ticket-list" aria-label={t.ticketList}>
      <div class="panel-heading"><h2>{t.ticketList}</h2><span class="count">{data.list?.total || 0}</span></div>
      {#if data.list?.tickets.length}
        <div class="list-scroll">
          {#each data.list.tickets as ticket}
            <a class:selected={data.selectedID === ticket.id} class="ticket-row" href={`/tickets?ticket=${ticket.id}`}>
              <div class="row-top"><strong>#{ticket.id}</strong><span class={`status ${ticket.status}`}>{statusLabels[ticket.status]}</span></div>
              <h3>{ticket.title}</h3>
          <div class="row-meta"><span>{priorityLabels[ticket.priority] || ticket.priority}</span><span>{ticket.reply_count} {t.ticketReply}</span><time datetime={new Date(ticket.updated_at * 1000).toISOString()}>{timeLabel(ticket.updated_at)}</time></div>
            </a>
          {/each}
        </div>
        <nav class="pagination" aria-label="工单分页">
          <span>{t.ticketPage.replace("{page}", String(data.list.page)).replace("{pages}", String(totalPages)).replace("{total}", String(data.list.total))}</span>
          {#if data.list.page > 1}<a href={`/tickets?page=${data.list.page - 1}`}>{t.ticketPrevious}</a>{/if}
          {#if data.list.page < totalPages}<a href={`/tickets?page=${data.list.page + 1}`}>{t.ticketNext}</a>{/if}
        </nav>
      {:else}<div class="empty"><strong>{t.ticketEmpty}</strong><p>{t.ticketEmptyHelp}</p></div>{/if}
    </aside>

    <article class="panel conversation" aria-label={t.ticketConversation}>
      {#if selected}
        <header class="conversation-heading">
          <div class="conversation-title"><div class="row-top"><strong>#{selected.id}</strong><span class={`status ${selected.status}`}>{statusLabels[selected.status]}</span><span class="priority">{priorityLabels[selected.priority] || selected.priority}</span></div><h2>{selected.title}</h2><p class="muted">{t.ticketCreated} {timeLabel(selected.created_at)} · {t.ticketUpdated} {timeLabel(selected.updated_at)}</p></div>
          <div class="toolbar">
            <form method="POST" action="?/notify"><input type="hidden" name="ticket_id" value={selected.id} /><input type="hidden" name="enabled" value={selected.notify_telegram ? "false" : "true"} /><button class="button secondary" type="submit">{selected.notify_telegram ? t.ticketNotifyOn : t.ticketNotifyOff}</button></form>
            {#if isClosed(selected)}<form method="POST" action="?/reopen"><input type="hidden" name="ticket_id" value={selected.id} /><button class="button secondary" type="submit">{t.ticketReopen}</button></form>{:else}<form method="POST" action="?/close"><input type="hidden" name="ticket_id" value={selected.id} /><button class="button danger" type="submit">{t.ticketClose}</button></form>{/if}
          </div>
        </header>
        <div class="conversation-scroll">
          <section class="message original"><div class="message-meta"><strong>{t.ticketUser}</strong><time datetime={new Date(selected.created_at * 1000).toISOString()}>{timeLabel(selected.created_at)}</time></div><p>{selected.content}</p></section>
          {#each selected.replies || [] as reply}
            <section class:admin-message={reply.author === "admin" || reply.role === 0} class="message"><div class="message-meta"><strong>{reply.author === "admin" || reply.role === 0 ? t.ticketAdmin : t.ticketUser}</strong><span>{reply.username}</span><time datetime={new Date(reply.created_at * 1000).toISOString()}>{timeLabel(reply.created_at)}</time></div><p>{reply.content}</p></section>
          {/each}
        </div>
        {#if !isClosed(selected)}
          <form method="POST" action="?/reply" class="reply-box"><input type="hidden" name="ticket_id" value={selected.id} /><label for="reply-content">{t.ticketReply}</label><textarea id="reply-content" name="content" rows="4" maxlength="5000" placeholder={t.ticketReplyPlaceholder} required></textarea><div class="reply-actions"><span class="muted small">{t.ticketAdminReplyHint}</span><button class="button primary" type="submit">{t.ticketSend}</button></div></form>
        {:else}<p class="closed-note">{t.ticketClosedHelp}</p>{/if}
      {:else}<div class="empty conversation-empty"><strong>{t.ticketChoose}</strong><p>{t.ticketEmptyHelp}</p></div>{/if}
    </article>
  </div>

  <section class="panel create-panel" aria-labelledby="create-ticket-title">
    <div class="panel-heading"><div><h2 id="create-ticket-title">{t.ticketCreate}</h2><p class="muted">{t.ticketCreatedDescription}</p></div></div>
    <form method="POST" action="?/create" class="create-form">
      <div class="field"><label for="ticket-title">{t.ticketTitle}</label><input id="ticket-title" name="title" maxlength="200" value={action.draft?.title || ""} required /></div>
      <div class="field wide"><label for="ticket-content">{t.ticketContent}</label><textarea id="ticket-content" name="content" rows="4" maxlength="10000" required>{action.draft?.content || ""}</textarea></div>
      <div class="field"><label for="ticket-type">{t.ticketType}</label><select id="ticket-type" name="type"><option value="">{t.ticketNoTypes}</option>{#each types as type}<option value={type}>{type}</option>{/each}</select></div>
      <div class="field"><label for="ticket-priority">{t.ticketPriority}</label><select id="ticket-priority" name="priority"><option value="low">{t.ticketPriorityLow}</option><option value="medium" selected>{t.ticketPriorityMedium}</option><option value="high">{t.ticketPriorityHigh}</option><option value="urgent">{t.ticketPriorityUrgent}</option></select></div>
      <label class="check"><input type="hidden" name="notify_telegram" value="false" /><input type="checkbox" name="notify_telegram" value="true" checked /> <span><strong>{t.ticketNotify}</strong><small>{t.ticketNotifyHelp}</small></span></label>
      <button class="button primary create-button" type="submit">{t.ticketSubmit}</button>
    </form>
  </section>
</section>

<style>
  .tickets-page { display: grid; gap: 1rem; }
  .page-heading, .panel-heading, .conversation-heading, .row-top, .row-meta, .message-meta, .reply-actions { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .conversation-heading, .panel-heading { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.25rem; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .5rem; text-transform: uppercase; }
  h1, h2, h3 { margin: 0; }
  h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); overflow-wrap: anywhere; }
  h2 { font-size: 1.1rem; }
  h3 { font-size: .96rem; overflow-wrap: anywhere; }
  .muted { color: #52606d; margin: .45rem 0 0; }
  .small { font-size: .8rem; }
  .back-link { align-self: center; padding: .5rem 0; }
  .workspace { align-items: stretch; display: grid; gap: 1rem; grid-template-columns: minmax(16rem, 22rem) minmax(0, 1fr); min-height: min(38rem, 68dvh); }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; }
  .ticket-list, .conversation { display: flex; flex-direction: column; min-height: 0; }
  .count { background: #eef2f4; border-radius: 999px; color: #486581; font-size: .8rem; padding: .2rem .5rem; }
  .list-scroll, .conversation-scroll { min-height: 0; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .list-scroll { margin: .75rem -.3rem 0; }
  .ticket-row { border-left: 3px solid transparent; color: inherit; display: grid; gap: .35rem; padding: .75rem .65rem; text-decoration: none; }
  .ticket-row:hover, .ticket-row.selected { background: #f4f7f8; border-left-color: #245b75; }
  .row-top, .row-meta { align-items: center; flex-wrap: wrap; justify-content: space-between; }
  .row-top strong { color: #486581; font: .8rem ui-monospace, monospace; }
  .row-meta { color: #7b8794; font-size: .75rem; }
  .status, .priority { border: 1px solid #c8d2da; border-radius: 999px; font-size: .72rem; padding: .15rem .4rem; }
  .status.open { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; }
  .status.in_progress { background: #edf5f8; border-color: #a9c8d5; color: #245b75; }
  .status.resolved { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .status.closed { background: #eef2f4; color: #52606d; }
  .priority { color: #52606d; }
  .pagination { align-items: center; border-top: 1px solid #e1e8ed; display: flex; font-size: .78rem; gap: .6rem; justify-content: space-between; margin-top: auto; padding-top: .75rem; }
  .pagination a { color: #245b75; }
  .conversation-heading { border-bottom: 1px solid #e1e8ed; flex-wrap: wrap; padding-bottom: .8rem; }
  .conversation-title { min-width: 0; }
  .conversation-title h2 { margin-top: .5rem; overflow-wrap: anywhere; }
  .conversation-title .muted { font-size: .78rem; }
  .toolbar { display: flex; flex-wrap: wrap; gap: .4rem; }
  .toolbar form { display: flex; }
  .conversation-scroll { display: grid; align-content: start; gap: .75rem; padding: 1rem .25rem 1rem 0; }
  .message { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .45rem; margin-left: 1.5rem; padding: .75rem; }
  .message.original, .message.admin-message { margin-left: 0; margin-right: 1.5rem; }
  .message.admin-message { background: #edf5f8; border-color: #b8d0da; }
  .message-meta { color: #52606d; font-size: .76rem; flex-wrap: wrap; }
  .message-meta time { margin-left: auto; }
  .message p { margin: .6rem 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .reply-box { border-top: 1px solid #e1e8ed; display: grid; gap: .5rem; padding-top: .8rem; }
  label { color: #243b53; font-size: .88rem; font-weight: 650; }
  textarea, input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; }
  textarea { resize: vertical; }
  input:focus, textarea:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .reply-actions { align-items: center; justify-content: space-between; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; padding: .5rem .85rem; }
  .button.primary { background: #245b75; color: #fff; }
  .button.primary:hover { background: #1c465a; }
  .button.secondary { background: #e8eef2; color: #16394a; }
  .button.secondary:hover { background: #d6e1e7; }
  .button.danger { background: #a63d40; color: #fff; }
  .notice { border: 1px solid; border-radius: .3rem; padding: .65rem .75rem; }
  .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .empty { color: #52606d; display: grid; gap: .35rem; padding: 2rem 1rem; place-content: center; text-align: center; }
  .empty p { margin: 0; }
  .conversation-empty { flex: 1; }
  .closed-note { border-top: 1px solid #e1e8ed; color: #7b8794; margin: 0; padding-top: .8rem; }
  .create-panel { display: grid; gap: .9rem; }
  .create-panel .panel-heading { justify-content: flex-start; }
  .create-form { display: grid; gap: .75rem; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .field { display: grid; gap: .45rem; }
  .field.wide { grid-column: 1 / -1; }
  .check { align-items: flex-start; display: flex; gap: .5rem; min-width: 0; }
  .check input[type="checkbox"] { accent-color: #245b75; flex: 0 0 auto; height: 1.1rem; min-height: 1.1rem; width: 1.1rem; }
  .check span { display: grid; gap: .2rem; }
  .check small { color: #52606d; font-size: .78rem; font-weight: 400; }
  .create-button { align-self: end; justify-self: end; }
  @media (max-width: 760px) { .workspace { grid-template-columns: 1fr; min-height: auto; } .ticket-list { max-height: 24rem; } .list-scroll { max-height: 17rem; } .conversation { min-height: 35rem; } }
  @media (max-width: 560px) { .page-heading, .conversation-heading { flex-direction: column; } .back-link { align-self: flex-start; } .create-form { grid-template-columns: 1fr; } .field.wide { grid-column: auto; } .create-button { justify-self: stretch; width: 100%; } .reply-actions { align-items: stretch; flex-direction: column; } .reply-actions .button { width: 100%; } .toolbar, .toolbar form { width: 100%; } .toolbar .button { width: 100%; } .message { margin-left: .5rem; } .message.original, .message.admin-message { margin-right: .5rem; } }
</style>
