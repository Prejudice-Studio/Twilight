package api

import (
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type securityService struct {
	app *App
}

func (s *securityService) listDevices(uid int64) []map[string]any {
	items := []map[string]any{}
	for _, d := range s.app.store().ListDevices(uid) {
		items = append(items, map[string]any{
			"device_id":   d.DeviceID,
			"device_name": d.DeviceName,
			"client":      d.Client,
			"last_ip":     d.LastIP,
			"first_seen":  d.FirstSeen,
			"last_seen":   d.LastSeen,
			"is_trusted":  d.Trusted,
			"blocked":     d.Blocked,
		})
	}
	return items
}

func (s *securityService) blockDevice(uid int64, deviceID string) error {
	return s.app.store().UpdateDevice(uid, deviceID, func(d *store.Device) {
		d.Blocked = true
		d.Trusted = false
	})
}

func (s *securityService) trustDevice(uid int64, deviceID string) error {
	return s.app.store().UpdateDevice(uid, deviceID, func(d *store.Device) {
		d.Trusted = true
		d.Blocked = false
	})
}

func (s *securityService) deleteDevice(uid int64, deviceID string) error {
	return s.app.store().DeleteDevice(uid, deviceID)
}

func (s *securityService) loginHistory(uid int64, limit int) []map[string]any {
	logs := s.app.store().LoginHistory(uid, false, 0, limit)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{
			"id":      log.ID,
			"ip":      log.IP,
			"device":  log.DeviceName,
			"client":  log.Client,
			"time":    log.Time,
			"blocked": log.Blocked,
			"country": log.Country,
			"city":    log.City,
		})
	}
	return items
}

func (s *securityService) listIPBlacklist() []store.IPBlacklistEntry {
	return s.app.store().ListIPBlacklist()
}

func (s *securityService) addIPBlacklist(ip, reason string, hours int) error {
	expireAt := int64(-1)
	if hours > 0 {
		expireAt = time.Now().Add(time.Duration(hours) * time.Hour).Unix()
	}
	return s.app.store().AddIPBlacklist(ip, reason, expireAt)
}

func (s *securityService) removeIPBlacklist(ip string) error {
	return s.app.store().RemoveIPBlacklist(ip)
}

func (s *securityService) suspiciousActivity(hours int) []map[string]any {
	logs := s.app.store().LoginHistory(0, true, time.Now().Add(-time.Duration(hours)*time.Hour).Unix(), 100)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{
			"uid":    log.UID,
			"ip":     log.IP,
			"device": log.DeviceName,
			"time":   log.Time,
			"reason": firstNonEmpty(log.Reason, "blocked"),
		})
	}
	return items
}

func (a *App) security() *securityService {
	return &securityService{app: a}
}
