.PHONY: help install install-dev clean test lint format type-check run dev prod docs

VENV := venv
PYTHON := $(VENV)/bin/python
PIP := $(VENV)/bin/pip

help:
	@echo "Twilight 开发命令:"
	@echo "  make install       - 安装生产依赖"
	@echo "  make install-dev   - 安装全部依赖（包括开发工具）"
	@echo "  make clean         - 清理缓存和临时文件"
	@echo "  make test          - 运行测试"
	@echo "  make test-cov      - 运行测试并生成覆盖率报告"
	@echo "  make lint          - 检查代码风格 (flake8)"
	@echo "  make format        - 格式化代码 (black)"
	@echo "  make type-check    - 类型检查 (mypy)"
	@echo "  make run           - 运行开发服务器"
	@echo "  make prod          - 运行生产服务器"
	@echo "  make docs          - 生成 HTML 文档"

install: $(VENV)
	$(PIP) install -r requirements.txt

install-dev: $(VENV)
	$(PIP) install -r requirements.txt -r requirements-dev.txt

$(VENV):
	python3 -m venv $(VENV)
	$(PIP) install --upgrade pip setuptools wheel

clean:
	find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
	find . -type d -name .pytest_cache -exec rm -rf {} + 2>/dev/null || true
	find . -type d -name htmlcov -exec rm -rf {} + 2>/dev/null || true
	find . -type d -name .mypy_cache -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete
	find . -type f -name "*.pyo" -delete
	find . -type f -name "*.egg-info" -delete

test:
	$(PYTHON) -m pytest tests/ -v

test-cov:
	$(PYTHON) -m pytest tests/ -v --cov=src --cov-report=html --cov-report=term-missing

lint:
	$(PYTHON) -m flake8 src/ tests/

format:
	$(PYTHON) -m black src/ tests/
	$(PYTHON) -m isort src/ tests/

type-check:
	$(PYTHON) -m mypy src/ --ignore-missing-imports || true

run:
	$(PYTHON) main.py api --debug

prod:
	$(PYTHON) -m uvicorn asgi:app --host 0.0.0.0 --port 5000 --workers 4

docs:
	cd docs && $(PYTHON) -m sphinx -b html . _build/html

dev-setup: install-dev format lint test
	@echo "✅ 开发环境设置完成！"
