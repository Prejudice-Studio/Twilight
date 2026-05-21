"""Git-based online update helpers."""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import threading
import time
from pathlib import Path
from urllib.parse import urlparse


PROJECT_ROOT = Path(__file__).resolve().parents[2]
GIT_BRANCH_RE = re.compile(r"^[A-Za-z0-9._/-]{1,128}$")


def validate_update_repo_url(repo_url: str) -> tuple[bool, str]:
    raw = (repo_url or "").strip()
    if not raw:
        return False, "缺少 Git 仓库地址"
    if any(ch.isspace() for ch in raw) or any(ord(ch) < 32 for ch in raw):
        return False, "Git 仓库地址包含非法字符"
    parsed = urlparse(raw)
    if parsed.scheme != "https":
        return False, "仅支持 https Git 仓库地址"
    if not parsed.netloc:
        return False, "Git 仓库地址格式不正确"
    if parsed.username or parsed.password:
        return False, "Git 仓库地址不能包含用户名或密码"
    if not parsed.path or parsed.path in {"/", ""}:
        return False, "Git 仓库地址缺少路径"
    return True, ""


def validate_update_branch(branch: str) -> tuple[bool, str]:
    value = (branch or "main").strip()
    if not GIT_BRANCH_RE.fullmatch(value):
        return False, "分支名只能包含字母、数字、点、下划线、斜杠和短横线"
    if value.startswith(("-", "/", ".")) or value.endswith(("/", ".")):
        return False, "分支名格式不正确"
    if ".." in value or "//" in value or "@{" in value:
        return False, "分支名格式不正确"
    return True, ""


def run_command(args: list[str], timeout: int = 120) -> dict:
    started = time.time()
    proc = subprocess.run(
        args,
        cwd=str(PROJECT_ROOT),
        text=True,
        capture_output=True,
        timeout=timeout,
        shell=False,
    )
    return {
        "command": " ".join(args),
        "returncode": proc.returncode,
        "stdout": (proc.stdout or "")[-8000:],
        "stderr": (proc.stderr or "")[-8000:],
        "duration_ms": int((time.time() - started) * 1000),
    }


def schedule_systemd_restart(services: list[str], delay: float = 1.5) -> None:
    def _restart():
        time.sleep(max(0.1, delay))
        subprocess.Popen(
            ["systemctl", "restart", *services],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )

    threading.Thread(target=_restart, daemon=True, name="twilight-systemd-restart").start()


def apply_git_update(repo_url: str, branch: str = "main", *, restart_services: bool = True) -> dict:
    ok, msg = validate_update_repo_url(repo_url)
    if not ok:
        return {"success": False, "message": msg, "code": 400, "results": []}
    ok, msg = validate_update_branch(branch)
    if not ok:
        return {"success": False, "message": msg, "code": 400, "results": []}
    if not (PROJECT_ROOT / ".git").is_dir():
        return {"success": False, "message": f"当前目录不是 Git 仓库: {PROJECT_ROOT}", "code": 400, "results": []}
    if shutil.which("git") is None:
        return {"success": False, "message": "服务器未安装 git", "code": 500, "results": []}

    commands: list[list[str]] = [
        ["git", "remote", "set-url", "origin", repo_url],
        ["git", "fetch", "--prune", "origin", branch],
        ["git", "checkout", branch],
        ["git", "pull", "--ff-only", "origin", branch],
    ]

    results = []
    try:
        for command in commands:
            result = run_command(command, timeout=120)
            results.append(result)
            if result["returncode"] != 0:
                return {"success": False, "message": "自动更新失败，请查看返回日志", "code": 500, "results": results}
    except subprocess.TimeoutExpired as exc:
        return {"success": False, "message": f"自动更新命令超时: {' '.join(exc.cmd)}", "code": 500, "results": results}
    except Exception as exc:  # pragma: no cover
        return {"success": False, "message": f"自动更新异常: {exc}", "code": 500, "results": results}

    services = ["twilight", "twilight-bot", "twilight-scheduler"]
    restart_scheduled = False
    message = "代码已更新"
    if restart_services:
        if os.name == "nt":
            message = "代码已更新；当前不是 Linux/systemd 环境，未自动重启服务"
        elif shutil.which("systemctl") is None:
            message = "代码已更新；未找到 systemctl，未自动重启服务"
        else:
            schedule_systemd_restart(services, delay=1.5)
            restart_scheduled = True
            message = "代码已更新，服务即将重启"

    return {
        "success": True,
        "message": message,
        "code": 200,
        "project_root": str(PROJECT_ROOT),
        "repo_url": repo_url,
        "branch": branch,
        "restart_scheduled": restart_scheduled,
        "services": services if restart_scheduled else [],
        "results": results,
    }
