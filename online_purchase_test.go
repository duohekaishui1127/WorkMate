package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"workmate/internal/admin"
	"workmate/internal/entitlement"
)

func TestManualBackendToClientActivationAndOfflineReload(t *testing.T) {
	serverDir := t.TempDir()
	store, err := admin.Open(serverDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Exercise the production topology: a verified HTTPS proxy in front of HTTP.
	proxy := &httputil.ReverseProxy{}
	srv := httptest.NewUnstartedServer(proxy)
	origin := "https://" + srv.Listener.Addr().String()
	trusted, err := admin.ParseTrustedProxies("127.0.0.1,::1")
	if err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(admin.NewHandler(store, admin.HTTPOptions{PublicURL: origin, TrustedProxies: trusted}))
	defer backend.Close()
	target, _ := url.Parse(backend.URL)
	proxy.Rewrite = func(r *httputil.ProxyRequest) { r.SetURL(target); r.SetXForwarded() }
	srv.StartTLS()
	defer srv.Close()
	oldKey := onlineLicensePublicKeyB64
	onlineLicensePublicKeyB64 = store.PublicKey()
	defer func() { onlineLicensePublicKeyB64 = oldKey }()
	b, err := os.ReadFile(filepath.Join(serverDir, "initial-password.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	var csrf string
	call := func(method, path, token string, body any, out any) {
		t.Helper()
		var payload []byte
		raw, isImage := body.([]byte)
		if isImage {
			payload = raw
		} else if body != nil {
			payload, _ = json.Marshal(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if body != nil && !isImage {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", srv.URL)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != nil {
			req.AddCookie(cookie)
			req.Header.Set("X-CSRF-Token", csrf)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			t.Fatalf("%s %s: HTTP %d", method, path, res.StatusCode)
		}
		if path == "/api/admin/login" {
			cookie = res.Cookies()[0]
			if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatal("production proxy login did not protect its session cookie")
			}
		}
		if out != nil {
			if err = json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var login struct {
		CSRF string `json:"csrf"`
	}
	call("POST", "/api/admin/login", "", map[string]string{"username": "admin", "password": strings.TrimSpace(string(b))}, &login)
	csrf = login.CSRF
	var qr bytes.Buffer
	if err = png.Encode(&qr, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/admin/payment-qr", "", qr.Bytes(), nil)
	call("POST", "/api/admin/settings", "", admin.Settings{Enabled: true, PriceCents: 3490, Contact: "owner@example.com", TrialHours: 48}, nil)
	dataDir := t.TempDir()
	device := strings.Repeat("a", 64)
	client, err := newPurchaseClient(srv.URL, device, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = srv.Client().Transport // Trust the test CA; keep TLS verification enabled.
	start := time.Now().Add(-25 * time.Hour)
	lm := &LicenseManager{dataDir: dataDir, deviceID: device, trialStart: start}
	if lm.TrialActive(time.Now()) {
		t.Fatal("default trial unexpectedly active")
	}
	cfg, err := client.config()
	if err != nil {
		t.Fatal(err)
	}
	if err = lm.ApplyTrialPolicy(cfg.TrialPolicy); err != nil {
		t.Fatal(err)
	}
	if !lm.TrialActive(time.Now()) || !lm.trialStart.Equal(start) {
		t.Fatal("admin trial change reset original start or was not applied")
	}
	ticket, order, err := client.create()
	if err != nil || order.Status != "created" || order.AmountCents != 3490 {
		t.Fatal("order creation failed", err)
	}
	if err = client.saveTicket(ticket); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newPurchaseClient(srv.URL, device, dataDir)
	if err != nil || reloaded.ticket == nil || reloaded.ticket.OrderID != ticket.OrderID {
		t.Fatal("pending order not retained", err)
	}
	if !strings.Contains(client.purchaseURL(), "#"+ticket.Token) {
		t.Fatal("browser access token must use URL fragment")
	}
	call("POST", "/api/orders/"+ticket.OrderID+"/evidence", ticket.Token, qr.Bytes(), nil)
	call("POST", "/api/orders/"+ticket.OrderID+"/submit", ticket.Token, admin.Submission{}, nil)
	order, err = client.status(ticket)
	if err != nil || order.Status != "pending" || order.LicenseCode != "" || lm.IsPro() {
		t.Fatal("unconfirmed payment unlocked Pro", err)
	}
	stored, err := store.Order(ticket.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/admin/orders/"+ticket.OrderID+"/approve", "", admin.Decision{Revision: stored.Revision, Confirmed: true, ReceivedCents: 3490, Receipt: "OWNER-ACTUAL-RECEIPT"}, nil)
	order, err = client.status(ticket)
	if err != nil || order.Status != "approved" {
		t.Fatal(err)
	}
	if err = lm.Activate(order.LicenseCode); err != nil || !lm.IsPro() {
		t.Fatal("signed backend license did not activate", err)
	}
	if err = client.acknowledgeActivation(ticket); err != nil {
		t.Fatal("activated license receipt failed", err)
	}
	if err = client.sendTelemetry("/api/telemetry", map[string]any{"device_id": device, "version": appVersion, "trial_started_at": lm.trialStart.UTC().Format(time.RFC3339), "pro": true}); err != nil {
		t.Fatal("client lifecycle report failed", err)
	}
	stats, err := store.AnalyticsOverview()
	if err != nil || stats.ActivatedOrders != 1 || stats.TodayActive != 1 || stats.KnownProDevices != 1 {
		t.Fatalf("backend did not observe activation and usage: %+v %v", stats, err)
	}
	srv.Close()
	offline := &LicenseManager{dataDir: dataDir, deviceID: device}
	offline.loadLicense()
	if !offline.IsPro() {
		t.Fatal("offline restart lost activated license")
	}
	if verifyActivationCode(order.LicenseCode, strings.Repeat("b", 64), store.PublicKey()) == nil {
		t.Fatal("license worked on another device")
	}
}

func TestSignedTrialPolicyCannotBeForgedOrReplayed(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := onlineLicensePublicKeyB64
	onlineLicensePublicKeyB64 = base64.StdEncoding.EncodeToString(pub)
	defer func() { onlineLicensePublicKeyB64 = old }()
	dir := t.TempDir()
	start := time.Now().Add(-5 * time.Hour)
	lm := &LicenseManager{dataDir: dir, deviceID: strings.Repeat("c", 64), trialStart: start}
	newPolicy, _ := entitlement.Sign(entitlement.TrialPolicy{Version: 1, Kind: "trial-policy", Hours: 6, IssuedAt: "2026-09-29T10:00:00Z"}, key)
	if err = lm.ApplyTrialPolicy(newPolicy); err != nil || lm.TrialRemaining(time.Now()) > time.Hour {
		t.Fatal("policy duration invalid", err)
	}
	older, _ := entitlement.Sign(entitlement.TrialPolicy{Version: 1, Kind: "trial-policy", Hours: 100, IssuedAt: "2026-09-29T09:00:00Z"}, key)
	if err = lm.ApplyTrialPolicy(older); err != nil || lm.effectiveTrialDuration() != 6*time.Hour {
		t.Fatal("older policy replay extended trial", err)
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	forged, _ := entitlement.Sign(entitlement.TrialPolicy{Version: 1, Kind: "trial-policy", Hours: 100, IssuedAt: "2026-09-29T11:00:00Z"}, other)
	if lm.ApplyTrialPolicy(forged) == nil || lm.effectiveTrialDuration() != 6*time.Hour {
		t.Fatal("forged policy accepted")
	}
	cached, err := os.ReadFile(filepath.Join(dir, "trial-policy.dat"))
	if err != nil {
		t.Fatal(err)
	}
	offline := &LicenseManager{trialStart: start}
	if err = offline.applyTrialPolicy(string(cached), false); err != nil || offline.effectiveTrialDuration() != 6*time.Hour {
		t.Fatal("signed offline policy not restored", err)
	}
}

func TestPurchaseURLsAndTicketOwnership(t *testing.T) {
	for _, bad := range []string{"http://example.com", "http://192.168.1.2:8090", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=1", "file:///tmp/order"} {
		if _, err := validateServerURL(bad); err == nil {
			t.Errorf("unsafe server URL accepted: %s", bad)
		}
	}
	for _, good := range []string{"https://pro.example.com", "http://127.0.0.1:8090", "http://localhost:8090", "http://[::1]:8090"} {
		if _, err := validateServerURL(good); err != nil {
			t.Error(err)
		}
	}
	dir := t.TempDir()
	device := strings.Repeat("d", 64)
	c, err := newPurchaseClient("https://pro.example.com", device, dir)
	if err != nil {
		t.Fatal(err)
	}
	ticket := purchaseTicket{ServerURL: c.base, DeviceID: device, OrderID: "WM-TEST-ORDER", Token: strings.Repeat("1", 64)}
	if err = c.saveTicket(ticket); err != nil {
		t.Fatal(err)
	}
	other, err := newPurchaseClient(c.base, strings.Repeat("e", 64), dir)
	if err != nil || other.ticket != nil {
		t.Fatal("another device inherited order token", err)
	}
	other, err = newPurchaseClient("https://another.example.com", device, dir)
	if err != nil || other.ticket != nil {
		t.Fatal("order token sent to a different backend", err)
	}
	ticket.OrderID = "../admin"
	if c.saveTicket(ticket) == nil {
		t.Fatal("invalid order ID accepted")
	}
}

func TestPurchaseRequestsDoNotForwardTokensOnRedirect(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1); w.Write([]byte(`{}`)) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer srv.Close()
	device := strings.Repeat("f", 64)
	c, err := newPurchaseClient(srv.URL, device, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.status(purchaseTicket{ServerURL: c.base, DeviceID: device, OrderID: "WM-TEST-ORDER", Token: strings.Repeat("1", 64)})
	if err == nil || received.Load() != 0 {
		t.Fatal("token followed redirect to another server")
	}
}
