<script lang="ts">
  import { t } from "$lib/i18n";
  import { mediaPoster, mediaStatusLabel, mediaTitle, safeExternalURL, safeImageURL } from "$lib/media";
  import type { MediaDetail, MediaItem, MediaRequest } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let action = $derived((form ?? {}) as FormState);
  let detail = $derived(data.detail as MediaDetail | null);
  let inventory = $derived(data.inventory);
  let tab = $derived(data.tab);
  let landscapeIDs = $state<Set<string>>(new Set());
  let failedImageIDs = $state<Set<string>>(new Set());
  let detailImageFailed = $state(false);

  $effect(() => {
    data.selectedID;
    failedImageIDs = new Set();
    landscapeIDs = new Set();
    detailImageFailed = false;
  });

  function dateLabel(value: number): string {
    return value ? new Date(value * 1000).toLocaleString("zh-CN") : "-";
  }

  function title(item: MediaItem | MediaDetail | MediaRequest): string {
    return mediaTitle(item) || t.mediaUnknown;
  }

  function poster(item: MediaItem | MediaDetail | MediaRequest): string {
    return mediaPoster(item);
  }

  function logo(item: MediaItem | MediaDetail): string {
    return safeImageURL(item.logo_url || item.logo);
  }

  function mediaTypeLabel(item: MediaItem | MediaDetail): string {
    if (item.source === "bangumi") return t.mediaBangumi;
    return item.media_type === "tv" ? t.mediaTV : t.mediaMovie;
  }

  function sourceLabel(source: string): string {
    return source === "bangumi" ? t.mediaBangumi : t.mediaTMDB;
  }

  function detailHref(item: MediaItem): string {
    const params = new URLSearchParams({
      q: data.query,
      source: item.source === "bangumi" ? "bangumi" : item.source === "tmdb" ? "tmdb" : "all",
      type: item.media_type === "tv" ? "tv" : "movie",
      media_id: String(item.id)
    });
    return `/media?${params.toString()}`;
  }

  function availableSeasons(): string {
    return inventory?.seasons_available?.length ? inventory.seasons_available.join(", ") : "-";
  }

  function resultKey(item: MediaItem): string {
    return `${item.source}:${item.id}:${item.media_type}`;
  }

  function markOrientation(item: MediaItem, event: Event): void {
    const image = event.currentTarget as HTMLImageElement;
    const key = resultKey(item);
    const next = new Set(landscapeIDs);
    if (image.naturalWidth > image.naturalHeight) next.add(key);
    else next.delete(key);
    landscapeIDs = next;
  }

  function markImageFailed(key: string): void {
    failedImageIDs = new Set([...failedImageIDs, key]);
  }

  function resultGroups(): { landscape: MediaItem[]; portrait: MediaItem[] } {
    const landscape: MediaItem[] = [];
    const portrait: MediaItem[] = [];
    for (const item of data.results) {
      if (landscapeIDs.has(resultKey(item))) landscape.push(item);
      else portrait.push(item);
    }
    return { landscape, portrait };
  }

  async function copyKey(value: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(value);
      window.alert(t.mediaCopied);
    } catch {
      window.alert(t.mediaCopyFailed);
    }
  }
</script>

<svelte:head><title>{t.media} - {t.siteName}</title></svelte:head>

