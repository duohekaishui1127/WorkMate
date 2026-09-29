//go:build windows

package main

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWindowsFloatingDockAndPanicHide(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	oldApp, oldDock, oldDay := app, floatingDockState, dayEditDate
	defer func() { app, floatingDockState, dayEditDate = oldApp, oldDock, oldDay }()
	s := testStore(t)
	s.Settings.FloatingAutoHide = true
	h, _, _ := pGetModuleHandle.Call(0)
	app = appState{store: s, license: &LicenseManager{pro: true, deviceID: strings.Repeat("a", 64)}, hInst: HINSTANCE(h), reminderSeen: map[string]bool{}, calendarMonth: time.Now()}
	registerWindowClasses()
	app.main = createMainWindow()
	if app.main == 0 {
		t.Fatal("main window creation failed")
	}
	defer func() {
		for _, hwnd := range []HWND{dayEditWnd, settingsWnd, calendarWnd, timelineWnd, purchaseWnd, app.floating, app.main} {
			if hwnd != 0 {
				pDestroyWindow.Call(uintptr(hwnd))
			}
		}
	}()
	toggleFloating(true)
	if app.floating == 0 {
		t.Fatal("floating window creation failed")
	}
	w, height := floatingSize()
	work := floatingWorkArea(app.floating)
	x, y := work.Left+(work.width()-w)/2, work.Top+(work.height()-height)/2
	now := testDate("2026-09-29 10:00")
	for _, tc := range []struct {
		name string
		r    desktopRect
		edge dockEdge
	}{
		{"left", desktopRect{work.Left, y, work.Left + w, y + height}, dockLeft},
		{"right", desktopRect{work.Right - w, y, work.Right, y + height}, dockRight},
		{"top", desktopRect{x, work.Top, x + w, work.Top + height}, dockTop},
		{"bottom", desktopRect{x, work.Bottom - height, x + w, work.Bottom}, dockBottom},
	} {
		t.Log("native dock:", tc.name)
		floatWndProc(app.floating, WM_ENTERSIZEMOVE, 0, 0)
		positionFloating(tc.r)
		floatWndProc(app.floating, WM_EXITSIZEMOVE, 0, 0)
		if floatingDockState.edge != tc.edge {
			t.Fatalf("native docking chose %v, want %v", floatingDockState.edge, tc.edge)
		}
		updateFloatingDock(now, work.Right+100, work.Bottom+100)
		updateFloatingDock(now.Add(floatingDockDelay), work.Right+100, work.Bottom+100)
		strip := floatingWindowRect()
		if !floatingDockState.collapsed {
			t.Fatal("native window did not collapse")
		}
		if tc.edge == dockLeft || tc.edge == dockRight {
			if strip.width() != 4 || strip.height() != height {
				t.Fatalf("unexpected native strip: %+v", strip)
			}
		} else if strip.height() != 4 || strip.width() != w {
			t.Fatalf("unexpected native strip: %+v", strip)
		}
		updateFloatingDock(now.Add(time.Second), (strip.Left+strip.Right)/2, (strip.Top+strip.Bottom)/2)
		if floatingDockState.collapsed || floatingWindowRect() != tc.r {
			t.Fatalf("hover failed to restore native geometry: %+v", floatingWindowRect())
		}
	}

	style := func() uintptr {
		v, _, _ := user32.NewProc("GetWindowLongPtrW").Call(uintptr(app.floating), ^uintptr(19))
		return v
	}
	if style()&WS_EX_TOPMOST == 0 || style()&WS_EX_NOACTIVATE == 0 {
		t.Fatal("default topmost/no-activate styles missing")
	}
	s.Settings.FloatingTopmost = false
	applyFloatingSettings()
	if style()&WS_EX_TOPMOST != 0 {
		t.Fatal("topmost was not disabled immediately")
	}
	s.Settings.FloatingTopmost = true
	applyFloatingSettings()
	if style()&WS_EX_TOPMOST == 0 {
		t.Fatal("topmost was not enabled immediately")
	}

	// Create every auxiliary window, then exercise the real global hotkey handler.
	settingsWnd = createOwnedWindow(settingsClass, "Settings test", 920, 666, app.main)
	calendarWnd = createOwnedWindow(calendarClass, "Calendar test", 850, 690, app.main)
	timelineWnd = createOwnedWindow(timelineClass, "Timeline test", 580, 530, app.main)
	dayEditDate = time.Now()
	dayEditWnd = createOwnedWindow(dayEditClass, "Day test", 470, 430, calendarWnd)
	purchaseWnd = createOwnedWindow(purchaseClass, "Purchase test", 620, 614, app.main)
	all := []HWND{app.main, settingsWnd, calendarWnd, timelineWnd, dayEditWnd, purchaseWnd, app.floating}
	for _, hwnd := range all {
		if hwnd == 0 {
			t.Fatal("auxiliary window creation failed")
		}
		show(hwnd, SW_SHOWNOACTIVATE)
		if visible, _, _ := pIsWindowVisible.Call(uintptr(hwnd)); visible == 0 {
			t.Fatal("test window not visible before panic")
		}
	}
	mainWndProc(app.main, WM_HOTKEY, hotkeyPanic, 0)
	for _, hwnd := range all {
		if visible, _, _ := pIsWindowVisible.Call(uintptr(hwnd)); visible != 0 {
			t.Fatalf("panic left window visible: %v", hwnd)
		}
	}
	strip := collapsedFloatingRect(floatingDockState.normal, floatingDockState.edge)
	updateFloatingDock(now.Add(time.Second), strip.Left, strip.Top)
	floatWndProc(app.floating, WM_TIMER, timerFloatingDock, 0)
	applyFloatingSettings()
	if visible, _, _ := pIsWindowVisible.Call(uintptr(app.floating)); visible != 0 {
		t.Fatal("hover/timer/settings resurrected explicitly hidden widget")
	}
	toggleFloating(false)
	if visible, _, _ := pIsWindowVisible.Call(uintptr(app.floating)); visible == 0 || floatingDockState.collapsed {
		t.Fatal("single tray toggle did not restore hidden widget")
	}
	pSendMessage.Call(uintptr(app.floating), WM_NCRBUTTONDOWN, HTCAPTION, 0)
	pSendMessage.Call(uintptr(app.floating), WM_NCRBUTTONUP, HTCAPTION, 0)
	if visible, _, _ := pIsWindowVisible.Call(uintptr(app.floating)); visible != 0 {
		t.Fatal("right-click on draggable body did not hide widget")
	}
	toggleFloating(true)
	updateFloatingDock(now, work.Right+100, work.Bottom+100)
	updateFloatingDock(now.Add(floatingDockDelay), work.Right+100, work.Bottom+100)
	if !floatingDockState.collapsed {
		t.Fatal("widget not collapsed before persistence test")
	}
	normal := floatingDockState.normal
	pDestroyWindow.Call(uintptr(app.floating))
	loaded, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.FloatingLeft != normal.Left || loaded.Settings.FloatingTop != normal.Top {
		t.Fatal("saved collapsed strip position instead of full window position")
	}
	toggleFloating(true)
	if floatingWindowRect() != normal {
		t.Fatalf("recreated widget geometry differs: %+v, want %+v", floatingWindowRect(), normal)
	}
	updateFloatingDock(now, work.Right+100, work.Bottom+100)
	updateFloatingDock(now.Add(floatingDockDelay), work.Right+100, work.Bottom+100)
	s.Settings.FloatingAutoHide = false
	applyFloatingSettings()
	if floatingDockState.collapsed || floatingWindowRect() != normal {
		t.Fatal("disabling auto-hide did not restore full window")
	}
}
