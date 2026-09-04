<script lang="ts">
  import { t } from "$lib/i18n";
  import type { BangumiCollectionEntry } from "$lib/types";
  import type { PageData } from "./$types";

  type FormState = { action?: string; error?: string };
  let { data, form }: { data: PageData; form: unknown } = $props();
  let pageData = $derived(data.pageData);
  let entries = $derived(pageData?.entries || []);
  let action = $derived((form ?? {}) as FormState);
  let currentPage = $derived(data.page ?? 1);
  let perPage = $derived(data.perPage ?? 24);
  let tagFilter = $state("");
  let sortBy = $state("default");
  let viewMode = $state<"grid" | "list">("grid");

  $effect(() => {
    tagFilter = data.filters.tag;
    sortBy = data.filters.sort;
  });

  const metadata: Record<number, { title: string; label: string }> = {
    1: { title: t.bangumiCollectionWishlist, label: t.bangumiCollectionWishlist },
    2: { title: t.bangumiCollectionCollected, label: t.bangumiCollectionCollected },
    3: { title: t.bangumiCollectionWatching, label: t.bangumiCollectionWatching },
    4: { title: t.bangumiCollectionOnHold, label: t.bangumiCollectionOnHold },
    5: { title: t.bangumiCollectionDropped, label: t.bangumiCollectionDropped }
  };
  let meta = $derived(metadata[data.collectionType] || metadata[3]);

  function itemTitle(item: BangumiCollectionEntry): string {
    return item.subject?.name_cn || item.subject?.name || t.bangumiUnknownItem;
  }

  function tags(item: BangumiCollectionEntry): string[] {
    return item.subject?.tags || [];
  }

  function poster(item: BangumiCollectionEntry): string {
    return `/api/v1/bangumi/cover/${item.subject_id}`;
  }

  function dateLabel(value: number): string {
    if (!value) return "-";
    return new Date(value * 1000).toLocaleString("zh-CN");
  }

  function visibleEntries(): BangumiCollectionEntry[] {
    const filter = tagFilter.trim().toLocaleLowerCase();
    const list = entries.filter((item) => !filter || tags(item).some((tag) => tag.toLocaleLowerCase().includes(filter)));
    return [...list].sort((left, right) => {
      if (sortBy === "ep_asc") return left.ep_status - right.ep_status;
      if (sortBy === "ep_desc") return right.ep_status - left.ep_status;
      if (sortBy === "date_desc") return right.updated_at - left.updated_at;
      if (sortBy === "rate_desc") return right.rate - left.rate;
      return 0;
    });
  }

  function pageCount(): number {
    return Math.max(1, Math.ceil((pageData?.total || 0) / perPage));
  }

  function pageHref(nextPage: number): string {
    const params = new URLSearchParams({ page: String(nextPage), per_page: String(perPage) });
    if (tagFilter) params.set("tag", tagFilter);
    if (sortBy !== "default") params.set("sort", sortBy);
    return `/bangumi/collections/${data.collectionType}?${params.toString()}`;
  }

  function refreshHref(): string {
    const params = new URLSearchParams({ page: String(currentPage), per_page: String(perPage), refresh: "1" });
    if (tagFilter) params.set("tag", tagFilter);
    if (sortBy !== "default") params.set("sort", sortBy);
    return `/bangumi/collections/${data.collectionType}?${params.toString()}`;
  }

  function clearFilterHref(): string {
    const params = new URLSearchParams({ page: String(currentPage), per_page: String(perPage) });
    if (sortBy !== "default") params.set("sort", sortBy);
    return `/bangumi/collections/${data.collectionType}?${params.toString()}`;
  }

  function statusOptions(): Array<[number, string]> {
    return [[1, t.bangumiCollectionWishlist], [2, t.bangumiCollectionCollected], [3, t.bangumiCollectionWatching], [4, t.bangumiCollectionOnHold], [5, t.bangumiCollectionDropped]];
  }
</script>

<svelte:head><title>{meta.title} - {t.bangumiTitle}</title></svelte:head>

