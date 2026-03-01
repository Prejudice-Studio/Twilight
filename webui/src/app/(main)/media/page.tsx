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
import { api, type MediaItem, type MediaDetail, type InventoryCheckResult, type MediaRequest } from "@/lib/api";
import { formatRelativeTime, cn } from "@/lib/utils";

export default function MediaPage() {
  const { toast } = useToast();
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
    return () => {
      searchAbortRef.current?.abort();
      detailAbortRef.current?.abort();
      requestsAbortRef.current?.abort();
    };
  }, []);

  const isAbortError = (error: unknown) => {
    return error instanceof DOMException && error.name === "AbortError";
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
            title: "无效的 ID",
            description: "请输入有效的数字 ID",
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
            title: "请选择来源",
            description: "使用 ID 搜索时，请选择 TMDB 或 Bangumi",
            variant: "destructive",
          });
          setIsSearching(false);
          return;
        }

        if (detailRes.success && detailRes.data) {
          // 转换为 MediaItem 格式并直接显示
          const detail = detailRes.data;
          const mediaItem: MediaItem = {
            id: detail.id,
            title: detail.title,
            original_title: detail.original_title,
            media_type: detail.media_type,
            overview: detail.overview,
            release_date: detail.release_date,
            year: detail.year,
            poster: detail.poster || detail.poster_url,
            poster_url: detail.poster_url,
            rating: detail.rating || detail.vote_average,
            vote_average: detail.vote_average,
            source: detail.source,
            source_url: detail.source_url,
          };
          setResults([mediaItem]);
          
          // 直接使用已获取的详情数据，避免重复请求
          setSelectedMedia(mediaItem);
          setIsLoadingDetail(true);
          setMediaDetail(null);
          setInventoryCheck(null);
          setSelectedSeason(undefined);
          setRequestNote("");

          if (controller.signal.aborted) return;

          const detailKey = `${detail.source}-${detail.id}-${detail.media_type}`;
          detailCacheRef.current.set(detailKey, detail);
          
          try {
            // 只检查库存，不再重复获取详情
            const inventoryRes = await api.checkInventory({
              source: detail.source,
              media_id: detail.id,
              media_type: detail.media_type,
              title: detail.title,
              year: detail.year,
            }, controller.signal);

            if (controller.signal.aborted) return;
            
            // 直接使用已获取的详情数据
            setMediaDetail(detail);
            if (inventoryRes.success && inventoryRes.data) {
              setInventoryCheck(inventoryRes.data);
              inventoryCacheRef.current.set(detailKey, inventoryRes.data);
            }
          } catch (error: any) {
            if (isAbortError(error)) return;
            console.error(error);
            // 即使库存检查失败，也显示详情
            setMediaDetail(detail);
          } finally {
            setIsLoadingDetail(false);
          }
        } else {
          toast({
            title: "未找到媒体",
            description: detailRes.message || "该 ID 不存在或已被删除",
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
          
          res.data.results.forEach((item: any) => {
            const key = `${item.source}-${item.id}-${item.media_type}`;
            if (!uniqueResults.has(key)) {
              uniqueResults.set(key, {
                ...item,
                poster: item.poster || item.poster_url,
                rating: item.rating || item.vote_average,
              });
            }
          });
          
          const finalResults = Array.from(uniqueResults.values());
          setResults(finalResults);
          searchCacheRef.current.set(searchCacheKey, finalResults);
          
          if (finalResults.length === 0) {
            toast({
              title: "未找到结果",
              description: "尝试换个关键词搜索",
            });
          }
        }
      }
    } catch (error: any) {
      if (isAbortError(error)) return;
      toast({
        title: "搜索失败",
        description: error.message,
        variant: "destructive",
      });
    } finally {
      setIsSearching(false);
    }
  };

  const handleSelectMedia = async (media: MediaItem) => {
    detailAbortRef.current?.abort();
    const controller = new AbortController();
    detailAbortRef.current = controller;

    setSelectedMedia(media);
    setIsLoadingDetail(true);
    setMediaDetail(null);
    setInventoryCheck(null);
    setSelectedSeason(undefined);
    setRequestNote("");

    try {
      const detailKey = `${media.source}-${media.id}-${media.media_type}`;
      const cachedDetail = detailCacheRef.current.get(detailKey);
      const cachedInventory = inventoryCacheRef.current.get(detailKey);

      if (cachedDetail) {
        setMediaDetail(cachedDetail);
      }
      if (cachedInventory) {
        setInventoryCheck(cachedInventory);
      }
      if (cachedDetail && cachedInventory) {
        return;
      }

      // 获取详情和库存检查
      const [detailRes, inventoryRes] = await Promise.all([
        api.getMediaDetail(media.source, media.id, media.media_type, controller.signal),
        api.checkInventory({
          source: media.source,
          media_id: media.id,
          media_type: media.media_type,
          title: media.title,
          year: media.year,
        }, controller.signal),
      ]);

      if (controller.signal.aborted) return;

      if (detailRes.success && detailRes.data) {
        setMediaDetail(detailRes.data);
        detailCacheRef.current.set(detailKey, detailRes.data);
      } else {
        toast({
          title: "获取详情失败",
          description: detailRes.message || "无法获取媒体详情",
          variant: "destructive",
        });
      }
      if (inventoryRes.success && inventoryRes.data) {
        setInventoryCheck(inventoryRes.data);
        inventoryCacheRef.current.set(detailKey, inventoryRes.data);
      }
    } catch (error: any) {
      if (isAbortError(error)) return;
      console.error(error);
      toast({
        title: "获取详情失败",
        description: error.message || "网络错误",
        variant: "destructive",
      });
    } finally {
      setIsLoadingDetail(false);
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
        season: selectedSeason,
        note: requestNote || undefined,
      });

      if (res.success) {
        toast({
          title: "求片成功！",
          description: "管理员会尽快处理您的请求",
          variant: "success",
        });
        myRequestsCacheRef.current = null;
        setSelectedMedia(null);
      } else {
        toast({
          title: "求片失败",
          description: res.message,
          variant: "destructive",
        });
      }
    } catch (error: any) {
      toast({
        title: "求片失败",
        description: error.message,
        variant: "destructive",
      });
    } finally {
      setIsRequesting(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm("确定要删除这条求片请求吗？")) return;

    try {
      const res = await api.deleteMediaRequest(id);
      if (res.success) {
        toast({ title: "删除成功", variant: "success" });
        myRequestsCacheRef.current = null;
        loadMyRequests();
      } else {
        toast({ title: "删除失败", description: res.message, variant: "destructive" });
      }
    } catch (error: any) {
      toast({ title: "删除失败", description: error.message, variant: "destructive" });
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
      setIsRequestsLoading(false);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "UNHANDLED": return <Badge variant="outline" className="rounded-lg bg-gray-100/50">待处理</Badge>;
      case "ACCEPTED": return <Badge variant="default" className="rounded-lg bg-blue-500/10 text-blue-500 border-blue-200">已接受</Badge>;
      case "DOWNLOADING": return <Badge variant="default" className="rounded-lg bg-orange-500/10 text-orange-500 border-orange-200">下载中</Badge>;
      case "REJECTED": return <Badge variant="destructive" className="rounded-lg">已拒绝</Badge>;
      case "COMPLETED": return <Badge variant="default" className="rounded-lg bg-emerald-500/10 text-emerald-500 border-emerald-200">已完成</Badge>;
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

  return (
    <div className="space-y-8 pb-10">
      <div className="flex flex-col gap-2">
        <h1 className="text-4xl font-black tracking-tighter text-foreground">媒体搜索</h1>
        <p className="text-muted-foreground font-medium">寻找你心仪的作品，我们为你带回家</p>
      </div>

      {/* Navigation Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="grid w-full max-w-[400px] grid-cols-2 p-1.5 glass-frosted rounded-2xl mb-8">
          <TabsTrigger value="search" className="gap-2 rounded-xl py-2 font-bold data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=active]:shadow-md">
            <Search className="h-4 w-4" />
            媒体搜索
          </TabsTrigger>
          <TabsTrigger value="requests" className="gap-2 rounded-xl py-2 font-bold data-[state=active]:bg-primary data-[state=active]:text-primary-foreground data-[state=active]:shadow-md" onClick={loadMyRequests}>
            <ListTodo className="h-4 w-4" />
            我的求片
          </TabsTrigger>
        </TabsList>

        <TabsContent value="search" className="space-y-8 outline-none">
          {/* Search Section */}
          <div className="premium-card p-1">
            <div className="p-6 space-y-6">
              <div className="flex flex-col gap-6">
                {/* 搜索模式切换 */}
                <div className="flex items-center gap-4 flex-wrap">
                  <span className="text-xs font-black uppercase tracking-widest text-muted-foreground">Search Mode</span>
                  <div className="flex p-1 bg-secondary rounded-xl">
                    <button 
                      onClick={() => setSearchMode("name")}
                      className={cn("px-4 py-1.5 rounded-lg text-xs font-bold transition-all", searchMode === "name" ? "bg-white text-primary shadow-sm" : "text-muted-foreground")}
                    >
                      名称搜索
                    </button>
                    <button 
                      onClick={() => setSearchMode("id")}
                      className={cn("px-4 py-1.5 rounded-lg text-xs font-bold transition-all", searchMode === "id" ? "bg-white text-primary shadow-sm" : "text-muted-foreground")}
                    >
                      ID 搜索
                    </button>
                  </div>
                  
                  {searchMode === "id" && (
                    <div className="text-[10px] font-bold text-primary px-3 py-1 bg-primary/5 rounded-full border border-primary/10">
                      请选择特定来源进行精确匹配
                    </div>
                  )}
                </div>

                {/* 搜索输入区域 */}
                <div className="flex flex-col gap-4 lg:flex-row items-stretch">
                  <div className="relative flex-1 group">
                    <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground transition-colors group-focus-within:text-primary" />
                    <Input
                      placeholder={
                        searchMode === "id"
                          ? "输入媒体 ID（如 550, 400602）..."
                          : "输入名称、TMDB URL 或 Bangumi URL..."
                      }
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                      className="h-14 pl-12 rounded-[1.25rem] border-white/40 bg-white/50 backdrop-blur-md focus:bg-white transition-all shadow-inner text-base font-medium"
                    />
                  </div>
                  
                  <div className="flex gap-2 p-1 bg-white/40 backdrop-blur-md rounded-[1.25rem] border border-white/50">
                    {searchMode === "name" && (
                      <button 
                        onClick={() => setSource("all")}
                        className={cn("px-6 rounded-xl font-bold text-sm transition-all", source === "all" ? "bg-primary text-primary-foreground shadow-lg" : "hover:bg-white/50")}
                      >
                        全部
                      </button>
                    )}
                    <button 
                      onClick={() => setSource("tmdb")}
                      className={cn("px-6 rounded-xl font-bold text-sm transition-all", source === "tmdb" ? "bg-primary text-primary-foreground shadow-lg" : "hover:bg-white/50")}
                    >
                      TMDB
                    </button>
                    <button 
                      onClick={() => setSource("bangumi")}
                      className={cn("px-6 rounded-xl font-bold text-sm transition-all", source === "bangumi" ? "bg-primary text-primary-foreground shadow-lg" : "hover:bg-white/50")}
                    >
                      Bangumi
                    </button>
                  </div>

                  {searchMode === "id" && source === "tmdb" && (
                    <div className="flex p-1 bg-white/40 backdrop-blur-md rounded-[1.25rem] border border-white/50">
                      <button 
                        onClick={() => setMediaType("movie")}
                        className={cn("px-6 rounded-xl font-bold text-sm transition-all", mediaType === "movie" ? "bg-primary text-primary-foreground shadow-lg" : "hover:bg-white/50")}
                      >
                        电影
                      </button>
                      <button 
                        onClick={() => setMediaType("tv")}
                        className={cn("px-6 rounded-xl font-bold text-sm transition-all", mediaType === "tv" ? "bg-primary text-primary-foreground shadow-lg" : "hover:bg-white/50")}
                      >
                        剧集
                      </button>
                    </div>
                  )}
                  
                  <Button onClick={handleSearch} disabled={isSearching} className="h-14 px-8 rounded-[1.25rem] shadow-xl shadow-primary/20 active:scale-95 transition-all">
                    {isSearching ? (
                      <Loader2 className="mr-2 h-5 w-5 animate-spin" />
                    ) : (
                      <Search className="mr-2 h-5 w-5" />
                    )}
                    探索
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
                className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
              >
                {results.map((media) => (
                  <motion.div
                    key={`${media.source}-${media.id}`}
                    variants={itemAnim}
                  >
                    <div
                      className="group cursor-pointer premium-card p-0 h-full flex flex-col hover:ring-2 ring-primary/40"
                      onClick={() => handleSelectMedia(media)}
                    >
                      <div className="aspect-[2/3] relative rounded-t-[2rem] overflow-hidden bg-muted">
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
                          <Badge className="bg-white/60 backdrop-blur-xl border-white/40 text-black/80 font-black text-[10px] tracking-widest px-2.5 py-1">
                            {media.source.toUpperCase()}
                          </Badge>
                          <Badge className="bg-black/40 backdrop-blur-xl border-0 text-white font-black text-[10px] tracking-widest px-2.5 py-1 uppercase">
                            {media.media_type === "movie" ? "Movie" : "TV Show"}
                          </Badge>
                        </div>

                        <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500" />
                        
                        <div className="absolute bottom-4 left-4 right-4 translate-y-4 opacity-0 group-hover:translate-y-0 group-hover:opacity-100 transition-all duration-500">
                           {media.rating && (
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
                          {media.year || "未知年份"}
                        </p>
                      </div>
                    </div>
                  </motion.div>
                ))}
              </motion.div>
            )}
          </AnimatePresence>
        </TabsContent>

        <TabsContent value="requests" className="outline-none">
          <div className="premium-card p-1">
            <div className="p-6">
              <div className="mb-6">
                <h2 className="text-xl font-black">我的求片记录</h2>
                <p className="text-sm text-muted-foreground font-medium">追踪您提交的求片申请状态</p>
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
                  <p className="font-bold">暂无任何求片记录</p>
                  <Button variant="outline" size="sm" className="rounded-xl" onClick={() => setActiveTab("search")}>
                    去搜索求片
                  </Button>
                </div>
              ) : (
                <div className="grid gap-4">
                  {myRequests.map((req) => (
                    <motion.div
                      key={req.id}
                      initial={{ opacity: 0, x: -10 }}
                      animate={{ opacity: 1, x: 0 }}
                      className="group flex flex-col md:flex-row md:items-center justify-between gap-4 p-4 rounded-3xl bg-secondary/30 border border-white/40 hover:bg-white/60 transition-all duration-300"
                    >
                      <div className="flex items-center gap-4 flex-1 min-w-0">
                        <div className="relative flex h-20 w-14 shrink-0 items-center justify-center rounded-2xl bg-white overflow-hidden shadow-sm border border-white/60">
                          {req.media_info?.poster_url || req.media_info?.poster ? (
                            <Image
                              src={req.media_info.poster_url || req.media_info.poster || ""}
                              alt={req.media_info.title}
                              fill
                              unoptimized
                              sizes="56px"
                              className="h-full w-full object-cover"
                            />
                          ) : (
                            req.media_info?.media_type === "movie" ? <Film className="h-6 w-6 text-muted-foreground/30" /> : <Tv className="h-6 w-6 text-muted-foreground/30" />
                          )}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <p className="font-black text-foreground truncate">{req.media_info?.title || "未知媒体"}</p>
                            {req.media_info?.season && (
                              <span className="px-2 py-0.5 bg-primary/10 text-primary rounded-full text-[10px] font-black uppercase tracking-tighter">
                                Sea {req.media_info.season}
                              </span>
                            )}
                          </div>
                          <div className="flex items-center gap-2 mt-1">
                             <span className="text-[10px] font-black text-muted-foreground uppercase tracking-widest">
                               {formatRelativeTime(req.timestamp * 1000)}
                             </span>
                             <span className="w-1 h-1 bg-muted-foreground/30 rounded-full" />
                             <span className="text-[10px] font-black text-primary/70 uppercase">
                               {req.source.toUpperCase()}
                             </span>
                          </div>
                          
                          {req.admin_note && (
                            <div className="mt-2 text-[11px] font-bold text-primary bg-primary/5 px-3 py-1.5 rounded-xl border border-primary/10">
                              💌 管理回复: {req.admin_note}
                            </div>
                          )}
                        </div>
                      </div>
                      
                      <div className="flex items-center justify-between md:flex-col md:items-end gap-3 shrink-0">
                        <div className="flex items-center gap-2">
                           {getStatusBadge(req.status)}
                           <Button 
                             size="icon" 
                             variant="ghost" 
                             className="h-10 w-10 rounded-xl hover:bg-red-50 hover:text-red-500 transition-colors"
                             onClick={() => handleDelete(req.id)}
                           >
                             <Trash2 className="h-4 w-4" />
                           </Button>
                        </div>
                        
                        {req.status === "COMPLETED" && (
                          <Button size="sm" variant="outline" className="h-8 rounded-lg text-[10px] font-black tracking-widest uppercase border-primary/20 hover:bg-primary hover:text-white transition-all" asChild>
                            <a href={`/search?q=${encodeURIComponent(req.media_info?.title || "")}`} target="_blank" rel="noreferrer">
                              <ExternalLink className="h-3 w-3 mr-1.5" />
                              View Media
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
      <Dialog open={!!selectedMedia} onOpenChange={() => {
        setSelectedMedia(null);
        setMediaDetail(null);
        setInventoryCheck(null);
      }}>
        <DialogContent className="max-w-3xl border-0 p-0 overflow-hidden glass-acrylic rounded-[3rem] shadow-2xl">
          {isLoadingDetail ? (
            <div className="flex h-[400px] items-center justify-center">
              <div className="relative">
                <div className="h-12 w-12 rounded-full border-4 border-primary/20 border-t-primary animate-spin" />
                <div className="mt-4 text-xs font-black text-primary uppercase tracking-widest animate-pulse">Loading Details</div>
              </div>
            </div>
          ) : !selectedMedia ? null : mediaDetail ? (
            <div className="flex flex-col md:flex-row h-full max-h-[85vh]">
              {/* Left Side: Poster */}
              <div className="w-full md:w-1/3 aspect-[2/3] md:aspect-auto relative group">
                {mediaDetail.poster ? (
                  <Image
                    src={mediaDetail.poster}
                    alt={mediaDetail.title}
                    fill
                    unoptimized
                    sizes="(max-width: 768px) 100vw, 33vw"
                    className="h-full w-full object-cover"
                  />
                ) : (
                  <div className="flex h-full items-center justify-center bg-secondary">
                    <Film className="h-20 w-20 text-muted-foreground/20" />
                  </div>
                )}
                <div className="absolute inset-0 bg-gradient-to-t from-black/60 to-transparent" />
                <div className="absolute bottom-6 left-6 right-6">
                   <div className="flex items-center gap-2 mb-2">
                     <Badge className="bg-white/20 backdrop-blur-md border-white/20 text-white font-black text-[10px] tracking-widest px-2.5 py-1">
                        {mediaDetail.source?.toUpperCase()}
                     </Badge>
                     {mediaDetail.rating && (
                        <div className="flex items-center gap-1 px-2 py-0.5 bg-yellow-400 rounded-lg text-[10px] font-black text-black">
                          <Star className="h-3 w-3 fill-black" />
                          {mediaDetail.rating.toFixed(1)}
                        </div>
                     )}
                   </div>
                   <h2 className="text-2xl font-black text-white leading-none truncate">{mediaDetail.title}</h2>
                </div>
              </div>

              {/* Right Side: Content */}
              <div className="flex-1 p-8 overflow-y-auto custom-scrollbar bg-white/40">
                <div className="space-y-6">
                  {/* Status & Genres */}
                  <div className="flex flex-wrap gap-2">
                    <Badge variant="outline" className="border-primary/20 text-primary font-bold px-3 py-1 rounded-xl">
                      {mediaDetail.media_type === "movie" ? "电影作品" : "电视连续剧"}
                    </Badge>
                    {mediaDetail.genres?.map(genre => (
                      <Badge key={genre} variant="secondary" className="bg-white/60 border border-white/40 text-muted-foreground font-bold px-3 py-1 rounded-xl">
                        {genre}
                      </Badge>
                    ))}
                  </div>

                  {/* Overview */}
                  <div className="space-y-2">
                    <p className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground">About</p>
                    <p className="text-sm leading-relaxed text-foreground/80 font-medium">
                      {mediaDetail.overview || "暂无简介内容"}
                    </p>
                  </div>

                  {/* Inventory Info */}
                  {inventoryCheck && (
                    <div className={cn(
                      "p-4 rounded-[1.5rem] border transition-all duration-500",
                      inventoryCheck.exists 
                        ? "bg-emerald-50 border-emerald-100 shadow-sm" 
                        : "bg-amber-50 border-amber-100 shadow-sm"
                    )}>
                      <div className="flex items-center gap-3">
                        <div className={cn(
                          "flex h-8 w-8 items-center justify-center rounded-full shadow-sm",
                          inventoryCheck.exists ? "bg-emerald-500 text-white" : "bg-amber-500 text-white"
                        )}>
                          {inventoryCheck.exists ? <Check className="h-4 w-4" /> : <Package className="h-4 w-4" />}
                        </div>
                        <div>
                          <p className="text-sm font-black text-foreground">
                            {inventoryCheck.exists ? "库存状态：已入库" : "库存状态：未入库"}
                          </p>
                          <p className="text-[11px] font-medium text-muted-foreground">
                            {inventoryCheck.exists ? "您可以直接在 Emby 中观看此内容" : "提交求片后，管理员将为您安排下载"}
                          </p>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* TV Season Selection */}
                  {mediaDetail.media_type !== "movie" && mediaDetail.seasons && (
                    <div className="space-y-3">
                      <p className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground">Seasons</p>
                      <div className="flex flex-wrap gap-2">
                        <button
                          onClick={() => setSelectedSeason(undefined)}
                          className={cn(
                            "px-4 py-2 rounded-xl text-xs font-black transition-all border shadow-sm",
                            selectedSeason === undefined 
                              ? "bg-primary text-primary-foreground border-primary shadow-primary/20" 
                              : "bg-white border-white/40 text-muted-foreground hover:bg-white/80"
                          )}
                        >
                          全部季度
                        </button>
                        {Array.from({ length: mediaDetail.seasons }, (_, i) => i + 1).map((s) => {
                          const isAvailable = inventoryCheck?.seasons_available?.includes(s);
                          return (
                            <button
                              key={s}
                              onClick={() => !isAvailable && setSelectedSeason(s)}
                              disabled={isAvailable}
                              className={cn(
                                "px-4 py-2 rounded-xl text-xs font-black transition-all border shadow-sm relative overflow-hidden group",
                                selectedSeason === s 
                                  ? "bg-primary text-primary-foreground border-primary shadow-primary/20" 
                                  : isAvailable
                                    ? "bg-emerald-50 border-emerald-100 text-emerald-600 opacity-60 cursor-not-allowed"
                                    : "bg-white border-white/40 text-muted-foreground hover:bg-white/80"
                              )}
                            >
                              Season {s}
                              {isAvailable && <Check className="ml-1.5 h-3 w-3 inline-block" />}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}

                  {/* Request Note */}
                  <div className="space-y-3">
                    <p className="text-[10px] font-black uppercase tracking-[0.2em] text-muted-foreground">Instructions</p>
                    <Input
                      placeholder="有什么特别的要求吗？（选填）"
                      value={requestNote}
                      onChange={(e) => setRequestNote(e.target.value)}
                      className="rounded-[1.25rem] border-white/60 bg-white/40 shadow-inner h-12"
                    />
                  </div>
                </div>

                <div className="mt-10 flex gap-3">
                  <Button variant="outline" className="flex-1 h-12 rounded-2xl font-black border-white/60 bg-white/40 hover:bg-white transition-all shadow-sm" onClick={() => setSelectedMedia(null)}>
                    关闭
                  </Button>
                  <Button
                    onClick={handleRequest}
                    disabled={isRequesting || (inventoryCheck?.exists && !selectedSeason)}
                    className="flex-[2] h-12 rounded-2xl font-black shadow-xl shadow-primary/20 active:scale-95 transition-all"
                  >
                    {isRequesting ? (
                      <Loader2 className="mr-2 h-5 w-5 animate-spin" />
                    ) : (
                      <Send className="mr-2 h-5 w-5" />
                    )}
                    {inventoryCheck?.exists && !selectedSeason ? "内容已入库" : "立即求片"}
                  </Button>
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}

