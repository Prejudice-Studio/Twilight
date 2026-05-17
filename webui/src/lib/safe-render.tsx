"use client";

/**
 * 安全的公告渲染器：支持 plain / markdown / bbcode 三种模式。
 *
 * 设计取舍
 * ========
 * - **完全不接受 raw HTML**。所有渲染都由 React 元素树负责，永远不调用
 *   ``dangerouslySetInnerHTML``，从根上规避 XSS。
 * - markdown 用一个手写的极小子集解析器（标题 / 加粗 / 斜体 / 行内代码 /
 *   代码块 / 引用 / 列表 / 链接 / 自动换行），不引入新依赖。链接 URL 强校验
 *   为 http(s)/mailto 或站内相对路径，其它（javascript:/data:/file: 等）一律
 *   降级为纯文本。
 * - bbcode 采用基于栈的解析；只白名单了 ``b/i/u/s/code/quote/url/color/size/list``，
 *   非允许标签按字面输出。URL/颜色/尺寸都做严格校验。
 *
 * 不追求与 GitHub / phpBB 等的完全兼容，目标是「足够安全、足够能看」。
 */
import React from "react";

type Mode = "plain" | "markdown" | "bbcode";

const SAFE_URL_PROTOCOLS = /^(https?:|mailto:)/i;

function isSafeUrl(raw: string): boolean {
  const value = (raw || "").trim();
  if (!value) return false;
  if (value.startsWith("/")) return true; // 站内相对路径
  if (value.startsWith("#")) return true; // 锚点
  // 阻断 javascript:, data:, vbscript:, file: 等
  return SAFE_URL_PROTOCOLS.test(value);
}

function isSafeColor(raw: string): boolean {
  // 仅允许 #RGB / #RRGGBB / 命名颜色 (字母数字)
  const v = (raw || "").trim();
  if (!v) return false;
  if (/^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$/.test(v)) return true;
  if (/^[a-zA-Z]{3,20}$/.test(v)) return true;
  return false;
}

function isSafeSize(raw: string): boolean {
  // 1 ~ 7 或 8~36px
  const v = (raw || "").trim();
  if (/^[1-7]$/.test(v)) return true;
  if (/^\d{1,2}(px)?$/.test(v)) {
    const n = parseInt(v, 10);
    return n >= 8 && n <= 36;
  }
  return false;
}

function normalizeSize(raw: string): string {
  const v = raw.trim();
  if (/^[1-7]$/.test(v)) {
    // 映射到 px：1->10, 2->12, 3->14, 4->16, 5->18, 6->22, 7->26
    const map: Record<string, string> = { "1": "10px", "2": "12px", "3": "14px", "4": "16px", "5": "18px", "6": "22px", "7": "26px" };
    return map[v];
  }
  return v.endsWith("px") ? v : `${v}px`;
}

