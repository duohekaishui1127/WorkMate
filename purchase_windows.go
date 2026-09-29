//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

var purchaseWnd HWND
var purchaseFont HFONT

func requirePro(feature string) bool {
	if app.license == nil || app.license.HasProAccess(time.Now()) {
		return true
	}
	openPurchaseWindow()
	if feature != "" {
		showBalloon("需要 WorkMate Pro", feature+" 属于 Pro 功能。", false)
	}
	return false
}

func openPurchaseWindow() {
	if purchaseWnd != 0 {
		show(purchaseWnd, SW_RESTORE)
		pSetForegroundWindow.Call(uintptr(purchaseWnd))
		return
	}
	purchaseWnd = createOwnedWindow(purchaseClass, "WorkMate Pro · 一次买断", 620, 614, app.main)
	show(purchaseWnd, SW_SHOW)
}

func purchaseWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		f, _, _ := pCreateFont.Call(uintptr(^uint32(14-1)), 0, 0, 0, FW_NORMAL, 0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY, 0, uintptr(unsafe.Pointer(u16("Microsoft YaHei UI"))))
		purchaseFont = HFONT(f)
		ctl := func(class, text string, style uintptr, x, y, w, h, id int) HWND {
			child := createCtl(hwnd, class, text, style, x, y, w, h, id)
			pSendMessage.Call(uintptr(child), WM_SETFONT, uintptr(purchaseFont), 1)
			return child
		}
		ctl("STATIC", "WorkMate Pro · 一次买断", 0, 26, 20, 566, 38, 0)
		ctl("STATIC", "试用期间可完整体验 Pro；到期后保留基础功能，高级功能需要激活。", 0, 26, 62, 566, 44, 0)
		ctl("STATIC", "当前状态："+app.license.StatusText(time.Now()), 0, 26, 112, 566, 26, 507)
		cfg, err := loadCommerceConfig(exeDir())
		if err == nil {
			err = cfg.PurchaseReady(exeDir())
		}
		purchaseText := "购买入口尚未开放。已有激活码可直接激活。"
		contactText := ""
		if err == nil {
			purchaseText = "买断价格：" + cfg.Price
			contactText = "联系开发者：" + cfg.Contact
		}
		ctl("STATIC", purchaseText, 0, 26, 146, 566, 28, 508)
		ctl("STATIC", contactText, 0, 26, 178, 566, 40, 509)
		pay := ctl("BUTTON", "① 查看收款码", BS_PUSHBUTTON|WS_TABSTOP, 26, 226, 210, 36, 501)
		if err != nil {
			pEnableWindow.Call(uintptr(pay), 0)
		}
		ctl("BUTTON", "② 复制完整设备码", BS_PUSHBUTTON|WS_TABSTOP, 248, 226, 190, 36, 502)
		ctl("STATIC", "付款后，将付款凭证与完整设备码发给开发者。确认到账后，你会收到绑定当前电脑的永久激活码。", 0, 26, 278, 566, 48, 0)
		ctl("STATIC", "完整设备码：", 0, 26, 343, 110, 26, 0)
		dev := ctl("EDIT", app.license.deviceID, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 135, 339, 457, 30, 503)
		pSendMessage.Call(uintptr(dev), EM_SETREADONLY, 1, 0)
		ctl("STATIC", "激活码：", 0, 26, 388, 90, 26, 0)
		ctl("EDIT", "", WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 110, 384, 482, 31, 504)
		ctl("BUTTON", "验证并永久解锁", BS_PUSHBUTTON|WS_TABSTOP, 350, 439, 180, 40, 505)
		ctl("BUTTON", "继续使用", BS_PUSHBUTTON|WS_TABSTOP, 150, 439, 170, 40, 506)
		ctl("STATIC", "激活需要开发者人工确认付款并签发激活码。工资、加班和报告保存在本机。", 0, 26, 504, 566, 56, 0)
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case 501:
			openPaymentQR(hwnd)
		case 502:
			if err := copyTextToClipboard(hwnd, app.license.deviceID); err != nil {
				msgBox(hwnd, "复制失败", err.Error(), MB_OK|MB_ICONWARNING)
			} else {
				setText(getDlgItem(hwnd, 502), "设备码已复制")
			}
		case 505:
			code := strings.TrimSpace(getText(getDlgItem(hwnd, 504)))
			if code == "" {
				msgBox(hwnd, "请输入激活码", "请填写开发者提供的激活码。", MB_OK|MB_ICONWARNING)
				return 0
			}
			if err := app.license.Activate(code); err != nil {
				msgBox(hwnd, "激活失败", err.Error(), MB_OK|MB_ICONERROR)
			} else {
				msgBox(hwnd, "激活成功", "WorkMate Pro 已永久解锁。", MB_OK|MB_ICONINFORMATION)
				pDestroyWindow.Call(uintptr(hwnd))
				invalidate(app.main)
			}
		case 506:
			pDestroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		if purchaseFont != 0 {
			pDeleteObject.Call(uintptr(purchaseFont))
			purchaseFont = 0
		}
		purchaseWnd = 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func openPaymentQR(owner HWND) {
	cfg, err := loadCommerceConfig(exeDir())
	if err == nil {
		err = cfg.PurchaseReady(exeDir())
	}
	if err != nil {
		msgBox(owner, "购买入口尚未开放", err.Error(), MB_OK|MB_ICONINFORMATION)
		return
	}
	p := filepath.Join(exeDir(), "payment_qr.png")
	r, _, _ := pShellExecute.Call(uintptr(owner), 0, uintptr(unsafe.Pointer(u16(p))), 0, 0, SW_SHOWNORMAL)
	if int(r) <= 32 {
		msgBox(owner, "无法打开收款码", fmt.Sprintf("无法打开收款码图片（错误 %d）。", r), MB_OK|MB_ICONERROR)
	}
}
