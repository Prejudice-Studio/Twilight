package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/store"
)

const schedulerRunningWindowSeconds = int64(30 * 60)

func (a *App) RunScheduler(ctx context.Context) error {
	zap.L().Info("scheduler runner started")
	// 主循环 panic 兜底：reloadConfigIfChanged / runDueSchedulerJobs 调用栈深，
	// 一处空指针或 map race 会让整个 RunScheduler 协程退出，所有定时任务静默
	// 失效；之前只在 runScheduledJob / startManualSchedulerJob 协程入口加了
	// recover，daemon 主循环本身没保护。这里 recover 后退避重启循环，
	// 直到外层 ctx 真的 Done 才退出。
	for {
		err := a.runSchedulerLoop(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
		}
		zap.L().Error("scheduler loop crashed, restarting after backoff", zap.Error(err))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

// runSchedulerLoop 是单次主循环；panic 经 recover 后转 error 由 RunScheduler 重启。
func (a *App) runSchedulerLoop(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// panic value 可能携带敏感字段，强制走 redactSensitiveText 字符串路径，
			// 不走 zap.Any 的反射 dump。
			zap.L().Error("scheduler loop panic", zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", r))))
			err = fmt.Errorf("scheduler loop panic: %v", r)
		}
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	a.runDueSchedulerJobs(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.reloadConfigIfChanged()
			a.runDueSchedulerJobs(ctx)
		}
	}
}

func (a *App) runDueSchedulerJobs(ctx context.Context) {
	if !a.cfg().SchedulerEnabled {
		return
	}
	now := time.Now()
	for _, job := range schedulerJobs {
		jobID := fmt.Sprint(job["id"])
		if jobID == "" || boolish(job["manual_only"]) || !schedulerJobEnabledByConfig(a.cfg().SystemUpdateEnabled, job) {
			continue
		}
		spec := a.schedulerTriggerSpec(jobID)
		if strings.EqualFold(asString(spec["type"]), "manual") {
			continue
		}
		if !a.schedulerJobDue(jobID, spec, now) {
			continue
		}
		// runScheduledJob 现在自己负责 fork 内部 goroutine：daemon 在拿到锁 +
		// 落库 auto 记录之前不返回，下一轮 tick 调用 schedulerJobDue 时 PG 上
		// 的 auto 记录已经可见——之前 `go a.runScheduledJob` 让 INSERT 还没
		// 落地 ticker 就推进了 30s，慢 PG 上偶发同 job 二次起跑。
		a.runScheduledJob(ctx, jobID)
	}
}

func schedulerJobEnabledByConfig(systemUpdateEnabled bool, job map[string]any) bool {
	if enabled, ok := job["enabled"].(bool); ok && !enabled {
		if fmt.Sprint(job["id"]) != "system_auto_update" || !systemUpdateEnabled {
			return false
		}
	}
	return true
}

func (a *App) schedulerJobDue(jobID string, spec map[string]any, now time.Time) bool {
	snapshot := a.store().SchedulerRunSnapshot(jobID, 20)
	if schedulerSnapshotRecentlyRunning(snapshot, now) {
		return false
	}
	// 关键：last 必须从全量历史里取最新 auto run，否则 admin 在窗口内对同
	// job 手动重跑 21 次会把 auto 记录挤出 SchedulerRuns(20)，进而把 last
	// 退化为 0，cron_daily 路径会判定"今天还没跑过 auto"再起一次。
	last := int64(0)
	if snapshot.HasLatestAuto {
		last = snapshot.LatestAuto.StartedAt
	}
	switch strings.ToLower(asString(spec["type"])) {
	case "cron_daily", "daily":
		hour := clamp(int(numeric(spec["hour"])), 0, 23)
		minute := clamp(int(numeric(spec["minute"])), 0, 59)
		due := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		return !now.Before(due) && last < due.Unix()
	case "interval":
		seconds := clamp(int(numeric(spec["seconds"])), 60, 604800)
		return last == 0 || now.Unix()-last >= int64(seconds)
	default:
		return false
	}
}

func schedulerSnapshotRecentlyRunning(snapshot store.SchedulerRunSnapshot, now time.Time) bool {
	return snapshot.HasLatestRunning && now.Unix()-snapshot.LatestRunning.StartedAt < schedulerRunningWindowSeconds
}

func (a *App) runScheduledJob(ctx context.Context, jobID string) {
	// 同步段：拿锁 + INSERT auto run 记录。daemon 在这两步完成前 *不* 返回，
	// 保证下一轮 30s tick 调用 schedulerJobDue 时 PG 已经能看到这条 auto 行；
	// 之前整个函数运行在 `go a.runScheduledJob` 里，慢 PG 上 INSERT 还没落
	// 盘 ticker 就推进了，schedulerJobDue 看不到 last → 又一次判定 due → 同
	// job 并发起跑。重活仍在内层 goroutine 跑，daemon 主循环不会被堵住。
	runCtx, processRun, finish, ok := a.startSchedulerRun(ctx, jobID)
	if !ok {
		return
	}
	started := time.Now().Unix()
	run, err := a.store().AddSchedulerRunReturning(store.SchedulerRun{JobID: jobID, Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: started})
	if err != nil {
		finish()
		zap.L().Warn("scheduler job run record create failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}
	processRun.runID.Store(run.ID)
	if processRun.terminated.Load() {
		a.markSchedulerRunTerminated(run.ID)
	}
	zap.L().Info("scheduler job started", zap.String("job_id", jobID), zap.String("type", "auto"), zap.Int64("run_id", run.ID))
	go a.executeSchedulerRun(runCtx, run, jobID, "auto", "scheduler", "/scheduler/internal", started, nil, false, finish)
}

func (a *App) startManualSchedulerJob(ctx context.Context, jobID string, params map[string]any) (store.SchedulerRun, bool) {
	runCtx, processRun, finish, ok := a.startSchedulerRun(ctx, jobID)
	if !ok {
		return store.SchedulerRun{}, false
	}
	started := time.Now().Unix()
	run, err := a.store().AddSchedulerRunReturning(store.SchedulerRun{JobID: jobID, Type: "manual", Trigger: "manual", Status: "running", Message: "running", StartedAt: started})
	if err != nil {
		finish()
		zap.L().Warn("manual scheduler job run record create failed", zap.String("job_id", jobID), zap.Error(err))
		return store.SchedulerRun{}, false
	}
	processRun.runID.Store(run.ID)
	if processRun.terminated.Load() {
		a.markSchedulerRunTerminated(run.ID)
	}
	zap.L().Info("scheduler job started", zap.String("job_id", jobID), zap.String("type", "manual"), zap.Int64("run_id", run.ID))
	go a.executeSchedulerRun(runCtx, run, jobID, "manual", "manual", "/scheduler/manual", started, params, true, finish)
	return run, true
}

func (a *App) executeSchedulerRun(runCtx context.Context, run store.SchedulerRun, jobID, runType, trigger, requestPath string, started int64, params map[string]any, manual bool, finish func()) {
	var (
		summary map[string]any
		logs    []string
		jobErr  error
	)
	defer func() {
		if recovered := recover(); recovered != nil {
			panicText := redactSensitiveText(fmt.Sprintf("%v", recovered))
			jobErr = fmt.Errorf("scheduler job panic: %s", panicText)
			summary = map[string]any{"success": false, "panic": true}
			logs = append(logs, "job panic: "+panicText)
			zap.L().Error("scheduler job panic", zap.String("job_id", jobID), zap.Int64("run_id", run.ID), zap.String("panic", panicText))
		}
		finished := schedulerFinishedRun(jobID, runType, trigger, started, summary, logs, jobErr)
		finished.ID = run.ID
		if _, updateErr := a.store().UpdateSchedulerRun(run.ID, func(current *store.SchedulerRun) error {
			if schedulerRunTerminatedByAdministrator(*current) {
				return nil
			}
			*current = finished
			return nil
		}); updateErr != nil {
			zap.L().Warn("scheduler job run record update failed", zap.String("job_id", jobID), zap.Int64("run_id", run.ID), zap.Error(updateErr))
		}
		if jobErr != nil {
			zap.L().Warn("scheduler job failed", zap.String("job_id", jobID), zap.String("type", runType), zap.Error(jobErr))
		} else {
			zap.L().Info("scheduler job completed", zap.String("job_id", jobID), zap.String("type", runType))
		}
		finish()
	}()
	req, _ := http.NewRequestWithContext(runCtx, http.MethodPost, requestPath, nil)
	if manual {
		req = req.WithContext(context.WithValue(req.Context(), schedulerParamsContextKey, params))
		req = req.WithContext(context.WithValue(req.Context(), schedulerManualContextKey, true))
	}
	summary, logs, jobErr = a.runSchedulerJob(req, jobID)
}

func schedulerFinishedRun(jobID, runType, trigger string, started int64, summary map[string]any, logs []string, err error) store.SchedulerRun {
	status := "success"
	message := "job completed"
	errText := ""
	if err != nil {
		status = "failed"
		// SchedulerRun.Message / Error 会被 PG INSERT 持久化，admin 后台直接
		// 显示。job 内部错误链常包含 git remote URL（含明文 PAT）、emby
		// /Auth 响应（含 password fragment）、telegram API 错误（含 bot
		// token URL）等敏感字段。统一走 redactSensitiveText。
		message = redactSensitiveText(err.Error())
		errText = message
		if errors.Is(err, context.Canceled) {
			message = "job terminated by administrator"
			errText = message
			if summary == nil {
				summary = map[string]any{}
			}
			summary["success"] = false
			summary["terminated"] = true
		}
	}
	finished := time.Now().Unix()
	return store.SchedulerRun{
		JobID:      jobID,
		Type:       runType,
		Trigger:    trigger,
		Status:     status,
		Message:    message,
		StartedAt:  started,
		FinishedAt: finished,
		EndedAt:    finished,
		Summary:    sanitizeSchedulerSummary(summary),
		Logs:       sanitizeSchedulerLogs(logs),
		Error:      errText,
	}
}

func sanitizeSchedulerSummary(summary map[string]any) map[string]any {
	if summary == nil {
		return nil
	}
	sanitized, _ := sanitizeSchedulerValue(summary).(map[string]any)
	return sanitized
}

func sanitizeSchedulerLogs(logs []string) []string {
	if len(logs) == 0 {
		return nil
	}
	out := make([]string, 0, len(logs))
	for _, logLine := range logs {
		out = append(out, redactSensitiveText(logLine))
	}
	return out
}

func sanitizeSchedulerValue(value any) any {
	switch v := value.(type) {
	case string:
		return redactSensitiveText(v)
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, redactSensitiveText(item))
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeSchedulerValue(item))
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeSchedulerSummary(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = sanitizeSchedulerValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, item := range v {
			out[key] = redactSensitiveText(item)
		}
		return out
	default:
		return value
	}
}

func (a *App) schedulerTriggerSpec(jobID string) map[string]any {
	if schedule, ok := a.store().SchedulerSchedule(jobID); ok && len(schedule.TriggerSpec) > 0 {
		return schedule.TriggerSpec
	}
	return a.schedulerDefaultTriggerSpec(jobID)
}

func schedulerTriggerDisabled(spec map[string]any) bool {
	return strings.EqualFold(asString(spec["type"]), "manual")
}

func (a *App) schedulerNextRunAt(jobID string, spec map[string]any, now time.Time) int64 {
	// 与 schedulerJobDue 对齐：从全量历史里取最新 auto run，避免被 manual
	// 重跑挤出 SchedulerRuns(20) 时把 last 退化成 0，让前端"下次自动运行"
	// 时间显示成 now / 当天而不是真正的次日。
	snapshot := a.store().SchedulerRunSnapshot(jobID, 1)
	return schedulerNextRunAtFromSnapshot(spec, now, snapshot)
}

func schedulerNextRunAtFromSnapshot(spec map[string]any, now time.Time, snapshot store.SchedulerRunSnapshot) int64 {
	if schedulerTriggerDisabled(spec) {
		return 0
	}
	last := int64(0)
	if snapshot.HasLatestAuto {
		last = snapshot.LatestAuto.StartedAt
	}
	switch strings.ToLower(asString(spec["type"])) {
	case "cron_daily", "daily":
		hour := clamp(int(numeric(spec["hour"])), 0, 23)
		minute := clamp(int(numeric(spec["minute"])), 0, 59)
		due := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !now.Before(due) || last >= due.Unix() {
			due = due.Add(24 * time.Hour)
		}
		return due.Unix()
	case "interval":
		seconds := clamp(int(numeric(spec["seconds"])), 60, 604800)
		if last == 0 {
			return now.Unix()
		}
		return time.Unix(last+int64(seconds), 0).Unix()
	default:
		return 0
	}
}

func (a *App) schedulerDefaultTriggerSpec(jobID string) map[string]any {
	switch jobID {
	case "check_expired":
		return dailySpec(a.cfg().SchedulerExpiredCheckTime, 3, 0)
	case "check_expiring", "expiry_reminders":
		return dailySpec(a.cfg().SchedulerExpiringCheckTime, 9, 0)
	case "daily_stats":
		return dailySpec(a.cfg().SchedulerDailyStatsTime, 0, 5)
	case "cleanup_sessions":
		hours := a.cfg().SchedulerSessionCleanupInterval
		if hours <= 0 {
			hours = 6
		}
		return map[string]any{"type": "interval", "seconds": hours * 3600}
	case "cleanup_no_emby":
		return dailySpec("03:30", 3, 30)
	case "cleanup_pending_emby_entitlements":
		return dailySpec("03:45", 3, 45)
	case "system_auto_update":
		switch strings.ToLower(strings.TrimSpace(a.cfg().SystemUpdateTriggerType)) {
		case "daily", "cron_daily":
			return dailySpec(a.cfg().SystemUpdateTime, 4, 0)
		case "manual":
			return map[string]any{"type": "manual"}
		default:
			hours := a.cfg().SystemUpdateIntervalHours
			if hours <= 0 {
				hours = 24
			}
			return map[string]any{"type": "interval", "seconds": hours * 3600}
		}
	case "emby_sync", "kick_unknown_group_members":
		return map[string]any{"type": "manual"}
	case "cleanup_unused_uploads":
		return dailySpec("02:20", 2, 20)
	default:
		return dailySpec("03:00", 3, 0)
	}
}

func dailySpec(value string, fallbackHour, fallbackMinute int) map[string]any {
	hour, minute := parseClock(value, fallbackHour, fallbackMinute)
	return map[string]any{"type": "cron_daily", "hour": hour, "minute": minute}
}

func parseClock(value string, fallbackHour, fallbackMinute int) (int, int) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return fallbackHour, fallbackMinute
	}
	hour, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
	minute, errM := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errH != nil || errM != nil {
		return fallbackHour, fallbackMinute
	}
	return clamp(hour, 0, 23), clamp(minute, 0, 59)
}

