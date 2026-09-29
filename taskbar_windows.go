//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"
)

type taskbarSlot struct {
	parent HWND
	rect   desktopRect // Relative to the taskbar client area.
}

var floatingTaskbarParent HWND
var findTaskbarSlot = nativeTaskbarSlot
var taskbarCreatedMessage uint32

type taskbarProbe struct {
	origin        POINT
	buttons, tray desktopRect
	occupied      []desktopRect
}

var activeTaskbarProbe *taskbarProbe
var taskbarEnumCallback = syscall.NewCallback(enumTaskbarChild)

func enumTaskbarChild(hwnd HWND, _ uintptr) uintptr {
	p := activeTaskbarProbe
	if p == nil {
		return 1
	}
	visible, _, _ := pIsWindowVisible.Call(uintptr(hwnd))
	if visible == 0 {
		return 1
	}
	var name [256]uint16
	pGetClassName.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	class := syscall.UTF16ToString(name[:])
	switch class {
	case floatClass, "Windows.UI.Composition.DesktopWindowContentBridge", "Windows.UI.Input.InputSite.WindowClass", "Windows.UI.Core.CoreWindow":
		return 1 // Background surfaces; the actual app-button bounds are separate.
	}
	var wr RECT
	if ok, _, _ := pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr))); ok == 0 {
		return 1
	}
	r := desktopRect{int(wr.Left - p.origin.X), int(wr.Top - p.origin.Y), int(wr.Right - p.origin.X), int(wr.Bottom - p.origin.Y)}
	if r.width() <= 0 || r.height() <= 0 {
		return 1
	}
	switch class {
	case "MSTaskListWClass":
		p.buttons = r
	case "TrayNotifyWnd":
		p.tray = r
	}
	p.occupied = append(p.occupied, r)
	return 1
}

// Read the taskbar layout without changing its buttons, tray, or settings.
func nativeTaskbarSlot(size string) (taskbarSlot, bool) {
	h, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(u16("Shell_TrayWnd"))), 0)
	if h == 0 {
		return taskbarSlot{}, false
	}
	parent := HWND(h)
	// Use the shell's DPI context for both measurement and child-window creation.
	restore := enterWindowDPI(parent)
	defer restore()
	var wr, client RECT
	if ok, _, _ := pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr))); ok == 0 {
		return taskbarSlot{}, false
	}
	monitor, _, _ := pMonitorFromWindow.Call(h, MONITOR_DEFAULTTONEAREST)
	info := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
	if ok, _, _ := pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 || wr.Top < info.Monitor.Top+(info.Monitor.Bottom-info.Monitor.Top)/2 {
		return taskbarSlot{}, false // Only horizontal bottom taskbars are supported.
	}
	if ok, _, _ := pGetClientRect.Call(h, uintptr(unsafe.Pointer(&client))); ok == 0 {
		return taskbarSlot{}, false
	}
	probe := taskbarProbe{}
	pClientToScreen.Call(h, uintptr(unsafe.Pointer(&probe.origin)))
	activeTaskbarProbe = &probe
	pEnumChildWindows.Call(h, taskbarEnumCallback, 0)
	activeTaskbarProbe = nil
	bridge, _, _ := pFindWindowEx.Call(h, 0, uintptr(unsafe.Pointer(u16("Windows.UI.Composition.DesktopWindowContentBridge"))), 0)
	if bridge != 0 {
		// Windows 11's legacy task-list HWND can be hidden and have stale bounds.
		// The XAML subtree excludes our sibling widget, avoiding self-UI queries.
		controls, ready := cachedTaskbarButtons(HWND(bridge), parent)
		if !ready {
			return taskbarSlot{}, false
		}
		probe.buttons = desktopRect{}
		for _, wr := range controls {
			r := desktopRect{int(wr.Left - probe.origin.X), int(wr.Top - probe.origin.Y), int(wr.Right - probe.origin.X), int(wr.Bottom - probe.origin.Y)}
			if r.Top >= int(client.Bottom) || r.Bottom <= 0 {
				continue
			}
			probe.occupied = append(probe.occupied, r)
			if r.Right <= probe.tray.Left && r.Right > probe.buttons.Right {
				probe.buttons = r
			}
		}
	}
	bar := desktopRect{int(client.Left), int(client.Top), int(client.Right), int(client.Bottom)}
	r, ok := taskbarEmbedRect(bar, probe.buttons, probe.tray, probe.occupied, taskbarWidgetWidth(size))
	return taskbarSlot{parent, r}, ok
}

func enterWindowDPI(hwnd HWND) func() {
	if pGetWindowDpiAwarenessContext.Find() != nil || pSetThreadDpiAwarenessContext.Find() != nil {
		return func() {}
	}
	context, _, _ := pGetWindowDpiAwarenessContext.Call(uintptr(hwnd))
	previous, _, _ := pSetThreadDpiAwarenessContext.Call(context)
	return func() {
		if previous != 0 {
			pSetThreadDpiAwarenessContext.Call(previous)
		}
	}
}

func syncTaskbarWidget() {
	if !app.exiting && app.store.Settings.FloatingMode == "TaskbarEmbed" && (app.floatingWanted || app.floating != 0) {
		applyFloatingSettings()
	}
}

func applyTaskbarSlot(slot taskbarSlot) bool {
	if app.floating != 0 && floatingTaskbarParent != slot.parent {
		pDestroyWindow.Call(uintptr(app.floating))
	}
	created := app.floating == 0
	if created && !createFloatingWindow(slot.parent, slot.rect) {
		return false
	}
	restore := enterWindowDPI(slot.parent)
	defer restore()
	floatingDockState = floatingDock{normal: slot.rect}
	r := slot.rect
	pSetWindowPos.Call(uintptr(app.floating), 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), SWP_NOACTIVATE|SWP_NOOWNERZORDER)
	pKillTimer.Call(uintptr(app.floating), timerFloatingDock)
	setFloatingOpacity()
	if created && app.floatingWanted {
		show(app.floating, SW_SHOWNOACTIVATE)
	}
	invalidate(app.floating)
	return true
}

func paintTaskbarFloating(hwnd HWND) {
	var ps PAINTSTRUCT
	hdc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	var r RECT
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	bg, fg, ac := floatPalette()
	fill(HDC(hdc), r, bg)
	now := time.Now()
	top, detail := floatingText(now)
	if r.Bottom >= 36 {
		mid := (r.Bottom - 3) / 2
		drawText(HDC(hdc), top, RECT{8, 0, r.Right - 24, mid}, 12, FW_BOLD, fg, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(HDC(hdc), detail, RECT{8, mid, r.Right - 24, r.Bottom - 3}, 10, FW_NORMAL, fg, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	} else {
		if detail != "" {
			top += " · " + detail
		}
		drawText(HDC(hdc), top, RECT{8, 0, r.Right - 24, r.Bottom - 3}, 10, FW_BOLD, fg, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	}
	if app.store.Settings.FloatingShowProgress && !app.store.DayInfo(now).IsRestDay {
		_, progress := app.store.TodayEarned(now)
		bar := RECT{8, r.Bottom - 3, r.Right - 24, r.Bottom - 1}
		bar.Right = bar.Left + int32(float64(bar.Right-bar.Left)*progress)
		if progress > 0 {
			fill(HDC(hdc), bar, ac)
		}
	}
	drawText(HDC(hdc), "×", RECT{r.Right - 23, 0, r.Right - 2, r.Bottom}, 12, FW_NORMAL, fg, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}
