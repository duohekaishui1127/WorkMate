//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

type HWND uintptr
type HINSTANCE uintptr
type HICON uintptr
type HCURSOR uintptr
type HBRUSH uintptr
type HDC uintptr
type HFONT uintptr
type HMENU uintptr
type HBITMAP uintptr

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type MONITORINFO struct {
	CbSize        uint32
	Monitor, Work RECT
	Flags         uint32
}
type MSG struct {
	HWnd           HWND
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             POINT
	LPrivate       uint32
}
type PAINTSTRUCT struct {
	Hdc                  HDC
	FErase               int32
	RcPaint              RECT
	FRestore, FIncUpdate int32
	RgbReserved          [32]byte
}
type WNDCLASSEX struct {
	CbSize                      uint32
	Style                       uint32
	LpfnWndProc                 uintptr
	CbClsExtra, CbWndExtra      int32
	HInstance                   HINSTANCE
	HIcon                       HICON
	HCursor                     HCURSOR
	HbrBackground               HBRUSH
	LpszMenuName, LpszClassName *uint16
	HIconSm                     HICON
}
type NOTIFYICONDATA struct {
	CbSize            uint32
	HWnd              HWND
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             HICON
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          [16]byte
	HBalloonIcon      HICON
}
type OPENFILENAME struct {
	LStructSize       uint32
	HwndOwner         HWND
	HInstance         HINSTANCE
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        unsafe.Pointer
	DwReserved        uint32
	FlagsEx           uint32
}
type BITMAPINFOHEADER struct {
	BiSize                           uint32
	BiWidth, BiHeight                int32
	BiPlanes, BiBitCount             uint16
	BiCompression                    uint32
	BiSizeImage                      uint32
	BiXPelsPerMeter, BiYPelsPerMeter int32
	BiClrUsed, BiClrImportant        uint32
}
type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	Colors [1]uint32
}

const MONITOR_DEFAULTTONEAREST = 2

