"use client";

import { useEffect, useState } from "react";
import { Film, Tv } from "lucide-react";
import { cn } from "@/lib/utils";

interface MediaPosterProps {
  src?: string;
  alt: string;
  mediaType: string;
  eager?: boolean;
  frameClassName?: string;
  imageClassName?: string;
  fallbackClassName?: string;
  iconClassName?: string;
  onLoad?: (dimensions: { width: number; height: number }) => void;
}

export function MediaPoster({
  src,
  alt,
  mediaType,
  eager = false,
  frameClassName,
  imageClassName,
  fallbackClassName,
  iconClassName,
  onLoad,
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
        className={cn(
          frameClassName
            ? "flex h-full w-full items-center justify-center bg-muted"
            : "flex aspect-[2/3] w-full items-center justify-center bg-muted",
          frameClassName,
          fallbackClassName,
        )}
      >
        <Icon className={cn("h-12 w-12 text-muted-foreground/30", iconClassName)} />
      </div>
    );
  }

  const image = (
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
      onLoad={(event) => {
        setFailed(false);
        onLoad?.({ width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight });
      }}
      className={cn(
        frameClassName ? "block h-full w-full object-contain" : "block h-auto w-full max-w-full",
        imageClassName,
      )}
    />
  );

  return frameClassName ? <div className={cn("relative h-full w-full", frameClassName)}>{image}</div> : image;
}
