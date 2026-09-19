"use client";

import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/i18n";
import type { PlayRankGroupBy, PlayRankMediaItem } from "@/lib/api";

// 媒体榜一行的标题区。两个页面（用户侧 / 管理侧）共用，避免"哪边改了另一边没改"。
//
// 单集模式的核心诉求是"看出这是哪部剧的哪一集"：剧名放主行，S1E8 做成徽标紧跟
// 其后，单集的标题降为副行。只有剧名没有集号（比如 Emby 没返回编号、或媒体在库里
// 已经删掉）时徽标自动消失，不会渲染出一个空的角标。
//
// 按剧聚合模式下这一行代表整部剧，"第几集"没有意义，副行改显示覆盖了多少集。
export function PlayRankMediaLabel({
  item,
  groupBy,
  showViewers = false,
}: {
  item: PlayRankMediaItem;
  groupBy: PlayRankGroupBy;
  showViewers?: boolean;
}) {
  const { t } = useI18n();

  if (groupBy === "series") {
    const episodes = item.episodes ?? 0;
    return (
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium" title={item.title}>
          {item.title || t("playRank.unknown")}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {episodes > 0 ? `${t("playRank.episodeCount", { count: episodes })} · ` : ""}
          {showViewers ? `${t("playRank.viewers")} ${item.viewers}` : (item.media_type || t("playRank.unknown"))}
        </p>
      </div>
    );
  }

  const series = item.series_name?.trim() || "";
  const primary = series || item.title || t("playRank.unknown");
  // 有剧名时 item.title 是这一集自己的名字，正好作为副行；没有剧名说明是电影，
  // 副行退回显示媒体类型。
  const secondary = series ? item.title || "" : item.media_type || t("playRank.unknown");

  return (
    <div className="min-w-0 flex-1">
      <div className="flex min-w-0 items-center gap-1.5">
        <p className="min-w-0 truncate text-sm font-medium" title={primary}>
          {primary}
        </p>
        {item.episode_label ? (
          <Badge variant="secondary" className="shrink-0 px-1.5 py-0 font-mono text-[10px]">
            {item.episode_label}
          </Badge>
        ) : null}
      </div>
      <p className="truncate text-xs text-muted-foreground">
        {secondary ? `${secondary}${showViewers ? " · " : ""}` : ""}
        {showViewers ? `${t("playRank.viewers")} ${item.viewers}` : ""}
      </p>
    </div>
  );
}
