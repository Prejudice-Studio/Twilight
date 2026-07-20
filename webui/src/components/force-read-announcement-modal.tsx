"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AlertTriangle, Info, Megaphone, AlertOctagon, Clock, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { api, type Announcement } from "@/lib/api";
import { SafeAnnouncementContent } from "@/lib/safe-render";
import { useI18n } from "@/lib/i18n";

const levelIcons: Record<string, typeof Info> = {
  info: Info,
  notice: Megaphone,
  warning: AlertTriangle,
  critical: AlertOctagon,
};

const levelColors: Record<string, string> = {
  info: "text-blue-500",
  notice: "text-emerald-500",
  warning: "text-amber-500",
  critical: "text-destructive",
};

interface ForceReadAnnouncementModalProps {
  userId: number;
  onAllAcknowledged: () => void;
}

export function ForceReadAnnouncementModal({ userId, onAllAcknowledged }: ForceReadAnnouncementModalProps) {
  const { t } = useI18n();
  const [announcements, setAnnouncements] = useState<Announcement[]>([]);
  const [currentIndex, setCurrentIndex] = useState(0);
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [loading, setLoading] = useState(true);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const hasMounted = useRef(false);

  const current = announcements[currentIndex];
  const isLast = currentIndex >= announcements.length - 1;
  const requiredSeconds = current?.force_read_seconds ?? 10;
  const canClose = isLast && secondsLeft <= 0;

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const startTimer = useCallback((seconds: number) => {
    clearTimer();
    setSecondsLeft(seconds);
    if (seconds <= 0) return;
    timerRef.current = setInterval(() => {
      setSecondsLeft((prev) => {
        if (prev <= 1) {
          clearTimer();
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
  }, [clearTimer]);

  const acknowledgeAll = useCallback(async () => {
    if (announcements.length === 0) return;
    try {
      await api.ackAnnouncements(announcements.map((a) => a.id));
    } catch {
      // non-fatal
    }
    clearTimer();
    onAllAcknowledged();
  }, [announcements, clearTimer, onAllAcknowledged]);

  useEffect(() => {
    if (hasMounted.current) return;
    hasMounted.current = true;

    const controller = new AbortController();
    void (async () => {
      try {
        const res = await api.getMyAnnouncements();
        if (controller.signal.aborted) return;
        if (res.success && res.data?.unseen_force_read?.length) {
          setAnnouncements(res.data.unseen_force_read);
        }
      } catch {
        // non-fatal
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();

    return () => {
      controller.abort();
      clearTimer();
    };
  }, [clearTimer]);

  useEffect(() => {
    if (announcements.length > 0 && currentIndex < announcements.length) {
      const sec = announcements[currentIndex].force_read_seconds ?? 10;
      startTimer(Math.max(3, sec));
    }
  }, [announcements, currentIndex, startTimer]);

  if (loading || announcements.length === 0) return null;

  const Icon = levelIcons[current?.level ?? "info"] ?? Info;

  return (
    <Dialog open modal onOpenChange={() => {}}>
      <DialogContent
        className="max-w-xl sm:max-w-2xl max-h-[85vh] overflow-y-auto"
        onPointerDownOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <div className="flex items-center gap-2">
            <Icon className={`h-5 w-5 ${levelColors[current?.level ?? "info"]}`} />
            <DialogTitle className="text-lg">{current?.title || t("announcements.title")}</DialogTitle>
          </div>
          <DialogDescription className="flex flex-wrap items-center gap-2 pt-1">
            <Badge variant={secondsLeft > 0 ? "destructive" : "outline"} className="gap-1">
              <Clock className="h-3 w-3" />
              {secondsLeft > 0
                ? t("announcements.forceReadRemaining", { seconds: secondsLeft })
                : t("announcements.forceReadDone")}
            </Badge>
            {announcements.length > 1 && (
              <Badge variant="secondary">
                {currentIndex + 1} / {announcements.length}
              </Badge>
            )}
            {current?.force_read && secondsLeft > 0 && (
              <span className="text-xs text-muted-foreground">
                {t("announcements.forceReadHint")}
              </span>
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="prose prose-sm max-w-none dark:prose-invert py-2">
          {current && (
            <SafeAnnouncementContent content={current.content} mode={current.render_mode || "plain"} />
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 pt-2">
          <div className="flex gap-2">
            {announcements.length > 1 && (
              <Button
                variant="outline"
                size="sm"
                disabled={currentIndex === 0}
                onClick={() => setCurrentIndex((i) => Math.max(0, i - 1))}
              >
                {t("common.cancel")}
              </Button>
            )}
            {!isLast && (
              <Button
                variant="outline"
                size="sm"
                disabled={secondsLeft > 0}
                onClick={() => setCurrentIndex((i) => i + 1)}
              >
                {secondsLeft > 0 ? t("announcements.forceReadPleaseWait") : t("announcements.nextPage")}
              </Button>
            )}
          </div>

          {canClose && (
            <Button onClick={acknowledgeAll}>
              <Check className="mr-2 h-4 w-4" />
              {t("announcements.iHaveRead")}
            </Button>
          )}
          {!canClose && (
            <Button disabled variant="outline" className="opacity-50">
              <Clock className="mr-2 h-4 w-4 animate-pulse" />
              {t("announcements.forceReadPleaseWait")}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
