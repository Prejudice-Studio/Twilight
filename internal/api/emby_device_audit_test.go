package api

import (
	"reflect"
	"testing"
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
