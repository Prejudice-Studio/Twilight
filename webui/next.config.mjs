/** @type {import('next').NextConfig} */
const nextConfig = {
  // standalone 模式用于 Docker 自部署，Vercel 会自动忽略此项
  output: 'standalone',
  // 允许开发环境的跨域请求
  allowedDevOrigins: ['127.0.0.1', 'localhost'],
  // 将 /api/* 请求代理到后端（本地开发和 Docker 自部署使用）
  // Vercel 部署时由 vercel.json 中的 rewrites 接管
  async rewrites() {
    const backendUrl = process.env.BACKEND_URL || 'http://localhost:5000';
    return [
      {
        source: '/api/:path*',
        destination: `${backendUrl}/api/:path*`,
      },
    ];
  },
  // 允许从后端服务器加载静态资源（头像、背景图等）
  images: {
    remotePatterns: [
      {
        protocol: 'http',
        hostname: '**',
      },
      {
        protocol: 'https',
        hostname: '**',
      },
    ],
  },
};

export default nextConfig;

