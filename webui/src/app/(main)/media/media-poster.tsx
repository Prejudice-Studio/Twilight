"use client";

import { useEffect, useState } from "react";
import { Film, Tv } from "lucide-react";
import { cn } from "@/lib/utils";

interface MediaPosterProps {
  src?: string;
  alt: string;
  mediaType: string;
  eager?: boolean;
  imageClassName?: string;
  fallbackClassName?: string;
  iconClassName?: string;
}

export function MediaPoster({
  src,
  alt,
  mediaType,
  eager = false,
  imageClassName,
  fallbackClassName,
  iconClassName,
}: MediaPosterProps) {
  const [failed, setFailed] = useState(!src);

  useEffect(() => {
    setFailed(!src);
  }, [src]);

  if (!src || failed) {
    const Icon = mediaType === "movie" ? Film : Tv;
    return (
      <div
        data-media-poster-fallback
        className={cn("flex aspect-[2/3] w-full items-center justify-center bg-muted", fallbackClassName)}
      >
        <Icon className={cn("h-12 w-12 text-muted-foreground/30", iconClassName)} />
      </div>
    );
  }

  return (
    // Native dimensions remain authoritative; failed remote images fall back without collapsing the layout.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      data-media-poster
      src={src}
      alt={alt}
      loading={eager ? "eager" : "lazy"}
      decoding="async"
      draggable={false}
      onError={() => setFailed(true)}
      className={cn("block h-auto w-full max-w-full", imageClassName)}
    />
  );
}
