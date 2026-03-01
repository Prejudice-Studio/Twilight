# Twilight 前端开发文档

Twilight 当前主前端位于 `webui/`，技术栈为 Next.js + TypeScript + Tailwind CSS。

## 前端目录说明

- `webui/`：主前端（Next.js 16，生产使用）
- `webui/webui/`：历史 Vite 原型工程（默认不参与主流程）

## 技术栈（主前端）

- Next.js 16（App Router）
- TypeScript
- Tailwind CSS
- Radix UI + 自定义组件
- Zustand（状态管理）
- TanStack Query（数据请求）
- Framer Motion（动效）

## 本地开发

### 1) 安装依赖

```bash
cd webui
pnpm install
# 或 npm install
```

### 2) 配置环境变量

创建 `webui/.env.local`：

```env
NEXT_PUBLIC_API_URL=http://localhost:5000
NEXT_PUBLIC_SITE_NAME=Twilight
```

### 3) 启动开发服务

```bash
pnpm dev
# 或 npm run dev
```

默认访问：`http://localhost:3000`

## 与后端联调

- 后端推荐启动命令：`python main.py api --debug`
- Swagger：`http://localhost:5000/api/v1/docs`
- 前端 API 客户端：`webui/src/lib/api.ts`

示例：

```ts
import { api } from "@/lib/api";

const me = await api.getMe();
```

## 构建与发布

```bash
pnpm build
pnpm start
```

若使用 npm：

```bash
npm run build
npm run start
```

## 常见问题

### 前端请求 401

- 确认已登录并有有效 Token
- 检查浏览器存储中的认证信息是否过期

### 前端请求不到后端

- 检查 `NEXT_PUBLIC_API_URL`
- 确认后端服务运行在对应端口（默认 5000）
- 如跨域，检查后端 CORS 配置

### 页面样式异常

- 删除缓存后重装依赖：`rm -rf node_modules .next`（Windows 可手动删除）
- 重新安装并启动

## 参考文档

- [后端 API 文档](./BACKEND_API.md)
- [开发指南](./DEVELOPMENT.md)
- [安装部署指南](./INSTALL.md)