type schedulerProcessRun struct {
	cancel     context.CancelFunc
	started    int64
	runID      atomic.Int64
	terminated atomic.Bool
}

// schedulerProcessLocks 已迁移到 App.schedulerLocks（app.go）。原本是 package
// 级 sync.Map，单进程 prod 没问题，但测试 setup 反复 New() 出多个 App 时
// 该表跨实例共享，会让一个 case 的 cancel 影响另一 case 的 LoadOrStore，
// 偶发 flake。改为 instance 字段后每个 App 自带独立锁。

func (a *App) startSchedulerRun(ctx context.Context, jobID string) (context.Context, *schedulerProcessRun, func(), bool) {
	runCtx, cancel := context.WithCancel(ctx)
	run := &schedulerProcessRun{cancel: cancel, started: time.Now().Unix()}
	actual, loaded := a.schedulerLocks.LoadOrStore(jobID, run)
	if loaded {
		cancel()
		_ = actual
		return ctx, nil, func() {}, false
	}
	finish := func() {
		cancel()
		if current, ok := a.schedulerLocks.Load(jobID); ok && current == run {
			a.schedulerLocks.Delete(jobID)
		}
	}
	return runCtx, run, finish, true
}

func (a *App) schedulerJobRunning(jobID string) bool {
	_, ok := a.schedulerLocks.Load(jobID)
	return ok
}

