//go:build windows

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsUISettingsAndReports(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldApp, oldAutoStart := app, applyAutoStart
	defer func() { app = oldApp; applyAutoStart = oldAutoStart }()
	s := testStore(t)
	h, _, _ := pGetModuleHandle.Call(0)
	app = appState{store: s, license: &LicenseManager{pro: true, deviceID: strings.Repeat("a", 64)}, hInst: HINSTANCE(h), reminderSeen: map[string]bool{}}
	registerWindowClasses()
	app.main = createMainWindow()
	if app.main == 0 {
		t.Fatal("main window creation failed")
	}
	defer func() {
		for _, wnd := range []HWND{settingsWnd, purchaseWnd, app.floating, app.main} {
			if wnd != 0 {
				pDestroyWindow.Call(uintptr(wnd))
			}
		}
	}()
	settingsWnd = createOwnedWindow(settingsClass, "Settings test", 920, 666, app.main)
	if settingsWnd == 0 {
		t.Fatal("settings window creation failed")
	}
	var client RECT
	pGetClientRect.Call(uintptr(settingsWnd), uintptr(unsafe.Pointer(&client)))
	var origin POINT
	user32.NewProc("ClientToScreen").Call(uintptr(settingsWnd), uintptr(unsafe.Pointer(&origin)))
	ids := []int{100, 101, 102, 103, 104, 105, 106, 107, 108, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119, 120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 190, 191}
	for _, id := range ids {
		ctl := getDlgItem(settingsWnd, id)
		if ctl == 0 {
			t.Fatalf("missing settings control %d", id)
		}
		var r RECT
		pGetWindowRect.Call(uintptr(ctl), uintptr(unsafe.Pointer(&r)))
		if r.Left < origin.X || r.Top < origin.Y || r.Right > origin.X+client.Right || r.Bottom > origin.Y+client.Bottom {
			t.Fatalf("control %d lies outside client: %+v, origin=%+v client=%+v", id, r, origin, client)
		}
	}
	var save, last RECT
	pGetWindowRect.Call(uintptr(getDlgItem(settingsWnd, 190)), uintptr(unsafe.Pointer(&save)))
	pGetWindowRect.Call(uintptr(getDlgItem(settingsWnd, 129)), uintptr(unsafe.Pointer(&last)))
	if save.Top < last.Bottom {
		t.Fatal("save button overlaps last settings checkbox")
	}
	if getText(getDlgItem(settingsWnd, 122)) != "16:30" {
		t.Fatal("reminder time not initialized")
	}
	for _, id := range []int{114, 115, 120, 126, 127, 128} {
		pSendMessage.Call(uintptr(getDlgItem(settingsWnd, id)), BM_SETCHECK, BST_CHECKED, 0)
	}
	pSendMessage.Call(uintptr(getDlgItem(settingsWnd, 129)), BM_SETCHECK, 0, 0)
	setText(getDlgItem(settingsWnd, 122), "17:45")
	setText(getDlgItem(settingsWnd, 123), "65")
	pSendMessage.Call(uintptr(getDlgItem(settingsWnd, 124)), CB_SETCURSEL, 2, 0)
	pSendMessage.Call(uintptr(getDlgItem(settingsWnd, 125)), CB_SETCURSEL, 2, 0)
	autoStartCalled := false
	applyAutoStart = func(bool) error { autoStartCalled = true; return nil } // Never modify the user's Run registry value.
	saveSettingsFromWindow(settingsWnd)
	if settingsWnd != 0 || !autoStartCalled {
		t.Fatal("settings save did not complete")
	}
	loaded, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	v := loaded.Settings
	if !v.FloatingAutoHide || v.FloatingTopmost || !v.StartHidden || !v.ShowFloatingOnStartup || !v.ShowFloatingOnAutoStart || !v.FloatingShowPhrase || !v.FloatingCompact || v.FloatingSize != "Large" || v.FloatingPalette != "Mint" || v.FloatingOpacity != .65 || v.ReminderEveningHour != 17 || v.ReminderEveningMinute != 45 {
		t.Fatalf("UI settings not persisted: %+v", v)
	}
	// Exercise the real timer handler at expiry without loading trial registry anchors.
	proLicense := app.license
	app.license = &LicenseManager{dataDir: s.DataDir, deviceID: strings.Repeat("a", 64), trialStart: time.Now().Add(-trialDuration), trialLastSeen: time.Now(), lastPersist: time.Now()}
	mainWndProc(app.main, WM_TIMER, timerMain, 0)
	if purchaseWnd == 0 {
		t.Fatal("timer did not open purchase window at expiry")
	}
	if visible, _, _ := pIsWindowVisible.Call(uintptr(purchaseWnd)); visible == 0 {
		t.Fatal("expiry purchase window is not visible")
	}
	pDestroyWindow.Call(uintptr(purchaseWnd))
	mainWndProc(app.main, WM_TIMER, timerMain, 0)
	if purchaseWnd != 0 {
		t.Fatal("timer reopened dismissed purchase window")
	}
	app.license = proLicense
	// Create the purchase window without showing it or opening a payment image.
	purchaseWnd = createOwnedWindow(purchaseClass, "Purchase test", 620, 614, app.main)
	if purchaseWnd == 0 {
		t.Fatal("purchase window creation failed")
	}
	if !strings.Contains(getText(getDlgItem(purchaseWnd, 508)), "尚未开放") {
		t.Fatal("unconfigured purchase entry advertised as open")
	}
	if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(uintptr(getDlgItem(purchaseWnd, 501))); enabled != 0 {
		t.Fatal("unconfigured payment button enabled")
	}
	if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(uintptr(getDlgItem(purchaseWnd, 505))); enabled == 0 {
		t.Fatal("existing activation codes blocked")
	}
	for _, tc := range []struct {
		kind   string
		height int
	}{{"daily", 1380}, {"month", 1780}, {"year", 2200}} {
		path := filepath.Join(t.TempDir(), tc.kind+".png")
		if err := renderShareCard(path, tc.kind, time.Now()); err != nil {
			t.Fatalf("%s report: %v", tc.kind, err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 1080 || img.Bounds().Dy() != tc.height {
			t.Fatalf("%s report size: %v", tc.kind, img.Bounds())
		}
		r, g, b, _ := img.At(0, 0).RGBA()
		if r == 0 && g == 0 && b == 0 {
			t.Fatalf("%s report drawing buffer was empty", tc.kind)
		}
	}
	// Resizing an existing widget must apply compact mode immediately.
	toggleFloating(true)
	if app.floating == 0 {
		t.Fatal("floating window creation failed")
	}
	var rect RECT
	pGetWindowRect.Call(uintptr(app.floating), uintptr(unsafe.Pointer(&rect)))
	if rect.Right-rect.Left != 420 || rect.Bottom-rect.Top != 84 {
		t.Fatalf("widget settings not applied: %+v", rect)
	}
}

func TestWindowsClipboardMemoryCopy(t *testing.T) {
	if err := pRtlMoveMemory.Find(); err != nil {
		t.Fatal(err)
	}
	data := syscallStringToUTF16("WorkMate 设备码测试")
	mem, _, _ := pGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(len(data)*2))
	if mem == 0 {
		t.Fatal("GlobalAlloc failed")
	}
	defer pGlobalFree.Call(mem)
	ptr, _, _ := pGlobalLock.Call(mem)
	if ptr == 0 {
		t.Fatal("GlobalLock failed")
	}
	defer pGlobalUnlock.Call(mem)
	pRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	got := make([]uint16, len(data))
	pRtlMoveMemory.Call(uintptr(unsafe.Pointer(&got[0])), ptr, uintptr(len(data)*2))
	for i, v := range data {
		if got[i] != v {
			t.Fatal("UTF-16 clipboard memory copy failed")
		}
	}
}
