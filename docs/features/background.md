# 背景与头像

Twilight 支持用户头像、个人背景和认证页背景图。

## 认证页背景

- 上传接口：`POST /admin/config/upload-auth-background`
- 读取接口：`GET /system/auth-background`
- 配置键：`Global.auth_background_url`
- 环境变量：`TWILIGHT_AUTH_BACKGROUND_URL`

## 安全规则

- 校验扩展名和 MIME 类型。
- 文件路径必须限制在上传根目录内。
- 不允许通过背景 URL 注入任意脚本或危险 CSS。
- 兼容旧行为时也要保留路径安全检查。
