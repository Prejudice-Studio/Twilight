package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

var schedulerJobs = []map[string]any{
	{"id": "cleanup_bind_codes", "name": "清理过期绑定码", "description": "删除过期 Telegram 绑定码，避免重启后残留无效凭据。", "manual_only": false, "enabled": true},
	{"id": "check_expired", "name": "检查已过期用户", "description": "扫描已过期账号，并按规则禁用系统或 Emby 访问。", "manual_only": false, "enabled": true},
	{"id": "check_expiring", "name": "检查即将到期用户", "description": "统计近期即将到期的用户，供管理员确认续期风险。", "manual_only": false, "enabled": true},
	{"id": "expiry_reminders", "name": "发送到期提醒", "description": "向即将到期且已绑定 Telegram 的用户发送续期提醒。", "manual_only": false, "enabled": true},
	{"id": "daily_stats", "name": "每日统计", "description": "生成每日用户与活跃状态汇总。", "manual_only": false, "enabled": true},
	{"id": "cleanup_sessions", "name": "会话巡检", "description": "读取 Emby 当前会话，统计活跃播放情况。", "manual_only": false, "enabled": true},
	{"id": "emby_sync", "name": "同步 Emby 用户", "description": "同步本地用户与 Emby 用户名称、启用状态等信息。", "manual_only": true, "enabled": true},
	{"id": "cleanup_no_emby", "name": "清理无 Emby Web 账号", "description": "清理注册后长期没有绑定或注册 Emby、且没有 Emby 开通资格的 Web 账号。", "manual_only": false, "enabled": true},
	{"id": "cleanup_pending_emby_entitlements", "name": "清理未使用 Emby 开通资格", "description": "收回长期拥有 Emby 注册资格但尚未创建 Emby 的资格，不删除 Web 账号。", "manual_only": false, "enabled": true},
	{"id": "enforce_group_membership", "name": "Telegram 群成员校验", "description": "校验用户是否仍在要求加入的 Telegram 群组内，并按配置处理退群用户。", "manual_only": false, "enabled": true},
	{"id": "check_telegram_bindings", "name": "Telegram 绑定检查", "description": "检查重复或异常的 Telegram 绑定关系。", "manual_only": false, "enabled": true},
	{"id": "system_auto_update", "name": "系统自动更新", "description": "按配置拉取可信 Git 仓库更新，并可选择重启服务。", "manual_only": false, "enabled": false},
	{"id": "cleanup_unused_uploads", "name": "清理未使用上传文件", "description": "删除未被头像、背景或服务器图标引用的历史上传文件。", "manual_only": false, "enabled": true},
	{"id": "kick_unknown_group_members", "name": "踢出未知 Telegram 群成员", "description": "根据已观察到的群成员名册，踢出无系统账号、未绑定 Emby 或已禁用的成员。", "manual_only": true, "enabled": true, "runtime_params": []string{"dry_run", "max_per_run"}},
}

func (a *App) handleSchedulerJobs(w http.ResponseWriter, r *http.Request, _ Params) {
	jobs := make([]map[string]any, 0, len(schedulerJobs))
	for _, job := range schedulerJobs {
		item := cloneMap(job)
		jobID := fmt.Sprint(job["id"])
		var spec map[string]any
		if schedule, okSchedule := a.store.SchedulerSchedule(jobID); okSchedule {
			spec = schedule.TriggerSpec
			item["is_custom"] = schedule.IsCustom
			item["runtime_params"] = a.schedulerRuntimeParamsFromSchedule(jobID, schedule.RuntimeParams)
		} else {
			spec = a.schedulerDefaultTriggerSpec(jobID)
			item["is_custom"] = false
			item["runtime_params"] = a.schedulerDefaultRuntimeParams(jobID)
		}
		item["trigger_spec"] = spec
		item["default_trigger_spec"] = a.schedulerDefaultTriggerSpec(jobID)
		item["last_run"] = nil
		running := a.schedulerJobRunning(jobID)
		a.reconcileSchedulerRunState(jobID, running)
		item["next_run_at"] = zeroNil(a.schedulerNextRunAt(jobID, spec, time.Now()))
		item["auto_disabled"] = schedulerTriggerDisabled(spec)
		if runs := a.store.SchedulerRuns(jobID, 20); len(runs) > 0 {
			item["last_run"] = runs[0]
			var lastAuto, lastManual int64
			for _, run := range runs {
				if run.Type == "auto" && lastAuto == 0 {
					lastAuto = run.StartedAt
				}
				if run.Type == "manual" && lastManual == 0 {
					lastManual = run.StartedAt
				}
			}
			item["last_auto_run_at"] = zeroNil(lastAuto)
			item["last_manual_run_at"] = zeroNil(lastManual)
		}
		item["is_running"] = running
		jobs = append(jobs, item)
	}
	ok(w, "OK", map[string]any{"jobs": jobs})
}

