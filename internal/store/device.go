package store

import (
	"sort"
	"time"
)

func (s *Store) UpsertDevice(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	if d.FirstSeen == 0 {
		d.FirstSeen = time.Now().Unix()
	}
	if d.LastSeen == 0 {
		d.LastSeen = d.FirstSeen
	}
	s.state.Devices[deviceKey(d.UID, d.DeviceID)] = d
	return s.saveLocked()
}

func (s *Store) ListDevices(uid int64) []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0)
	for _, d := range s.state.Devices {
		if d.UID == uid && !d.Blocked {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

func (s *Store) UpdateDevice(uid int64, deviceID string, fn func(*Device)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	key := deviceKey(uid, deviceID)
	d, ok := s.state.Devices[key]
	if !ok {
		now := time.Now().Unix()
		d = Device{UID: uid, DeviceID: deviceID, DeviceName: deviceID, FirstSeen: now, LastSeen: now}
	}
	fn(&d)
	s.state.Devices[key] = d
	return s.saveLocked()
}

func (s *Store) DeleteDevice(uid int64, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	delete(s.state.Devices, deviceKey(uid, deviceID))
	return s.saveLocked()
}

func deviceKey(uid int64, deviceID string) string {
	return strconv36(uid) + ":" + deviceID
}
