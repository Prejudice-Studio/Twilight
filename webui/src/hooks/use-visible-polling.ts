"use client";

import { useEffect, useRef } from "react";

export function useVisiblePolling(
  callback: (signal?: AbortSignal) => void | Promise<void>,
  intervalMs: number,
  enabled = true,
) {
  const callbackRef = useRef(callback);
  callbackRef.current = callback;

  useEffect(() => {
    if (!enabled || intervalMs <= 0) return;

    let cancelled = false;
    let running = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let controller: AbortController | null = null;
    let lastRunAt = 0;

    function clearTimer() {
      if (!timer) return;
      clearTimeout(timer);
      timer = null;
    }

    function schedule(delay = intervalMs) {
      if (cancelled) return;
      clearTimer();
      timer = setTimeout(() => void run(), Math.max(0, delay));
    }

    async function run() {
      if (cancelled || running) return;
      clearTimer();
      if (document.visibilityState !== "visible") {
        return;
      }
      running = true;
      lastRunAt = Date.now();
      controller = new AbortController();
      try {
        await callbackRef.current(controller.signal);
      } catch {
        // Polling callers keep their own last-known data and error UI.
      } finally {
        controller = null;
        running = false;
        if (document.visibilityState === "visible") {
          schedule();
        }
      }
    }

    const handleVisibility = () => {
      if (document.visibilityState !== "visible") {
        clearTimer();
        controller?.abort();
        return;
      }
      if (running) return;
      clearTimer();
      const elapsed = Date.now() - lastRunAt;
      if (elapsed >= intervalMs) {
        void run();
      } else {
        schedule(intervalMs - elapsed);
      }
    };

    document.addEventListener("visibilitychange", handleVisibility);
    if (document.visibilityState === "visible") {
      schedule();
    }
    return () => {
      cancelled = true;
      controller?.abort();
      clearTimer();
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [enabled, intervalMs]);
}
