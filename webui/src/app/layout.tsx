import type { Metadata } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import { ThemeProvider } from "@/components/theme-provider";
import { Toaster } from "@/components/ui/toaster";
import { ConfirmDialogProvider } from "@/components/ui/confirm-dialog";
import {
  SITE_DESCRIPTION,
  SITE_ICON,
  SITE_NAME,
  SITE_TITLE,
} from "../lib/site-config";
import "./globals.css";

// 注意：`NEXT_PUBLIC_*` 在 `next build` 时会被静态替换，运行时无法再覆盖。
// 这里同时接受不带前缀的同名环境变量（在 Cloudflare Worker 通过 wrangler `vars`
// 或 Pages/Workers 的 Runtime 环境变量注入），交给 `generateMetadata` 在请求时读取，
// 从而支持「不重新构建即可改名/换图标」的部署诉求。
export async function generateMetadata(): Promise<Metadata> {
  const pickEnv = (...keys: string[]): string | undefined => {
    for (const key of keys) {
      const value = process.env[key];
      if (typeof value === "string" && value.trim()) return value.trim();
    }
    return undefined;
  };

  const siteName = pickEnv("SITE_NAME", "NEXT_PUBLIC_SITE_NAME") || SITE_NAME;
  const siteTitle =
    pickEnv("SITE_TITLE", "NEXT_PUBLIC_SITE_TITLE") ||
    (siteName !== SITE_NAME ? siteName : SITE_TITLE);
  const siteDescription =
    pickEnv("SITE_DESCRIPTION", "NEXT_PUBLIC_SITE_DESCRIPTION") ||
    SITE_DESCRIPTION;
  const siteIcon = pickEnv("SITE_ICON", "NEXT_PUBLIC_SITE_ICON") || SITE_ICON;

  const icons: Metadata["icons"] | undefined = siteIcon
    ? { icon: siteIcon, shortcut: siteIcon, apple: siteIcon }
    : undefined;

  return {
    title: siteTitle,
    description: siteDescription,
    ...(icons ? { icons } : {}),
  };
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
      </head>
      <body
        className={`${GeistSans.variable} ${GeistMono.variable} font-sans antialiased`}
      >
        <ThemeProvider
          attribute="class"
          defaultTheme="light"
          enableSystem={false}
          themes={["light", "dark"]}
          disableTransitionOnChange={false}
        >
          <ConfirmDialogProvider>
            {children}
            <Toaster />
          </ConfirmDialogProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}

