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
	repoURL, err := validateUpdateRepoURL(repoURL)
	if err != nil {
		return map[string]any{"success": false, "message": err.Error(), "code": 400, "results": []any{}}
	}
	branch, err = validateUpdateBranch(branch)
	if err != nil {
		return map[string]any{"success": false, "message": err.Error(), "code": 400, "results": []any{}}
	}
	projectRoot, err := os.Getwd()
	if err != nil {
		return map[string]any{"success": false, "message": err.Error(), "code": 500, "results": []any{}}
	}
	if _, err := os.Stat(filepathJoin(projectRoot, ".git")); err != nil {
		return map[string]any{"success": false, "message": "current directory is not a Git repository", "code": 400, "project_root": projectRoot, "results": []any{}}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return map[string]any{"success": false, "message": "git executable was not found", "code": 500, "project_root": projectRoot, "results": []any{}}
	}
	before, err := gitRepositoryState(ctx, projectRoot)
	if err != nil {
		return map[string]any{"success": false, "message": "failed to inspect Git repository", "code": 500, "project_root": projectRoot, "error": err.Error(), "results": []any{}}
	}
	if boolish(before["dirty"]) && !allowDirty {
		return map[string]any{
			"success":      false,
			"message":      "worktree has uncommitted changes; update refused",
			"code":         409,
			"project_root": projectRoot,
			"repo_url":     redactGitURL(repoURL),
			"branch":       branch,
			"dry_run":      dryRun,
			"before":       before,
			"results":      []any{},
		}
	}
	if dryRun {
		return map[string]any{
			"success":           true,
			"message":           "update preflight passed",
			"code":              200,
			"project_root":      projectRoot,
			"repo_url":          redactGitURL(repoURL),
			"branch":            branch,
			"dry_run":           true,
			"restart_available": commandExists("systemctl"),
			"before":            before,
			"results":           []any{},
		}
	}
	commands := [][]string{
		{"git", "remote", "set-url", "origin", repoURL},
		{"git", "fetch", "--prune", "origin", branch},
		{"git", "checkout", branch},
		{"git", "pull", "--ff-only", "origin", branch},
	}
	results := make([]map[string]any, 0, len(commands))
	for _, command := range commands {
		result := runUpdateCommand(ctx, projectRoot, command, 120*time.Second)
		result["command"] = redactedUpdateCommand(command)
		results = append(results, result)
		if code, _ := result["returncode"].(int); code != 0 {
			return map[string]any{"success": false, "message": "automatic update failed", "code": 500, "project_root": projectRoot, "repo_url": redactGitURL(repoURL), "branch": branch, "dry_run": false, "before": before, "results": results}
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
	response := map[string]any{"success": true, "message": message, "code": 200, "project_root": projectRoot, "repo_url": redactGitURL(repoURL), "branch": branch, "dry_run": false, "updated": updated, "restart_requested": restartServices, "restart_scheduled": restartScheduled, "restart_method": restartMethod, "restart_available": commandExists("systemctl"), "services": outServices, "before": before, "results": results}
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
