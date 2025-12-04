"use client";

import { useState } from "react";
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
import { api, type MediaItem, type MediaDetail, type InventoryCheckResult } from "@/lib/api";

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

  const handleSearch = async () => {
    if (!searchQuery.trim()) return;

    setIsSearching(true);
    try {
      if (searchMode === "id") {
        // ID 搜索模式
        const mediaId = parseInt(searchQuery.trim());
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
          detailRes = await api.getMediaByTmdbId(mediaId, mediaType);
        } else if (source === "bangumi" || source === "bgm") {
          detailRes = await api.getMediaByBangumiId(mediaId);
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
          
          try {
            // 只检查库存，不再重复获取详情
            const inventoryRes = await api.checkInventory({
              source: detail.source,
              media_id: detail.id,
              media_type: detail.media_type,
              title: detail.title,
              year: detail.year,
            });
            
            // 直接使用已获取的详情数据
            setMediaDetail(detail);
            if (inventoryRes.success && inventoryRes.data) {
              setInventoryCheck(inventoryRes.data);
            }
          } catch (error: any) {
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
      const res = await api.searchMedia(searchQuery, source);
      if (res.success && res.data) {
          // 确保图片字段正确映射
          const mappedResults = res.data.results.map((item: any) => ({
            ...item,
            poster: item.poster || item.poster_url, // 兼容两种字段名
            rating: item.rating || item.vote_average, // 兼容两种字段名
          }));
          setResults(mappedResults);
          if (mappedResults.length === 0) {
          toast({
            title: "未找到结果",
            description: "尝试换个关键词搜索",
          });
          }
        }
      }
    } catch (error: any) {
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
    setSelectedMedia(media);
    setIsLoadingDetail(true);
    setMediaDetail(null);
    setInventoryCheck(null);
    setSelectedSeason(undefined);
    setRequestNote("");

    try {
      // Get detail and check inventory in parallel
      const [detailRes, inventoryRes] = await Promise.all([
        api.getMediaDetail(media.source, media.id, media.media_type),
        api.checkInventory({
          source: media.source,
          media_id: media.id,
          media_type: media.media_type,
          title: media.title,
          year: media.year,
        }),
      ]);

      if (detailRes.success && detailRes.data) {
        setMediaDetail(detailRes.data);
      } else {
        toast({
          title: "获取详情失败",
          description: detailRes.message || "无法获取媒体详情",
          variant: "destructive",
        });
      }
      if (inventoryRes.success && inventoryRes.data) {
        setInventoryCheck(inventoryRes.data);
      }
    } catch (error: any) {
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">媒体搜索</h1>
        <p className="text-muted-foreground">搜索电影、电视剧、动漫，提交求片请求</p>
      </div>

      {/* Search Section */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex flex-col gap-4">
            {/* 搜索模式切换 */}
            <div className="flex items-center gap-4">
              <div className="text-sm font-medium text-muted-foreground">搜索方式:</div>
              <Tabs value={searchMode} onValueChange={(v) => setSearchMode(v as "name" | "id")} className="w-auto">
                <TabsList>
                  <TabsTrigger value="name" className="gap-2">
                    <Type className="h-4 w-4" />
                    名称搜索
                  </TabsTrigger>
                  <TabsTrigger value="id" className="gap-2">
                    <Hash className="h-4 w-4" />
                    ID 搜索
                  </TabsTrigger>
                </TabsList>
              </Tabs>
              
              {searchMode === "id" && (
                <div className="text-xs text-muted-foreground">
                  使用 ID 搜索时请选择具体来源
                </div>
              )}
            </div>

            {/* 搜索输入区域 */}
          <div className="flex flex-col gap-4 sm:flex-row">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                  placeholder={
                    searchMode === "id"
                      ? "输入媒体 ID（纯数字）..."
                      : "输入名称、TMDB URL 或 Bangumi URL..."
                  }
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleSearch()}
                className="pl-10"
              />
            </div>
              
              <Tabs 
                value={source} 
                onValueChange={setSource} 
                className="w-auto"
              >
              <TabsList>
                  {searchMode === "name" && <TabsTrigger value="all">全部</TabsTrigger>}
                <TabsTrigger value="tmdb">TMDB</TabsTrigger>
                <TabsTrigger value="bangumi">Bangumi</TabsTrigger>
              </TabsList>
            </Tabs>

              {/* TMDB 类型选择（仅在 ID 搜索 + TMDB 时显示） */}
              {searchMode === "id" && source === "tmdb" && (
                <Tabs value={mediaType} onValueChange={(v) => setMediaType(v as "movie" | "tv")} className="w-auto">
                  <TabsList>
                    <TabsTrigger value="movie">电影</TabsTrigger>
                    <TabsTrigger value="tv">剧集</TabsTrigger>
                  </TabsList>
                </Tabs>
              )}
              
            <Button onClick={handleSearch} disabled={isSearching}>
              {isSearching ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Search className="mr-2 h-4 w-4" />
              )}
              搜索
            </Button>
            </div>

            {/* 搜索提示 */}
            {searchMode === "name" && (
              <div className="text-xs text-muted-foreground">
                💡 支持名称、TMDB URL (https://www.themoviedb.org/movie/123)、Bangumi URL (https://bgm.tv/subject/456)
              </div>
            )}
            {searchMode === "id" && (
              <div className="text-xs text-muted-foreground">
                💡 示例：TMDB 电影 ID: 550 (搏击俱乐部) | TMDB 剧集 ID: 1399 (权力的游戏) | Bangumi ID: 329982 (葬送的芙莉莲)
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Results */}
      <AnimatePresence mode="wait">
        {results.length > 0 && (
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -20 }}
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
          >
            {results.map((media, index) => (
              <motion.div
                key={`${media.source}-${media.id}`}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.05 }}
              >
                <Card
                  className="cursor-pointer overflow-hidden transition-all hover:ring-2 hover:ring-primary"
                  onClick={() => handleSelectMedia(media)}
                >
                  <div className="aspect-[2/3] relative bg-muted">
                    {media.poster ? (
                      <img
                        src={media.poster}
                        alt={media.title}
                        className="h-full w-full object-cover"
                      />
                    ) : (
                      <div className="flex h-full items-center justify-center">
                        {media.media_type === "movie" ? (
                          <Film className="h-12 w-12 text-muted-foreground" />
                        ) : (
                          <Tv className="h-12 w-12 text-muted-foreground" />
                        )}
                      </div>
                    )}
                    <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent p-3">
                      <div className="flex items-center gap-2">
                        <Badge
                          variant={media.source === "tmdb" ? "default" : "secondary"}
                          className="text-xs"
                        >
                          {media.source.toUpperCase()}
                        </Badge>
                        {media.rating && (
                          <Badge variant="outline" className="text-xs">
                            <Star className="mr-1 h-3 w-3 fill-yellow-400 text-yellow-400" />
                            {media.rating.toFixed(1)}
                          </Badge>
                        )}
                      </div>
                    </div>
                  </div>
                  <CardContent className="p-3">
                    <h3 className="font-medium line-clamp-1">{media.title}</h3>
                    <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                      <span>{media.media_type === "movie" ? "电影" : "剧集"}</span>
                      {media.year && (
                        <>
                          <span>•</span>
                          <span>{media.year}</span>
                        </>
                      )}
                    </div>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </motion.div>
        )}
      </AnimatePresence>

      {/* Detail Dialog */}
      <Dialog open={!!selectedMedia} onOpenChange={() => {
        setSelectedMedia(null);
        setMediaDetail(null);
        setInventoryCheck(null);
      }}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          {isLoadingDetail ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : !selectedMedia ? null : mediaDetail ? (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  {mediaDetail.title}
                  {mediaDetail.year && (
                    <Badge variant="outline">{mediaDetail.year}</Badge>
                  )}
                </DialogTitle>
                {mediaDetail.original_title && mediaDetail.original_title !== mediaDetail.title && (
                  <DialogDescription>{mediaDetail.original_title}</DialogDescription>
                )}
              </DialogHeader>

              <div className="grid gap-4 md:grid-cols-[1fr,2fr]">
                <div className="aspect-[2/3] overflow-hidden rounded-lg bg-muted">
                  {mediaDetail.poster ? (
                    <img
                      src={mediaDetail.poster}
                      alt={mediaDetail.title}
                      className="h-full w-full object-cover"
                    />
                  ) : (
                    <div className="flex h-full items-center justify-center">
                      <Film className="h-12 w-12 text-muted-foreground" />
                    </div>
                  )}
                </div>

                <div className="space-y-4">
                  {/* Inventory Check */}
                  {inventoryCheck && (
                    <div
                      className={`rounded-lg p-3 ${
                        inventoryCheck.exists
                          ? "bg-emerald-500/10 border border-emerald-500/30"
                          : "bg-amber-500/10 border border-amber-500/30"
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        {inventoryCheck.exists ? (
                          <Check className="h-4 w-4 text-emerald-500" />
                        ) : (
                          <Package className="h-4 w-4 text-amber-500" />
                        )}
                        <span className="text-sm font-medium">
                          {inventoryCheck.exists ? "库中已有" : "库中暂无"}
                        </span>
                      </div>
                      {inventoryCheck.seasons_available && inventoryCheck.seasons_available.length > 0 && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          已有季度: {inventoryCheck.seasons_available.join(", ")}
                        </p>
                      )}
                    </div>
                  )}

                  {/* Info */}
                  <div className="space-y-2 text-sm">
                    <div className="flex items-center gap-2">
                      <Badge variant={selectedMedia?.source === "tmdb" ? "default" : "secondary"}>
                        {selectedMedia?.source.toUpperCase()}
                      </Badge>
                      <Badge variant="outline">
                        {mediaDetail.media_type === "movie" ? "电影" : "剧集"}
                      </Badge>
                      {mediaDetail.rating && (
                        <Badge variant="outline">
                          <Star className="mr-1 h-3 w-3 fill-yellow-400 text-yellow-400" />
                          {mediaDetail.rating.toFixed(1)}
                        </Badge>
                      )}
                    </div>

                    {mediaDetail.genres && mediaDetail.genres.length > 0 && (
                      <div className="flex flex-wrap gap-1">
                        {mediaDetail.genres.map((genre) => (
                          <Badge key={genre} variant="secondary" className="text-xs">
                            {genre}
                          </Badge>
                        ))}
                      </div>
                    )}

                    {mediaDetail.overview && (
                      <p className="text-muted-foreground line-clamp-4">
                        {mediaDetail.overview}
                      </p>
                    )}
                  </div>

                  {/* Season Select (for TV) */}
                  {mediaDetail.media_type !== "movie" && mediaDetail.seasons && (
                    <div className="space-y-2">
                      <Label>选择季度（可选）</Label>
                      <div className="flex flex-wrap gap-2">
                        <Button
                          size="sm"
                          variant={selectedSeason === undefined ? "default" : "outline"}
                          onClick={() => setSelectedSeason(undefined)}
                        >
                          全部
                        </Button>
                        {Array.from({ length: mediaDetail.seasons }, (_, i) => i + 1).map((s) => (
                          <Button
                            key={s}
                            size="sm"
                            variant={selectedSeason === s ? "default" : "outline"}
                            onClick={() => setSelectedSeason(s)}
                            disabled={inventoryCheck?.seasons_available?.includes(s)}
                          >
                            第 {s} 季
                            {inventoryCheck?.seasons_available?.includes(s) && (
                              <Check className="ml-1 h-3 w-3" />
                            )}
                          </Button>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Note */}
                  <div className="space-y-2">
                    <Label>备注（可选）</Label>
                    <Input
                      placeholder="例如：希望有中文字幕"
                      value={requestNote}
                      onChange={(e) => setRequestNote(e.target.value)}
                    />
                  </div>
                </div>
              </div>

              <DialogFooter>
                <Button variant="outline" onClick={() => setSelectedMedia(null)}>
                  取消
                </Button>
                <Button
                  variant="gradient"
                  onClick={handleRequest}
                  disabled={isRequesting || (inventoryCheck?.exists && !selectedSeason)}
                >
                  {isRequesting ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Send className="mr-2 h-4 w-4" />
                  )}
                  提交求片
                </Button>
              </DialogFooter>
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}

