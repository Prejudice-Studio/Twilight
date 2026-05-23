package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) RunScheduler(ctx context.Context) error {
	slog.Info("scheduler runner started")
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
	if !a.cfg.SchedulerEnabled {
		return
	}
	for _, job := range schedulerJobs {
		jobID := fmt.Sprint(job["id"])
		if jobID == "" || boolish(job["manual_only"]) || !schedulerJobEnabledByConfig(a.cfg.SystemUpdateEnabled, job) {
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
	runs := a.store.SchedulerRuns(jobID, 20)
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
	runCtx, finish, ok := a.startSchedulerRun(ctx, jobID)
	if !ok {
		return
	}
	defer finish()
	started := time.Now().Unix()
	run := store.SchedulerRun{JobID: jobID, Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: started}
	run, _ = a.store.AddSchedulerRunReturning(run)
	req, _ := http.NewRequestWithContext(runCtx, http.MethodPost, "/scheduler/internal", nil)
	summary, logs, err := a.runSchedulerJob(req, jobID)
	finished := schedulerFinishedRun(jobID, "auto", "scheduler", started, summary, logs, err)
	finished.ID = run.ID
	_, _ = a.store.UpdateSchedulerRun(run.ID, func(run *store.SchedulerRun) error {
		*run = finished
		return nil
	})
	if err != nil {
		slog.Warn("scheduler job failed", "job_id", jobID, "error", err)
	} else {
		slog.Info("scheduler job completed", "job_id", jobID)
	}
}

func (a *App) startManualSchedulerJob(ctx context.Context, jobID string, params map[string]any) (store.SchedulerRun, bool) {
	runCtx, finish, ok := a.startSchedulerRun(ctx, jobID)
	if !ok {
		return store.SchedulerRun{}, false
	}
	started := time.Now().Unix()
	run, _ := a.store.AddSchedulerRunReturning(store.SchedulerRun{JobID: jobID, Type: "manual", Trigger: "manual", Status: "running", Message: "running", StartedAt: started})
	go func() {
		defer finish()
		req, _ := http.NewRequestWithContext(runCtx, http.MethodPost, "/scheduler/manual", nil)
		req = req.WithContext(context.WithValue(req.Context(), schedulerParamsContextKey, params))
		req = req.WithContext(context.WithValue(req.Context(), schedulerManualContextKey, true))
		summary, logs, err := a.runSchedulerJob(req, jobID)
		finished := schedulerFinishedRun(jobID, "manual", "manual", started, summary, logs, err)
		finished.ID = run.ID
		_, _ = a.store.UpdateSchedulerRun(run.ID, func(current *store.SchedulerRun) error {
			*current = finished
			return nil
		})
		if err != nil {
			slog.Warn("manual scheduler job failed", "job_id", jobID, "error", err)
		} else {
			slog.Info("manual scheduler job completed", "job_id", jobID)
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
		message = err.Error()
		errText = err.Error()
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
	if schedule, ok := a.store.SchedulerSchedule(jobID); ok && len(schedule.TriggerSpec) > 0 {
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
	if runs := a.store.SchedulerRuns(jobID, 20); len(runs) > 0 {
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
		return dailySpec(a.cfg.SchedulerExpiredCheckTime, 3, 0)
	case "check_expiring", "expiry_reminders":
		return dailySpec(a.cfg.SchedulerExpiringCheckTime, 9, 0)
	case "daily_stats":
		return dailySpec(a.cfg.SchedulerDailyStatsTime, 0, 5)
	case "cleanup_sessions":
		hours := a.cfg.SchedulerSessionCleanupInterval
		if hours <= 0 {
			hours = 6
		}
		return map[string]any{"type": "interval", "seconds": hours * 3600}
	case "system_auto_update":
		switch strings.ToLower(strings.TrimSpace(a.cfg.SystemUpdateTriggerType)) {
		case "daily", "cron_daily":
			return dailySpec(a.cfg.SystemUpdateTime, 4, 0)
		case "manual":
			return map[string]any{"type": "manual"}
		default:
			hours := a.cfg.SystemUpdateIntervalHours
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

var schedulerProcessLocks sync.Map

func (a *App) startSchedulerRun(ctx context.Context, jobID string) (context.Context, func(), bool) {
	runCtx, cancel := context.WithCancel(ctx)
	run := &schedulerProcessRun{cancel: cancel, started: time.Now().Unix()}
	actual, loaded := schedulerProcessLocks.LoadOrStore(jobID, run)
	if loaded {
		cancel()
		_ = actual
		return ctx, func() {}, false
	}
	finish := func() {
		cancel()
		if current, ok := schedulerProcessLocks.Load(jobID); ok && current == run {
			schedulerProcessLocks.Delete(jobID)
		}
	}
	return runCtx, finish, true
}

func (a *App) schedulerJobRunning(jobID string) bool {
	_, ok := schedulerProcessLocks.Load(jobID)
	return ok
}

func (a *App) terminateSchedulerJob(jobID string) bool {
	value, ok := schedulerProcessLocks.Load(jobID)
	if !ok {
		return false
	}
	run, ok := value.(*schedulerProcessRun)
	if !ok || run.cancel == nil {
		return false
	}
	run.cancel()
	return true
}
