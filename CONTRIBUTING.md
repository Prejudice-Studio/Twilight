# 贡献指南

感谢你对 Twilight 项目的关注！我们欢迎各种形式的贡献，包括但不限于：

- 🐛 报告 Bug
- 💡 提出新功能建议
- 📖 改进文档
- 🔧 提交代码修复或新功能
- 🌍 翻译和国际化

## 行为准则

参与本项目即表示你同意遵守以下准则：

- 尊重所有贡献者和用户
- 使用友好、包容的语言
- 接受建设性的批评
- 关注对社区最有利的事情

## 如何贡献

### 报告 Bug

在提交 Bug 之前，请先搜索 [Issues](https://github.com/Prejudice-Studio/Twilight/issues) 确认问题尚未被报告。

**Bug 报告应包含**：

- **清晰的标题**：简短描述问题
- **复现步骤**：详细的操作步骤
- **预期行为**：你期望发生什么
- **实际行为**：实际发生了什么
- **环境信息**：
  - Twilight 版本
  - 操作系统和版本
  - Go 版本
  - PostgreSQL 版本
  - 浏览器（如果是前端问题）
- **日志和截图**：相关的错误日志或截图

### 提出新功能

在提交新功能建议前，请先搜索现有 Issues 确认类似建议不存在。

**功能建议应包含**：

- **使用场景**：为什么需要这个功能
- **期望行为**：功能应该如何工作
- **替代方案**：是否考虑过其他实现方式
- **影响范围**：对现有功能的影响

### 提交代码

#### 准备工作

1. **Fork 仓库**到你的 GitHub 账号
2. **克隆**你 fork 的仓库到本地
3. **创建分支**：`git checkout -b feature/your-feature-name`

#### 开发环境

详细的开发环境设置请参考 [开发指南](docs/guides/development.md)。

**基础要求**：

- Go 1.25+
- Node.js 18+
- PostgreSQL 14+
- pnpm 8+ (前端)

**启动开发服务**：

```bash
# 后端
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml --debug

# 前端
cd webui-v2
pnpm install
pnpm dev
```

#### 代码规范

**Go 后端**：

- 使用 `gofmt` 格式化代码
- 运行 `go vet ./...` 和 `go test ./...`
- 遵循 [Effective Go](https://go.dev/doc/effective_go) 规范
- 关键业务逻辑添加单元测试
- 不在代码、配置、文档中包含绝对路径

**前端**：

- 使用 `pnpm format` 格式化代码
- 使用 TypeScript 严格模式
- 组件和函数添加适当的类型注解
- 遵循项目现有的代码风格

**通用规范**：

- Commit 信息使用中文，格式：`类型：简短描述`
  - 类型：功能、修复、文档、重构、性能、测试、构建、样式
  - 示例：`功能：增加 Telegram 身份历史追踪`
- 一个 Commit 只做一件事
- 不提交调试代码、临时文件、二进制文件
- 敏感信息（密钥、密码、Token）不能提交

#### 提交 Pull Request

1. **确保代码通过测试**：

```bash
# 后端
go test ./...
go vet ./...

# 前端
cd webui-v2
pnpm check
pnpm build
```

2. **推送到你的 fork**：

```bash
git push origin feature/your-feature-name
```

3. **创建 Pull Request**：
   - 标题：清晰描述改动内容
   - 描述：
     - 改动的动机和背景
     - 主要改动内容
     - 测试方法
     - 相关 Issue（如有）

4. **等待审查**：
   - 维护者会审查你的代码
   - 可能需要修改，请及时响应反馈
   - 审查通过后会被合并

#### PR 审查标准

- ✅ 代码符合项目规范
- ✅ 功能完整且可用
- ✅ 包含必要的测试
- ✅ 文档已更新（如需要）
- ✅ 不破坏现有功能
- ✅ Commit 信息清晰
- ✅ 无敏感信息泄露

### 改进文档

文档位于 `docs/` 目录，使用 Markdown 格式。

**文档贡献**：

- 修正错误或过时内容
- 补充缺失的文档
- 改进文档结构和可读性
- 翻译文档（目前主要是中文）

**文档风格**：

- 使用简洁、清晰的语言
- 代码示例要完整且可运行
- 使用标题、列表和表格组织内容
- 添加必要的链接

### 翻译和国际化

前端国际化文件位于 `webui-v2/src/lib/locales/`。

**添加新语言**：

1. 复制 `zh-CN.json` 为新语言代码（如 `en-US.json`）
2. 翻译所有字符串
3. 在 `webui-v2/src/lib/i18n.ts` 注册新语言
4. 测试所有页面的翻译效果

详见 [前端多语言开发](docs/guides/i18n.md)。

## 开发流程

### 分支策略

- `main` - 主分支，保持稳定，只接受 PR
- `codex/*` - 功能分支，开发新功能或重构
- `hotfix/*` - 紧急修复分支

### 发布流程

1. 从 `main` 创建 `release/vX.Y.Z` 分支
2. 更新 `CHANGELOG.md`
3. 测试并修复问题
4. 合并到 `main` 并打标签
5. 发布到 GitHub Releases

## 许可证

通过提交代码，你同意你的贡献将使用 [AGPL-3.0](LICENSE) 许可证。

## 获取帮助

- 📖 阅读 [开发指南](docs/guides/development.md)
- 💬 加入 [Telegram 群组](https://t.me/TwilightPanelChat)
- 🐛 提交 [GitHub Issue](https://github.com/Prejudice-Studio/Twilight/issues)

## 致谢

感谢所有为 Twilight 做出贡献的开发者！

你的贡献将出现在 [贡献者列表](https://github.com/Prejudice-Studio/Twilight/graphs/contributors) 中。
