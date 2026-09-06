<script lang="ts">
  import { page } from "$app/state";
  import { safeAvatarPath } from "$lib/assets";
  import { t } from "$lib/i18n";
  import { accountNavigation, adminNavigation, isActivePath, primaryNavigation } from "$lib/navigation";
  import type { UserInfo } from "$lib/types";
  import type { Snippet } from "svelte";

  let { user, children }: { user: UserInfo | null; children: Snippet } = $props();
  let pathname = $derived(page.url.pathname);
  let avatar = $derived(safeAvatarPath(user?.avatar));
</script>

<svelte:head>
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
  <meta name="theme-color" content="#17202a" />
  <title>{t.siteName}</title>
</svelte:head>

<div class="app-shell">
  <header class="topbar">
    <a class="brand" href={user ? "/dashboard" : "/login"}>{t.siteName}</a>
    {#if user}
      <nav class="primary-nav" aria-label="主导航">
        {#each primaryNavigation as item (item.href)}
          <a href={item.href} aria-current={isActivePath(pathname, item) ? "page" : undefined}>{t[item.label]}</a>
        {/each}
      </nav>
      <div class="menu-bar">
        <details class="menu">
          <summary aria-label={t.account}>
            {#if avatar}<img src={avatar} alt="" width="28" height="28" />{:else}<span class="avatar-fallback" aria-hidden="true">{user.username.slice(0, 1)}</span>{/if}
            <span class="account-name">{user.username}</span>
          </summary>
          <div class="menu-popover account-popover">
            <div class="menu-heading"><strong>{user.username}</strong><span>{t.account}</span></div>
            <div class="menu-links">
              {#each accountNavigation as item (item.href)}
                <a href={item.href} aria-current={isActivePath(pathname, item) ? "page" : undefined}>{t[item.label]}</a>
              {/each}
            </div>
            <form method="POST" action="/logout"><button type="submit">{t.logout}</button></form>
          </div>
        </details>
        {#if user.role === 0}
          <details class="menu admin-menu">
            <summary>{t.adminHomeTitle}</summary>
            <div class="menu-popover admin-popover">
              {#each adminNavigation as group (group.label)}
                <section class="menu-group" aria-label={t[group.label]}>
                  <h2>{t[group.label]}</h2>
                  <div class="menu-links">
                    {#each group.items as item (item.href)}
                      <a href={item.href} aria-current={isActivePath(pathname, item) ? "page" : undefined}>{t[item.label]}</a>
                    {/each}
                  </div>
                </section>
              {/each}
            </div>
          </details>
        {/if}
      </div>
    {/if}
  </header>
  <main class="page-frame">{@render children()}</main>
</div>

<style>
  .app-shell { min-height: 100dvh; }
  .topbar { align-items: center; background: #17202a; color: #fff; display: grid; gap: .65rem 1rem; grid-template-columns: auto minmax(0, 1fr) auto; min-height: 3.75rem; padding: .65rem max(1rem, env(safe-area-inset-right)) .65rem max(1rem, env(safe-area-inset-left)); position: sticky; top: 0; z-index: 20; }
  .brand { color: inherit; font-weight: 700; text-decoration: none; }
  .primary-nav { align-items: center; display: flex; gap: .2rem; min-width: 0; overflow-x: auto; overscroll-behavior-x: contain; scrollbar-color: #607487 #17202a; scrollbar-width: thin; }
  .primary-nav a, summary, .menu-popover a, .menu-popover button { border: 0; color: inherit; cursor: pointer; font: inherit; min-height: 2.3rem; text-decoration: none; }
  .primary-nav a { border-radius: .3rem; flex: 0 0 auto; padding: .5rem .65rem; white-space: nowrap; }
  .primary-nav a:hover, .primary-nav a[aria-current="page"] { background: #2b3b4b; }
  .menu-bar { align-items: center; display: flex; flex: 0 0 auto; gap: .45rem; }
  .menu { position: relative; }
  summary { align-items: center; border-radius: .3rem; display: flex; gap: .4rem; list-style: none; padding: .35rem .55rem; white-space: nowrap; }
  summary::-webkit-details-marker { display: none; }
  summary:hover, .menu[open] summary { background: #2b3b4b; }
  summary:focus-visible, .menu-popover a:focus-visible, .menu-popover button:focus-visible, .primary-nav a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  summary img, .avatar-fallback { border-radius: 50%; height: 1.75rem; object-fit: cover; width: 1.75rem; }
  .avatar-fallback { align-items: center; background: #486581; display: inline-flex; font-size: .78rem; font-weight: 700; justify-content: center; text-transform: uppercase; }
  .menu-popover { background: #fff; border: 1px solid #c8d2da; border-radius: .45rem; box-shadow: 0 12px 30px rgb(13 24 33 / 20%); color: #17202a; max-height: min(70dvh, 42rem); min-width: min(19rem, 92vw); overflow: auto; overscroll-behavior: contain; padding: .55rem; position: absolute; right: 0; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; top: calc(100% + .5rem); width: min(27rem, 92vw); z-index: 30; }
  .menu-heading { border-bottom: 1px solid #e1e8ed; display: grid; gap: .15rem; padding: .5rem .6rem .65rem; }
  .menu-heading span { color: #52606d; font-size: .8rem; }
  .menu-links { display: grid; gap: .15rem; }
  .menu-links a, .menu-popover button { align-items: center; border-radius: .3rem; color: #243b53; display: flex; font-weight: 600; padding: .5rem .6rem; text-align: left; width: 100%; }
  .menu-links a:hover, .menu-links a[aria-current="page"], .menu-popover button:hover { background: #e8eef2; }
  .menu-popover form { border-top: 1px solid #e1e8ed; margin-top: .55rem; padding-top: .55rem; }
  .menu-popover button { background: #fff1f0; color: #a63d40; }
  .admin-popover { display: grid; gap: .65rem; width: min(36rem, 94vw); }
  .menu-group { border-bottom: 1px solid #e1e8ed; display: grid; gap: .25rem; padding: .35rem .1rem .65rem; }
  .menu-group:last-child { border-bottom: 0; padding-bottom: .1rem; }
  .menu-group h2 { color: #52606d; font-size: .78rem; margin: 0; padding: 0 .5rem; }
  .page-frame { margin: 0 auto; max-width: 72rem; padding: 1.25rem max(1rem, env(safe-area-inset-right)) 3rem max(1rem, env(safe-area-inset-left)); }
  @media (max-width: 760px) {
    .topbar { grid-template-columns: minmax(0, 1fr) auto; }
    .primary-nav { grid-column: 1 / -1; grid-row: 2; }
    .account-name { display: none; }
    .menu-popover { max-height: 65dvh; }
    .page-frame { padding-top: 1rem; }
  }
  @media (max-width: 420px) {
    .menu-bar { gap: .2rem; }
    summary { padding-inline: .4rem; }
    .admin-popover { right: min(-3.5rem, calc(100vw - 18rem)); }
  }
</style>
