"use client";

import React from "react";

type Mode = "plain" | "markdown" | "bbcode";

const SAFE_URL_PROTOCOLS = /^(https?:|mailto:)/i;
const SAFE_IMAGE_PROTOCOLS = /^https?:/i;

function isSafeUrl(raw: string): boolean {
  const value = raw.trim();
  if (!value || /[\x00-\x1f\x7f]/.test(value) || value.startsWith("//")) return false;
  return value.startsWith("/") || value.startsWith("#") || SAFE_URL_PROTOCOLS.test(value);
}

function isSafeImageUrl(raw: string): boolean {
  const value = raw.trim();
  if (!value || /[\x00-\x1f\x7f]/.test(value) || value.startsWith("//")) return false;
  return value.startsWith("/") || SAFE_IMAGE_PROTOCOLS.test(value);
}

function splitWithBreaks(text: string, key: string): React.ReactNode[] {
  const parts = text.split("\n");
  return parts.flatMap((part, index) => {
    if (index === parts.length - 1) return part ? [part] : [];
    return part ? [part, <br key={`${key}-br-${index}`} />] : [<br key={`${key}-br-${index}`} />];
  });
}

function renderInline(text: string, keyPrefix: string): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  let i = 0;
  let buffer = "";

  const pushText = () => {
    if (!buffer) return;
    out.push(...splitWithBreaks(buffer, `${keyPrefix}-${out.length}`));
    buffer = "";
  };

  const readUntil = (marker: string, from: number) => {
    const end = text.indexOf(marker, from);
    return end > from ? end : -1;
  };

  while (i < text.length) {
    if (text[i] === "\\" && i + 1 < text.length) {
      buffer += text[i + 1];
      i += 2;
      continue;
    }

    if (text[i] === "`") {
      const end = readUntil("`", i + 1);
      if (end > 0) {
        pushText();
        out.push(
          <code key={`${keyPrefix}-code-${i}`} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">
            {text.slice(i + 1, end)}
          </code>,
        );
        i = end + 1;
        continue;
      }
    }

    if (text.startsWith("![", i)) {
      const labelEnd = text.indexOf("]", i + 2);
      const urlStart = labelEnd + 1;
      if (labelEnd > i && text[urlStart] === "(") {
        const urlEnd = text.indexOf(")", urlStart + 1);
        if (urlEnd > urlStart) {
          const alt = text.slice(i + 2, labelEnd).slice(0, 120);
          const url = text.slice(urlStart + 1, urlEnd).trim();
          pushText();
          out.push(
            isSafeImageUrl(url) ? (
              // eslint-disable-next-line @next/next/no-img-element -- Announcement images are user-provided URLs.
              <img
                key={`${keyPrefix}-img-${i}`}
                src={url}
                alt={alt}
                loading="lazy"
                referrerPolicy="no-referrer"
                className="my-2 max-h-80 max-w-full rounded-md border border-border/60 object-contain"
              />
            ) : (
              `![${alt}](${url})`
            ),
          );
          i = urlEnd + 1;
          continue;
        }
      }
    }

    if (text[i] === "[") {
      const labelEnd = text.indexOf("]", i + 1);
      const urlStart = labelEnd + 1;
      if (labelEnd > i && text[urlStart] === "(") {
        const urlEnd = text.indexOf(")", urlStart + 1);
        if (urlEnd > urlStart) {
          const label = text.slice(i + 1, labelEnd);
          const url = text.slice(urlStart + 1, urlEnd).trim();
          pushText();
          out.push(
            isSafeUrl(url) ? (
              <a
                key={`${keyPrefix}-a-${i}`}
                href={url}
                target="_blank"
                rel="noopener noreferrer nofollow ugc"
                className="break-words text-primary underline-offset-2 hover:underline"
              >
                {renderInline(label, `${keyPrefix}-a-${i}`)}
              </a>
            ) : (
              `[${label}](${url})`
            ),
          );
          i = urlEnd + 1;
          continue;
        }
      }
    }

    const autoLink = /^https?:\/\/[^\s<>)]+/i.exec(text.slice(i));
    if (autoLink) {
      const url = autoLink[0].replace(/[.,!?;:]+$/, "");
      const trailing = autoLink[0].slice(url.length);
      if (isSafeUrl(url)) {
        pushText();
        out.push(
          <a key={`${keyPrefix}-auto-${i}`} href={url} target="_blank" rel="noopener noreferrer nofollow ugc" className="break-all text-primary underline-offset-2 hover:underline">
            {url}
          </a>,
        );
        if (trailing) out.push(trailing);
        i += autoLink[0].length;
        continue;
      }
    }

    let handledDelimited = false;
    for (const [marker, tag] of [
      ["**", "strong"],
      ["__", "strong"],
      ["~~", "s"],
      ["*", "em"],
      ["_", "em"],
    ] as const) {
      if (!text.startsWith(marker, i)) continue;
      const end = readUntil(marker, i + marker.length);
      if (end < 0) continue;
      const body = renderInline(text.slice(i + marker.length, end), `${keyPrefix}-${tag}-${i}`);
      pushText();
      if (tag === "strong") out.push(<strong key={`${keyPrefix}-strong-${i}`} className="font-semibold">{body}</strong>);
      if (tag === "em") out.push(<em key={`${keyPrefix}-em-${i}`} className="italic">{body}</em>);
      if (tag === "s") out.push(<s key={`${keyPrefix}-s-${i}`} className="line-through opacity-80">{body}</s>);
      i = end + marker.length;
      handledDelimited = true;
      break;
    }
    if (handledDelimited) continue;

    buffer += text[i];
    i += 1;
  }

  pushText();
  return out;
}

