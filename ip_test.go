package ip

import (
	"testing"
	"time"
)

type testContext struct {
	ip      string
	headers map[string]string
	locals  map[any]any
	next    bool
	json    any
}

func newTestContext(ip string) *testContext {
	return &testContext{
		ip:      ip,
		headers: map[string]string{},
		locals:  map[any]any{},
	}
}

func (c *testContext) Set(key string, value string) {
	c.headers[key] = value
}

func (c *testContext) Next() error {
	c.next = true
	return nil
}

func (c *testContext) Get(key string, def ...string) string {
	if value, ok := c.headers[key]; ok {
		return value
	}
	if len(def) > 0 {
		return def[0]
	}
	return ""
}

func (c *testContext) IP() string {
	return c.ip
}

func (c *testContext) Locals(key any, value ...any) any {
	if len(value) > 0 {
		c.locals[key] = value[0]
		return value[0]
	}
	return c.locals[key]
}

func (c *testContext) JSON(data any, ctype ...string) error {
	c.json = data
	return nil
}

func TestChangeTimezoneInvalidLocationReturnsInput(t *testing.T) {
	in := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	out := ChangeTimezone(in, "Invalid/Timezone")
	if !out.Equal(in) {
		t.Fatalf("expected unchanged time for invalid timezone, got %v", out)
	}
}

func TestNewFilterInstancesAreIndependent(t *testing.T) {
	first := NewFilter(Config{
		BlockedIPs: []string{"203.0.113.10"},
	})
	second := NewFilter(Config{
		AllowedIPs:     []string{"203.0.113.10"},
		BlockByDefault: true,
	})

	firstCtx := newTestContext("203.0.113.10")
	if err := first(firstCtx); err != nil {
		t.Fatalf("first filter returned error: %v", err)
	}
	if firstCtx.next {
		t.Fatal("expected first filter to keep its original blocked IP config")
	}

	secondCtx := newTestContext("203.0.113.10")
	if err := second(secondCtx); err != nil {
		t.Fatalf("second filter returned error: %v", err)
	}
	if !secondCtx.next {
		t.Fatal("expected second filter to allow its configured IP")
	}
}

func TestNewFilterDoesNotTrustProxyByDefault(t *testing.T) {
	middleware := NewFilter(Config{
		AllowedIPs:     []string{"198.51.100.1"},
		BlockByDefault: true,
	})
	c := newTestContext("203.0.113.10")
	c.headers["X-Forwarded-For"] = "198.51.100.1"

	if err := middleware(c); err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if c.next {
		t.Fatal("expected spoofed X-Forwarded-For to be ignored without TrustProxy")
	}
}

func TestNewFilterTrustProxyUsesForwardedHeader(t *testing.T) {
	middleware := NewFilter(Config{
		AllowedIPs:     []string{"198.51.100.1"},
		BlockByDefault: true,
		TrustProxy:     true,
	})
	c := newTestContext("203.0.113.10")
	c.headers["X-Forwarded-For"] = "198.51.100.1"

	if err := middleware(c); err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if !c.next {
		t.Fatal("expected trusted X-Forwarded-For to be used")
	}
	if got := c.Locals("ip"); got != "198.51.100.1" {
		t.Fatalf("expected detected IP in locals, got %v", got)
	}
}

func TestNewFilterIgnoresNonStringLocalIP(t *testing.T) {
	middleware := NewFilter(Config{
		AllowedIPs:     []string{"203.0.113.10"},
		BlockByDefault: true,
	})
	c := newTestContext("203.0.113.10")
	c.locals["ip"] = 42

	if err := middleware(c); err != nil {
		t.Fatalf("filter returned error: %v", err)
	}
	if !c.next {
		t.Fatal("expected fallback to request IP instead of panicking")
	}
}
