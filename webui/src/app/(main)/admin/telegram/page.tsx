"use client";

import Link from "next/link";
import {
  AlertTriangle,
  BookOpen,
  CheckCircle2,
  Code2,
  Loader2,
  MessageSquare,
  Send,
  Users,
  WifiOff,
  XCircle,
} from "lucide-react";
import { AdminConfigSections } from "@/components/admin/config-section-editor";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useState } from "react";

type BotTestResult = {
  target: string;
  success: boolean;
  error: string | null;
  username?: string;
  bot_id?: number;
  title?: string;
  bot_status?: string;
};

type BotRuntime = {
  polling?: boolean;
  last_ok_at?: number | null;
  last_error_at?: number | null;
  last_error?: string;
};

const TELEGRAM_BASE_FIELDS = [
  "enable_telegram",
  "force_bind_telegram",
  "telegram_bot_token",
  "telegram_api_url",
  "admin_id",
  "group_id",
  "channel_id",
  "telegram_force_subscribe",
  "telegram_force_bind_group",
  "telegram_force_bind_channel",
  "telegram_require_membership",
  "telegram_group_check_concurrency",
  "telegram_group_action_concurrency",
  "telegram_ban_on_leave",
  "telegram_auto_enable_rejoined",
  "telegram_enable_panel",
  "bot_start_text",
  "bot_group_start_text",
  "bot_start_title",
  "bot_start_intro",
  "bot_bind_prompt_text",
  "bot_help_text",
  "bot_admin_help_text",
  "bot_help_header",
  "bot_help_footer",
  "bot_about",
] as const;