function renderMarkdown(content: string): React.ReactNode {
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  const blocks: React.ReactNode[] = [];
  let i = 0;

  const readParagraph = () => {
    const para: string[] = [];
    while (i < lines.length && lines[i].trim() !== "" && !isBlockStart(lines[i])) {
      para.push(lines[i]);
      i += 1;
    }
    return para.join("\n");
  };

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();
    if (!trimmed) {
      i += 1;
      continue;
    }

    const fence = /^```\s*([\w-]+)?\s*$/.exec(line);
    if (fence) {
      const code: string[] = [];
      i += 1;
      while (i < lines.length && !/^```\s*$/.test(lines[i])) {
        code.push(lines[i]);
        i += 1;
      }
      if (i < lines.length) i += 1;
      blocks.push(
        <pre key={`code-${blocks.length}`} className="my-2 overflow-x-auto rounded-md bg-muted p-3 text-xs leading-snug">
          <code>{code.join("\n")}</code>
        </pre>,
      );
      continue;
    }

    const heading = /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line);
    if (heading) {
      const level = Math.min(6, heading[1].length);
      const Tag = `h${level}` as keyof JSX.IntrinsicElements;
      const className = level === 1 ? "mt-3 text-xl font-bold first:mt-0" : level === 2 ? "mt-3 text-lg font-bold first:mt-0" : "mt-2 text-base font-semibold first:mt-0";
      blocks.push(<Tag key={`h-${blocks.length}`} className={className}>{renderInline(heading[2], `h-${blocks.length}`)}</Tag>);
      i += 1;
      continue;
    }

    if (/^>\s?/.test(line)) {
      const quote: string[] = [];
      while (i < lines.length && /^>\s?/.test(lines[i])) {
        quote.push(lines[i].replace(/^>\s?/, ""));
        i += 1;
      }
      blocks.push(
        <blockquote key={`q-${blocks.length}`} className="my-2 border-l-4 border-primary/50 pl-3 text-muted-foreground">
          {renderInline(quote.join("\n"), `q-${blocks.length}`)}
        </blockquote>,
      );
      continue;
    }

    if (/^[-*_]\s*([-*_]\s*){2,}$/.test(trimmed)) {
      blocks.push(<hr key={`hr-${blocks.length}`} className="my-3 border-border" />);
      i += 1;
      continue;
    }

    const unordered = /^\s*[-*+]\s+(.+)$/.exec(line);
    const ordered = /^\s*\d+\.\s+(.+)$/.exec(line);
    if (unordered || ordered) {
      const orderedList = Boolean(ordered);
      const items: string[] = [];
      while (i < lines.length) {
        const match = orderedList ? /^\s*\d+\.\s+(.+)$/.exec(lines[i]) : /^\s*[-*+]\s+(.+)$/.exec(lines[i]);
        if (!match) break;
        items.push(match[1]);
        i += 1;
      }
      const ListTag = orderedList ? "ol" : "ul";
      blocks.push(
        <ListTag key={`list-${blocks.length}`} className={`${orderedList ? "list-decimal" : "list-disc"} my-2 space-y-1 pl-5`}>
          {items.map((item, index) => (
            <li key={`li-${blocks.length}-${index}`} className="leading-relaxed">
              {renderInline(item, `li-${blocks.length}-${index}`)}
            </li>
          ))}
        </ListTag>,
      );
      continue;
    }

    const paragraph = readParagraph();
    blocks.push(
      <p key={`p-${blocks.length}`} className="my-1 break-words leading-relaxed">
        {renderInline(paragraph, `p-${blocks.length}`)}
      </p>,
    );
  }

  return <div className="space-y-1">{blocks}</div>;
}

function isBlockStart(line: string): boolean {
  return /^```/.test(line) ||
    /^(#{1,6})\s+/.test(line) ||
    /^>\s?/.test(line) ||
    /^\s*[-*+]\s+/.test(line) ||
    /^\s*\d+\.\s+/.test(line) ||
    /^[-*_]\s*([-*_]\s*){2,}$/.test(line.trim());
}

function renderBBCode(content: string): React.ReactNode {
  const translated = content
    .replace(/\[b\]([\s\S]*?)\[\/b\]/gi, "**$1**")
    .replace(/\[i\]([\s\S]*?)\[\/i\]/gi, "*$1*")
    .replace(/\[s\]([\s\S]*?)\[\/s\]/gi, "~~$1~~")
    .replace(/\[u\]([\s\S]*?)\[\/u\]/gi, "$1")
    .replace(/\[code\]([\s\S]*?)\[\/code\]/gi, "`$1`")
    .replace(/\[quote\]([\s\S]*?)\[\/quote\]/gi, "> $1")
    .replace(/\[url=([^\]]+)\]([\s\S]*?)\[\/url\]/gi, "[$2]($1)")
    .replace(/\[url\]([\s\S]*?)\[\/url\]/gi, "$1");
  return renderMarkdown(translated);
}

export function SafeAnnouncementContent({ content, mode }: { content: string; mode?: Mode | null }) {
  const selected = (mode || "plain") as Mode;
  if (selected === "markdown") {
    return <div className="text-sm">{renderMarkdown(content)}</div>;
  }
  if (selected === "bbcode") {
    return <div className="text-sm">{renderBBCode(content)}</div>;
  }
  return <div className="whitespace-pre-wrap break-words text-sm leading-relaxed">{content}</div>;
}
