package store

import "time"

const maxStoredSchedulerRuns = 200

func (s *Store) AddSchedulerRun(run SchedulerRun) error {
	_, err := s.AddSchedulerRunReturning(run)
	return err
}

func (s *Store) AddSchedulerRunReturning(run SchedulerRun) (SchedulerRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return SchedulerRun{}, err
	}
	if run.ID == 0 {
		run.ID = s.state.NextSchedulerRunID
		s.state.NextSchedulerRunID++
	}
	if run.Type == "" {
		run.Type = "manual"
	}
	if run.Trigger == "" {
		run.Trigger = "manual"
	}
	normalizeSchedulerRunTimestamps(&run)
	s.state.SchedulerRuns = append([]SchedulerRun{run}, s.state.SchedulerRuns...)
	if len(s.state.SchedulerRuns) > maxStoredSchedulerRuns {
		s.state.SchedulerRuns = s.state.SchedulerRuns[:maxStoredSchedulerRuns]
	}
	return run, s.saveLocked()
}

func (s *Store) UpdateSchedulerRun(id int64, fn func(*SchedulerRun) error) (SchedulerRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return SchedulerRun{}, err
	}
	if id == 0 {
		return SchedulerRun{}, ErrNotFound
	}
	for i := range s.state.SchedulerRuns {
		if s.state.SchedulerRuns[i].ID != id {
			continue
		}
		run := s.state.SchedulerRuns[i]
		if err := fn(&run); err != nil {
			return SchedulerRun{}, err
		}
		normalizeSchedulerRunTimestamps(&run)
		s.state.SchedulerRuns[i] = run
		return run, s.saveLocked()
	}
	return SchedulerRun{}, ErrNotFound
}

func (s *Store) SchedulerRuns(jobID string, limit int) []SchedulerRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	out := make([]SchedulerRun, 0, limit)
	for _, run := range s.state.SchedulerRuns {
		if jobID == "" || run.JobID == jobID {
			out = append(out, run)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func (s *Store) SetSchedulerSchedule(jobID string, spec map[string]any, custom bool) (SchedulerSchedule, error) {
	return s.SetSchedulerScheduleWithParams(jobID, spec, nil, custom)
}

func (s *Store) SetSchedulerScheduleWithParams(jobID string, spec map[string]any, params map[string]any, custom bool) (SchedulerSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return SchedulerSchedule{}, err
	}
	schedule := SchedulerSchedule{JobID: jobID, TriggerSpec: spec, RuntimeParams: params, IsCustom: custom, UpdatedAt: time.Now().Unix()}
	if !custom {
		delete(s.state.SchedulerSchedules, jobID)
		return schedule, s.saveLocked()
	}
	s.state.SchedulerSchedules[jobID] = schedule
	return schedule, s.saveLocked()
}

func (s *Store) SchedulerSchedule(jobID string) (SchedulerSchedule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	schedule, ok := s.state.SchedulerSchedules[jobID]
	return schedule, ok
}

func normalizeSchedulerRunTimestamps(run *SchedulerRun) {
	if run.FinishedAt == 0 && run.EndedAt != 0 {
		run.FinishedAt = run.EndedAt
	}
}
