/** @type {import('next').NextConfig} */
const nextConfig = {
  poweredByHeader: false,
  // standalone 模式：build 后可独立运行，无需 node_modules
  output: 'standalone',
  // 允许开发环境的跨域请求
  allowedDevOrigins: ['127.0.0.1', 'localhost'],
  // 本地开发时将 /api/* 代理到后端（避免跨域）
  // 生产环境通过 NEXT_PUBLIC_API_URL 直连后端
  async rewrites() {
    const rules = [
      {
        source: '/favicon.ico',
        destination: '/favicon.svg',
      },
    ];

    // 仅当未设置 NEXT_PUBLIC_API_URL 时启用代理（即本地开发）
    if (process.env.NEXT_PUBLIC_API_URL) return rules;
    const backendUrl = process.env.BACKEND_URL || 'http://localhost:5000';
    return [
      ...rules,
      {
        source: '/api/:path*',
        destination: `${backendUrl}/api/:path*`,
      },
    ];
  },
  async headers() {
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'X-Frame-Options', value: 'DENY' },
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
        ],
      },
    ];
  },
  // 禁用 Next 服务端图片优化，避免特殊部署环境中由图片优化器代拉任意远程 URL。
  // 页面内图片组件均显式使用 unoptimized，头像/背景等资源由浏览器直接请求受控 API 或可信外部 CDN。
  images: {
    unoptimized: true,
  },
};

export default nextConfig;

if (process.env.TWILIGHT_OPENNEXT_DEV === 'true') {
  import('@opennextjs/cloudflare').then((m) => m.initOpenNextCloudflareForDev());
}
