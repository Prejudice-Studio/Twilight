"use client";

import { Loader2, Search, Star } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { MediaItem } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import {
  mediaCacheKey,
  type MediaSearchMode,
  type MediaSearchSource,
  type TMDBMediaType,
} from "./media-model";
import { MediaPoster } from "./media-poster";

interface MediaSearchViewProps {
  query: string;
  mode: MediaSearchMode;
  source: MediaSearchSource;
  mediaType: TMDBMediaType;
  results: MediaItem[];
  searching: boolean;
  searched: boolean;
  error: string | null;
  onQueryChange: (value: string) => void;
  onModeChange: (value: MediaSearchMode) => void;
  onSourceChange: (value: MediaSearchSource) => void;
  onMediaTypeChange: (value: TMDBMediaType) => void;
  onSearch: () => void;
  onSelect: (media: MediaItem) => void;
}

export type MediaTypeLabelKey =
  | "media.movieWork"
  | "media.tvSeries"
  | "media.animation"
  | "media.liveAction"
  | "media.book"
  | "media.music"
  | "media.game"
  | "media.unknownType";

export function mediaTypeLabelKey(media: MediaItem): MediaTypeLabelKey {
  switch (String(media.media_type || "").toLowerCase()) {
    case "movie": return "media.movieWork";
    case "tv": return "media.tvSeries";
    case "动画":
    case "anime": return "media.animation";
    case "三次元":
    case "live_action": return "media.liveAction";
    case "书籍": return "media.book";
    case "音乐": return "media.music";
    case "游戏": return "media.game";
    default: return "media.unknownType";
  }
}

