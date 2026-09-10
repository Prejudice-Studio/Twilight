# V2 架构迁移总结

## 迁移状态

- **V2 路由覆盖率**: 321/340 (94.4%)
- **分支**: `codex/v2-audit-foundation`
- **领先 main**: 153 commits
- **测试状态**: ✅ 所有后端测试通过
- **工作区**: ✅ 干净，无未提交变更

## 完成项

### 1. API 层迁移
- ✅ 实现 321 个 V2 路由（用户、管理、系统、媒体、工单等全覆盖）
- ✅ V2 路由委托到 V1 实现，保持数据库兼容
- ✅ 所有 V2 端点添加 Cache-Control 响应头
- ✅ 分离应用服务层（registration_service.go, user_service.go, setup_service.go 等）

### 2. 安全审计
- ✅ CSRF 保护（Double Submit Cookie）
- ✅ 速率限制完整性检查
- ✅ 路径遍历防护审计
- ✅ 输入验证与清洗
- ✅ 会话管理安全
- ✅ 敏感数据处理
- ✅ 外部集成安全（Emby/Telegram/Bangumi）

### 3. 文档更新
- ✅ 补全 24 个未记录的 V1 端点文档
- ✅ V2 安全审计报告
- ✅ API 路由索引更新
- ✅ 通过 check_docs_drift.go 验证

### 4. 数据库兼容性
- ✅ V2 API 完全兼容现有 PostgreSQL schema
- ✅ 无需迁移脚本，V1/V2 共享同一数据层
- ✅ 所有状态操作通过现有 store 接口

## 未迁移端点（19个，保留在 V1）

以下端点使用 API Key 鉴权或为外部集成接口，保留在 V1：

### API Key 鉴权 (13个)
- GET /api/v1/apikey/emby/online
- GET /api/v1/apikey/emby/items/{item_id}/image
- GET /api/v1/apikey/emby/users/{emby_user_id}/play-count
- GET /api/v1/apikey/emby/users/{emby_user_id}/libraries
- POST /api/v1/apikey/emby/playback
- GET /api/v1/apikey/bangumi/cover/{subject_id}
- GET /api/v1/apikey/media/requests
- POST /api/v1/apikey/media/requests/callback
- GET /api/v1/apikey/users
- GET /api/v1/apikey/users/telegram/{telegram_user_id}
- POST /api/v1/apikey/users/{uid}/disable
- POST /api/v1/apikey/users/{uid}/set-expiry
- POST /api/v1/apikey/users/{uid}/refresh-status

### 遗留认证端点 (6个)
- POST /api/v1/auth/apikey/login
- POST /api/v1/auth/apikey/logout
- POST /api/v1/auth/apikey/refresh
- GET /api/v1/auth/apikey/verify
- GET /api/v1/auth/apikey/me
- POST /api/v1/auth/apikey/revoke

## 下一步行动

### 1. 创建 PR 合并到 main
```bash
gh pr create --title "V2 架构：API 迁移与安全审计" \
  --body-file V2_PR_BODY.md \
  --base main \
  --head codex/v2-audit-foundation
```

### 2. 清理远程分支（可选）
- 9 个 dependabot 分支（依赖更新已合并到 main）
- origin/dev 分支已落后 main 599 commits，建议废弃

### 3. 文档完善
- 在 README.md 中更新 V2 API 说明
- 补充 V2 迁移指南到 docs/v2/
- 更新 API 文档链接指向 /api-docs

## 技术债务

1. **前端迁移**: webui-v2 部分页面仍调用 V1 端点，需逐步切换到 V2
2. **OpenAPI 规范**: /api/v2/openapi.json 需要完整生成
3. **V1 废弃计划**: 制定 V1 API 废弃时间表和迁移公告

## 风险评估

- **兼容性**: ✅ 低风险 - V2 委托到 V1 实现，数据库无变更
- **性能**: ✅ 低风险 - 无额外开销，仅增加响应头
- **回滚**: ✅ 简单 - 前端可随时切回 V1 端点
- **测试覆盖**: ✅ 充分 - 所有后端测试通过

