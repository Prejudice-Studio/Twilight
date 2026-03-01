# 背景自定义功能完整实现 - 变更总结（历史记录）

> 本文档用于保留一次性实现过程记录，不作为日常维护主文档。
> 当前请优先参考：`BACKGROUND_FEATURES.md`、`BACKGROUND_GUIDE.md`、`docs/BACKEND_API.md`。

## 概述
成功实现了完整的用户背景自定义系统，支持三种方式：CSS 渐变、图片 URL、文件上传。

## 修改文件列表

### 后端

#### 1. `src/config.py`
**变更**: 添加文件上传配置
- 为 `APIConfig` 类添加 `UPLOAD_FOLDER` 配置（默认: `./uploads`）
- 为 `APIConfig` 类添加 `MAX_UPLOAD_SIZE` 配置（默认: 5MB）

#### 2. `src/api/__init__.py`
**变更**: 配置 Flask 应用支持文件上传和静态文件服务
- 导入 `APIConfig` 配置
- 设置 Flask 的 `static_folder` 为上传目录
- 设置 `static_url_path` 为 `/uploads`
- 配置 `MAX_CONTENT_LENGTH` 限制上传文件大小
- 自动创建上传目录

#### 3. `src/api/v1/users.py`
**变更**: 添加文件上传端点和完善背景 API
- 修复 `update_user_background()` 函数：
  - 支持 4 个字段：`lightBg`, `darkBg`, `lightBgImage`, `darkBgImage`
  - 添加完整的认证检查
  - 添加 JSON 解析错误处理
  
- 修复 `delete_user_background()` 函数：
  - 添加完整的认证检查
  - 改进 JSON 处理逻辑
  
- **新增** `upload_background_image()` 端点:
  - 支持 POST 上传图片文件
  - 需要认证 (`@require_auth` 装饰器)
  - 接受参数：`file`（图片文件）、`type`（"light" 或 "dark"）
  - 验证文件大小（≤ 5MB）
  - 验证文件类型（JPG、PNG、GIF、WebP）
  - 生成唯一文件名（UUID + 原始扩展名）
  - 返回访问 URL：`/uploads/backgrounds/{filename}`

### 前端

#### 1. `webui/src/app/(main)/layout.tsx`
**变更**: 增强背景加载逻辑
- 修改 `loadUserBg()` 函数支持所有 4 个字段
- 实现智能字段组合：
  - 如果 CSS 和 URL 都有：`url(...), css(...)`
  - 如果只有 URL：使用 URL
  - 如果只有 CSS：使用 CSS
- 根据当前主题自动选择相应背景（light/dark）

#### 2. `webui/src/app/(main)/settings/background/page.tsx`
**变更**: 完整重写背景设置页面
- 分离浅色和暗色背景配置为独立组件
- 支持三种配置方式：
  1. CSS `background-image` 属性
  2. 图片 URL（`url(...)`格式）
  3. 本地文件上传
  
- 实施函数：
  - `loadBackgroundConfig()` - 加载已保存的配置
  - `updatePreview()` - 实时预览背景
  - `handleFileUpload()` - 处理文件上传
  - `handleSave()` - 保存所有配置
  - `handleReset()` - 重置为默认背景
  - `applyPreset()` - 应用预设渐变
  
- 用户交互：
  - 实时预览窗口
  - 6 个热门渐变预设
  - 文件上传进度提示
  - 错误提示和成功反馈
  - 禁用/启用按钮状态管理

### 新增文件

#### 1. `uploads/backgrounds/` 目录
- 自动创建，用于存储上传的背景图片

#### 2. `BACKGROUND_FEATURES.md`
- 详细文档说明所有功能、API 和使用方式

## API 端点

### 获取用户背景配置
```
GET /api/v1/users/{uid}/background
```

### 更新背景配置
```
PUT /api/v1/users/me/background
Authentication: Required
Content-Type: application/json

Body:
{
  "lightBg": "linear-gradient(...)",
  "darkBg": "linear-gradient(...)",
  "lightBgImage": "url(https://...)",
  "darkBgImage": "url(https://...)"
}
```

### 上传背景图片 ⭐ 新增
```
POST /api/v1/users/me/background/upload
Authentication: Required
Content-Type: multipart/form-data

Parameters:
- file: 图片文件
- type: "light" 或 "dark"

Response:
{
  "success": true,
  "data": {
    "url": "/uploads/backgrounds/abc123.jpg",
    "type": "light",
    "filename": "abc123.jpg"
  }
}
```

### 删除背景配置
```
DELETE /api/v1/users/me/background
Authentication: Required
```

## 技术细节

### 文件验证
- **大小限制**: 5MB
- **格式支持**: JPG、PNG、GIF、WebP
- **命名方式**: UUID 生成唯一名称（避免冲突）

### 存储结构
```json
{
  "OTHER": {
    "background": {
      "lightBg": "CSS 背景值",
      "darkBg": "CSS 背景值",
      "lightBgImage": "URL 值",
      "darkBgImage": "URL 值",
      "updated_at": "时间戳"
    }
  }
}
```

### URL 路由
- 上传文件存储: `/uploads/backgrounds/`
- 访问 URL: `/uploads/backgrounds/{filename}`
- 由 Flask 静态文件服务提供

## 安全考虑

✅ **身份认证**
- 所有修改 API 需要 Bearer Token 认证
- 上传端点强制认证

✅ **文件验证**
- 验证文件扩展名
- 验证 MIME 类型
- 限制文件大小
- 生成唯一文件名避免覆盖

✅ **访问控制**
- 用户只能修改/删除自己的背景
- 获取背景时检查用户ID

## 测试步骤

1. **登录系统**
   ```
   POST /api/v1/auth/login
   ```

2. **访问背景设置页面**
   ```
   Navigate to /settings/background
   ```

3. **测试 CSS 背景**
   - 输入: `linear-gradient(135deg, #667eea 0%, #764ba2 100%)`
   - 预览应该显示蓝色渐变

4. **测试 URL 背景**
   - 输入: `url(https://via.placeholder.com/1920x1080)`
   - 预览应该显示图片（如可访问）

5. **测试文件上传**
   - 选择本地图片文件
   - 点击"选择文件"或上传
   - 等待上传完成
   - 预览应该显示已上传的图片

6. **保存配置**
   - 点击"保存设置"
   - 刷新页面验证效果

7. **主题切换**
   - 切换主题（Light/Dark）
   - 背景应该自动切换到相应主题的配置

## 已知限制

- 跨域图片需要服务器支持 CORS
- 上传的文件只能通过 `url()` 方式在 CSS 中使用
- 删除背景配置后恢复需要重新设置

## 后续优化建议

1. **图片压缩** - 上传时自动压缩图片
2. **图库功能** - 提供预选的高质量背景库
3. **临时上传链接** - 实现过期机制
4. **CDN 集成** - 通过 CDN 加速图片加载
5. **主题同步** - 从其他用户的主题库导入

## 版本信息

- **实现日期**: 2024
- **兼容性**: 
  - 后端: Flask 3.1+, SQLAlchemy 2.0+
  - 前端: Next.js 16+, Tailwind CSS 3.0+
  - 浏览器: 现代浏览器（支持 CSS 变量）

## 注意事项

⚠️ **重要**:
- 确保 `/uploads` 目录有写入权限
- 生产环境建议使用云存储（如 S3）替代本地存储
- 定期清理过期的上传文件
- 备份用户背景配置数据
