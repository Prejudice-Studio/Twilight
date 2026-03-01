# Twilight 开发辅助脚本（Windows PowerShell）
# 使用: .\dev.ps1 -Task install

param(
    [Parameter(Mandatory=$false)]
    [string]$Task = "help",
    
    [Parameter(Mandatory=$false)]
    [switch]$Debug = $false
)

$ErrorActionPreference = "Stop"
$VENV_PATH = ".\venv"
$PYTHON = "$VENV_PATH\Scripts\python.exe"
$PIP = "$VENV_PATH\Scripts\pip.exe"

function Show-Help {
    Write-Host "
╔════════════════════════════════════════════════════════════════╗
║          Twilight 开发辅助脚本 (Windows PowerShell)            ║
╚════════════════════════════════════════════════════════════════╝

用法: .\dev.ps1 -Task <任务名>

可用任务:
  help          - 显示此帮助信息
  init          - 初始化开发环境（创建虚拟环境）
  install       - 安装生产依赖
  install-dev   - 安装全部依赖（包括开发工具）
  clean         - 清理缓存和临时文件
  test          - 运行单元测试
  test-cov      - 运行测试后生成覆盖率报告
  lint          - 检查代码风格 (flake8)
  format        - 格式化代码 (black + isort)
  type-check    - 类型检查 (mypy)
  run           - 运行开发服务器
  prod          - 运行生产服务器
  setup         - 完整设置（init -> install-dev -> format -> test）

示例:
  .\dev.ps1 -Task install
  .\dev.ps1 -Task test -Debug
    " -ForegroundColor Cyan
}

function Test-VenvExists {
    return Test-Path $VENV_PATH
}

function Init-Environment {
    Write-Host "🔧 初始化开发环境..." -ForegroundColor Yellow
    
    if (Test-VenvExists) {
        Write-Host "✓ 虚拟环境已存在" -ForegroundColor Green
    } else {
        Write-Host "📦 创建虚拟环境..." -ForegroundColor Cyan
        python -m venv $VENV_PATH
        if ($LASTEXITCODE -ne 0) {
            Write-Host "✗ 虚拟环境创建失败" -ForegroundColor Red
            exit 1
        }
    }
    
    Write-Host "📦 升级 pip, setuptools, wheel..." -ForegroundColor Cyan
    & $PYTHON -m pip install --upgrade pip setuptools wheel
    
    Write-Host "✅ 开发环境初始化完成" -ForegroundColor Green
}

function Install-Requirements {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在，请先运行: .\dev.ps1 -Task init" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "📦 安装生产依赖..." -ForegroundColor Cyan
    & $PIP install -r requirements.txt
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ 生产依赖安装完成" -ForegroundColor Green
    } else {
        Write-Host "✗ 依赖安装失败" -ForegroundColor Red
        exit 1
    }
}

function Install-Dev-Requirements {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在，请先运行: .\dev.ps1 -Task init" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "📦 安装全部依赖..." -ForegroundColor Cyan
    & $PIP install -r requirements.txt -r requirements-dev.txt
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ 全部依赖安装完成" -ForegroundColor Green
    } else {
        Write-Host "✗ 依赖安装失败" -ForegroundColor Red
        exit 1
    }
}

function Clean {
    Write-Host "🧹 清理缓存和临时文件..." -ForegroundColor Yellow
    
    Get-ChildItem -Path . -Include __pycache__ -Recurse -Directory | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    Get-ChildItem -Path . -Include .pytest_cache -Recurse -Directory | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    Get-ChildItem -Path . -Include htmlcov -Recurse -Directory | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    Get-ChildItem -Path . -Include *.pyc -Recurse -Force | Remove-Item -Force -ErrorAction SilentlyContinue
    
    Write-Host "✅ 清理完成" -ForegroundColor Green
}

function Run-Tests {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "🧪 运行单元测试..." -ForegroundColor Cyan
    & $PYTHON -m pytest tests/ -v $(if ($Debug) { "-s" })
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ 所有测试通过" -ForegroundColor Green
    } else {
        Write-Host "✗ 部分测试失败" -ForegroundColor Red
        exit 1
    }
}

function Run-Tests-With-Coverage {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "🧪 运行测试并生成覆盖率报告..." -ForegroundColor Cyan
    & $PYTHON -m pytest tests/ -v --cov=src --cov-report=html --cov-report=term-missing
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ 覆盖率报告已生成: htmlcov/index.html" -ForegroundColor Green
    } else {
        Write-Host "✗ 测试失败" -ForegroundColor Red
        exit 1
    }
}

function Run-Lint {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "🔍 检查代码风格..." -ForegroundColor Cyan
    & $PYTHON -m flake8 src/ tests/
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ 代码风格检查通过" -ForegroundColor Green
    } else {
        Write-Host "⚠️ 发现风格问题" -ForegroundColor Yellow
    }
}

function Run-Format {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "✨ 格式化代码..." -ForegroundColor Cyan
    & $PYTHON -m black src/ tests/
    & $PYTHON -m isort src/ tests/
    
    Write-Host "✅ 代码格式化完成" -ForegroundColor Green
}

function Run-Type-Check {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "📝 进行类型检查..." -ForegroundColor Cyan
    & $PYTHON -m mypy src/ --ignore-missing-imports
}

function Run-Dev-Server {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "🚀 启动开发服务器..." -ForegroundColor Cyan
    & $PYTHON main.py api --debug
}

function Run-Prod-Server {
    if (-not (Test-VenvExists)) {
        Write-Host "✗ 虚拟环境不存在" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "🚀 启动生产服务器..." -ForegroundColor Cyan
    & $PYTHON -m uvicorn asgi:app --host 0.0.0.0 --port 5000 --workers 4
}

function Complete-Setup {
    Write-Host "
╔════════════════════════════════════════════════════════════════╗
║             开始完整的开发环境设置...                          ║
╚════════════════════════════════════════════════════════════════╝
    " -ForegroundColor Cyan
    
    Init-Environment
    Install-Dev-Requirements
    Run-Format
    Run-Lint
    Run-Tests
    
    Write-Host "
╔════════════════════════════════════════════════════════════════╗
║                 ✅ 环境设置完成！                             ║
║                                                                ║
║  运行开发服务器: .\dev.ps1 -Task run                         ║
║  运行测试:      .\dev.ps1 -Task test                         ║
╚════════════════════════════════════════════════════════════════╝
    " -ForegroundColor Green
}

# 执行任务
switch ($Task.ToLower()) {
    "help" { Show-Help }
    "init" { Init-Environment }
    "install" { Install-Requirements }
    "install-dev" { Install-Dev-Requirements }
    "clean" { Clean }
    "test" { Run-Tests }
    "test-cov" { Run-Tests-With-Coverage }
    "lint" { Run-Lint }
    "format" { Run-Format }
    "type-check" { Run-Type-Check }
    "run" { Run-Dev-Server }
    "prod" { Run-Prod-Server }
    "setup" { Complete-Setup }
    default { 
        Write-Host "✗ 未知任务: $Task" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
