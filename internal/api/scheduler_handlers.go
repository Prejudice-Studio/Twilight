package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

var schedulerJobs = []map[string]any{
	{"id": "check_expired", "name": "检查已过期用户", "description": "扫描已过期账号，按规则禁用系统或 Emby 访问，并清除过期会话。", "manual_only": false, "enabled": true},
	{"id": "check_expiring", "name": "检查即将到期用户", "description": "统计近期即将到期的用户数量，供管理员评估续期风险。", "manual_only": false, "enabled": true},
	{"id": "expiry_reminders", "name": "发送到期提醒", "description": "向即将到期且已绑定 Telegram 的用户发送续期通知。", "manual_only": false, "enabled": true},
	{"id": "daily_stats", "name": "每日统计", "description": "记录每日用户总数与活跃用户数。", "manual_only": false, "enabled": true},
	{"id": "cleanup_sessions", "name": "会话巡检与清理", "description": "清理过期会话与邮箱验证码，并读取 Emby 当前活跃会话数。", "manual_only": false, "enabled": true},
	{"id": "emby_sync", "name": "同步 Emby 用户", "description": "将本地用户与 Emby 远程用户的 ID、名称、禁用状态同步，修复占位 ID。", "manual_only": true, "enabled": true},
	{"id": "cleanup_no_emby", "name": "清理无 Emby 账号", "description": "删除注册后长期未绑定 Emby 且无开通资格的 Web 账号。", "manual_only": false, "enabled": true},
	{"id": "cleanup_pending_emby_entitlements", "name": "清理未使用的 Emby 开通资格", "description": "收回长期未创建 Emby 的开通资格，保留 Web 账号。", "manual_only": false, "enabled": true},
	{"id": "enforce_group_membership", "name": "Telegram 群成员校验", "description": "校验用户是否仍在要求的群组内，按配置处理退群（禁用/封禁/自动解禁）。", "manual_only": false, "enabled": true},
	{"id": "check_telegram_bindings", "name": "Telegram 绑定检查", "description": "扫描重复或异常的 Telegram 绑定关系。", "manual_only": false, "enabled": true},
	{"id": "system_auto_update", "name": "系统自动更新", "description": "从 Git 拉取更新并选择性重启服务。", "manual_only": false, "enabled": false},
	{"id": "cleanup_unused_uploads", "name": "清理未使用上传文件", "description": "删除未被引用的过期间接上传文件。", "manual_only": false, "enabled": true},
	{"id": "cleanup_audit_logs", "name": "审计日志自动清理", "description": "按保留天数/条数策略清理过期操作日志，可保留管理员记录。", "manual_only": false, "enabled": true},
	{"id": "cleanup_ticket_images", "name": "清理过期工单图片", "description": "按保留天数清理已关闭工单的图片附件及元数据。", "manual_only": false, "enabled": true},
	{"id": "refresh_bangumi_collections", "name": "刷新 Bangumi 收藏缓存", "description": "每小时为开启 BGM 管理且配置 Token 的用户缓存在看、想看、看过收藏列表。", "manual_only": false, "enabled": true},
	{"id": "sync_emby_activity_logs", "name": "同步 Emby 活动日志", "description": "每10分钟从 Emby 拉取活动日志(播放/登录等)并存入数据库，用于播放统计。", "manual_only": false, "enabled": true},
	{"id": "cleanup_unlinked_emby", "name": "清理孤立 Emby 账号", "description": "扫描 Emby 中未绑定任何 Web 账号的孤立用户，支持仅扫描与删除模式。", "manual_only": false, "enabled": false, "runtime_params": []string{"dry_run", "delete"}},
	{"id": "kick_unknown_group_members", "name": "踢出未知 Telegram 群成员", "description": "根据观察到的群成员名册，踢出无账号/未绑定 Emby/已禁用的成员。", "manual_only": true, "enabled": true, "runtime_params": []string{"dry_run", "max_per_run"}},
}

