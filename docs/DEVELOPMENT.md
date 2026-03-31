# Twilight 开发指南

本文档面向想要参与 Twilight 开发的开发者。

## 目录

- [开发环境设置](#开发环境设置)
- [项目结构](#项目结构)
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

```
Twilight/
├── src/
│   ├── api/              # API 模块
│   │   ├── v1/          # API v1 接口（auth, users, apikey, score, media, emby, admin, ...）
│   │   └── swagger_template.py
│   ├── bot/              # Telegram Bot
│   ├── core/             # 核心工具
│   ├── db/               # 数据库模块（ORM 模型 + 数据访问）
│   ├── services/         # 业务逻辑服务（emby, bangumi, score, scheduler, ...）
│   └── schemas/          # 数据模型
├── tests/                # 单元测试
├── docs/                 # 文档
├── uploads/              # 用户上传文件（背景图片等）
├── webui/                # Next.js 前端
├── main.py               # 应用入口
├── asgi.py               # ASGI 入口（生产）
├── migrate.py            # 数据库迁移脚本
├── config.toml           # 配置文件
├── requirements.txt      # 生产依赖
├── requirements-dev.txt  # 开发依赖
└── dev.ps1 / Makefile    # 开发辅助脚本
```

### 关键目录说明

- **src/api/v1/** - REST API 接口实现，按功能分为多个蓝图
- **src/services/** - 业务逻辑层，包含 Emby、Bangumi、积分等服务
- **src/db/** - 数据库操作层，包含 ORM 模型和数据访问对象
- **tests/** - 单元测试和集成测试

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

### 基本测试

```bash
# 运行所有测试
pytest tests/ -v

# 运行特定测试文件
pytest tests/test_system.py -v

# 运行特定测试函数
pytest tests/test_system.py::test_health_check -v

# 运行并显示打印输出
pytest tests/ -v -s
```

### 生成覆盖率报告

```bash
# 生成覆盖率报告
pytest tests/ --cov=src --cov-report=html

# 查看报告
# Linux/macOS
open htmlcov/index.html
# Windows PowerShell
start htmlcov/index.html
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

```
feat: 添加新的认证方式
fix: 修复 token 过期检查的 bug
docs: 更新安装文档
refactor: 重构积分系统
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
