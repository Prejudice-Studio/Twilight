<script lang="ts">
  import PageHeader from "$lib/components/PageHeader.svelte";
  import Panel from "$lib/components/Panel.svelte";
  import { t } from "$lib/i18n";
  import type { AdminUserListResponse, UserInfo } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; success?: boolean; error?: string; created?: { username: string; password: string } };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let payload = $derived(data.payload as AdminUserListResponse | null);

  function roleLabel(user: UserInfo): string {
    if (user.role === 0) return t.adminUsersAdmin;
    if (user.role === 2) return t.adminUsersWhitelist;
    if (user.role === 3) return t.adminUsersUnknownRole;
    return t.adminUsersNormal;
  }

  function dateLabel(value?: number): string {
    if (value === -1) return t.adminUsersSetPermanent;
    if (!value || value < 0) return t.adminUsersNotSet;
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function embyLabel(user: UserInfo): string {
    if (!user.emby_id) return t.adminUsersUnbound;
    if (isEmbyDisabled(user)) return t.adminUsersDisabled;
    return t.adminUsersEnabled;
  }

  function isEmbyDisabled(user: UserInfo): boolean {
    return Boolean(user.emby_disabled || user.emby_disabled_by_expiry);
  }

  function queryForPage(page: number): string {
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(data.query.per_page));
    for (const name of ["search", "role", "active", "emby", "emby_status", "email_status", "sort"] as const) {
      if (data.query[name]) params.set(name, data.query[name]);
    }
    return `/admin/users?${params.toString()}`;
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }
</script>

<svelte:head><title>{t.adminUsersTitle} - {t.siteName}</title></svelte:head>

