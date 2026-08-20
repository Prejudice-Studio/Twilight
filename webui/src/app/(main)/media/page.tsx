"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { motion, AnimatePresence } from "framer-motion";
import {
  Search,
  Film,
  Tv,
  Star,
  Calendar,
  Loader2,
  Check,
  X,
  Package,
  Send,
  Hash,
  Type,
  ListTodo,
  ExternalLink,
  Trash2,
  Fingerprint,
  Copy,
} from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { api, type ApiResponse, type MediaItem, type MediaDetail, type InventoryCheckResult, type MediaRequest } from "@/lib/api";
import { formatRelativeTime, cn } from "@/lib/utils";
import { useSystemStore } from "@/store/system";
import { mediaRequestExternalUrl } from "@/lib/media-external-url";
import { useI18n } from "@/lib/i18n";
import { sanitizeExternalUrl, sanitizeImageUrl } from "@/lib/safe-url";

const MAX_SEARCH_CACHE_ENTRIES = 20;
const MAX_DETAIL_CACHE_ENTRIES = 40;
const MAX_INVENTORY_CACHE_ENTRIES = 40;

function rememberCache<K, V>(cache: Map<K, V>, key: K, value: V, limit: number) {
  if (cache.has(key)) {
    cache.delete(key);
  }
  cache.set(key, value);
  while (cache.size > limit) {
    const oldestKey = cache.keys().next().value as K | undefined;
    if (oldestKey === undefined) break;
    cache.delete(oldestKey);
  }
}

interface ResponseOutcome<T> {
  data?: T;
  message?: string;
  error?: unknown;
}

async function settleResponse<T>(request: Promise<ApiResponse<T>>): Promise<ResponseOutcome<T>> {
  try {
    const response = await request;
    if (response.success && response.data !== undefined) {
      return { data: response.data };
    }
    return { message: response.message };
  } catch (error) {
    return { error };
  }
}