<section class="media-page" aria-labelledby="media-title">
  <header class="page-heading">
    <div>
      <p class="eyebrow">{t.siteName}</p>
      <h1 id="media-title">{t.media}</h1>
      <p class="muted">{t.mediaIntro}</p>
    </div>
    <div class="heading-actions">
      <a class="text-link" href="/dashboard">{t.mediaBackDashboard}</a>
      <a class="button secondary" href="/media">{t.mediaRefresh}</a>
    </div>
  </header>

  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.result === "created"}<p class="notice success" role="status">{t.mediaResultCreated}</p>{/if}
  {#if data.result === "deleted"}<p class="notice success" role="status">{t.mediaResultDeleted}</p>{/if}

  <nav class="tabs" aria-label={t.media}>
    <a class:active={tab === "search"} href={`/media${data.query ? `?q=${encodeURIComponent(data.query)}&source=${encodeURIComponent(data.source)}&type=${encodeURIComponent(data.mediaType)}` : ""}`}>{t.mediaSearch}</a>
    <a class:active={tab === "requests"} href="/media?tab=requests">{t.mediaRequests}</a>
  </nav>

  {#if tab === "requests"}
    <section class="panel requests-panel" aria-labelledby="requests-title">
      <div class="section-heading"><div><h2 id="requests-title">{t.mediaRequests}</h2><p class="muted">{t.mediaRequestsIntro}</p></div><a class="button secondary" href="/media?tab=requests">{t.mediaRequestRefresh}</a></div>
      {#if data.requestsFailed}<p class="notice error">{t.mediaRequestsLoadFailed}</p>
      {:else if data.requests && data.requests.length}
        <div class="request-list">
          {#each data.requests as request (request.require_key)}
            <article class="request-row">
              {#if poster(request)}<img class="request-poster" src={poster(request)} alt={t.mediaPosterAlt.replace("{title}", title(request))} loading="lazy" />{:else}<div class="poster-fallback small">{t.mediaNoPoster}</div>{/if}
              <div class="request-body">
                <div class="request-head"><div><h3>{title(request)}</h3><p class="meta">{sourceLabel(request.source)} #{request.media_id}{request.season ? ` · ${t.mediaSeason.replace("{season}", String(request.season))}` : ""}</p></div><span class="status" data-status={request.status}>{mediaStatusLabel(request.status)}</span></div>
                {#if request.note}<p class="request-note">{request.note}</p>{/if}
                {#if request.admin_note}<p class="admin-note">{t.mediaRequestAdminNote.replace("{note}", request.admin_note)}</p>{/if}
                <div class="request-foot"><small>{t.mediaRequestCreatedAt.replace("{date}", dateLabel(request.timestamp))}</small><code>{request.require_key}</code><button class="link-button" type="button" onclick={() => copyKey(request.require_key)}>{t.mediaCopyKey}</button><form method="POST" action="?/deleteRequest" onsubmit={(event) => { if (!confirm(t.mediaRequestDeleteConfirm)) event.preventDefault(); }}><input type="hidden" name="require_key" value={request.require_key} /><button class="danger-link" type="submit">{t.mediaRequestDelete}</button></form></div>
              </div>
            </article>
          {/each}
        </div>
      {:else}<div class="empty"><p>{t.mediaRequestsEmpty}</p><a class="button secondary" href="/media">{t.mediaSearch}</a></div>{/if}
    </section>
  {:else}
    <section class="panel search-panel" aria-labelledby="search-title">
      <form class="search-form" method="GET" action="/media">
        <label class="search-input"><span class="sr-only">{t.mediaSearch}</span><input name="q" value={data.query} placeholder={t.mediaSearchPlaceholder} maxlength="120" /></label>
        <label class="select-field"><span>{t.mediaSearchSource}</span><select name="source" value={data.source}><option value="all">{t.mediaAllSources}</option><option value="tmdb">{t.mediaTMDB}</option><option value="bangumi">{t.mediaBangumi}</option></select></label>
        <label class="select-field"><span>{t.mediaType}</span><select name="type" value={data.mediaType}><option value="movie">{t.mediaMovie}</option><option value="tv">{t.mediaTV}</option></select></label>
        <button class="button primary search-button" type="submit">{t.mediaSearchSubmit}</button>
      </form>
    </section>

    {#if data.searchFailed}<p class="notice error">{t.mediaSearchFailed}</p>{/if}
    {#if data.searchWarning}<p class="notice warning">{data.searchWarning}</p>{/if}
    {#if data.results.length}
      <section class="panel results-panel" aria-labelledby="results-title">
        <div class="section-heading"><div><h2 id="results-title">{t.mediaResults}</h2><p class="muted">{t.mediaResultCount.replace("{count}", String(data.results.length))}</p></div></div>
        {#if resultGroups().landscape.length}
          <h3 class="result-group-title">{t.mediaLandscapeResults}</h3>
          <div class="result-grid landscape-grid">
            {#each resultGroups().landscape as item (resultKey(item))}
              <a class:selected={String(item.id) === data.selectedID && (data.source === "all" || item.source === data.source)} class="result-card" href={detailHref(item)}>
                {#if poster(item) && !failedImageIDs.has(resultKey(item))}<img src={poster(item)} alt={t.mediaPosterAlt.replace("{title}", title(item))} loading="lazy" onload={(event) => markOrientation(item, event)} onerror={() => markImageFailed(resultKey(item))} />{:else}<div class="poster-fallback">{t.mediaNoPoster}</div>{/if}
                <span class="result-info"><strong>{title(item)}</strong>{#if item.original_title && item.original_title !== title(item)}<small>{item.original_title}</small>{/if}<span class="result-meta"><b>{sourceLabel(item.source)}</b>{mediaTypeLabel(item)}{#if item.year} · {item.year}{/if}{#if (item.rating || item.vote_average)} · {t.mediaRating.replace("{score}", Number(item.rating || item.vote_average).toFixed(1))}{/if}</span></span>
              </a>
            {/each}
          </div>
        {/if}
        {#if resultGroups().portrait.length}<h3 class="result-group-title">{t.mediaPortraitResults}</h3>
        <div class="result-grid portrait-grid">
          {#each resultGroups().portrait as item (resultKey(item))}
            <a class:selected={String(item.id) === data.selectedID && (data.source === "all" || item.source === data.source)} class="result-card" href={detailHref(item)}>
              {#if poster(item) && !failedImageIDs.has(resultKey(item))}<img src={poster(item)} alt={t.mediaPosterAlt.replace("{title}", title(item))} loading="lazy" onload={(event) => markOrientation(item, event)} onerror={() => markImageFailed(resultKey(item))} />{:else}<div class="poster-fallback">{t.mediaNoPoster}</div>{/if}
              <span class="result-info"><strong>{title(item)}</strong>{#if item.original_title && item.original_title !== title(item)}<small>{item.original_title}</small>{/if}<span class="result-meta"><b>{sourceLabel(item.source)}</b>{mediaTypeLabel(item)}{#if item.year} · {item.year}{/if}{#if (item.rating || item.vote_average)} · {t.mediaRating.replace("{score}", Number(item.rating || item.vote_average).toFixed(1))}{/if}</span></span>
            </a>
          {/each}
        </div>{/if}
      </section>
    {:else if data.query && !data.searchFailed}<div class="empty panel"><p>{t.mediaNoResults}</p></div>{:else if !data.query}<div class="empty panel"><p>{t.mediaSearchEmpty}</p></div>{/if}

    {#if detail}
      <section class="detail-layout" aria-labelledby="detail-title">
        <div class="detail-poster">
          {#if poster(detail) && !detailImageFailed}<img src={poster(detail)} alt={t.mediaPosterAlt.replace("{title}", title(detail))} onerror={() => detailImageFailed = true} />{:else}<div class="poster-fallback detail-fallback">{t.mediaNoPoster}</div>{/if}
        </div>
        <article class="panel detail-panel">
          <div class="detail-heading"><div class="detail-title-block">{#if logo(detail)}<img class="detail-logo" src={logo(detail)} alt={title(detail)} loading="lazy" />{/if}<div class="detail-title-text"><span class="source-badge">{sourceLabel(detail.source)} · {mediaTypeLabel(detail)}</span><h2 id="detail-title">{title(detail)}</h2>{#if detail.original_title && detail.original_title !== title(detail)}<p class="original-title">{t.mediaOriginalTitle}：{detail.original_title}</p>{/if}</div></div>{#if detail.rating || detail.vote_average}<strong class="rating">★ {Number(detail.rating || detail.vote_average).toFixed(1)}</strong>{/if}</div>
          <div class="detail-facts">{#if detail.release_date || detail.year}<span><b>{t.mediaReleaseDate}</b>{detail.release_date || detail.year}</span>{/if}{#if detail.runtime}<span><b>{t.mediaRuntime.replace("{minutes}", String(detail.runtime))}</b></span>{/if}{#if detail.episodes}<span><b>{t.mediaEpisodes.replace("{count}", String(detail.episodes))}</b></span>{/if}{#if detail.seasons}<span><b>{t.mediaSeasons.replace("{count}", String(detail.seasons))}</b></span>{/if}{#if detail.volumes}<span><b>{t.mediaVolumes.replace("{count}", String(detail.volumes))}</b></span>{/if}{#if detail.vote_count}<span><b>{t.mediaVotes.replace("{count}", String(detail.vote_count))}</b></span>{/if}</div>
          {#if detail.genres?.length}<p class="chips">{#each detail.genres as genre}<span>{genre}</span>{/each}</p>{/if}
          <section class="copy-section"><h3>{t.mediaOverview}</h3><p>{detail.overview || t.mediaNoOverview}</p></section>
          {#if detail.aliases?.length}<section class="copy-section"><h3>{t.mediaAliases}</h3><p>{detail.aliases.join(" / ")}</p></section>{/if}
          {#if detail.creators?.length || detail.cast?.length}<section class="copy-section"><h3>{t.mediaCreators}</h3><p>{[...(detail.creators || []), ...(detail.cast || [])].join(" / ")}</p></section>{/if}
          <div class="detail-links">{#if safeExternalURL(detail.source_url)}<a href={safeExternalURL(detail.source_url)} target="_blank" rel="noopener noreferrer">{t.mediaOpenSource}</a>{/if}{#if safeExternalURL(detail.official_url)}<a href={safeExternalURL(detail.official_url)} target="_blank" rel="noopener noreferrer">{t.mediaOpenOfficial}</a>{/if}</div>
          <section class:available={inventory?.exists} class="inventory"><h3>{t.mediaInventory}</h3>{#if inventory}<strong>{inventory.exists ? t.mediaInventoryAvailable : t.mediaInventoryMissing}</strong><p>{inventory.message}</p>{#if inventory.seasons_available?.length}<small>{t.mediaSeasonsAvailable.replace("{seasons}", availableSeasons())}</small>{/if}{:else}<p>{t.mediaInventoryFailed}</p>{/if}</section>
          <form class="request-form" method="POST" action="?/create">
            <input type="hidden" name="source" value={detail.source} /><input type="hidden" name="media_id" value={detail.id} /><input type="hidden" name="media_type" value={detail.media_type === "tv" ? "tv" : detail.media_type} /><input type="hidden" name="title" value={detail.title} /><input type="hidden" name="original_title" value={detail.original_title || ""} /><input type="hidden" name="poster" value={detail.poster || ""} /><input type="hidden" name="poster_url" value={detail.poster_url || ""} /><input type="hidden" name="overview" value={detail.overview || ""} /><input type="hidden" name="year" value={detail.year || ""} />
            {#if detail.seasons && detail.seasons > 0}<label for="request-season">{t.mediaSeasons}<select id="request-season" name="season"><option value="">{t.mediaAllSeasons}</option>{#each Array.from({ length: detail.seasons }, (_, index) => index + 1) as season}<option value={season} selected={data.selectedSeason === season}>{t.mediaSeason.replace("{season}", String(season))}{inventory?.seasons_available?.includes(season) ? ` · ${t.mediaSeasonInLibrary}` : ""}</option>{/each}</select></label>{/if}
            <label for="request-note">{inventory?.exists ? t.mediaRequestIssue : t.mediaRequest}><textarea id="request-note" name="note" maxlength="500" rows="3" required={Boolean(inventory?.exists)} placeholder={inventory?.exists ? t.mediaRequestNoteRequired : t.mediaRequestNotePlaceholder}></textarea></label>
            <button class="button primary" type="submit">{inventory?.exists ? t.mediaRequestIssue : t.mediaSubmitRequest}</button>
          </form>
        </article>
      </section>
    {/if}
  {/if}
</section>

<style>
  .media-page { display: grid; gap: 1rem; min-width: 0; }
  .page-heading, .section-heading, .heading-actions, .detail-heading, .request-head, .request-foot { align-items: flex-start; display: flex; gap: .75rem; }
  .page-heading, .section-heading, .detail-heading { justify-content: space-between; }
  .page-heading { border-bottom: 1px solid #d7dee5; padding-bottom: 1.15rem; }
  .heading-actions { align-items: center; flex-wrap: wrap; }
  .eyebrow { color: #486581; font-size: .8rem; font-weight: 700; letter-spacing: .08em; margin: 0 0 .45rem; text-transform: uppercase; }
  h1, h2, h3, p { overflow-wrap: anywhere; } h1, h2, h3 { margin: 0; } h1 { font-size: 2.1rem; } h2 { font-size: 1.2rem; } h3 { font-size: .95rem; }
  .muted, .meta, small { color: #52606d; } .muted { margin: .4rem 0 0; }
  .text-link { min-height: 2.5rem; padding: .55rem 0; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.5rem; max-width: 100%; padding: .5rem .85rem; text-decoration: none; } .button.primary { background: #245b75; color: #fff; } .button.secondary { background: #e8eef2; color: #16394a; } .button:disabled { cursor: not-allowed; opacity: .55; }
  .panel, .notice { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; padding: 1rem; } .notice { margin: 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.warning { background: #fff8e6; border-color: #e9c46a; color: #7b4f00; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; }
  .tabs { border-bottom: 1px solid #d7dee5; display: flex; gap: .25rem; overflow-x: auto; } .tabs a { border-bottom: 3px solid transparent; color: #52606d; min-height: 2.75rem; padding: .7rem 1rem .55rem; text-decoration: none; white-space: nowrap; } .tabs a.active { border-color: #245b75; color: #16394a; font-weight: 700; }
  .search-form { align-items: end; display: grid; gap: .75rem; grid-template-columns: minmax(12rem, 1fr) minmax(8rem, 10rem) minmax(8rem, 10rem) auto; } .search-input, .select-field, .request-form label { color: #243b53; display: grid; font-size: .82rem; font-weight: 650; gap: .35rem; min-width: 0; } .search-input span, .select-field span { height: 1px; overflow: hidden; position: absolute; width: 1px; clip: rect(0 0 0 0); }
  input, select, textarea { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.5rem; min-width: 0; max-width: 100%; padding: .5rem .65rem; } textarea { resize: vertical; } input:focus-visible, select:focus-visible, textarea:focus-visible, button:focus-visible, a:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  .result-group-title { color: #243b53; font-size: .9rem; margin: 1rem 0 0; } .result-grid { display: grid; gap: 1rem; grid-template-columns: repeat(5, minmax(0, 1fr)); margin-top: .55rem; } .result-card { border: 1px solid #d7dee5; color: inherit; display: flex; flex-direction: column; min-width: 0; overflow: hidden; text-decoration: none; } .result-card:hover, .result-card.selected { border-color: #245b75; } .result-card img, .detail-poster img, .request-poster { display: block; height: auto; max-width: 100%; object-fit: contain; } .result-card img { width: 100%; } .result-info { display: grid; gap: .25rem; min-width: 0; padding: .65rem; } .result-info strong, .result-info small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } .result-meta { color: #52606d; display: block; font-size: .75rem; overflow-wrap: anywhere; } .result-meta b { color: #245b75; margin-right: .35rem; }
  .poster-fallback { align-items: center; background: #eef2f4; color: #52606d; display: flex; justify-content: center; min-height: 14rem; padding: 1rem; text-align: center; } .poster-fallback.small { flex: 0 0 6rem; min-height: 8.5rem; width: 6rem; } .detail-fallback { min-height: 28rem; width: 100%; }
  .detail-layout { align-items: start; display: grid; gap: 1rem; grid-template-columns: minmax(12rem, 28rem) minmax(0, 1fr); } .detail-poster { min-width: 0; } .detail-poster img { width: 100%; } .detail-panel { display: grid; gap: 1rem; min-width: 0; } .detail-heading { border-bottom: 1px solid #e1e8ed; padding-bottom: .9rem; } .detail-heading > div { min-width: 0; } .detail-title-block { align-items: center; display: flex; gap: .9rem; min-width: 0; } .detail-title-text { min-width: 0; } .detail-logo { display: block; height: auto; max-height: 6rem; max-width: min(45%, 18rem); object-fit: contain; } .source-badge { color: #245b75; font-size: .78rem; font-weight: 700; } .detail-heading h2 { margin-top: .35rem; } .original-title { color: #52606d; margin: .35rem 0 0; } .rating { color: #8a5a00; white-space: nowrap; }
  .detail-facts { border-bottom: 1px solid #e1e8ed; display: flex; flex-wrap: wrap; gap: .65rem 1rem; padding-bottom: .9rem; } .detail-facts span { color: #52606d; font-size: .82rem; } .detail-facts b { color: #243b53; display: block; font-size: .72rem; margin-bottom: .15rem; }
  .chips { display: flex; flex-wrap: wrap; gap: .4rem; margin: 0; } .chips span { background: #eef2f4; border: 1px solid #d7dee5; border-radius: 999px; color: #486581; font-size: .75rem; padding: .2rem .5rem; }
  .copy-section { border-top: 1px solid #e1e8ed; padding-top: .9rem; } .copy-section p { color: #52606d; line-height: 1.65; margin: .4rem 0 0; white-space: pre-line; }
  .detail-links { display: flex; flex-wrap: wrap; gap: .75rem; } .detail-links a { color: #245b75; min-height: 2.25rem; padding: .45rem 0; }
  .inventory { background: #fff8e6; border: 1px solid #e9c46a; display: grid; gap: .3rem; padding: .8rem; } .inventory.available { background: #edf7f0; border-color: #a9d5b4; } .inventory p, .inventory small { color: #52606d; margin: 0; }
  .request-form { border-top: 1px solid #e1e8ed; display: grid; gap: .65rem; padding-top: .9rem; } .request-form .button { justify-self: start; }
  .request-list { display: grid; gap: .75rem; margin-top: 1rem; max-height: 65dvh; overflow: auto; overscroll-behavior: contain; scrollbar-color: #9fb3c8 #eef2f4; scrollbar-width: thin; } .request-row { border-bottom: 1px solid #e1e8ed; display: flex; gap: .75rem; min-width: 0; padding: .75rem .1rem; } .request-body { display: grid; gap: .45rem; min-width: 0; flex: 1; } .request-head { justify-content: space-between; } .request-head > div { min-width: 0; } .request-head h3 { overflow-wrap: anywhere; } .status { border: 1px solid #c8d2da; border-radius: 999px; color: #52606d; flex: 0 0 auto; font-size: .72rem; padding: .2rem .45rem; white-space: nowrap; } .status[data-status="completed"] { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .status[data-status="rejected"] { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .status[data-status="accepted"], .status[data-status="downloading"] { background: #edf5f8; border-color: #a9c8d5; color: #245b75; } .request-note, .admin-note { color: #52606d; margin: 0; overflow-wrap: anywhere; white-space: pre-line; } .admin-note { border-left: 2px solid #9fb3c8; padding-left: .55rem; }
  .request-foot { align-items: center; color: #52606d; flex-wrap: wrap; font-size: .75rem; } .request-foot code { max-width: 15rem; overflow: hidden; text-overflow: ellipsis; } .link-button, .danger-link { background: transparent; border: 0; color: #245b75; cursor: pointer; font: inherit; min-height: 2rem; padding: .35rem 0; } .danger-link { color: #a63d40; } .request-foot form { margin-left: auto; }
  .empty { align-items: center; display: grid; gap: .75rem; justify-items: center; min-height: 12rem; text-align: center; } .empty p { color: #52606d; margin: 0; }
  .sr-only { clip: rect(0, 0, 0, 0); clip-path: inset(50%); height: 1px; overflow: hidden; position: absolute; white-space: nowrap; width: 1px; }
  @media (max-width: 900px) { .search-form { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); } .search-input { grid-column: 1 / -1; } .search-button { width: 100%; } .result-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); } .detail-layout { grid-template-columns: minmax(10rem, 20rem) minmax(0, 1fr); } }
  @media (max-width: 640px) { .page-heading, .section-heading, .detail-heading { flex-direction: column; } .heading-actions { align-items: stretch; width: 100%; } .heading-actions .button { width: 100%; } .search-form { grid-template-columns: 1fr; } .search-input { grid-column: auto; } .result-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .detail-layout { grid-template-columns: 1fr; } .detail-poster { justify-self: center; max-width: 100%; width: min(100%, 24rem); } .detail-panel { padding: .85rem; } .detail-title-block { align-items: flex-start; width: 100%; } .detail-logo { max-width: 42%; } .request-head { align-items: flex-start; flex-direction: column; } .request-foot form { margin-left: 0; } h1 { font-size: 1.8rem; } }
</style>