const (
	CS_HREDRAW           = 0x0002
	CS_VREDRAW           = 0x0001
	WS_OVERLAPPED        = 0x00000000
	WS_CAPTION           = 0x00C00000
	WS_SYSMENU           = 0x00080000
	WS_MINIMIZEBOX       = 0x00020000
	WS_VISIBLE           = 0x10000000
	WS_CHILD             = 0x40000000
	WS_TABSTOP           = 0x00010000
	WS_VSCROLL           = 0x00200000
	CBS_DROPDOWNLIST     = 0x0003
	CB_ADDSTRING         = 0x0143
	CB_GETCURSEL         = 0x0147
	CB_SETCURSEL         = 0x014e
	WS_BORDER            = 0x00800000
	WS_POPUP             = 0x80000000
	WS_EX_TOOLWINDOW     = 0x00000080
	WS_EX_TOPMOST        = 0x00000008
	WS_EX_LAYERED        = 0x00080000
	WS_EX_NOACTIVATE     = 0x08000000
	BS_PUSHBUTTON        = 0x00000000
	BS_AUTOCHECKBOX      = 0x00000003
	BM_GETCHECK          = 0x00F0
	BM_SETCHECK          = 0x00F1
	BST_CHECKED          = 1
	ES_AUTOHSCROLL       = 0x0080
	ES_NUMBER            = 0x2000
	EM_SETREADONLY       = 0x00CF
	EM_SETSEL            = 0x00B1
	SW_HIDE              = 0
	SW_SHOWNORMAL        = 1
	SW_SHOWNOACTIVATE    = 4
	SW_SHOW              = 5
	SW_RESTORE           = 9
	WM_NULL              = 0x0000
	WM_CREATE            = 0x0001
	WM_DESTROY           = 0x0002
	WM_PAINT             = 0x000F
	WM_CLOSE             = 0x0010
	WM_CONTEXTMENU       = 0x007B
	WM_USER              = 0x0400
	NIN_SELECT           = WM_USER + 0
	NIN_KEYSELECT        = WM_USER + 1
	WM_COMMAND           = 0x0111
	WM_TIMER             = 0x0113
	WM_HOTKEY            = 0x0312
	WM_LBUTTONUP         = 0x0202
	WM_LBUTTONDBLCLK     = 0x0203
	WM_RBUTTONUP         = 0x0205
	WM_MOUSEMOVE         = 0x0200
	WM_NCRBUTTONDOWN     = 0x00A4
	WM_NCRBUTTONUP       = 0x00A5
	WM_NCHITTEST         = 0x0084
	WM_ENTERSIZEMOVE     = 0x0231
	WM_EXITSIZEMOVE      = 0x0232
	WM_DISPLAYCHANGE     = 0x007E
	WM_SETTINGCHANGE     = 0x001A
	WM_SETCURSOR         = 0x0020
	WM_APP               = 0x8000
	WM_APP_TRAY          = WM_APP + 1
	WM_APP_SHOW          = WM_APP + 2
	WM_APP_EXIT          = WM_APP + 3
	HTCAPTION            = 2
	HTCLIENT             = 1
	MB_OK                = 0x00000000
	MB_OKCANCEL          = 0x00000001
	MB_YESNO             = 0x00000004
	MB_ICONINFORMATION   = 0x40
	MB_ICONWARNING       = 0x30
	MB_ICONERROR         = 0x10
	IDOK                 = 1
	IDCANCEL             = 2
	IDYES                = 6
	COLOR_WINDOW         = 5
	IDC_ARROW            = 32512
	IDI_APPLICATION      = 32512
	IMAGE_ICON           = 1
	LR_LOADFROMFILE      = 0x0010
	LR_DEFAULTSIZE       = 0x0040
	DT_LEFT              = 0x00000000
	DT_CENTER            = 0x00000001
	DT_RIGHT             = 0x00000002
	DT_VCENTER           = 0x00000004
	DT_SINGLELINE        = 0x00000020
	DT_WORDBREAK         = 0x00000010
	DT_END_ELLIPSIS      = 0x00008000
	TRANSPARENT          = 1
	OPAQUE               = 2
	FW_NORMAL            = 400
	FW_SEMIBOLD          = 600
	FW_BOLD              = 700
	DEFAULT_CHARSET      = 1
	CLEARTYPE_QUALITY    = 5
	PS_SOLID             = 0
	NIM_ADD              = 0x0
	NIM_MODIFY           = 0x1
	NIM_DELETE           = 0x2
	NIM_SETVERSION       = 0x4
	NIF_MESSAGE          = 0x1
	NIF_ICON             = 0x2
	NIF_TIP              = 0x4
	NIF_INFO             = 0x10
	NIF_GUID             = 0x20
	NIF_SHOWTIP          = 0x80
	NIIF_INFO            = 0x1
	NIIF_WARNING         = 0x2
	NOTIFYICON_VERSION_4 = 4
	MF_STRING            = 0x0000
	MF_SEPARATOR         = 0x0800
	TPM_RIGHTBUTTON      = 0x0002
	TPM_RETURNCMD        = 0x0100
	MOD_ALT              = 0x0001
	MOD_CONTROL          = 0x0002
	LWA_ALPHA            = 0x2
	GWLP_USERDATA        = -21
	SWP_NOSIZE           = 0x0001
	SWP_NOZORDER         = 0x0004
	SWP_NOACTIVATE       = 0x0010
	SWP_NOOWNERZORDER    = 0x0200
	SPI_GETWORKAREA      = 0x0030
	OFN_EXPLORER         = 0x00080000
	OFN_PATHMUSTEXIST    = 0x00000800
	OFN_FILEMUSTEXIST    = 0x00001000
	OFN_OVERWRITEPROMPT  = 0x00000002
	BI_RGB               = 0
	DIB_RGB_COLORS       = 0
	SRCCOPY              = 0x00CC0020
	WM_SETFONT           = 0x0030
	ERROR_ALREADY_EXISTS = 183
	CF_UNICODETEXT       = 13
	GMEM_MOVEABLE        = 0x0002
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pRegisterClassEx            = user32.NewProc("RegisterClassExW")
	pCreateWindowEx             = user32.NewProc("CreateWindowExW")
	pDefWindowProc              = user32.NewProc("DefWindowProcW")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pUpdateWindow               = user32.NewProc("UpdateWindow")
	pGetMessage                 = user32.NewProc("GetMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessage            = user32.NewProc("DispatchMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pBeginPaint                 = user32.NewProc("BeginPaint")
	pEndPaint                   = user32.NewProc("EndPaint")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pGetClientRect              = user32.NewProc("GetClientRect")
	pMessageBox                 = user32.NewProc("MessageBoxW")
	pLoadCursor                 = user32.NewProc("LoadCursorW")
	pLoadIcon                   = user32.NewProc("LoadIconW")
	pLoadImage                  = user32.NewProc("LoadImageW")
	pGetModuleHandle            = kernel32.NewProc("GetModuleHandleW")
	pSetTimer                   = user32.NewProc("SetTimer")
	pKillTimer                  = user32.NewProc("KillTimer")
	pDestroyWindow              = user32.NewProc("DestroyWindow")
	pSetWindowText              = user32.NewProc("SetWindowTextW")
	pGetWindowText              = user32.NewProc("GetWindowTextW")
	pGetWindowTextLength        = user32.NewProc("GetWindowTextLengthW")
	pCreateSolidBrush           = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject               = gdi32.NewProc("DeleteObject")
	pFillRect                   = user32.NewProc("FillRect")
	pRoundRect                  = gdi32.NewProc("RoundRect")
	pRectangle                  = gdi32.NewProc("Rectangle")
	pCreatePen                  = gdi32.NewProc("CreatePen")
	pSelectObject               = gdi32.NewProc("SelectObject")
	pSetBkMode                  = gdi32.NewProc("SetBkMode")
	pSetTextColor               = gdi32.NewProc("SetTextColor")
	pCreateFont                 = gdi32.NewProc("CreateFontW")
	pDrawText                   = user32.NewProc("DrawTextW")
	pRegisterHotKey             = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey           = user32.NewProc("UnregisterHotKey")
	pFindWindow                 = user32.NewProc("FindWindowW")
	pSendMessage                = user32.NewProc("SendMessageW")
	pPostMessage                = user32.NewProc("PostMessageW")
	pSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	pIsIconic                   = user32.NewProc("IsIconic")
	pIsWindowVisible            = user32.NewProc("IsWindowVisible")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pGetWindowRect              = user32.NewProc("GetWindowRect")
	pMonitorFromWindow          = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo             = user32.NewProc("GetMonitorInfoW")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pAppendMenu                 = user32.NewProc("AppendMenuW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pShellNotifyIcon            = shell32.NewProc("Shell_NotifyIconW")
	pShellExecute               = shell32.NewProc("ShellExecuteW")
	pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	pSystemParametersInfo       = user32.NewProc("SystemParametersInfoW")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pMoveWindow                 = user32.NewProc("MoveWindow")
	pAdjustWindowRectEx         = user32.NewProc("AdjustWindowRectEx")
	pGetOpenFileName            = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileName            = comdlg32.NewProc("GetSaveFileNameW")
	pGetDlgItem                 = user32.NewProc("GetDlgItem")
	pEnableWindow               = user32.NewProc("EnableWindow")
	pSetFocus                   = user32.NewProc("SetFocus")
	pCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	pDeleteDC                   = gdi32.NewProc("DeleteDC")
	pCreateDIBSection           = gdi32.NewProc("CreateDIBSection")
	pBitBlt                     = gdi32.NewProc("BitBlt")
	pCreateMutex                = kernel32.NewProc("CreateMutexW")
	pGetLastError               = kernel32.NewProc("GetLastError")
	pCloseHandle                = kernel32.NewProc("CloseHandle")
	pOpenClipboard              = user32.NewProc("OpenClipboard")
	pEmptyClipboard             = user32.NewProc("EmptyClipboard")
	pSetClipboardData           = user32.NewProc("SetClipboardData")
	pCloseClipboard             = user32.NewProc("CloseClipboard")
	pGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	pGlobalLock                 = kernel32.NewProc("GlobalLock")
	pGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	pGlobalFree                 = kernel32.NewProc("GlobalFree")
	pIsDialogMessage            = user32.NewProc("IsDialogMessageW")
	pRtlMoveMemory              = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")
	pGdiFlush                   = gdi32.NewProc("GdiFlush")
)

func u16(s string) *uint16    { p, _ := syscall.UTF16PtrFromString(s); return p }
func rgb(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
func loword(v uintptr) int    { return int(v & 0xffff) }
func hiword(v uintptr) int    { return int((v >> 16) & 0xffff) }
func signed16(v int) int32    { return int32(int16(v & 0xffff)) }
func msgBox(owner HWND, title, body string, flags uintptr) int {
	r, _, _ := pMessageBox.Call(uintptr(owner), uintptr(unsafe.Pointer(u16(body))), uintptr(unsafe.Pointer(u16(title))), flags)
	return int(r)
}
func exeDir() string { e, _ := os.Executable(); return filepath.Dir(e) }
func loadAppIcon() HICON {
	p := filepath.Join(exeDir(), "WorkMate.ico")
	r, _, _ := pLoadImage.Call(0, uintptr(unsafe.Pointer(u16(p))), IMAGE_ICON, 0, 0, LR_LOADFROMFILE|LR_DEFAULTSIZE)
	if r != 0 {
		return HICON(r)
	}
	r, _, _ = pLoadIcon.Call(0, IDI_APPLICATION)
	return HICON(r)
}
func setText(hwnd HWND, s string) {
	pSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(u16(s))))
}
func getText(hwnd HWND) string {
	n, _, _ := pGetWindowTextLength.Call(uintptr(hwnd))
	buf := make([]uint16, int(n)+1)
	pGetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}
func getDlgItem(hwnd HWND, id int) HWND {
	r, _, _ := pGetDlgItem.Call(uintptr(hwnd), uintptr(id))
	return HWND(r)
}
func show(hwnd HWND, cmd int) {
	pShowWindow.Call(uintptr(hwnd), uintptr(cmd))
	pUpdateWindow.Call(uintptr(hwnd))
}
func invalidate(hwnd HWND) { pInvalidateRect.Call(uintptr(hwnd), 0, 1) }
func move(hwnd HWND, x, y, w, h int) {
	pMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

func copyUTF16(dst []uint16, s string) { u := syscall.StringToUTF16(s); copy(dst, u) }

func chooseFile(owner HWND, save bool, title, filter, defExt, initial string) (string, bool) {
	buf := make([]uint16, 4096)
	if initial != "" {
		copy(buf, syscall.StringToUTF16(initial))
	}
	f := syscall.StringToUTF16(filter)
	for i := range f {
		if f[i] == '|' {
			f[i] = 0
		}
	}
	ofn := OPENFILENAME{LStructSize: uint32(unsafe.Sizeof(OPENFILENAME{})), HwndOwner: owner, LpstrFilter: &f[0], NFilterIndex: 1, LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), LpstrTitle: u16(title), Flags: OFN_EXPLORER | OFN_PATHMUSTEXIST}
	if save {
		ofn.Flags |= OFN_OVERWRITEPROMPT
		ofn.LpstrDefExt = u16(defExt)
		r, _, _ := pGetSaveFileName.Call(uintptr(unsafe.Pointer(&ofn)))
		if r == 0 {
			return "", false
		}
	} else {
		ofn.Flags |= OFN_FILEMUSTEXIST
		r, _, _ := pGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
		if r == 0 {
			return "", false
		}
	}
	return syscall.UTF16ToString(buf), true
}

func createOwnedWindow(class, title string, clientW, clientH int, owner HWND) HWND {
	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU)
	r := RECT{0, 0, int32(clientW), int32(clientH)}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	var wa RECT
	pSystemParametersInfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	x, y := int(wa.Left)+(int(wa.Right-wa.Left)-w)/2, int(wa.Top)+(int(wa.Bottom-wa.Top)-h)/2
	if x < int(wa.Left) {
		x = int(wa.Left)
	}
	if y < int(wa.Top) {
		y = int(wa.Top)
	}
	hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(title))), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(owner), 0, uintptr(app.hInst), 0)
	return HWND(hwnd)
}

func copyTextToClipboard(owner HWND, text string) error {
	data, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	ok, _, _ := pOpenClipboard.Call(uintptr(owner))
	if ok == 0 {
		return fmt.Errorf("剪贴板正忙，请稍后重试。")
	}
	defer pCloseClipboard.Call()
	mem, _, _ := pGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(len(data)*2))
	if mem == 0 {
		return fmt.Errorf("无法分配剪贴板内存。")
	}
	transferred := false
	defer func() {
		if !transferred {
			pGlobalFree.Call(mem)
		}
	}()
	ptr, _, _ := pGlobalLock.Call(mem)
	if ptr == 0 {
		return fmt.Errorf("无法写入剪贴板。")
	}
	pRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	pGlobalUnlock.Call(mem)
	if ok, _, _ := pEmptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("无法清空剪贴板。")
	}
	if ok, _, _ := pSetClipboardData.Call(CF_UNICODETEXT, mem); ok == 0 {
		return fmt.Errorf("无法复制到剪贴板。")
	}
	transferred = true
	return nil
}