export default function AdminTelegramPage() {
  const { toast } = useToast();
  const { t } = useI18n();
  const [testing, setTesting] = useState(false);
  const [botResults, setBotResults] = useState<BotTestResult[] | null>(null);
  const [botRuntime, setBotRuntime] = useState<BotRuntime | null>(null);

  const testBot = async () => {
    setTesting(true);
    setBotResults(null);
    setBotRuntime(null);
    try {
      const res = await api.testBotConnectivity();
      if (res.success && res.data) {
        setBotResults(res.data.results);
        setBotRuntime(res.data.runtime || null);
      }
      toast({
        title: res.success ? t("adminTelegram.testSuccess") : t("adminTelegram.testFailed"),
        description: res.message,
        variant: res.success ? "success" : "destructive",
      });
    } catch (err) {
      toast({ title: t("adminTelegram.testFailed"), description: err instanceof Error ? err.message : undefined, variant: "destructive" });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("adminTelegram.title")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("adminTelegram.description")}</p>
        </div>
      </div>

      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <h2 className="flex items-center gap-2 text-base font-semibold">
                <Send className="h-4 w-4" />
                {t("adminTelegram.botTestTitle")}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">{t("adminTelegram.botTestDescription")}</p>
            </div>
            <Button variant="outline" onClick={() => void testBot()} disabled={testing} className="min-h-10 whitespace-normal leading-tight">
              {testing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
              {t("adminTelegram.testBot")}
            </Button>
          </div>

          {botRuntime && (
            <div className="grid gap-2 rounded-lg border bg-muted/30 p-3 text-xs text-muted-foreground sm:grid-cols-2">
              <div>{t("adminTelegram.pollingLabel")}{botRuntime.polling ? t("adminTelegram.pollingRunning") : t("adminTelegram.pollingStopped")}</div>
              {botRuntime.last_ok_at ? <div>{t("adminTelegram.lastOk")}{new Date(botRuntime.last_ok_at * 1000).toLocaleString()}</div> : null}
              {botRuntime.last_error_at ? <div>{t("adminTelegram.lastErrorAt")}{new Date(botRuntime.last_error_at * 1000).toLocaleString()}</div> : null}
              {botRuntime.last_error ? <div className="break-words text-destructive sm:col-span-2">{t("adminTelegram.lastError")}{botRuntime.last_error}</div> : null}
            </div>
          )}

          {botResults && (
            <div className="space-y-3">
              <div className="grid gap-2 sm:grid-cols-2">
                {botResults.map((result, index) => (
                  <div
                    key={`${result.target}:${index}`}
                    className={`rounded-lg border p-3 ${
                      result.success
                        ? "border-emerald-500/20 bg-emerald-500/5"
                        : "border-destructive/20 bg-destructive/5"
                    }`}
                  >
                    <div className="mb-1 flex items-center gap-2">
                      {result.success ? (
                        <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-destructive" />
                      )}
                      <span className="font-mono text-sm font-medium">{result.target}</span>
                    </div>
                    {result.error ? (
                      <p className="mt-1 text-xs text-destructive">{result.error}</p>
                    ) : (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {[result.username ? `@${result.username}` : "", result.title || "", result.bot_status ? `${t("adminTelegram.botStatusPrefix")}${result.bot_status}` : ""].filter(Boolean).join(" · ")}
                      </p>
                    )}
                    {result.success && <p className="mt-1 text-xs text-muted-foreground">{t("adminTelegram.sendSuccess")}</p>}
                  </div>
                ))}
              </div>
              <div className="flex items-center gap-2">
                {botResults.every((result) => result.success) ? (
                  <>
                    <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                    <span className="text-sm font-medium text-emerald-500">{t("adminTelegram.allTargetsOk")}</span>
                  </>
                ) : (
                  <>
                    <WifiOff className="h-5 w-5 text-destructive" />
                    <span className="text-sm font-medium text-destructive">{t("adminTelegram.partialTargetsFailed")}</span>
                  </>
                )}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-3 md:grid-cols-2">
        <Link href="/admin/telegram-rebind-requests">
          <Card className="h-full transition-colors hover:border-primary/50 hover:bg-accent/30">
            <CardContent className="flex min-h-[104px] gap-3 p-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <MessageSquare className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="font-medium leading-tight">{t("adminTelegram.rebindTitle")}</p>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t("adminTelegram.rebindDescription")}</p>
              </div>
            </CardContent>
          </Card>
        </Link>
        <Link href="/admin/scheduler">
          <Card className="h-full transition-colors hover:border-primary/50 hover:bg-accent/30">
            <CardContent className="flex min-h-[104px] gap-3 p-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Users className="h-5 w-5" />
              </div>
              <div className="min-w-0">
                <p className="font-medium leading-tight">{t("adminTelegram.rosterTitle")}</p>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t("adminTelegram.rosterDescription")}</p>
              </div>
            </CardContent>
          </Card>
        </Link>
      </div>

      <Alert className="border-amber-500/40 bg-amber-500/10">
        <AlertTriangle className="h-4 w-4" />
        <AlertTitle>{t("adminTelegram.customCommandNoticeTitle")}</AlertTitle>
        <AlertDescription>{t("adminTelegram.customCommandNotice")}</AlertDescription>
      </Alert>

      <Card>
        <CardContent className="grid gap-3 p-4 md:grid-cols-3">
          <div className="md:col-span-2">
            <h2 className="flex items-center gap-2 text-base font-semibold">
              <Code2 className="h-4 w-4" />
              {t("adminTelegram.customCommandsTitle")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">{t("adminTelegram.customCommandsDescription")}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              {["ctx", "args", "user", "reply(text)", "log(text)", "js:"].map((item) => (
                <Badge key={item} variant="outline">{item}</Badge>
              ))}
            </div>
          </div>
          <div className="flex flex-col gap-2 md:justify-self-end">
            <Button asChild variant="default" className="min-h-10 whitespace-normal leading-tight">
              <Link href="/admin/telegram/commands">
                <Code2 className="mr-2 h-4 w-4" />
                {t("adminTelegram.openCommandManager")}
              </Link>
            </Button>
            <Button asChild variant="outline" className="min-h-10 whitespace-normal leading-tight">
              <Link href="/admin/developer">
                <BookOpen className="mr-2 h-4 w-4" />
                {t("adminTelegram.openDeveloperDocs")}
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      <AdminConfigSections
        sectionKeys={["Telegram"]}
        sectionFieldKeys={{ Telegram: [...TELEGRAM_BASE_FIELDS] }}
        title={t("adminTelegram.configTitle")}
        description={t("adminTelegram.configDescription")}
        notice={t("adminTelegram.configNotice")}
      />
    </div>
  );
}
