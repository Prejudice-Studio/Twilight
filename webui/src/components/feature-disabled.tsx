"use client";

import { ShieldOff } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { useI18n } from "@/lib/i18n";
import { useSystemStore } from "@/store/system";

/**
 * 功能开关未开启时页面该长什么样。
 *
 * 侧边栏会按开关隐藏入口，但隐藏不等于拦住：用户仍可能从历史记录、书签或直接
 * 输入 URL 进入页面。没有这层拦截的话，页面会照常发请求、拿到 403，最后显示一
 * 个含义不明的空列表。这里统一给出"功能未开启"的结论。
 *
 * 判定用 `!== false` 而不是 `=== true`：系统信息还没加载完时 features 是
 * undefined，此时按"开启"处理，避免首屏闪一下未开启。
 */
export function useFeatureEnabled(feature: string): boolean {
  return useSystemStore((s) => s.info?.features?.[feature]) !== false;
}

export function FeatureDisabledNotice() {
  const { t } = useI18n();
  return (
    <div className="page-enter">
      <Card className="border-border/60">
        <CardContent className="flex min-h-[320px] flex-col items-center justify-center gap-3 p-8 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <ShieldOff className="h-7 w-7" />
          </div>
          <h1 className="text-xl font-semibold">{t("common.featureDisabled")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("common.featureDisabledHint")}</p>
        </CardContent>
      </Card>
    </div>
  );
}
