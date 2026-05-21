# 开发者开发流程

本文档规定 Twilight 的日常开发、验证、Git 分支和发布流程。默认协作模式：开发在 `dev` 分支完成，仓库维护者手动 push 并合并到 `main`。

## 分支规则

| 分支 | 用途 | 权限 |
| ---- | ---- | ---- |
| `main` | 稳定分支，面向部署和发布。 | 只允许维护者合并。 |
| `dev` | 日常开发集成分支。 | 开发者在此提交变更。 |
| `feature/*` | 可选，复杂功能拆分开发。 | 完成后合并回 `dev`。 |
| `hotfix/*` | 可选，紧急修复。 | 修复后同时回合 `main` 与 `dev`。 |

当前工作流要求：不要直接 push 到 `main`。开发者完成工作后告知维护者，由维护者手动检查、push、合并。

## 首次准备

```bash
git fetch origin
git switch main
git pull --ff-only origin main
git switch -c dev
```

如果 `dev` 已存在：

```bash
git fetch origin
git switch dev
git merge --ff-only origin/main
```

## 日常开发流程

1. 确认当前分支：`git status --short --branch`。
2. 确认在 `dev` 分支：`git switch dev`。
3. 开发前拉取最新 `main`：`git fetch origin` 后由维护者或开发者确认是否需要合并。
4. 修改代码时遵循最小正确改动，避免无关重构。
5. 涉及接口、配置、安全、部署、前端行为时同步更新 `docs/`。
6. 涉及版本发布时更新 `docs/VERSION_HISTORY.md` 和版本号文件。
7. 本地运行验证命令。
8. 检查 diff，确认没有密钥、数据库、上传文件或本地配置误提交。
9. 通知维护者检查并执行 push/合并。

## 推荐验证命令

后端语法检查：

```bash
python -m py_compile main.py asgi.py
```

修改指定后端文件时可补充：

```bash
python -m py_compile src/api/v1/users.py src/api/v1/admin.py src/services/user_service.py
```

前端 lint：

```bash
cd webui
npm run lint
```

如项目后续补齐测试目录，优先运行：

```bash
pytest
```

## 修改 API 的规则

- 新增接口必须登记到 `docs/API_INDEX.md`。
- 需要请求体、响应体、安全说明、限流说明的接口必须更新 `docs/BACKEND_API.md`。
- API Key 专用接口必须更新 `docs/API_KEY_API.md`。
- 公开接口必须明确限流和信息泄露风险。
- 管理接口默认必须有登录鉴权和管理员鉴权。

## 修改配置的规则

配置项必须同时更新：

1. `src/config.py` 中的配置类默认值。
2. `src/api/v1/system.py` 的配置 schema。
3. `config.toml` 和 `config.production.toml` 示例。
4. 需要迁移旧字段时，在配置 sweep 迁移规则里登记。
5. 需要固定枚举时，前端使用单选，后端保存接口校验预设值。

## 修改前端的规则

- 优先复用 `webui/src/components/ui/` 的基础组件。
- 保持现有布局和视觉语言，不新增无关 UI 框架。
- API 调用集中放在 `webui/src/lib/api.ts`。
- 涉及认证态、系统配置、区域刷新时复用已有 store/hook。
- 新页面必须考虑桌面和移动端基本可用。

## 修改 TestWeb 演示的规则

- `/testweb`、`/testwebuser`、`/testwebadmin` 只能调用 `/api/v1/demo/*`。
- TestWeb 不读取真实登录态，不调用真实用户、管理员、Emby、注册、求片、配置接口。
- 需要新增演示数据时，先在 `src/api/v1/demo.py` 增加预设假数据，再让前端读取该字段。
- `src/api/v1/demo.py` 禁止导入 `src.db.*`、`src.services.*` 或读取 `g.current_user` / Cookie / Authorization。
- 演示按钮可以调用 `/demo/action/{action_name}`，但该接口必须忽略请求体，只返回 `simulated=true`，不得回显用户输入、写库或调用外部服务。

## 修改数据库的规则

- 现有数据库按模块拆分在 `src/db/*.py`。
- 新增字段要考虑旧数据默认值、空值、迁移和文档。
- 不要把 sqlite 数据库文件提交到仓库。
- 新增 `src/db/*.py` 后执行 `git check-ignore -v src/db/<file>.py`，避免被错误忽略。

## 安全检查

提交前必须确认：

- 没有真实 `bot_token`、`emby_token`、`api_key`、密码、Cookie secret。
- 没有上传用户资源、数据库、日志、备份文件。
- 公开接口有速率限制或明确说明不需要。
- 管理接口有 `require_auth` 和 `require_admin`。
- 文件上传和静态资源访问走受控接口，不直接暴露上传目录。

## 维护者合并流程

维护者在合并前执行：

```bash
git status --short --branch
git diff
git log --oneline -10
```

确认无误后：

```bash
git switch dev
git push origin dev
git switch main
git pull --ff-only origin main
git merge --no-ff dev
git push origin main
```

发布版本时：

```bash
git tag v0.0.1
git push origin v0.0.1
```

如果维护者使用 GitHub PR，则由 `dev` 创建 PR 到 `main`，检查通过后合并。

## 冲突处理

- 不使用 `git reset --hard` 清理他人改动。
- 不随意 `git checkout -- <file>` 回退文件。
- 冲突发生时先确认冲突来源，再由维护者决定保留哪边。
- 配置、数据库、锁文件、生成物冲突优先不提交，除非明确属于版本内容。

## 版本发布流程

1. 在 `dev` 更新版本号和 `docs/VERSION_HISTORY.md`。
2. 运行验证。
3. 维护者检查 diff。
4. 维护者 push `dev`。
5. 维护者合并到 `main`。
6. 维护者打 tag。
7. 发布更新说明。
