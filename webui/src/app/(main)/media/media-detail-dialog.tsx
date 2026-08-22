"use client";

import { Check, ExternalLink, Loader2, Package, Send, Star } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { InventoryCheckResult, MediaDetail } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { sanitizeExternalUrl, sanitizeImageUrl } from "@/lib/safe-url";
import { mediaTypeLabelKey } from "./media-search-view";
import { MediaPoster } from "./media-poster";

interface MediaDetailDialogProps {
  detail: MediaDetail | null;
  inventory: InventoryCheckResult | null;
  loading: boolean;
  requesting: boolean;
  selectedSeason?: number;
  note: string;
  onClose: () => void;
  onSeasonChange: (season?: number) => void;
  onNoteChange: (note: string) => void;
  onSubmit: () => void;
}

interface FactProps {
  label: string;
  value?: string | number | null;
}

function Fact({ label, value }: FactProps) {
  if (value === undefined || value === null || value === "") return null;
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words text-sm font-medium leading-5">{value}</dd>
    </div>
  );
}

export function MediaDetailDialog(props: MediaDetailDialogProps) {
  const { t } = useI18n();
  const detail = props.detail;
  const poster = sanitizeImageUrl(detail?.poster || detail?.poster_url);
  const logo = sanitizeImageUrl(detail?.logo || detail?.logo_url);
  const sourceURL = sanitizeExternalUrl(detail?.source_url);
  const officialURL = sanitizeExternalUrl(detail?.official_url);
  const trailerURL = sanitizeExternalUrl(detail?.trailer_url);
  const selectedSeasonAvailable = Boolean(
    props.selectedSeason !== undefined && props.inventory?.seasons_available?.includes(props.selectedSeason),
  );
  const requiresIssueNote = Boolean(
    props.inventory?.exists && (props.selectedSeason === undefined || selectedSeasonAvailable),
  );
  const issueReady = Boolean(requiresIssueNote && props.note.trim());
  const submitBlocked = Boolean(requiresIssueNote && !props.note.trim());

  return (
    <Dialog open={Boolean(detail)} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent className="!max-h-[calc(100dvh-1rem)] !w-[calc(100vw-1rem)] !max-w-[calc(100vw-1rem)] !gap-0 overflow-y-auto overflow-x-hidden rounded-md !p-0 sm:!max-h-[calc(100dvh-2rem)] lg:!w-[min(1140px,calc(100vw-1rem))] lg:overflow-hidden [&>button:last-child]:z-20 [&>button:last-child]:bg-background/90 [&>button:last-child]:p-1.5 [&>button:last-child]:shadow-sm">
        {detail && (
          <>
            <DialogHeader className="sr-only">
              <DialogTitle>{detail.title}</DialogTitle>
              <DialogDescription>{t("media.detailDialogDescription", { title: detail.title })}</DialogDescription>
            </DialogHeader>

            <div className="grid min-w-0 lg:grid-cols-[auto_minmax(0,1fr)]">
              <aside className="min-w-0 bg-muted/45 lg:w-fit">
                <div className="w-full lg:w-fit">
                  <MediaPoster
                    src={poster}
                    alt={detail.title}
                    mediaType={detail.media_type}
                    eager
                    imageClassName="lg:h-auto lg:w-auto lg:max-h-[calc(100dvh-2rem)] lg:max-w-[420px]"
                    fallbackClassName="lg:w-[min(420px,calc((100dvh-2rem)*2/3))]"
                    iconClassName="h-16 w-16"
                  />
                </div>
              </aside>

              <div className="relative min-w-0 lg:h-full lg:min-h-0 lg:self-stretch">
              <main className="min-w-0 lg:absolute lg:inset-0 lg:flex lg:min-h-0 lg:flex-col">
                <div className="space-y-6 p-4 sm:p-6 lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:p-7">
                  {props.loading && (
                    <div className="flex items-center gap-2 border border-info/25 bg-info/5 px-3 py-2 text-sm text-info">
                      <Loader2 className="h-4 w-4 animate-spin" />{t("media.loadingDetails")}
                    </div>
                  )}

                  <header className="min-w-0 border-b pb-5">
                    {logo && (
                      <div className="mb-5 flex min-h-20 items-end">
                        {/* Transparent title artwork stays unframed and keeps its native ratio. */}
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img src={logo} alt={t("media.logoAlt", { title: detail.title })} loading="lazy" decoding="async" className="block h-auto max-h-28 max-w-[min(100%,360px)] object-contain object-left drop-shadow-sm" />
                      </div>
                    )}
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="secondary">{t(mediaTypeLabelKey(detail))}</Badge>
                      <Badge variant="outline">{detail.source.toUpperCase()}</Badge>
                      {detail.rating !== undefined && detail.rating > 0 && (
                        <Badge variant="warning" className="gap-1"><Star className="h-3 w-3 fill-current" />{detail.rating.toFixed(1)}</Badge>
                      )}
                    </div>
                    <h2 className="mt-3 break-words text-2xl font-semibold leading-tight sm:text-3xl">{detail.title}</h2>
                    {detail.original_title && detail.original_title !== detail.title && (
                      <p className="mt-2 break-words text-sm text-muted-foreground">{t("media.originalTitle")}: {detail.original_title}</p>
                    )}
                  </header>

                  <div className="flex flex-wrap gap-2">
                    {detail.genres?.map((genre) => <Badge key={genre} variant="secondary">{genre}</Badge>)}
                    {sourceURL && <Button asChild size="sm" variant="outline" className="sm:ml-auto"><a href={sourceURL} target="_blank" rel="noopener noreferrer">{t("media.openSource")}<ExternalLink className="ml-1.5 h-3.5 w-3.5" /></a></Button>}
                    {officialURL && officialURL !== sourceURL && <Button asChild size="sm" variant="outline"><a href={officialURL} target="_blank" rel="noopener noreferrer">{t("media.openOfficialSite")}<ExternalLink className="ml-1.5 h-3.5 w-3.5" /></a></Button>}
                    {trailerURL && <Button asChild size="sm" variant="outline"><a href={trailerURL} target="_blank" rel="noopener noreferrer">{t("media.watchTrailer")}<ExternalLink className="ml-1.5 h-3.5 w-3.5" /></a></Button>}
                  </div>

                  <dl className="grid gap-x-5 gap-y-4 border-y py-4 sm:grid-cols-2 xl:grid-cols-3">
                    <Fact label={t("media.sourceIdentifier")} value={`${detail.source.toUpperCase()}#${detail.id}`} />
                    <Fact label={t("media.releaseDate")} value={detail.release_date || detail.year || t("media.unknownYear")} />
                    <Fact label={t("media.runtime")} value={detail.runtime && detail.runtime > 0 ? t("media.runtimeMinutes", { minutes: detail.runtime }) : null} />
                    <Fact label={t("media.episodes")} value={detail.episodes && detail.episodes > 0 ? t("media.episodeCount", { count: detail.episodes }) : null} />
                    <Fact label={t("media.seasons")} value={detail.seasons && detail.seasons > 0 ? t("media.seasonCount", { count: detail.seasons }) : null} />
                    <Fact label={t("media.volumes")} value={detail.volumes && detail.volumes > 0 ? t("media.volumeCount", { count: detail.volumes }) : null} />
                    <Fact label={t("media.sourceStatus")} value={detail.status} />
                    <Fact label={t("media.endDate")} value={detail.end_date} />
                    <Fact label={t("media.platform")} value={detail.platform} />
                    <Fact label={t("media.broadcast")} value={detail.broadcast} />
                    <Fact label={t("media.rank")} value={detail.rank && detail.rank > 0 ? t("media.rankValue", { rank: detail.rank }) : null} />
                    <Fact label={t("media.voteCount")} value={detail.vote_count && detail.vote_count > 0 ? t("media.voteCountValue", { count: detail.vote_count }) : null} />
                    <Fact label={t("media.countries")} value={detail.countries?.join(" / ")} />
                    <Fact label={t("media.languages")} value={detail.languages?.join(" / ")} />
                  </dl>

                  {detail.tagline && <p className="border-l-2 border-info/40 pl-4 text-sm italic text-muted-foreground">{detail.tagline}</p>}

                  {detail.aliases?.length ? <section><h3 className="text-sm font-semibold">{t("media.aliases")}</h3><p className="mt-2 break-words text-sm leading-6 text-muted-foreground">{detail.aliases.join(" / ")}</p></section> : null}

                  {(detail.creators?.length || detail.studios?.length || detail.cast?.length) ? (
                    <dl className="grid gap-4 border-y py-4 text-sm sm:grid-cols-2">
                      <Fact label={t("media.creators")} value={detail.creators?.join(" / ")} />
                      <Fact label={t("media.studios")} value={detail.studios?.join(" / ")} />
                      <div className="min-w-0 sm:col-span-2"><Fact label={t("media.cast")} value={detail.cast?.join(" / ")} /></div>
                    </dl>
                  ) : null}

                  <section>
                    <h3 className="text-sm font-semibold">{t("media.about")}</h3>
                    <p className="mt-2 whitespace-pre-line break-words text-sm leading-6 text-muted-foreground">{detail.overview || t("media.noOverview")}</p>
                  </section>

                  {props.inventory && (
                    <section className={cn("border p-4", props.inventory.exists ? "border-success/30 bg-success/5" : "border-warning/30 bg-warning/5")}>
                      <div className="flex items-start gap-3">
                        <div className={cn("flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-white", props.inventory.exists ? "bg-success" : "bg-warning")}>
                          {props.inventory.exists ? <Check className="h-4 w-4" /> : <Package className="h-4 w-4" />}
                        </div>
                        <div className="min-w-0"><h3 className="text-sm font-semibold">{props.inventory.exists ? t("media.inventoryAvailable") : t("media.inventoryMissing")}</h3><p className="mt-1 text-xs leading-5 text-muted-foreground">{props.inventory.exists ? t("media.inventoryAvailableDescription") : t("media.inventoryMissingDescription")}</p></div>
                      </div>
                    </section>
                  )}

                  {detail.media_type !== "movie" && detail.seasons !== undefined && detail.seasons > 0 && (
                    <section className="space-y-3">
                      <h3 className="text-sm font-semibold">{t("media.seasons")}</h3>
                      <div className="flex flex-wrap gap-2">
                        <Button size="sm" variant={props.selectedSeason === undefined ? "default" : "outline"} onClick={() => props.onSeasonChange(undefined)}>{t("media.allSeasons")}</Button>
                        {Array.from({ length: detail.seasons }, (_, index) => index + 1).map((season) => {
                          const available = props.inventory?.seasons_available?.includes(season);
                          return <Button key={season} size="sm" variant={props.selectedSeason === season ? "default" : "outline"} className={available ? "border-success/40 text-success" : undefined} onClick={() => props.onSeasonChange(season)}>{t("media.season", { season })}{available && <Check className="ml-1.5 h-3 w-3" />}</Button>;
                        })}
                      </div>
                    </section>
                  )}

                  <section className="space-y-2">
                    <Label htmlFor="media-request-note">{t("media.instructions")}</Label>
                    <Input id="media-request-note" value={props.note} maxLength={500} onChange={(event) => props.onNoteChange(event.target.value)} placeholder={props.inventory?.exists ? t("media.issuePlaceholder") : t("media.requestPlaceholder")} />
                  </section>

                </div>
                <footer className="flex shrink-0 flex-col gap-2 border-t bg-background px-4 py-4 sm:flex-row sm:px-6 lg:px-7">
                  <Button variant="outline" className="sm:flex-1" onClick={props.onClose}>{t("common.close")}</Button>
                  <Button className="sm:flex-[2]" onClick={props.onSubmit} disabled={props.requesting || submitBlocked}>
                    {props.requesting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
                    {submitBlocked ? t("media.submitIssueAfterNote") : issueReady ? t("media.submitIssue") : t("media.requestNow")}
                  </Button>
                </footer>
              </main>
              </div>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