func (a *App) terminateSchedulerJob(jobID string) bool {
	value, ok := a.schedulerLocks.Load(jobID)
	if !ok {
		return false
	}
	run, ok := value.(*schedulerProcessRun)
	if !ok || run.cancel == nil {
		return false
	}
	run.terminated.Store(true)
	run.cancel()
	a.markSchedulerRunTerminated(run.runID.Load())
	// 立即把 jobID 从注册表里摘掉。原实现只 cancel，依赖 runSchedulerJob 自己
	// 的 finish() 闭包在 deferred 里 Delete；但被取消的任务若卡在不响应 ctx
	// 的远端调用（emby 慢响应、git pull 远端无应答、Bangumi 5xx 无超时），
	// finish() 永不执行，下一次 admin 点"重跑"被 LoadOrStore 短路成 not started。
	// finish() 的 `current == run` 守卫保证：旧 goroutine 若日后真的醒来，
	// 不会误删新一轮起的同名任务。
	if current, ok := a.schedulerLocks.Load(jobID); ok && current == run {
		a.schedulerLocks.Delete(jobID)
	}
	return true
}

func (a *App) markSchedulerRunTerminated(runID int64) {
	if runID == 0 {
		return
	}
	now := time.Now().Unix()
	if _, err := a.store().UpdateSchedulerRun(runID, func(current *store.SchedulerRun) error {
		if current.Status != "running" {
			return nil
		}
		current.Status = "failed"
		current.Message = "job terminated by administrator"
		current.Error = current.Message
		current.FinishedAt = now
		current.EndedAt = now
		if current.Summary == nil {
			current.Summary = map[string]any{}
		}
		current.Summary["success"] = false
		current.Summary["terminated"] = true
		return nil
	}); err != nil && !errors.Is(err, store.ErrNotFound) {
		zap.L().Warn("scheduler job run termination update failed", zap.Int64("run_id", runID), zap.Error(err))
	}
}

func schedulerRunTerminatedByAdministrator(run store.SchedulerRun) bool {
	return run.Status != "running" && run.Message == "job terminated by administrator" && boolish(run.Summary["terminated"])
}
