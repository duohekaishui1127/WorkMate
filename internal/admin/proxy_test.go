package admin

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedProxyCannotSpoofClientAddress(t *testing.T) {
	proxies, err := ParseTrustedProxies("127.0.0.1,::1,10.20.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{proxies: proxies}
	for _, tc := range []struct{ peer, forwarded, expected string }{
		{"127.0.0.1:8000", "203.0.113.8", "203.0.113.8"},
		{"[::1]:8000", "2001:db8::8", "2001:db8::8"},
		{"127.0.0.1:8000", "198.51.100.9, 203.0.113.8", "203.0.113.8"},
		{"127.0.0.1:8000", "203.0.113.8, 10.20.1.2", "203.0.113.8"},
		{"198.51.100.2:8000", "203.0.113.8", "198.51.100.2"},
		{"127.0.0.1:8000", "not-an-ip", "127.0.0.1"},
		{"127.0.0.1:8000", "203.0.113.8, not-an-ip", "127.0.0.1"},
		{"127.0.0.1:8000", "0.0.0.0", "127.0.0.1"},
		{"127.0.0.1:8000", "fe80::1%forged-zone", "127.0.0.1"},
		{"127.0.0.1:8000", "::ffff:203.0.113.8", "203.0.113.8"},
		{"127.0.0.1:8000", strings.Repeat("1", 4097), "127.0.0.1"},
		{"127.0.0.1:8000", strings.Repeat("10.20.1.1,", 33) + "203.0.113.8", "127.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/healthz", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := h.clientIP(r); got != tc.expected {
			t.Errorf("%s with %q: got %s, want %s", tc.peer, tc.forwarded, got, tc.expected)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:8000"
	r.Header.Set("X-Forwarded-For", "203.0.113.8")
	if got := (&handler{}).clientIP(r); got != "127.0.0.1" {
		t.Fatal("unconfigured proxy header was trusted", got)
	}
}
func TestTrustedProxyUsersHaveSeparateRateLimits(t *testing.T) {
	proxies, _ := ParseTrustedProxies("127.0.0.1")
	h := &handler{proxies: proxies, limits: map[string]quota{}}
	request := func(ip string) bool {
		r := httptest.NewRequest("POST", "/api/orders", nil)
		r.RemoteAddr = "127.0.0.1:8000"
		r.Header.Set("X-Forwarded-For", ip)
		return h.allowed(httptest.NewRecorder(), r, "orders", 1)
	}
	if !request("203.0.113.1") || request("203.0.113.1") || !request("203.0.113.2") || request("203.0.113.2") {
		t.Fatal("proxy clients did not receive independent limits")
	}
}
