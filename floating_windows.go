//go:build windows

package main

import (
	"fmt"
	"time"
	"unsafe"
)

const timerFloatingDock = 2

var floatingDockState floatingDock

func floatingSize() (int, int) { return floatingDimensions(app.store.Settings) }
func toggleFloating(forceShow bool) {
	if app.floating != 0 {
		visible, _, _ := pIsWindowVisible.Call(uintptr(app.floating))
		if forceShow || visible == 0 || floatingDockState.collapsed {
			restoreFloating()
			return
		}
		pDestroyWindow.Call(uintptr(app.floating))
		return
	}
	w, h := floatingSize()
	x, y := app.store.Settings.FloatingLeft, app.store.Settings.FloatingTop
	wa := floatingWorkArea(0)
	// Negative coordinates are valid on monitors to the left or above the primary.
	if app.store.Settings.FloatingMode == "Taskbar" || (x == -1 && y == -1) {
		x, y = wa.Right-w-18, wa.Bottom-h-12
	}
	ex := uintptr(WS_EX_TOOLWINDOW | WS_EX_LAYERED | WS_EX_NOACTIVATE)
	if app.store.Settings.FloatingTopmost {
		ex |= WS_EX_TOPMOST
	}
	floatingDockState = floatingDock{}
	r, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(u16(floatClass))), uintptr(unsafe.Pointer(u16("WorkMate 挂件"))), WS_POPUP, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(app.main), 0, uintptr(app.hInst), 0)
	app.floating = HWND(r)
	if app.floating == 0 {
		return
	}
	applyFloatingSettings()
	show(app.floating, SW_SHOWNOACTIVATE)
}

func floatingWorkArea(hwnd HWND) desktopRect {
	if hwnd != 0 {
		monitor, _, _ := pMonitorFromWindow.Call(uintptr(hwnd), MONITOR_DEFAULTTONEAREST)
		info := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
		if ok, _, _ := pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
			return desktopRect{int(info.Work.Left), int(info.Work.Top), int(info.Work.Right), int(info.Work.Bottom)}
		}
	}
	var wa RECT
	pSystemParametersInfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	return desktopRect{int(wa.Left), int(wa.Top), int(wa.Right), int(wa.Bottom)}
}

func floatingWindowRect() desktopRect {
	var wr RECT
	pGetWindowRect.Call(uintptr(app.floating), uintptr(unsafe.Pointer(&wr)))
	return desktopRect{int(wr.Left), int(wr.Top), int(wr.Right), int(wr.Bottom)}
}

func positionFloating(r desktopRect) {
	pSetWindowPos.Call(uintptr(app.floating), 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), SWP_NOZORDER|SWP_NOACTIVATE|SWP_NOOWNERZORDER)
	invalidate(app.floating)
}

func restoreFloating() {
	floatingDockState.collapsed = false
	floatingDockState.outsideSince = time.Time{}
	positionFloating(floatingDockState.normal)
	show(app.floating, SW_SHOWNOACTIVATE)
}

func hideFloating() {
	if app.floating != 0 {
		floatingDockState.outsideSince = time.Time{}
		show(app.floating, SW_HIDE)
	}
}

func updateFloatingDock(now time.Time, x, y int) {
	if app.floating == 0 || !app.store.Settings.FloatingAutoHide {
		return
	}
	// Explicit hide (including the panic key) must never be reversed by hovering.
	if visible, _, _ := pIsWindowVisible.Call(uintptr(app.floating)); visible == 0 {
		floatingDockState.outsideSince = time.Time{}
		return
	}
	if floatingDockState.update(now, x, y) {
		positionFloating(floatingDockState.visibleRect())
	}
}

func pollFloatingDock() {
	var pt POINT
	if ok, _, _ := pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); ok != 0 {
		updateFloatingDock(time.Now(), int(pt.X), int(pt.Y))
	}
}

func saveFloatingPosition() {
	if app.store.Settings.FloatingMode != "Taskbar" {
		app.store.Settings.FloatingLeft = floatingDockState.normal.Left
		app.store.Settings.FloatingTop = floatingDockState.normal.Top
		_ = app.store.SaveAll()
	}
}

func floatWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		paintFloating(hwnd)
		return 0
	case WM_TIMER:
		if wParam == timerFloatingDock {
			pollFloatingDock()
		}
		return 0
	case WM_MOUSEMOVE:
		pollFloatingDock()
		return 0
	case WM_ENTERSIZEMOVE:
		floatingDockState.dragging = true
		floatingDockState.outsideSince = time.Time{}
		return 0
	case WM_EXITSIZEMOVE:
		floatingDockState.dragging = false
		floatingDockState.place(floatingWindowRect(), floatingWorkArea(hwnd), app.store.Settings.FloatingAutoHide)
		positionFloating(floatingDockState.normal)
		saveFloatingPosition()
		return 0
	case WM_DISPLAYCHANGE, WM_SETTINGCHANGE:
		applyFloatingSettings()
		return 0
	case WM_LBUTTONUP:
		if floatingDockState.collapsed {
			restoreFloating()
			return 0
		}
		var r RECT
		pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
		x, y := signed16(loword(lParam)), signed16(hiword(lParam))
		if x > r.Right-38 && y < 36 {
			hideFloating()
		}
		return 0
	case WM_NCHITTEST:
		if floatingDockState.collapsed {
			return HTCLIENT
		}
		var wr RECT
		pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))
		sx, sy := signed16(loword(lParam)), signed16(hiword(lParam))
		if sx >= wr.Right-42 && sy <= wr.Top+38 {
			return HTCLIENT
		}
		return HTCAPTION
	case WM_NCRBUTTONDOWN:
		return 0 // Avoid the default caption context menu.
	case WM_RBUTTONUP, WM_NCRBUTTONUP:
		panicHide()
		return 0
	case WM_CLOSE:
		hideFloating()
		return 0
	case WM_DESTROY:
		pKillTimer.Call(uintptr(hwnd), timerFloatingDock)
		saveFloatingPosition()
		floatingDockState = floatingDock{}
		app.floating = 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func floatPalette() (uint32, uint32, uint32) {
	p := currentPalette()
	switch app.store.Settings.FloatingPalette {
	case "Peach":
		return p.Peach, p.Text, p.Money
	case "Mint":
		return p.Mint, p.Text, p.Success
	case "Pink":
		return p.Pink, p.Text, p.Danger
	case "Sky":
		return p.Sky, p.Text, p.Accent
	}
	return p.Lavender, p.Text, p.Accent
}
func applyFloatingSettings() {
	if app.floating == 0 {
		return
	}
	w, h := floatingSize()
	r := floatingDockState.normal
	if r.width() <= 0 || r.height() <= 0 {
		r = floatingWindowRect()
	}
	// Keep right/bottom anchoring when dimensions change while docked.
	x, y := r.Left, r.Top
	if floatingDockState.edge == dockRight {
		x = r.Right - w
	}
	if floatingDockState.edge == dockBottom {
		y = r.Bottom - h
	}
	wa := floatingWorkArea(app.floating)
	if app.store.Settings.FloatingMode == "Taskbar" {
		x, y = wa.Right-w-18, wa.Bottom-h-12
	}
	floatingDockState.place(desktopRect{x, y, x + w, y + h}, wa, app.store.Settings.FloatingAutoHide)
	r = floatingDockState.normal
	z := ^uintptr(1) // HWND_NOTOPMOST
	if app.store.Settings.FloatingTopmost {
		z = ^uintptr(0) // HWND_TOPMOST
	}
	pSetWindowPos.Call(uintptr(app.floating), z, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), SWP_NOACTIVATE|SWP_NOOWNERZORDER)
	alpha := byte(app.store.Settings.FloatingOpacity * 255)
	if alpha < 100 {
		alpha = 100
	}
	pSetLayeredWindowAttributes.Call(uintptr(app.floating), 0, uintptr(alpha), LWA_ALPHA)
	if app.store.Settings.FloatingAutoHide {
		pSetTimer.Call(uintptr(app.floating), timerFloatingDock, 100, 0)
	} else {
		pKillTimer.Call(uintptr(app.floating), timerFloatingDock)
	}
	invalidate(app.floating)
}

func paintFloating(hwnd HWND) {
	var ps PAINTSTRUCT
	hdc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	var r RECT
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	bg, fg, ac := floatPalette()
	if floatingDockState.collapsed {
		fill(HDC(hdc), r, ac)
		return
	}
	fill(HDC(hdc), r, bg)
	now := time.Now()
	earned, progress := app.store.TodayEarned(now)
	info := app.store.DayInfo(now)
	set := app.store.Settings
	top, detail := "", ""
	if info.IsRestDay {
		top = "今天不用打工"
		detail = "下次上班 " + formatDateShort(app.store.NextWorkDay(now))
	} else {
		if set.FloatingShowEarned {
			top = fmt.Sprintf("≈¥%.0f", earned)
		}
		if set.FloatingShowProgress {
			if top != "" {
				top += " · "
			}
			top += fmt.Sprintf("%d%%", int(progress*100))
		}
		if top == "" {
			top = "打工搭子"
		}
		if set.FloatingShowCountdown {
			a, b := workCountdown(now)
			detail = a + " " + b
		}
	}
	size, detailY, phraseY := 20, int32(39), int32(67)
	if set.FloatingCompact {
		size, detailY, phraseY = 15, 26, 49
	}
	drawText(HDC(hdc), top, RECT{16, 4, r.Right - 42, detailY}, size, FW_BOLD, fg, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(HDC(hdc), detail, RECT{16, detailY, r.Right - 16, phraseY}, 11, FW_NORMAL, currentPalette().Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	if set.FloatingShowPhrase {
		phrase := "每一分钟都算数。"
		if info.IsRestDay {
			phrase = "请珍惜这短暂的自由。"
		} else if progress >= 1 {
			phrase = "今天辛苦了。"
		}
		drawText(HDC(hdc), phrase, RECT{16, phraseY, r.Right - 16, phraseY + 22}, 11, FW_NORMAL, currentPalette().Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	}
	if set.FloatingShowProgress && !info.IsRestDay {
		bar := RECT{16, r.Bottom - 10, r.Right - 16, r.Bottom - 5}
		fill(HDC(hdc), bar, currentPalette().Surface)
		bar.Right = bar.Left + int32(float64(bar.Right-bar.Left)*progress)
		if progress > 0 {
			fill(HDC(hdc), bar, ac)
		}
	}
	drawText(HDC(hdc), "×", RECT{r.Right - 36, 4, r.Right - 6, 32}, 17, FW_BOLD, currentPalette().Sub, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}
