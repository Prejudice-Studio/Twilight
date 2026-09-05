<script lang="ts">
  import { t } from "$lib/i18n";
  import type { AdminInviteTreeRow, ConfigField } from "$lib/types";
  import type { PageData } from "./$types";

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as { action?: string; error?: string });
  let selectedUids = $state<number[]>([]);
  let selectedCount = $derived(selectedUids.length);
  let allSelected = $derived(Boolean(data.tree?.rows.length) && data.tree!.rows.every((row) => selectedUids.includes(row.uid)));

  $effect(() => {
    const visible = new Set(data.tree?.rows.map((row) => row.uid) || []);
    selectedUids = selectedUids.filter((uid) => visible.has(uid));
  });

  function dateLabel(value: number | null | undefined): string {
    return value && value > 0 ? new Date(value * 1000).toLocaleString("zh-CN") : t.adminInvitePermanent;
  }

  function roleLabel(role: number): string {
    if (role === 0) return t.adminInviteRoleAdmin;
    if (role === 2) return t.adminInviteRoleWhitelist;
    return t.adminInviteRoleUser;
  }

  function pageHref(page: number, overrides: Partial<PageData["query"]> = {}): string {
    const value = { ...data.query, ...overrides, page: Math.max(1, page) };
    const params = new URLSearchParams();
    if (value.view !== "tree") params.set("view", value.view);
    if (value.page > 1) params.set("page", String(value.page));
    if (value.per_page !== 300) params.set("per_page", String(value.per_page));
    if (value.search) params.set("search", value.search);
    if (value.root !== "all") params.set("root", value.root);
    if (value.selected > 0) params.set("selected", String(value.selected));
    if (value.collapsed.length) params.set("collapsed", value.collapsed.join(","));
    if (value.code_page > 1) params.set("code_page", String(value.code_page));
    if (value.code_per_page !== 50) params.set("code_per_page", String(value.code_per_page));
    if (value.code_search) params.set("code_search", value.code_search);
    return `/admin/invite${params.toString() ? `?${params}` : ""}`;
  }

  function tabHref(view: "tree" | "codes" | "config"): string {
    return pageHref(1, { view, selected: 0 });
  }

  function toggleCollapse(uid: number): string {
    const collapsed = new Set(data.query.collapsed);
    if (collapsed.has(uid)) collapsed.delete(uid);
    else collapsed.add(uid);
    return pageHref(1, { collapsed: [...collapsed], selected: 0 });
  }

  function toggleSelection(uid: number, checked: boolean): void {
    selectedUids = checked
      ? [...new Set([...selectedUids, uid])]
      : selectedUids.filter((item) => item !== uid);
  }

  function toggleAll(checked: boolean): void {
    const visible = data.tree?.rows.map((row) => row.uid) || [];
    selectedUids = checked ? [...new Set([...selectedUids, ...visible])] : selectedUids.filter((uid) => !visible.includes(uid));
  }

  function confirmSubmit(event: SubmitEvent, message: string): void {
    if (!window.confirm(message)) event.preventDefault();
  }

  function confirmBatch(event: SubmitEvent): void {
    const submitter = event.submitter as HTMLButtonElement | null;
    confirmSubmit(event, submitter?.dataset.confirm || t.adminInviteBatchDetachConfirm);
  }

  function confirmCascade(event: SubmitEvent): void {
    const submitter = event.submitter as HTMLButtonElement | null;
    confirmSubmit(event, submitter?.dataset.confirm || t.adminInviteCascadeConfirm);
  }

  function replace(value: string, placeholder: string, replacement: string): string {
    return value.replace(placeholder, replacement);
  }

  function totalLabel(): string {
    const tree = data.tree;
    if (!tree) return "";
    return t.adminInviteTotal
      .replace("{count}", String(tree.total_rows))
      .replace("{nodes}", String(tree.total_nodes))
      .replace("{relations}", String(tree.total_relations));
  }

  function codeStatus(code: { active: boolean; use_count: number; use_count_limit: number }): string {
    if (!code.active) return t.adminInviteCodeDisabled;
    if (code.use_count > 0 || code.use_count_limit === 0) return t.adminInviteCodeUsed;
    return t.adminInviteCodeAvailable;
  }

  function fieldInputType(field: ConfigField): string {
    return field.type === "int" || field.type === "float" ? "number" : "text";
  }
