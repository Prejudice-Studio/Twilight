package api

import "net/http"

// Scheduler V2 is a resource-oriented adapter over the shared scheduler
// handlers. Keeping the transition rules in one place prevents V1 and V2
// actions from drifting on manual-only jobs and runtime parameter validation.
func (a *App) handleV2SchedulerJobs(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleSchedulerJobs(w, r, p)
}

func (a *App) handleV2SchedulerRun(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleSchedulerRunV2(w, r, p)
}

func (a *App) handleV2SchedulerTerminate(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleSchedulerTerminate(w, r, p)
}

func (a *App) handleV2SchedulerLastRun(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleSchedulerLastRun(w, r, p)
}

func (a *App) handleV2SchedulerHistory(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleSchedulerHistory(w, r, p)
}

func (a *App) handleV2SchedulerSchedule(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleSchedulerSchedule(w, r, p)
}
