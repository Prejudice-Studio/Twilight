"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import Image from "next/image";
import {
  Film,
  Check,
  X,
  Clock,
  Loader2,
  ChevronLeft,
  ChevronRight,
  MessageSquare,
  Hash,
  Fingerprint,
  Trash2,
  ExternalLink,
  Copy,
  Search,
  RefreshCw,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button, IconButton } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { PageError } from "@/components/layout/page-state";
import { api, type MediaRequest, type MediaRequestStatusCounts } from "@/lib/api";
import { ApiError } from "@/lib/api-request";
import { formatDate } from "@/lib/utils";
import { mediaRequestExternalUrl } from "@/lib/media-external-url";
import { useI18n } from "@/lib/i18n";
import { sanitizeImageUrl } from "@/lib/safe-url";

type AdminRequestStatus = keyof MediaRequestStatusCounts;

const EMPTY_STATUS_COUNTS: MediaRequestStatusCounts = {
  all: 0,
  active: 0,
  pending: 0,
  accepted: 0,
  downloading: 0,
  rejected: 0,
  completed: 0,
};

function isActiveStatus(status: string): boolean {
  return status === "pending" || status === "accepted" || status === "downloading";
}

function matchesStatusFilter(status: string, filter: AdminRequestStatus): boolean {
  if (filter === "all") return true;
  if (filter === "active") return isActiveStatus(status);
  return status === filter;
}

function updateStatusCounts(
  counts: MediaRequestStatusCounts,
  previousStatus: string,
  nextStatus?: string,
): MediaRequestStatusCounts {
  const next = { ...counts };
  const previousKey = previousStatus as keyof MediaRequestStatusCounts;
  if (previousKey in next) next[previousKey] = Math.max(0, next[previousKey] - 1);
  next.all = Math.max(0, next.all - 1);
  if (isActiveStatus(previousStatus)) next.active = Math.max(0, next.active - 1);
  if (nextStatus) {
    const nextKey = nextStatus as keyof MediaRequestStatusCounts;
    if (nextKey in next) next[nextKey] += 1;
    next.all += 1;
    if (isActiveStatus(nextStatus)) next.active += 1;
  }
  return next;
}

function mediaRequestPoster(request: MediaRequest): string | null {
  return sanitizeImageUrl(request.media_info?.poster || request.media_info?.poster_url) || null;
}

function mediaRequestRating(value: unknown): string | null {
  const rating = typeof value === "number" ? value : Number(value);
  return Number.isFinite(rating) && rating > 0 ? rating.toFixed(1) : null;
}

