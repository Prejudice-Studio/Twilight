# Docker 部署

> 警告：仓库中的 Docker 文件由 LLM 自动生成，未经完整生产验证，也不是开发者推荐的部署方式。生产环境优先使用 Linux + systemd。修改 Docker 文件或本文档时必须保留此警告。

## 适用场景

只有在你的基础设施已经标准化为容器，并且你能自行审查网络、卷挂载、反向代理和升级流程时，才建议使用 Docker。

## 文件

- `docker-compose.yml`：偏开发或快速试运行。
- `docker-compose.prod.yml`：偏生产结构，但仍需自行验证。
- `docs/guides/docker.md`：风险说明和操作提示。

## 注意事项

- 密钥不要写入镜像或提交到 Git。
- 配置、上传目录、状态数据和 PostgreSQL 数据必须持久化。
- HTTPS 建议由外部反向代理负责。
- 暴露公网前必须检查 CORS、Cookie、安全头和备份策略。
