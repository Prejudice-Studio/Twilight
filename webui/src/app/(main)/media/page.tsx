"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Film, ListTodo, Search } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/hooks/use-toast";
import {
  api,
  type InventoryCheckResult,
  type MediaDetail,
  type MediaItem,
  type MediaRequest,
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";
import { MediaDetailDialog } from "./media-detail-dialog";
import {
  errorMessage,
  isAbortError,
  mediaCacheKey,
  mergeMediaDetail,
  mergeMediaItems,
  normalizeMediaDetail,
  normalizeMediaItem,
  normalizeMediaYear,
  rememberCache,
  settleResponse,
  type ResponseOutcome,
  type MediaSearchMode,
  type MediaSearchSource,
  type TMDBMediaType,
} from "./media-model";
import { MediaSearchView } from "./media-search-view";
import { MyMediaRequests } from "./my-media-requests";

const MAX_SEARCH_CACHE_ENTRIES = 16;
const MAX_DETAIL_CACHE_ENTRIES = 24;
const MAX_INVENTORY_CACHE_ENTRIES = 24;

export default function MediaPage() {
  const { t } = useI18n();
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();

  const [activeTab, setActiveTab] = useState<"search" | "requests">("search");
  const [searchInput, setSearchInput] = useState("");
  const [searchMode, setSearchMode] = useState<MediaSearchMode>("name");
  const [source, setSource] = useState<MediaSearchSource>("all");
  const [mediaType, setMediaType] = useState<TMDBMediaType>("movie");
  const [results, setResults] = useState<MediaItem[]>([]);
  const [searched, setSearched] = useState(false);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);

  const [selectedMedia, setSelectedMedia] = useState<MediaItem | null>(null);
  const [mediaDetail, setMediaDetail] = useState<MediaDetail | null>(null);
  const [inventory, setInventory] = useState<InventoryCheckResult | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [requesting, setRequesting] = useState(false);
  const [selectedSeason, setSelectedSeason] = useState<number | undefined>();
  const [requestNote, setRequestNote] = useState("");

  const [myRequests, setMyRequests] = useState<MediaRequest[]>([]);
  const [requestsLoaded, setRequestsLoaded] = useState(false);
  const [requestsLoading, setRequestsLoading] = useState(false);
  const [requestsError, setRequestsError] = useState<string | null>(null);
  const [deletingKey, setDeletingKey] = useState("");

  const searchAbortRef = useRef<AbortController | null>(null);
  const detailAbortRef = useRef<AbortController | null>(null);
  const requestsAbortRef = useRef<AbortController | null>(null);
  const searchSequenceRef = useRef(0);
  const detailSequenceRef = useRef(0);
  const requestsSequenceRef = useRef(0);
  const searchCacheRef = useRef<Map<string, MediaItem[]>>(new Map());
  const detailCacheRef = useRef<Map<string, MediaDetail>>(new Map());
  const inventoryCacheRef = useRef<Map<string, InventoryCheckResult>>(new Map());

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  useEffect(() => () => {
    searchAbortRef.current?.abort();
    detailAbortRef.current?.abort();
    requestsAbortRef.current?.abort();
    searchCacheRef.current.clear();
    detailCacheRef.current.clear();
    inventoryCacheRef.current.clear();
  }, []);

  const closeDetail = useCallback(() => {
    detailAbortRef.current?.abort();
    detailAbortRef.current = null;
    detailSequenceRef.current += 1;
    setSelectedMedia(null);
    setMediaDetail(null);
    setInventory(null);
    setDetailLoading(false);
    setSelectedSeason(undefined);
    setRequestNote("");
  }, []);

  const handleSearchModeChange = (mode: MediaSearchMode) => {
    setSearchMode(mode);
    if (mode === "id" && source === "all") setSource("tmdb");
  };

  const handleSearch = async () => {
    const query = searchInput.trim();
    if (!query) return;
    if (searchMode === "id" && !/^\d+$/.test(query)) {
      toast({ title: t("media.invalidId"), description: t("media.invalidIdDescription"), variant: "destructive" });
      return;
    }
    if (searchMode === "id" && source === "all") {
      toast({ title: t("media.selectSource"), description: t("media.selectSourceDescription"), variant: "destructive" });
      return;
    }

    searchAbortRef.current?.abort();
    const controller = new AbortController();
    searchAbortRef.current = controller;
    const sequence = ++searchSequenceRef.current;
    const cacheKey = `${searchMode}|${source}|${mediaType}|${query.toLocaleLowerCase()}`;
    const cached = searchCacheRef.current.get(cacheKey);
    setSearched(true);
    setSearchError(null);
    if (cached) {
      setResults(cached);
      setSearching(false);
      searchAbortRef.current = null;
      rememberCache(searchCacheRef.current, cacheKey, cached, MAX_SEARCH_CACHE_ENTRIES);
      return;
    }

    setSearching(true);
    try {
      let finalResults: MediaItem[] = [];
      if (searchMode === "id") {
        const mediaID = Number.parseInt(query, 10);
        const response = source === "tmdb"
          ? await api.getMediaByTmdbId(mediaID, mediaType, true, controller.signal)
          : await api.getMediaByBangumiId(mediaID, true, controller.signal);
        if (!response.success || !response.data) throw new Error(response.message || t("media.notFoundDescription"));
        const detail = normalizeMediaDetail(response.data);
        finalResults = [normalizeMediaItem(detail)];
        rememberCache(detailCacheRef.current, mediaCacheKey(detail), detail, MAX_DETAIL_CACHE_ENTRIES);
      } else {
        const response = await api.searchMedia(query, source, controller.signal);
        if (!response.success || !response.data) throw new Error(response.message || t("media.searchFailed"));
        const unique = new Map<string, MediaItem>();
        for (const item of response.data.results) {
          const normalized = normalizeMediaItem(item);
          const key = mediaCacheKey(normalized);
          const existing = unique.get(key);
          unique.set(key, existing ? mergeMediaItems(existing, normalized) : normalized);
        }
        finalResults = Array.from(unique.values());
        const warnings = response.data.warnings ? Object.values(response.data.warnings).filter(Boolean) : [];
        if (warnings.length > 0) {
          toast({ title: t("media.partialSearchFailed"), description: warnings.join("\n"), variant: "destructive" });
        }
      }
      if (controller.signal.aborted || sequence !== searchSequenceRef.current) return;
      setResults(finalResults);
      rememberCache(searchCacheRef.current, cacheKey, finalResults, MAX_SEARCH_CACHE_ENTRIES);
    } catch (error) {
      if (isAbortError(error) || sequence !== searchSequenceRef.current) return;
      setSearchError(errorMessage(error, t("common.networkError")));
    } finally {
      if (sequence === searchSequenceRef.current) setSearching(false);
      if (searchAbortRef.current === controller) searchAbortRef.current = null;
    }
  };

  const handleSelectMedia = async (media: MediaItem) => {
    detailAbortRef.current?.abort();
    const controller = new AbortController();
    detailAbortRef.current = controller;
    const sequence = ++detailSequenceRef.current;
    const normalized = normalizeMediaItem(media);
    const key = mediaCacheKey(normalized);
    const cachedDetail = detailCacheRef.current.get(key);
    const cachedInventory = inventoryCacheRef.current.get(key);
    const inventorySupported = normalized.source !== "bangumi" && normalized.source !== "bgm";

    setSelectedMedia(normalized);
    setMediaDetail(cachedDetail || normalizeMediaDetail(normalized));
    setInventory(cachedInventory || null);
    setSelectedSeason(undefined);
    setRequestNote("");

    const needsDetail = !cachedDetail;
    const needsInventory = inventorySupported && !cachedInventory;
    if (!needsDetail && !needsInventory) {
      setDetailLoading(false);
      detailAbortRef.current = null;
      return;
    }
    setDetailLoading(true);

    const [detailResult, inventoryResult] = await Promise.all([
      needsDetail
        ? settleResponse(api.getMediaDetail(normalized.source, normalized.id, normalized.media_type, controller.signal))
        : Promise.resolve<ResponseOutcome<MediaDetail>>({ data: cachedDetail }),
      needsInventory
        ? settleResponse(api.checkInventory({
            source: normalized.source,
            media_id: normalized.id,
            media_type: normalized.media_type,
            title: normalized.title,
            original_title: normalized.original_title,
            year: normalizeMediaYear(normalized.year),
          }, controller.signal))
        : Promise.resolve<ResponseOutcome<InventoryCheckResult>>({ data: cachedInventory }),
    ]);

    if (controller.signal.aborted || sequence !== detailSequenceRef.current) return;
    if (detailResult.data) {
      const resolved = mergeMediaDetail(normalized, detailResult.data);
      setMediaDetail(resolved);
      rememberCache(detailCacheRef.current, key, resolved, MAX_DETAIL_CACHE_ENTRIES);
    } else if (needsDetail) {
      toast({ title: t("media.detailFailed"), description: detailResult.message || errorMessage(detailResult.error, t("media.detailFailedDescription")), variant: "destructive" });
    }
    if (inventoryResult.data) {
      setInventory(inventoryResult.data);
      rememberCache(inventoryCacheRef.current, key, inventoryResult.data, MAX_INVENTORY_CACHE_ENTRIES);
    } else if (needsInventory) {
      toast({ title: t("media.inventoryCheckFailed"), description: inventoryResult.message || errorMessage(inventoryResult.error, t("common.networkError")), variant: "destructive" });
    }
    if (sequence === detailSequenceRef.current) setDetailLoading(false);
    if (detailAbortRef.current === controller) detailAbortRef.current = null;
  };

  const handleSubmitRequest = async () => {
    if (!selectedMedia) return;
    const detail = mediaDetail || normalizeMediaDetail(selectedMedia);
    setRequesting(true);
    try {
      const response = await api.createMediaRequest({
        source: selectedMedia.source,
        media_id: selectedMedia.id,
        media_type: selectedMedia.media_type,
        title: detail.title || selectedMedia.title,
        original_title: detail.original_title || selectedMedia.original_title,
        poster: detail.poster || selectedMedia.poster,
        poster_url: detail.poster_url || selectedMedia.poster_url,
        overview: detail.overview || selectedMedia.overview,
        year: normalizeMediaYear(detail.year ?? selectedMedia.year),
        season: selectedSeason,
        note: requestNote.trim() || undefined,
      });
      if (!response.success) throw new Error(response.message || t("media.requestFailed"));
      toast({ title: t("media.requestSuccess"), description: t("media.requestSuccessDescription"), variant: "success" });
      setRequestsLoaded(false);
      closeDetail();
    } catch (error) {
      toast({ title: t("media.requestFailed"), description: errorMessage(error, t("common.networkError")), variant: "destructive" });
    } finally {
      setRequesting(false);
    }
  };

  const loadMyRequests = useCallback(async () => {
    requestsAbortRef.current?.abort();
    const controller = new AbortController();
    requestsAbortRef.current = controller;
    const sequence = ++requestsSequenceRef.current;
    setRequestsLoading(true);
    setRequestsError(null);
    try {
      const response = await api.getMyRequests(controller.signal);
      if (controller.signal.aborted || sequence !== requestsSequenceRef.current) return;
      if (!response.success || !response.data) throw new Error(response.message || t("media.requestsLoadFailed"));
      setMyRequests(response.data);
      setRequestsLoaded(true);
    } catch (error) {
      if (isAbortError(error) || sequence !== requestsSequenceRef.current) return;
      setRequestsError(errorMessage(error, t("media.requestsLoadFailed")));
    } finally {
      if (sequence === requestsSequenceRef.current) setRequestsLoading(false);
      if (requestsAbortRef.current === controller) requestsAbortRef.current = null;
    }
  }, [t]);

  const handleTabChange = (value: string) => {
    const next = value === "requests" ? "requests" : "search";
    setActiveTab(next);
    if (next === "requests" && !requestsLoaded && !requestsLoading) void loadMyRequests();
  };

  const handleDeleteRequest = async (request: MediaRequest) => {
    const confirmed = await confirm({ title: t("media.deleteConfirmTitle"), description: t("media.irreversible"), tone: "danger", confirmLabel: t("common.delete") });
    if (!confirmed) return;
    setDeletingKey(request.require_key);
    try {
      const response = await api.deleteMyMediaRequest(request.require_key);
      if (!response.success) throw new Error(response.message || t("common.deleteFailed"));
      setMyRequests((current) => current.filter((item) => item.require_key !== request.require_key));
      toast({ title: t("media.deleteSuccess"), variant: "success" });
    } catch (error) {
      toast({ title: t("common.deleteFailed"), description: errorMessage(error, t("common.networkError")), variant: "destructive" });
    } finally {
      setDeletingKey("");
    }
  };

  const copyRequestKey = (key: string) => {
    navigator.clipboard.writeText(key).then(
      () => toast({ title: t("media.keyCopied"), variant: "success" }),
      () => toast({ title: t("common.copyFailed"), variant: "destructive" }),
    );
  };

  if (systemInfo?.features?.media_request === false) {
    return (
      <Card><CardContent className="flex min-h-80 flex-col items-center justify-center p-8 text-center"><Film className="mb-4 h-10 w-10 text-muted-foreground/40" /><h1 className="text-xl font-semibold">{t("media.disabledTitle")}</h1><p className="mt-2 text-sm text-muted-foreground">{t("media.disabledDescription")}</p></CardContent></Card>
    );
  }

  return (
    <div className="min-w-0 space-y-6 pb-10">
      <header className="border-b pb-5">
        <h1 className="text-2xl font-semibold sm:text-3xl">{t("media.title")}</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">{t("media.description")}</p>
      </header>

      <Tabs value={activeTab} onValueChange={handleTabChange}>
        <TabsList className="grid h-auto w-full grid-cols-2 sm:w-80">
          <TabsTrigger value="search" className="min-h-10 gap-2"><Search className="h-4 w-4" />{t("media.searchTab")}</TabsTrigger>
          <TabsTrigger value="requests" className="min-h-10 gap-2"><ListTodo className="h-4 w-4" />{t("media.requestsTab")}</TabsTrigger>
        </TabsList>
      </Tabs>

      {activeTab === "search" ? (
        <MediaSearchView
          query={searchInput}
          mode={searchMode}
          source={source}
          mediaType={mediaType}
          results={results}
          searching={searching}
          searched={searched}
          error={searchError}
          onQueryChange={setSearchInput}
          onModeChange={handleSearchModeChange}
          onSourceChange={setSource}
          onMediaTypeChange={setMediaType}
          onSearch={() => void handleSearch()}
          onSelect={(media) => void handleSelectMedia(media)}
        />
      ) : (
        <MyMediaRequests
          requests={myRequests}
          loading={requestsLoading}
          error={requestsError}
          deletingKey={deletingKey}
          onRefresh={() => void loadMyRequests()}
          onDelete={(request) => void handleDeleteRequest(request)}
          onCopy={copyRequestKey}
          onSearch={() => setActiveTab("search")}
        />
      )}

      <MediaDetailDialog
        detail={mediaDetail}
        inventory={inventory}
        loading={detailLoading}
        requesting={requesting}
        selectedSeason={selectedSeason}
        note={requestNote}
        onClose={closeDetail}
        onSeasonChange={setSelectedSeason}
        onNoteChange={setRequestNote}
        onSubmit={() => void handleSubmitRequest()}
      />
    </div>
  );
}
