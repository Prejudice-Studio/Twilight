import { cookies } from "next/headers";
import { redirect } from "next/navigation";

/**
 * Root path 的唯一职责是：把请求重定向到 /dashboard 或 /login。
 *
 * 历史实现是 client component：先 SSR 出一个 "正在加载..." 占位，再用
 * useEffect 调 useAuthStore 拿持久化状态，最后 router.replace 跳转。问题：
 *
 *   1. SSR 阶段已经把首页 HTML 推给浏览器，未登录用户会先看到一闪的 logo +
 *      "正在加载..." 才被踢去 /login——既不优雅，又让登录前的 visible 内容
 *      被渲染（即便没敏感数据，也是无谓字节 + LCP 抖动）；
 *   2. 已登录用户的体验是同样的 flash → /dashboard 跳转；
 *   3. 跟 middleware 的 cookie 守卫不一致：middleware 已经在 /login /
 *      /dashboard 之间做服务端 redirect，根路径反而退化到客户端 effect，
 *      逻辑分裂，新人难判断"哪个文件在主导跳转"。
 *
 * 改成 server component：SSR 阶段就读 cookie 决定 redirect 目标，浏览器
 * 收到的第一份 HTTP 响应就是 302，不需要等 React 跑起来。
 *
 * 注意：cookie 仅证明"曾经登录过"——session 是否真的有效仍由后端在每个
 * API 请求里校验，与 middleware.ts 注释里说的同一道理。这里只为消除 SSR
 * 阶段的肉眼闪烁，不替代鉴权。
 */
export default async function Home() {
  const sessionCookie = (await cookies()).get("twilight_session")?.value;
  redirect(sessionCookie ? "/dashboard" : "/login");
}
