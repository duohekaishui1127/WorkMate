//go:build windows

package main

import (
	"image"
	"image/png"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func taskbarTestApp(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	oldApp, oldDock, oldParent, oldProbe := app, floatingDockState, floatingTaskbarParent, findTaskbarSlot
	s := testStore(t)
	t.Cleanup(func() {
		for _, h := range []HWND{settingsWnd, app.floating, app.main} {
			if h != 0 {
				pDestroyWindow.Call(uintptr(h))
			}
		}
		app, floatingDockState, floatingTaskbarParent, findTaskbarSlot = oldApp, oldDock, oldParent, oldProbe
		runtime.UnlockOSThread()
	})
	s.Settings.FloatingMode = "TaskbarEmbed"
	h, _, _ := pGetModuleHandle.Call(0)
	app = appState{store: s, license: &LicenseManager{pro: true, deviceID: strings.Repeat("a", 64)}, hInst: HINSTANCE(h), reminderSeen: map[string]bool{}}
	registerWindowClasses()
	app.main = createMainWindow()
	if app.main == 0 {
		t.Fatal("main window creation failed")
	}
}

func TestWindowsRealTaskbarEmbedding(t *testing.T) {
	taskbarTestApp(t)
	slot, ok := nativeTaskbarSlot("Medium")
	deadline := time.Now().Add(3 * time.Second)
	for !ok && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		slot, ok = nativeTaskbarSlot("Medium")
	}
	if !ok {
		t.Skip("no supported bottom taskbar gap on this desktop")
	}
	var before, after RECT
	pGetWindowRect.Call(uintptr(slot.parent), uintptr(unsafe.Pointer(&before)))
	toggleFloating(true)
	if app.floating == 0 || floatingTaskbarParent != slot.parent {
		t.Fatal("could not create actual taskbar child")
	}
	parent, _, _ := user32.NewProc("GetParent").Call(uintptr(app.floating))
	style, _, _ := user32.NewProc("GetWindowLongPtrW").Call(uintptr(app.floating), ^uintptr(15))
	if HWND(parent) != slot.parent || style&WS_CHILD == 0 || style&WS_POPUP != 0 {
		t.Fatal("widget is not embedded inside the shell taskbar")
	}
	restore := enterWindowDPI(slot.parent)
	r := floatingWindowRect()
	restore()
	if r.Left < int(before.Left) || r.Right > int(before.Right) || r.Top < int(before.Top) || r.Bottom > int(before.Bottom) {
		t.Fatalf("widget extends outside taskbar: %+v", r)
	}
	pUpdateWindow.Call(uintptr(app.floating)) // Exercise the native compact painter.
	pGetWindowRect.Call(uintptr(slot.parent), uintptr(unsafe.Pointer(&after)))
	if before != after {
		t.Fatal("embedding changed shell taskbar geometry")
	}
	t.Logf("real taskbar child=%v parent=%v screen rect=%+v", app.floating, slot.parent, r)
	if path := os.Getenv("WORKMATE_TASKBAR_CAPTURE"); path != "" {
		captureTaskbarWidget(t, path)
	}
	panicHide()
	syncTaskbarWidget()
	if visible, _, _ := pIsWindowVisible.Call(uintptr(app.floating)); visible != 0 {
		t.Fatal("taskbar sync undid panic hiding")
	}
}