</script>

<svelte:head><title>{t.adminInviteTitle} - {t.siteName}</title></svelte:head>

<section class="invite-admin-page" aria-labelledby="invite-admin-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.adminArea}</p>
      <h1 id="invite-admin-title">{t.adminInviteTitle}</h1>
      <p class="muted">{t.adminInviteDescription}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/admin/status">{t.adminStatusTitle}</a>
      <a class="button secondary" href={pageHref(data.query.page)}>{t.adminInviteRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.notice === "detached"}<p class="notice success" role="status">{t.adminInviteDetached}</p>{/if}
  {#if data.notice === "deleted_emby"}<p class="notice success" role="status">{t.adminInviteDeletedEmby}</p>{/if}
  {#if data.notice === "batch_detached"}<p class="notice success" role="status">{t.adminInviteBatchDetached}</p>{/if}
  {#if data.notice === "quick_maintained"}<p class="notice success" role="status">{t.adminInviteQuickMaintained}</p>{/if}
  {#if data.notice === "cascade_updated"}<p class="notice success" role="status">{t.adminInviteCascadeUpdated}</p>{/if}
  {#if data.notice === "deleted"}<p class="notice success" role="status">{t.adminInviteDeleted}</p>{/if}
  {#if data.notice === "config_saved"}<p class="notice success" role="status">{t.adminInviteConfigSaved}</p>{/if}

  <nav class="tabs" aria-label={t.adminInviteTitle}>
    <a class:active={data.view === "tree"} href={tabHref("tree")}>{t.adminInviteTreeTab}</a>
    <a class:active={data.view === "codes"} href={tabHref("codes")}>{t.adminInviteCodesTab}</a>
    <a class:active={data.view === "config"} href={tabHref("config")}>{t.adminInviteConfigTab}</a>
  </nav>

  {#if data.view === "tree"}
    <section class="panel" aria-labelledby="tree-filter-title">
      <header class="panel-heading">
        <div><h2 id="tree-filter-title">{t.adminInviteFilter}</h2><p class="muted">{totalLabel()}</p></div>
        {#if data.tree}<span class="meta">{replace(t.adminInviteDepthSummary, "{depth}", String(data.tree.max_depth))}</span>{/if}
      </header>
      <form method="GET" action="/admin/invite" class="filter-form">
        <input type="hidden" name="view" value="tree" />
        <input type="hidden" name="page" value="1" />
        <label class="search-field">{t.adminInviteSearch}
          <input name="search" maxlength="120" value={data.query.search} placeholder={t.adminInviteSearchPlaceholder} />
        </label>
        <label>{t.adminInviteRoot}
          <select name="root">
            <option value="all" selected={data.query.root === "all"}>{t.adminInviteAllRoots}</option>
            {#each data.tree?.roots || [] as root (root.uid)}<option value={root.uid} selected={data.query.root === String(root.uid)}>#{root.uid} {root.username}</option>{/each}
          </select>
        </label>
        <label>{t.adminInvitePerPage}
          <select name="per_page">
            {#each [100, 300, 500] as size}<option value={size} selected={data.query.per_page === size}>{size}</option>{/each}
          </select>
        </label>
        <button class="button primary" type="submit">{t.adminInviteApply}</button>
        <a class="button secondary" href="/admin/invite">{t.adminInviteReset}</a>
      </form>
    </section>

    {#if data.tree && data.tree.total_nodes > 0}
      <form id="invite-batch-form" method="POST" action="?/batchDetach" onsubmit={confirmBatch}>
        <input type="hidden" name="view" value="tree" />
        <input type="hidden" name="page" value={data.query.page} />
        <input type="hidden" name="per_page" value={data.query.per_page} />
        <input type="hidden" name="search" value={data.query.search} />
        <input type="hidden" name="root" value={data.query.root} />
        <input type="hidden" name="collapsed" value={data.query.collapsed.join(",")} />
        <input type="hidden" name="root_uid" value={data.query.selected || ""} />
        <div class="bulk-toolbar">
          <div class="selection-meta">{replace(t.adminInviteSelection, "{count}", String(selectedCount))}</div>
          <div class="bulk-actions">
            <label class="renew-field">{t.adminInviteQuickRenewDays}<input name="renew_days" type="number" min="-1" max="36500" value="30" placeholder={t.adminInviteQuickRenewPlaceholder} /></label>
            <button class="button secondary compact" type="button" onclick={() => toggleAll(!allSelected)}>{allSelected ? t.adminInviteClearSelection : t.adminInviteSelectAll}</button>
            <button class="button secondary compact" type="button" onclick={() => selectedUids = []}>{t.adminInviteClearSelection}</button>
            <button class="button secondary compact" type="submit" data-confirm={t.adminInviteBatchDetachConfirm}>{t.adminInviteBatchDetach}</button>
            <button class="button danger compact" type="submit" name="operation" value="delete_emby" data-confirm={t.adminInviteBatchDeleteEmbyConfirm}>{t.adminInviteBatchDeleteEmby}</button>
            <button class="button danger compact" type="submit" name="operation" value="only_emby_disabled" data-confirm={t.adminInviteBatchDeleteEmbyConfirm}>{t.adminInviteBatchDisabledEmby}</button>
            <button class="button secondary compact" type="submit" formaction="?/quickMaintenance" name="scope" value="selected" data-confirm={t.adminInviteQuickSelectedConfirm}>{t.adminInviteQuickSelected}</button>
            <button class="button secondary compact" type="submit" formaction="?/quickMaintenance" name="scope" value="all" data-confirm={t.adminInviteQuickAllConfirm}>{t.adminInviteQuickAll}</button>
            {#if data.query.selected > 0}<button class="button secondary compact" type="submit" formaction="?/quickMaintenance" name="scope" value="subtree" data-confirm={t.adminInviteQuickSubtreeConfirm}>{t.adminInviteQuickSubtree}</button>{/if}
          </div>
        </div>

        {#if data.tree.rows.length}
          <div class="table-region">
            <table class="invite-table">
              <thead><tr><th class="check-col"><span class="sr-only">{t.adminInviteSelectAll}</span></th><th>{t.adminInviteUser}</th><th>{t.adminInviteRole}</th><th>{t.adminInviteStatus}</th><th>{t.adminInviteEmby}</th><th>{t.adminInviteTelegram}</th><th>{t.adminInviteChildren}</th><th>{t.adminInviteActions}</th></tr></thead>
              <tbody>
                {#each data.tree.rows as row (row.uid)}
                  <tr class:selected-row={data.query.selected === row.uid}>
                    <td class="check-col"><input type="checkbox" name="uids" value={row.uid} checked={selectedUids.includes(row.uid)} onchange={(event) => toggleSelection(row.uid, (event.currentTarget as HTMLInputElement).checked)} aria-label={`${t.adminInviteUser} ${row.username}`} /></td>
                    <td>
                      <div class="tree-user" style={`--depth: ${row.depth}`}>
                        {#if row.direct_children > 0}<a class="collapse-link" href={toggleCollapse(row.uid)} aria-label={row.collapsed ? t.adminInviteExpand : t.adminInviteCollapse}>{row.collapsed ? "+" : "−"}</a>{:else}<span class="collapse-spacer"></span>{/if}
                        <div class="identity"><a href={pageHref(data.query.page, { selected: row.uid })}><strong>{row.username}</strong></a><small>{t.adminInviteUID} {row.uid} · {t.adminInviteLevel} {row.depth + 1} · {t.adminInviteRootUID} {row.root_uid}</small></div>
                      </div>
                    </td>
                    <td>{roleLabel(row.role)}</td>
                    <td><span class:badge-danger={!row.active} class="badge">{row.active ? t.adminInviteEnabled : t.adminInviteDisabled}</span></td>
                    <td><span class:badge-danger={row.emby_disabled} class="badge">{row.emby_disabled ? t.adminInviteEmbyDisabled : row.emby_bound ? t.adminInviteBound : t.adminInviteUnbound}</span></td>
                    <td>{row.telegram_id || "-"}</td>
                    <td>{replace(t.adminInviteChildSummary, "{direct}", String(row.direct_children)).replace("{total}", String(row.descendants))}</td>
                    <td><a class="button secondary compact" href={pageHref(data.query.page, { selected: row.uid })}>{t.adminInviteDetails}</a></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}<p class="empty">{t.adminInviteNoMatches}</p>{/if}
      </form>

      {#if data.tree.pages > 1}
        <nav class="pagination" aria-label={t.adminInviteTitle}>
          {#if data.tree.page > 1}<a class="button secondary" href={pageHref(data.tree.page - 1)}>{t.adminInvitePrevious}</a>{:else}<span></span>{/if}
          <span>{replace(t.adminInviteRows, "{page}", String(data.tree.page)).replace("{pages}", String(data.tree.pages)).replace("{count}", String(data.tree.total_rows))}</span>
          {#if data.tree.page < data.tree.pages}<a class="button secondary" href={pageHref(data.tree.page + 1)}>{t.adminInviteNext}</a>{:else}<span></span>{/if}
        </nav>
      {/if}
    {:else if data.tree}
      <section class="panel empty-panel"><strong>{t.adminInviteEmpty}</strong><p class="muted">{t.adminInviteEmptyDescription}</p></section>
    {/if}

    {#if data.tree?.selected}
      {@const selected = data.tree.selected}
      <section class="panel detail-panel" aria-labelledby="invite-detail-title">
        <header class="panel-heading"><div><h2 id="invite-detail-title">{selected.username}</h2><p class="muted">{t.adminInviteUID} {selected.uid}</p></div><a class="button secondary compact" href={pageHref(data.tree.page, { selected: 0 })}>{t.adminInviteClearSelection}</a></header>
        <dl class="facts"><div><dt>{t.adminInviteRole}</dt><dd>{roleLabel(selected.role)}</dd></div><div><dt>{t.adminInviteStatus}</dt><dd>{selected.active ? t.adminInviteEnabled : t.adminInviteDisabled}</dd></div><div><dt>{t.adminInviteEmby}</dt><dd>{selected.emby_disabled ? t.adminInviteEmbyDisabled : selected.emby_bound ? t.adminInviteBound : t.adminInviteUnbound}</dd></div><div><dt>{t.adminInviteRegisteredAt}</dt><dd>{dateLabel(selected.register_time)}</dd></div><div><dt>{t.adminInviteExpiresAt}</dt><dd>{dateLabel(selected.expired_at)}</dd></div><div><dt>{t.adminInviteSubtree.replace("{count}", String(selected.descendants))}</dt><dd>{t.adminInviteRootUID} {selected.root_uid}</dd></div></dl>
        <div class="detail-actions">
          {#if selected.is_root}<span class="muted">{t.adminInviteAlreadyRoot}</span>{:else}<form method="POST" action="?/detach" onsubmit={(event) => confirmSubmit(event, t.adminInviteDetachDescription)}><input type="hidden" name="uid" value={selected.uid} /><input type="hidden" name="view" value="tree" /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="root" value={data.query.root} /><input type="hidden" name="collapsed" value={data.query.collapsed.join(",")} /><button class="button secondary" type="submit">{t.adminInviteDetach}</button></form>{/if}
          {#if selected.role !== 0}<form method="POST" action="?/detachDeleteEmby" onsubmit={(event) => confirmSubmit(event, t.adminInviteDetachDeleteEmbyDescription)}><input type="hidden" name="uid" value={selected.uid} /><input type="hidden" name="view" value="tree" /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="root" value={data.query.root} /><input type="hidden" name="collapsed" value={data.query.collapsed.join(",")} /><button class="button danger" type="submit">{t.adminInviteDetachDeleteEmby}</button></form>{/if}
          <form method="POST" action="?/cascadeToggle" onsubmit={confirmCascade}><input type="hidden" name="uid" value={selected.uid} /><input type="hidden" name="enable" value="false" /><input type="hidden" name="view" value="tree" /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="root" value={data.query.root} /><input type="hidden" name="collapsed" value={data.query.collapsed.join(",")} /><label>{t.adminInviteCascadeDepth}<input name="depth" type="number" min="-1" max="5000" value="1" /></label><button class="button secondary" type="submit" data-confirm={t.adminInviteCascadeConfirm}>{t.adminInviteCascadeDisable}</button></form>
          <form method="POST" action="?/cascadeToggle" onsubmit={confirmCascade}><input type="hidden" name="uid" value={selected.uid} /><input type="hidden" name="enable" value="true" /><input type="hidden" name="view" value="tree" /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="root" value={data.query.root} /><input type="hidden" name="collapsed" value={data.query.collapsed.join(",")} /><label>{t.adminInviteCascadeDepth}<input name="depth" type="number" min="-1" max="5000" value="1" /></label><button class="button secondary" type="submit" data-confirm={t.adminInviteCascadeConfirm}>{t.adminInviteCascadeEnable}</button></form>
          {#if selected.role !== 0}<form method="POST" action="?/cascadeDelete" onsubmit={(event) => confirmSubmit(event, t.adminInviteCascadeDeleteConfirm)}><input type="hidden" name="uid" value={selected.uid} /><input type="hidden" name="view" value="tree" /><input type="hidden" name="page" value={data.query.page} /><input type="hidden" name="per_page" value={data.query.per_page} /><input type="hidden" name="search" value={data.query.search} /><input type="hidden" name="root" value={data.query.root} /><input type="hidden" name="collapsed" value={data.query.collapsed.join(",")} /><label>{t.adminInviteCascadeDepth}<input name="depth" type="number" min="-1" max="5000" value="1" /></label><button class="button danger" type="submit">{t.adminInviteCascadeDelete}</button></form>{/if}
        </div>
      </section>
    {/if}
  {:else if data.view === "codes"}
    <section class="panel" aria-labelledby="codes-heading">
      <header class="panel-heading"><div><h2 id="codes-heading">{t.adminInviteCodesTab}</h2><p class="muted">{t.adminInviteCodesDescription}</p></div></header>
      <form method="GET" action="/admin/invite" class="filter-form code-filter"><input type="hidden" name="view" value="codes" /><input type="hidden" name="code_page" value="1" /><label class="search-field">{t.adminInviteCodeSearch}<input name="code_search" maxlength="120" value={data.query.code_search} /></label><label>{t.adminInvitePerPage}<select name="code_per_page">{#each [20, 50, 100] as size}<option value={size} selected={data.query.code_per_page === size}>{size}</option>{/each}</select></label><button class="button primary" type="submit">{t.adminInviteApply}</button><a class="button secondary" href={tabHref("codes")}>{t.adminInviteReset}</a></form>
      {#if data.codes?.codes.length}<div class="table-region code-region"><table class="codes-table"><thead><tr><th>{t.adminInviteCode}</th><th>{t.adminInviteInviter}</th><th>{t.adminInviteDays}</th><th>{t.adminInviteUse}</th><th>{t.adminInviteCodeStatus}</th><th>{t.adminInviteExpires}</th><th>{t.adminInviteCodeTarget}</th><th>{t.adminInviteCodeNote}</th></tr></thead><tbody>{#each data.codes.codes as code (code.code)}<tr><td><code>{code.code}</code></td><td>{code.inviter_username || `UID ${code.inviter_uid}`}</td><td>{code.days < 0 ? t.adminInvitePermanent : `${code.days} 天`}</td><td>{code.use_count} / {code.use_count_limit < 0 ? "∞" : code.use_count_limit}</td><td><span class="badge">{codeStatus(code)}</span></td><td>{dateLabel(code.expires_at)}</td><td>{code.target_username || "-"}</td><td>{code.note || "-"}</td></tr>{/each}</tbody></table></div>{:else}<p class="empty">{t.adminInviteCodeEmpty}</p>{/if}
      {#if data.codes && data.codes.pages > 1}<nav class="pagination" aria-label={t.adminInviteCodesTab}>{#if data.codes.page > 1}<a class="button secondary" href={pageHref(1, { view: "codes", page: 1, code_page: data.codes.page - 1 })}>{t.adminInvitePrevious}</a>{:else}<span></span>{/if}<span>{replace(t.adminInviteRows, "{page}", String(data.codes.page)).replace("{pages}", String(data.codes.pages)).replace("{count}", String(data.codes.total))}</span>{#if data.codes.page < data.codes.pages}<a class="button secondary" href={pageHref(1, { view: "codes", page: 1, code_page: data.codes.page + 1 })}>{t.adminInviteNext}</a>{:else}<span></span>{/if}</nav>{/if}
    </section>
  {:else}
    <section class="panel" aria-labelledby="config-heading">
      <header class="panel-heading"><div><h2 id="config-heading">{t.adminInviteConfigTab}</h2><p class="muted">{t.adminInviteConfigDescription}</p></div></header>
      {#if data.config}
        <form method="POST" action="?/saveConfig" class="config-form"><input type="hidden" name="view" value="config" />
          {#each data.config.fields as field (field.key)}
            <article class="config-field"><div><label for={`invite-${field.key}`}>{field.label || field.key}</label><code>{field.key}</code><p class="muted">{field.description}</p></div><div class="config-control">{#if field.type === "bool"}<input type="hidden" name={field.key} value="false" /><label class="switch"><input id={`invite-${field.key}`} type="checkbox" name={field.key} value="true" checked={Boolean(field.value)} /><span>{Boolean(field.value) ? t.adminInviteConfigBooleanOn : t.adminInviteConfigBooleanOff}</span></label>{:else if field.type === "select"}<select id={`invite-${field.key}`} name={field.key}>{#each field.options || [] as option}<option value={option.value} selected={String(option.value) === String(field.value)}>{option.label}</option>{/each}</select>{:else}<input id={`invite-${field.key}`} name={field.key} type={fieldInputType(field)} value={String(field.value ?? "")} />{/if}</div></article>
          {/each}
          <footer class="form-footer"><p class="muted">{t.adminInviteConfigHelp}</p><button class="button primary" type="submit">{t.adminInviteConfigSave}</button></footer>
        </form>
      {:else}<p class="empty">{t.adminInviteConfigLoadFailed}</p>{/if}
    </section>
  {/if}
</section>

<style>
  .invite-admin-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .heading-actions, .panel-heading, .bulk-toolbar, .bulk-actions, .pagination, .detail-actions, .facts, .form-footer { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .panel-heading, .bulk-toolbar, .pagination, .form-footer { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.1rem; }
  .heading-actions, .bulk-actions, .detail-actions { flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: 2rem; } h2 { font-size: 1.15rem; }
  .muted { color: #52606d; margin: .4rem 0 0; } .meta { color: #52606d; font-size: .82rem; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { align-items: center; background: #e8eef2; border: 0; border-radius: .3rem; color: #16394a; cursor: pointer; display: inline-flex; font: inherit; font-weight: 650; justify-content: center; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; white-space: normal; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button.compact { min-height: 2.25rem; padding: .35rem .6rem; }
  .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: 1rem; min-width: 0; padding: 1rem; }
  .notice { border: 1px solid; border-radius: .35rem; margin: 0; padding: .7rem .8rem; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #eef8f1; border-color: #a5d6b0; color: #276749; }
  .tabs { align-items: stretch; border-bottom: 1px solid #c8d2da; display: flex; gap: .35rem; overflow-x: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; }
  .tabs a { color: #36566a; min-height: 2.7rem; padding: .65rem .85rem; text-decoration: none; white-space: nowrap; } .tabs a.active { border-bottom: 3px solid #245b75; color: #16394a; font-weight: 700; }
  .filter-form { align-items: end; display: grid; gap: .75rem; grid-template-columns: minmax(0, 2fr) minmax(10rem, 1fr) minmax(8rem, .7fr) auto auto; }
  label { color: #243b53; display: grid; font-size: .84rem; font-weight: 650; gap: .35rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } input:focus, select:focus, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .bulk-toolbar { align-items: center; background: #eef2f4; border: 1px solid #c8d2da; border-radius: .35rem; flex-wrap: wrap; padding: .75rem; } .selection-meta { color: #243b53; font-weight: 700; } .renew-field { align-items: center; display: flex; flex-direction: row; gap: .45rem; } .renew-field input { width: 7rem; }
  .table-region { max-height: min(70dvh, 900px); min-width: 0; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } table { border-collapse: collapse; width: 100%; } .invite-table { min-width: 980px; } .codes-table { min-width: 860px; } th, td { border-bottom: 1px solid #e1e8ed; padding: .7rem .65rem; text-align: left; vertical-align: top; } th { background: #f4f7f8; color: #243b53; font-size: .8rem; position: sticky; top: 0; z-index: 1; } tr.selected-row { background: #f0f5f7; } .check-col { width: 3rem; text-align: center; } .tree-user { align-items: flex-start; display: flex; gap: .45rem; padding-left: calc(var(--depth) * 1.1rem); } .collapse-link { align-items: center; border: 1px solid #9fb3c8; border-radius: .25rem; color: #36566a; display: inline-flex; flex: 0 0 1.5rem; height: 1.5rem; justify-content: center; line-height: 1; text-decoration: none; } .collapse-spacer { flex: 0 0 1.5rem; } .identity { display: grid; min-width: 0; } .identity a { color: #16394a; overflow-wrap: anywhere; } small { color: #52606d; overflow-wrap: anywhere; } .badge { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; display: inline-block; font-size: .76rem; max-width: 100%; overflow-wrap: anywhere; padding: .2rem .45rem; white-space: normal; } .badge-danger { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; }
  .empty, .empty-panel { color: #52606d; padding: 1.5rem; text-align: center; } .pagination { align-items: center; border-top: 1px solid #e1e8ed; padding-top: .85rem; } .pagination span { color: #52606d; font-size: .84rem; }
  .detail-panel { scroll-margin-top: 1rem; } .facts { flex-wrap: wrap; margin: 0; } .facts div { min-width: 10rem; } dt { color: #52606d; font-size: .76rem; } dd { margin: .2rem 0 0; overflow-wrap: anywhere; } code { background: #f4f7f8; border: 1px solid #d7dee5; border-radius: .25rem; font: .78rem ui-monospace, SFMono-Regular, Consolas, monospace; max-width: 100%; overflow-wrap: anywhere; padding: .15rem .3rem; }
  .detail-actions form { align-items: end; display: flex; flex-wrap: wrap; gap: .45rem; } .detail-actions form label { min-width: 12rem; } .detail-actions form label input { width: 7rem; }
  .config-form { display: grid; gap: .75rem; } .config-field { align-items: start; border-bottom: 1px solid #e1e8ed; display: grid; gap: 1rem; grid-template-columns: minmax(0, 1.4fr) minmax(12rem, .8fr); padding: .8rem 0; } .config-field label:not(.switch) { color: #16394a; font-size: .95rem; } .config-field code { display: inline-block; margin-top: .3rem; } .config-control { min-width: 0; } .switch { align-items: center; display: flex; gap: .5rem; min-height: 2.5rem; } .form-footer { align-items: center; flex-wrap: wrap; padding-top: .5rem; }
  .sr-only { clip: rect(0, 0, 0, 0); clip-path: inset(50%); height: 1px; overflow: hidden; position: absolute; white-space: nowrap; width: 1px; }
  @media (max-width: 900px) { .filter-form { grid-template-columns: repeat(2, minmax(0, 1fr)); } .search-field { grid-column: span 2; } .filter-form > .button, .filter-form > a { width: 100%; } .code-filter { grid-template-columns: minmax(0, 1fr) minmax(7rem, .45fr); } .code-filter .search-field { grid-column: span 2; } .code-filter > .button, .code-filter > a { width: 100%; } }
  @media (max-width: 650px) { .page-heading, .heading-actions, .panel-heading, .bulk-toolbar, .form-footer { align-items: stretch; flex-direction: column; } .heading-actions, .heading-actions > *, .heading-actions .button { width: 100%; } .filter-form, .code-filter { grid-template-columns: 1fr; } .search-field, .code-filter .search-field { grid-column: auto; } .filter-form > .button, .filter-form > a, .code-filter > .button, .code-filter > a { width: 100%; } .bulk-actions { align-items: stretch; flex-direction: column; } .bulk-actions > *, .renew-field, .renew-field input { width: 100%; } .config-field { grid-template-columns: 1fr; } .detail-actions { display: grid; } .detail-actions form, .detail-actions form label, .detail-actions form button { width: 100%; } .pagination { align-items: stretch; } .pagination .button { min-width: 0; } h1 { font-size: 1.65rem; } .panel { padding: .85rem; } }
</style>
