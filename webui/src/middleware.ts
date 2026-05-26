import { NextRequest, NextResponse } from "next/server";

/**
 * Middleware：CSP 头 + 服务端 session cookie 守卫。
 *
 * 两件事合在一起做的原因是 Next 的 middleware 是请求最早的 server-side hook，
 * 想避开"未登录用户先看到管理面板再被 client-side router.push('/login') 踢
 * 走"的肉眼闪烁，必须在 RSC payload 写出之前就完成 redirect——而 RSC payload
 * 是由这个 middleware 之后的 React 服务器渲染产出的。
 *
 * ## CSP（脚本部分）
 *
 * 当前生产环境用 `script-src 'self' 'unsafe-inline'`。它是"在 Next 16 上能
 * 真正跑通的最严格策略"——并不是没尝试更紧的：
 *
 *   1. 最初为了挡反射型 XSS，曾改成 `script-src 'self' 'nonce-XXX' 'strict-dynamic'`。
 *      但 `'strict-dynamic'` 让浏览器忽略 `'self'`，而 Next 16 自动 nonce
 *      注入对 `_next/static/chunks/...` 偶尔漏标，生产出现整段 chunk 被
 *      拒绝、整页白屏（vercel/next.js 已知 issue）。
 *   2. 退到 `script-src 'self' 'nonce-XXX'`。chunk 走 'self' 通过了，
 *      但 Next 16 同样会塞内联 bootstrap `<script>` 不带 nonce——CSP3
 *      规范规定一旦 source-list 里有 nonce-source，`'unsafe-inline'`
 *      就会被忽略，于是这些内联脚本被全部拒绝、依赖 hydration 的页面
 *      整片报"Executing inline script violates ..."然后死透。
 *   3. 现在退到 `'self' 'unsafe-inline'`：与 Next 16 的内联 bootstrap +
 *      `_next/static/chunks` 共存的最简形式。等 Next 把 auto-nonce 修稳
 *      （或我们升级到内联脚本全 hash 化的版本）再回头收紧。
 *
 * 同源策略 + `frame-ancestors 'none'` + `object-src 'none'` 仍然挡掉了
 * `<iframe src=>` 嵌入与 Flash/PDF object 注入；XSS 表面 = "攻击者能写
 * 进同源 DOM"，与项目其他地方（HttpOnly cookie、CSRF token、后端输入
 * 校验）的纵深防御边界一致。
 *
 * ## Session cookie 守卫
 *
 * 后端在登录成功时写 `twilight_session`（HttpOnly + SameSite=Lax）。客户端
 * JS 拿不到，但 middleware 在 server 端可以读：
 *
 *   - protectedPrefixes 里的路径若没有 cookie ⇒ 302 -> /login?next=<path>
 *   - authPrefixes 里的路径若已有 cookie ⇒ 302 -> /dashboard
 *
 * 注意：cookie 仅证明"曾经登录过"，session 是否真的有效仍由后端在每个 API
 * 请求里校验；这里的目的只是消除 SSR 阶段的"先渲染管理面板，再被 client
 * effect 踢走"闪烁，不替代后端鉴权。前端的 router 仍然保留 useEffect 兜底，
 * 应对"cookie 还在但被 server 标记 invalid / 5xx 后清掉"的退化场景。
 */

const protectedPrefixes = [
  "/dashboard",
  "/admin",
  "/announcements",
  "/invite",
  "/media",
  "/score",
  "/settings",
];

const authPrefixes = ["/login", "/register", "/forgot-password"];

const SESSION_COOKIE = "twilight_session";

function pathMatches(pathname: string, prefixes: string[]): boolean {
  for (const prefix of prefixes) {
    if (pathname === prefix || pathname.startsWith(prefix + "/")) {
      return true;
    }
  }
  return false;
}

export function middleware(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  const hasSession = Boolean(request.cookies.get(SESSION_COOKIE)?.value);

  // 1) 未登录访问受保护页面 → 直接 302 到 /login？next=...
  //    用 redirect 而不是 rewrite：浏览器地址栏要变成 /login，避免用户在
  //    一个看起来仍在 /admin 的 URL 上看到登录页（既会让书签错乱，也容易
  //    被钓鱼站借用）。`next` 仅在白名单内才回填，避免 open redirect。
  if (!hasSession && pathMatches(pathname, protectedPrefixes)) {
    const loginURL = new URL("/login", request.url);
    if (pathMatches(pathname, protectedPrefixes)) {
      loginURL.searchParams.set("next", pathname + (search || ""));
    }
    return NextResponse.redirect(loginURL);
  }

  // 2) 已登录访问登录/注册/找回密码 → 直接送回 /dashboard，避免回退按钮
  //    把已登录用户卡在登录页上反复 submit。
  if (hasSession && pathMatches(pathname, authPrefixes)) {
    return NextResponse.redirect(new URL("/dashboard", request.url));
  }

  // 3) 走到这里说明鉴权 OK，继续注入 CSP。
  const isDev = process.env.NODE_ENV !== "production";
  // dev 下 Next 用 eval 做 HMR / RSC payload 解析；生产构建后丢掉 unsafe-eval。
  const scriptExtras = isDev ? " 'unsafe-eval'" : "";

  const extraConnect = process.env.NEXT_PUBLIC_CSP_CONNECT?.trim();
  const connectSrc = extraConnect ? `'self' ${extraConnect}` : "'self'";

  // script-src 见文件顶部注释：Next 16 的内联 bootstrap 与 nonce-source 互斥
  // （CSP3 规范要求 source-list 含 nonce-source 时忽略 'unsafe-inline'），
  // 走 'self' 'unsafe-inline' 是当前最稳的策略——同源 + frame-ancestors 'none'
  // + object-src 'none' 已挡掉框架嵌入与对象注入，纵深防御交给 HttpOnly cookie
  // / CSRF token / 后端输入校验。
  const csp = [
    "default-src 'self'",
    `script-src 'self' 'unsafe-inline'${scriptExtras}`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob: https:",
    "font-src 'self' data:",
    `connect-src ${connectSrc}`,
    "media-src 'self' https:",
    "frame-ancestors 'none'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "worker-src 'self' blob:",
    "manifest-src 'self'",
    "upgrade-insecure-requests",
  ].join("; ");

  const response = NextResponse.next();
  response.headers.set("Content-Security-Policy", csp);
  return response;
}

export const config = {
  matcher: [
    // 跳过 _next 静态资源、图片优化、favicon、其它静态资产；
    // 这些请求永远不会触发脚本执行，跳过 middleware 节省每请求 CPU。
    "/((?!_next/static|_next/image|favicon\\.ico|favicon\\.svg|images/|api/).*)",
  ],
};