func (a *App) handleSchedulerTerminate(w http.ResponseWriter, r *http.Request, params Params) {
	jobID := params["job_id"]
	if !schedulerJobExists(jobID) {
		failWithCode(w, http.StatusNotFound, ErrSchedulerJobNotFound, "调度任务不存在")
		return
	}
	if !a.terminateSchedulerJob(jobID) {
		a.reconcileSchedulerRunState(jobID, false)
		ok(w, "job is not running", map[string]any{"job_id": jobID, "terminated": false, "already_stopped": true})
		return
	}
	ok(w, "job termination requested", map[string]any{"job_id": jobID, "terminated": true})
}
func (a *App) handleSchedulerLastRun(w http.ResponseWriter, r *http.Request, params Params) {
	a.reconcileSchedulerRunState(params["job_id"], a.schedulerJobRunning(params["job_id"]))
	runs := a.store.SchedulerRuns(params["job_id"], 1)
	var last any
	if len(runs) > 0 {
		last = runs[0]
	}
	ok(w, "OK", map[string]any{"job_id": params["job_id"], "last_run": last})
}
func (a *App) handleSchedulerHistory(w http.ResponseWriter, r *http.Request, params Params) {
	a.reconcileSchedulerRunState(params["job_id"], a.schedulerJobRunning(params["job_id"]))
	runs := a.store.SchedulerRuns(params["job_id"], queryInt(r, "limit", 20))
	ok(w, "OK", map[string]any{"job_id": params["job_id"], "history": runs, "total": len(runs)})
}
func (a *App) handleSchedulerSchedule(w http.ResponseWriter, r *http.Request, params Params) {
	jobID := params["job_id"]
	if !schedulerJobExists(jobID) {
		failWithCode(w, http.StatusNotFound, ErrSchedulerJobNotFound, "调度任务不存在")
		return
	}
	if r.Method == http.MethodDelete {
		schedule, err := a.store.SetSchedulerSchedule(jobID, a.schedulerDefaultTriggerSpec(jobID), false)
		if statusFromError(w, err) {
			return
		}
		ok(w, "schedule reset", map[string]any{"job_id": jobID, "trigger_spec": schedule.TriggerSpec, "runtime_params": a.schedulerDefaultRuntimeParams(jobID), "is_custom": false})
		return
	}
	payload := decodeMap(r)
	spec := map[string]any{"type": firstNonEmpty(stringValue(payload, "type"), "interval")}
	if spec["type"] == "manual" {
		spec = map[string]any{"type": "manual"}
	} else if spec["type"] == "cron_daily" {
		spec["hour"] = clamp(intValue(payload, "hour", 0), 0, 23)
		spec["minute"] = clamp(intValue(payload, "minute", 0), 0, 59)
	} else {
		spec["type"] = "interval"
		spec["seconds"] = clamp(intValue(payload, "seconds", 3600), 60, 604800)
	}
	runtimeParams := a.schedulerRuntimeParamsFromPayload(jobID, payload)
	schedule, err := a.store.SetSchedulerScheduleWithParams(jobID, spec, runtimeParams, true)
	if statusFromError(w, err) {
		return
	}
	ok(w, "schedule updated", map[string]any{"job_id": jobID, "trigger_spec": schedule.TriggerSpec, "runtime_params": a.schedulerRuntimeParamsFromSchedule(jobID, schedule.RuntimeParams), "is_custom": true})
}