func TestWindowsTaskbarLifecycleAndFallback(t *testing.T) {
	taskbarTestApp(t)
	makeHost := func() HWND { return createOwnedWindow(calendarClass, "Taskbar lifecycle test", 800, 60, 0) }
	host := makeHost()
	if host == 0 {
		t.Fatal("test taskbar host creation failed")
	}
	defer func() {
		if host != 0 {
			pDestroyWindow.Call(uintptr(host))
		}
	}()
	show(host, SW_SHOWNOACTIVATE)
	slot := taskbarSlot{host, desktopRect{500, 4, 740, 44}}
	available := true
	findTaskbarSlot = func(string) (taskbarSlot, bool) { return slot, available }
	toggleFloating(true)
	if floatingTaskbarParent != host {
		t.Fatal("test child not attached")
	}
	app.store.Settings.FloatingAutoHide = true
	applyFloatingSettings()
	if floatWndProc(app.floating, WM_NCHITTEST, 0, 0) != HTCLIENT || floatingDockState.edge != dockNone {
		t.Fatal("embedded widget can be dragged or separately auto-hidden")
	}
	// Parent replacement simulates Explorer rebuilding the taskbar, without stopping Explorer.
	pDestroyWindow.Call(uintptr(host))
	host = makeHost()
	slot.parent = host
	show(host, SW_SHOWNOACTIVATE)
	syncTaskbarWidget()
	if app.floating == 0 || floatingTaskbarParent != host || !app.floatingWanted {
		t.Fatal("visible widget not recreated after host replacement")
	}
	panicHide()
	pDestroyWindow.Call(uintptr(host))
	host = makeHost()
	slot.parent = host
	show(host, SW_SHOWNOACTIVATE)
	syncTaskbarWidget()
	if app.floating != 0 || app.floatingWanted {
		t.Fatal("host replacement resurrected a manually hidden widget")
	}
	toggleFloating(true)
	if floatingTaskbarParent != host {
		t.Fatal("manual restoration failed")
	}
	available = false
	syncTaskbarWidget()
	if app.floating == 0 || floatingTaskbarParent != 0 {
		t.Fatal("unsupported taskbar did not fall back to an ordinary widget")
	}
	wa := floatingWorkArea(app.floating)
	r := floatingWindowRect()
	if r.Bottom > wa.Bottom || r.Right > wa.Right {
		t.Fatalf("fallback widget is outside work area: %+v", r)
	}
	available = true
	syncTaskbarWidget()
	if floatingTaskbarParent != host {
		t.Fatal("widget did not re-embed when room became available")
	}
	pSendMessage.Call(uintptr(app.floating), WM_LBUTTONUP, 0, 12|(12<<16))
	if visible, _, _ := pIsWindowVisible.Call(uintptr(app.main)); visible == 0 {
		t.Fatal("click did not open main window")
	}
	pSendMessage.Call(uintptr(app.floating), WM_RBUTTONUP, 0, 0)
	if app.floatingWanted {
		t.Fatal("right-click did not hide widget")
	}
	toggleFloating(true)
	pSendMessage.Call(uintptr(app.floating), WM_LBUTTONUP, 0, 236|(12<<16))
	if app.floatingWanted {
		t.Fatal("close button did not hide widget")
	}
	toggleFloating(true)
	app.store.Settings.FloatingLeft, app.store.Settings.FloatingTop = 200, 200
	app.store.Settings.FloatingMode = "Screen"
	applyFloatingSettings()
	if floatingTaskbarParent != 0 || floatingWindowRect().width() != 340 {
		t.Fatal("switching back to desktop did not restore full widget")
	}
}

func TestWindowsTaskbarModeSettingsControl(t *testing.T) {
	taskbarTestApp(t)
	oldAutoStart := applyAutoStart
	defer func() { applyAutoStart = oldAutoStart }()
	applyAutoStart = func(bool) error { return nil }
	settingsWnd = createOwnedWindow(settingsClass, "Taskbar settings test", 920, 666, app.main)
	if selectedChoice(settingsWnd, 121, floatingModeChoices) != "TaskbarEmbed" {
		t.Fatal("embedded mode not selected in settings")
	}
	for _, id := range []int{126, 127, 128, 129} {
		if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(uintptr(getDlgItem(settingsWnd, id))); enabled != 0 {
			t.Fatal("desktop-only option enabled in taskbar mode")
		}
	}
	pSendMessage.Call(uintptr(getDlgItem(settingsWnd, 121)), CB_SETCURSEL, 1, 0)
	pSendMessage.Call(uintptr(settingsWnd), WM_COMMAND, 121|(1<<16), uintptr(getDlgItem(settingsWnd, 121)))
	for _, id := range []int{126, 127, 128, 129} {
		if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(uintptr(getDlgItem(settingsWnd, id))); enabled == 0 {
			t.Fatal("desktop option did not re-enable after switching modes")
		}
	}
	saveSettingsFromWindow(settingsWnd)
	loaded, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.FloatingMode != "Taskbar" {
		t.Fatal("position dropdown did not save")
	}
}

// Optional visual evidence captures only the test widget's rectangle.
func captureTaskbarWidget(t *testing.T, path string) {
	t.Helper()
	restore := enterWindowDPI(floatingTaskbarParent)
	defer restore()
	r := floatingWindowRect()
	w, h := r.width(), r.height()
	screen, _, _ := user32.NewProc("GetDC").Call(0)
	if screen == 0 {
		t.Fatal("screen DC creation failed")
	}
	defer user32.NewProc("ReleaseDC").Call(0, screen)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	if mem == 0 {
		t.Fatal("capture DC creation failed")
	}
	defer pDeleteDC.Call(mem)
	bmi := BITMAPINFO{Header: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var bits unsafe.Pointer
	bitmap, _, _ := pCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		t.Fatal("capture bitmap creation failed")
	}
	old, _, _ := pSelectObject.Call(mem, bitmap)
	defer func() { pSelectObject.Call(mem, old); pDeleteObject.Call(bitmap) }()
	dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmFlush")
	dwm.Call()
	if ok, _, _ := pBitBlt.Call(mem, 0, 0, uintptr(w), uintptr(h), screen, uintptr(r.Left), uintptr(r.Top), SRCCOPY|0x40000000); ok == 0 {
		t.Fatal("widget capture failed")
	}
	pGdiFlush.Call()
	raw := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(raw); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = raw[i+2], raw[i+1], raw[i], 255
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
