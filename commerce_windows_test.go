//go:build windows

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"workmate/internal/entitlement"
)

func TestWindowsPurchaseReceivesAuthorizationWithoutBlockingUI(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldApp, oldUI, oldKey := app, onlinePurchase, onlineLicensePublicKeyB64
	defer func() { app = oldApp; onlinePurchase = oldUI; onlineLicensePublicKeyB64 = oldKey }()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	onlineLicensePublicKeyB64 = base64.StdEncoding.EncodeToString(pub)
	device := strings.Repeat("a", 64)
	code, _ := entitlement.Sign(entitlement.License{Version: 1, LicenseID: "WM-WINDOWS-TEST", Edition: "pro", DeviceID: device, IssuedAt: time.Now().UTC().Format(time.RFC3339)}, key)
	policy, _ := entitlement.Sign(entitlement.TrialPolicy{Version: 1, Kind: "trial-policy", Hours: 48, IssuedAt: time.Now().UTC().Format(time.RFC3339)}, key)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		if r.URL.Path == "/api/config" {
			json.NewEncoder(w).Encode(onlineConfig{Enabled: true, PriceCents: 1990, Contact: "test", QRAvailable: true, TrialHours: 48, TrialPolicy: policy})
			return
		}
		json.NewEncoder(w).Encode(onlineOrder{ID: "WM-WINDOWS-TEST", DeviceID: device, Status: "approved", AmountCents: 1990, LicenseCode: code})
	}))
	defer srv.Close()
	s := testStore(t)
	client, err := newPurchaseClient(srv.URL, device, s.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.saveTicket(purchaseTicket{ServerURL: srv.URL, DeviceID: device, OrderID: "WM-WINDOWS-TEST", Token: strings.Repeat("1", 64)}); err != nil {
		t.Fatal(err)
	}
	h, _, _ := pGetModuleHandle.Call(0)
	app = appState{store: s, license: &LicenseManager{dataDir: s.DataDir, deviceID: device, trialStart: time.Now(), trialLastSeen: time.Now(), lastPersist: time.Now()}, hInst: HINSTANCE(h), reminderSeen: map[string]bool{}}
	onlinePurchase = &onlinePurchaseUI{client: client, results: make(chan purchaseResult, 2), nextSync: time.Now().Add(time.Hour), status: "测试订单"}
	registerWindowClasses()
	app.main = createMainWindow()
	if app.main == 0 {
		t.Fatal("cannot create main window")
	}
	defer func() {
		if purchaseWnd != 0 {
			pDestroyWindow.Call(uintptr(purchaseWnd))
		}
		pDestroyWindow.Call(uintptr(app.main))
	}()
	purchaseWnd = createOwnedWindow(purchaseClass, "Online purchase test", 620, 674, app.main)
	if purchaseWnd == 0 {
		t.Fatal("cannot create purchase window")
	}
	show(purchaseWnd, SW_SHOWNOACTIVATE)
	if getDlgItem(purchaseWnd, 520) == 0 || getDlgItem(purchaseWnd, 523) == 0 || getDlgItem(purchaseWnd, 514) == 0 {
		t.Fatal("free and Pro feature comparison or backup activation entry missing")
	}
	if visible, _, _ := pIsWindowVisible.Call(uintptr(getDlgItem(purchaseWnd, 504))); visible != 0 {
		t.Fatal("manual code entry was not hidden in the online flow")
	}
	var clientRect RECT
	var origin POINT
	pGetClientRect.Call(uintptr(purchaseWnd), uintptr(unsafe.Pointer(&clientRect)))
	user32.NewProc("ClientToScreen").Call(uintptr(purchaseWnd), uintptr(unsafe.Pointer(&origin)))
	for _, id := range []int{501, 502, 503, 504, 505, 506, 507, 508, 509, 510, 511, 512, 513} {
		control := getDlgItem(purchaseWnd, id)
		if control == 0 {
			t.Fatalf("missing purchase control %d", id)
		}
		if visible, _, _ := pIsWindowVisible.Call(uintptr(control)); visible == 0 {
			continue
		}
		var r RECT
		pGetWindowRect.Call(uintptr(control), uintptr(unsafe.Pointer(&r)))
		if r.Left < origin.X || r.Top < origin.Y || r.Right > origin.X+clientRect.Right || r.Bottom > origin.Y+clientRect.Bottom {
			t.Fatalf("purchase control %d outside window", id)
		}
	}
	setPurchaseAdvanced(purchaseWnd, true)
	user32.NewProc("ClientToScreen").Call(uintptr(purchaseWnd), uintptr(unsafe.Pointer(&origin)))
	pGetClientRect.Call(uintptr(purchaseWnd), uintptr(unsafe.Pointer(&clientRect)))
	for _, id := range []int{502, 503, 504, 505, 513, 516, 517, 518} {
		var r RECT
		pGetWindowRect.Call(uintptr(getDlgItem(purchaseWnd, id)), uintptr(unsafe.Pointer(&r)))
		if r.Top < origin.Y || r.Bottom > origin.Y+clientRect.Bottom {
			t.Fatalf("expanded backup activation control %d outside window", id)
		}
	}
	start := time.Now()
	syncOnlinePurchase(start, true)
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("network request blocked UI thread")
	}
	deadline := time.Now().Add(3 * time.Second)
	for onlinePurchase.busy && time.Now().Before(deadline) {
		drainOnlinePurchase()
		time.Sleep(10 * time.Millisecond)
	}
	if onlinePurchase.busy || !app.license.IsPro() {
		t.Fatal("native window did not apply approved authorization")
	}
	mainWndProc(app.main, WM_TIMER, timerMain, 0)
	if !strings.Contains(getText(getDlgItem(purchaseWnd, 507)), "永久版") || !strings.Contains(getText(getDlgItem(purchaseWnd, 511)), "解锁") {
		t.Fatal("purchase window did not reflect activation")
	}
}