func (a *App) schedulerDefaultRuntimeParams(jobID string) map[string]any {
	switch jobID {
	case "cleanup_no_emby":
		days := a.cfg.AutoCleanupNoEmbyDays
		if days <= 0 {
			days = 7
		}
		return map[string]any{"enabled": a.cfg.AutoCleanupNoEmby, "auto_enabled": a.cfg.AutoCleanupNoEmby, "days": days, "preserve_tg_bound": a.cfg.EmbyDirectRegisterEnabled}
	case "cleanup_pending_emby_entitlements":
		return map[string]any{"enabled": a.cfg.AutoCleanupPendingEmby, "auto_enabled": a.cfg.AutoCleanupPendingEmby, "scope": "all"}
	case "kick_unknown_group_members":
		return map[string]any{"dry_run": true, "max_per_run": 200}
	default:
		return nil
	}
}

func (a *App) schedulerRuntimeParamsFromSchedule(jobID string, stored map[string]any) map[string]any {
	defaults := a.schedulerDefaultRuntimeParams(jobID)
	if len(defaults) == 0 {
		return nil
	}
	out := cloneMap(defaults)
	for key, value := range stored {
		out[key] = value
	}
	return a.normalizeSchedulerRuntimeParams(jobID, out)
}

func (a *App) schedulerRuntimeParamsFromPayload(jobID string, payload map[string]any) map[string]any {
	params := schedulerRuntimeParamsMap(payload["runtime_params"])
	if len(params) == 0 {
		params = payload
	}
	defaults := a.schedulerDefaultRuntimeParams(jobID)
	if len(defaults) == 0 {
		return nil
	}
	out := cloneMap(defaults)
	for key, value := range params {
		out[key] = value
	}
	return a.normalizeSchedulerRuntimeParams(jobID, out)
}

func (a *App) normalizeSchedulerRuntimeParams(jobID string, params map[string]any) map[string]any {
	switch jobID {
	case "cleanup_no_emby":
		enabled := boolValue(params, "enabled", boolValue(params, "auto_enabled", a.cfg.AutoCleanupNoEmby))
		days := clamp(intValue(params, "days", a.cfg.AutoCleanupNoEmbyDays), 1, 3650)
		return map[string]any{"enabled": enabled, "auto_enabled": enabled, "days": days, "preserve_tg_bound": boolValue(params, "preserve_tg_bound", a.cfg.EmbyDirectRegisterEnabled)}
	case "cleanup_pending_emby_entitlements":
		enabled := boolValue(params, "enabled", boolValue(params, "auto_enabled", a.cfg.AutoCleanupPendingEmby))
		return map[string]any{"enabled": enabled, "auto_enabled": enabled, "scope": "all"}
	case "kick_unknown_group_members":
		return map[string]any{"dry_run": boolValue(params, "dry_run", true), "max_per_run": clamp(intValue(params, "max_per_run", 200), 1, 500)}
	default:
		return nil
	}
}

func schedulerRuntimeParamsMap(value any) map[string]any {
	params, _ := value.(map[string]any)
	return params
}

func (a *App) reconcileSchedulerRunState(jobID string, running bool) {
	if running {
		return
	}
	now := time.Now().Unix()
	for _, run := range a.store.SchedulerRuns(jobID, 20) {
		if run.Status != "running" {
			continue
		}
		if _, err := a.store.UpdateSchedulerRun(run.ID, func(current *store.SchedulerRun) error {
			if current.Status != "running" {
				return nil
			}
			current.Status = "failed"
			current.Message = "job interrupted before completion"
			current.Error = "job interrupted before completion"
			current.FinishedAt = now
			current.EndedAt = now
			if current.Summary == nil {
				current.Summary = map[string]any{}
			}
			current.Summary["interrupted"] = true
			current.Summary["success"] = false
			return nil
		}); err != nil {
			zap.L().Warn("scheduler run reconciliation failed", zap.String("job_id", jobID), zap.Int64("run_id", run.ID), zap.Error(err))
		}
	}
}
