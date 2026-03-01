# 背景自定义功能

## 功能概述

用户现在可以完全自定义主页的背景，支持三种方式：
1. **CSS 渐变** - 输入任何有效的 CSS `background-image` 值
2. **图片 URL** - 使用外部图片链接
3. **文件上传** - 直接上传本地图片文件

## 后端 API

### 获取背景配置
```
GET /api/v1/users/{uid}/background
```

返回当前用户的背景配置（4个字段）：
```json
{
  "lightBg": "linear-gradient(...)",
  "darkBg": "linear-gradient(...)",
  "lightBgImage": "url(...)",
  "darkBgImage": "url(...)"
}
```

### 更新背景配置
```
PUT /api/v1/users/me/background
Authorization: Bearer {token}
Content-Type: application/json
```

请求体：
```json
{
  "lightBg": "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
  "darkBg": "linear-gradient(135deg, #1e1e1e 0%, #1a1a2e 100%)",
  "lightBgImage": "url(https://example.com/light.jpg)",
  "darkBgImage": "url(https://example.com/dark.jpg)"
}
```

### 上传背景图片
```
POST /api/v1/users/me/background/upload
Authorization: Bearer {token}
Content-Type: multipart/form-data

Parameters:
- file: 图片文件（最大 5MB）
- type: "light" 或 "dark"
```

返回：
```json
{
  "url": "/uploads/backgrounds/abc123.jpg",
  "type": "light",
  "filename": "abc123.jpg"
}
```

### 删除背景配置
```
DELETE /api/v1/users/me/background
Authorization: Bearer {token}
```

## 前端使用

### 背景设置页面
访问 `/settings/background` 进行背景自定义：

1. **浅色主题背景** - 自定义浅色模式下的背景
2. **暗色主题背景** - 自定义暗色模式下的背景

每个部分支持：
- 输入 CSS `background-image` 值
- 输入 URL 格式的图片链接
- 上传本地图片文件
- 使用预设渐变

### 主页背景加载
主页自动加载并应用用户配置的背景：
- 支持 CSS 和图片的组合（图片在CSS下层）
- 根据当前主题（light/dark）自动选择相应背景
- 背景固定（`background-attachment: fixed`）

## 存储结构

用户背景配置存储在用户表的 `OTHER` JSON 字段中：

```json
{
  "background": {
    "lightBg": "...",
    "darkBg": "...",
    "lightBgImage": "...",
    "darkBgImage": "...",
    "updated_at": "2024-01-01T12:00:00"
  }
}
```

## 上传文件位置

上传的背景图片存储在：`/uploads/backgrounds/`

可通过 `/uploads/backgrounds/{filename}` 访问

## 限制

- **文件大小** - 最大 5MB
- **文件类型** - JPG、PNG、GIF、WebP
- **CSS 长度** - 最大 2000 字符
- **URL 长度** - 最大 2000 字符

## 示例

### 仅使用 CSS 渐变
```json
{
  "lightBg": "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
  "darkBg": "linear-gradient(135deg, #1e1e1e 0%, #1a1a2e 100%)"
}
```

### 仅使用图片 URL
```json
{
  "lightBgImage": "url(https://via.placeholder.com/1920x1080?text=Light)",
  "darkBgImage": "url(https://via.placeholder.com/1920x1080?text=Dark)"
}
```

### 组合使用（图片 + CSS）
```json
{
  "lightBg": "linear-gradient(rgba(255,255,255,0.8), rgba(255,255,255,0.8))",
  "lightBgImage": "url(https://example.com/light.jpg)"
}
```

当两个都设置时，图片会显示在 CSS 背景下方。

## 技术实现

### 后端
- Flask 异步路由
- Werkzeug 文件管理
- 用户认证检查（`@require_auth`）
- 文件验证和大小检查

### 前端
- Next.js 16+ App Router
- Framer Motion 动画
- 实时预览
- 主题感知加载

## 故障排除

### 背景不显示
1. 检查是否已按保存按钮
2. 刷新页面
3. 检查浏览器控制台是否有错误

### 上传失败
1. 确保文件大小 < 5MB
2. 确保文件格式是支持的图片格式
3. 检查网络连接

### 跨域图片不显示
- 确保远程服务器支持 CORS
- 或上传图片文件而不是使用 URL

## 查看效果

1. 登录系统
2. 进入设置 → 背景设置
3. 选择浅色或暗色主题配置
4. 使用任意组合方式（CSS、URL、上传）
5. 点击"保存设置"
6. 刷新主页查看效果
7. 切换主题时背景自动切换
