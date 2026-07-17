package store

import (
	"path/filepath"
	"testing"
)

func TestDeviceLifecycleDefaultsSortsAndDeletes(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid := int64(42)
	if err := st.UpsertDevice(Device{UID: uid, DeviceID: "old", DeviceName: "Old", LastSeen: 10}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(Device{UID: uid, DeviceID: "new", DeviceName: "New", FirstSeen: 5, LastSeen: 20}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(Device{UID: uid, DeviceID: "blocked", DeviceName: "Blocked", LastSeen: 30, Blocked: true}); err != nil {
		t.Fatal(err)
	}

	devices := st.ListDevices(uid)
	if len(devices) != 2 {
		t.Fatalf("expected 2 unblocked devices, got %#v", devices)
	}
	if devices[0].DeviceID != "new" || devices[1].DeviceID != "old" {
		t.Fatalf("devices not sorted by LastSeen desc: %#v", devices)
	}
	if devices[1].FirstSeen == 0 {
		t.Fatalf("expected missing FirstSeen to default, got %#v", devices[1])
	}

	if err := st.UpdateDevice(uid, "created", func(d *Device) {
		d.DeviceName = "Created"
	}); err != nil {
		t.Fatal(err)
	}
	created := st.ListDevices(uid)[0]
	if created.DeviceID != "created" || created.FirstSeen == 0 || created.LastSeen == 0 {
		t.Fatalf("expected update to create missing device with timestamps, got %#v", created)
	}

	if err := st.DeleteDevice(uid, "created"); err != nil {
		t.Fatal(err)
	}
	for _, d := range st.ListDevices(uid) {
		if d.DeviceID == "created" {
			t.Fatal("device was not deleted")
		}
	}
	if err := st.DeleteDevice(uid, "missing"); err != nil {
		t.Fatalf("deleting a missing device should be idempotent, got %v", err)
	}
}

func TestUpdateDeviceKeepsKeyIdentityStable(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.UpdateDevice(7, "browser", func(d *Device) {
		d.UID = 999
		d.DeviceID = "other"
		d.DeviceName = "Browser"
		d.LastSeen = 123
	}); err != nil {
		t.Fatal(err)
	}
	devices := st.ListDevices(7)
	if len(devices) != 1 {
		t.Fatalf("expected one device for original uid, got %#v", devices)
	}
	if devices[0].UID != 7 || devices[0].DeviceID != "browser" {
		t.Fatalf("device update must keep key identity stable, got %#v", devices[0])
	}
	if other := st.ListDevices(999); len(other) != 0 {
		t.Fatalf("device should not leak to mutated uid: %#v", other)
	}
}