// ============== 行内 Markdown 渲染（手写极小子集） ==============
// 支持：行内代码 `...`、加粗 **...**、斜体 *...* 或 _..._、删除线 ~~..~~、
// 链接 [label](url)、自动链接 https://...，以及把单独 `\n` 渲染为 <br/>。
function renderMarkdownInline(text: string, keyPrefix = ""): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  let i = 0;
  let buf = "";

  const pushBuf = () => {
    if (!buf) return;
    // 段内换行 → <br/>
    const segments = buf.split("\n");
    segments.forEach((seg, idx) => {
      if (seg) out.push(seg);
      if (idx < segments.length - 1) {
        out.push(<br key={`${keyPrefix}br-${i}-${idx}-${out.length}`} />);
      }
    });
    buf = "";
  };

  const tryDelim = (open: string): number => {
    // 从 i 起寻找下一处闭合 open；要求闭合处后面不是 open 的延续字符
    // （避免 *italic* 把 ** 截断）
    const len = open.length;
    let j = i + len;
    while (j < text.length) {
      const idx = text.indexOf(open, j);
      if (idx < 0) return -1;
      // 排除空内容；排除前面立刻就是反斜杠转义
      if (idx === i + len) {
        j = idx + len;
        continue;
      }
      return idx;
    }
    return -1;
  };

  while (i < text.length) {
    const ch = text[i];

    // 反斜杠转义：跳过下一个字符作为字面
    if (ch === "\\" && i + 1 < text.length) {
      buf += text[i + 1];
      i += 2;
      continue;
    }

    // 行内代码 `...`（也支持 `` `code` `` 双反引号）
    if (ch === "`") {
      const isDouble = text[i + 1] === "`";
      const close = isDouble ? "``" : "`";
      const end = text.indexOf(close, i + close.length);
      if (end > i) {
        pushBuf();
        out.push(
          <code
            key={`${keyPrefix}c-${i}`}
            className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]"
          >
            {text.slice(i + close.length, end)}
          </code>,
        );
        i = end + close.length;
        continue;
      }
    }

    // 链接 [label](url)
    if (ch === "[") {
      const labelEnd = text.indexOf("]", i + 1);
      if (labelEnd > i && text[labelEnd + 1] === "(") {
        const urlEnd = text.indexOf(")", labelEnd + 2);
        if (urlEnd > labelEnd + 1) {
          const label = text.slice(i + 1, labelEnd);
          const url = text.slice(labelEnd + 2, urlEnd).trim();
          pushBuf();
          if (isSafeUrl(url)) {
            out.push(
              <a
                key={`${keyPrefix}a-${i}`}
                href={url}
                target="_blank"
                rel="noopener noreferrer nofollow ugc"
                className="text-primary underline-offset-2 hover:underline break-words"
              >
                {renderMarkdownInline(label, `${keyPrefix}a${i}-`)}
              </a>,
            );
          } else {
            out.push(`[${label}](${url})`);
          }
          i = urlEnd + 1;
          continue;
        }
      }
    }

    // 自动链接 http(s)://
    if (ch === "h" && /^https?:\/\//i.test(text.slice(i))) {
      const match = /^https?:\/\/[^\s<>)]+/i.exec(text.slice(i));
      if (match) {
        const url = match[0];
        if (isSafeUrl(url)) {
          pushBuf();
          out.push(
            <a
              key={`${keyPrefix}al-${i}`}
              href={url}
              target="_blank"
              rel="noopener noreferrer nofollow ugc"
              className="text-primary underline-offset-2 hover:underline break-all"
            >
              {url}
            </a>,
          );
          i += url.length;
          continue;
        }
      }
    }

    // 加粗 **...** 或 __...__
    if ((ch === "*" || ch === "_") && text[i + 1] === ch) {
      const open = ch + ch;
      const end = tryDelim(open);
      if (end > i + 1) {
        pushBuf();
        out.push(
          <strong key={`${keyPrefix}b-${i}`} className="font-semibold">
            {renderMarkdownInline(text.slice(i + 2, end), `${keyPrefix}b${i}-`)}
          </strong>,
        );
        i = end + 2;
        continue;
      }
    }

    // 斜体 *...* 或 _..._
    if (ch === "*" || ch === "_") {
      // 避免和加粗冲突：要求左右紧邻的不是同一个分隔符
      if (text[i + 1] !== ch) {
        const end = text.indexOf(ch, i + 1);
        if (end > i + 1 && text[end + 1] !== ch && text[end - 1] !== ch) {
          // 排除"裸 *"作为列表的情况：这种情况已在 inline 调用之前被块级解析吃掉，
          // 这里只关心行内对儿。
          pushBuf();
          out.push(
            <em key={`${keyPrefix}i-${i}`} className="italic">
              {renderMarkdownInline(text.slice(i + 1, end), `${keyPrefix}i${i}-`)}
            </em>,
          );
          i = end + 1;
          continue;
        }
      }
    }

    // 删除线 ~~...~~
    if (ch === "~" && text[i + 1] === "~") {
      const end = text.indexOf("~~", i + 2);
      if (end > i) {
        pushBuf();
        out.push(
          <s key={`${keyPrefix}s-${i}`} className="line-through opacity-80">
            {renderMarkdownInline(text.slice(i + 2, end), `${keyPrefix}s${i}-`)}
          </s>,
        );
        i = end + 2;
        continue;
      }
    }

    buf += ch;
    i += 1;
  }

  pushBuf();
  return out;
}

