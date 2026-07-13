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

    function schedule(delay = intervalMs) {
      if (cancelled) return;
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => void run(), Math.max(0, delay));
    }

    async function run() {
      if (cancelled || running) return;
      if (document.visibilityState !== "visible") {
        schedule();
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
        schedule();
      }
    }

    const handleVisibility = () => {
      if (document.visibilityState !== "visible") {
        controller?.abort();
        return;
      }
      if (running) return;
      if (timer) clearTimeout(timer);
      timer = null;
      const elapsed = Date.now() - lastRunAt;
      if (elapsed >= intervalMs) {
        void run();
      } else {
        schedule(intervalMs - elapsed);
      }
    };

    schedule();
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      cancelled = true;
      controller?.abort();
      if (timer) clearTimeout(timer);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [enabled, intervalMs]);
}
