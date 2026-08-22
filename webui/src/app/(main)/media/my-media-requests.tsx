"use client";

import { Copy, ExternalLink, Fingerprint, ListTodo, Loader2, RefreshCw, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button, IconButton } from "@/components/ui/button";
import type { MediaRequest } from "@/lib/api";
import { mediaRequestExternalUrl } from "@/lib/media-external-url";
import { sanitizeImageUrl } from "@/lib/safe-url";
import { formatRelativeTime } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { MediaPoster } from "./media-poster";

interface MyMediaRequestsProps {
  requests: MediaRequest[];
  loading: boolean;
  error: string | null;
  deletingKey: string;
  onRefresh: () => void;
  onDelete: (request: MediaRequest) => void;
  onCopy: (key: string) => void;
  onSearch: () => void;
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useI18n();
  switch (status) {
    case "UNHANDLED": return <Badge variant="secondary">{t("media.statusUnhandled")}</Badge>;
    case "ACCEPTED": return <Badge variant="info">{t("media.statusAccepted")}</Badge>;
    case "DOWNLOADING": return <Badge variant="warning">{t("media.statusDownloading")}</Badge>;
    case "REJECTED": return <Badge variant="destructive">{t("media.statusRejected")}</Badge>;
    case "COMPLETED": return <Badge variant="success">{t("media.statusCompleted")}</Badge>;
    default: return <Badge variant="secondary">{status}</Badge>;
  }
}

export function MyMediaRequests(props: MyMediaRequestsProps) {
  const { t } = useI18n();
  return (
    <section className="space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b pb-4">
        <div><h2 className="text-xl font-semibold">{t("media.requestsTitle")}</h2><p className="text-sm text-muted-foreground">{t("media.requestsDescription")}</p></div>
        <Button variant="outline" size="sm" onClick={props.onRefresh} disabled={props.loading}><RefreshCw className={props.loading ? "mr-2 h-4 w-4 animate-spin" : "mr-2 h-4 w-4"} />{t("common.refresh")}</Button>
      </header>

      {props.error && <div className="border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{props.error}</div>}
      {props.loading && props.requests.length === 0 ? <div className="flex min-h-52 items-center justify-center"><Loader2 className="h-6 w-6 animate-spin text-primary" /></div> : props.requests.length === 0 ? (
        <div className="flex min-h-52 flex-col items-center justify-center border-y text-center"><ListTodo className="mb-3 h-8 w-8 text-muted-foreground/40" /><p className="font-medium">{t("media.requestsEmpty")}</p><Button variant="outline" size="sm" className="mt-4" onClick={props.onSearch}>{t("media.goSearch")}</Button></div>
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {props.requests.map((request) => {
            const poster = sanitizeImageUrl(request.media_info?.poster_url || request.media_info?.poster);
            const title = request.media_info?.title || request.title || t("media.unknownMedia");
            const url = mediaRequestExternalUrl(request);
            return (
              <article key={request.require_key || `${request.source}-${request.id}`} className="flex min-w-0 flex-col gap-3 border-b py-4 sm:flex-row sm:items-start">
                <div className="w-20 shrink-0 overflow-hidden rounded-md bg-muted sm:w-24">
                  <MediaPoster src={poster} alt={title} mediaType={request.media_type} iconClassName="h-6 w-6" />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0">
                      {url ? <a href={url} target="_blank" rel="noopener noreferrer" className="inline-flex min-w-0 items-center gap-1 font-semibold hover:text-primary"><span className="break-words">{title}</span><ExternalLink className="h-3.5 w-3.5 shrink-0" /></a> : <h3 className="break-words font-semibold">{title}</h3>}
                      <p className="mt-1 text-xs text-muted-foreground">{request.source.toUpperCase()}#{String(request.media_id)} · {formatRelativeTime(request.timestamp * 1000)}</p>
                    </div>
                    <StatusBadge status={request.status} />
                  </div>
                  {request.season && <p className="mt-2 text-xs text-muted-foreground">{t("media.season", { season: request.season })}</p>}
                  {request.admin_note && <p className="mt-2 break-words border-l-2 border-info/40 pl-3 text-sm text-muted-foreground">{t("media.adminReply", { note: request.admin_note })}</p>}
                  <div className="mt-3 flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                    <Fingerprint className="h-3.5 w-3.5 shrink-0" /><code className="min-w-0 truncate">{request.require_key}</code>
                    <IconButton variant="ghost" className="h-8 w-8 shrink-0" aria-label={t("media.copyKey")} onClick={() => props.onCopy(request.require_key)}><Copy className="h-3.5 w-3.5" /></IconButton>
                    <IconButton variant="ghost" className="ml-auto h-8 w-8 shrink-0 text-destructive" aria-label={t("common.delete")} disabled={props.deletingKey === request.require_key} onClick={() => props.onDelete(request)}>{props.deletingKey === request.require_key ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}</IconButton>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
