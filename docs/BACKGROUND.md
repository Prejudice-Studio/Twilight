# 背景自定义功能

用户可完全自定义主页背景，支持三种方式：CSS 渐变、图片 URL、本地文件上传。

---

## 用户指南

### 访问入口

登录后进入 **设置 → 背景设置**，可分别配置浅色和暗色主题的背景。

### 四种配置方式

| 方式 | 说明 | 推荐度 |
|------|------|--------|
| 预设渐变 | 点击底部"快速预设"中的任意渐变 | ⭐ 最简单 |
| CSS 渐变 | 输入 CSS `background-image` 值，如 `linear-gradient(135deg, #667eea 0%, #764ba2 100%)` | 灵活 |
| 图片 URL | 输入 `url(https://example.com/bg.jpg)` 格式的链接 | 外部图片 |
| 上传本地图片 | 点击"选择文件"上传 JPG/PNG/GIF/WebP | ⭐ 推荐 |

### 组合使用

可同时设置 CSS 和图片，图片显示在 CSS 渐变下方。推荐搭配半透明渐变叠加：

```json
{
  "lightBg": "linear-gradient(rgba(255,255,255,0.8), rgba(255,255,255,0.8))",
  "lightBgImage": "url(https://example.com/light.jpg)"
}
```

### 注意事项

- 修改后**需刷新页面**才能看到效果
- 跨域图片需服务器支持 CORS
- 上传文件最大 5MB
- 切换主题时背景自动切换

---

## API 接口

### 获取背景配置

```http
GET /api/v1/users/{uid}/background
```

返回 4 个字段：`lightBg`、`darkBg`、`lightBgImage`、`darkBgImage`。

### 更新背景配置

```http
PUT /api/v1/users/me/background
Authorization: Bearer {token}
Content-Type: application/json
```

```json
{
  "lightBg": "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
  "darkBg": "linear-gradient(135deg, #1e1e1e 0%, #1a1a2e 100%)",
  "lightBgImage": "url(https://example.com/light.jpg)",
  "darkBgImage": "url(https://example.com/dark.jpg)"
}
```

### 上传背景图片

```http
POST /api/v1/users/me/background/upload
Authorization: Bearer {token}
Content-Type: multipart/form-data
```

参数：`file`（图片文件，≤5MB）、`type`（`"light"` 或 `"dark"`）

返回：`{ "url": "/uploads/backgrounds/abc123.jpg", "type": "light", "filename": "abc123.jpg" }`

### 删除背景配置

```http
DELETE /api/v1/users/me/background
Authorization: Bearer {token}
```

---

## 存储结构

背景配置存储在用户表的 `OTHER` JSON 字段中：

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

上传的图片文件存储在 `./uploads/backgrounds/`，通过 `/uploads/backgrounds/{filename}` 访问。

### 限制

| 项目 | 限制 |
|------|------|
| 文件大小 | 最大 5MB |
| 文件类型 | JPG、PNG、GIF、WebP |
| CSS 长度 | 最大 2000 字符 |
| URL 长度 | 最大 2000 字符 |

---

## 运维与部署

### 服务器配置

```python
# src/config.py
class APIConfig(BaseConfig):
    UPLOAD_FOLDER: str = "./uploads"           # 上传目录
    MAX_UPLOAD_SIZE: int = 5 * 1024 * 1024     # 最大文件大小
```

### 生产环境

#### Nginx

```nginx
location /uploads/ {
    alias /path/to/project/uploads/;
    expires 30d;
    add_header Cache-Control "public, immutable";
}
```

#### 定期清理

```bash
find uploads/backgrounds -type f -mtime +90 -delete
```

### 故障排除

| 问题 | 原因 | 解决 |
|------|------|------|
| 背景不显示 | 未保存或未刷新 | 点击保存后刷新页面 |
| 上传失败 - 文件过大 | 超过 5MB | 压缩后重试 |
| 上传失败 - 格式不支持 | 非 JPG/PNG/GIF/WebP | 转换格式 |
| 图片 404 | 目录不存在或权限错误 | 检查 `uploads/backgrounds/` 权限 |
| 跨域图片不显示 | 远程服务器不支持 CORS | 上传到本地 |

---

## 常见问题

**Q: 能否同时使用 CSS 和图片？**
A: 可以，图片显示在 CSS 背景下方。

**Q: 支持 GIF 动画吗？**
A: 支持上传 GIF，显示效果取决于浏览器。

**Q: 删除背景后能否恢复？**
A: 不能，需重新设置。

**Q: 上传的图片会被压缩吗？**
A: 不会，建议上传前自行优化。
