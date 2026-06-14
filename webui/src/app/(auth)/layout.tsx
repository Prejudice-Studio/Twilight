"use client";

import { useEffect, useMemo } from "react";
import { useSystemStore } from "@/store/system";
import { sanitizeImageUrl } from "@/lib/safe-url";
import { API_BASE } from "@/lib/api-request";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { info: systemInfo, fetchInfo } = useSystemStore();
  const bgUrl = systemInfo?.auth_background_url;
  const safeBg = useMemo(() => {
    if (!bgUrl) return undefined;
    if (bgUrl.startsWith("http")) return sanitizeImageUrl(bgUrl);
    if (bgUrl.startsWith("/")) return sanitizeImageUrl(`${API_BASE}/api/v1${bgUrl}`);
    return sanitizeImageUrl(bgUrl);
  }, [bgUrl]);

  useEffect(() => {
    void fetchInfo();
  }, [fetchInfo]);

  const backgroundStyle = safeBg
    ? {
        backgroundImage: `url(${safeBg})`,
        backgroundSize: "cover",
        backgroundPosition: "center",
        backgroundRepeat: "no-repeat",
      }
    : undefined;

  return (
    <div
      className="relative min-h-screen overflow-hidden bg-background"
      style={backgroundStyle}
    >
      {!safeBg && (
        <>
          <div className="shell-glow shell-glow-left" />
          <div className="shell-glow shell-glow-right" />
          <div className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(circle_at_20%_10%,hsl(var(--primary)/0.12),transparent_35%),radial-gradient(circle_at_80%_90%,hsl(var(--primary)/0.08),transparent_30%)]" />
        </>
      )}
      {safeBg && (
        <div className="pointer-events-none absolute inset-0 -z-10 bg-background/50 backdrop-blur-[2px]" />
      )}
      <div className="relative z-10 min-h-screen">
        {children}
      </div>
    </div>
  );
}
