<script lang="ts">
  import PageHeader from "$lib/components/PageHeader.svelte";
  import Panel from "$lib/components/Panel.svelte";
  import { t } from "$lib/i18n";
  import type { AdminTicketSummary, TicketPriority, TicketStatus } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let payload = $derived(data.payload);
  let totalPages = $derived(Math.max(1, Math.ceil((payload?.total || 0) / Math.max(1, payload?.per_page || 20))));

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

  function statusLabel(value: string): string {
    return statusLabels[value as TicketStatus] || value || "-";
  }

  function priorityLabel(value: string): string {
    return priorityLabels[value as TicketPriority] || value || "-";
  }

  function queryHref(page: number): string {
    const params = new URLSearchParams();
    params.set("page", String(Math.max(1, page)));
    params.set("per_page", String(data.query.per_page));
    for (const name of ["status", "type", "priority"] as const) {
      if (data.query[name]) params.set(name, data.query[name]);
    }
    if (data.query.uid > 0) params.set("uid", String(data.query.uid));
    return `/admin/tickets?${params.toString()}`;
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function statusActionLabel(status: TicketStatus): string {
    if (status === "open") return t.adminTicketsReopen;
    if (status === "in_progress") return t.adminTicketsMarkInProgress;
    if (status === "resolved") return t.adminTicketsMarkResolved;
    return t.adminTicketsClose;
  }
</script>

<svelte:head><title>{t.adminTicketsTitle} - {t.siteName}</title></svelte:head>

<section class="tickets-page" aria-labelledby="admin-tickets-title">
  <PageHeader id="admin-tickets-title" eyebrow={t.adminArea} title={t.adminTicketsTitle} description={t.adminTicketsDescription}>
    {#snippet actions()}
      <a class="text-link" href="/admin/status">{t.adminUsersServerStatus}</a>
      <a class="button secondary" href={queryHref(data.query.page)}>{t.adminTicketsRefresh}</a>
    {/snippet}
  </PageHeader>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}

  <Panel id="filters-title" className="filters" title={t.adminTicketsFilters} description={t.adminTicketsFiltersHelp}>
    <span class="count">{payload?.total?.toLocaleString("zh-CN") || "0"}</span>
    <form method="GET" action="/admin/tickets" class="filter-form">
      <label class="field">{t.adminTicketsStatus}<select name="status">
        <option value="" selected={data.query.status === ""}>{t.adminTicketsActive}</option>
        <option value="all" selected={data.query.status === "all"}>{t.adminTicketsAllStatuses}</option>
        {#each statusOptions as status}<option value={status} selected={data.query.status === status}>{statusLabels[status]}</option>{/each}
      </select></label>
      <label class="field">{t.ticketType}<select name="type">
        <option value="" selected={data.query.type === ""}>{t.adminTicketsAllTypes}</option>
        {#each payload?.ticket_types || [] as type}<option value={type} selected={data.query.type === type}>{type}</option>{/each}
      </select></label>
      <label class="field">{t.ticketPriority}<select name="priority">
        <option value="" selected={data.query.priority === ""}>{t.adminTicketsAllPriorities}</option>
        {#each priorityOptions as priority}<option value={priority} selected={data.query.priority === priority}>{priorityLabels[priority]}</option>{/each}
      </select></label>
      <label class="field">{t.adminTicketsUserID}<input name="uid" inputmode="numeric" value={data.query.uid || ""} placeholder={t.adminTicketsUserIDPlaceholder} /></label>
      <label class="field">{t.adminTicketsPerPage}<select name="per_page">
        <option value="20" selected={data.query.per_page === 20}>20</option>
        <option value="50" selected={data.query.per_page === 50}>50</option>
        <option value="100" selected={data.query.per_page === 100}>100</option>
      </select></label>
      <button class="button primary" type="submit">{t.adminTicketsApply}</button>
    </form>
    <form method="GET" action="/admin/tickets" class="jump-form">
      <label class="field">{t.adminTicketsJump}<input name="ticket" inputmode="numeric" required placeholder={t.adminTicketsJumpPlaceholder} /></label>
      <button class="button secondary" type="submit">{t.adminTicketsOpen}</button>
    </form>
  </Panel>

  <details class="panel type-panel">
    <summary>{t.adminTicketsManageTypes}</summary>
    <p class="muted">{t.adminTicketsManageTypesHelp}</p>
    <form method="POST" action="?/addType" class="type-add-form">
      <input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} />
      <input type="hidden" name="status" value={data.query.status} /><input type="hidden" name="type" value={data.query.type} /><input type="hidden" name="priority" value={data.query.priority} /><input type="hidden" name="uid" value={data.query.uid} />
      <label class="field grow">{t.adminTicketsNewType}<input name="name" maxlength="50" required placeholder={t.adminTicketsNewTypePlaceholder} /></label>
      <button class="button secondary" type="submit">{t.adminTicketsAddType}</button>
    </form>
    <div class="type-list" aria-label={t.adminTicketsExistingTypes}>
      {#if payload?.ticket_types?.length}
        {#each payload.ticket_types as type}
          <div class="type-row">
            <strong>{type}</strong>
            <form method="POST" action="?/renameType" class="type-action-form">
              <input type="hidden" name="old_name" value={type} />
              <input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="status" value={data.query.status} /><input type="hidden" name="type" value={data.query.type} /><input type="hidden" name="priority" value={data.query.priority} /><input type="hidden" name="uid" value={data.query.uid} />
              <input name="new_name" maxlength="50" required placeholder={t.adminTicketsRenameTypePlaceholder} aria-label={`${t.adminTicketsRenameType}: ${type}`} />
              <button class="button compact secondary" type="submit">{t.adminTicketsRenameType}</button>
            </form>
            <form method="POST" action="?/deleteType" class="type-action-form" onsubmit={(event) => confirmSubmit(event, t.adminTicketsDeleteTypeConfirm.replace("{name}", type))}>
              <input type="hidden" name="name" value={type} /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="status" value={data.query.status} /><input type="hidden" name="type" value={data.query.type} /><input type="hidden" name="priority" value={data.query.priority} /><input type="hidden" name="uid" value={data.query.uid} />
              <button class="button compact warning" type="submit">{t.adminTicketsDeleteType}</button>
            </form>
          </div>
        {/each}
      {:else}<p class="muted">{t.adminTicketsNoTypes}</p>{/if}
    </div>
  </details>

  {#if payload}
    <Panel id="list-title" className="list-panel" title={t.adminTicketsQueue} description={t.adminTicketsPageOf.replace("{page}", String(payload.page)).replace("{pages}", String(Math.max(1, Math.ceil(payload.total / Math.max(1, payload.per_page))))).replace("{total}", payload.total.toLocaleString("zh-CN"))}>
      {#if payload.tickets.length === 0}
        <p class="empty">{t.adminTicketsNoTickets}</p>
      {:else}
        <div class="ticket-list" role="list">
          {#each payload.tickets as ticket (ticket.id)}
            <article class:closed={ticket.status === "closed"} class="ticket-card" role="listitem">
              <div class="ticket-main">
                <div class="ticket-badges">
                  <span class={`status ${ticket.status}`}>{statusLabel(ticket.status)}</span>
                  <span class={`priority ${ticket.priority}`}>{priorityLabel(ticket.priority)}</span>
                  {#if ticket.type}<span class="tag">{ticket.type}</span>{/if}
                  <span class="ticket-id">#{ticket.id}</span>
                </div>
                <a class="ticket-title" href={`/admin/tickets/${ticket.id}`}>{ticket.title}</a>
                <div class="ticket-meta"><span>{ticket.username} · UID {ticket.uid}</span><span>{t.adminTicketsUpdatedAt.replace("{date}", dateLabel(ticket.updated_at))}</span></div>
                <div class="ticket-counts"><span>{t.adminTicketsReplies.replace("{count}", String(ticket.reply_count))}</span><span>{t.adminTicketsAttachments.replace("{count}", String(ticket.attachment_count))}</span>{#if ticket.admin_note}<span>{t.adminTicketsHasNote}</span>{/if}</div>
              </div>
              <div class="ticket-actions">
                <a class="button primary" href={`/admin/tickets/${ticket.id}`}>{t.adminTicketsOpen}</a>
                <details class="quick-actions">
                  <summary>{t.adminTicketsQuickActions}</summary>
                  <div class="quick-action-list">
                    {#each statusOptions as nextStatus}
                      {#if nextStatus !== ticket.status}
                        <form method="POST" action="?/quickStatus" onsubmit={(event) => confirmSubmit(event, t.adminTicketsStatusConfirm.replace("{status}", statusLabel(nextStatus)))}>
                          <input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="status" value={nextStatus} />
                          <input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="filter_status" value={data.query.status} /><input type="hidden" name="type" value={data.query.type} /><input type="hidden" name="priority" value={data.query.priority} /><input type="hidden" name="uid" value={data.query.uid} />
                          <button class="button compact secondary" type="submit">{statusActionLabel(nextStatus)}</button>
                        </form>
                      {/if}
                    {/each}
                    <form method="POST" action="?/delete" onsubmit={(event) => confirmSubmit(event, t.adminTicketsDeleteConfirm.replace("{id}", String(ticket.id)))}>
                      <input type="hidden" name="ticket_id" value={ticket.id} /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="status" value={data.query.status} /><input type="hidden" name="type" value={data.query.type} /><input type="hidden" name="priority" value={data.query.priority} /><input type="hidden" name="uid" value={data.query.uid} />
                      <button class="button compact danger" type="submit">{t.adminTicketsDelete}</button>
                    </form>
                  </div>
                </details>
              </div>
            </article>
          {/each}
        </div>
      {/if}
      {#if Math.ceil(payload.total / Math.max(1, payload.per_page)) > 1}
        <nav class="pagination" aria-label={t.adminTicketsPagination}>
          {#if payload.page > 1}<a class="button secondary" href={queryHref(payload.page - 1)}>{t.adminTicketsPrevious}</a>{:else}<span></span>{/if}
          <span>{payload.page} / {totalPages}</span>
          {#if payload.page < totalPages}<a class="button secondary" href={queryHref(payload.page + 1)}>{t.adminTicketsNext}</a>{:else}<span></span>{/if}
        </nav>
      {/if}
    </Panel>
  {/if}
</section>

<style>
  .tickets-page { display: grid; gap: 1rem; min-width: 0; }
  .ticket-badges, .ticket-meta, .ticket-counts, .ticket-actions, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .ticket-card, .pagination { justify-content: space-between; }
  p { overflow-wrap: anywhere; }
  .muted { color: #52606d; margin: .4rem 0 0; } .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; max-width: 100%; min-height: 2.5rem; padding: .5rem .85rem; text-decoration: none; white-space: normal; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; }
  .button.warning { background: #fff0d2; color: #7b4f00; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; }
  .notice { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .count { background: #eef2f4; border-radius: 999px; color: #486581; flex: 0 0 auto; font-size: .8rem; padding: .25rem .55rem; }
  .filter-form { align-items: end; display: grid; gap: .75rem; grid-template-columns: repeat(6, minmax(0, 1fr)); } .field { color: #243b53; display: grid; font-size: .85rem; font-weight: 650; gap: .35rem; min-width: 0; } .field.grow { flex: 1; }
  input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } input:focus, select:focus, button:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .jump-form { align-items: end; display: flex; gap: .75rem; margin-top: .85rem; max-width: 30rem; } .jump-form .field { flex: 1; }
  .type-panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: .75rem; min-width: 0; padding: 1rem; } .type-panel > summary { cursor: pointer; font-weight: 700; } .type-add-form { align-items: end; display: flex; gap: .75rem; } .type-list { display: grid; gap: .5rem; max-height: 18rem; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .type-row { align-items: center; border-top: 1px solid #e1e8ed; display: flex; flex-wrap: wrap; gap: .5rem; min-width: 0; padding-top: .5rem; } .type-row > strong { min-width: 8rem; overflow-wrap: anywhere; } .type-action-form { align-items: end; display: flex; flex: 1 1 16rem; gap: .4rem; min-width: 0; } .type-action-form input { flex: 1; min-width: 8rem; }
  .ticket-list { display: grid; gap: .7rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding: .1rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .ticket-card { align-items: flex-start; border: 1px solid #c8d2da; border-left: 4px solid #9fb3c8; display: flex; gap: 1rem; min-width: 0; padding: .9rem; } .ticket-card.closed { opacity: .75; } .ticket-main { display: grid; gap: .5rem; min-width: 0; } .ticket-badges { align-items: center; flex-wrap: wrap; } .status, .priority, .tag, .ticket-id { border: 1px solid #c8d2da; border-radius: 999px; font-size: .74rem; padding: .2rem .5rem; white-space: nowrap; } .status.open { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .status.in_progress { background: #edf5f8; border-color: #a9c8d5; color: #245b75; } .status.resolved { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .status.closed { background: #eef2f4; color: #52606d; } .priority { background: #f4f6f8; color: #52606d; } .priority.high { background: #fff8e6; color: #7b4f00; } .priority.urgent { background: #fff1f0; color: #a61b1b; } .tag { background: #f4f7f8; color: #16394a; } .ticket-id { color: #52606d; font-family: ui-monospace, monospace; }
  .ticket-title { color: #16394a; font-size: 1.08rem; font-weight: 700; overflow-wrap: anywhere; text-decoration: none; } .ticket-title:hover { text-decoration: underline; } .ticket-meta, .ticket-counts { color: #52606d; flex-wrap: wrap; font-size: .78rem; } .ticket-counts { color: #7b8794; }
  .ticket-actions { align-items: stretch; flex: 0 0 auto; flex-direction: column; } .quick-actions { border-top: 1px solid #e1e8ed; padding-top: .35rem; } .quick-actions summary { color: #245b75; cursor: pointer; font-size: .8rem; font-weight: 650; padding: .3rem 0; } .quick-action-list { display: grid; gap: .35rem; margin-top: .35rem; } .quick-action-list form, .quick-action-list .button { width: 100%; }
  .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .75rem; } .empty { color: #52606d; padding: 2rem 1rem; text-align: center; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: repeat(3, minmax(0, 1fr)); } .ticket-card { flex-direction: column; } .ticket-actions { align-items: stretch; flex-direction: row; flex-wrap: wrap; width: 100%; } .ticket-actions > .button { flex: 1 1 12rem; } .quick-actions { flex: 1 1 12rem; } }
  @media (max-width: 600px) { .ticket-card, .type-add-form, .jump-form, .pagination { align-items: stretch; flex-direction: column; } .filter-form { grid-template-columns: 1fr; } .filter-form .button, .jump-form .button, .type-add-form .button { width: 100%; } .type-row { align-items: stretch; flex-direction: column; } .type-row > strong { min-width: 0; } .type-action-form { flex-basis: auto; width: 100%; } .type-action-form .button { flex: 0 0 auto; } .ticket-actions, .ticket-actions > .button, .quick-actions { width: 100%; } .pagination { text-align: center; } .pagination .button, .type-panel { width: 100%; } .type-panel { padding: .85rem; } .notice { padding: .85rem; } }
</style>
