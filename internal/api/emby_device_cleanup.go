package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	embyDeviceCleanupDefaultWorkers = 10
	embyDeviceCleanupMaxWorkers     = 10
)

type embyDeviceCleanupOptions struct {
	DryRun        bool
	MaxWorkers    int
	SkipUsernames []string
}

type embyDeviceCleanupTarget struct {
	ID           string
	Name         string
	AppName      string
	AppVersion   string
	LastUserID   string
	LastUserName string
}

func embyDeviceListFromRaw(raw any) []map[string]any {
	items := raw
	if wrapped, ok := raw.(map[string]any); ok {
		items = wrapped["Items"]
	}
	list, ok := items.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func (a *App) embyDeviceCleanupProtection(skipUsernames []string) (map[string]bool, map[string]bool) {
	skipNames := map[string]bool{}
	protectedIDs := map[string]bool{}
	addName := func(name string) {
		name = strings.ToLower(normalizeEmbyDisplayText(name))
		if name != "" {
			skipNames[name] = true
		}
	}
	for _, name := range skipUsernames {
		addName(name)
	}
	for _, u := range a.store().ListUsers() {
		if !a.userIsProtected(u) {
			continue
		}
		if u.EmbyID != "" {
			protectedIDs[u.EmbyID] = true
		}
		addName(u.Username)
		addName(u.EmbyUsername)
	}
	return skipNames, protectedIDs
}

func embyDeviceCleanupSkipList(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(asString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		var out []string
		for _, part := range strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ';'
		}) {
			if s := strings.TrimSpace(part); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func embyDeviceCleanupTargetFromMap(dev map[string]any) embyDeviceCleanupTarget {
	return embyDeviceCleanupTarget{
		ID:           firstNonEmpty(asString(dev["Id"]), asString(dev["ReportedId"])),
		Name:         normalizeEmbyDisplayText(asString(dev["Name"])),
		AppName:      normalizeEmbyDisplayText(asString(dev["AppName"])),
		AppVersion:   normalizeEmbyDisplayText(asString(dev["AppVersion"])),
		LastUserID:   asString(dev["LastUserId"]),
		LastUserName: normalizeEmbyDisplayText(asString(dev["LastUserName"])),
	}
}

func (a *App) embyDeleteDeviceWithRetry(ctx context.Context, deviceID string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		err := a.embyDelete(ctx, "/Devices?Id="+url.QueryEscape(deviceID))
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func (a *App) cleanupEmbyDevices(ctx context.Context, opts embyDeviceCleanupOptions) (map[string]any, []string, error) {
	if !a.embyConfigured() {
		return map[string]any{"success": true, "configured": false, "dry_run": opts.DryRun, "scanned": 0, "candidates": 0}, []string{"Emby not configured"}, nil
	}
	workers := opts.MaxWorkers
	if workers <= 0 {
		workers = embyDeviceCleanupDefaultWorkers
	}
	workers = clamp(workers, 1, embyDeviceCleanupMaxWorkers)
	var raw any
	if err := embyRetryOn5xx(ctx, func(opCtx context.Context) error {
		return a.embyGet(opCtx, "/Devices", &raw)
	}); err != nil {
		return map[string]any{"success": false, "configured": true, "dry_run": opts.DryRun}, nil, err
	}
	devices := embyDeviceListFromRaw(raw)
	skipNames, protectedIDs := a.embyDeviceCleanupProtection(opts.SkipUsernames)
	seenIDs := map[string]bool{}
	targets := make([]embyDeviceCleanupTarget, 0, len(devices))
	skippedNoID := 0
	skippedTwilight := 0
	skippedProtected := 0
	for _, dev := range devices {
		target := embyDeviceCleanupTargetFromMap(dev)
		if target.ID == "" {
			skippedNoID++
			continue
		}
		if seenIDs[target.ID] {
			continue
		}
		seenIDs[target.ID] = true
		if isTwilightEmbyDevice(target.ID, target.Name, target.AppName) {
			skippedTwilight++
			continue
		}
		if protectedIDs[target.LastUserID] || skipNames[strings.ToLower(strings.TrimSpace(target.LastUserName))] {
			skippedProtected++
			continue
		}
		targets = append(targets, target)
	}

	summary := map[string]any{
		"success":           true,
		"configured":        true,
		"dry_run":           opts.DryRun,
		"max_workers":       workers,
		"scanned":           len(devices),
		"candidates":        len(targets),
		"deleted":           0,
		"failed":            0,
		"skipped_no_id":     skippedNoID,
		"skipped_twilight":  skippedTwilight,
		"skipped_protected": skippedProtected,
	}
	logs := []string{fmt.Sprintf("scanned %d Emby device records, candidates=%d, dry_run=%v", len(devices), len(targets), opts.DryRun)}
	if opts.DryRun || len(targets) == 0 {
		for i, target := range targets {
			if i >= 20 {
				logs = append(logs, fmt.Sprintf("... %d more candidates", len(targets)-i))
				break
			}
			logs = append(logs, fmt.Sprintf("candidate device id=%s user=%s name=%s app=%s", target.ID, firstNonEmpty(target.LastUserName, target.LastUserID, "unknown"), firstNonEmpty(target.Name, "unknown"), firstNonEmpty(target.AppName, "unknown")))
		}
		return summary, logs, nil
	}

	type result struct {
		target embyDeviceCleanupTarget
		err    error
	}
	jobs := make(chan embyDeviceCleanupTarget)
	results := make(chan result, len(targets))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				results <- result{target: target, err: a.embyDeleteDeviceWithRetry(ctx, target.ID)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, target := range targets {
			if ctx.Err() != nil {
				return
			}
			jobs <- target
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	deleted := 0
	failed := 0
	for res := range results {
		if res.err != nil {
			failed++
			if len(logs) < 50 {
				logs = append(logs, fmt.Sprintf("delete failed id=%s user=%s name=%s: %s", res.target.ID, firstNonEmpty(res.target.LastUserName, res.target.LastUserID, "unknown"), firstNonEmpty(res.target.Name, "unknown"), truncateString(redactSensitiveText(res.err.Error()), 160)))
			}
			continue
		}
		deleted++
		if len(logs) < 50 {
			logs = append(logs, fmt.Sprintf("deleted device id=%s user=%s name=%s app=%s", res.target.ID, firstNonEmpty(res.target.LastUserName, res.target.LastUserID, "unknown"), firstNonEmpty(res.target.Name, "unknown"), firstNonEmpty(res.target.AppName, "unknown")))
		}
	}
	if err := ctx.Err(); err != nil {
		summary["success"] = false
		summary["terminated"] = true
	}
	summary["deleted"] = deleted
	summary["failed"] = failed
	if deleted > 0 {
		a.invalidateEmbySessionsSnapshot()
		a.auditSystem("scheduler", "cleanup_emby_devices", 0, map[string]any{
			"scanned":           len(devices),
			"candidates":        len(targets),
			"deleted":           deleted,
			"failed":            failed,
			"skipped_twilight":  skippedTwilight,
			"skipped_protected": skippedProtected,
		})
	}
	if err := ctx.Err(); err != nil {
		return summary, append(logs, "job terminated"), err
	}
	return summary, logs, nil
}
