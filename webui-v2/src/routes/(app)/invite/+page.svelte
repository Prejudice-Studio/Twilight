<script lang="ts">
  import { t } from "$lib/i18n";
  import type { InviteCodeItem, InviteStatus, InviteTreeNode } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = {
    action?: string;
    error?: string;
    success?: boolean;
    generated?: {
      code: string;
      target_username: string;
      days: number;
      validity_hours: number;
    };
  };

  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let payload = $derived(data.payload);
  let config = $derived(payload?.config);
  let invite = $derived(payload?.invite);
  let codes = $derived(invite?.codes || []);
  let tree = $derived(invite?.tree || null);
  let canCreate = $derived(Boolean(config?.enabled && invite?.can_invite));

  function dateLabel(value?: number | null): string {
    if (!value || value <= 0) return t.inviteNeverExpires;
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function codeStatus(code: InviteCodeItem): string {
    if (code.active && (code.use_count_limit === -1 || code.use_count < code.use_count_limit)) return t.inviteAvailable;
    if (code.use_count > 0) return t.inviteUsed;
    return t.inviteDisabled;
  }

  function statusLabel(child: { active: boolean; has_emby: boolean; emby_disabled?: boolean }): string {
    if (!child.active) return t.inviteInactive;
    if (child.emby_disabled) return t.inviteEmbyDisabled;
    if (!child.has_emby) return t.inviteNoEmby;
    return t.inviteActive;
  }

  function canDetach(node: { can_delete_emby_and_detach?: boolean }): boolean {
    return Boolean(node.can_delete_emby_and_detach);
  }

  async function copyCode(code: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(code);
      window.alert(t.inviteCopied);
    } catch {
      window.alert(t.inviteCopyFailed);
    }
  }
</script>

