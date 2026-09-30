//go:build windows

package main

import (
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsFreeLedgerPreviewAndContextualUpgrade(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldApp, oldUI := app, onlinePurchase
	defer func() { app = oldApp; onlinePurchase = oldUI }()
	s := testStore(t)
	now := time.Now()
	day := now.Format("2006-01-02")
	s.Records = []DailyRecord{{Date: day, WorkMinutes: 120, OvertimeMinutes: 30}}
	h, _, _ := pGetModuleHandle.Call(0)
	app = appState{store: s, license: &LicenseManager{deviceID: strings.Repeat("a", 64), trialStart: now.Add(-25 * time.Hour), trialLastSeen: now}, hInst: HINSTANCE(h), reminderSeen: map[string]bool{}}
	onlinePurchase = nil
	registerWindowClasses()
	app.main = createMainWindow()
	if app.main == 0 {
		t.Fatal("main window failed")
	}
	defer func() {
		for _, wnd := range []HWND{dayEditWnd, purchaseWnd, ledgerWnd, calendarWnd, app.main} {
			if wnd != 0 {
				pDestroyWindow.Call(uintptr(wnd))
			}
		}
	}()
	openLedgerWindow()
	if ledgerWnd == 0 || getDlgItem(ledgerWnd, 610) == 0 {
		t.Fatal("free ledger preview did not open")
	}
	count, _, _ := pSendMessage.Call(uintptr(getDlgItem(ledgerWnd, 610)), lvmFirst+4, 0, 0)
	if count != 1 || ledgerSnapshot.Totals.OvertimeMinutes != 30 {
		t.Fatalf("free ledger did not show recorded day: %d", count)
	}
	ledgerWndProc(ledgerWnd, WM_COMMAND, 611, 0)
	if purchaseWnd == 0 || !strings.Contains(getText(getDlgItem(purchaseWnd, 519)), "导出工时账本") {
		t.Fatal("paid export did not explain its value")
	}
	if visible, _, _ := pIsWindowVisible.Call(uintptr(getDlgItem(purchaseWnd, 504))); visible == 0 {
		t.Fatal("offline activation should remain visible when no online backend is configured")
	}
	pDestroyWindow.Call(uintptr(purchaseWnd))
	openDayEdit(now)
	if dayEditWnd == 0 {
		t.Fatal("free record editor did not open")
	}
	if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(uintptr(getDlgItem(dayEditWnd, 301))); enabled == 0 {
		t.Fatal("free overtime input disabled")
	}
	if enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(uintptr(getDlgItem(dayEditWnd, 302))); enabled != 0 {
		t.Fatal("Pro leave input not disabled for free user")
	}
	pEnableWindow.Call(uintptr(getDlgItem(dayEditWnd, 302)), 0)
	setText(getDlgItem(dayEditWnd, 301), "1.25")
	dayEditWndProc(dayEditWnd, WM_COMMAND, 390, 0)
	if dayEditWnd != 0 || s.DayEntryAt(now).OvertimeHours != 1.25 {
		t.Fatal("free user could not save overtime correction")
	}
	refreshLedgerWindow(ledgerWnd)
	if ledgerSnapshot.Totals.OvertimeMinutes != 75 {
		t.Fatal("ledger preview did not refresh after free edit")
	}
	var window RECT
	pGetWindowRect.Call(uintptr(ledgerWnd), uintptr(unsafe.Pointer(&window)))
	if window.Right <= window.Left || window.Bottom <= window.Top {
		t.Fatal("ledger window has invalid bounds")
	}
}
