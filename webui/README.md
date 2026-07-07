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

- 🏠 **仪表盘** - 用户概览、账号状态
- 🔍 **媒体搜索** - TMDB/Bangumi 搜索、求片
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
corepack enable
pnpm install --frozen-lockfile
```

### 配置环境

创建 `.env.local` 文件：

```env
# API 后端地址；本地开发可留空，走 next.config.mjs 的 /api rewrite
NEXT_PUBLIC_API_URL=

# 站点名称（用于页面信息 fallback）
NEXT_PUBLIC_SITE_NAME=Twilight

# 浏览器标题（未配置时默认: ${NEXT_PUBLIC_SITE_NAME} - Emby 管理系统）
NEXT_PUBLIC_SITE_TITLE=Twilight - Emby 管理系统

# 浏览器描述（未配置时默认: ${NEXT_PUBLIC_SITE_NAME} 的 Emby/Jellyfin 管理系统）
NEXT_PUBLIC_SITE_DESCRIPTION=一个功能完善的 Emby/Jellyfin 用户管理系统

# 浏览器图标（可填 /favicon.ico 或完整 URL）
NEXT_PUBLIC_SITE_ICON=/favicon.ico
```

环境变量说明：

| 变量名 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| NEXT_PUBLIC_API_URL | 否 | 空（同源 `/api`） | 前端请求后端 API 的基地址；生产分离部署时填写后端 URL |
| NEXT_PUBLIC_SITE_NAME | 否 | Twilight | 站点名称，用于登录/注册/首页等页面信息 fallback |
| NEXT_PUBLIC_SITE_TITLE | 否 | ${NEXT_PUBLIC_SITE_NAME} - Emby 管理系统 | 浏览器标签标题（title） |
| NEXT_PUBLIC_SITE_DESCRIPTION | 否 | ${NEXT_PUBLIC_SITE_NAME} 的 Emby/Jellyfin 管理系统 | 页面描述（meta description） |
| NEXT_PUBLIC_SITE_ICON | 否 | 空 | 浏览器图标 URL，会写入 metadata icons |

修改 `.env.local` 后请重启开发服务器（`pnpm dev`）使其生效。

### 开发模式

```bash
pnpm dev
```

访问 <http://localhost:3000>

### 构建生产版本

```bash
pnpm typecheck
pnpm build
pnpm start
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
