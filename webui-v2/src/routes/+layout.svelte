<script lang="ts">
  import { t } from "$lib/i18n";
  import type { LayoutData } from "./$types";

  let { data, children }: { data: LayoutData; children: import("svelte").Snippet } = $props();
</script>

<svelte:head>
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
  <meta name="theme-color" content="#17202a" />
  <title>{t.siteName}</title>
</svelte:head>

<div class="app-shell">
  <header class="topbar">
    <a class="brand" href={data.user ? "/dashboard" : "/login"}>{t.siteName}</a>
    {#if data.user}
      <nav aria-label="主导航">
        <a href="/dashboard">{t.dashboard}</a>
        <a href="/wiki">Wiki</a>
        <a href="/announcements">{t.announcements}</a>
        <a href="/score">{t.signin}</a>
        <a href="/invite">{t.inviteTitle}</a>
        <a href="/bangumi">{t.bangumiTitle}</a>
        <a href="/media">{t.media}</a>
        <a href="/tickets">{t.tickets}</a>
        <a href="/settings">{t.settings}</a>
        {#if data.user.role === 0}<a href="/admin">{t.adminHomeTitle}</a><a href="/admin/status">{t.adminStatusTitle}</a><a href="/admin/users">{t.adminUsersTitle}</a><a href="/admin/emby">{t.adminEmbyTitle}</a><a href="/admin/telegram">{t.adminTelegramTitle}</a><a href="/admin/telegram-rebind-requests">{t.adminTelegramRebind}</a><a href="/admin/email">{t.adminEmailTitle}</a><a href="/admin/tickets">{t.adminTicketsTitle}</a><a href="/admin/requests">{t.adminRequestsTitle}</a><a href="/admin/invite">{t.adminInviteTitle}</a><a href="/admin/regcodes">{t.adminRegcodesTitle}</a><a href="/admin/audit-logs">{t.adminAuditLogTitle}</a><a href="/admin/violations">{t.adminViolationsTitle}</a><a href="/admin/announcements">{t.adminAnnouncementsTitle}</a><a href="/admin/logs">{t.adminRuntimeLogsTitle}</a><a href="/admin/config">{t.adminConfigTitle}</a><a href="/admin/database">{t.adminDatabaseTitle}</a>{/if}
        <form method="POST" action="/logout">
          <button type="submit">{t.logout}</button>
        </form>
      </nav>
    {/if}
  </header>
  <main class="page-frame">{@render children()}</main>
</div>

<style>
  :global(*) { box-sizing: border-box; }
  :global(html) { background: #f4f6f8; color: #17202a; font-family: system-ui, sans-serif; }
  :global(body) { margin: 0; min-width: 320px; }
  :global(button), :global(input) { font: inherit; }
  :global(button) { min-height: 2.5rem; }
  :global(a) { color: #245b75; }
  .app-shell { min-height: 100dvh; }
  .topbar { align-items: center; background: #17202a; color: #fff; display: flex; gap: 1rem; justify-content: space-between; min-height: 3.75rem; padding: 0.75rem max(1rem, env(safe-area-inset-right)) 0.75rem max(1rem, env(safe-area-inset-left)); }
  .brand { color: inherit; font-weight: 700; text-decoration: none; }
  nav { align-items: center; display: flex; flex-wrap: wrap; gap: 0.75rem; justify-content: flex-end; }
  nav a, nav button { background: transparent; border: 0; color: inherit; cursor: pointer; font: inherit; min-height: 2.25rem; padding: 0.5rem; text-decoration: none; }
  nav a:hover, nav button:hover { background: #2b3b4b; }
  .page-frame { margin: 0 auto; max-width: 72rem; padding: 1.25rem max(1rem, env(safe-area-inset-right)) 3rem max(1rem, env(safe-area-inset-left)); }
  @media (prefers-reduced-motion: reduce) { :global(*), :global(*::before), :global(*::after) { scroll-behavior: auto !important; transition-duration: 0.01ms !important; animation-duration: 0.01ms !important; } }
  @media (max-width: 560px) { .topbar { align-items: flex-start; flex-direction: column; } nav { justify-content: flex-start; width: 100%; } .page-frame { padding-top: 1rem; } }
</style>
