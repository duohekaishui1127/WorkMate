//go:build windows

package main

import (
	"fmt"
	"time"
	"unsafe"
)

func floatingSize() (int, int) { return floatingDimensions(app.store.Settings) }
func toggleFloating(forceShow bool) {
	if app.floating != 0 {
		if forceShow {
			show(app.floating, SW_SHOW)
			return
		}
		pDestroyWindow.Call(uintptr(app.floating))
		return
	}
	w, h := floatingSize()
	x, y := app.store.Settings.FloatingLeft, app.store.Settings.FloatingTop
	var wa RECT
	pSystemParametersInfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	if app.store.Settings.FloatingMode == "Taskbar" || x < 0 || y < 0 {
		x = int(wa.Right) - w - 18
		y = int(wa.Bottom) - h - 12
	}
	style := uintptr(WS_POPUP)
	ex := uintptr(WS_EX_TOOLWINDOW | WS_EX_TOPMOST | WS_EX_LAYERED)
	r, _, _ := pCreateWindowEx.Call(ex, uintptr(unsafe.Pointer(u16(floatClass))), uintptr(unsafe.Pointer(u16("WorkMate 挂件"))), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(app.main), 0, uintptr(app.hInst), 0)
	app.floating = HWND(r)
	if app.floating == 0 {
		return
	}
	applyFloatingSettings()
	show(app.floating, SW_SHOW)
}

func floatWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		paintFloating(hwnd)
		return 0
	case WM_LBUTTONUP:
		var r RECT
		pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
		x := signed16(loword(lParam))
		y := signed16(hiword(lParam))
		if x > r.Right-38 && y < 36 {
			show(hwnd, SW_HIDE)
		}
		return 0
	case WM_NCHITTEST:
		var wr RECT
		pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))
		sx := signed16(loword(lParam))
		sy := signed16(hiword(lParam))
		if sx >= wr.Right-42 && sy <= wr.Top+38 {
			return HTCLIENT
		}
		return HTCAPTION
	case WM_RBUTTONUP:
		show(hwnd, SW_HIDE)
		return 0
	case WM_CLOSE:
		show(hwnd, SW_HIDE)
		return 0
	case WM_DESTROY:
		var wr RECT
		pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))
		if app.store.Settings.FloatingMode != "Taskbar" {
			app.store.Settings.FloatingLeft = int(wr.Left)
			app.store.Settings.FloatingTop = int(wr.Top)
		}
		_ = app.store.SaveAll()
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
	var wr RECT
	pGetWindowRect.Call(uintptr(app.floating), uintptr(unsafe.Pointer(&wr)))
	x, y := int(wr.Left), int(wr.Top)
	if app.store.Settings.FloatingMode == "Taskbar" {
		var wa RECT
		pSystemParametersInfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
		x, y = int(wa.Right)-w-18, int(wa.Bottom)-h-12
	}
	move(app.floating, x, y, w, h)
	alpha := byte(app.store.Settings.FloatingOpacity * 255)
	if alpha < 100 {
		alpha = 100
	}
	pSetLayeredWindowAttributes.Call(uintptr(app.floating), 0, uintptr(alpha), LWA_ALPHA)
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