// ============== 块级 Markdown 渲染 ==============
//
// 重写历史
// --------
// 第一版会因为「列表 / 标题 / 引用前忘记 flush 段落」导致 `abc\n- def`
// 渲染成乱序。这一版统一在「跳出段落模式」时调用 ``closeParagraph``，
// 列表 / 标题 / 引用 / 代码块 / 分割线进入前一律先把段落和列表关闭，
// 同样的，进入列表时也先关闭段落，避免上一行被吞掉。
function renderMarkdown(content: string): React.ReactNode {
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  const out: React.ReactNode[] = [];
  let paragraphBuf: string[] = [];
  let listBuf: string[] = [];
  let listType: "ul" | "ol" | null = null;
  let blockCodeBuf: string[] | null = null;

  const flushParagraph = () => {
    if (paragraphBuf.length === 0) return;
    const text = paragraphBuf.join("\n");
    out.push(
      <p key={`p-${out.length}`} className="leading-relaxed break-words my-1">
        {renderMarkdownInline(text, `p${out.length}-`)}
      </p>,
    );
    paragraphBuf = [];
  };

  const flushList = () => {
    if (!listType || listBuf.length === 0) {
      listBuf = [];
      listType = null;
      return;
    }
    const items = listBuf.map((item, idx) => (
      <li key={`li-${out.length}-${idx}`} className="ml-5 leading-relaxed">
        {renderMarkdownInline(item, `li${out.length}-${idx}-`)}
      </li>
    ));
    if (listType === "ul") {
      out.push(
        <ul key={`ul-${out.length}`} className="my-2 list-disc list-outside space-y-0.5 pl-1">
          {items}
        </ul>,
      );
    } else {
      out.push(
        <ol key={`ol-${out.length}`} className="my-2 list-decimal list-outside space-y-0.5 pl-1">
          {items}
        </ol>,
      );
    }
    listBuf = [];
    listType = null;
  };

  const flushCodeBlock = () => {
    if (!blockCodeBuf) return;
    out.push(
      <pre
        key={`pre-${out.length}`}
        className="my-2 overflow-x-auto rounded-md bg-muted p-3 text-xs font-mono leading-snug"
      >
        <code>{blockCodeBuf.join("\n")}</code>
      </pre>,
    );
    blockCodeBuf = null;
  };

  /** 切换离开「段落 / 列表」上下文时的统一收尾。 */
  const flushBlockState = () => {
    flushParagraph();
    flushList();
  };

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];

    // 1) 代码块围栏 ```
    const fence = /^```\s*[\w-]*\s*$/.exec(line);
    if (fence) {
      if (blockCodeBuf) {
        flushCodeBlock();
      } else {
        flushBlockState();
        blockCodeBuf = [];
      }
      i += 1;
      continue;
    }
    if (blockCodeBuf) {
      blockCodeBuf.push(line);
      i += 1;
      continue;
    }

    // 2) 标题
    const heading = /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line);
    if (heading) {
      flushBlockState();
      const level = heading[1].length;
      const text = heading[2];
      const sizeClass =
        level <= 1 ? "text-xl font-bold"
        : level === 2 ? "text-lg font-bold"
        : level === 3 ? "text-base font-bold"
        : "text-sm font-semibold";
      const Tag = `h${Math.min(6, Math.max(1, level))}` as keyof JSX.IntrinsicElements;
      out.push(
        <Tag key={`h-${out.length}`} className={`mt-3 first:mt-0 ${sizeClass}`}>
          {renderMarkdownInline(text, `h${out.length}-`)}
        </Tag>,
      );
      i += 1;
      continue;
    }

    // 3) 引用
    if (/^>\s?/.test(line)) {
      flushBlockState();
      const quoteLines: string[] = [];
      while (i < lines.length && /^>\s?/.test(lines[i])) {
        quoteLines.push(lines[i].replace(/^>\s?/, ""));
        i += 1;
      }
      out.push(
        <blockquote
          key={`quote-${out.length}`}
          className="my-2 border-l-4 border-primary/50 pl-3 italic text-muted-foreground"
        >
          {renderMarkdownInline(quoteLines.join("\n"), `quote${out.length}-`)}
        </blockquote>,
      );
      continue;
    }

    // 4) 分割线（先于列表判定，避免 `---` 被当成 ul）
    //    规则：整行只有 ≥3 个相同字符（- * _），允许中间空格
    {
      const trimmed = line.trim();
      const compact = trimmed.replace(/\s+/g, "");
      if (
        compact.length >= 3 &&
        (compact === "-".repeat(compact.length) ||
          compact === "*".repeat(compact.length) ||
          compact === "_".repeat(compact.length))
      ) {
        flushBlockState();
        out.push(<hr key={`hr-${out.length}`} className="my-3 border-border" />);
        i += 1;
        continue;
      }
    }

    // 5) 列表
    //   - 进入列表前必须先 flushParagraph，避免上一段被吞
    //   - 兼容 `-/* /+` 三种无序符号；有序为 `数字.`
    const ul = /^\s*[-*+]\s+(.*)$/.exec(line);
    const ol = /^\s*\d+\.\s+(.*)$/.exec(line);
    if (ul) {
      flushParagraph();
      if (listType && listType !== "ul") flushList();
      listType = "ul";
      listBuf.push(ul[1]);
      i += 1;
      continue;
    }
    if (ol) {
      flushParagraph();
      if (listType && listType !== "ol") flushList();
      listType = "ol";
      listBuf.push(ol[1]);
      i += 1;
      continue;
    }
    if (listType) flushList();

    // 6) 空行 → 段落分隔
    if (line.trim() === "") {
      flushParagraph();
      i += 1;
      continue;
    }

    // 普通段落
    paragraphBuf.push(line);
    i += 1;
  }

  flushBlockState();
  flushCodeBlock();
  return <div className="space-y-1">{out}</div>;
}

// ============== BBCode 解析（基于栈、白名单） ==============
type BBToken =
  | { type: "text"; value: string }
  | { type: "open"; name: string; arg: string | null }
  | { type: "close"; name: string };

const BB_ALLOWED = new Set([
  "b", "i", "u", "s", "code", "quote", "url", "color", "size", "list", "*",
]);

function tokenizeBB(src: string): BBToken[] {
  const tokens: BBToken[] = [];
  const re = /\[(\/?)([a-zA-Z*]+)(?:=([^\]]*))?\]/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(src)) !== null) {
    if (m.index > last) {
      tokens.push({ type: "text", value: src.slice(last, m.index) });
    }
    const isClose = m[1] === "/";
    const name = m[2].toLowerCase();
    const arg = m[3] ?? null;
    if (isClose) {
      tokens.push({ type: "close", name });
    } else {
      tokens.push({ type: "open", name, arg });
    }
    last = re.lastIndex;
  }
  if (last < src.length) {
    tokens.push({ type: "text", value: src.slice(last) });
  }
  return tokens;
}

interface BBFrame {
  name: string;
  arg: string | null;
  children: React.ReactNode[];
  start: number;
}

function renderBBCode(src: string): React.ReactNode {
  const tokens = tokenizeBB(src);
  const root: React.ReactNode[] = [];
  const stack: BBFrame[] = [];

  const currentChildren = (): React.ReactNode[] =>
    stack.length ? stack[stack.length - 1].children : root;

  const pushText = (s: string) => {
    if (!s) return;
    // 自动把 \n 转成 <br/>
    const lines = s.split(/\r?\n/);
    lines.forEach((line, idx) => {
      currentChildren().push(line);
      if (idx < lines.length - 1) {
        currentChildren().push(<br key={`br-${stack.length}-${idx}-${currentChildren().length}`} />);
      }
    });
  };

  const closeFrame = (): void => {
    const frame = stack.pop();
    if (!frame) return;
    const node = renderBBNode(frame);
    currentChildren().push(node);
  };

  for (const tok of tokens) {
    if (tok.type === "text") {
      pushText(tok.value);
      continue;
    }
    if (tok.type === "open") {
      if (!BB_ALLOWED.has(tok.name)) {
        // 不在白名单 → 直接当字面文本
        pushText(`[${tok.name}${tok.arg !== null ? `=${tok.arg}` : ""}]`);
        continue;
      }
      // 处理列表项 [*]：自封闭 → 下一个 [*] 或 [/list] 来终止
      if (tok.name === "*") {
        // 把它压入 stack；遇到下一个 * 或 list 结束时弹出
        // 这里把同层未闭合的 * 自动 close
        if (stack.length && stack[stack.length - 1].name === "*") {
          closeFrame();
        }
        stack.push({ name: "*", arg: null, children: [], start: 0 });
        continue;
      }
      stack.push({ name: tok.name, arg: tok.arg, children: [], start: 0 });
      continue;
    }
    if (tok.type === "close") {
      if (!BB_ALLOWED.has(tok.name)) {
        pushText(`[/${tok.name}]`);
        continue;
      }
      // list 关闭前先把残留的 * 闭合
      if (tok.name === "list") {
        while (stack.length && stack[stack.length - 1].name === "*") {
          closeFrame();
        }
      }
      // 找最近匹配标签；不匹配则忽略多余 close
      let idx = stack.length - 1;
      while (idx >= 0 && stack[idx].name !== tok.name) idx -= 1;
      if (idx < 0) {
        pushText(`[/${tok.name}]`);
        continue;
      }
      // 关掉栈顶到 idx 之间所有未闭合的；为了简单，全部按顺序关掉
      while (stack.length > idx) {
        closeFrame();
      }
    }
  }

  // 收尾：未闭合的标签按出现顺序关掉
  while (stack.length) {
    closeFrame();
  }

  return <div className="space-y-1 whitespace-pre-wrap break-words leading-relaxed">{root}</div>;
}

function renderBBNode(frame: BBFrame): React.ReactNode {
  const { name, arg, children } = frame;
  const key = `${name}-${Math.random().toString(36).slice(2, 9)}`;
  switch (name) {
    case "b":
      return <strong key={key} className="font-semibold">{children}</strong>;
    case "i":
      return <em key={key} className="italic">{children}</em>;
    case "u":
      return <u key={key} className="underline">{children}</u>;
    case "s":
      return <s key={key} className="line-through opacity-80">{children}</s>;
    case "code": {
      // 含换行 → 渲染为块级 <pre>；单行 → 行内 <code>
      const hasBlock = (children as React.ReactNode[]).some(
        (n) => n && typeof n !== "string" && (n as any).type === "br",
      );
      if (hasBlock) {
        // 把 <br/> 替换为字符 "\n" 后塞进 <pre>
        const text = (children as React.ReactNode[])
          .map((n) => {
            if (typeof n === "string") return n;
            if (n && typeof n !== "string" && (n as any).type === "br") return "\n";
            return "";
          })
          .join("");
        return (
          <pre
            key={key}
            className="my-2 overflow-x-auto rounded-md bg-muted p-3 text-xs font-mono leading-snug whitespace-pre-wrap break-words"
          >
            <code>{text}</code>
          </pre>
        );
      }
      return (
        <code key={key} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">
          {children}
        </code>
      );
    }
    case "quote":
      return (
        <blockquote key={key} className="my-2 border-l-4 border-primary/50 pl-3 italic text-muted-foreground">
          {arg && <div className="text-xs text-muted-foreground/80 not-italic">{arg}</div>}
          {children}
        </blockquote>
      );
    case "url": {
      // 形态 1：[url]https://...[/url]，arg=null，children 是 URL
      // 形态 2：[url=https://...]label[/url]，arg=URL，children 是 label
      let url = arg || "";
      let labelNodes: React.ReactNode = children;
      if (!arg) {
        // children 都是字符串拼起来作为 URL
        url = (children as any[]).map((n) => (typeof n === "string" ? n : "")).join("").trim();
        labelNodes = url;
      }
      if (!isSafeUrl(url)) {
        return <span key={key}>{labelNodes}</span>;
      }
      return (
        <a
          key={key}
          href={url}
          target="_blank"
          rel="noopener noreferrer nofollow ugc"
          className="text-primary underline-offset-2 hover:underline break-words"
        >
          {labelNodes}
        </a>
      );
    }
    case "color": {
      if (!arg || !isSafeColor(arg)) return <span key={key}>{children}</span>;
      return <span key={key} style={{ color: arg }}>{children}</span>;
    }
    case "size": {
      if (!arg || !isSafeSize(arg)) return <span key={key}>{children}</span>;
      return <span key={key} style={{ fontSize: normalizeSize(arg) }}>{children}</span>;
    }
    case "list": {
      const ordered = (arg || "").trim() === "1";
      const items = (children as React.ReactNode[]).filter(Boolean);
      if (ordered) {
        return <ol key={key} className="list-decimal ml-5 my-2 space-y-1">{items}</ol>;
      }
      return <ul key={key} className="list-disc ml-5 my-2 space-y-1">{items}</ul>;
    }
    case "*":
      return <li key={key}>{children}</li>;
    default:
      return <span key={key}>{children}</span>;
  }
}

export function SafeAnnouncementContent({ content, mode }: { content: string; mode?: Mode | null }) {
  const m: Mode = (mode || "plain") as Mode;
  if (m === "markdown") {
    return <div className="text-sm">{renderMarkdown(content)}</div>;
  }
  if (m === "bbcode") {
    return <div className="text-sm">{renderBBCode(content)}</div>;
  }
  // 纯文本：保留换行；React 文本节点本身不会被解析为 HTML，天然安全
  return (
    <div className="text-sm leading-relaxed whitespace-pre-wrap break-words">
      {content}
    </div>
  );
}
