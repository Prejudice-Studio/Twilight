# 背景设置快速指南

## 对用户的说明

### 访问背景设置
1. 登录系统后，进入右上角的**设置**
2. 点击**背景设置**
3. 选择想要自定义的主题（浅色或暗色）

### 四种配置方式

#### 方式 1: 使用预设渐变 ⭐ 最简单
1. 直接点击底部的"快速预设"中的任意渐变
2. 立即在预览区看到效果
3. 点击"保存设置"即可

#### 方式 2: 自定义 CSS 渐变
1. 在"CSS 背景样式"输入框中输入 CSS 代码
2. 例如: `linear-gradient(135deg, #667eea 0%, #764ba2 100%)`
3. 预览区会实时显示效果
4. 点击"保存设置"

#### 方式 3: 使用图片 URL
1. 在"背景图片 URL"输入框中输入图片链接
2. 例如: `url(https://example.com/background.jpg)`
3. 预览区会加载并显示图片
4. 点击"保存设置"

#### 方式 4: 上传本地图片 ⭐ 推荐
1. 点击"选择文件"按钮
2. 从计算机选择图片（支持 JPG、PNG、GIF、WebP）
3. 图片会自动上传并在预览区显示
4. 点击"保存设置"应用更改

### 组合使用
可以同时设置 CSS 和图片：
- CSS 作为背景色或渐变
- 图片作为覆盖层（推荐使用半透明效果）
- 图片会显示在 CSS 渐变下方

### 浅色和暗色分别设置
系统支持为浅色和暗色主题设置不同的背景：
- **浅色主题背景** - 明亮、舒适的背景
- **暗色主题背景** - 深色、护眼的背景

### 重要提示
- 修改后**需要刷新页面**才能看到效果
- 跨域图片需要服务器支持 CORS
- 上传文件最大 5MB
- 记得点击"保存设置"确认修改

---

## 对管理员/开发者的说明

### 服务器配置

#### 文件存储位置
上传的背景图片存储在: `./uploads/backgrounds/`

#### 配置上传大小限制
编辑 `src/config.py`:
```python
class APIConfig(BaseConfig):
    MAX_UPLOAD_SIZE: int = 5 * 1024 * 1024  # 修改这里
```

#### 更改上传目录
编辑 `src/config.py`:
```python
class APIConfig(BaseConfig):
    UPLOAD_FOLDER: str = "/path/to/custom/uploads"
```

### 生产环境部署

#### 1. 创建上传目录
```bash
mkdir -p uploads/backgrounds
chmod 755 uploads/backgrounds
```

#### 2. 配置 Web 服务器 (Nginx)
```nginx
server {
    location /uploads/ {
        alias /path/to/project/uploads/;
        expires 30d;
        add_header Cache-Control "public, immutable";
    }
}
```

#### 3. 配置 Web 服务器 (Apache)
```apache
Alias /uploads /path/to/project/uploads
<Directory /path/to/project/uploads>
    Options +Indexes
    Require all granted
    ExpiresActive On
    ExpiresDefault "access plus 30 days"
</Directory>
```

#### 4. 定期清理过期文件
创建定时任务清理超过 90 天的文件:
```bash
find uploads/backgrounds -type f -mtime +90 -delete
```

### 迁移到云存储 (推荐)

对于高并发或分布式部署，建议使用云存储服务:

#### AWS S3 示例
```python
import boto3
from flask import request

s3_client = boto3.client('s3')

def upload_to_s3(file, bucket, key):
    s3_client.upload_fileobj(
        file,
        bucket,
        key,
        ExtraArgs={'ACL': 'public-read'}
    )
    return f"https://{bucket}.s3.amazonaws.com/{key}"
```

#### Azure Blob Storage 示例
```python
from azure.storage.blob import BlobServiceClient

blob_service_client = BlobServiceClient.from_connection_string(conn_str)
container_client = blob_service_client.get_container_client("backgrounds")
blob_client = container_client.upload_blob(name=filename, data=file)
return blob_client.url
```

### 备份用户背景配置

#### 导出用户背景
```bash
sqlite3 your_database.db "
SELECT uid, OTHER FROM USER WHERE OTHER LIKE '%background%';
" > backup_backgrounds.csv
```

#### 恢复用户背景
```bash
# 使用 Python 脚本恢复
```

### 安全建议

✅ **必须做的**
- [ ] 限制上传文件大小
- [ ] 验证文件类型（扩展名 + MIME）
- [ ] 为上传目录配置正确权限
- [ ] 定期备份用户数据
- [ ] 使用 HTTPS 传输

✅ **最佳实践**
- [ ] 使用 CDN 加速静态文件
- [ ] 启用病毒扫描（ClamAV）
- [ ] 实施数据加密
- [ ] 定期审计文件
- [ ] 设置访问日志

### 故障排除

#### 上传失败
```
错误: "文件过大"
解决: 检查 MAX_UPLOAD_SIZE 配置和 Web 服务器限制

错误: "格式不支持"
解决: 只支持 JPG、PNG、GIF、WebP

错误: "权限不足"
解决: 检查 uploads/ 目录权限 (需要 755 或更高)
```

#### 背景不显示
```
问题: 图片 404
原因: 文件未保存成功或目录配置错误
解决: 检查 uploads/backgrounds/ 目录存在且有正确权限

问题: 跨域图片不显示
原因: 远程服务器不支持 CORS
解决: 上传文件到本地或使用支持 CORS 的服务
```

### 性能优化

#### 图片优化
```python
from PIL import Image
import io

def optimize_image(file):
    img = Image.open(file)
    
    # 限制尺寸
    max_size = (1920, 1080)
    img.thumbnail(max_size)
    
    # 压缩质量
    output = io.BytesIO()
    img.save(output, format='JPEG', quality=85)
    
    return output.getvalue()
```

#### 缓存策略
```bash
# 配置 HTTP 缓存头
Cache-Control: public, max-age=2592000  # 30天
ETag: 强验证
Last-Modified: 修改日期
```

---

## 常见问题

### Q: 支持哪些图片格式?
A: JPG、PNG、GIF、WebP，最大 5MB

### Q: 能否同时使用 CSS 和图片?
A: 可以，图片会显示在 CSS 背景下方

### Q: 能否从其他用户导入背景?
A: 目前不支持，可通过分享 CSS 代码或图片 URL 实现共享

### Q: 删除背景后能否恢复?
A: 需要重新设置，系统不保留历史记录

### Q: 上传的图片会被压缩吗?
A: 不会，建议上传前自行优化

### Q: 支持 GIF 动画吗?
A: 支持上传 GIF，但显示效果取决于浏览器和 CSS 支持

### Q: 能否限制某些用户无法上传?
A: 目前没有，所有认证用户都可以上传
