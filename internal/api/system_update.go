package api

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var gitBranchPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,128}$`)
var systemdServicePattern = regexp.MustCompile(`^twilight(-[a-z0-9]+)?$`)

func validateUpdateRepoURL(repoURL string) (string, error) {
	raw := strings.TrimSpace(repoURL)
	if raw == "" {
		return "", fmt.Errorf("missing Git repository URL")
	}
	if strings.ContainsAny(raw, " \t\r\n") || strings.ContainsFunc(raw, func(r rune) bool { return r < 32 }) {
		return "", fmt.Errorf("Git repository URL contains invalid characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" {
		return "", fmt.Errorf("only complete https Git repository URLs are supported")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("Git repository URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Git repository URL must not contain query strings or fragments")
	}
	return raw, nil
}

func validateUpdateBranch(branch string) (string, error) {
	value := strings.TrimSpace(firstNonEmpty(branch, "main"))
	if !gitBranchPattern.MatchString(value) {
		return "", fmt.Errorf("branch contains invalid characters")
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, ".") ||
		strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") ||
		strings.Contains(value, "..") || strings.Contains(value, "//") || strings.Contains(value, "@{") {
		return "", fmt.Errorf("branch format is invalid")
	}
	return value, nil
}

func applyGitUpdate(ctx context.Context, repoURL, branch string, restartServices bool, dryRun bool, allowDirty bool) map[string]any {
	_ = allowDirty
	repoURL, err := validateUpdateRepoURL(repoURL)
	if err != nil {
		return map[string]any{"success": false, "message": err.Error(), "error_code": ErrUpdateRepoInvalid, "code": 400, "results": []any{}}
	}
	branch, err = validateUpdateBranch(branch)
	if err != nil {
		return map[string]any{"success": false, "message": err.Error(), "error_code": ErrUpdateBranchInvalid, "code": 400, "results": []any{}}
	}
	projectRoot, err := os.Getwd()
	if err != nil {
		return map[string]any{"success": false, "message": err.Error(), "error_code": ErrInternal, "code": 500, "results": []any{}}
	}
	if _, err := os.Stat(filepathJoin(projectRoot, ".git")); err != nil {
		return map[string]any{"success": false, "message": "当前目录不是 Git 仓库", "error_code": ErrUpdateNotGitRepo, "code": 400, "project_root": projectRoot, "results": []any{}}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return map[string]any{"success": false, "message": "未找到 git 可执行文件", "error_code": ErrUpdateGitMissing, "code": 500, "project_root": projectRoot, "results": []any{}}
	}
	before, err := gitRepositoryState(ctx, projectRoot)
	if err != nil {
		return map[string]any{"success": false, "message": "无法读取 Git 仓库状态", "error_code": ErrUpdateInspectFailed, "code": 500, "project_root": projectRoot, "error": err.Error(), "results": []any{}}
	}
	dirtyBefore := boolish(before["dirty"])
	if dryRun {
		message := "update preflight passed"
		if dirtyBefore {
			message = "update preflight passed; local changes will be stashed before update"
		}
		return map[string]any{
			"success":           true,
			"message":           message,
			"code":              200,
			"project_root":      projectRoot,
			"repo_url":          redactGitURL(repoURL),
			"branch":            branch,
			"dry_run":           true,
			"restart_available": commandExists("systemctl"),
			"dirty_before":      dirtyBefore,
			"stash_created":     false,
			"stash_restored":    false,
			"stash_conflicts":   []string{},
			"before":            before,
			"results":           []any{},
		}
	}

	results := make([]map[string]any, 0, 6)
	stashCreated := false
	stashRestored := false
	stashConflicts := []string{}
	if dirtyBefore {
		stashMessage := "twilight-auto-update-" + strconv.FormatInt(time.Now().Unix(), 10)
		stash := runUpdateCommand(ctx, projectRoot, []string{"git", "stash", "push", "--include-untracked", "-m", stashMessage}, 120*time.Second)
		stash["command"] = "git stash push --include-untracked -m " + stashMessage
		results = append(results, stash)
		if code, _ := stash["returncode"].(int); code != 0 {
			return map[string]any{"success": false, "message": "暂存本地修改失败，自动更新已中止", "error_code": ErrUpdateGitFailed, "code": 500, "project_root": projectRoot, "repo_url": redactGitURL(repoURL), "branch": branch, "dry_run": false, "dirty_before": dirtyBefore, "stash_created": false, "stash_restored": false, "stash_conflicts": stashConflicts, "before": before, "results": results}
		}
		stashCreated = true
	}

	commands := [][]string{
		{"git", "remote", "set-url", "origin", repoURL},
		{"git", "fetch", "--prune", "origin", branch},
		{"git", "checkout", branch},
		{"git", "pull", "--ff-only", "origin", branch},
	}
	for _, command := range commands {
		result := runUpdateCommand(ctx, projectRoot, command, 120*time.Second)
		result["command"] = redactedUpdateCommand(command)
		results = append(results, result)
		if code, _ := result["returncode"].(int); code != 0 {
			if stashCreated && !stashRestored {
				restore := runUpdateCommand(ctx, projectRoot, []string{"git", "stash", "pop"}, 120*time.Second)
				restore["command"] = "git stash pop"
				results = append(results, restore)
				stashRestored = restore["returncode"] == 0
				if state, stateErr := gitRepositoryState(ctx, projectRoot); stateErr == nil && boolish(state["dirty"]) {
					stashConflicts = stringSlice(state["dirty_files"])
				}
			}
			return map[string]any{"success": false, "message": "自动更新失败", "error_code": ErrUpdateGitFailed, "code": 500, "project_root": projectRoot, "repo_url": redactGitURL(repoURL), "branch": branch, "dry_run": false, "dirty_before": dirtyBefore, "stash_created": stashCreated, "stash_restored": stashRestored, "stash_conflicts": stashConflicts, "before": before, "results": results}
		}
	}

	if stashCreated {
		restore := runUpdateCommand(ctx, projectRoot, []string{"git", "stash", "pop"}, 120*time.Second)
		restore["command"] = "git stash pop"
		results = append(results, restore)
		stashRestored = restore["returncode"] == 0
		if !stashRestored {
			if state, stateErr := gitRepositoryState(ctx, projectRoot); stateErr == nil && boolish(state["dirty"]) {
				stashConflicts = stringSlice(state["dirty_files"])
			}
			return map[string]any{"success": false, "message": "更新已拉取，但本地改动恢复失败", "error_code": ErrUpdateGitFailed, "code": 409, "project_root": projectRoot, "repo_url": redactGitURL(repoURL), "branch": branch, "dry_run": false, "dirty_before": dirtyBefore, "stash_created": stashCreated, "stash_restored": false, "stash_conflicts": stashConflicts, "before": before, "results": results}
		}
	}

	after, stateErr := gitRepositoryState(ctx, projectRoot)
	services := []string{"twilight", "twilight-bot", "twilight-scheduler"}
	restartScheduled := false
	restartMethod := ""
	updated := false
	message := "代码已更新"
	if stateErr == nil {
		updated = asString(before["commit"]) != asString(after["commit"])
	}
	if restartServices {
		if !updated {
			message = "代码已是最新，已跳过服务重启"
		} else if commandExists("systemctl") {
			restartScheduled, restartMethod = scheduleSystemdRestart(services)
			if restartScheduled {
				message = "代码已更新，服务将在稍后重启"
			} else {
				message = "代码已更新，但服务重启调度失败"
			}
		} else {
			message = "代码已更新，但未找到 systemctl"
		}
	}
	outServices := []string{}
	if restartScheduled {
		outServices = services
	}
	response := map[string]any{"success": true, "message": message, "code": 200, "project_root": projectRoot, "repo_url": redactGitURL(repoURL), "branch": branch, "dry_run": false, "updated": updated, "restart_requested": restartServices, "restart_scheduled": restartScheduled, "restart_method": restartMethod, "restart_available": commandExists("systemctl"), "services": outServices, "dirty_before": dirtyBefore, "stash_created": stashCreated, "stash_restored": stashRestored, "stash_conflicts": stashConflicts, "before": before, "results": results}
	if stateErr != nil {
		response["after_error"] = stateErr.Error()
	} else {
		response["after"] = after
	}
	return response
}
func runUpdateCommand(ctx context.Context, cwd string, args []string, timeout time.Duration) map[string]any {
	started := time.Now()
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, args[0], args[1:]...)
	cmd.Dir = cwd
	stdout, stderr := strings.Builder{}, strings.Builder{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		code = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
		if cmdCtx.Err() == context.DeadlineExceeded {
			stderr.WriteString("\ncommand timed out")
		}
	}
	return map[string]any{"command": strings.Join(args, " "), "returncode": code, "stdout": tailString(redactSensitiveText(stdout.String()), 8000), "stderr": tailString(redactSensitiveText(stderr.String()), 8000), "duration_ms": time.Since(started).Milliseconds()}
}

func gitRepositoryState(ctx context.Context, cwd string) (map[string]any, error) {
	branch, _, err := runGitOutput(ctx, cwd, 15*time.Second, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	commit, _, err := runGitOutput(ctx, cwd, 15*time.Second, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	remote, _, _ := runGitOutput(ctx, cwd, 15*time.Second, "remote", "get-url", "origin")
	status, _, err := runGitOutput(ctx, cwd, 15*time.Second, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	files := nonEmptyLines(status)
	return map[string]any{
		"branch":      strings.TrimSpace(branch),
		"commit":      strings.TrimSpace(commit),
		"remote_url":  redactGitURL(strings.TrimSpace(remote)),
		"dirty":       len(files) > 0,
		"dirty_files": limitStrings(files, 50),
		"dirty_count": len(files),
	}, nil
}

func runGitOutput(ctx context.Context, cwd string, timeout time.Duration, args ...string) (string, string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = cwd
	stdout, stderr := strings.Builder{}, strings.Builder{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if cmdCtx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String(), cmdCtx.Err()
	}
	if err != nil {
		if stderr.Len() > 0 {
			return stdout.String(), stderr.String(), fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), nil
}

func redactedUpdateCommand(args []string) string {
	if len(args) >= 5 && args[0] == "git" && args[1] == "remote" && args[2] == "set-url" {
		redacted := append([]string{}, args...)
		redacted[4] = redactGitURL(redacted[4])
		return strings.Join(redacted, " ")
	}
	return strings.Join(args, " ")
}

func redactGitURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func scheduleSystemdRestart(services []string) (bool, string) {
	for _, service := range services {
		if !systemdServicePattern.MatchString(service) {
			return false, ""
		}
	}
	args := append([]string{"restart"}, services...)
	if commandExists("systemd-run") {
		unit := "twilight-delayed-restart-" + strconv.FormatInt(time.Now().Unix(), 10)
		runArgs := append([]string{"--unit", unit, "--on-active=2", "--collect", "systemctl"}, args...)
		if err := exec.Command("systemd-run", runArgs...).Start(); err == nil {
			return true, "systemd-run"
		}
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		_ = exec.Command("systemctl", args...).Start()
	}()
	return true, "background-systemctl"
}

func tailString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func filepathJoin(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out = strings.TrimRight(out, `/\`) + string(os.PathSeparator) + strings.TrimLeft(part, `/\`)
	}
	return out
}
