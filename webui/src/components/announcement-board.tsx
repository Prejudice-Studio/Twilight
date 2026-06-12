"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronUp, Clock3, Pin, Megaphone, Info, AlertTriangle, AlertOctagon } from "lucide-react";
import { api, type Announcement } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SafeAnnouncementContent } from "@/lib/safe-render";
import { useI18n, type MessageKey } from "@/lib/i18n";

const LEVEL_STYLES: Record<Announcement["level"], { icon: typeof Info; cardClass: string; iconClass: string; labelKey: MessageKey }> = {
  info: {
    icon: Info,
    cardClass: "border-blue-500/30 bg-blue-500/[0.04]",
    iconClass: "text-blue-500",
    labelKey: "announcements.levelInfo",
  },
  notice: {
    icon: Megaphone,
    cardClass: "border-emerald-500/30 bg-emerald-500/[0.04]",
    iconClass: "text-emerald-500",
    labelKey: "announcements.levelNotice",
  },
  warning: {
    icon: AlertTriangle,
    cardClass: "border-amber-500/35 bg-amber-500/[0.05]",
    iconClass: "text-amber-500",
    labelKey: "announcements.levelWarning",
  },
  critical: {
    icon: AlertOctagon,
    cardClass: "border-destructive/40 bg-destructive/[0.05]",
    iconClass: "text-destructive",
    labelKey: "announcements.levelCritical",
  },
};

function formatTime(unix: number): string {
  if (!unix) return "";
  return new Date(unix * 1000).toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

interface AnnouncementBoardProps {
  limit?: number;
  /** 折叠/展开历史的入口；默认显示前 2 条，其余折叠 */
  collapseAfter?: number;
  /** 自定义标题；传入 null 隐藏标题 */
  title?: string | null;
  /** 列表为空时是否显示占位（独立页面建议 true，dashboard 嵌入建议 false） */
  showEmptyState?: boolean;
  /**
   * 同时展示「置顶公告」和「最新公告」两组，避免置顶把最新挤下去看不到。
   * 仪表盘开启；独立公告页保持时间线视图（false）。
   */
  splitPinned?: boolean;
}

function AnnouncementCard({ ann, maxContentHeight = 300 }: { ann: Announcement; maxContentHeight?: number }) {
  const { t } = useI18n();
  const style = LEVEL_STYLES[ann.level] || LEVEL_STYLES.info;
  const Icon = style.icon;
  const [contentExpanded, setContentExpanded] = useState(false);
  const [needsCollapse, setNeedsCollapse] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (contentRef.current) {
      setNeedsCollapse(contentRef.current.scrollHeight > maxContentHeight + 8);
    }
  }, [ann.content, maxContentHeight]);

  return (
    <article className={`rounded-xl border p-4 ${style.cardClass}`}>
      <header className="flex items-start gap-3">
        <div className={`mt-0.5 shrink-0 ${style.iconClass}`}>
          <Icon className="h-4 w-4" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {ann.pinned && (
              <Pin className="h-3.5 w-3.5 text-primary shrink-0" />
            )}
            {ann.title && (
              <h3 className="text-sm font-bold leading-snug">{ann.title}</h3>
            )}
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              {t(style.labelKey)}
            </Badge>
          </div>
          <p className="text-xs text-muted-foreground mt-1">
            {formatTime(ann.created_at)}
            {ann.updated_at && ann.updated_at !== ann.created_at && (
              <> · {t("announcements.updatedAt", { time: formatTime(ann.updated_at) })}</>
            )}
            {ann.expires_at > 0 && (
              <> · {t("announcements.expiresAt", { time: formatTime(ann.expires_at) })}</>
            )}
          </p>
        </div>
      </header>
      <div
        ref={contentRef}
        className="mt-3 text-sm leading-relaxed break-words"
        style={needsCollapse && !contentExpanded ? { maxHeight: maxContentHeight, overflow: "hidden" } : undefined}
      >
        <SafeAnnouncementContent content={ann.content} mode={ann.render_mode} />
      </div>
      {needsCollapse && (
        <Button
          variant="ghost"
          size="sm"
          className="mt-1 h-7 text-xs text-muted-foreground"
          onClick={() => setContentExpanded(!contentExpanded)}
        >
          {contentExpanded ? t("announcements.collapse") : t("announcements.expand")}
        </Button>
      )}
    </article>
  );
}

interface SectionProps {
  items: Announcement[];
  heading: { icon: typeof Pin; label: string; tone: "primary" | "muted" };
  collapseAfter: number;
}