<section class="users-page" aria-labelledby="users-title">
  <PageHeader id="users-title" eyebrow={t.adminArea} title={t.adminUsersTitle} description={t.adminUsersDescription}>
    {#snippet actions()}
      <a class="text-link" href="/admin/status">{t.adminUsersServerStatus}</a>
      <a class="button secondary" href={queryForPage(data.query.page)}>{t.adminUsersRefresh}</a>
    {/snippet}
  </PageHeader>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if action.success && action.created}
    <section class="notice success" role="status">
      <strong>{t.adminUsersCreateSuccess.replace("{username}", action.created.username)}</strong>
      <p>{t.adminUsersTemporaryPassword}</p>
      <code>{action.created.password}</code>
    </section>
  {/if}

  <Panel id="create-title" className="create-panel" title={t.adminUsersCreate}>
    <details>
      <summary>{t.adminUsersCreate}</summary>
      <form method="POST" action="?/create" class="create-form">
        <label>{t.adminUsersUsername}<input name="username" maxlength="64" autocomplete="off" required /></label>
        <label>{t.adminUsersPasswordOptional}<input name="password" type="password" maxlength="256" autocomplete="new-password" /></label>
        <label>{t.adminUsersEmailOptional}<input name="email" type="email" maxlength="254" autocomplete="email" /></label>
        <label>{t.adminUsersRole}<select name="role"><option value="1">{t.adminUsersNormal}</option><option value="2">{t.adminUsersWhitelist}</option><option value="0">{t.adminUsersAdmin}</option></select></label>
        <label>{t.adminUsersDays}<input name="days" type="number" min="-1" max="36500" value="30" required /></label>
        <button class="button primary" type="submit">{t.adminUsersCreate}</button>
      </form>
    </details>
  </Panel>

  <Panel id="filter-title" className="filters" title={t.adminUsersFilters}>
    <form method="GET" action="/admin/users" class="filter-form">
      <label class="search-field">{t.adminUsersSearch}<input name="search" value={data.query.search} maxlength="100" placeholder={t.adminUsersSearchPlaceholder} /></label>
      <label>{t.adminUsersRole}<select name="role"><option value="" selected={data.query.role === ""}>{t.adminUsersAllRoles}</option><option value="0" selected={data.query.role === "0"}>{t.adminUsersAdmin}</option><option value="1" selected={data.query.role === "1"}>{t.adminUsersNormal}</option><option value="2" selected={data.query.role === "2"}>{t.adminUsersWhitelist}</option></select></label>
      <label>{t.adminUsersWebStatus}<select name="active"><option value="" selected={data.query.active === ""}>{t.adminUsersAllStatuses}</option><option value="true" selected={data.query.active === "true"}>{t.adminUsersEnabled}</option><option value="false" selected={data.query.active === "false"}>{t.adminUsersDisabled}</option></select></label>
      <label>{t.adminUsersEmby}<select name="emby"><option value="" selected={data.query.emby === ""}>{t.adminUsersAll}</option><option value="bound" selected={data.query.emby === "bound"}>{t.adminUsersBound}</option><option value="unbound" selected={data.query.emby === "unbound"}>{t.adminUsersUnbound}</option></select></label>
      <label>{t.adminUsersEmbyStatus}<select name="emby_status"><option value="" selected={data.query.emby_status === ""}>{t.adminUsersAll}</option><option value="active" selected={data.query.emby_status === "active"}>{t.adminUsersEnabled}</option><option value="disabled" selected={data.query.emby_status === "disabled"}>{t.adminUsersDisabled}</option></select></label>
      <label>{t.adminUsersEmail}<select name="email_status"><option value="" selected={data.query.email_status === ""}>{t.adminUsersAll}</option><option value="verified" selected={data.query.email_status === "verified"}>{t.adminUsersVerified}</option><option value="unverified" selected={data.query.email_status === "unverified"}>{t.adminUsersUnverified}</option><option value="bound" selected={data.query.email_status === "bound"}>{t.adminUsersEmailBound}</option><option value="none" selected={data.query.email_status === "none"}>{t.adminUsersEmailNone}</option></select></label>
      <label>{t.adminUsersSort}<select name="sort"><option value="uid_asc" selected={data.query.sort === "uid_asc"}>{t.adminUsersUIDAsc}</option><option value="uid_desc" selected={data.query.sort === "uid_desc"}>{t.adminUsersUIDDesc}</option><option value="username_asc" selected={data.query.sort === "username_asc"}>{t.adminUsersUsernameAsc}</option><option value="username_desc" selected={data.query.sort === "username_desc"}>{t.adminUsersUsernameDesc}</option><option value="expire_asc" selected={data.query.sort === "expire_asc"}>{t.adminUsersExpiryAsc}</option><option value="expire_desc" selected={data.query.sort === "expire_desc"}>{t.adminUsersExpiryDesc}</option></select></label>
      <label>{t.adminUsersPerPage}<select name="per_page"><option value="20" selected={data.query.per_page === 20}>20</option><option value="50" selected={data.query.per_page === 50}>50</option><option value="100" selected={data.query.per_page === 100}>100</option></select></label>
      <button class="button primary" type="submit">{t.adminUsersApply}</button>
    </form>
  </Panel>

  {#if payload}
    <Panel id="list-title" className="list-panel" title={t.adminUsersList} description={t.adminUsersPageOf.replace("{page}", String(payload.page)).replace("{pages}", String(Math.max(payload.pages, 1))).replace("{total}", payload.total.toLocaleString("zh-CN"))}>
      {#if payload.users.length === 0}
        <p class="empty">{t.adminUsersEmpty}</p>
      {:else}
        <div class="user-list" role="list">
          {#each payload.users as user (user.uid)}
            {@const state = user.admin_action_state}
            <article class="user-card" role="listitem">
              <div class="user-summary">
                <div class="identity"><strong>{user.username}</strong><span>{t.adminUsersUID.replace("{uid}", String(user.uid))}</span></div>
                <div class="badges"><span class="badge">{roleLabel(user)}</span><span class:good={user.active} class:danger={!user.active} class="badge">{t.adminUsersWebBadge.replace("{value}", user.active ? t.adminUsersEnabled : t.adminUsersDisabled)}</span><span class:good={Boolean(user.emby_id) && !isEmbyDisabled(user)} class:danger={Boolean(user.emby_id && isEmbyDisabled(user))} class="badge">{t.adminUsersEmbyBadge.replace("{value}", embyLabel(user))}</span></div>
              </div>
              <dl class="facts">
                <div><dt>{t.adminUsersExpiry}</dt><dd>{dateLabel(user.expired_at)}</dd></div>
                <div><dt>{t.adminUsersEmail}</dt><dd>{user.email ? `${user.email}${user.email_verified ? `（${t.adminUsersVerified}）` : `（${t.adminUsersUnverified}）`}` : t.adminUsersNotSet}</dd></div>
                <div><dt>{t.adminUsersTelegram}</dt><dd>{user.telegram_id ? t.adminUsersBoundAs.replace("{value}", user.telegram_username ? ` @${user.telegram_username}` : "") : t.adminUsersUnbound}</dd></div>
                <div><dt>{t.adminUsersEmbyUser}</dt><dd>{user.emby_username || user.emby_id || t.adminUsersUnbound}</dd></div>
              </dl>
              <details class="actions">
                <summary>{t.adminUsersManage}</summary>
                <div class="action-groups">
                  <section><h3>{t.adminUsersAccount}</h3><div class="action-row">
                    <form method="POST" action="?/toggle" onsubmit={(event) => confirmSubmit(event, user.active ? t.adminUsersConfirmDisable.replace("{username}", user.username) : t.adminUsersConfirmEnable.replace("{username}", user.username))}>
                      <input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="enable" value={String(!user.active)} /><input type="hidden" name="cascade_depth" value="1" />
                      {#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
                      <button class="button secondary" type="submit">{user.active ? t.adminUsersDisableWeb : t.adminUsersEnableWeb}</button>
                    </form>
                    <form method="POST" action="?/renew" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmRenew.replace("{username}", user.username))}>
                      <input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="days" value="30" />
                      {#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
                      <button class="button secondary" type="submit">{t.adminUsersRenew30}</button>
                    </form>
                    <form method="POST" action="?/renew" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmPermanent.replace("{username}", user.username))}>
                      <input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="days" value="-1" />
                      {#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
                      <button class="button secondary" type="submit">{t.adminUsersSetPermanent}</button>
                    </form>
                  </div></section>

                  <section><h3>{t.adminUsersEmbyGroup}</h3><div class="action-row">
                    {#if user.emby_id}
                      <form method="POST" action="?/embyToggle" onsubmit={(event) => confirmSubmit(event, isEmbyDisabled(user) ? t.adminUsersConfirmEmbyEnable.replace("{username}", user.username) : t.adminUsersConfirmEmbyDisable.replace("{username}", user.username))}>
                        <input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="enable" value={String(isEmbyDisabled(user))} />
                        {#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
                        <button class="button secondary" type="submit" disabled={isEmbyDisabled(user) ? !state?.can_enable_emby : !state?.can_disable_emby}>{isEmbyDisabled(user) ? t.adminUsersEnableEmby : t.adminUsersDisableEmby}</button>
                      </form>
                      <form method="POST" action="?/unbindEmby" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmUnbindEmby.replace("{username}", user.username))}>
                        <input type="hidden" name="uid" value={user.uid} />{#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}
                        <button class="button warning" type="submit">{t.adminUsersUnbindEmby}</button>
                      </form>
                    {:else}<span class="muted">{t.adminUsersNoEmbyBinding}</span>{/if}
                  </div></section>

                  <section><h3>{t.adminUsersIdentity}</h3><div class="action-row">
                    <form method="POST" action="?/refresh"><input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="scope" value="both" />{#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button secondary" type="submit">{t.adminUsersRefreshExternal}</button></form>
                    {#if user.telegram_id}<form method="POST" action="?/unbindTelegram" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmUnbindTelegram.replace("{username}", user.username))}><input type="hidden" name="uid" value={user.uid} />{#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button warning" type="submit">{t.adminUsersUnbindTelegram}</button></form>{/if}
                  </div></section>

                  <section><h3>{t.adminUsersPermissionsDanger}</h3><div class="action-row">
                    {#if user.role === 0}
                      <form method="POST" action="?/role" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmRemoveAdmin.replace("{username}", user.username))}><input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="admin" value="false" />{#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button warning" type="submit">{t.adminUsersRemoveAdmin}</button></form>
                    {:else if !state?.protected_role}
                      <form method="POST" action="?/role" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmMakeAdmin.replace("{username}", user.username))}><input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="admin" value="true" />{#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button warning" type="submit">{t.adminUsersMakeAdmin}</button></form>
                    {/if}
                    {#if state?.can_delete !== false}
                      <form method="POST" action="?/delete" onsubmit={(event) => confirmSubmit(event, t.adminUsersConfirmDelete.replace("{username}", user.username))}><input type="hidden" name="uid" value={user.uid} /><input type="hidden" name="mode" value={user.emby_id ? "with_emby" : "local_only"} /><input type="hidden" name="cascade_depth" value="1" />{#each Object.entries(data.query) as [name, value]}<input type="hidden" name={name} value={value} />{/each}<button class="button danger" type="submit">{t.adminUsersDelete}</button></form>
                    {:else}<span class="muted">{t.adminUsersProtected}</span>{/if}
                  </div></section>
                </div>
              </details>
            </article>
          {/each}
        </div>
      {/if}
      {#if payload.pages > 1}
        <nav class="pagination" aria-label={t.adminUsersPagination}>
          {#if payload.page > 1}<a class="button secondary" href={queryForPage(payload.page - 1)}>{t.adminUsersPrevious}</a>{:else}<span></span>{/if}
          <span>{t.adminUsersPageSimple.replace("{page}", String(payload.page)).replace("{pages}", String(payload.pages))}</span>
          {#if payload.page < payload.pages}<a class="button secondary" href={queryForPage(payload.page + 1)}>{t.adminUsersNext}</a>{:else}<span></span>{/if}
        </nav>
      {/if}
    </Panel>
  {/if}
</section>

<style>
  .users-page { display: grid; gap: 1rem; min-width: 0; }
  .user-summary, .action-row, .pagination { align-items: flex-start; display: flex; gap: .75rem; }
  .user-summary, .pagination { justify-content: space-between; }
  h3, p { overflow-wrap: anywhere; } h3 { margin: 0; font-size: .9rem; }
  .muted { color: #52606d; margin: .4rem 0 0; } .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; } .button.primary { background: #245b75; color: #fff; } .button.secondary { background: #e8eef2; color: #16394a; } .button.warning { background: #fff0d2; color: #7b4f00; } .button.danger { background: #a63d40; color: #fff; } .button:disabled { cursor: not-allowed; opacity: .5; }
  .notice { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; min-width: 0; padding: 1rem; } .notice { margin: 0; } .notice p { margin: .4rem 0 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; } code { background: #f4f6f8; border: 1px solid #d7dee5; border-radius: .25rem; display: inline-block; margin-top: .5rem; max-width: 100%; overflow-wrap: anywhere; padding: .3rem .45rem; }
  summary { cursor: pointer; font-weight: 700; overflow-wrap: anywhere; } .create-form, .filter-form { display: grid; gap: .75rem; grid-template-columns: repeat(3, minmax(0, 1fr)); margin-top: 1rem; } label { color: #243b53; display: grid; font-size: .85rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } .create-form .button, .filter-form .button { align-self: end; justify-self: start; }
  .search-field { grid-column: span 2; } .user-list { display: grid; gap: .75rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .user-card { border: 1px solid #d7dee5; display: grid; gap: .8rem; min-width: 0; padding: .9rem; } .identity { display: grid; gap: .2rem; min-width: 0; } .identity strong { font-size: 1.05rem; overflow-wrap: anywhere; } .identity span, dt { color: #52606d; font-size: .8rem; } .badges, .action-row { flex-wrap: wrap; } .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; font-size: .72rem; padding: .2rem .48rem; white-space: nowrap; } .badge.good { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .badge.danger { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .facts { display: grid; gap: .45rem; grid-template-columns: repeat(4, minmax(0, 1fr)); margin: 0; } .facts div { min-width: 0; } dd { font-weight: 650; margin: .2rem 0 0; overflow-wrap: anywhere; } .actions { border-top: 1px solid #e1e8ed; padding-top: .75rem; } .action-groups { display: grid; gap: .8rem; margin-top: .75rem; } .action-groups section { border-left: 3px solid #9fb3c8; display: grid; gap: .5rem; min-width: 0; padding-left: .7rem; } .action-row form { display: flex; max-width: 100%; } .action-row .button { white-space: normal; }
  .empty { color: #52606d; margin: 0; padding: 1.5rem; text-align: center; } .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination > span { color: #52606d; font-size: .85rem; }
  button:focus-visible, a:focus-visible, input:focus-visible, select:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 900px) { .create-form, .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .search-field { grid-column: 1 / -1; } .facts { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 600px) { .user-summary, .pagination { align-items: stretch; flex-direction: column; } .create-form, .filter-form, .facts { grid-template-columns: 1fr; } .search-field { grid-column: auto; } .create-form .button, .filter-form .button { justify-self: stretch; width: 100%; } .badges { justify-content: flex-start; } .user-list { max-height: 68dvh; } .notice { padding: .85rem; } .pagination { align-items: stretch; text-align: center; } .pagination .button { width: 100%; } }
</style>
