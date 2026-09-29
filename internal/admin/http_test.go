package admin

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"workmate/internal/entitlement"
)

// Keep the original placeholder independent of the owner's configured payment image.
//
//go:embed testdata/payment_qr_placeholder.png
var placeholderPaymentQR []byte

type adminFixture struct {
	store               *Store
	handler             http.Handler
	cookie              *http.Cookie
	csrf, password, dir string
}

func fixture(t *testing.T) *adminFixture {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b, err := os.ReadFile(filepath.Join(dir, "initial-password.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f := &adminFixture{store: s, handler: NewHandler(s, HTTPOptions{}), dir: dir, password: strings.TrimSpace(string(b))}
	r := f.request("POST", "/api/admin/login", map[string]string{"username": "admin", "password": f.password}, false)
	wantStatus(t, r, 200)
	var result struct {
		CSRF string `json:"csrf"`
	}
	if err = json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	f.csrf = result.CSRF
	f.cookie = r.Result().Cookies()[0]
	return f
}
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	img.Set(1, 1, color.RGBA{R: 80, G: 160, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func (f *adminFixture) request(method, path string, body any, auth bool) *httptest.ResponseRecorder {
	var b []byte
	raw, ok := body.([]byte)
	if ok {
		b = raw
	} else if body != nil {
		b, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:12345"
	if body != nil && !ok {
		r.Header.Set("Content-Type", "application/json")
	}
	if auth {
		r.AddCookie(f.cookie)
		r.Header.Set("X-CSRF-Token", f.csrf)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func wantStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP %d, expected %d: %s", w.Code, status, w.Body.String())
	}
}
func (f *adminFixture) ready(t *testing.T) {
	t.Helper()
	wantStatus(t, f.request("POST", "/api/admin/payment-qr", testPNG(t), true), 200)
	wantStatus(t, f.request("POST", "/api/admin/settings", Settings{Enabled: true, PriceCents: 1990, Contact: "开发者邮箱", TrialHours: 48}, true), 200)
}
func (f *adminFixture) pending(t *testing.T, device string) (Order, string) {
	t.Helper()
	o, token, err := f.store.CreateOrder(device)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.Submit(o.ID, token, Submission{Contact: "测试用户", Method: "wechat", Reference: "USER-TRANSACTION"}); err != nil {
		t.Fatal(err)
	}
	o, err = f.store.Order(o.ID)
	if err != nil {
		t.Fatal(err)
	}
	return o, token
}

func TestManualApprovalIsAtomicAndReceiptsCannotBeReused(t *testing.T) {
	f := fixture(t)
	f.ready(t)
	o, _ := f.pending(t, strings.Repeat("a", 64))
	bad := Decision{Revision: o.Revision, Confirmed: true, ReceivedCents: 1, Receipt: "ACTUAL-001"}
	if _, err := f.store.Decide(o.ID, "approve", bad); err == nil {
		t.Fatal("wrong payment amount approved")
	}
	bad.ReceivedCents = o.AmountCents
	bad.Confirmed = false
	if _, err := f.store.Decide(o.ID, "approve", bad); err == nil {
		t.Fatal("unconfirmed receipt approved")
	}
	bad.Confirmed = true
	var wg sync.WaitGroup
	codes := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			approved, err := f.store.Decide(o.ID, "approve", bad)
			if err != nil {
				errs <- err
			} else {
				codes <- approved.LicenseCode
			}
		}()
	}
	wg.Wait()
	close(codes)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var code string
	for current := range codes {
		if code != "" && code != current {
			t.Fatal("duplicate clicks signed different licenses")
		}
		code = current
	}
	var license entitlement.License
	if err := entitlement.Verify(code, f.store.PublicKey(), &license); err != nil || license.DeviceID != o.DeviceID || license.LicenseID != o.ID {
		t.Fatal("invalid device-bound entitlement", err)
	}
	var count int
	if err := f.store.db.QueryRow("SELECT count(*) FROM audit WHERE action='approve' AND order_id=?", o.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("approval was not recorded exactly once", count, err)
	}
	other, _ := f.pending(t, strings.Repeat("b", 64))
	bad.Revision = other.Revision
	if _, err := f.store.Decide(other.ID, "approve", bad); err == nil {
		t.Fatal("same receipt granted to a different device")
	}
	if _, err := f.store.Decide(o.ID, "reject", Decision{Revision: 99, Note: "撤销"}); err == nil {
		t.Fatal("permanent license silently revoked")
	}
}

func TestProofRevisionRejectionAndResubmission(t *testing.T) {
	f := fixture(t)
	f.ready(t)
	o, token := f.pending(t, strings.Repeat("c", 64))
	before := o.Revision
	r := httptest.NewRequest("POST", "/api/orders/"+o.ID+"/evidence", bytes.NewReader(testPNG(t)))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	wantStatus(t, w, 200)
	if _, err := f.store.Decide(o.ID, "approve", Decision{Revision: before, Confirmed: true, ReceivedCents: 1990, Receipt: "ACTUAL-002"}); err == nil {
		t.Fatal("stale evidence approved")
	}
	o, _ = f.store.Order(o.ID)
	rejected, err := f.store.Decide(o.ID, "reject", Decision{Revision: o.Revision, Note: "请补充付款时间"})
	if err != nil || rejected.Status != "rejected" {
		t.Fatal(err)
	}
	if err = f.store.Submit(o.ID, token, Submission{Contact: "user@example.com", Method: "wechat", Reference: "REF-NEW", Note: "已补充"}); err != nil {
		t.Fatal(err)
	}
	o, _ = f.store.Order(o.ID)
	if o.Status != "pending" || o.AdminNote != "" || !o.HasEvidence {
		t.Fatal("resubmission lost evidence or retained rejection")
	}
	approved, err := f.store.Decide(o.ID, "approve", Decision{Revision: o.Revision, Confirmed: true, ReceivedCents: 1990, Receipt: "ACTUAL-NEW"})
	if err != nil || approved.Status != "approved" {
		t.Fatal(err)
	}
	if err = f.store.Submit(o.ID, token, Submission{Contact: "other", Method: "wechat", Reference: "REF"}); err == nil {
		t.Fatal("approved order could be edited")
	}
}

func TestAdminAuthenticationCSRFAndOrderOwnership(t *testing.T) {
	f := fixture(t)
	f.ready(t)
	o, token := f.pending(t, strings.Repeat("d", 64))
	wantStatus(t, f.request("GET", "/api/admin/orders", nil, false), 401)
	wantStatus(t, f.request("GET", "/api/admin/orders/"+o.ID+"/evidence", nil, false), 401)
	wantStatus(t, f.request("GET", "/api/orders/"+o.ID, nil, false), 404)
	r := httptest.NewRequest("POST", "/api/admin/settings", strings.NewReader(`{"enabled":false,"price_cents":1990,"trial_hours":24,"contact":"owner"}`))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	wantStatus(t, w, 403)
	r.Header.Set("X-CSRF-Token", f.csrf)
	r.Header.Set("Origin", "https://attacker.example")
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	wantStatus(t, w, 403)
	r = httptest.NewRequest("GET", "/api/orders/"+o.ID, nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("0", 64))
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	wantStatus(t, w, 404)
	r = httptest.NewRequest("GET", "/api/orders/"+o.ID, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	wantStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "token_hash") || strings.Contains(w.Body.String(), token) {
		t.Fatal("order response leaked credential")
	}
	for i := 0; i < 4; i++ {
		wantStatus(t, f.request("POST", "/api/admin/login", map[string]string{"username": "admin", "password": "bad"}, false), 401)
	}
	wantStatus(t, f.request("POST", "/api/admin/login", map[string]string{"username": "admin", "password": "bad"}, false), 429)
	wantStatus(t, f.request("POST", "/api/admin/password", map[string]string{"old_password": f.password, "new_password": "changed-password-for-tests"}, true), 200)
	wantStatus(t, f.request("GET", "/api/admin/orders", nil, true), 401)
	if _, err := os.Stat(filepath.Join(f.dir, "initial-password.txt")); !os.IsNotExist(err) {
		t.Fatal("initial plaintext password retained after change")
	}
}

func TestSettingsPriceSnapshotImagesAndRestart(t *testing.T) {
	f := fixture(t)
	wantStatus(t, f.request("POST", "/api/admin/settings", Settings{Enabled: true, PriceCents: 1990, Contact: "owner", TrialHours: 24}, true), 400)
	wantStatus(t, f.request("POST", "/api/admin/payment-qr", []byte("not an image"), true), 400)
	wantStatus(t, f.request("POST", "/api/admin/payment-qr", bytes.Repeat([]byte{1}, (4<<20)+1), true), 400)
	wantStatus(t, f.request("POST", "/api/admin/payment-qr", placeholderPaymentQR, true), 400)
	f.ready(t)
	o, token := f.pending(t, strings.Repeat("e", 64))
	pub := f.store.PublicKey()
	if err := f.store.UpdateSettings(Settings{Enabled: true, PriceCents: 2990, Contact: "owner", TrialHours: 72}); err != nil {
		t.Fatal(err)
	}
	old, _ := f.store.Order(o.ID)
	if old.AmountCents != 1990 {
		t.Fatal("price edit changed an existing order")
	}
	cfg, err := f.store.Settings()
	if err != nil {
		t.Fatal(err)
	}
	p, err := entitlement.VerifyTrial(cfg.TrialPolicy, pub)
	if err != nil || p.Hours != 72 {
		t.Fatal("trial configuration lacks a valid signature", err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.PublicKey() != pub {
		t.Fatal("restart replaced release signing key")
	}
	got, err := reopened.OwnedOrder(o.ID, token)
	if err != nil || got.Status != "pending" || got.AmountCents != 1990 {
		t.Fatal("order lost after restart", err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(f.dir, "license-private.key")); err != nil {
		t.Fatal(err)
	}
	if lost, err := Open(f.dir); err == nil {
		lost.Close()
		t.Fatal("missing original key silently regenerated")
	}
}
