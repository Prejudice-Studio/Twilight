import { useCallback, useEffect, useRef, useState } from "react";

interface UseAsyncResourceOptions<T> {
  immediate?: boolean;
  initialData?: T;
}

function isAbortError(err: unknown): boolean {
  if (err instanceof DOMException && err.name === "AbortError") {
    return true;
  }
  if (err instanceof Error && err.name === "AbortError") {
    return true;
  }
  return false;
}

export function useAsyncResource<T>(
  loader: (signal?: AbortSignal) => Promise<T>,
  options: UseAsyncResourceOptions<T> = {}
) {
  const { immediate = true, initialData } = options;
  const [data, setData] = useState<T | undefined>(initialData);
  const [isLoading, setIsLoading] = useState<boolean>(immediate);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const abortControllerRef = useRef<AbortController | null>(null);
  const requestIdRef = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      abortControllerRef.current?.abort();
    };
  }, []);

  const execute = useCallback(async () => {
    abortControllerRef.current?.abort();
    const controller = new AbortController();
    abortControllerRef.current = controller;
    const currentRequestId = ++requestIdRef.current;

    setIsLoading(true);
    setError(null);

    try {
      const result = await loader(controller.signal);
      if (mountedRef.current && currentRequestId === requestIdRef.current) {
        setData(result);
      }
      return result;
    } catch (err: unknown) {
      if (isAbortError(err)) {
        return undefined;
      }
      const message = err instanceof Error ? err.message : "请求失败";
      if (mountedRef.current && currentRequestId === requestIdRef.current) {
        setError(message);
      }
      throw err;
    } finally {
      if (mountedRef.current && currentRequestId === requestIdRef.current) {
        setIsLoading(false);
      }
    }
  }, [loader]);

  useEffect(() => {
    if (!immediate) {
      setIsLoading(false);
      return;
    }
    void execute();
  }, [execute, immediate]);

  const setPartialData = useCallback((updater: (prev: T | undefined) => T) => {
    setData((prev) => updater(prev));
  }, []);

  return {
    data,
    isLoading,
    error,
    execute,
    setData,
    setPartialData,
  };
}