function normalizeMediaYear(value: MediaItem["year"]): number | undefined {
  const parsed = typeof value === "number" ? value : Number.parseInt(String(value || ""), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function normalizeMediaRating(value: MediaItem["rating"] | MediaItem["vote_average"]): number | undefined {
  const parsed = typeof value === "number" ? value : Number.parseFloat(String(value ?? ""));
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function normalizeMediaItem(item: MediaItem): MediaItem {
  const source = String(item.source || "tmdb").toLowerCase();
  const mediaType = String(item.media_type || "movie").toLowerCase();
  const poster = sanitizeImageUrl(item.poster || item.poster_url);
  return {
    ...item,
    source,
    media_type: mediaType,
    title: String(item.title || ""),
    poster,
    poster_url: poster,
    source_url: sanitizeExternalUrl(item.source_url),
    year: normalizeMediaYear(item.year),
    rating: normalizeMediaRating(item.rating ?? item.vote_average),
  };
}

function normalizeMediaDetail(detail: MediaDetail): MediaDetail {
  const item = normalizeMediaItem(detail);
  const backdrop = sanitizeImageUrl(detail.backdrop || detail.backdrop_url);
  return {
    ...detail,
    ...item,
    backdrop,
    backdrop_url: backdrop,
  };
}

function mergeMediaItems(current: MediaItem, incoming: MediaItem): MediaItem {
  const first = normalizeMediaItem(current);
  const next = normalizeMediaItem(incoming);
  return normalizeMediaItem({
    id: first.id,
    source: first.source,
    media_type: first.media_type,
    title: first.title || next.title,
    original_title: first.original_title || next.original_title,
    overview: first.overview || next.overview,
    poster: first.poster || next.poster,
    poster_url: first.poster_url || next.poster_url,
    release_date: first.release_date || next.release_date,
    year: first.year ?? next.year,
    source_url: first.source_url || next.source_url,
    rating: first.rating ?? next.rating,
    vote_average: first.vote_average ?? next.vote_average,
  });
}

function mediaCacheKey(media: MediaItem): string {
  return `${media.source.toLowerCase()}-${media.id}-${media.media_type.toLowerCase()}`;
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default function MediaPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();
  const { info: systemInfo, fetchInfo: fetchSystemInfo } = useSystemStore();
  const [searchQuery, setSearchQuery] = useState("");
  const [source, setSource] = useState("all");
  const [searchMode, setSearchMode] = useState<"name" | "id">("name"); // 搜索模式：名称或ID
  const [mediaType, setMediaType] = useState<"movie" | "tv">("movie"); // TMDB 媒体类型
  const [results, setResults] = useState<MediaItem[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [selectedMedia, setSelectedMedia] = useState<MediaItem | null>(null);
  const [mediaDetail, setMediaDetail] = useState<MediaDetail | null>(null);
  const [inventoryCheck, setInventoryCheck] = useState<InventoryCheckResult | null>(null);
  const [isLoadingDetail, setIsLoadingDetail] = useState(false);
  const [isRequesting, setIsRequesting] = useState(false);
  const [selectedSeason, setSelectedSeason] = useState<number | undefined>();
  const [requestNote, setRequestNote] = useState("");
  
  // My Requests state
  const [activeTab, setActiveTab] = useState("search");
  const [myRequests, setMyRequests] = useState<MediaRequest[]>([]);
  const [isRequestsLoading, setIsRequestsLoading] = useState(false);
  const searchAbortRef = useRef<AbortController | null>(null);
  const detailAbortRef = useRef<AbortController | null>(null);
  const requestsAbortRef = useRef<AbortController | null>(null);
  const searchCacheRef = useRef<Map<string, MediaItem[]>>(new Map());
  const detailCacheRef = useRef<Map<string, MediaDetail>>(new Map());
  const inventoryCacheRef = useRef<Map<string, InventoryCheckResult>>(new Map());
  const myRequestsCacheRef = useRef<{ data: MediaRequest[]; ts: number } | null>(null);

  useEffect(() => {
    const searchCache = searchCacheRef.current;
    const detailCache = detailCacheRef.current;
    const inventoryCache = inventoryCacheRef.current;
    return () => {
      searchAbortRef.current?.abort();
      detailAbortRef.current?.abort();
      requestsAbortRef.current?.abort();
      searchCache.clear();
      detailCache.clear();
      inventoryCache.clear();
      myRequestsCacheRef.current = null;
    };
  }, []);

  useEffect(() => {
    void fetchSystemInfo();
  }, [fetchSystemInfo]);

  const isAbortError = (error: unknown) => {
    return error instanceof DOMException && error.name === "AbortError";
  };

  const closeMediaDetail = () => {
    detailAbortRef.current?.abort();
    detailAbortRef.current = null;
    setSelectedMedia(null);
    setMediaDetail(null);
    setInventoryCheck(null);
    setIsLoadingDetail(false);
  };

  const handleSearch = async () => {
    if (!searchQuery.trim()) return;

    searchAbortRef.current?.abort();
    const controller = new AbortController();
    searchAbortRef.current = controller;

    const normalizedQuery = searchQuery.trim();
    const searchCacheKey = `${searchMode}|${source}|${mediaType}|${normalizedQuery}`;

    if (searchMode === "name") {
      const cachedResults = searchCacheRef.current.get(searchCacheKey);
      if (cachedResults) {
        setResults(cachedResults);
        setIsSearching(false);
        return;
      }
    }

    setIsSearching(true);
    try {
      if (searchMode === "id") {
        // ID 搜索模式
        const mediaId = parseInt(normalizedQuery);
        if (isNaN(mediaId)) {
          toast({
            title: t("media.invalidId"),
            description: t("media.invalidIdDescription"),
            variant: "destructive",
          });
          setIsSearching(false);
          return;
        }

        // 根据来源调用不同的 API
        let detailRes;
        if (source === "tmdb") {
          detailRes = await api.getMediaByTmdbId(mediaId, mediaType, true, controller.signal);
        } else if (source === "bangumi" || source === "bgm") {
          detailRes = await api.getMediaByBangumiId(mediaId, true, controller.signal);
        } else {
          toast({
            title: t("media.selectSource"),
            description: t("media.selectSourceDescription"),
            variant: "destructive",
          });
          setIsSearching(false);
          return;
        }

        if (detailRes.success && detailRes.data) {
          const detail = normalizeMediaDetail(detailRes.data);
          const mediaItem = normalizeMediaItem(detail);
          setResults([mediaItem]);
          const detailKey = mediaCacheKey(detail);
          rememberCache(detailCacheRef.current, detailKey, detail, MAX_DETAIL_CACHE_ENTRIES);
          void handleSelectMedia(mediaItem, detail);
        } else {
          toast({
            title: t("media.notFound"),
            description: detailRes.message || t("media.notFoundDescription"),
            variant: "destructive",
          });
        }
      } else {
        // 名称搜索模式
        const res = await api.searchMedia(normalizedQuery, source, controller.signal);
        if (controller.signal.aborted) return;
        if (res.success && res.data) {
          // 聚合逻辑：确保 TMDB 同片多季（或重复结果）被折叠
          const uniqueResults = new Map<string, MediaItem>();
          
          res.data.results.forEach((item) => {
            const normalized = normalizeMediaItem(item);
            const key = mediaCacheKey(normalized);
            const current = uniqueResults.get(key);
            uniqueResults.set(key, current ? mergeMediaItems(current, normalized) : normalized);
          });
          
          const finalResults = Array.from(uniqueResults.values());
          setResults(finalResults);
          rememberCache(searchCacheRef.current, searchCacheKey, finalResults, MAX_SEARCH_CACHE_ENTRIES);
          if (res.data.warnings && Object.keys(res.data.warnings).length > 0) {
            toast({
              title: t("media.partialSearchFailed"),
              description: Object.values(res.data.warnings).join("\n"),
              variant: "destructive",
            });
          }
          
          if (finalResults.length === 0) {
            toast({
              title: t("media.noResults"),
              description: t("media.tryAnotherKeyword"),
            });
          }
        }
      }
    } catch (error: unknown) {
      if (isAbortError(error)) return;
      toast({
        title: t("media.searchFailed"),
        description: errorMessage(error, t("common.networkError")),
        variant: "destructive",
      });
    } finally {
      if (searchAbortRef.current === controller) {
        setIsSearching(false);
      }
    }
  };

  const handleSelectMedia = async (media: MediaItem, initialDetail?: MediaDetail) => {
    detailAbortRef.current?.abort();
    const controller = new AbortController();
    detailAbortRef.current = controller;

    const normalizedMedia = normalizeMediaItem(media);
    const detailKey = mediaCacheKey(normalizedMedia);
    const cachedDetail = initialDetail
      ? normalizeMediaDetail(initialDetail)
      : detailCacheRef.current.get(detailKey);
    const cachedInventory = inventoryCacheRef.current.get(detailKey);
    const inventorySupported = normalizedMedia.source !== "bangumi" && normalizedMedia.source !== "bgm";
    const shouldLoadDetail = !cachedDetail;
    const shouldLoadInventory = inventorySupported && !cachedInventory;

    setSelectedMedia(normalizedMedia);
    // 搜索结果本身已包含标题和海报，先展示它，再在后台补齐详情。
    setMediaDetail(cachedDetail || normalizeMediaDetail(normalizedMedia));
    setInventoryCheck(cachedInventory || null);
    setIsLoadingDetail(shouldLoadDetail || shouldLoadInventory);
    setSelectedSeason(undefined);
    setRequestNote("");

    if (cachedDetail) {
      rememberCache(detailCacheRef.current, detailKey, cachedDetail, MAX_DETAIL_CACHE_ENTRIES);
    }
    if (!shouldLoadDetail && !shouldLoadInventory) {
      return;
    }

    try {
      const [detailOutcome, inventoryOutcome] = await Promise.all([
        shouldLoadDetail
          ? settleResponse(api.getMediaDetail(
              normalizedMedia.source,
              normalizedMedia.id,
              normalizedMedia.media_type,
              controller.signal,
            ))
          : Promise.resolve<ResponseOutcome<MediaDetail>>({ data: cachedDetail }),
        shouldLoadInventory
          ? settleResponse(api.checkInventory({
              source: normalizedMedia.source,
              media_id: normalizedMedia.id,
              media_type: normalizedMedia.media_type,
              title: normalizedMedia.title,
              original_title: normalizedMedia.original_title,
              year: normalizeMediaYear(normalizedMedia.year),
            }, controller.signal))
          : Promise.resolve<ResponseOutcome<InventoryCheckResult>>({ data: cachedInventory }),
      ]);

      if (controller.signal.aborted) return;

      if (detailOutcome.data) {
        const normalizedDetail = normalizeMediaDetail(detailOutcome.data);
        const detail = normalizeMediaDetail({
          ...normalizedDetail,
          title: normalizedDetail.title || normalizedMedia.title,
          original_title: normalizedDetail.original_title || normalizedMedia.original_title,
          overview: normalizedDetail.overview || normalizedMedia.overview,
          poster: normalizedDetail.poster || normalizedMedia.poster,
          poster_url: normalizedDetail.poster_url || normalizedMedia.poster_url,
          year: normalizedDetail.year ?? normalizedMedia.year,
          release_date: normalizedDetail.release_date || normalizedMedia.release_date,
          source_url: normalizedDetail.source_url || normalizedMedia.source_url,
          rating: normalizedDetail.rating ?? normalizedMedia.rating,
          vote_average: normalizedDetail.vote_average ?? normalizedMedia.vote_average,
        });
        setMediaDetail(detail);
        rememberCache(detailCacheRef.current, detailKey, detail, MAX_DETAIL_CACHE_ENTRIES);
      } else if (shouldLoadDetail) {
        toast({
          title: t("media.detailFailed"),
          description: detailOutcome.message || errorMessage(detailOutcome.error, t("media.detailFailedDescription")),
          variant: "destructive",
        });
      }
      if (inventoryOutcome.data) {
        setInventoryCheck(inventoryOutcome.data);
        rememberCache(inventoryCacheRef.current, detailKey, inventoryOutcome.data, MAX_INVENTORY_CACHE_ENTRIES);
      } else if (shouldLoadInventory) {
        toast({
          title: t("media.inventoryCheckFailed"),
          description: inventoryOutcome.message || errorMessage(inventoryOutcome.error, t("common.networkError")),
          variant: "destructive",
        });
      }
    } catch (error: unknown) {
      if (isAbortError(error)) return;
      toast({
        title: t("media.detailFailed"),
        description: errorMessage(error, t("common.networkError")),
        variant: "destructive",
      });
    } finally {
      if (detailAbortRef.current === controller) {
        setIsLoadingDetail(false);
      }
    }
  };

  const handleRequest = async () => {
    if (!selectedMedia) return;

    setIsRequesting(true);
    try {
      const res = await api.createMediaRequest({
        source: selectedMedia.source,
        media_id: selectedMedia.id,
        media_type: selectedMedia.media_type,
        title: mediaDetail?.title || selectedMedia.title,
        original_title: mediaDetail?.original_title || selectedMedia.original_title,
        poster: mediaDetail?.poster || selectedMedia.poster,
        poster_url: mediaDetail?.poster_url || selectedMedia.poster_url,
        overview: mediaDetail?.overview || selectedMedia.overview,
        year: normalizeMediaYear(mediaDetail?.year ?? selectedMedia.year),
        season: selectedSeason,
        note: requestNote.trim() || undefined,
      });

      if (res.success) {
        toast({
          title: t("media.requestSuccess"),
          description: t("media.requestSuccessDescription"),
          variant: "success",
        });
        myRequestsCacheRef.current = null;
        closeMediaDetail();
      } else {
        toast({
          title: t("media.requestFailed"),
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: t("media.requestFailed"),
        description: error.message,
        variant: "destructive",
      });
    } finally {
      setIsRequesting(false);
    }
  };

  const handleDelete = async (requireKey: string) => {
    if (!requireKey) {
      toast({ title: t("media.missingRequireKey"), variant: "destructive" });
      return;
    }
    const ok = await confirm({
      title: t("media.deleteConfirmTitle"),
      description: t("media.irreversible"),
      tone: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;

    try {
      const res = await api.deleteMyMediaRequest(requireKey);
      if (res.success) {
        toast({ title: t("media.deleteSuccess"), variant: "success" });
        myRequestsCacheRef.current = null;
        loadMyRequests();
      } else {
        toast({ title: t("common.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: t("common.deleteFailed"), description: error.message, variant: "destructive" });
    }
  };

  const loadMyRequests = async () => {
    const now = Date.now();
    if (myRequestsCacheRef.current && now - myRequestsCacheRef.current.ts < 30000) {
      setMyRequests(myRequestsCacheRef.current.data);
      return;
    }

    requestsAbortRef.current?.abort();
    const controller = new AbortController();
    requestsAbortRef.current = controller;

    setIsRequestsLoading(true);
    try {
      const res = await api.getMyRequests(controller.signal);
      if (controller.signal.aborted) return;
      if (res.success && res.data) {
        setMyRequests(res.data);
        myRequestsCacheRef.current = {
          data: res.data,
          ts: now,
        };
      }
    } catch (error: unknown) {
      if (isAbortError(error)) return;
      console.error(error);
    } finally {
      if (requestsAbortRef.current === controller) {
        setIsRequestsLoading(false);
      }
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "UNHANDLED": return <Badge variant="outline" className="rounded-lg bg-gray-100/50 dark:bg-slate-900/40 dark:text-slate-100">{t("media.statusUnhandled")}</Badge>;
      case "ACCEPTED": return <Badge variant="default" className="rounded-lg bg-blue-500/10 text-blue-500 border-blue-200">{t("media.statusAccepted")}</Badge>;
      case "DOWNLOADING": return <Badge variant="default" className="rounded-lg bg-orange-500/10 text-orange-500 border-orange-200">{t("media.statusDownloading")}</Badge>;
      case "REJECTED": return <Badge variant="destructive" className="rounded-lg">{t("media.statusRejected")}</Badge>;
      case "COMPLETED": return <Badge variant="default" className="rounded-lg bg-emerald-500/10 text-emerald-500 border-emerald-200">{t("media.statusCompleted")}</Badge>;
      default: return <Badge variant="secondary" className="rounded-lg">{status}</Badge>;
    }
  };

  const container = {
    hidden: { opacity: 0 },
    show: {
      opacity: 1,
      transition: {
        staggerChildren: 0.1
      }
    }
  };

  const itemAnim = {
    hidden: { opacity: 0, y: 20 },
    show: { opacity: 1, y: 0 }
  };
  const compactSegmentClass = "grid w-full grid-cols-2 overflow-hidden rounded-xl border border-border/60 bg-secondary/80 p-0.5 sm:w-auto";
  const compactSegmentButtonClass = "min-w-0 rounded-[0.65rem] px-3 py-1.5 text-xs font-bold transition-colors sm:px-4";
  const sourceSegmentClass = "isolate flex h-14 w-full overflow-hidden rounded-[1.25rem] border border-border/70 bg-card/60 backdrop-blur-md dark:bg-slate-950/40 sm:w-auto";
  const sourceSegmentButtonClass = "min-w-0 flex-1 px-3 text-sm font-bold transition-colors sm:flex-none sm:px-6";
  const activeSegmentClass = "bg-primary text-primary-foreground";
  const inactiveSegmentClass = "text-muted-foreground hover:bg-accent/80";
  const mediaRequestDisabled = systemInfo?.features?.media_request === false;
  const selectedSeasonAvailable = Boolean(
    selectedSeason !== undefined && inventoryCheck?.seasons_available?.includes(selectedSeason)
  );
  const requestNeedsIssueNote = Boolean(
    inventoryCheck?.exists && (selectedSeason === undefined || selectedSeasonAvailable)
  );
  const inventoryIssueReady = Boolean(requestNeedsIssueNote && requestNote.trim());
  const requestBlockedByInventory = Boolean(requestNeedsIssueNote && !requestNote.trim());
  const activeMediaDetail = selectedMedia
    ? normalizeMediaDetail(mediaDetail || selectedMedia)
    : null;
  const detailPoster = sanitizeImageUrl(activeMediaDetail?.poster || activeMediaDetail?.poster_url);
  const detailSourceURL = sanitizeExternalUrl(activeMediaDetail?.source_url);

  if (mediaRequestDisabled) {
    return (
      <Card className="border-border/60">
        <CardContent className="flex min-h-[320px] flex-col items-center justify-center gap-3 p-8 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <Film className="h-7 w-7" />
          </div>
          <div>
            <h1 className="text-xl font-semibold">{t("media.disabledTitle")}</h1>
            <p className="mt-2 text-sm text-muted-foreground">{t("media.disabledDescription")}</p>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-8 pb-10">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-black tracking-tighter text-foreground sm:text-4xl">{t("media.title")}</h1>
        <p className="text-muted-foreground font-medium">{t("media.description")}</p>
      </div>

      {/* Navigation Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="i18n-stable-tabs mb-8 grid h-auto w-full grid-cols-2 rounded-2xl p-1.5 glass-frosted sm:max-w-[400px]">
          <TabsTrigger value="search" className="gap-2 rounded-xl py-2 font-bold data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=active]:shadow-md">
            <Search className="h-4 w-4" />
            {t("media.searchTab")}
          </TabsTrigger>
          <TabsTrigger value="requests" className="gap-2 rounded-xl py-2 font-bold data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=active]:shadow-md" onClick={loadMyRequests}>
            <ListTodo className="h-4 w-4" />
            {t("media.requestsTab")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="search" className="space-y-8 outline-none">
          {/* Search Section */}
          <div className="premium-card p-1">
            <div className="p-6 space-y-6">
              <div className="flex flex-col gap-6">
                {/* 搜索模式切换 */}
                <div className="flex items-center gap-4 flex-wrap">
                  <span className="text-xs font-black uppercase tracking-widest text-muted-foreground">{t("media.searchMode")}</span>
                  <div className={compactSegmentClass}>
                    <button 
                      onClick={() => setSearchMode("name")}
                      className={cn(compactSegmentButtonClass, searchMode === "name" ? "bg-background text-primary shadow-sm dark:text-primary-foreground" : inactiveSegmentClass)}
                    >
                      {t("media.nameSearch")}
                    </button>
                    <button 
                      onClick={() => setSearchMode("id")}
                      className={cn(compactSegmentButtonClass, searchMode === "id" ? "bg-background text-primary shadow-sm dark:text-primary-foreground" : inactiveSegmentClass)}
                    >
                      {t("media.idSearch")}
                    </button>
                  </div>
                  
                  {searchMode === "id" && (
                    <div className="text-[10px] font-bold text-primary px-3 py-1 bg-primary/5 rounded-full border border-primary/10">
                      {t("media.idSearchHint")}
                    </div>
                  )}
                </div>

                {/* 搜索输入区域 */}
                <div className="flex min-w-0 flex-col items-stretch gap-4 lg:flex-row">
                  <div className="relative flex-1 group">
                    <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground transition-colors group-focus-within:text-primary" />
                    <Input
                      placeholder={
                        searchMode === "id"
                          ? t("media.idPlaceholder")
                          : t("media.namePlaceholder")
                      }
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                      className="h-14 pl-12 rounded-[1.25rem] border-white/40 bg-white/50 text-slate-950 backdrop-blur-md focus:bg-white transition-all shadow-inner text-base font-medium dark:border-slate-700/70 dark:bg-slate-950/80 dark:text-slate-100 dark:focus:bg-slate-900/95"
                    />
                  </div>
                  
                  <div className={sourceSegmentClass}>
                    {searchMode === "name" && (
                      <button 
                        onClick={() => setSource("all")}
                        className={cn(sourceSegmentButtonClass, source === "all" ? activeSegmentClass : inactiveSegmentClass)}
                      >
                        {t("media.allSources")}
                      </button>
                    )}
                    <button 
                      onClick={() => setSource("tmdb")}
                      className={cn(sourceSegmentButtonClass, source === "tmdb" ? activeSegmentClass : inactiveSegmentClass)}
                    >
                      TMDB
                    </button>
                    <button 
                      onClick={() => setSource("bangumi")}
                      className={cn(sourceSegmentButtonClass, source === "bangumi" ? activeSegmentClass : inactiveSegmentClass)}
                    >
                      Bangumi
                    </button>
                  </div>

                  {searchMode === "id" && source === "tmdb" && (
                    <div className={sourceSegmentClass}>
                      <button 
                        onClick={() => setMediaType("movie")}
                        className={cn(sourceSegmentButtonClass, mediaType === "movie" ? activeSegmentClass : inactiveSegmentClass)}
                      >
                        {t("media.movie")}
                      </button>
                      <button 
                        onClick={() => setMediaType("tv")}
                        className={cn(sourceSegmentButtonClass, mediaType === "tv" ? activeSegmentClass : inactiveSegmentClass)}
                      >
                        {t("media.tv")}
                      </button>
                    </div>
                  )}
                  
                  <Button onClick={handleSearch} disabled={isSearching} className="h-14 w-full rounded-[1.25rem] px-8 shadow-xl shadow-primary/20 transition-all active:scale-95 lg:w-auto">
                    {isSearching ? (
                      <Loader2 className="mr-2 h-5 w-5 animate-spin" />
                    ) : (
                      <Search className="mr-2 h-5 w-5" />
                    )}
                    {t("media.explore")}
                  </Button>
                </div>
              </div>
            </div>
          </div>

          {/* Results Grid */}
          <AnimatePresence mode="wait">
            {results.length > 0 && (
              <motion.div
                variants={container}
                initial="hidden"
                animate="show"
                className="space-y-4"
              >
                <div className="flex flex-wrap items-end justify-between gap-2">
                  <div>
                    <h2 className="text-xl font-bold">{t("media.resultsTitle")}</h2>
                    <p className="text-sm text-muted-foreground">{t("media.resultsDescription")}</p>
                  </div>
                  <Badge variant="outline">{t("media.resultCount", { count: results.length })}</Badge>
                </div>
                <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                  {results.map((media) => (
                    <motion.div
                      key={mediaCacheKey(media)}
                      variants={itemAnim}
                    >
                      <button
                        type="button"
                        aria-label={t("media.openDetails", { title: media.title })}
                        className="group premium-card flex h-full w-full flex-col overflow-hidden p-0 text-left ring-primary/40 hover:ring-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                      onClick={() => handleSelectMedia(media)}
                    >
                      <div className="aspect-[2/3] relative overflow-hidden bg-muted">
                        {media.poster ? (
                          <Image
                            src={media.poster}
                            alt={media.title}
                            fill
                            unoptimized
                            sizes="(max-width: 768px) 50vw, (max-width: 1280px) 33vw, 25vw"
                            className="h-full w-full object-cover transition-transform duration-700 group-hover:scale-110"
                          />
                        ) : (
                          <div className="flex h-full items-center justify-center">
                            {media.media_type === "movie" ? (
                              <Film className="h-12 w-12 text-muted-foreground/30" />
                            ) : (
                              <Tv className="h-12 w-12 text-muted-foreground/30" />
                            )}
                          </div>
                        )}
                        
                        <div className="absolute top-4 left-4 z-10 flex flex-col gap-2">
                          <Badge className="bg-white/60 backdrop-blur-xl border-white/40 text-black/80 font-black text-[10px] tracking-widest px-2.5 py-1 dark:border-slate-700/70 dark:bg-slate-950/70 dark:text-slate-100">
                            {media.source.toUpperCase()}
                          </Badge>
                          <Badge className="bg-black/40 backdrop-blur-xl border-0 text-white font-black text-[10px] tracking-widest px-2.5 py-1 uppercase">
                            {media.media_type === "movie" ? t("media.movieBadge") : t("media.tvBadge")}
                          </Badge>
                        </div>

                        <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
                        
                        <div className="absolute bottom-4 left-4 right-4 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-500">
                           {media.rating !== undefined && media.rating > 0 && (
                             <div className="flex items-center gap-1.5 px-3 py-1.5 bg-yellow-400 rounded-full w-fit shadow-lg shadow-yellow-400/20">
                               <Star className="h-3.5 w-3.5 fill-black text-black" />
                               <span className="text-[12px] font-black text-black">{media.rating.toFixed(1)}</span>
                             </div>
                           )}
                        </div>
                      </div>
                      
                      <div className="p-5 flex-1">
                        <h3 className="font-black text-lg line-clamp-1 group-hover:text-primary transition-colors">{media.title}</h3>
                        <p className="mt-1 text-xs font-bold text-muted-foreground uppercase tracking-widest">
                          {media.year || t("media.unknownYear")}
                        </p>
                      </div>
                      </button>
                    </motion.div>
                  ))}
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </TabsContent>

        <TabsContent value="requests" className="outline-none">
          <div className="premium-card p-1">
            <div className="p-6">
              <div className="mb-6">
                <h2 className="text-xl font-black">{t("media.requestsTitle")}</h2>
                <p className="text-sm text-muted-foreground font-medium">{t("media.requestsDescription")}</p>
              </div>
              
              {isRequestsLoading ? (
                <div className="flex h-32 items-center justify-center">
                  <Loader2 className="h-6 w-6 animate-spin text-primary" />
                </div>
              ) : myRequests.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-48 text-muted-foreground gap-4">
                  <div className="p-4 bg-secondary rounded-full">
                    <ListTodo className="h-8 w-8 opacity-40" />
                  </div>
                  <p className="font-bold">{t("media.requestsEmpty")}</p>
                  <Button variant="outline" size="sm" className="rounded-xl" onClick={() => setActiveTab("search")}>
                    {t("media.goSearch")}
                  </Button>
                </div>
              ) : (
                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                  {myRequests.map((req) => (
                    <motion.div
                      key={req.require_key || `${req.source}-${req.id}`}
                      initial={{ opacity: 0, x: -10 }}
                      animate={{ opacity: 1, x: 0 }}
                      className="group flex h-full flex-col gap-4 rounded-3xl border border-white/40 bg-secondary/30 p-4 transition-all duration-300 hover:bg-white/60 dark:border-slate-700/70 dark:bg-slate-950/40 dark:hover:bg-slate-900/80"
                    >
                      <div className="flex min-w-0 flex-1 gap-4">
                        <div className="relative flex aspect-[2/3] w-24 shrink-0 items-center justify-center overflow-hidden rounded-2xl border border-white/60 bg-white/90 shadow-sm dark:border-slate-700/70 dark:bg-slate-950/70 sm:w-28">
                          {sanitizeImageUrl(req.media_info?.poster_url || req.media_info?.poster) ? (
                            <Image
                              src={sanitizeImageUrl(req.media_info?.poster_url || req.media_info?.poster) || ""}
                              alt={req.media_info?.title || t("media.unknownMedia")}
                              fill
                              unoptimized
                              sizes="112px"
                              className="h-full w-full object-contain"
                            />
                          ) : (
                            req.media_info?.media_type === "movie" ? <Film className="h-6 w-6 text-muted-foreground/30" /> : <Tv className="h-6 w-6 text-muted-foreground/30" />
                          )}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            {(() => {
                              const url = mediaRequestExternalUrl(req);
                              const title = req.media_info?.title || t("media.unknownMedia");
                              if (!url) {
                                return <p className="font-black text-foreground truncate">{title}</p>;
                              }
                              return (
                                <a
                                  href={url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="font-black text-foreground truncate underline decoration-dotted underline-offset-2 hover:text-primary inline-flex items-center gap-1"
                                  title={t("media.viewOnSource", { source: req.source.toUpperCase() })}
                                >
                                  {title}
                                  <ExternalLink className="h-3 w-3 shrink-0 opacity-70" />
                                </a>
                              );
                            })()}
                            {req.media_info?.season && (
                              <span className="px-2 py-0.5 bg-primary/10 text-primary rounded-full text-[10px] font-black uppercase tracking-tighter">
                                Sea {req.media_info.season}
                              </span>
                            )}
                          </div>
                          <div className="flex items-center gap-2 mt-1 flex-wrap">
                             <span className="text-[10px] font-black text-muted-foreground uppercase tracking-widest">
                               {formatRelativeTime(req.timestamp * 1000)}
                             </span>
                             <span className="w-1 h-1 bg-muted-foreground/30 rounded-full" />
                             <span className="text-[10px] font-black text-primary/70 uppercase">
                               {req.source.toUpperCase()}#{String(req.media_id ?? "")}
                             </span>
                             {req.require_key && (
                               <>
                                 <span className="w-1 h-1 bg-muted-foreground/30 rounded-full" />
                                 <span
                                   className="inline-flex items-center gap-1 text-[10px] text-muted-foreground"
                                    title={t("media.externalKeyTitle")}
                                 >
                                   <Fingerprint className="h-3 w-3" />
                                   <code className="max-w-[8rem] truncate rounded bg-muted px-1 text-foreground sm:max-w-[14rem]">
                                     {req.require_key}
                                   </code>
                                   <button
                                     type="button"
                                     onClick={() => {
                                       navigator.clipboard.writeText(req.require_key).then(
                                          () => toast({ title: t("media.keyCopied"), variant: "success" }),
                                          () => toast({ title: t("common.copyFailed"), variant: "destructive" }),
                                       );
                                     }}
                                     className="text-muted-foreground hover:text-foreground"
                                      title={t("media.copyKey")}
                                   >
                                     <Copy className="h-3 w-3" />
                                   </button>
                                 </span>
                               </>
                             )}
                          </div>

                          {req.admin_note && (
                            <div className="mt-2 text-[11px] font-bold text-primary bg-primary/5 px-3 py-1.5 rounded-xl border border-primary/10 dark:bg-primary/10 dark:text-primary dark:border-primary/20">
                              {t("media.adminReply", { note: req.admin_note })}
                            </div>
                          )}
                        </div>
                      </div>
                      
                      <div className="flex shrink-0 items-center justify-between gap-3 border-t border-border/50 pt-3">
                        <div className="flex items-center gap-2">
                           {getStatusBadge(req.status)}
                           <Button 
                             size="icon" 
                             variant="ghost" 
                             className="h-10 w-10 rounded-xl hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/20 dark:hover:text-red-300 transition-colors"
                             onClick={() => handleDelete(req.require_key)}
                           >
                             <Trash2 className="h-4 w-4" />
                           </Button>
                        </div>
                        
                        {req.status === "COMPLETED" && (
                          <Button size="sm" variant="outline" className="h-8 rounded-lg text-[10px] font-black tracking-widest uppercase border-primary/20 hover:bg-primary hover:text-white transition-all" asChild>
                            <a href={`/search?q=${encodeURIComponent(req.media_info?.title || "")}`} target="_blank" rel="noreferrer">
                              <ExternalLink className="h-3 w-3 mr-1.5" />
                              {t("media.viewMedia")}
                            </a>
                          </Button>
                        )}
                      </div>
                    </motion.div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </TabsContent>
      </Tabs>

      {/* Detail Dialog */}
      <Dialog open={!!selectedMedia} onOpenChange={(open) => !open && closeMediaDetail()}>
        <DialogContent className="max-h-[calc(100dvh-1rem)] max-w-5xl overflow-hidden rounded-xl border-0 p-0 glass-acrylic shadow-2xl [&>button:last-child]:bg-black/55 [&>button:last-child]:text-white">
          {activeMediaDetail && (
            <>
              <DialogHeader className="sr-only">
                <DialogTitle>{activeMediaDetail.title}</DialogTitle>
                <DialogDescription>{t("media.detailDialogDescription", { title: activeMediaDetail.title })}</DialogDescription>
              </DialogHeader>
              <div className="grid max-h-[calc(100dvh-1rem)] overflow-y-auto md:grid-cols-[minmax(240px,320px)_minmax(0,1fr)] md:overflow-hidden">
                <div className="relative flex min-h-[380px] items-center justify-center bg-black p-4 md:min-h-0">
                  {detailPoster ? (
                    <div className="relative aspect-[2/3] w-full max-w-[260px] overflow-hidden rounded-lg bg-black shadow-2xl md:max-w-none">
                      <Image
                        src={detailPoster}
                        alt={activeMediaDetail.title}
                        fill
                        unoptimized
                        sizes="(max-width: 768px) 260px, 320px"
                        className="h-full w-full object-contain"
                      />
                    </div>
                  ) : (
                    <div className="flex aspect-[2/3] w-full max-w-[260px] items-center justify-center rounded-lg bg-secondary md:max-w-none">
                      {activeMediaDetail.media_type === "movie" ? (
                        <Film className="h-20 w-20 text-muted-foreground/25" />
                      ) : (
                        <Tv className="h-20 w-20 text-muted-foreground/25" />
                      )}
                    </div>
                  )}
                  <div className="pointer-events-none absolute inset-x-0 bottom-0 h-44 bg-gradient-to-t from-black/90 to-transparent" />
                  <div className="absolute bottom-5 left-5 right-5 min-w-0">
                    <div className="mb-2 flex flex-wrap items-center gap-2">
                      <Badge className="border-white/20 bg-white/20 px-2.5 py-1 text-[10px] font-black tracking-widest text-white backdrop-blur-md">
                        {activeMediaDetail.source.toUpperCase()}
                      </Badge>
                      {activeMediaDetail.rating !== undefined && activeMediaDetail.rating > 0 && (
                        <div className="flex items-center gap-1 rounded-md bg-yellow-400 px-2 py-1 text-[10px] font-black text-black">
                          <Star className="h-3 w-3 fill-black" />
                          {activeMediaDetail.rating.toFixed(1)}
                        </div>
                      )}
                    </div>
                    <h2 className="line-clamp-2 break-words text-xl font-black leading-tight text-white sm:text-2xl">
                      {activeMediaDetail.title}
                    </h2>
                  </div>
                </div>

                <div className="custom-scrollbar overflow-y-auto bg-card/95 p-5 text-foreground sm:p-7">
                  <div className="space-y-6">
                    {isLoadingDetail && (
                      <div className="flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2 text-xs font-medium text-primary">
                        <Loader2 className="h-4 w-4 animate-spin" />
                        {t("media.loadingDetails")}
                      </div>
                    )}

                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline" className="rounded-lg border-primary/20 px-3 py-1 font-bold text-primary">
                        {activeMediaDetail.media_type === "movie" ? t("media.movieWork") : t("media.tvSeries")}
                      </Badge>
                      {activeMediaDetail.genres?.map((genre) => (
                        <Badge key={genre} variant="secondary" className="rounded-lg border border-border bg-muted/80 px-3 py-1 font-bold text-muted-foreground">
                          {genre}
                        </Badge>
                      ))}
                      {detailSourceURL && (
                        <Button asChild size="sm" variant="outline" className="ml-auto">
                          <a href={detailSourceURL} target="_blank" rel="noopener noreferrer">
                            {t("media.openSource")}
                            <ExternalLink className="ml-1.5 h-3.5 w-3.5" />
                          </a>
                        </Button>
                      )}
                    </div>

                    <dl className="grid gap-3 rounded-lg border bg-muted/25 p-4 text-sm sm:grid-cols-2">
                      <div className="min-w-0">
                        <dt className="text-xs text-muted-foreground">{t("media.sourceIdentifier")}</dt>
                        <dd className="mt-1 break-all font-mono">{activeMediaDetail.source.toUpperCase()}#{activeMediaDetail.id}</dd>
                      </div>
                      <div className="min-w-0">
                        <dt className="text-xs text-muted-foreground">{t("media.releaseDate")}</dt>
                        <dd className="mt-1 break-words">{activeMediaDetail.release_date || activeMediaDetail.year || t("media.unknownYear")}</dd>
                      </div>
                      {activeMediaDetail.original_title && activeMediaDetail.original_title !== activeMediaDetail.title && (
                        <div className="min-w-0 sm:col-span-2">
                          <dt className="text-xs text-muted-foreground">{t("media.originalTitle")}</dt>
                          <dd className="mt-1 break-words">{activeMediaDetail.original_title}</dd>
                        </div>
                      )}
                      {activeMediaDetail.runtime !== undefined && activeMediaDetail.runtime > 0 && (
                        <div className="min-w-0">
                          <dt className="text-xs text-muted-foreground">{t("media.runtime")}</dt>
                          <dd className="mt-1">{t("media.runtimeMinutes", { minutes: activeMediaDetail.runtime })}</dd>
                        </div>
                      )}
                      {activeMediaDetail.episodes !== undefined && activeMediaDetail.episodes > 0 && (
                        <div className="min-w-0">
                          <dt className="text-xs text-muted-foreground">{t("media.episodes")}</dt>
                          <dd className="mt-1">{t("media.episodeCount", { count: activeMediaDetail.episodes })}</dd>
                        </div>
                      )}
                      {activeMediaDetail.status && (
                        <div className="min-w-0">
                          <dt className="text-xs text-muted-foreground">{t("media.sourceStatus")}</dt>
                          <dd className="mt-1 break-words">{activeMediaDetail.status}</dd>
                        </div>
                      )}
                    </dl>

                    <div className="space-y-2">
                      <p className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground">{t("media.about")}</p>
                      <p className="whitespace-pre-line text-sm font-medium leading-relaxed text-foreground">
                        {activeMediaDetail.overview || t("media.noOverview")}
                      </p>
                    </div>

                    {inventoryCheck && (
                      <div className={cn(
                        "rounded-lg border p-4",
                        inventoryCheck.exists
                          ? "border-emerald-200 bg-emerald-50 dark:border-emerald-500/30 dark:bg-emerald-500/10"
                          : "border-amber-200 bg-amber-50 dark:border-amber-500/30 dark:bg-amber-500/10",
                      )}>
                        <div className="flex items-start gap-3">
                          <div className={cn(
                            "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-white",
                            inventoryCheck.exists ? "bg-emerald-500" : "bg-amber-500",
                          )}>
                            {inventoryCheck.exists ? <Check className="h-4 w-4" /> : <Package className="h-4 w-4" />}
                          </div>
                          <div className="min-w-0">
                            <p className="text-sm font-black text-foreground">
                              {inventoryCheck.exists ? t("media.inventoryAvailable") : t("media.inventoryMissing")}
                            </p>
                            <p className="mt-1 text-xs font-medium text-muted-foreground">
                              {inventoryCheck.exists ? t("media.inventoryAvailableDescription") : t("media.inventoryMissingDescription")}
                            </p>
                          </div>
                        </div>
                      </div>
                    )}

                    {activeMediaDetail.media_type !== "movie" && activeMediaDetail.seasons !== undefined && activeMediaDetail.seasons > 0 && (
                      <div className="space-y-3">
                        <p className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground">{t("media.seasons")}</p>
                        <div className="flex flex-wrap gap-2">
                          <button
                            type="button"
                            onClick={() => setSelectedSeason(undefined)}
                            className={cn(
                              "rounded-lg border px-4 py-2 text-xs font-black transition-colors",
                              selectedSeason === undefined
                                ? "border-primary bg-primary text-primary-foreground"
                                : "border-border bg-card text-muted-foreground hover:bg-accent",
                            )}
                          >
                            {t("media.allSeasons")}
                          </button>
                          {Array.from({ length: activeMediaDetail.seasons }, (_, index) => index + 1).map((season) => {
                            const isAvailable = inventoryCheck?.seasons_available?.includes(season);
                            const canReportAvailableSeason = Boolean(isAvailable && requestNote.trim());
                            return (
                              <button
                                key={season}
                                type="button"
                                onClick={() => (!isAvailable || canReportAvailableSeason) && setSelectedSeason(season)}
                                disabled={Boolean(isAvailable && !canReportAvailableSeason)}
                                className={cn(
                                  "rounded-lg border px-4 py-2 text-xs font-black transition-colors",
                                  selectedSeason === season
                                    ? "border-primary bg-primary text-primary-foreground"
                                    : isAvailable
                                      ? cn("border-emerald-200 bg-emerald-50 text-emerald-600 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-300", canReportAvailableSeason ? "hover:bg-emerald-100" : "cursor-not-allowed opacity-60")
                                      : "border-border bg-background text-muted-foreground hover:bg-accent",
                                )}
                              >
                                {t("media.season", { season })}
                                {isAvailable && <Check className="ml-1.5 inline-block h-3 w-3" />}
                              </button>
                            );
                          })}
                        </div>
                      </div>
                    )}

                    <div className="space-y-3">
                      <Label htmlFor="media-request-note" className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground">
                        {t("media.instructions")}
                      </Label>
                      <Input
                        id="media-request-note"
                        placeholder={inventoryCheck?.exists ? t("media.issuePlaceholder") : t("media.requestPlaceholder")}
                        value={requestNote}
                        onChange={(event) => setRequestNote(event.target.value)}
                        className="h-12 rounded-lg border-border bg-background shadow-inner"
                      />
                    </div>
                  </div>

                  <div className="mt-8 flex flex-col gap-3 border-t pt-5 sm:flex-row">
                    <Button variant="outline" className="sm:flex-1" onClick={closeMediaDetail}>
                      {t("common.close")}
                    </Button>
                    <Button
                      onClick={handleRequest}
                      disabled={isRequesting || requestBlockedByInventory}
                      className="sm:flex-[2]"
                    >
                      {isRequesting ? <Loader2 className="mr-2 h-5 w-5 animate-spin" /> : <Send className="mr-2 h-5 w-5" />}
                      {requestBlockedByInventory ? t("media.submitIssueAfterNote") : inventoryIssueReady ? t("media.submitIssue") : t("media.requestNow")}
                    </Button>
                  </div>
                </div>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
