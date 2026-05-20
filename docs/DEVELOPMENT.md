# Twilight 开发指南

本文档面向想要参与 Twilight 开发的开发者。

## 目录

- [开发环境设置](#开发环境设置)
- [项目结构](#项目结构)
- [关键架构决策](#关键架构决策)
- [编码规范](#编码规范)
- [运行测试](#运行测试)
- [调试技巧](#调试技巧)
- [常见任务](#常见任务)
- [贡献流程](#贡献流程)

## 开发环境设置

### 安装开发依赖

```bash
# 激活虚拟环境
source venv/bin/activate  # Linux/macOS
# 或
.\venv\Scripts\Activate.ps1  # Windows PowerShell

# 安装生产和开发依赖
pip install -r requirements.txt -r requirements-dev.txt
```

### 配置 IDE

#### VS Code

推荐扩展：

- **Python** - ms-python.python
- **Pylance** - ms-python.vscode-pylance
- **Black Formatter** - ms-python.black-formatter
- **Flake8** - ms-python.flake8
- **MyPy** - ms-python.mypy-type-checker

创建 `.vscode/settings.json`：

```json
{
  "python.linting.enabled": true,
  "python.linting.flake8Enabled": true,
  "python.formatting.provider": "black",
  "[python]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": "explicit"
    }
  }
}
```

#### PyCharm

设置 Python 解释器为虚拟环境，启用 Black 和 Flake8 集成。

## 项目结构

```text
Twilight/
├── src/
│   ├── api/              # API 模块
│   │   ├── v1/          # API v1 接口（auth, users, apikey, media, emby, admin, ...）
│   │   └── swagger_template.py
│   ├── bot/              # Telegram Bot
│   ├── core/             # 核心工具
│   ├── db/               # 数据库模块（ORM 模型 + 数据访问）
│   ├── services/         # 业务逻辑服务（emby, bangumi, scheduler, ...）
│   └── schemas/          # 数据模型
├── tests/                # 可选：单元测试目录（按需补充）
├── docs/                 # 文档
├── uploads/              # 用户上传文件（背景图片等）
├── webui/                # Next.js 前端
├── main.py               # 应用入口
├── asgi.py               # ASGI 入口（生产）
├── config.toml           # 配置文件
├── requirements.txt      # 生产依赖
├── requirements-dev.txt  # 开发依赖
└── dev.ps1 / Makefile    # 开发辅助脚本
```

### 关键目录说明

- **src/api/v1/** - REST API 接口实现，按功能分为多个蓝图
- **src/services/** - 业务逻辑层，包含 Emby、Bangumi、调度等服务
- **src/db/** - 数据库操作层，包含 ORM 模型和数据访问对象
- **config_backups/** - 配置变更自动生成的轮转备份（已被 `.gitignore` 忽略；保留份数受 `TWILIGHT_CONFIG_BACKUP_RETENTION` 控制）
- **tests/** - 可选测试目录（按需创建与维护）

## 关键架构决策

以下决策是经过线上验证的，**修改前请先与维护者讨论**。

### 1. Emby 媒体库访问策略（`apply_library_policy`）

`src/services/emby.py::EmbyClient.apply_library_policy` 是用户媒体库可见性的**唯一**写入入口。
所有需要变更媒体库访问的逻辑（`sync_user_to_emby`、管理员 `set_user_library_access`）
都必须通过它。

实现要点：

- 单次 `POST /Users/{Id}/Policy`（替换式写入），**不允许**拆成多步
- 同时写入 `EnableAllFolders=False` + `EnabledFolders=[GUID 列表]` + `BlockedMediaFolders=[名称列表]`
  - `EnabledFolders` 取 `/Library/VirtualFolders` 返回项的 `ItemId` / `Guid`
  - `BlockedMediaFolders` 用库**名称**（不是 GUID）
  - 两者必须同时正确：依赖任一字段都可能因 Emby 版本差异导致用户被锁
- 排除法逐库决策：未在 enable/disable 名单的库由 `default_enable` 参数决定
  - `default_enable=True`：用户自助修改时未触及的库保持可见
  - `default_enable=False`：管理员严格白名单，未授权的库不可见

参考实现：[Sakura_embyboss](https://github.com/berry8838/Sakura_embyboss) `update_user_enabled_folder`。

### 2. 配置变更走整进程重启（不再热重载）

`src/api/v1/system.py::_schedule_process_restart` 在保存 `config.toml` 后向进程组发 SIGTERM，
再 `os._exit(0)` 兜底。**禁止**再为 config 改动新增"热重载"逻辑。

部署侧要求：必须由 systemd / docker / `start_backend_prod.sh` 外层守护脚本拉起，否则进程退出后无法自启。

### 3. Bot 连通性测试不复用全局 Bot

`/system/admin/bot/test` 使用独立 `httpx.AsyncClient` 直接调用 Telegram Bot HTTP API，
**不要**调用 `bot.send_message`——否则会触发跨事件循环异常。

### 4. TG Bot 不展示服务器线路/URL

Emby 面板（`src/bot/handlers/emby_handlers.py`）只保留播放统计 / 密码说明 / 主菜单。
新增 Bot 功能时不应直接泄露 Emby URL；如需暴露，应引导到网页端。

### 5. `.gitignore` 中 `/db/` 必须有前导斜杠

旧版 `db/` 模式会同时匹配根目录 `db/`（sqlite）与 `src/db/`（Python 包），
导致新增 `src/db/*.py` 模块（如 `apikey.py`）被静默忽略。
新增 `src/db/*.py` 后用 `git check-ignore -v <path>` 验证一次。

### 6. 定时任务必须用 `RunContext` 记录 summary / 日志

`src/services/scheduler_service.py::SchedulerService._run_with_tracking` 是所有
定时任务的统一入口（APScheduler 调度、管理员手动触发、进程启动时的 `daily_stats`
都走这里）。它会：

1. 在 `db/scheduler_run.db` 写一条 `STATUS=running` 的记录；
2. 把 `RunContext` 实例传给 job 函数；
3. job 结束后回填 `STATUS / FINISHED_AT / ERROR / SUMMARY / LOGS`，并按
   `SchedulerRunOperate.HISTORY_LIMIT_PER_JOB`（默认 50）裁剪历史；
4. 进程启动时调用 `reconcile_orphans()`，把 6 小时前仍 `running` 的残留行改判
   `failed`，避免前端永远转圈。

新增 job 时**必须**遵循下面的签名 / 约定：

```python
@staticmethod
async def my_job(ctx: RunContext):
    ctx.log("开始处理…")               # 同时写到 logger 和落库的 LOGS 字段
    ctx.summary["scanned"] = total
    ctx.summary["disabled"] = ok
    # 不要直接调用 logger.info 来报告统计——前端拿不到
```

并在 `JOB_DEFINITIONS` / `_resolve_job` 中注册，否则管理后台看不到、无法手动触发。
新 job 的 `JOB_DEFINITIONS` 条目里 `default_trigger` 字段决定首次启动时的触发规则
（`cron_daily` 用 `config_field` 指向 `SchedulerConfig` / `TelegramConfig` 的 HH:MM
字符串，`interval` 用 `config_field + unit` 指向数值），管理员后续可以通过
`PUT /admin/scheduler/jobs/<id>/schedule` 写入覆盖到 `db/scheduler_schedule.db`，
覆盖在进程重启后仍生效；要恢复默认走 `DELETE` 同一端点。
前端展示的中文标签在 `webui/src/app/(main)/admin/scheduler/page.tsx` 的
`SUMMARY_LABELS` 字典里维护，新增 summary 键时同步补一行翻译。

### 7. Emby/媒体接口默认走鉴权

`/api/v1/emby/*` 与 `/api/v1/media/search*`、`/api/v1/media/inventory/*` 全部
需要 `@require_auth`。线路下发**只能**走 `GET /system/emby-urls`（按角色和 Emby
绑定状态过滤）；旧的 `GET /emby/urls` 现在固定返回 `410 Gone`。新增 Emby 相关
端点时默认加 `@require_auth`，需要对外开放的必须明确写出理由并经过 review。

### 8. 背景图片 URL 验证支持 CSS 包装

`src/api/v1/users.py::_is_valid_background_url` 同时接受：
- 受控站内资源 `/api/v1/users/assets/...`（兼容历史 `/uploads/...`，读取时会改写）
- 裸 `http(s)://` URL
- `url("...")` / `url('...')` CSS 包装
- `linear-gradient(...)` / `radial-gradient(...)` / `conic-gradient(...)` 等渐变函数

前端 `webui/src/app/(main)/layout.tsx::normalizeBgImageValue` 会把裸 URL 包装成 `url("...")`，
后端必须能识别这种包装形式。

### 9. 配置文件自动整理（sweep）

`src/config.py::sweep_config_toml` 在每次进程启动时自动跑一遍：

1. **迁移**：把已废弃的 section/key（如 `[Signin].enabled` → `[SAR].signin_enabled`）搬到新归属
2. **裁剪**：删除任何没有被 `BaseConfig` 子类声明的孤立 section / key
3. **补齐**：把代码里声明但 toml 没写的字段按默认值补上
4. **备份**：若有变更，写回前调 `backup_config_file()` 落档（详见 §10）

新增配置项的步骤：

1. 在对应 `XxxConfig` 类里加字段 + 默认值 + 注释
2. 顶部 `config.toml` / `config.production.toml` 模板加注释 + 默认值（不必须，sweep 会自动补，但建议手工补让示例完整）
3. 在 `src/api/v1/system.py` 的 schema 端点对应 section 里加 `{'key', 'label', 'type', 'description', 'value'}` 一行（前端会自动渲染为表单字段）
4. 不要为废弃字段写"兼容读取"——直接在 `_LEGACY_SECTION_KEY_MIGRATIONS` 注册搬迁规则

### 10. 配置文件备份

`src/config.py::backup_config_file` 是统一的备份入口：

- 写到 `config_backups/<filename>.<YYYYmmdd-HHMMSS>.<reason>.bak`，权限收紧 0o600
- 同时维护一份 `config.toml.backup` 单文件副本（兼容旧手动恢复脚本）
- 保留份数：默认 20，环境变量 `TWILIGHT_CONFIG_BACKUP_RETENTION` 覆盖；超过按 mtime 淘汰

调用方：

- 启动期 sweep（`reason='sweep'`）
- `fill_missing_config_items`（`reason='fill-missing'`）
- 管理员 PUT toml / schema 接口（`reason='manual'`）

`config_backups/` 已加入 `.gitignore`，备份文件**含敏感字段**，请勿提交。

### 11. API 速率限制

`src/core/utils.py::rate_limit_check(namespace, key, *, max_requests, window_seconds)` 是统一限流器：

- 单进程内存的滑动窗口，进程重启清零
- 失败返回 `(False, retry_after_seconds)`，业务侧返回 `HTTP 429`
- 维度任选：IP / UID / 业务 key（如绑定码、request_id）；多维度叠加用不同 `namespace`

具体已启用的端点见 [BACKEND_API.md §3.1](./BACKEND_API.md#31-速率限制)。
扩展时优先选择"防滥用价值高 + 误伤合法用户成本低"的端点，避免每个 GET 都套限速。

### 12. Telegram 群成员花名册 + 退群封禁

- 花名册（`src/db/telegram_roster.py::TelegramGroupRosterModel`）依赖被动观察：`chat_member` 事件 + 群消息 + 用户主动 `/bind` 时的成员探测。`TelegramMembershipService.check_user_in_groups(..., sync_roster=True)` 会在每次绑定时把当前成员状态同步进表，弥补"从未发言用户"的盲区。
- `Telegram.ban_on_leave=True` 时，scheduler 巡检 (`enforce_group_membership`) 发现退群用户会调 `TelegramMembershipService.ban_user_permanently()` 在所有 `GROUP_ID` 群里永久 ban（不 unban）。永封模式下"重新入群识别"分支会被跳过。
- 误判 = 不可逆，运维风险高，默认关闭。详见 [SECURITY.md §3](./SECURITY.md#3-telegram-相关安全)。

## 编码规范

### Python 代码风格

遵循 PEP 8，使用 Black 格式化：

```bash
# 格式化单个文件
black src/api/v1/auth.py

# 格式化整个项目
black src/

# 检查风格（不修改）
flake8 src/
```

### 命名约定

- **类名**: `PascalCase` - `UserModel`, `AuthService`
- **函数名**: `snake_case` - `get_user_info`, `verify_password`
- **常量**: `UPPER_SNAKE_CASE` - `MAX_RETRY`, `TOKEN_EXPIRE`
- **私有方法**: `_snake_case` - `_verify_signature`

### 类型注解

所有公开函数都应使用类型注解：

```python
from typing import Optional, List, Dict, Any

async def get_user_by_uid(uid: int) -> Optional[UserModel]:
    """根据 UID 获取用户"""
    pass

async def get_users(limit: int = 100, offset: int = 0) -> tuple[List[UserModel], int]:
    """分页获取用户列表，返回 (用户列表, 总数)"""
    pass
```

### 文档字符串

使用 Google 风格的文档字符串：

```python
async def create_user(username: str, email: str) -> UserModel:
    """
    创建新用户
    
    Args:
        username: 用户名
        email: 邮箱地址
    
    Returns:
        创建的用户对象
    
    Raises:
        ValueError: 用户名已存在或邮箱格式错误
    """
    pass
```

### 错误处理

所有异步操作都应正确处理异常：

```python
try:
    result = await external_service.fetch_data()
except ConnectionError as e:
    logger.error(f"External service error: {e}")
    # 返回适当的错误响应
except Exception as e:
    logger.exception(f"Unexpected error: {e}")
    # 不要吞掉异常，除非有特殊原因
    raise
```

## 运行测试

当前仓库主要包含后端与前端代码，测试目录可根据项目需要逐步补充。

### 使用 pytest

```bash
# 运行所有测试
pytest -v

# 运行特定测试文件
pytest tests/test_system.py -v

# 运行特定测试函数
pytest tests/test_system.py::test_health_check -v

# 运行并显示打印输出
pytest -v -s
```

### 生成覆盖率报告

```bash
pytest --cov=src --cov-report=html
```

### 测试异步代码

确保测试函数使用 `async def`：

```python
import pytest

@pytest.mark.asyncio
async def test_async_function():
    result = await some_async_function()
    assert result is not None
```

## 调试技巧

### 启用日志

```python
import logging

logger = logging.getLogger(__name__)
logger.debug("Debug message")
logger.info("Info message")
logger.warning("Warning message")
logger.error("Error message")
```

### 使用 PDB 调试

```python
import pdb; pdb.set_trace()  # 在需要调试的地方添加

# 或使用 ipdb（更友好）
import ipdb; ipdb.set_trace()
```

### 性能分析

```bash
# 使用 scalene 进行性能分析
scalene --profile-interval 0.001 main.py api

# 导出为 HTML 报告
scalene --profile-interval 0.001 --html main.py api > profile.html
```

## 常见任务

### 添加新的 API 端点

1. 在 `src/api/v1/` 下创建或编辑蓝图文件
2. 定义异步路由处理器
3. 使用 `@require_auth` 或 `@require_admin` 装饰器进行认证
4. 返回 `api_response()` 格式化的响应
5. 在 `src/api/v1/__init__.py` 中注册蓝图

示例：

```python
# src/api/v1/example.py
from flask import Blueprint, request, g
from src.api.v1.auth import require_auth, api_response

example_bp = Blueprint('example', __name__, url_prefix='/example')

@example_bp.route('/test', methods=['GET'])
async def test_endpoint():
    """测试端点"""
    return api_response(True, "Success", {'data': 'test'})

@example_bp.route('/protected', methods=['POST'])
@require_auth
async def protected_endpoint():
    """需要认证的端点"""
    user_id = g.current_user.UID
    return api_response(True, "Protected access granted", {'user_id': user_id})
```

### 添加新的数据库模型

1. 在 `src/db/` 下创建模型文件
2. 定义 SQLAlchemy 模型类
3. 创建数据访问对象 (DAO) 类
4. 在 `src/db/__init__.py` 中导出

### 添加新的服务

1. 在 `src/services/` 下创建服务文件
2. 实现业务逻辑
3. 在 API 路由中调用服务

### 更新数据库模式

数据库模式变更目前是手动的。如要修改模式：

1. 编辑 `src/db/` 中的模型类
2. 删除旧的数据库文件（`db/*.db`）
3. 重新启动应用，自动创建新的数据库

在生产环境中，应实现迁移脚本。

## 贡献流程

### 提交 Pull Request

1. **Fork** 本项目
2. **创建分支** - `git checkout -b feature/your-feature`
3. **提交更改** - `git commit -am 'Add some feature'`
4. 确保代码格式化和测试通过
5. **推送分支** - `git push origin feature/your-feature`
6. **创建 Pull Request**

### 代码审查

所有 PR 需要通过代码审查。审查内容包括：

- 代码风格和质量
- 类型注解的正确性
- 测试覆盖率
- 文档的完整性
- 安全问题

### Commit 规范

使用简洁清晰的 Commit Message：

```text
feat: 添加新的认证方式
fix: 修复 token 过期检查的 bug
docs: 更新安装文档
refactor: 重构用户状态同步流程
test: 添加用户认证测试
perf: 优化数据库查询性能
```

## 常见问题

### 修改了 Config 但没有生效?

确保：

1. 重启应用
2. 正确设置环境变量前缀 `TWILIGHT_`
3. 检查 `config.toml` 中相应配置的优先级

### Redis 连接失败怎么办?

应用会自动回退到内存存储。在生产环境中：

```bash
# 使用 Docker 启动 Redis
docker run -d -p 6379:6379 redis:latest

# 配置环境变量
# Linux/macOS
export TWILIGHT_REDIS_URL=redis://localhost:6379/0
# Windows PowerShell
$env:TWILIGHT_REDIS_URL="redis://localhost:6379/0"
```

### 如何跳过某个测试?

```python
import pytest

@pytest.mark.skip(reason="还未实现")
def test_unimplemented():
    pass

@pytest.mark.skipif(not have_redis, reason="Redis 未安装")
async def test_redis_feature():
    pass
```

## 资源链接

- [Flask 官方文档](https://flask.palletsprojects.com/)
- [SQLAlchemy 文档](https://docs.sqlalchemy.org/)
- [Pytest 文档](https://pytest.org/)
- [PEP 8 风格指南](https://www.python.org/dev/peps/pep-0008/)
- [Black 格式化器](https://black.readthedocs.io/)