export default function AdminRequestsPage() {
  const { toast } = useToast();
  const { confirm } = useConfirm();
  const { t } = useI18n();
  const [requests, setRequests] = useState<MediaRequest[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const [status, setStatus] = useState<AdminRequestStatus>("active");
  const [source, setSource] = useState("all");
  const [searchInput, setSearchInput] = useState("");
  const [query, setQuery] = useState("");
  const [statusCounts, setStatusCounts] = useState<MediaRequestStatusCounts>(EMPTY_STATUS_COUNTS);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loadedKey, setLoadedKey] = useState("");
  const [hasLoaded, setHasLoaded] = useState(false);
  const [deletingKey, setDeletingKey] = useState("");
  const requestAbortRef = useRef<AbortController | null>(null);
  const requestSequenceRef = useRef(0);

  // Action dialog
  const [actionOpen, setActionOpen] = useState(false);
  const [selectedRequest, setSelectedRequest] = useState<MediaRequest | null>(null);
  const [selectedStatus, setSelectedStatus] = useState("accepted");
  const [adminNote, setAdminNote] = useState("");
  const [isActioning, setIsActioning] = useState(false);
  const requestKey = `${page}|${perPage}|${status}|${source}|${query}`;

  const loadRequests = useCallback(async () => {
    requestAbortRef.current?.abort();
    const controller = new AbortController();
    requestAbortRef.current = controller;
    const sequence = ++requestSequenceRef.current;
    setIsLoading(true);
    setLoadError(null);
    try {
      const res = await api.getMediaRequests({ page, perPage, status, source, query }, controller.signal);
      if (controller.signal.aborted || sequence !== requestSequenceRef.current) return;
      if (!res.success || !res.data) {
        throw new Error(res.message || t("adminRequests.loadFailed"));
      }
      setRequests(res.data.requests);
      setTotal(res.data.total);
      setStatusCounts({ ...EMPTY_STATUS_COUNTS, ...res.data.status_counts });
      setLoadedKey(requestKey);
      setHasLoaded(true);
      if (res.data.total_pages > 0 && page > res.data.total_pages) {
        setPage(res.data.total_pages);
      }
    } catch (error: unknown) {
      if (controller.signal.aborted) return;
      const message = error instanceof Error ? error.message : t("adminRequests.loadFailed");
      setLoadError(message);
    } finally {
      if (sequence === requestSequenceRef.current) {
        setIsLoading(false);
      }
      if (requestAbortRef.current === controller) {
        requestAbortRef.current = null;
      }
    }
  }, [page, perPage, query, requestKey, source, status, t]);

  useEffect(() => {
    void loadRequests();
    return () => requestAbortRef.current?.abort();
  }, [loadRequests]);

  const handleSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nextQuery = searchInput.trim().slice(0, 120);
    if (nextQuery === query && page === 1) {
      void loadRequests();
      return;
    }
    setPage(1);
    setQuery(nextQuery);
  };

  const handleMutationConflict = (error: unknown): boolean => {
    if (!(error instanceof ApiError) || error.errorCode !== "MEDIA_REQUEST_CONFLICT") return false;
    toast({
      title: t("adminRequests.conflictTitle"),
      description: t("adminRequests.conflictDescription"),
      variant: "destructive",
    });
    setActionOpen(false);
    void loadRequests();
    return true;
  };

  const handleAction = async () => {
    if (!selectedRequest) return;
    if (!selectedRequest.require_key) {
      toast({ title: t("adminRequests.missingRequireKey"), variant: "destructive" });
      return;
    }

    setIsActioning(true);
    try {
      const previous = selectedRequest;
      const res = await api.updateMediaRequest(
        previous.require_key,
        selectedStatus,
        adminNote,
        previous.revision,
      );

      if (res.success && res.data) {
        const updated = res.data;
        toast({
          title: t("common.operationSuccess"),
          variant: "success",
        });
        setActionOpen(false);
        setSelectedRequest(null);
        setAdminNote("");
        setStatusCounts((current) => updateStatusCounts(current, previous.status, updated.status));
        if (matchesStatusFilter(updated.status, status)) {
          setRequests((current) => current.map((item) => (
            item.require_key === previous.require_key ? updated : item
          )));
        } else {
          setRequests((current) => current.filter((item) => item.require_key !== previous.require_key));
          setTotal((current) => Math.max(0, current - 1));
        }
      } else {
        toast({ title: t("common.operationFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: unknown) {
      if (!handleMutationConflict(error)) {
        toast({
          title: t("common.operationFailed"),
          description: error instanceof Error ? error.message : t("common.networkError"),
          variant: "destructive",
        });
      }
    } finally {
      setIsActioning(false);
    }
  };

  const handleDelete = async (request: MediaRequest) => {
    if (!request.require_key) {
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
      setDeletingKey(request.require_key);
      const res = await api.deleteMediaRequest(request.require_key, request.revision);
      if (res.success) {
        toast({ title: t("media.deleteSuccess"), variant: "success" });
        setRequests((current) => current.filter((item) => item.require_key !== request.require_key));
        setStatusCounts((current) => updateStatusCounts(current, request.status));
        setTotal((current) => Math.max(0, current - 1));
        if (requests.length === 1 && page > 1) {
          setPage((current) => Math.max(1, current - 1));
        }
      } else {
        toast({ title: t("common.deleteFailed"), description: res.message, variant: "destructive" });
      }
    } catch (error: unknown) {
      if (!handleMutationConflict(error)) {
        toast({
          title: t("common.deleteFailed"),
          description: error instanceof Error ? error.message : t("common.networkError"),
          variant: "destructive",
        });
      }
    } finally {
      setDeletingKey("");
    }
  };

  const openActionDialog = (request: MediaRequest) => {
    setSelectedRequest(request);
    setSelectedStatus(request.status === "pending" ? "accepted" : request.status);
    setAdminNote(request.admin_note || "");
    setActionOpen(true);
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "pending":
        return (
          <Badge variant="warning">
            <Clock className="mr-1 h-3 w-3" />
            {t("media.statusUnhandled")}
          </Badge>
        );
      case "accepted":
        return (
          <Badge variant="success">
            <Check className="mr-1 h-3 w-3" />
            {t("media.statusAccepted")}
          </Badge>
        );
      case "rejected":
        return (
          <Badge variant="destructive">
            <X className="mr-1 h-3 w-3" />
            {t("media.statusRejected")}
          </Badge>
        );
      case "downloading":
        return (
          <Badge variant="info">
            <Loader2 className="mr-1 h-3 w-3 animate-spin" />
            {t("media.statusDownloading")}
          </Badge>
        );
      case "completed":
        return (
          <Badge variant="success">
            <Check className="mr-1 h-3 w-3" />
            {t("media.statusCompleted")}
          </Badge>
        );
      default:
        return <Badge variant="secondary">{status}</Badge>;
    }
  };

  const pages = Math.ceil(total / perPage);
  const isChangingView = isLoading && loadedKey !== requestKey;

  if (loadError && !hasLoaded) {
    return <PageError message={loadError} onRetry={() => void loadRequests()} />;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold sm:text-3xl">{t("adminRequests.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("adminRequests.description")}</p>
        </div>
        <Badge variant="outline" className="self-start px-3 py-1.5 text-sm sm:self-auto sm:px-4 sm:py-2 sm:text-lg">
          {t("adminReview.totalRequests", { count: total })}
        </Badge>
      </div>

      <div className="flex flex-col gap-3 rounded-xl border bg-card/70 p-3 sm:p-4">
        <form className="flex flex-col gap-2 lg:flex-row" onSubmit={handleSearch}>
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <Input
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              placeholder={t("adminRequests.searchPlaceholder")}
              aria-label={t("adminRequests.searchLabel")}
              className="h-11 pl-9"
            />
          </div>
          <Select value={source} onValueChange={(value) => { setSource(value); setPage(1); }}>
            <SelectTrigger className="h-11 w-full lg:w-36" aria-label={t("adminRequests.sourceLabel")}>
              <SelectValue placeholder={t("adminRequests.sourceLabel")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("adminReview.all")}</SelectItem>
              <SelectItem value="tmdb">TMDB</SelectItem>
              <SelectItem value="bangumi">Bangumi</SelectItem>
            </SelectContent>
          </Select>
          <Button type="submit" className="h-11 shrink-0 px-5">
            <Search className="mr-2 h-4 w-4" />
            {t("common.search")}
          </Button>
          <Button type="button" variant="outline" className="h-11 shrink-0" onClick={() => void loadRequests()} disabled={isLoading}>
            <RefreshCw className={isLoading ? "mr-2 h-4 w-4 animate-spin" : "mr-2 h-4 w-4"} />
            {t("common.refresh")}
          </Button>
        </form>

        <Tabs value={status} onValueChange={(value) => { setStatus(value as AdminRequestStatus); setPage(1); }}>
          <TabsList className="flex w-full overflow-x-auto sm:inline-flex sm:w-auto">
            <TabsTrigger value="active">{t("adminRequests.activeQueue")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.active}</span></TabsTrigger>
            <TabsTrigger value="pending">{t("media.statusUnhandled")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.pending}</span></TabsTrigger>
            <TabsTrigger value="accepted">{t("media.statusAccepted")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.accepted}</span></TabsTrigger>
            <TabsTrigger value="downloading">{t("media.statusDownloading")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.downloading}</span></TabsTrigger>
            <TabsTrigger value="rejected">{t("media.statusRejected")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.rejected}</span></TabsTrigger>
            <TabsTrigger value="completed">{t("media.statusCompleted")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.completed}</span></TabsTrigger>
            <TabsTrigger value="all">{t("adminReview.all")} <span className="ml-1 text-xs text-muted-foreground">{statusCounts.all}</span></TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {loadError && hasLoaded && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-amber-300/50 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
          <span>{loadError}</span>
          <Button size="sm" variant="outline" onClick={() => void loadRequests()}>{t("common.retry")}</Button>
        </div>
      )}

      {/* Requests List */}
      <Card>
        <CardContent className="p-0">
          {isChangingView ? (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-primary" />
            </div>
          ) : requests.length === 0 ? (
            <div className="flex h-64 items-center justify-center text-muted-foreground">
              {t("adminReview.emptyRequests", { pending: status === "pending" ? t("adminReview.pendingPrefix") : "" })}
            </div>
          ) : (
            <div className="divide-y">
              {requests.map((request) => (
                <div
                  key={request.require_key || `${request.source}-${request.id}`}
                  className="flex flex-col gap-3 p-4 hover:bg-muted/30 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="flex min-w-0 flex-1 items-start gap-4">
                    <div className="relative flex h-20 w-14 shrink-0 items-center justify-center rounded-lg bg-primary/5 overflow-hidden border border-primary/10">
                      {mediaRequestPoster(request) ? (
                        <Image
                          src={mediaRequestPoster(request) || ""}
                          alt={request.media_info?.title || request.title}
                          fill
                          unoptimized
                          sizes="56px"
                          className="h-full w-full object-cover"
                        />
                      ) : (
                        <Film className="h-6 w-6 text-primary/50" />
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        {(() => {
                        const url = mediaRequestExternalUrl(request);
                          const title = request.media_info?.title || request.title;
                          if (!url) return <p className="break-words font-medium">{title}</p>;
                          return (
                            <a
                              href={url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="break-words font-medium underline decoration-dotted underline-offset-2 hover:text-primary inline-flex items-center gap-1"
                              title={t("media.viewOnSource", { source: request.source.toUpperCase() })}
                            >
                              {title}
                              <ExternalLink className="h-3 w-3 shrink-0 opacity-70" />
                            </a>
                          );
                        })()}
                        {request.media_info?.season && (
                          <Badge variant="outline" className="text-xs">
                            {t("media.season", { season: request.media_info.season })}
                          </Badge>
                        )}
                        {mediaRequestRating(request.media_info?.vote_average) && (
                          <Badge variant="outline" className="text-xs border-amber-500/20 text-amber-500">
                             ★ {mediaRequestRating(request.media_info?.vote_average)}
                          </Badge>
                        )}
                        {mediaRequestRating(request.media_info?.rating) && (
                          <Badge variant="outline" className="text-xs border-amber-500/20 text-amber-500">
                             ★ {mediaRequestRating(request.media_info?.rating)}
                          </Badge>
                        )}
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                        <div className="flex items-center gap-1">
                          {(() => {
                                const url = mediaRequestExternalUrl(request);
                            const inner = (
                              <Badge variant="secondary" className="h-5 text-[10px]">
                                {request.source.toLowerCase() === "bangumi" ? "Bangumi" : request.source.toUpperCase()}
                              </Badge>
                            );
                            return url ? (
                              <a
                                href={url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="inline-flex items-center gap-1 hover:opacity-80"
                                title={t("media.viewOnSource", { source: request.source.toUpperCase() })}
                              >
                                {inner}
                              </a>
                            ) : inner;
                          })()}
                        </div>
                        <span className="hidden sm:inline">•</span>
                        <span className="flex items-center gap-0.5" title={t("adminRequests.internalId", { source: request.source })}>
                          <Hash className="h-3 w-3" />{request.id}
                        </span>
                        {request.media_id !== undefined && request.media_id !== null && request.media_id !== "" && (
                          <>
                            <span className="hidden sm:inline">•</span>
                            <span className="flex items-center gap-0.5" title={`${request.source.toUpperCase()} ID`}>
                              {request.source.toUpperCase()}#{String(request.media_id)}
                            </span>
                          </>
                        )}
                        <span className="hidden sm:inline">•</span>
                        <span className="flex min-w-0 items-center gap-0.5" title={t("media.externalKeyTitle")}>
                          <Fingerprint className="h-3 w-3 shrink-0" />
                          <code className="max-w-[10rem] truncate rounded bg-muted px-1 text-foreground sm:max-w-[16rem]">
                            {request.require_key}
                          </code>
                          <IconButton
                            type="button"
                            variant="ghost"
                            className="h-5 w-5 text-muted-foreground hover:text-foreground"
                            title={t("media.copyKey")}
                            aria-label={t("media.copyKey")}
                            onClick={() => {
                              navigator.clipboard.writeText(request.require_key).then(
                                () => toast({ title: t("adminRequests.keyCopied"), variant: "success" }),
                                () => toast({ title: t("common.copyFailed"), variant: "destructive" }),
                              );
                            }}
                          >
                            <Copy className="h-3 w-3" />
                          </IconButton>
                        </span>
                        <span className="hidden sm:inline">•</span>
                        <span>{request.media_info?.media_type === "movie" ? t("media.movie") : t("media.tv")}</span>
                        <span className="hidden sm:inline">•</span>
                        <span>{formatDate(request.timestamp)}</span>
                        {request.user && (
                          <>
                            <span className="hidden sm:inline">•</span>
                            <span className="truncate">{t("adminRequests.user", { username: request.user.username || request.user.telegram_id })}</span>
                          </>
                        )}
                      </div>
                      {request.media_info?.overview && (
                        <p className="mt-2 line-clamp-2 max-w-full break-words text-xs text-muted-foreground sm:max-w-2xl">
                          {request.media_info.overview}
                        </p>
                      )}
                      {request.media_info?.note && (
                        <p className="mt-1 break-words text-xs text-muted-foreground">
                          <MessageSquare className="mr-1 inline h-3 w-3" />
                          {request.media_info.note}
                        </p>
                      )}
                      {request.admin_note && (
                        <p className="mt-1 break-words text-xs text-primary">
                          {t("adminRequests.adminNote", { note: request.admin_note })}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-2 sm:gap-3 sm:self-center">
                    {getStatusBadge(request.status)}
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => openActionDialog(request)}
                    >
                      {t("adminRequests.handle")}
                    </Button>
                    <IconButton
                      variant="ghost"
                      className="h-8 w-8 text-muted-foreground hover:text-destructive dark:hover:bg-destructive/15"
                      onClick={() => void handleDelete(request)}
                      disabled={deletingKey === request.require_key}
                      title={t("adminRequests.deleteRequest")}
                      aria-label={t("adminRequests.deleteRequest")}
                    >
                      {deletingKey === request.require_key ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                    </IconButton>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <div className="flex flex-wrap items-center justify-center gap-2">
        {pages > 1 && (
          <>
            <IconButton
              variant="outline"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1}
              aria-label={t("adminRequests.previousPage")}
            >
              <ChevronLeft className="h-4 w-4" />
            </IconButton>
            <span className="text-sm">
              {t("adminRequests.pageStatus", { page, pages })}
            </span>
            <IconButton
              variant="outline"
              onClick={() => setPage((p) => Math.min(pages, p + 1))}
              disabled={page === pages}
              aria-label={t("adminRequests.nextPage")}
            >
              <ChevronRight className="h-4 w-4" />
            </IconButton>
          </>
        )}
          <Select value={String(perPage)} onValueChange={(value) => { setPerPage(Number(value)); setPage(1); }}>
            <SelectTrigger className="h-10 w-28" aria-label={t("adminRequests.pageSize")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="20">20 / {t("adminRequests.pageSizeShort")}</SelectItem>
              <SelectItem value="50">50 / {t("adminRequests.pageSizeShort")}</SelectItem>
              <SelectItem value="100">100 / {t("adminRequests.pageSizeShort")}</SelectItem>
            </SelectContent>
          </Select>
      </div>

      {/* Action Dialog */}
      <Dialog open={actionOpen} onOpenChange={setActionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("adminRequests.dialogTitle")}</DialogTitle>
            <DialogDescription>
              {t("adminRequests.dialogDescription", { id: selectedRequest?.id, mediaId: selectedRequest?.media_id })}
              <br />
              {selectedRequest?.media_info?.title}
              {selectedRequest?.media_info?.season && ` - ${t("media.season", { season: selectedRequest.media_info.season })}`}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("adminRequests.changeStatus")}</Label>
              <Select value={selectedStatus} onValueChange={setSelectedStatus}>
                <SelectTrigger>
                  <SelectValue placeholder={t("adminRequests.selectStatus")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pending">{t("media.statusUnhandled")}</SelectItem>
                  <SelectItem value="accepted">{t("media.statusAccepted")}</SelectItem>
                  <SelectItem value="downloading">{t("media.statusDownloading")}</SelectItem>
                  <SelectItem value="rejected">{t("media.statusRejected")}</SelectItem>
                  <SelectItem value="completed">{t("media.statusCompleted")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t("adminReview.adminNoteOptional")}</Label>
              <Textarea
                maxLength={1000}
                placeholder={t("adminRequests.notePlaceholder")}
                value={adminNote}
                onChange={(event) => setAdminNote(event.target.value)}
                className="min-h-24 resize-y"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setActionOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleAction}
              disabled={isActioning}
            >
              {isActioning && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t("adminRequests.confirmSave")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
