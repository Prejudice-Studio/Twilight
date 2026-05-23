package api

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) RunScheduler(ctx context.Context) error {
	zap.L().Info("scheduler runner started")
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
	runs := a.store.SchedulerRuns(jobID, 1)
	last := int64(0)
	if len(runs) > 0 {
		last = runs[0].StartedAt
		if runs[0].Status == "running" && time.Since(time.Unix(runs[0].StartedAt, 0)) < 30*time.Minute {
			return false
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
	if !a.markSchedulerRunning(jobID) {
		return
	}
	defer a.clearSchedulerRunning(jobID)
	started := time.Now().Unix()
	run := store.SchedulerRun{JobID: jobID, Type: "auto", Trigger: "scheduler", Status: "running", Message: "running", StartedAt: started}
	_ = a.store.AddSchedulerRun(run)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/scheduler/internal", nil)
	summary, logs, err := a.runSchedulerJob(req, jobID)
	status := "success"
	message := "job completed"
	errText := ""
	if err != nil {
		status = "failed"
		message = err.Error()
		errText = err.Error()
	}
	finished := time.Now().Unix()
	_ = a.store.AddSchedulerRun(store.SchedulerRun{
		JobID:      jobID,
		Type:       "auto",
		Trigger:    "scheduler",
		Status:     status,
		Message:    message,
		StartedAt:  started,
		FinishedAt: finished,
		EndedAt:    finished,
		Summary:    summary,
		Logs:       logs,
		Error:      errText,
	})
	if err != nil {
		zap.L().Warn("scheduler job failed", zap.String("job_id", jobID), zap.Error(err))
	} else {
		zap.L().Info("scheduler job completed", zap.String("job_id", jobID))
	}
}

func (a *App) schedulerTriggerSpec(jobID string) map[string]any {
	if schedule, ok := a.store.SchedulerSchedule(jobID); ok && len(schedule.TriggerSpec) > 0 {
		return schedule.TriggerSpec
	}
	return a.schedulerDefaultTriggerSpec(jobID)
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

var schedulerProcessLocks sync.Map

func (a *App) markSchedulerRunning(jobID string) bool {
	_, loaded := schedulerProcessLocks.LoadOrStore(jobID, true)
	return !loaded
}

func (a *App) clearSchedulerRunning(jobID string) {
	schedulerProcessLocks.Delete(jobID)
}
