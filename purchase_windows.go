//go:build windows
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
    "unsafe"
)

var purchaseWnd HWND
var purchaseFont HFONT

func requirePro(feature string) bool {
    if app.license == nil || app.license.HasProAccess(time.Now()) { return true }
    openPurchaseWindow()
    if feature != "" { showBalloon("需要 WorkMate Pro", feature+" 属于 Pro 功能。", false) }
    return false
}

func openPurchaseWindow() {
    if purchaseWnd != 0 { show(purchaseWnd, SW_RESTORE); pSetForegroundWindow.Call(uintptr(purchaseWnd)); return }
    style:=uintptr(WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU)
    r,_,_:=pCreateWindowEx.Call(0,uintptr(unsafe.Pointer(u16(purchaseClass))),uintptr(unsafe.Pointer(u16("WorkMate Pro · 一次买断"))),style,560,190,620,590,uintptr(app.main),0,uintptr(app.hInst),0)
    purchaseWnd=HWND(r); show(purchaseWnd,SW_SHOW)
}

func purchaseWndProc(hwnd HWND,msg uint32,wParam,lParam uintptr) uintptr {
    switch msg {
    case WM_CREATE:
        f,_,_:=pCreateFont.Call(uintptr(^uint32(14-1)),0,0,0,FW_NORMAL,0,0,0,DEFAULT_CHARSET,0,0,CLEARTYPE_QUALITY,0,uintptr(unsafe.Pointer(u16("Microsoft YaHei UI"))))
        purchaseFont=HFONT(f)
        createCtl(hwnd,"STATIC","WorkMate Pro · 一次买断",0,26,22,540,38,0)
        createCtl(hwnd,"STATIC","试用期内可完整体验 Pro；到期后保留基础功能，高级功能需要激活。",0,26,62,550,46,0)
        status:="当前状态："+app.license.StatusText(time.Now())
        createCtl(hwnd,"STATIC",status,0,26,112,550,28,0)
        createCtl(hwnd,"STATIC","建议首发价格：¥19.9（你可在发布前修改文案）",0,26,146,550,28,0)
        createCtl(hwnd,"BUTTON","① 查看开发者收款码",BS_PUSHBUTTON|WS_TABSTOP,26,190,210,38,501)
        createCtl(hwnd,"BUTTON","② 复制完整设备码",BS_PUSHBUTTON|WS_TABSTOP,248,190,190,38,502)
        createCtl(hwnd,"STATIC","付款后，把“完整设备码”发给开发者。开发者确认到账后会返回一个只绑定当前电脑的永久激活码。",0,26,240,550,58,0)
        createCtl(hwnd,"STATIC","完整设备码：",0,26,309,110,25,0)
        dev:=createCtl(hwnd,"EDIT",app.license.deviceID,WS_BORDER|ES_AUTOHSCROLL,135,305,430,30,503)
        pSendMessage.Call(uintptr(dev),EM_SETREADONLY,1,0)
        createCtl(hwnd,"STATIC","激活码：",0,26,354,90,25,0)
        createCtl(hwnd,"EDIT","",WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL,110,350,455,31,504)
        createCtl(hwnd,"BUTTON","验证并永久解锁",BS_PUSHBUTTON|WS_TABSTOP,335,404,160,40,505)
        createCtl(hwnd,"BUTTON","暂时继续使用 Free",BS_PUSHBUTTON|WS_TABSTOP,160,404,160,40,506)
        createCtl(hwnd,"STATIC","安全说明：WorkMate 不会把工资、加班或报告上传。个人收款码没有可靠的自动付款回调，因此此版本采用“付款后签发激活码”。要自动到账即解锁，需要以后接入商户支付 API + 你的授权服务器。",0,26,466,550,82,0)
        return 0
    case WM_COMMAND:
        switch loword(wParam) {
        case 501:
            openPaymentQR(hwnd)
        case 502:
            dev:=getDlgItem(hwnd,503); pSetFocus.Call(uintptr(dev)); pSendMessage.Call(uintptr(dev), EM_SETSEL, 0, ^uintptr(0)); msgBox(hwnd,"设备码已选中","按 Ctrl+C 复制完整设备码，然后付款后发给开发者。",MB_OK|MB_ICONINFORMATION)
        case 505:
            code:=strings.TrimSpace(getText(getDlgItem(hwnd,504)))
            if code=="" { msgBox(hwnd,"请输入激活码","先付款并向开发者获取激活码。",MB_OK|MB_ICONWARNING); return 0 }
            if err:=app.license.Activate(code); err!=nil { msgBox(hwnd,"激活失败",err.Error(),MB_OK|MB_ICONERROR) } else { msgBox(hwnd,"激活成功","WorkMate Pro 已永久解锁。",MB_OK|MB_ICONINFORMATION); pDestroyWindow.Call(uintptr(hwnd)); invalidate(app.main) }
        case 506:
            pDestroyWindow.Call(uintptr(hwnd))
        }
        return 0
    case WM_CLOSE:
        pDestroyWindow.Call(uintptr(hwnd)); return 0
    case WM_DESTROY:
        if purchaseFont!=0 { pDeleteObject.Call(uintptr(purchaseFont)); purchaseFont=0 }
        purchaseWnd=0; return 0
    }
    r,_,_:=pDefWindowProc.Call(uintptr(hwnd),uintptr(msg),wParam,lParam); return r
}

func openPaymentQR(owner HWND) {
    p:=filepath.Join(exeDir(),"payment_qr.png")
    if _,err:=os.Stat(p); err!=nil {
        msgBox(owner,"尚未配置收款码","发布前请把你的个人收款码图片命名为 payment_qr.png，放到 WorkMate.exe 同目录。\n\n正式自动解锁不能只依赖个人收款码，需要商户支付 API。",MB_OK|MB_ICONINFORMATION)
        return
    }
    r,_,_:=pShellExecute.Call(uintptr(owner),0,uintptr(unsafe.Pointer(u16(p))),0,0,SW_SHOWNORMAL)
    if int(r)<=32 { msgBox(owner,"无法打开收款码",fmt.Sprintf("ShellExecute 返回 %d",r),MB_OK|MB_ICONERROR) }
}
