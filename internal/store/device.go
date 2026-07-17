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
	key := deviceKey(d.UID, d.DeviceID)
	previous, existed := s.state.Devices[key]
	if existed && previous == d {
		return nil
	}
	s.state.Devices[key] = d
	if err := s.saveLocked(); err != nil {
		if existed {
			s.state.Devices[key] = previous
		} else {
			delete(s.state.Devices, key)
		}
		return err
	}
	return nil
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
	d, existed := s.state.Devices[key]
	if !existed {
		now := time.Now().Unix()
		d = Device{UID: uid, DeviceID: deviceID, DeviceName: deviceID, FirstSeen: now, LastSeen: now}
	}
	previous := d
	fn(&d)
	d.UID = uid
	d.DeviceID = deviceID
	if existed && previous == d {
		return nil
	}
	s.state.Devices[key] = d
	if err := s.saveLocked(); err != nil {
		if existed {
			s.state.Devices[key] = previous
		} else {
			delete(s.state.Devices, key)
		}
		return err
	}
	return nil
}

func (s *Store) DeleteDevice(uid int64, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	key := deviceKey(uid, deviceID)
	previous, existed := s.state.Devices[key]
	if !existed {
		return nil
	}
	delete(s.state.Devices, key)
	if err := s.saveLocked(); err != nil {
		s.state.Devices[key] = previous
		return err
	}
	return nil
}

// EnforceDeviceLimit 仅保留某用户最近活跃的 max 台设备（按 LastSeen 倒序），淘汰
// 更旧的「未受信任」设备。约定（防误踢/防锁死）：
//   - max<=0 视为不限制，直接返回；
//   - 受信任设备（Trusted）永不淘汰，即使超出名额；
//   - 刚登录的设备 LastSeen 最新，必在保留区，不会把当前会话踢掉；
//   - 已 Blocked 的设备不计入活跃集，也不参与淘汰（保持封禁状态）。
//
// 仅在 DeviceLimitEnabled 时由登录路径调用。
func (s *Store) EnforceDeviceLimit(uid int64, max int) error {
	if max <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	type keyed struct {
		key string
		dev Device
	}
	active := make([]keyed, 0)
	for key, d := range s.state.Devices {
		if d.UID == uid && !d.Blocked {
			active = append(active, keyed{key, d})
		}
	}
	if len(active) <= max {
		return nil
	}
	sort.Slice(active, func(i, j int) bool { return active[i].dev.LastSeen > active[j].dev.LastSeen })
	var previous map[string]Device
	changed := false
	for i, item := range active {
		if i < max || item.dev.Trusted {
			continue // 在名额内，或受信任 → 保留
		}
		if previous == nil {
			previous = make(map[string]Device)
		}
		previous[item.key] = item.dev
		delete(s.state.Devices, item.key)
		changed = true
	}
	if !changed {
		return nil
	}
	if err := s.saveLocked(); err != nil {
		for key, dev := range previous {
			s.state.Devices[key] = dev
		}
		return err
	}
	return nil
}

func deviceKey(uid int64, deviceID string) string {
	return strconv36(uid) + ":" + deviceID
}
