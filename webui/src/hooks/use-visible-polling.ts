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

    function schedule() {
      if (cancelled) return;
      timer = setTimeout(() => void run(), intervalMs);
    }

    async function run() {
      if (cancelled || running) return;
      if (document.visibilityState !== "visible") {
        schedule();
        return;
      }
      running = true;
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
      void run();
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
