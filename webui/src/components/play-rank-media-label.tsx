"use client";

import { Badge } from "@/components/ui/badge";
import { useI18n } from "@/lib/i18n";
import type { PlayRankGroupBy, PlayRankMediaItem } from "@/lib/api";

// 媒体榜一行的标题区。两个页面（用户侧 / 管理侧）共用，避免"哪边改了另一边没改"。
//
// 单集模式的核心诉求是"看出这是哪部剧的哪一集"：剧名放主行，集数做成徽标紧跟其后，
// 单集的标题降为副行。
//
// 集数怎么显示由这里决定，后端只给 season_number / episode_number 两个数字，不替
// 前端拼任何文案——拼出来的字符串没法翻译，也没法让不同语言按自己的习惯表达。
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

  // 两个编号都缺（电影、音乐，或 Emby 没返回）时整段不渲染，不会冒出空徽标。
  const episode = item.episode_number ?? 0;
  const season = item.season_number ?? 0;
  let badge: string | null = null;
  if (episode > 0) {
    // 只有集号说明媒体库没有季层级（或元数据没刮到季号），用本地化的"第 8 集"。
    // 不要退化成 "E8"：那是拿一个自造的缩写掩盖"其实不知道第几季"，中文界面里
    // 尤其别扭。
    badge = season > 0
      ? t("playRank.seasonEpisode", { season, episode })
      : t("playRank.episodeOnly", { n: episode });
  }

  return (
    <div className="min-w-0 flex-1">
      <div className="flex min-w-0 items-center gap-1.5">
        <p className="min-w-0 truncate text-sm font-medium" title={primary}>
          {primary}
        </p>
        {badge ? (
          <Badge variant="secondary" className="shrink-0 px-1.5 py-0 font-mono text-[10px]">
            {badge}
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
