<script lang="ts">
  import { t } from "$lib/i18n";
  import type { AdminEmbyActivityLog, AdminEmbyAuditUser, AdminEmbyDevice, AdminEmbyUser, EmbyConnectivityResult } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = {
    action?: string;
    success?: boolean;
    error?: string;
    connectivity?: EmbyConnectivityResult;
    activity?: PageData["activity"];
    broadcast?: { sent_count: number; failed?: Array<{ session_id?: string; error?: string }> };
    standalone?: { emby_id: string; emby_username: string };
    passwordReset?: { emby_id: string; emby_username: string; linked_local_user: boolean; new_password: string };
  };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let activity = $derived(action.activity || data.activity);

  function connectivityName(name: string): string {
    if (name === "configuration_url") return t.adminEmbyCheckURL;
    if (name === "configuration_token") return t.adminEmbyCheckToken;
    if (name === "backend_server_info") return t.adminEmbyCheckServer;
    if (name === "backend_users") return t.adminEmbyCheckUsers;
    if (name === "backend_libraries") return t.adminEmbyCheckLibraries;
    if (name.startsWith("backend_local_")) return `${t.adminEmbyCheckLocal} ${name.slice("backend_local_".length)}`;
    return t.adminEmbyUnknown;
  }

  function dateLabel(value: string | number | null | undefined): string {
    if (!value) return "-";
    const date = typeof value === "number" ? new Date(value * 1000) : new Date(value);
    return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString("zh-CN");
  }

  function queryHref(tab: string, values: Record<string, string | number> = {}): string {
    const params = new URLSearchParams({ tab });
    if (tab === "accounts") {
      params.set("page", String(values.page || data.query.page));
      params.set("per_page", String(data.query.per_page));
      for (const name of ["search", "link", "attribute"] as const) if (data.query[name]) params.set(name, data.query[name]);
      params.set("orphan_page", String(values.orphan_page || data.query.orphan_page));
      params.set("orphan_per_page", String(data.query.orphan_per_page));
    }
    if (tab === "devices") {
      params.set("device_page", String(values.device_page || data.query.device_page));
      params.set("device_per_page", String(data.query.device_per_page));
      if (data.query.device_search) params.set("device_search", data.query.device_search);
      if (values.refresh === 1) params.set("refresh", "1");
    }
    return `/admin/emby?${params}`;
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function userStatus(user: AdminEmbyUser): string {
    if (user.sync_status === "synced") return t.adminEmbySynced;
    if (user.sync_status === "name_mismatch") return t.adminEmbyMismatchStatus;
    return t.adminEmbyUnlinkedStatus;
  }

  function activityType(log: AdminEmbyActivityLog): string {
    return log.type || t.adminEmbyUnknown;
  }

  function deviceStatus(device: AdminEmbyDevice): string {
    return device.online ? t.adminEmbyOnline : t.adminEmbyOffline;
  }

  function deviceUserName(user: AdminEmbyAuditUser): string {
    return user.emby_user_name || user.local_user?.username || t.adminEmbyUnknown;
  }
</script>

<svelte:head><title>{t.adminEmbyTitle} - {t.siteName}</title></svelte:head>

<section class="emby-page" aria-labelledby="emby-title">
  <header class="page-heading">
    <div><p class="eyebrow">{t.adminArea}</p><h1 id="emby-title">{t.adminEmbyTitle}</h1><p class="muted">{t.adminEmbyDescription}</p></div>
    <div class="heading-actions"><a class="text-link" href="/admin/status">{t.adminEmbyStatus}</a><a class="button secondary" href={queryHref(data.tab)}>{t.adminEmbyRefresh}</a></div>
  </header>

  {#if data.errors.length}<div class="notice error" role="alert">{data.errors.join("；")}</div>{/if}
  {#if action.error}<div class="notice error" role="alert">{action.error}</div>{/if}

  <nav class="tabs" aria-label={t.adminEmbyTitle}>
    <a class:active={data.tab === "accounts"} href={queryHref("accounts")}>{t.adminEmbyAccounts}</a>
    <a class:active={data.tab === "devices"} href={queryHref("devices")}>{t.adminEmbyDevices}</a>
    <a class:active={data.tab === "activity"} href={queryHref("activity")}>{t.adminEmbyActivity}</a>
  </nav>

  {#if data.tab === "accounts"}
     <section class="panel" aria-labelledby="connectivity-title">
       <header class="panel-heading"><div><h2 id="connectivity-title">{t.adminEmbyConnectivity}</h2><p class="muted">{t.adminEmbyConnectivityHelp}</p></div><form method="POST" action="?/testConnectivity"><button class="button primary" type="submit">{t.adminEmbyTest}</button></form></header>
      {#if action.connectivity}
        <div class="connectivity-result">
          <strong class:good={action.connectivity.overall} class:bad={!action.connectivity.overall}>{action.connectivity.overall ? t.adminEmbyTestPassed : t.adminEmbyTestFailed}</strong>
          {#if action.connectivity.server_info}<dl class="facts compact"><div><dt>{t.adminEmbyServerName}</dt><dd>{action.connectivity.server_info.name || "-"}</dd></div><div><dt>{t.adminEmbyVersion}</dt><dd>{action.connectivity.server_info.version || "-"}</dd></div><div><dt>{t.adminEmbyOperatingSystem}</dt><dd>{action.connectivity.server_info.os || "-"}</dd></div></dl>{/if}
          <div class="test-list">{#each action.connectivity.tests as test}<div class="test-row"><span class:good={test.success} class:bad={!test.success}>{test.success ? "✓" : "×"}</span><strong>{connectivityName(test.name)}</strong><span>{test.message}</span>{#if test.latency_ms !== undefined}<small>{t.adminEmbyLatencyValue.replace("{value}", String(test.latency_ms))}</small>{/if}</div>{/each}</div>
        </div>
      {/if}
    </section>

    <section class="panel" aria-labelledby="users-title">
      <header class="panel-heading"><div><h2 id="users-title">{t.adminEmbyUsers}</h2><p class="muted">{t.adminEmbyUsersHelp}</p></div><div class="toolbar"><form method="POST" action="?/sync"><button class="button secondary" type="submit">{t.adminEmbySync}</button></form><form method="POST" action="?/importUsers"><button class="button secondary" type="submit">{t.adminEmbyImport}</button></form></div></header>
      <form method="GET" action="/admin/emby" class="filters"><input type="hidden" name="tab" value="accounts" /><label>{t.adminEmbySearch}<input name="search" value={data.query.search} maxlength="100" placeholder={t.adminEmbySearchPlaceholder} /></label><label>{t.adminEmbyLinkFilter}<select name="link"><option value="">{t.adminEmbyAll}</option><option value="linked" selected={data.query.link === "linked"}>{t.adminEmbyLinkedOnly}</option><option value="unlinked" selected={data.query.link === "unlinked"}>{t.adminEmbyUnlinkedOnly}</option><option value="name_mismatch" selected={data.query.link === "name_mismatch"}>{t.adminEmbyNameMismatch}</option></select></label><label>{t.adminEmbyAttribute}<select name="attribute"><option value="">{t.adminEmbyAll}</option><option value="admin" selected={data.query.attribute === "admin"}>{t.adminEmbyAdministrators}</option><option value="disabled" selected={data.query.attribute === "disabled"}>{t.adminEmbyDisabledOnly}</option><option value="hidden" selected={data.query.attribute === "hidden"}>{t.adminEmbyHiddenOnly}</option></select></label><label>{t.adminEmbyUsers}<select name="per_page"><option value="20" selected={data.query.per_page === 20}>20</option><option value="50" selected={data.query.per_page === 50}>50</option><option value="100" selected={data.query.per_page === 100}>100</option><option value="200" selected={data.query.per_page === 200}>200</option></select></label><button class="button primary" type="submit">{t.adminEmbyApply}</button></form>
      {#if data.users}
        <p class="result-meta">{t.adminEmbyPageOf.replace("{page}", String(data.users.page || data.query.page)).replace("{pages}", String(Math.max(1, data.users.pages || 1))).replace("{total}", String(data.users.total || 0))}</p>
        {#if data.users.emby_users.length}<div class="table-region"><table><thead><tr><th>{t.adminEmbyUsers}</th><th>{t.adminEmbyLocalAccount}</th><th>{t.adminEmbyAttribute}</th><th>{t.adminEmbyLastActivity}</th><th>{t.adminEmbyActions}</th></tr></thead><tbody>{#each data.users.emby_users as user (user.emby_id)}<tr><td><strong>{user.emby_name || t.adminEmbyUnknown}</strong><small>{user.emby_id}</small><span class="badge">{userStatus(user)}</span></td><td>{#if user.local_user}<strong>{user.local_user.username}</strong><small>UID {user.local_user.uid}</small>{:else}<span class="muted">{t.adminEmbyNoBinding}</span>{/if}</td><td><div class="badges">{#if user.is_admin}<span class="badge info">{t.adminEmbyAccountAdmin}</span>{/if}{#if user.is_disabled}<span class="badge danger">{t.adminEmbyAccountDisabled}</span>{/if}{#if user.is_hidden}<span class="badge">{t.adminEmbyAccountHidden}</span>{/if}</div></td><td>{dateLabel(user.last_activity)}</td><td><div class="row-actions"><form method="POST" action="?/setEmbyEnabled" onsubmit={(event) => confirmSubmit(event, user.is_disabled ? t.adminEmbyEnableConfirm : t.adminEmbyDisableConfirm)}><input type="hidden" name="emby_id" value={user.emby_id} /><input type="hidden" name="enabled" value={user.is_disabled ? "true" : "false"} /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="link" value={data.query.link} /><input type="hidden" name="attribute" value={data.query.attribute} /><button class="button small-button" type="submit">{user.is_disabled ? t.adminEmbyEnable : t.adminEmbyDisable}</button></form><form method="POST" action="?/kickEmby" onsubmit={(event) => confirmSubmit(event, t.adminEmbyKickConfirm)}><input type="hidden" name="emby_id" value={user.emby_id} /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="link" value={data.query.link} /><input type="hidden" name="attribute" value={data.query.attribute} /><button class="button small-button" type="submit">{t.adminEmbyKick}</button></form></div></td></tr>{/each}</tbody></table></div>{:else}<p class="empty">{t.adminEmbyNoUsers}</p>{/if}
        {#if (data.users.pages || 0) > 1}<nav class="pagination" aria-label={t.adminEmbyUsers}>{#if (data.users.page || 1) > 1}<a class="button secondary" href={queryHref("accounts", { page: (data.users.page || 1) - 1 })}>{t.adminEmbyPrevious}</a>{:else}<span></span>{/if}<span>{data.users.page || 1} / {data.users.pages || 1}</span>{#if (data.users.page || 1) < (data.users.pages || 1)}<a class="button secondary" href={queryHref("accounts", { page: (data.users.page || 1) + 1 })}>{t.adminEmbyNext}</a>{:else}<span></span>{/if}</nav>{/if}
      {:else}<p class="empty">{t.adminEmbyNoData}</p>{/if}
    </section>

    <section class="panel maintenance" aria-labelledby="maintenance-title"><h2 id="maintenance-title">{t.adminEmbyCleanup}</h2><p class="muted">{t.adminEmbyCleanupHelp}</p><div class="toolbar"><form method="POST" action="?/cleanup"><button class="button secondary" type="submit">{t.adminEmbyCleanup}</button></form><form method="POST" action="?/deleteUnlinked" onsubmit={(event) => confirmSubmit(event, t.adminEmbyDeleteUnlinkedConfirm)}><input type="hidden" name="confirm" value="DELETE_UNLINKED_EMBY" /> <button class="button danger" type="submit">{t.adminEmbyDeleteUnlinked}</button></form><form method="POST" action="?/reset" onsubmit={(event) => confirmSubmit(event, t.adminEmbyResetConfirm)}><input type="hidden" name="confirm" value="RESET_ALL_EMBY" /><button class="button danger" type="submit">{t.adminEmbyReset}</button></form></div><p class="muted small">{t.adminEmbyResetHelp}</p></section>
    <section class="panel" aria-labelledby="tools-title">
      <header class="panel-heading"><div><h2 id="tools-title">{t.adminEmbyTools}</h2><p class="muted">{t.adminEmbyToolsHelp}</p></div></header>
      <div class="tool-grid">
        <form method="POST" action="?/broadcast" class="tool-form" onsubmit={(event) => confirmSubmit(event, t.adminEmbyBroadcastConfirm)}>
          <h3>{t.adminEmbyBroadcast}</h3>
          <label>{t.adminEmbyBroadcastHeader}<input name="header" maxlength="80" value={t.adminEmbyBroadcastDefaultHeader} /></label>
          <label>{t.adminEmbyBroadcastText}<textarea name="text" maxlength="2000" required rows="4" placeholder={t.adminEmbyBroadcastPlaceholder}></textarea></label>
          <button class="button secondary" type="submit">{t.adminEmbyBroadcastSend}</button>
        </form>
        <form method="POST" action="?/createStandalone" class="tool-form" onsubmit={(event) => confirmSubmit(event, t.adminEmbyStandaloneConfirm)}>
          <h3>{t.adminEmbyStandalone}</h3>
          <label>{t.adminEmbyUsernameLabel}<input name="username" maxlength="64" required autocomplete="off" /></label>
          <label>{t.adminEmbyPasswordLabel}<input name="password" type="password" minlength="8" maxlength="256" required autocomplete="new-password" /></label>
          <button class="button secondary" type="submit">{t.adminEmbyStandaloneCreate}</button>
        </form>
        <form method="POST" action="?/forceSetPassword" class="tool-form" onsubmit={(event) => confirmSubmit(event, t.adminEmbyForcePasswordConfirm)}>
          <h3>{t.adminEmbyForcePassword}</h3>
          <label>{t.adminEmbyUsernameLabel}<input name="emby_username" maxlength="64" required autocomplete="off" /></label>
          <label>{t.adminEmbyNewPasswordOptional}<input name="new_password" type="password" minlength="8" maxlength="256" autocomplete="new-password" /></label>
          <button class="button secondary" type="submit">{t.adminEmbyForcePasswordSubmit}</button>
        </form>
      </div>
      {#if action.broadcast}<p class="notice success">{t.adminEmbyBroadcastResult.replace("{sent}", String(action.broadcast.sent_count)).replace("{failed}", String(action.broadcast.failed?.length || 0))}</p>{/if}
      {#if action.standalone}<p class="notice success">{t.adminEmbyStandaloneResult.replace("{name}", action.standalone.emby_username).replace("{id}", action.standalone.emby_id)}</p>{/if}
      {#if action.passwordReset}<div class="notice success secret-result"><p>{t.adminEmbyForcePasswordResult.replace("{name}", action.passwordReset.emby_username)}</p><code>{action.passwordReset.new_password}</code></div>{/if}
    </section>
    {#if data.users && data.users.total_orphans > 0}<section class="panel" aria-labelledby="orphans-title"><header class="panel-heading"><div><h2 id="orphans-title">{t.adminEmbyOrphanTitle}</h2><p class="muted">{t.adminEmbyOrphanHelp}</p></div></header><p class="result-meta">{t.adminEmbyOrphanPageOf.replace("{page}", String(data.users.orphan_page || data.query.orphan_page)).replace("{pages}", String(Math.max(1, data.users.orphan_pages || 1))).replace("{total}", String(data.users.total_orphans))}</p>{#if data.users.orphans.length}<div class="table-region orphan-region"><table><thead><tr><th>{t.adminEmbyLocalAccount}</th><th>UID</th><th>{t.adminEmbyEmbyID}</th></tr></thead><tbody>{#each data.users.orphans as orphan (orphan.uid)}<tr><td><strong>{orphan.username}</strong>{#if orphan.telegram_id}<small>Telegram {orphan.telegram_id}</small>{/if}</td><td>{orphan.uid}</td><td><code>{orphan.emby_id}</code></td></tr>{/each}</tbody></table></div>{:else}<p class="empty">{t.adminEmbyNoOrphans}</p>{/if}{#if (data.users.orphan_pages || 0) > 1}<nav class="pagination">{#if (data.users.orphan_page || 1) > 1}<a class="button secondary" href={queryHref("accounts", { orphan_page: (data.users.orphan_page || 1) - 1 })}>{t.adminEmbyPrevious}</a>{:else}<span></span>{/if}<span>{data.users.orphan_page || 1} / {data.users.orphan_pages || 1}</span>{#if (data.users.orphan_page || 1) < (data.users.orphan_pages || 1)}<a class="button secondary" href={queryHref("accounts", { orphan_page: (data.users.orphan_page || 1) + 1 })}>{t.adminEmbyNext}</a>{:else}<span></span>{/if}</nav>{/if}</section>{/if}
  {:else if data.tab === "devices"}
     <section class="panel" aria-labelledby="devices-title"><header class="panel-heading"><div><h2 id="devices-title">{t.adminEmbyDevices}</h2><p class="muted">{t.adminEmbyDeviceDescription}</p></div><a class="button secondary" href={queryHref("devices", { refresh: 1 })}>{t.adminEmbyRefresh}</a></header>
      {#if data.deviceAudit}
         {@const summary = data.deviceAudit.summary}
         <div class="summary-grid"><div><strong>{summary.total_users}</strong><span>{t.adminEmbyDeviceUsers}</span></div><div><strong>{summary.total_devices}</strong><span>{t.adminEmbyDeviceCount}</span></div><div><strong>{summary.online_devices}</strong><span>{t.adminEmbyOnlineCount}</span></div><div><strong>{summary.total_ips}</strong><span>{t.adminEmbyIPCount}</span></div></div>
         {#if data.deviceAudit.emby_configured && summary.sessions_available === false}<p class="notice error">{summary.sessions_error || t.adminEmbySessionsUnavailable}</p>{/if}
         {#if data.deviceAudit.emby_configured && summary.devices_available === false}<p class="notice error">{summary.devices_error || t.adminEmbyDevicesUnavailable}</p>{/if}
         {#if data.deviceAudit.emby_configured && summary.activity_available === false}<p class="notice error">{summary.activity_error || t.adminEmbyActivityUnavailable}</p>{/if}
        <form method="GET" action="/admin/emby" class="filters device-filter"><input type="hidden" name="tab" value="devices" /><label>{t.adminEmbySearch}<input name="device_search" value={data.query.device_search} maxlength="100" placeholder={t.adminEmbyDeviceSearch} /></label><input type="hidden" name="device_page" value="1" /><button class="button primary" type="submit">{t.adminEmbyApply}</button></form>
        {#if data.deviceAudit.users.length}<div class="audit-list">{#each data.deviceAudit.users as user (user.emby_user_id || user.emby_user_name)}<article class="audit-card"><header><div><strong>{deviceUserName(user)}</strong>{#if user.local_user}<small>{user.local_user.username} · UID {user.local_user.uid}</small>{/if}</div><div class="badges"><span class="badge">{t.adminEmbyDeviceCountDetail.replace("{count}", String(user.device_count))}</span><span class="badge">{user.ip_count} IP</span><span class="badge">{user.online_count} {t.adminEmbyOnline}</span></div></header>{#if user.ips.length}<div class="ip-list">{#each user.ips as ip}<code>{ip}</code>{/each}</div>{/if}<div class="table-region compact-region"><table><thead><tr><th>{t.adminEmbyDeviceList}</th><th>{t.adminEmbyIP}</th><th>{t.adminEmbyActivity}</th><th>{t.adminEmbyOnline}</th></tr></thead><tbody>{#each user.devices as device (device.device_id + device.device_name)}<tr><td><strong>{device.device_name || t.adminEmbyUnknown}</strong><small>{device.app_name} {device.app_version}</small>{#if (device.count || 1) > 1}<span class="badge">×{device.count}</span>{/if}</td><td>{#if device.ip}<code>{device.ip}</code>{#if device.ip_approx}<small>{t.adminEmbyApproxIP}</small>{/if}{:else}-{/if}</td><td>{dateLabel(device.last_activity)}</td><td><span class:good={device.online} class:bad={!device.online}>{deviceStatus(device)}</span></td></tr>{/each}</tbody></table></div></article>{/each}</div>{:else}<p class="empty">{t.adminEmbyNoDeviceUsers}</p>{/if}
        {#if (data.deviceAudit.pages || 0) > 1}<nav class="pagination" aria-label={t.adminEmbyDevices}>{#if (data.deviceAudit.page || 1) > 1}<a class="button secondary" href={queryHref("devices", { device_page: (data.deviceAudit.page || 1) - 1 })}>{t.adminEmbyPrevious}</a>{:else}<span></span>{/if}<span>{data.deviceAudit.page || 1} / {data.deviceAudit.pages || 1}</span>{#if (data.deviceAudit.page || 1) < (data.deviceAudit.pages || 1)}<a class="button secondary" href={queryHref("devices", { device_page: (data.deviceAudit.page || 1) + 1 })}>{t.adminEmbyNext}</a>{:else}<span></span>{/if}</nav>{/if}
      {:else}<p class="empty">{t.adminEmbyNoData}</p>{/if}
    </section>
  {:else}
     <section class="panel" aria-labelledby="activity-title"><header class="panel-heading"><div><h2 id="activity-title">{t.adminEmbyActivity}</h2><p class="muted">{t.adminEmbyActivityDescription}</p></div><div class="toolbar"><a class="button secondary" href={queryHref("activity")}>{t.adminEmbyReadActivity}</a><form method="POST" action="?/syncActivity"><button class="button primary" type="submit">{t.adminEmbySyncActivity}</button></form></div></header>
       {#if activity}<p class="result-meta">{#if action.activity}{t.adminEmbyActivitySynced} · {t.adminEmbyNewActivity.replace("{count}", String(action.activity.new_entries || 0))}{:else}{t.adminEmbyActivityCount.replace("{count}", String(activity.total))}{/if}</p>{#if activity.entries.length}<div class="activity-list">{#each activity.entries as log (log.id)}<article class="activity-row"><span class="activity-mark">•</span><div><strong>{activityType(log)}</strong><small>{log.user_name || t.adminEmbyUnknown}</small><p>{log.name || log.overview || "-"}</p></div><time>{dateLabel(log.date)}</time></article>{/each}</div>{:else}<p class="empty">{t.adminEmbyNoActivity}</p>{/if}{:else}<p class="empty">{t.adminEmbyNoData}</p>{/if}
    </section>
  {/if}
</section>

<style>
  .emby-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .toolbar, .pagination, .activity-row, .audit-card header { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .pagination, .audit-card header { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions, .toolbar { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.2rem; }
  .muted { color: #52606d; margin: .4rem 0 0; } .small { font-size: .8rem; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; white-space: normal; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; }
  .notice { border: 1px solid; border-radius: .35rem; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .tabs { display: flex; gap: .4rem; overflow-x: auto; overscroll-behavior-x: contain; padding-bottom: .2rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .tabs a { background: #e8eef2; border-bottom: 3px solid transparent; color: #16394a; min-height: 2.5rem; padding: .65rem .9rem; text-decoration: none; white-space: nowrap; } .tabs a.active { background: #d4e2e8; border-bottom-color: #245b75; font-weight: 700; }
  .filters { align-items: end; display: grid; gap: .7rem; grid-template-columns: minmax(14rem, 2fr) repeat(3, minmax(8rem, 1fr)) auto; } label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } textarea { min-height: 7rem; resize: vertical; } input:focus, select:focus, textarea:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .connectivity-result { border-top: 1px solid #e1e8ed; display: grid; gap: .75rem; padding-top: .85rem; } .good { color: #276749; } .bad { color: #a61b1b; } .facts { display: grid; gap: .55rem; grid-template-columns: repeat(3, minmax(0, 1fr)); margin: 0; } .facts div { min-width: 0; } dt { color: #52606d; font-size: .8rem; } dd { font-weight: 650; margin: .2rem 0 0; overflow-wrap: anywhere; }
  .test-list { display: grid; gap: .4rem; } .test-row { align-items: baseline; border-bottom: 1px solid #e1e8ed; display: grid; gap: .55rem; grid-template-columns: 1.2rem minmax(9rem, 1fr) minmax(0, 2fr) auto; padding: .5rem 0; } .test-row span { font-weight: 800; } .test-row small, .test-row > span:last-of-type { color: #52606d; }
  .result-meta { color: #52606d; font-size: .84rem; margin: 0; } .table-region { max-height: 62dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } table { border-collapse: collapse; min-width: 48rem; width: 100%; } th, td { border-bottom: 1px solid #e1e8ed; padding: .65rem; text-align: left; vertical-align: top; } th { background: #f4f7f8; color: #52606d; font-size: .8rem; position: sticky; top: 0; z-index: 1; } td { overflow-wrap: anywhere; } td small, td strong { display: block; } td small { color: #52606d; font-size: .76rem; margin-top: .2rem; }
  .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .72rem; margin: .25rem .25rem 0 0; padding: .2rem .45rem; white-space: nowrap; } .badge.info { background: #edf4f8; color: #245b75; } .badge.danger { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .badges { min-width: 0; }
  .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination > span { color: #52606d; font-size: .84rem; } .row-actions { display: flex; flex-wrap: wrap; gap: .35rem; min-width: 11rem; } .small-button { min-height: 2.1rem; padding: .35rem .55rem; } .orphan-region { max-height: 38dvh; }
  .maintenance { gap: .65rem; } .summary-grid { display: grid; gap: .65rem; grid-template-columns: repeat(4, minmax(0, 1fr)); } .summary-grid div { border: 1px solid #d7dee5; display: grid; gap: .2rem; padding: .7rem; } .summary-grid strong { font-size: 1.4rem; } .summary-grid span { color: #52606d; font-size: .8rem; } .tool-grid { display: grid; gap: .9rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .tool-form { border: 1px solid #d7dee5; display: grid; gap: .7rem; min-width: 0; padding: .85rem; } .tool-form h3 { font-size: 1rem; margin: 0; } .tool-form .button { justify-self: start; } .secret-result { display: grid; gap: .4rem; } .secret-result p { margin: 0; }
  .audit-list { display: grid; gap: .8rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; padding-right: .15rem; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .audit-card { border: 1px solid #d7dee5; display: grid; gap: .75rem; min-width: 0; padding: .85rem; } .audit-card header { align-items: center; } .audit-card header strong, .audit-card header small { display: block; overflow-wrap: anywhere; } .audit-card header small { color: #52606d; font-size: .78rem; margin-top: .2rem; } .compact-region { max-height: 32dvh; } .ip-list { display: flex; flex-wrap: wrap; gap: .35rem; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .2rem .35rem; }
  .activity-list { display: grid; gap: .5rem; max-height: 70dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .activity-row { border-bottom: 1px solid #e1e8ed; min-width: 0; padding: .7rem .2rem; } .activity-row > div { flex: 1; min-width: 0; } .activity-row strong, .activity-row small { display: block; overflow-wrap: anywhere; } .activity-row small, .activity-row p, .activity-row time { color: #52606d; font-size: .78rem; } .activity-row p { margin: .25rem 0 0; } .activity-row time { flex: 0 0 auto; max-width: 12rem; text-align: right; }
  .empty { color: #52606d; padding: 1.5rem; text-align: center; }
  @media (max-width: 900px) { .filters { grid-template-columns: repeat(2, minmax(0, 1fr)); } .filters label:first-of-type { grid-column: 1 / -1; } .filters .button { width: 100%; } .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .tool-grid { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading, .heading-actions, .panel-heading, .toolbar, .audit-card header { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button, .panel-heading .button, .panel-heading form, .toolbar form, .toolbar .button { width: 100%; } .filters { grid-template-columns: 1fr; } .filters label:first-of-type { grid-column: auto; } .facts, .summary-grid { grid-template-columns: 1fr; } .test-row { grid-template-columns: 1.2rem minmax(0, 1fr); } .test-row span:last-of-type, .test-row small { grid-column: 2; } .tabs a { flex: 1 0 9rem; text-align: center; } .panel { padding: .85rem; } h1 { font-size: 1.65rem; } .activity-row { flex-direction: column; } .activity-row time { max-width: none; text-align: left; } }
</style>
