package api

import (
	"fmt"
	"net/http"
	"time"
)

var schedulerJobs = []map[string]any{
	{"id": "check_expired", "name": "检查已过期用户", "description": "扫描已过期账号，并按规则禁用系统或 Emby 访问。", "manual_only": false, "enabled": true},
	{"id": "check_expiring", "name": "检查即将到期用户", "description": "统计近期即将到期的用户，供管理员确认续期风险。", "manual_only": false, "enabled": true},
	{"id": "expiry_reminders", "name": "发送到期提醒", "description": "向即将到期且已绑定 Telegram 的用户发送续期提醒。", "manual_only": false, "enabled": true},
	{"id": "daily_stats", "name": "每日统计", "description": "生成每日用户与活跃状态汇总。", "manual_only": false, "enabled": true},
	{"id": "cleanup_sessions", "name": "会话巡检", "description": "读取 Emby 当前会话，统计活跃播放情况。", "manual_only": false, "enabled": true},
	{"id": "emby_sync", "name": "同步 Emby 用户", "description": "同步本地用户与 Emby 用户名称、启用状态等信息。", "manual_only": true, "enabled": true},
	{"id": "cleanup_no_emby", "name": "清理未补建 Emby 用户", "description": "清理长期未补建 Emby 的本地账号，可配置保留已绑定 Telegram 的用户。", "manual_only": false, "enabled": true, "runtime_params": []string{"days", "preserve_tg_bound", "ignore_enabled_flag"}},
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
		} else {
			spec = a.schedulerDefaultTriggerSpec(jobID)
			item["is_custom"] = false
		}
		item["trigger_spec"] = spec
		item["default_trigger_spec"] = a.schedulerDefaultTriggerSpec(jobID)
		item["last_run"] = nil
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
		item["is_running"] = a.schedulerJobRunning(jobID)
		jobs = append(jobs, item)
	}
	ok(w, "OK", map[string]any{"jobs": jobs})
}

func (a *App) handleSchedulerTerminate(w http.ResponseWriter, r *http.Request, params Params) {
	jobID := params["job_id"]
	if !schedulerJobExists(jobID) {
		fail(w, http.StatusNotFound, "job not found")
		return
	}
	if !a.terminateSchedulerJob(jobID) {
		fail(w, http.StatusConflict, "job is not running")
		return
	}
	ok(w, "job termination requested", map[string]any{"job_id": jobID, "terminated": true})
}
func (a *App) handleSchedulerLastRun(w http.ResponseWriter, r *http.Request, params Params) {
	runs := a.store.SchedulerRuns(params["job_id"], 1)
	var last any
	if len(runs) > 0 {
		last = runs[0]
	}
	ok(w, "OK", map[string]any{"job_id": params["job_id"], "last_run": last})
}
func (a *App) handleSchedulerHistory(w http.ResponseWriter, r *http.Request, params Params) {
	runs := a.store.SchedulerRuns(params["job_id"], queryInt(r, "limit", 20))
	ok(w, "OK", map[string]any{"job_id": params["job_id"], "history": runs, "total": len(runs)})
}
func (a *App) handleSchedulerSchedule(w http.ResponseWriter, r *http.Request, params Params) {
	jobID := params["job_id"]
	if !schedulerJobExists(jobID) {
		fail(w, http.StatusNotFound, "job not found")
		return
	}
	if r.Method == http.MethodDelete {
		schedule, err := a.store.SetSchedulerSchedule(jobID, a.schedulerDefaultTriggerSpec(jobID), false)
		if statusFromError(w, err) {
			return
		}
		ok(w, "schedule reset", map[string]any{"job_id": jobID, "trigger_spec": schedule.TriggerSpec, "is_custom": false})
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
	schedule, err := a.store.SetSchedulerSchedule(jobID, spec, true)
	if statusFromError(w, err) {
		return
	}
	ok(w, "schedule updated", map[string]any{"job_id": jobID, "trigger_spec": schedule.TriggerSpec, "is_custom": true})
}
