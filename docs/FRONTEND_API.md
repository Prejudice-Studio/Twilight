# Twilight 前端开发文档

Twilight 前端采用现代化技术栈构建，专注于提供极佳的用户体验。

## 技术栈

- **框架**: Next.js 16 (App Router)
- **样式**: Tailwind CSS
- **组件库**: Radix UI + Lucide React
- **状态管理**: Zustand
- **数据获取**: TanStack Query (React Query)
- **动画**: Framer Motion

## 开发配置

### 环境变量

在 `webui` 目录下创建 `.env.local`:

```env
# 后端 API 地址
NEXT_PUBLIC_API_URL=http://localhost:5000
```

### 运行

```bash
npm install
npm run dev
```

## 项目结构

- **`src/app`**: 页面路由定义。
- **`src/components`**: 可复用的 UI 组件。
- **`src/lib`**: 工具类和 API 客户端 (`api.ts`)。
- **`src/store`**: 状态管理（认证、全局状态）。
- **`src/hooks`**: 自定义 React Hooks。

## API 调用规范

前端统一使用 `webui/src/lib/api.ts` 中的 `ApiClient` 类与后端通信。

示例：
```typescript
import { api } from "@/lib/api";

const userInfo = await api.getMe();
```

所有请求都会自动携带 Auth Store 中的 Token。

## 主题系统

系统支持三种内置主题，可通过侧边栏切换：
1. **淡蓝与奶白色**
2. **淡柑橘色**
3. **深色模式**

切换主题时使用圆周扩散动画效果。