function AnnouncementSection({ items, heading, collapseAfter }: SectionProps) {
  const { t } = useI18n();
  const [expanded, setExpanded] = useState(false);
  if (items.length === 0) return null;
  const visible = expanded ? items : items.slice(0, collapseAfter);
  const hasMore = items.length > collapseAfter;
  const HeadingIcon = heading.icon;
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs font-semibold tracking-wider uppercase">
        <HeadingIcon
          className={`h-3.5 w-3.5 ${heading.tone === "primary" ? "text-primary" : "text-muted-foreground"}`}
        />
        <span className={heading.tone === "primary" ? "text-primary" : "text-muted-foreground"}>
          {heading.label}
        </span>
        <Badge variant="secondary" className="text-[10px] font-bold">
          {items.length}
        </Badge>
      </div>
      <div className="space-y-2">
        {visible.map((ann) => (
          <AnnouncementCard key={ann.id} ann={ann} />
        ))}
      </div>
      {hasMore && (
        <Button
          variant="ghost"
          size="sm"
          className="w-full text-xs gap-1.5"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? (
            <>
              <ChevronUp className="h-3.5 w-3.5" />
              {t("announcements.collapse")}
            </>
          ) : (
            <>
              <ChevronDown className="h-3.5 w-3.5" />
              {t("announcements.viewAllItems", { count: items.length })}
            </>
          )}
        </Button>
      )}
    </div>
  );
}

export function AnnouncementBoard({
  limit = 50,
  collapseAfter = 2,
  title,
  showEmptyState = false,
  splitPinned = false,
}: AnnouncementBoardProps) {
  const [items, setItems] = useState<Announcement[]>([]);
  const { t } = useI18n();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.getActiveAnnouncements(limit);
      if (res.success && Array.isArray(res.data?.announcements)) {
        setItems(res.data!.announcements);
      } else {
        setError(res.message || t("announcements.loadFailed"));
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : t("announcements.loadFailedShort"));
    } finally {
      setLoading(false);
    }
  }, [limit, t]);

  const headingTitle = title === undefined ? t("announcements.boardTitle") : title;

  useEffect(() => {
    void load();
  }, [load]);

  // 按置顶 / 最新切两组；用 useMemo 避免每次重渲染重新切片
  const { pinned, latest } = useMemo(() => {
    const pinned = items.filter((it) => it.pinned);
    const latest = items.filter((it) => !it.pinned);
    return { pinned, latest };
  }, [items]);

  if (loading && !showEmptyState) return null;
  if (error && !showEmptyState) return null;
  if (items.length === 0 && !showEmptyState) return null;

  // Empty / loading / error states for standalone page mode
  if (showEmptyState && (loading || error || items.length === 0)) {
    return (
      <section className="space-y-3">
        {headingTitle !== null && (
          <h2 className="text-sm font-bold tracking-wider uppercase text-muted-foreground flex items-center gap-2">
            <Megaphone className="h-4 w-4" />
            {headingTitle}
          </h2>
        )}
        <div className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          {loading
            ? t("announcements.loading")
            : error
              ? t("announcements.loadFailedWithMessage", { message: error })
              : t("announcements.empty")}
        </div>
      </section>
    );
  }

  // 分组模式：两个独立子区，各自折叠/展开
  if (splitPinned) {
    return (
      <section className="space-y-4">
        {headingTitle !== null && (
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-bold tracking-wider uppercase text-muted-foreground flex items-center gap-2">
              <Megaphone className="h-4 w-4" />
              {headingTitle}
              <Badge variant="secondary" className="text-[10px] font-bold">
                {items.length}
              </Badge>
            </h2>
          </div>
        )}
        <AnnouncementSection
          items={pinned}
          heading={{ icon: Pin, label: t("announcements.pinned"), tone: "primary" }}
          collapseAfter={collapseAfter}
        />
        <AnnouncementSection
          items={latest}
          heading={{ icon: Clock3, label: t("announcements.latest"), tone: "muted" }}
          collapseAfter={collapseAfter}
        />
      </section>
    );
  }

  // 经典时间线视图：置顶在前，按时间倒序
  const visible = expanded ? items : items.slice(0, collapseAfter);
  const hasMore = items.length > collapseAfter;
  return (
    <section className="space-y-3">
      {headingTitle !== null && (
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-bold tracking-wider uppercase text-muted-foreground flex items-center gap-2">
            <Megaphone className="h-4 w-4" />
            {headingTitle}
            <Badge variant="secondary" className="text-[10px] font-bold">
              {items.length}
            </Badge>
          </h2>
        </div>
      )}

      <div className="space-y-2">
        {visible.map((ann) => (
          <AnnouncementCard key={ann.id} ann={ann} />
        ))}
      </div>

      {hasMore && (
        <Button
          variant="ghost"
          size="sm"
          className="w-full text-xs gap-1.5"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? (
            <>
              <ChevronUp className="h-3.5 w-3.5" />
              {t("announcements.collapseHistory")}
            </>
          ) : (
            <>
              <ChevronDown className="h-3.5 w-3.5" />
              {t("announcements.viewAllAnnouncements", { count: items.length })}
            </>
          )}
        </Button>
      )}
    </section>
  );
}