export function MediaSearchView(props: MediaSearchViewProps) {
  const { t } = useI18n();
  const modeButton = (active: boolean) => cn(
    "min-h-10 flex-1 px-3 text-sm font-medium transition-colors sm:flex-none sm:px-5",
    active ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
  );
  const sourceButton = (active: boolean) => cn(
    "h-14 min-w-0 flex-1 px-3 text-sm font-medium transition-colors sm:flex-none sm:px-5",
    active ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent hover:text-foreground",
  );

  return (
    <div className="space-y-6">
      <form
        className="space-y-4 border-y border-border/70 bg-card/45 py-5 sm:px-5"
        onSubmit={(event) => {
          event.preventDefault();
          props.onSearch();
        }}
      >
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <span className="shrink-0 text-sm font-medium text-muted-foreground">{t("media.searchMode")}</span>
          <div className="flex min-w-0 rounded-md border bg-muted/50 p-1">
            <button type="button" className={modeButton(props.mode === "name")} onClick={() => props.onModeChange("name")}>{t("media.nameSearch")}</button>
            <button type="button" className={modeButton(props.mode === "id")} onClick={() => props.onModeChange("id")}>{t("media.idSearch")}</button>
          </div>
          {props.mode === "id" && <span className="text-xs text-muted-foreground">{t("media.idSearchHint")}</span>}
        </div>

        <div className="grid min-w-0 gap-3 xl:grid-cols-[minmax(260px,1fr)_auto_auto_auto] xl:items-center">
          <div className="relative min-w-0">
            <Search className="pointer-events-none absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={props.query}
              onChange={(event) => props.onQueryChange(event.target.value)}
              placeholder={props.mode === "id" ? t("media.idPlaceholder") : t("media.namePlaceholder")}
              className="h-14 pl-11 text-base"
              autoComplete="off"
            />
          </div>

          <div className="flex h-14 min-w-0 overflow-hidden rounded-md border bg-background">
            {props.mode === "name" && <button type="button" className={sourceButton(props.source === "all")} onClick={() => props.onSourceChange("all")}>{t("media.allSources")}</button>}
            <button type="button" className={sourceButton(props.source === "tmdb")} onClick={() => props.onSourceChange("tmdb")}>TMDB</button>
            <button type="button" className={sourceButton(props.source === "bangumi")} onClick={() => props.onSourceChange("bangumi")}>Bangumi</button>
          </div>

          {props.mode === "id" && props.source === "tmdb" && (
            <div className="flex h-14 overflow-hidden rounded-md border bg-background">
              <button type="button" className={sourceButton(props.mediaType === "movie")} onClick={() => props.onMediaTypeChange("movie")}>{t("media.movie")}</button>
              <button type="button" className={sourceButton(props.mediaType === "tv")} onClick={() => props.onMediaTypeChange("tv")}>{t("media.tv")}</button>
            </div>
          )}

          <Button type="submit" className="h-14 min-w-28 px-6" disabled={props.searching || !props.query.trim()}>
            {props.searching ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Search className="mr-2 h-4 w-4" />}
            {t("common.search")}
          </Button>
        </div>
      </form>

      {props.error && (
        <div className="flex flex-wrap items-center justify-between gap-3 border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          <span className="break-words">{props.error}</span>
          <Button variant="outline" size="sm" onClick={props.onSearch}>{t("common.retry")}</Button>
        </div>
      )}

      {props.searching && props.results.length === 0 ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
          {Array.from({ length: 10 }, (_, index) => <div key={index} className="aspect-[2/3] animate-pulse rounded-md bg-muted" />)}
        </div>
      ) : props.results.length > 0 ? (
        <section className="space-y-3" aria-labelledby="media-results-title">
          <div className="flex flex-wrap items-end justify-between gap-2">
            <div>
              <h2 id="media-results-title" className="text-xl font-semibold">{t("media.resultsTitle")}</h2>
              <p className="text-sm text-muted-foreground">{t("media.resultsDescription")}</p>
            </div>
            <Badge variant="outline">{t("media.resultCount", { count: props.results.length })}</Badge>
          </div>
          <div className="grid items-start grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {props.results.map((media) => (
              <button
                key={mediaCacheKey(media)}
                type="button"
                onClick={() => props.onSelect(media)}
                aria-label={t("media.openDetails", { title: media.title })}
                className="group min-w-0 overflow-hidden rounded-md border bg-card text-left shadow-sm transition-colors hover:border-primary/50 hover:bg-accent/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                <div className="relative overflow-hidden bg-muted">
                  <MediaPoster src={media.poster} alt={media.title} mediaType={media.media_type} />
                  <div className="absolute left-2 top-2 flex flex-wrap gap-1.5">
                    <Badge variant="secondary" className="border bg-background/90 text-[10px] backdrop-blur-sm">{media.source.toUpperCase()}</Badge>
                    <Badge variant="secondary" className="border bg-background/90 text-[10px] backdrop-blur-sm">{t(mediaTypeLabelKey(media))}</Badge>
                  </div>
                </div>
                <div className="min-w-0 p-3">
                  <h3 className="line-clamp-2 min-h-10 break-words text-sm font-semibold leading-5 group-hover:text-primary">{media.title}</h3>
                  <div className="mt-2 flex min-w-0 items-center justify-between gap-2 text-xs text-muted-foreground">
                    <span className="truncate">{media.year || t("media.unknownYear")}</span>
                    {media.rating !== undefined && media.rating > 0 && (
                      <span className="inline-flex shrink-0 items-center gap-1 font-medium text-amber-600 dark:text-amber-400">
                        <Star className="h-3.5 w-3.5 fill-current" />{media.rating.toFixed(1)}
                      </span>
                    )}
                  </div>
                </div>
              </button>
            ))}
          </div>
        </section>
      ) : props.searched ? (
        <div className="flex min-h-52 flex-col items-center justify-center border-y text-center text-muted-foreground">
          <Search className="mb-3 h-8 w-8 opacity-40" />
          <p className="font-medium text-foreground">{t("media.noResults")}</p>
          <p className="mt-1 text-sm">{t("media.tryAnotherKeyword")}</p>
        </div>
      ) : null}
    </div>
  );
}
