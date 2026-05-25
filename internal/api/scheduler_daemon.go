package api

import (
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

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
	for _, job := range schedulerJobs {
		jobID := fmt.Sprint(job["id"])
		if jobID == "" || boolish(job["manual_only"]) || !schedulerJobEnabledByConfig(a.cfg().SystemUpdateEnabled, job) {
			continue
		}
		spec := a.schedulerTriggerSpec(jobID)
		if strings.EqualFold(asString(spec["type"]), "manual") {
			continue
		}
		if !a.schedulerJobDue(jobID, spec, time.Now()) {
			continue
		}
		go a.runScheduledJob(ctx, jobID)
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
	runs := a.store().SchedulerRuns(jobID, 20)
	last := int64(0)
	for _, run := range runs {
		if run.Status == "running" && time.Since(time.Unix(run.StartedAt, 0)) < 30*time.Minute {
			return false
		}
		if run.Type == "auto" && run.StartedAt > last {
			last = run.StartedAt
		}
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

func (a *App) runScheduledJob(ctx context.Context, jobID string) {
	// 协程入口加 panic recover，避免单次任务崩溃带垮整个调度器。
	defer func() {
		if r := recover(); r != nil {
			// 使用 zap.String + redactSensitiveText：避免 zap.Any 走 ReflectType
			// 反射 dump 整个 panic 值（panic value 可能是携带 token/password 字段
			// 的 struct，反射编码会绕过 sensitiveLogKey 字符串脱敏）。
			zap.L().Error("scheduler job panic", zap.String("job_id", jobID), zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", r))))
		}
	}()
	runCtx, finish, ok := a.startSchedulerRun(ctx, jobID)
	if !ok {
		return
	}
	defer finish()
	started := time.Now().Unix()
	run := store.SchedulerRun{JobID: jobID, Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: started}
	run, err := a.store().AddSchedulerRunReturning(run)
	if err != nil {
		zap.L().Warn("scheduler job run record create failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}
	zap.L().Info("scheduler job started", zap.String("job_id", jobID), zap.String("type", "auto"), zap.Int64("run_id", run.ID))
	req, _ := http.NewRequestWithContext(runCtx, http.MethodPost, "/scheduler/internal", nil)
	summary, logs, err := a.runSchedulerJob(req, jobID)
	finished := schedulerFinishedRun(jobID, "auto", "scheduler", started, summary, logs, err)
	finished.ID = run.ID
	if _, updateErr := a.store().UpdateSchedulerRun(run.ID, func(run *store.SchedulerRun) error {
		*run = finished
		return nil
	}); updateErr != nil {
		zap.L().Warn("scheduler job run record update failed", zap.String("job_id", jobID), zap.Int64("run_id", run.ID), zap.Error(updateErr))
	}
	if err != nil {
		zap.L().Warn("scheduler job failed", zap.String("job_id", jobID), zap.Error(err))
	} else {
		zap.L().Info("scheduler job completed", zap.String("job_id", jobID))
	}
}

func (a *App) startManualSchedulerJob(ctx context.Context, jobID string, params map[string]any) (store.SchedulerRun, bool) {
	runCtx, finish, ok := a.startSchedulerRun(ctx, jobID)
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
	zap.L().Info("scheduler job started", zap.String("job_id", jobID), zap.String("type", "manual"), zap.Int64("run_id", run.ID))
	go func() {
		// 同 runScheduledJob 的保护：panic 不能逃出协程。
		defer func() {
			if r := recover(); r != nil {
				// 同上：panic value 走 zap.String + redact 路径，杜绝 ReflectType
				// 反射输出绕过敏感字段脱敏。
				zap.L().Error("manual scheduler job panic", zap.String("job_id", jobID), zap.Int64("run_id", run.ID), zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", r))))
			}
		}()
		defer finish()
		req, _ := http.NewRequestWithContext(runCtx, http.MethodPost, "/scheduler/manual", nil)
		req = req.WithContext(context.WithValue(req.Context(), schedulerParamsContextKey, params))
		req = req.WithContext(context.WithValue(req.Context(), schedulerManualContextKey, true))
		summary, logs, err := a.runSchedulerJob(req, jobID)
		finished := schedulerFinishedRun(jobID, "manual", "manual", started, summary, logs, err)
		finished.ID = run.ID
		if _, updateErr := a.store().UpdateSchedulerRun(run.ID, func(current *store.SchedulerRun) error {
			*current = finished
			return nil
		}); updateErr != nil {
			zap.L().Warn("manual scheduler job run record update failed", zap.String("job_id", jobID), zap.Int64("run_id", run.ID), zap.Error(updateErr))
		}
		if err != nil {
			zap.L().Warn("manual scheduler job failed", zap.String("job_id", jobID), zap.Error(err))
		} else {
			zap.L().Info("manual scheduler job completed", zap.String("job_id", jobID))
		}
	}()
	return run, true
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
		Summary:    summary,
		Logs:       logs,
		Error:      errText,
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
	if schedulerTriggerDisabled(spec) {
		return 0
	}
	last := int64(0)
	if runs := a.store().SchedulerRuns(jobID, 20); len(runs) > 0 {
		for _, run := range runs {
			if run.Type == "auto" && run.StartedAt > last {
				last = run.StartedAt
			}
		}
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
	case "cleanup_bind_codes":
		return map[string]any{"type": "interval", "seconds": 3600}
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
	cancel  context.CancelFunc
	started int64
}

// schedulerProcessLocks 已迁移到 App.schedulerLocks（app.go）。原本是 package
// 级 sync.Map，单进程 prod 没问题，但测试 setup 反复 New() 出多个 App 时
// 该表跨实例共享，会让一个 case 的 cancel 影响另一 case 的 LoadOrStore，
// 偶发 flake。改为 instance 字段后每个 App 自带独立锁。

func (a *App) startSchedulerRun(ctx context.Context, jobID string) (context.Context, func(), bool) {
	runCtx, cancel := context.WithCancel(ctx)
	run := &schedulerProcessRun{cancel: cancel, started: time.Now().Unix()}
	actual, loaded := a.schedulerLocks.LoadOrStore(jobID, run)
	if loaded {
		cancel()
		_ = actual
		return ctx, func() {}, false
	}
	finish := func() {
		cancel()
		if current, ok := a.schedulerLocks.Load(jobID); ok && current == run {
			a.schedulerLocks.Delete(jobID)
		}
	}
	return runCtx, finish, true
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
	run.cancel()
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