<section class="collection-page" aria-labelledby="collection-title">
  <header class="page-heading">
    <div><a class="back-link" href="/bangumi">{t.bangumiCollectionBack}</a><h1 id="collection-title">{meta.title}</h1><p class="muted">{pageData ? t.bangumiCollectionPageSummary.replace("{from}", pageData.total ? String(pageData.offset + 1) : "0").replace("{to}", String(Math.min(pageData.offset + pageData.entries.length, pageData.total))).replace("{total}", String(pageData.total)) : "-"}</p></div>
    {#if pageData}<a class="button secondary" href={refreshHref()}>{t.bangumiRefresh}</a>{/if}
  </header>

  {#if data.loadError}<p class="notice error" role="alert">{data.loadError}</p>{/if}
  {#if action.error}<p class="notice error" role="alert">{action.error}</p>{/if}
  {#if data.result === "updated"}<p class="notice success" role="status">{t.bangumiCollectionSaved}</p>{/if}

  {#if pageData}
      <section class="toolbar" aria-label={t.bangumiCollectionTitle.replace("{name}", meta.label)}>
      <div class="toolbar-status"><span class="badge">{pageData.cached ? t.bangumiCollectionCache : t.bangumiCollectionLive}</span>{#if pageData.cache_updated_at}<span class="muted">{t.bangumiCollectionCachedAt.replace("{date}", dateLabel(pageData.cache_updated_at))}</span>{/if}</div>
      <form method="GET" class="filters"><input type="hidden" name="page" value="1" /><input type="hidden" name="per_page" value={perPage} /><label>{t.bangumiCollectionFilter}<input name="tag" bind:value={tagFilter} placeholder={t.bangumiCollectionFilterPlaceholder} /></label><label>{t.bangumiCollectionSort}<select name="sort" bind:value={sortBy}><option value="default">{t.bangumiCollectionSortDefault}</option><option value="ep_asc">{t.bangumiCollectionSortEpisodeAsc}</option><option value="ep_desc">{t.bangumiCollectionSortEpisodeDesc}</option><option value="date_desc">{t.bangumiCollectionSortUpdated}</option><option value="rate_desc">{t.bangumiCollectionSortRate}</option></select></label><button class="button secondary" type="submit">{t.bangumiCollectionApplyFilter}</button></form>
      <div class="view-switch" role="group" aria-label={t.bangumiCollectionView}><button class:active={viewMode === "grid"} type="button" onclick={() => viewMode = "grid"}>{t.bangumiCollectionViewGrid}</button><button class:active={viewMode === "list"} type="button" onclick={() => viewMode = "list"}>{t.bangumiCollectionViewList}</button></div>
    </section>

    {#if visibleEntries().length === 0}
      <section class="empty panel"><p>{entries.length ? t.bangumiCollectionNoMatch : t.bangumiCollectionEmpty}</p>{#if entries.length && tagFilter}<a class="text-link" href={clearFilterHref()}>{t.bangumiCollectionClearFilter}</a>{/if}</section>
    {:else}
      <div class:grid-view={viewMode === "grid"} class:list-view={viewMode === "list"}>
        {#each visibleEntries() as item (item.subject_id)}
          <article class="media-card">
            <div class="poster-frame">{#if item.subject?.images}<img src={poster(item)} alt={itemTitle(item)} loading="lazy" referrerpolicy="no-referrer" />{:else}<div class="poster-empty">{t.bangumiCollectionNoCover}</div>{/if}</div>
            <div class="media-info"><div class="title-row"><h2 title={itemTitle(item)}>{itemTitle(item)}</h2><span class="badge">{meta.label}</span></div>{#if item.subject?.name && item.subject.name !== itemTitle(item)}<p class="original-title">{item.subject.name}</p>{/if}<div class="meta-row">{#if item.subject?.date}<span>{item.subject.date}</span>{/if}{#if item.subject?.eps}<span>{t.bangumiCollectionEpisodes.replace("{count}", String(item.subject.eps))}</span>{/if}{#if item.subject?.volumes}<span>{t.bangumiCollectionVolumeCount.replace("{count}", String(item.subject.volumes))}</span>{/if}{#if item.subject?.rating?.score}<span>{t.bangumiCollectionScore.replace("{score}", String(item.subject.rating.score))}</span>{/if}{#if item.subject?.rating?.rank}<span>{t.bangumiCollectionRank.replace("{rank}", String(item.subject.rating.rank))}</span>{/if}</div>{#if tags(item).length}<div class="tags">{#each tags(item).slice(0, 8) as tag}<span>{tag}</span>{/each}</div>{/if}<div class="state-row"><span>{item.ep_status ? t.bangumiCollectionProgress.replace("{count}", String(item.ep_status)) : t.bangumiCollectionNoProgress}</span>{#if item.rate}<span>{t.bangumiCollectionMyRate.replace("{rate}", String(item.rate))}</span>{/if}</div><details class="edit-details"><summary>{t.bangumiCollectionEdit}</summary><form method="POST" action="?/update" class="edit-form"><input type="hidden" name="subject_id" value={item.subject_id} /><input type="hidden" name="page" value={currentPage} /><input type="hidden" name="per_page" value={perPage} /><label>{t.bangumiCollectionStatus}<select name="type" value={item.type || data.collectionType}>{#each statusOptions() as [value, label]}<option value={value}>{label}</option>{/each}</select></label><label>{t.bangumiCollectionEpisodeProgress}<input name="ep_status" type="number" min="0" value={item.ep_status} /></label><label>{t.bangumiCollectionRate}<input name="rate" type="number" min="0" max="10" value={item.rate} /></label><button class="button primary" type="submit">{t.bangumiCollectionSave}</button></form></details><div class="card-footer"><a class="text-link" href={`https://bgm.tv/subject/${item.subject_id}`} target="_blank" rel="noopener noreferrer">{t.bangumiCollectionExternal}</a><small>{dateLabel(item.updated_at)}</small></div></div>
          </article>
        {/each}
      </div>
    {/if}

    <nav class="pagination" aria-label={t.bangumiPagination}><a class="button secondary" class:disabled={currentPage <= 1} href={currentPage > 1 ? pageHref(1) : pageHref(currentPage)}>{t.bangumiPageFirst}</a><a class="button secondary" class:disabled={currentPage <= 1} href={currentPage > 1 ? pageHref(currentPage - 1) : pageHref(currentPage)}>{t.bangumiPagePrevious}</a><span>{t.bangumiPageNumber.replace("{page}", String(currentPage)).replace("{pages}", String(pageCount()))}</span><a class="button secondary" class:disabled={currentPage >= pageCount()} href={currentPage < pageCount() ? pageHref(currentPage + 1) : pageHref(currentPage)}>{t.bangumiPageNext}</a><a class="button secondary" class:disabled={currentPage >= pageCount()} href={currentPage < pageCount() ? pageHref(pageCount()) : pageHref(currentPage)}>{t.bangumiPageLast}</a></nav>
  {/if}
</section>

<style>
  .collection-page { display: grid; gap: 1rem; } .page-heading, .toolbar, .toolbar-status, .filters, .view-switch, .title-row, .meta-row, .state-row, .card-footer, .pagination { align-items: center; display: flex; gap: .65rem; } .page-heading, .toolbar, .title-row, .card-footer { justify-content: space-between; } .page-heading { align-items: flex-start; border-bottom: 1px solid #d7dee5; padding-bottom: 1rem; } .page-heading h1 { margin: .45rem 0 0; } .back-link, .text-link { min-height: 2.5rem; padding: .55rem 0; } .muted, small { color: #52606d; } .muted { margin: .35rem 0 0; } h1, h2, p { overflow-wrap: anywhere; } h1, h2 { margin: 0; } h1 { font-size: clamp(1.55rem, 6vw, 2.15rem); } h2 { font-size: 1rem; }
  .button { border: 0; border-radius: .3rem; cursor: pointer; font: inherit; font-weight: 650; min-height: 2.4rem; padding: .45rem .75rem; text-decoration: none; } .button.secondary { background: #e8eef2; color: #16394a; } .button.primary { background: #245b75; color: #fff; } .button:disabled, .button.disabled { cursor: not-allowed; opacity: .5; pointer-events: none; } .notice, .panel, .toolbar { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; padding: 1rem; } .notice { margin: 0; } .notice.error { background: #fff1f0; border-color: #f1a7a0; color: #a61b1b; } .notice.success { background: #edf7f0; border-color: #a9d5b4; color: #276749; } .badge, .tags span, .state-row span { background: #eef2f4; border: 1px solid #c8d2da; border-radius: 999px; color: #36566a; font-size: .72rem; padding: .18rem .45rem; white-space: nowrap; }
  .toolbar { display: grid; gap: .8rem; grid-template-columns: auto minmax(0, 1fr) auto; } .filters { justify-content: flex-end; flex-wrap: wrap; } label { color: #243b53; display: grid; font-size: .82rem; font-weight: 650; gap: .3rem; min-width: 0; } input, select { background: #fff; border: 1px solid #9fb3c8; border-radius: .3rem; font: inherit; min-height: 2.35rem; min-width: 0; padding: .4rem .55rem; } .filters input { width: min(18rem, 100%); } .filters select { min-width: 9rem; } .view-switch { border: 1px solid #c8d2da; border-radius: .3rem; overflow: hidden; } .view-switch button { background: #fff; border: 0; cursor: pointer; min-height: 2.35rem; padding: .4rem .6rem; } .view-switch button.active { background: #e8eef2; color: #16394a; font-weight: 700; }
  .grid-view { display: grid; gap: 1rem; grid-template-columns: repeat(3, minmax(0, 1fr)); } .list-view { display: grid; gap: .75rem; } .media-card { background: #fff; border: 1px solid #d7dee5; border-radius: .45rem; display: grid; gap: .8rem; min-width: 0; overflow: hidden; padding: .8rem; } .grid-view .media-card { grid-template-rows: auto 1fr; } .list-view .media-card { grid-template-columns: 5rem minmax(0, 1fr); } .poster-frame { aspect-ratio: 2 / 3; background: #eef2f4; border: 1px solid #e1e8ed; border-radius: .3rem; min-width: 0; overflow: hidden; } .poster-frame img { display: block; height: 100%; object-fit: cover; width: 100%; } .list-view .poster-frame { aspect-ratio: 2 / 3; } .poster-empty { align-items: center; color: #52606d; display: flex; height: 100%; justify-content: center; padding: .5rem; text-align: center; }
  .media-info { display: grid; gap: .5rem; min-width: 0; } .title-row { align-items: flex-start; } .title-row h2 { min-width: 0; overflow: hidden; text-overflow: ellipsis; } .original-title { color: #52606d; font-size: .8rem; margin: -.25rem 0 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } .meta-row, .tags, .state-row { flex-wrap: wrap; } .meta-row { color: #52606d; font-size: .76rem; } .tags { display: flex; gap: .3rem; } .edit-details { border-top: 1px solid #e1e8ed; padding-top: .45rem; } .edit-details summary { color: #245b75; cursor: pointer; font-size: .82rem; font-weight: 650; } .edit-form { display: grid; gap: .55rem; grid-template-columns: repeat(3, minmax(0, 1fr)); margin-top: .6rem; } .edit-form .button { align-self: end; } .card-footer { border-top: 1px solid #e1e8ed; font-size: .76rem; padding-top: .5rem; } .card-footer small { margin: 0; }
  .empty { align-items: center; display: grid; justify-items: center; min-height: 10rem; } .empty p { margin: 0; } .pagination { flex-wrap: wrap; justify-content: center; } .pagination > span { color: #52606d; min-width: 8rem; text-align: center; } button:focus-visible, a:focus-visible, input:focus-visible, select:focus-visible, summary:focus-visible { outline: 3px solid #9fb3c8; outline-offset: 2px; }
  @media (max-width: 1000px) { .toolbar { grid-template-columns: 1fr; } .filters { justify-content: flex-start; } .grid-view { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  @media (max-width: 620px) { .page-heading, .toolbar, .filters { align-items: stretch; flex-direction: column; } .toolbar { display: flex; } .toolbar-status { align-items: flex-start; flex-direction: column; } .filters input, .filters select, .filters .button { width: 100%; } .view-switch { align-self: flex-start; } .grid-view { grid-template-columns: 1fr; } .list-view .media-card { grid-template-columns: 4.5rem minmax(0, 1fr); } .edit-form { grid-template-columns: 1fr; } .edit-form .button { width: 100%; } .page-heading > .button { align-self: flex-start; } .pagination .button { flex: 1 1 calc(50% - .65rem); text-align: center; } .pagination > span { order: -1; width: 100%; } }
</style>