{#snippet treeNodes(nodes: InviteTreeNode[])}
  <div class="tree-list">
    {#each nodes as node (node.uid)}
      <div class="tree-node">
        <div class="tree-node-head">
          <div class="node-name"><strong>{node.username}</strong><span>{t.inviteTreeNode.replace("{uid}", String(node.uid)).replace("{depth}", String(node.depth))}</span></div>
          <div class="badges">
            <span class:good={node.active} class="badge">{node.active ? t.inviteActive : t.inviteInactive}</span>
            <span class="badge">{node.has_emby ? "Emby" : t.inviteNoEmby}</span>
            {#if node.emby_disabled}<span class="badge danger">{t.inviteEmbyDisabled}</span>{/if}
            {#if node.emby_expired}<span class="badge danger">{t.inviteExpired}</span>{/if}
          </div>
        </div>
        <p class="node-meta">{node.expire_status || "-"}</p>
        {#if node.children?.length}{@render treeNodes(node.children)}{/if}
      </div>
    {/each}
  </div>
{/snippet}

<svelte:head><title>{t.inviteTitle} - {t.siteName}</title></svelte:head>

<section class="invite-page" aria-labelledby="invite-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.account}</p>
      <h1 id="invite-title">{t.inviteTitle}</h1>
      <p class="muted">{t.inviteIntro}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/dashboard">{t.inviteBackDashboard}</a>
      <a class="button secondary" href="/invite">{t.inviteRefresh}</a>
    </div>
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.result === "created"}<p class="notice success" role="status">{t.inviteResultCreated}</p>{/if}
  {#if data.result === "deleted"}<p class="notice success" role="status">{t.inviteDeleted}</p>{/if}
  {#if data.result === "detached" || data.result === "self-detached"}<p class="notice success" role="status">{data.result === "self-detached" ? t.inviteResultSelfDetached : t.inviteResultDetached}</p>{/if}

  {#if payload && invite && config}
    {#if !config.enabled}
      <section class="notice warning"><strong>{t.inviteClosed}</strong><p>{t.inviteClosedHelp}</p></section>
    {/if}

    <section class="metrics" aria-label={t.inviteTitle}>
      <article class="metric"><span>{t.inviteCurrentDepth}</span><strong>{invite.depth} / {invite.max_depth}</strong><small>{invite.is_root ? t.inviteRoot : invite.parent ? t.inviteParent.replace("{username}", invite.parent.username) : t.inviteNoParent}</small></article>
      <article class="metric"><span>{t.inviteChildren}</span><strong>{invite.children.length}</strong><small>{t.inviteChildList}</small></article>
      <article class="metric"><span>{t.inviteDescendants}</span><strong>{tree?.descendant_count || 0}</strong><small>{t.inviteTree}</small></article>
      <article class="metric"><span>{t.inviteEligibility}</span><strong class:good-text={canCreate}>{canCreate ? t.inviteCanInvite : t.inviteCannotInvite}</strong><small>{canCreate ? t.inviteMaxDays.replace("{days}", String(invite.max_code_days || "-")) : invite.invite_block_reason || invite.max_code_days_reason || t.inviteConditionsNotMet}</small></article>
    </section>

    <section class="panel actions-panel" aria-labelledby="create-title">
      <div class="section-heading"><div><h2 id="create-title">{t.inviteGenerate}</h2><p class="muted">{t.inviteGenerateDescription}</p></div></div>
      <form method="POST" action="?/create" class="form-grid">
        <label>{t.inviteDays}<input name="days" type="number" min="1" max={invite.max_code_days || 36500} value={config.default_days > 0 ? Math.min(config.default_days, invite.max_code_days || config.default_days) : 30} required /></label>
        <label>{t.inviteTarget}<input name="target_username" maxlength="32" placeholder={t.inviteTargetPlaceholder} /></label>
        <label class="wide">{t.inviteNote}<input name="note" maxlength="255" placeholder={t.inviteNotePlaceholder} /></label>
        <button class="button primary" type="submit" disabled={!canCreate}>{t.inviteCreate}</button>
      </form>
    </section>

    {#if action.success && action.generated}
      <section class="panel generated-panel" aria-labelledby="generated-title">
        <div class="section-heading"><div><h2 id="generated-title">{t.inviteRenewSuccess}</h2><p class="muted">{t.inviteRenewCodeHelp}</p></div><button class="button secondary" type="button" onclick={() => void copyCode(action.generated?.code || "")}>{t.inviteCopy}</button></div>
        <code class="generated-code">{action.generated.code}</code>
        <p class="muted">{t.inviteRenewResult.replace("{username}", action.generated.target_username).replace("{days}", String(action.generated.days)).replace("{hours}", String(action.generated.validity_hours))}</p>
      </section>
    {/if}

    {#if tree}
      <section class="panel" aria-labelledby="tree-title">
        <div class="section-heading"><div><h2 id="tree-title">{t.inviteTree}</h2><p class="muted">{t.inviteDirectParent}: {invite.parent ? invite.parent.username : t.inviteNoParent}</p></div></div>
        {#if tree.descendants.length}{@render treeNodes(tree.descendants)}{:else}<p class="empty">{t.inviteTreeEmpty}</p>{/if}
      </section>
    {/if}

    {#if invite.parent && tree?.self && canDetach(tree.self)}
      <section class="panel warning-panel">
        <div><h2>{t.inviteDetachSelf}</h2><p class="muted">{t.inviteDetachSelfDescription}</p></div>
        <form method="POST" action="?/detachSelf" onsubmit={(event) => { if (!confirm(t.inviteDetachConfirm)) event.preventDefault(); }}><button class="button danger" type="submit">{t.inviteDetachSelf}</button></form>
      </section>
    {/if}

    {#if invite.children.length}
      <section class="panel" aria-labelledby="children-title">
        <div class="section-heading"><div><h2 id="children-title">{t.inviteChildList}</h2><p class="muted">{t.inviteChildList}</p></div></div>
        <div class="child-grid">
          {#each invite.children as child (child.uid)}
            <article class="child-card">
              <div class="child-head"><div><strong>{child.username}</strong><small>UID #{child.uid}</small></div><span class:good={child.active} class="badge">{statusLabel(child)}</span></div>
              <p class="muted">{t.inviteDaysStatus.replace("{status}", child.expire_status || "-")}</p>
              <div class="child-actions">
                {#if child.can_generate_renew_code}
                  <form method="POST" action="?/renew" class="renew-form">
                    <input type="hidden" name="target_uid" value={child.uid} />
                    <label>{t.inviteRenewDays}<input name="days" type="number" min="1" max={invite.max_code_days || 36500} value={Math.min(30, invite.max_code_days || 30)} required /></label>
                    <label>{t.inviteRenewValidity}<input name="validity_hours" type="number" min="1" max="720" value="72" required /></label>
                    <label>{t.inviteRenewNote}<input name="note" maxlength="120" /></label>
                    <button class="button secondary" type="submit">{t.inviteRenewCode}</button>
                  </form>
                {/if}
                {#if child.can_delete_emby_and_detach}
                  <form method="POST" action="?/detachChild" onsubmit={(event) => { if (!confirm(t.inviteDetachConfirm)) event.preventDefault(); }}>
                    <input type="hidden" name="uid" value={child.uid} />
                    <button class="button danger" type="submit">{t.inviteDetachChild}</button>
                  </form>
                {/if}
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    <section class="panel codes-panel" aria-labelledby="codes-title">
      <div class="section-heading"><div><h2 id="codes-title">{t.inviteCodes}</h2></div><span class="count">{codes.length}</span></div>
      {#if codes.length === 0}<p class="empty">{t.inviteCodesEmpty}</p>{:else}<div class="code-list">
        {#each codes as code (code.code)}
          {@const usedUp = code.use_count_limit !== -1 && code.use_count >= code.use_count_limit}
          <article class="code-row">
            <div class="code-main"><div class="code-line"><code>{code.code}</code><span class:good={code.active && !usedUp} class="badge">{codeStatus(code)}</span><span class="badge">{code.days > 0 ? t.inviteDaysLabel.replace("{days}", String(code.days)) : t.invitePermanent}</span></div>
              <p class="muted">{t.inviteCreatedAt.replace("{date}", dateLabel(code.created_at))}{code.expires_at ? ` · ${t.inviteExpiresAt.replace("{date}", dateLabel(code.expires_at))}` : ` · ${t.inviteNeverExpires}`}{code.target_username ? ` · ${t.inviteTargetLabel.replace("{username}", code.target_username)}` : ""}{code.used_by_username ? ` · ${t.inviteUsedBy.replace("{username}", code.used_by_username)}` : ""}{code.note ? ` · ${code.note}` : ""}</p>
            </div>
            <div class="code-actions"><button class="button secondary" type="button" onclick={() => void copyCode(code.code)}>{t.inviteCopy}</button><form method="POST" action="?/deleteCode" onsubmit={(event) => { if (!confirm(t.inviteDeleteConfirm)) event.preventDefault(); }}><input type="hidden" name="code" value={code.code} /><button class="button danger" type="submit">{t.inviteDelete}</button></form></div>
          </article>
        {/each}
      </div>{/if}
    </section>
  {:else if !data.loadError}
    <p class="notice error" role="alert">{t.inviteLoadFailed}</p>
  {/if}
</section>

<style>
  .invite-page { display: grid; gap: 1rem; }
  .page-heading, .section-heading, .heading-actions, .child-head, .code-line, .code-actions { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .section-heading, .child-head, .code-line { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.15rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: clamp(1.65rem, 7vw, 2.25rem); } h2 { font-size: 1.1rem; }
  .muted { color: #52606d; margin: .4rem 0 0; } small { color: #52606d; display: block; font-size: .78rem; margin-top: .25rem; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; padding: .5rem .85rem; }
  .button.primary { background: #245b75; color: #fff; } .button.primary:hover { background: #1c465a; } .button.secondary { background: #e8eef2; color: #16394a; } .button.secondary:hover { background: #d6e1e7; } .button.danger { background: #a63d40; color: #fff; } .button:disabled { cursor: not-allowed; opacity: .55; }
  .button:focus-visible, a:focus-visible, input:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .notice, .panel { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; padding: 1rem; } .notice { margin: 0; } .notice p { margin: .4rem 0 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .metrics { display: grid; gap: 1rem; grid-template-columns: repeat(4, minmax(0, 1fr)); } .metric { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: .35rem; min-width: 0; padding: 1rem; } .metric span { color: #52606d; font-size: .8rem; } .metric strong { font-size: 1.35rem; overflow-wrap: anywhere; } .metric small { min-height: 2.2em; } .good-text { color: #276749; }
  .form-grid { display: grid; gap: .75rem; grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 1rem; } label { color: #243b53; display: grid; font-size: .87rem; font-weight: 650; gap: .35rem; min-width: 0; } label.wide, .form-grid .wide { grid-column: 1 / -1; } input { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; padding: .5rem .65rem; } .form-grid .button { justify-self: start; }
  .badge, .count { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; font-size: .72rem; padding: .18rem .48rem; white-space: nowrap; } .badge.good { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .badge.danger { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .count { font-size: .8rem; }
  .tree-list { border-left: 2px solid #d7dee5; display: grid; gap: .65rem; margin-top: 1rem; padding-left: .9rem; } .tree-node { display: grid; gap: .4rem; min-width: 0; } .tree-node .tree-list { margin: .25rem 0 0 .6rem; } .tree-node-head { align-items: flex-start; display: flex; gap: .65rem; justify-content: space-between; } .node-name { display: grid; gap: .2rem; min-width: 0; } .node-name span, .node-meta { color: #52606d; font-size: .78rem; margin: 0; } .badges { display: flex; flex-wrap: wrap; gap: .3rem; justify-content: flex-end; }
  .empty { color: #52606d; margin: 0; padding: 1.5rem .5rem; text-align: center; } .warning-panel { align-items: center; background: #fff8e6; border-color: #e9c46a; display: flex; gap: 1rem; justify-content: space-between; } .warning-panel h2 { color: #7b4f00; }
  .child-grid { display: grid; gap: .75rem; grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 1rem; } .child-card { border: 1px solid #d7dee5; border-radius: .35rem; display: grid; gap: .55rem; min-width: 0; padding: .8rem; } .child-head { align-items: center; } .child-head > div { min-width: 0; } .child-actions { border-top: 1px solid #e1e8ed; display: grid; gap: .55rem; padding-top: .65rem; } .renew-form { display: grid; gap: .5rem; grid-template-columns: repeat(2, minmax(0, 1fr)); } .renew-form label:last-of-type { grid-column: 1 / -1; } .renew-form .button { justify-self: start; }
  .code-list { display: grid; gap: 0; margin-top: .5rem; } .code-row { align-items: flex-start; border-top: 1px solid #e1e8ed; display: flex; gap: 1rem; justify-content: space-between; min-width: 0; padding: .8rem 0; } .code-main { min-width: 0; } .code-line { align-items: center; flex-wrap: wrap; justify-content: flex-start; } code { background: #f4f6f8; border: 1px solid #d7dee5; border-radius: .25rem; font-family: ui-monospace, monospace; font-size: .82rem; max-width: 100%; overflow-wrap: anywhere; padding: .25rem .4rem; } .code-actions { flex: 0 0 auto; flex-wrap: wrap; } .generated-panel { border-color: #a9c8d5; } .generated-code { display: block; font-size: 1rem; margin-top: 1rem; padding: .75rem; }
  @media (max-width: 900px) { .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .child-grid { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .page-heading, .section-heading, .warning-panel { align-items: stretch; flex-direction: column; } .heading-actions { align-items: stretch; } .heading-actions .button { text-align: center; } .metrics, .form-grid, .renew-form { grid-template-columns: 1fr; } .form-grid .wide, .renew-form label:last-of-type { grid-column: auto; } .form-grid .button, .renew-form .button { justify-self: stretch; width: 100%; } .code-row { flex-direction: column; } .code-actions { width: 100%; } .code-actions .button, .code-actions form { flex: 1 1 0; } .code-actions form .button { width: 100%; } .panel, .notice { padding: .85rem; } .tree-node-head { flex-direction: column; } .badges { justify-content: flex-start; } }
</style>
