"use client";

import { useEffect, useState } from "react";

export function useCountdownState(initialValue = 0) {
  const [value, setValue] = useState(initialValue);

  useEffect(() => {
    if (value <= 0) return;
    const timer = window.setTimeout(() => {
      setValue((current) => (current > 1 ? current - 1 : 0));
    }, 1000);
    return () => window.clearTimeout(timer);
  }, [value]);

  return [value, setValue] as const;
}
