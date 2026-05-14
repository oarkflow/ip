package geoip

import "testing"

func TestFromHeaderPrefersFirstPublicIP(t *testing.T) {
	got := FromHeader("127.0.0.1", func(name string) string {
		if name == "X-Forwarded-For" {
			return "10.1.1.2, 8.8.8.8"
		}
		return ""
	})
	if got != "8.8.8.8" {
		t.Fatalf("expected public forwarded IP, got %q", got)
	}
}

func TestFromHeaderSupportsIPv6(t *testing.T) {
	got := FromHeader("::1", func(name string) string {
		if name == "X-Forwarded-For" {
			return "fc00::1, 2001:4860:4860::8888"
		}
		return ""
	})
	if got != "2001:4860:4860::8888" {
		t.Fatalf("expected public IPv6, got %q", got)
	}
}

func TestFromHeaderSupportsForwardedHeader(t *testing.T) {
	got := FromHeader("127.0.0.1", func(name string) string {
		if name == "Forwarded" {
			return `for=192.168.1.2, for="[2001:db8:cafe::17]"`
		}
		return ""
	})
	if got != "2001:db8:cafe::17" {
		t.Fatalf("expected forwarded IPv6 from RFC 7239 format, got %q", got)
	}
}

func TestFromHeaderFallsBackToClientIP(t *testing.T) {
	got := FromHeader("203.0.113.10", func(name string) string {
		return "unknown"
	})
	if got != "203.0.113.10" {
		t.Fatalf("expected client IP fallback, got %q", got)
	}
}

