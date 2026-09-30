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
var purchaseAdvanced bool
var purchaseFocusFeature featureID

func requireFeature(id featureID) bool {
	if canUseFeature(app.license, id, time.Now()) {
		return true
	}
	openPurchaseWindowFor(id)
	return false
}
func openPurchaseWindowFor(id featureID) {
	purchaseFocusFeature = id
	openPurchaseWindow()
	if purchaseWnd != 0 {
		updatePurchaseBenefit(purchaseWnd)
	}
}
func openPurchaseWindow() {
	if purchaseWnd != 0 {
		show(purchaseWnd, SW_RESTORE)
		pSetForegroundWindow.Call(uintptr(purchaseWnd))
		return
	}
	purchaseAdvanced = false
	purchaseWnd = createOwnedWindow(purchaseClass, "WorkMate Pro · 先看清楚再决定", 620, 674, app.main)
	if onlinePurchase == nil {
		setPurchaseAdvanced(purchaseWnd, true)
	}
	show(purchaseWnd, SW_SHOW)
}
func updatePurchaseBenefit(hwnd HWND) {
	if f, ok := findFeature(purchaseFocusFeature); ok {
		setText(getDlgItem(hwnd, 519), f.Name+"："+f.Benefit)
	} else {
		setText(getDlgItem(hwnd, 519), "Pro 帮你月底整理工时与休假；基础数据始终属于你。")
	}
}
func setPurchaseAdvanced(hwnd HWND, expanded bool) {
	if hwnd == 0 {
		return
	}
	purchaseAdvanced = expanded
	for _, id := range []int{502, 503, 504, 505, 516, 517, 518} {
		ctl := getDlgItem(hwnd, id)
		if ctl != 0 {
			if expanded {
				pShowWindow.Call(uintptr(ctl), SW_SHOW)
			} else {
				pShowWindow.Call(uintptr(ctl), SW_HIDE)
			}
		}
	}
	label := "已有激活码？展开备用入口"
	clientHeight, footerY := 674, 608
	if expanded {
		label = "收起备用激活入口"
		clientHeight, footerY = 858, 800
	}
	setText(getDlgItem(hwnd, 514), label)
	footer := getDlgItem(hwnd, 513)
	if footer != 0 {
		move(footer, 26, footerY, 566, 44)
	}
	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU)
	r := RECT{0, 0, 620, int32(clientHeight)}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	var current, work RECT
	pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&current)))
	pSystemParametersInfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&work)), 0)
	x, y := int(current.Left), int(current.Top)
	if y+h > int(work.Bottom) {
		y = int(work.Bottom) - h
	}
	if y < int(work.Top) {
		y = int(work.Top)
	}
	move(hwnd, x, y, w, h)
}

func purchaseWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		purchaseFont = newUIFont(14)
		ctl := func(class, text string, style uintptr, x, y, w, h, id int) HWND {
			child := createCtl(hwnd, class, text, style, x, y, w, h, id)
			pSendMessage.Call(uintptr(child), WM_SETFONT, uintptr(purchaseFont), 1)
			return child
		}
		ctl("STATIC", "WorkMate Pro · 按月整理你的打工记录", 0, 26, 18, 566, 35, 0)
		ctl("STATIC", "普通版也能长期使用，先体验；需要省去月底抄表和管理休假时再买断。", 0, 26, 56, 566, 42, 0)
		ctl("STATIC", "当前状态："+app.license.StatusText(time.Now()), 0, 26, 103, 566, 26, 507)
		ctl("STATIC", "普通版 · 免费", 0, 26, 139, 274, 27, 520)
		ctl("STATIC", "Pro · 一次买断", 0, 316, 139, 276, 27, 521)
		ctl("STATIC", "收入估算 / 下班倒计时 / 挂件\r\n加班计时、补记与今日时间轴\r\n基础下班提醒与今日分享卡\r\n本地备份和恢复，数据属于你", 0, 26, 171, 274, 106, 522)
		ctl("STATIC", "年假、补休与请假管理\r\n按月导出工时账本（CSV）\r\n调休、假期与发薪日提醒\r\n月报、年报与节假日配置", 0, 316, 171, 276, 106, 523)
		ctl("STATIC", "", 0, 26, 289, 566, 43, 519)
		updatePurchaseBenefit(hwnd)
		cfg, err := loadCommerceConfig(exeDir())
		if err == nil {
			err = cfg.PurchaseReady(exeDir())
		}
		purchaseText := "购买入口尚未开放。已有激活码仍可使用。"
		contactText := ""
		if err == nil {
			purchaseText = "买断价格：" + cfg.Price
			contactText = "联系开发者：" + cfg.Contact
		}
		payText := "查看收款码"
		instruction := "付款后向开发者发送付款凭证与设备码，核实后使用激活码。"
		footer := "工资与记录只存在本机；授权绑定当前电脑，可离线使用。"
		orderStatus := "付款需人工核实到账，不会在上传截图后立即开通。"
		if onlinePurchase != nil {
			payText = "扫码付款 / 查看订单"
			instruction = "付款后只需上传一张成功截图。设备自动绑定，核实到账后软件自动开通，无需抄设备码。"
			purchaseText = "购买价格和收款码以订单页面为准。"
			contactText = ""
			orderStatus = onlinePurchase.status
		}
		ctl("STATIC", purchaseText, 0, 26, 341, 566, 28, 508)
		ctl("STATIC", contactText, 0, 26, 373, 566, 28, 509)
		pay := ctl("BUTTON", payText, BS_PUSHBUTTON|WS_TABSTOP, 26, 413, 265, 38, 501)
		if (onlinePurchase == nil && err != nil) || (onlinePurchase != nil && onlinePurchase.setupError != nil) {
			pEnableWindow.Call(uintptr(pay), 0)
		}
		if onlinePurchase != nil {
			check := ctl("BUTTON", "检查订单 / 领取授权", BS_PUSHBUTTON|WS_TABSTOP, 305, 413, 287, 38, 510)
			if onlinePurchase.setupError != nil {
				pEnableWindow.Call(uintptr(check), 0)
			}
		}
		ctl("STATIC", instruction, 0, 26, 461, 566, 48, 512)
		ctl("STATIC", orderStatus, 0, 26, 511, 566, 42, 511)
		ctl("BUTTON", "已有激活码？展开备用入口", BS_PUSHBUTTON|WS_TABSTOP, 26, 562, 288, 36, 514)
		ctl("BUTTON", "继续免费使用", BS_PUSHBUTTON|WS_TABSTOP, 380, 562, 212, 36, 506)
		ctl("STATIC", footer, 0, 26, 608, 566, 44, 513)
		ctl("STATIC", "离线激活 / 已有激活码", 0, 26, 619, 566, 28, 518)
		ctl("STATIC", "当前电脑的完整设备码：", 0, 26, 653, 180, 26, 516)
		device := ctl("EDIT", app.license.deviceID, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 218, 650, 374, 28, 503)
		pSendMessage.Call(uintptr(device), EM_SETREADONLY, 1, 0)
		ctl("BUTTON", "复制设备码（备用）", BS_PUSHBUTTON|WS_TABSTOP, 26, 688, 202, 34, 502)
		ctl("STATIC", "激活码：", 0, 26, 740, 80, 28, 517)
		ctl("EDIT", "", WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 108, 737, 484, 30, 504)
		ctl("BUTTON", "验证并永久解锁", BS_PUSHBUTTON|WS_TABSTOP, 392, 777, 200, 38, 505)
		for _, id := range []int{502, 503, 504, 505, 516, 517, 518} {
			pShowWindow.Call(uintptr(getDlgItem(hwnd, id)), SW_HIDE)
		}
		refreshOnlinePurchaseWindow()
		syncOnlinePurchase(time.Now(), false)
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case 501:
			if onlinePurchase != nil {
				beginOnlinePurchase()
			} else {
				openPaymentQR(hwnd)
			}
		case 510:
			syncOnlinePurchase(time.Now(), true)
			refreshOnlinePurchaseWindow()
		case 514:
			setPurchaseAdvanced(hwnd, !purchaseAdvanced)
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
		purchaseAdvanced = false
		purchaseFocusFeature = ""
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
