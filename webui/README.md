# Twilight Web UI

基于 Next.js 16 + TypeScript + Tailwind CSS 的现代化 Web 管理界面。

## 技术栈

- **框架**: [Next.js 16](https://nextjs.org/) (App Router)
- **语言**: TypeScript
- **样式**: [Tailwind CSS](https://tailwindcss.com/)
- **UI 组件**: [shadcn/ui](https://ui.shadcn.com/) + [Radix UI](https://www.radix-ui.com/)
- **动画**: [Framer Motion](https://www.framer.com/motion/)
- **状态管理**: [Zustand](https://zustand-demo.pmnd.rs/)
- **数据请求**: [TanStack Query](https://tanstack.com/query)
- **图表**: [Recharts](https://recharts.org/)
- **图标**: [Lucide Icons](https://lucide.dev/)

## 功能页面

### 用户功能

- 🏠 **仪表盘** - 用户概览、签到、统计
- 🔍 **媒体搜索** - TMDB/Bangumi 搜索、求片
- 💰 **积分中心** - 余额、转账、记录、排行榜、红包
- ⚙️ **个人设置** - Telegram 绑定、Emby 绑定、偏好设置
- 🎬 **我的求片** - 查看已提交请求的状态与详情

### 管理功能

- 👥 **用户管理** - 列表、搜索、续期、禁用
- 📝 **注册码** - 生成、管理注册码
- 🎬 **求片审核** - 审批用户请求
- 📊 **数据统计** - 系统状态概览
- 🔐 **安全管理** - IP 限制、登录保护

## 快速开始

### 安装依赖

```bash
cd webui
pnpm install
# 或
npm install
```

### 配置环境

创建 `.env.local` 文件：

```env
# API 后端地址
NEXT_PUBLIC_API_URL=http://localhost:5000

# 站点名称
NEXT_PUBLIC_SITE_NAME=Twilight
```

### 开发模式

```bash
pnpm dev
# 或 npm run dev
```

访问 <http://localhost:3000>

### 构建生产版本

```bash
pnpm build
pnpm start
# 或 npm run build && npm run start
```

## 目录结构

```text
webui/
├── src/
│   ├── app/                    # Next.js App Router
│   │   ├── (auth)/             # 认证相关页面
│   │   │   ├── login/
│   │   │   └── register/
│   │   ├── (main)/             # 主要功能页面
│   │   │   ├── dashboard/
│   │   │   ├── media/
│   │   │   ├── score/
│   │   │   ├── settings/
│   │   │   └── admin/
│   │   ├── layout.tsx
│   │   └── page.tsx
│   ├── components/
│   │   ├── ui/                 # UI 组件
│   │   ├── layout/             # 布局组件
│   │   └── theme-provider.tsx
│   ├── hooks/                  # 自定义 Hooks
│   ├── lib/                    # 工具函数
│   │   ├── api.ts              # API 客户端
│   │   └── utils.ts
│   └── store/                  # 状态管理
│       └── auth.ts
├── public/
├── tailwind.config.ts
├── next.config.mjs
└── package.json
```

## 主题定制

项目使用 CSS 变量实现主题系统，支持亮色/暗色模式自动切换。

主题色定义在 `src/app/globals.css`：

```css
:root {
  --primary: 280 85% 55%;  /* 紫色系主色 */
  --accent: 280 70% 95%;
  /* ... */
}

.dark {
  --primary: 280 85% 65%;
  /* ... */
}
```

自定义色板在 `tailwind.config.ts`：

```typescript
colors: {
  twilight: { /* 紫色渐变 */ },
  sunset: { /* 橙色渐变 */ },
}
```

## API 对接

API 客户端位于 `src/lib/api.ts`，已封装以下功能：

- 自动 Token 管理
- 请求/响应拦截
- 错误处理
- TypeScript 类型定义

使用示例：

```typescript
import { api } from "@/lib/api";

// 登录
const res = await api.login(username, password);

// 获取用户信息
const user = await api.getMe();

// 搜索媒体
const results = await api.searchMedia("进击的巨人", "all");
```

## 部署

### Docker

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public

EXPOSE 3000
CMD ["node", "server.js"]
```

### Nginx 反向代理

```nginx
location / {
    proxy_pass http://localhost:3000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host $host;
    proxy_cache_bypass $http_upgrade;
}
```

## 许可证

MIT License