func (a *App) handleSchedulerJobs(w http.ResponseWriter, r *http.Request, _ Params) {
	jobs := make([]map[string]any, 0, len(schedulerJobs))
	now := time.Now()

	// Batch-fetch all snapshots in a single lock acquisition instead of
	// N separate SchedulerRunSnapshot calls (one per job). This reduces
	// lock contention from ~26 RLock/RUnlock cycles to 1 per request.
	jobIDs := make([]string, 0, len(schedulerJobs))
	for _, job := range schedulerJobs {
		jobIDs = append(jobIDs, fmt.Sprint(job["id"]))
	}
	activeJobIDs := a.schedulerActiveJobIDs(jobIDs)
	overview, err := a.store().SchedulerStateOverview(jobIDs, 20, activeJobIDs, now.Unix()-schedulerRunningWindowSeconds, now.Unix())
	if err != nil {
		zap.L().Warn("scheduler overview refresh failed; using in-memory snapshot", zap.Error(err))
		overview.Runs = a.store().BatchSchedulerRunSnapshots(jobIDs, 20)
		overview.Schedules = a.store().SchedulerSchedules(jobIDs)
	}

	for i, job := range schedulerJobs {
		item := cloneMap(job)
		jobID := jobIDs[i]
		var spec map[string]any
		if schedule, okSchedule := overview.Schedules[jobID]; okSchedule {
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
		snapshot := overview.Runs[jobID]
		running := activeJobIDs[jobID] || schedulerSnapshotRecentlyRunning(snapshot, now)
		item["next_run_at"] = zeroNil(schedulerNextRunAtFromSnapshot(spec, now, snapshot))
		item["auto_disabled"] = schedulerTriggerDisabled(spec)
		if runs := snapshot.Runs; len(runs) > 0 {
			item["last_run"] = schedulerRunListView(runs[0])
			if snapshot.HasLatestAuto {
				item["last_auto_run_at"] = zeroNil(snapshot.LatestAuto.StartedAt)
			}
			if snapshot.HasLatestManual {
				item["last_manual_run_at"] = zeroNil(snapshot.LatestManual.StartedAt)
			}
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
		a.reconcileSchedulerRunState(jobID, false, time.Now())
		ok(w, "job is not running", map[string]any{"job_id": jobID, "terminated": false, "already_stopped": true})
		return
	}
	ok(w, "job termination requested", map[string]any{"job_id": jobID, "terminated": true})
}
func (a *App) handleSchedulerLastRun(w http.ResponseWriter, r *http.Request, params Params) {
	runs := a.schedulerRunsForRead(params["job_id"], 1, time.Now())
	var last any
	if len(runs) > 0 {
		last = runs[0]
	}
	ok(w, "OK", map[string]any{"job_id": params["job_id"], "last_run": last})
}
func (a *App) handleSchedulerHistory(w http.ResponseWriter, r *http.Request, params Params) {
	runs := a.schedulerRunsForRead(params["job_id"], queryInt(r, "limit", 20), time.Now())
	ok(w, "OK", map[string]any{"job_id": params["job_id"], "history": runs, "total": len(runs)})
}
func (a *App) handleSchedulerSchedule(w http.ResponseWriter, r *http.Request, params Params) {
	jobID := params["job_id"]
	if !schedulerJobExists(jobID) {
		failWithCode(w, http.StatusNotFound, ErrSchedulerJobNotFound, "调度任务不存在")
		return
	}
	if r.Method == http.MethodDelete {
		schedule, err := a.store().SetSchedulerSchedule(jobID, a.schedulerDefaultTriggerSpec(jobID), false)
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
	schedule, err := a.store().SetSchedulerScheduleWithParams(jobID, spec, runtimeParams, true)
	if statusFromError(w, err) {
		return
	}
	ok(w, "schedule updated", map[string]any{"job_id": jobID, "trigger_spec": schedule.TriggerSpec, "runtime_params": a.schedulerRuntimeParamsFromSchedule(jobID, schedule.RuntimeParams), "is_custom": true})
}

func (a *App) schedulerDefaultRuntimeParams(jobID string) map[string]any {
	switch jobID {
	case "cleanup_no_emby":
		days := a.cfg().AutoCleanupNoEmbyDays
		if days <= 0 {
			days = 7
		}
		return map[string]any{"enabled": a.cfg().AutoCleanupNoEmby, "auto_enabled": a.cfg().AutoCleanupNoEmby, "days": days, "preserve_tg_bound": a.cfg().EmbyDirectRegisterEnabled}
	case "cleanup_pending_emby_entitlements":
		return map[string]any{"enabled": a.cfg().AutoCleanupPendingEmby, "auto_enabled": a.cfg().AutoCleanupPendingEmby, "scope": "all"}
	case "cleanup_audit_logs":
		return map[string]any{"enabled": a.cfg().AuditLogAutoCleanupEnabled, "auto_enabled": a.cfg().AuditLogAutoCleanupEnabled, "retention_days": a.cfg().AuditLogRetentionDays, "max_entries": a.cfg().AuditLogMaxEntries, "preserve_admin": a.cfg().AuditLogPreserveAdmin}
	case "cleanup_ticket_images":
		return map[string]any{"retention_days": a.cfg().TicketImageRetentionDays}
	case "cleanup_unlinked_emby":
		return map[string]any{"dry_run": true, "delete": false}
	case "kick_unknown_group_members":
		return map[string]any{"dry_run": true, "max_per_run": 200}
	case "enforce_group_membership":
		return map[string]any{"auto_enable_rejoined": a.cfg().TelegramAutoEnableRejoined}
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
		enabled := boolValue(params, "enabled", boolValue(params, "auto_enabled", a.cfg().AutoCleanupNoEmby))
		days := clamp(intValue(params, "days", a.cfg().AutoCleanupNoEmbyDays), 1, 3650)
		return map[string]any{"enabled": enabled, "auto_enabled": enabled, "days": days, "preserve_tg_bound": boolValue(params, "preserve_tg_bound", a.cfg().EmbyDirectRegisterEnabled)}
	case "cleanup_pending_emby_entitlements":
		enabled := boolValue(params, "enabled", boolValue(params, "auto_enabled", a.cfg().AutoCleanupPendingEmby))
		return map[string]any{"enabled": enabled, "auto_enabled": enabled, "scope": "all"}
	case "cleanup_audit_logs":
		enabled := boolValue(params, "enabled", boolValue(params, "auto_enabled", a.cfg().AuditLogAutoCleanupEnabled))
		// 前端可能发送 "days" 作为 "retention_days" 的别名
		retentionDays := clamp(intValue(params, "retention_days", intValue(params, "days", a.cfg().AuditLogRetentionDays)), 0, 3650)
		maxEntries := clamp(intValue(params, "max_entries", a.cfg().AuditLogMaxEntries), 0, 100000)
		return map[string]any{"enabled": enabled, "auto_enabled": enabled, "retention_days": retentionDays, "max_entries": maxEntries, "preserve_admin": boolValue(params, "preserve_admin", a.cfg().AuditLogPreserveAdmin)}
	case "cleanup_ticket_images":
		retentionDays := clamp(intValue(params, "retention_days", intValue(params, "days", a.cfg().TicketImageRetentionDays)), 0, 3650)
		return map[string]any{"retention_days": retentionDays}
	case "kick_unknown_group_members":
		return map[string]any{"dry_run": boolValue(params, "dry_run", true), "max_per_run": clamp(intValue(params, "max_per_run", 200), 1, 500)}
	case "enforce_group_membership":
		return map[string]any{"auto_enable_rejoined": boolValue(params, "auto_enable_rejoined", a.cfg().TelegramAutoEnableRejoined)}
	case "emby_sync":
		return map[string]any{"max_users": clamp(intValue(params, "max_users", 1000), 1, 50000)}
	case "sync_emby_activity_logs":
		return map[string]any{"since_hours": clamp(intValue(params, "since_hours", 24), 1, 720)}
	default:
		return nil
	}
}

func schedulerRuntimeParamsMap(value any) map[string]any {
	params, _ := value.(map[string]any)
	return params
}

func (a *App) reconcileSchedulerRunState(jobID string, running bool, now time.Time) {
	if running {
		return
	}
	if _, err := a.store().SchedulerStateOverview([]string{jobID}, 1, nil, now.Unix()-schedulerRunningWindowSeconds, now.Unix()); err != nil {
		zap.L().Warn("scheduler run reconciliation failed", zap.String("job_id", jobID), zap.Error(err))
	}
}

func (a *App) schedulerActiveJobIDs(jobIDs []string) map[string]bool {
	active := make(map[string]bool)
	for _, jobID := range jobIDs {
		if a.schedulerJobRunning(jobID) {
			active[jobID] = true
		}
	}
	return active
}

func (a *App) schedulerRunsForRead(jobID string, limit int, now time.Time) []store.SchedulerRun {
	active := a.schedulerActiveJobIDs([]string{jobID})
	overview, err := a.store().SchedulerStateOverview([]string{jobID}, limit, active, now.Unix()-schedulerRunningWindowSeconds, now.Unix())
	if err != nil {
		zap.L().Warn("scheduler run refresh failed; using in-memory history", zap.String("job_id", jobID), zap.Error(err))
		return a.store().SchedulerRuns(jobID, limit)
	}
	return overview.Runs[jobID].Runs
}

func schedulerRunListView(run store.SchedulerRun) store.SchedulerRun {
	run.Logs = nil
	return run
}
