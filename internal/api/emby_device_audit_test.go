package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestParseRemoteIP(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ipv4_with_port", "192.168.1.100:54321", "192.168.1.100"},
		{"ipv4_bare", "192.168.1.100", "192.168.1.100"},
		{"ipv6_bracket_port", "[2001:db8::1]:54321", "2001:db8::1"},
		{"ipv6_loopback_port", "[::1]:8096", "::1"},
		{"ipv6_bare", "2001:db8::1", "2001:db8::1"},
		{"ipv6_bracket_no_port", "[2001:db8::1]", "2001:db8::1"},
		{"surrounding_space", "  10.0.0.5:443 ", "10.0.0.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseRemoteIP(c.in); got != c.want {
				t.Fatalf("parseRemoteIP(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseEmbyTime(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"empty", "", false},
		{"rfc3339_z", "2026-06-09T13:57:29Z", true},
		{"rfc3339_frac_z", "2026-06-09T13:57:29.0000000Z", true},
		{"rfc3339_offset", "2026-06-09T13:57:29+08:00", true},
		{"garbage", "not-a-time", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := parseEmbyTime(c.in)
			if ok != c.ok {
				t.Fatalf("parseEmbyTime(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
		})
	}
}

// TestFillDeviceIPsFromHistory 覆盖离线设备 IP 回填的三条分支：
// 唯一 IP 全量回填、多 IP 时间相关回填、超窗口不猜；并确认已有 IP 不被覆盖。
func TestFillDeviceIPsFromHistory(t *testing.T) {
	base := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	dev := func(lastActivity, ip string) map[string]any {
		return map[string]any{"last_activity": lastActivity, "ip": ip, "ip_approx": false}
	}

	t.Run("sole_ip_fills_all_offline", func(t *testing.T) {
		online := dev(base.Format(time.RFC3339), "9.9.9.9")
		offline := dev(base.Add(-48*time.Hour).Format(time.RFC3339), "")
		u := &embyAuditUser{
			ipSet:   map[string]bool{"9.9.9.9": true},
			devices: []map[string]any{online, offline},
		}
		fillDeviceIPsFromHistory(u)
		if offline["ip"] != "9.9.9.9" || offline["ip_approx"] != true {
			t.Fatalf("offline device not filled from sole IP: %v", offline)
		}
		if online["ip"] != "9.9.9.9" || online["ip_approx"] != false {
			t.Fatalf("online device IP/approx must stay live: %v", online)
		}
	})

	t.Run("multi_ip_time_correlation", func(t *testing.T) {
		offline := dev(base.Format(time.RFC3339), "")
		u := &embyAuditUser{
			ipSet:   map[string]bool{"1.1.1.1": true, "2.2.2.2": true},
			devices: []map[string]any{offline},
			authEvents: []embyAuthEvent{
				{at: base.Add(-10 * time.Hour), ip: "1.1.1.1"},
				{at: base.Add(-1 * time.Hour), ip: "2.2.2.2"},
			},
		}
		fillDeviceIPsFromHistory(u)
		if offline["ip"] != "2.2.2.2" || offline["ip_approx"] != true {
			t.Fatalf("expected closest-in-time IP 2.2.2.2 approx, got %v", offline)
		}
	})

	t.Run("multi_ip_out_of_window_stays_empty", func(t *testing.T) {
		offline := dev(base.Format(time.RFC3339), "")
		u := &embyAuditUser{
			ipSet:   map[string]bool{"1.1.1.1": true, "2.2.2.2": true},
			devices: []map[string]any{offline},
			authEvents: []embyAuthEvent{
				{at: base.Add(-24 * time.Hour), ip: "1.1.1.1"},
				{at: base.Add(24 * time.Hour), ip: "2.2.2.2"},
			},
		}
		fillDeviceIPsFromHistory(u)
		if offline["ip"] != "" {
			t.Fatalf("device outside correlation window should stay empty, got %v", offline)
		}
	})
}

func TestActivityEntryIPs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"bare_ipv4", "203.0.113.7", []string{"203.0.113.7"}},
		{"ipv4_with_port", "203.0.113.7:51000", []string{"203.0.113.7"}},
		{"prefixed_text", "From 203.0.113.7", []string{"203.0.113.7"}},
		{"ipv6_bracket_port", "[2001:db8::42]:9000", []string{"2001:db8::42"}},
		{"no_ip", "User signed in via Chrome", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := activityEntryIPs(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("activityEntryIPs(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestBuildEmbyDeviceAuditNormalizesDeviceDisplayNames(t *testing.T) {
	app := newTestApp(t)
	app.cfg().EmbyToken = "emby-token"
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Sessions":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/Devices":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"device-1","Name":"iPhone\"\\","AppName":"VidHub&quot;\\","AppVersion":"1.0&quot;\\","LastUserId":"emby-user","LastUserName":"user","DateLastActivity":"2026-05-16T12:13:40Z"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/System/ActivityLog/Entries":
			_, _ = w.Write([]byte(`{"Items":[]}`))
		default:
			t.Fatalf("unexpected Emby request: %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer emby.Close()
	app.cfg().EmbyURL = emby.URL

	data, err := app.buildEmbyDeviceAudit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	users, _ := data["users"].([]map[string]any)
	if len(users) != 1 {
		t.Fatalf("users len=%d data=%#v", len(users), data["users"])
	}
	devices, _ := users[0]["devices"].([]map[string]any)
	if len(devices) != 1 {
		t.Fatalf("devices len=%d user=%#v", len(devices), users[0])
	}
	device := devices[0]
	if got := asString(device["device_name"]); got != "iPhone" {
		t.Fatalf("device_name=%q, want iPhone", got)
	}
	if got := asString(device["app_name"]); got != "VidHub" {
		t.Fatalf("app_name=%q, want VidHub", got)
	}
	if got := asString(device["app_version"]); got != "1.0" {
		t.Fatalf("app_version=%q, want 1.0", got)
	}
	summary, _ := data["summary"].(map[string]any)
	clients, _ := summary["clients"].([]map[string]any)
	if len(clients) != 1 || asString(clients[0]["name"]) != "VidHub" {
		t.Fatalf("clients=%#v, want one VidHub client", clients)
	}
}
