//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const mainClass = "WorkMateV7MainWindow"
const floatClass = "WorkMateV7FloatWindow"
const calendarClass = "WorkMateV7CalendarWindow"
const timelineClass = "WorkMateV7TimelineWindow"
const settingsClass = "WorkMateV7SettingsWindow"
const dayEditClass = "WorkMateV7DayEditWindow"
const purchaseClass = "WorkMateV8PurchaseWindow"

const (
	mainClientW  = 1120
	mainClientH  = 800
	timerMain    = 1
	hotkeyToggle = 1001
	hotkeyPanic  = 1002
)

type hitRect struct {
	ID string
	R  RECT
}
type appState struct {
	store          *Store
	license        *LicenseManager
	hInst          HINSTANCE
	main           HWND
	floating       HWND
	floatingWanted bool
	hits           []hitRect
	tray           NOTIFYICONDATA
	icon           HICON
	exiting        bool
	calendarMonth  time.Time
	calendarHits   []hitRect
	reminderSeen   map[string]bool
}

var app appState

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Keep a stable mutex name across future releases so different WorkMate
	// versions cannot run side-by-side. We also acquire the V7 legacy mutex
	// so upgrading while 0.7.0 is still running remains single-instance.
	handles, primary := acquireSingleInstance()
	if !primary {
		wakeExistingInstance()
		return
	}
	defer func() {
		for _, h := range handles {
			if h != 0 {
				pCloseHandle.Call(h)
			}
		}
	}()

	st, err := newStore()
	if err != nil {
		msgBox(0, "WorkMate 启动失败", err.Error(), MB_OK|MB_ICONERROR)
		return
	}
	app.store = st
	app.license = newLicenseManager(st.DataDir)
	initOnlinePurchase()
	// Older test builds may have left the Run value pointing at a previous
	// WorkMate.exe. Repair it on every startup when auto-start is enabled so
	// the next Windows login cannot launch an old copy alongside this one.
	if st.Settings.AutoStart {
		_ = setAutoStart(true)
	}
	app.calendarMonth = time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	app.reminderSeen = map[string]bool{}
	for _, k := range st.Runtime.NotificationKeys {
		app.reminderSeen[k] = true
	}
	h, _, _ := pGetModuleHandle.Call(0)
	app.hInst = HINSTANCE(h)
	app.icon = loadAppIcon()

	registerWindowClasses()
	app.main = createMainWindow()
	if app.main == 0 {
		msgBox(0, "WorkMate 启动失败", "无法创建主窗口。", MB_OK|MB_ICONERROR)
		return
	}
	addTrayIcon()
	pRegisterHotKey.Call(uintptr(app.main), hotkeyToggle, MOD_ALT|MOD_CONTROL, uintptr('G'))
	pRegisterHotKey.Call(uintptr(app.main), hotkeyPanic, MOD_ALT|MOD_CONTROL, uintptr('Q'))
	pSetTimer.Call(uintptr(app.main), timerMain, 1000, 0)

	background := false
	for _, a := range os.Args[1:] {
		if a == "--background" {
			background = true
		}
	}
	hidden, floating := startupPresentation(st.Settings, background)
	if hidden {
		show(app.main, SW_HIDE)
	} else {
		show(app.main, SW_SHOWNORMAL)
	}
	if floating {
		toggleFloating(true)
	}

	var msg MSG
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		handled := false
		for _, dialog := range []HWND{settingsWnd, dayEditWnd, purchaseWnd, ledgerWnd} {
			if dialog != 0 {
				if ok, _, _ := pIsDialogMessage.Call(uintptr(dialog), uintptr(unsafe.Pointer(&msg))); ok != 0 {
					handled = true
					break
				}
			}
		}
		if handled {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func acquireSingleInstance() ([]uintptr, bool) {
	// Keep one stable mutex identity across every native WorkMate release.
	// IMPORTANT: CreateMutexW reports ERROR_ALREADY_EXISTS as the last-error
	// of the CreateMutexW call itself.  Reading GetLastError in a later call is
	// racy because other Win32/runtime calls are allowed to overwrite it.
	// V7.1.2 therefore consumes the syscall error returned by Proc.Call
	// directly. This closes the remaining race that could allow two WorkMate
	// processes to create two tray icons on some systems.
	names := []string{
		`Local\WorkMate.SingleInstance`,
		`Local\WorkMateV7NativeSingleInstance`, // V7.0/V7.1 compatibility
	}
	handles := make([]uintptr, 0, len(names))
	for _, name := range names {
		h, _, callErr := pCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(u16(name))))
		if h == 0 {
			for _, owned := range handles {
				if owned != 0 {
					pCloseHandle.Call(owned)
				}
			}
			return nil, false
		}
		if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(ERROR_ALREADY_EXISTS) {
			pCloseHandle.Call(h)
			for _, owned := range handles {
				if owned != 0 {
					pCloseHandle.Call(owned)
				}
			}
			return nil, false
		}
		handles = append(handles, h)
	}
	return handles, true
}

func wakeExistingInstance() {
	// There is a short race between mutex creation and main-window creation.
	// Retry briefly so a second fast double-click still brings the first
	// instance to the foreground instead of appearing to do nothing.
	for i := 0; i < 30; i++ {
		h, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(u16(mainClass))), 0)
		if h != 0 {
			pPostMessage.Call(h, WM_APP_SHOW, 0, 0)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func registerClass(name string, proc uintptr) {
	cur, _, _ := pLoadCursor.Call(0, IDC_ARROW)
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), Style: CS_HREDRAW | CS_VREDRAW, LpfnWndProc: proc, HInstance: app.hInst, HIcon: app.icon, HCursor: HCURSOR(cur), LpszClassName: u16(name), HIconSm: app.icon}
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
}
func registerWindowClasses() {
	if taskbarCreatedMessage == 0 {
		r, _, _ := pRegisterWindowMessage.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
		taskbarCreatedMessage = uint32(r)
	}
	registerClass(mainClass, syscall.NewCallback(mainWndProc))
	registerClass(floatClass, syscall.NewCallback(floatWndProc))
	registerClass(calendarClass, syscall.NewCallback(calendarWndProc))
	registerClass(timelineClass, syscall.NewCallback(timelineWndProc))
	registerClass(settingsClass, syscall.NewCallback(settingsWndProc))
	registerClass(dayEditClass, syscall.NewCallback(dayEditWndProc))
	registerClass(purchaseClass, syscall.NewCallback(purchaseWndProc))
	registerClass(ledgerClass, syscall.NewCallback(ledgerWndProc))
}

func createMainWindow() HWND {
	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX)
	r := RECT{0, 0, mainClientW, mainClientH}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	w := int(r.Right - r.Left)
	h := int(r.Bottom - r.Top)
	x := (1920 - w) / 2
	y := (1080 - h) / 2
	hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(u16(mainClass))), uintptr(unsafe.Pointer(u16(fmt.Sprintf("打工搭子 WorkMate V%s", appVersion)))), style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, uintptr(app.hInst), 0)
	return HWND(hwnd)
}

func mainWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if taskbarCreatedMessage != 0 && msg == taskbarCreatedMessage {
		addTrayIcon()
		syncTaskbarWidget()
		return 0
	}
	switch msg {
	case WM_PAINT:
		paintMain(hwnd)
		return 0
	case WM_TIMER:
		if wParam == timerMain {
			now := time.Now()
			drainOnlinePurchase()
			syncOnlinePurchase(now, false)
			app.store.Tick(now)
			app.license.Tick(now)
			if purchaseWnd != 0 {
				setText(getDlgItem(purchaseWnd, 507), "当前状态："+app.license.StatusText(now))
			}
			checkReminders(now)
			syncTaskbarWidget()
			invalidate(hwnd)
			if app.floating != 0 {
				invalidate(app.floating)
			}
		}
		return 0
	case WM_LBUTTONUP:
		x := signed16(loword(lParam))
		y := signed16(hiword(lParam))
		handleMainClick(int(x), int(y))
		return 0
	case WM_HOTKEY:
		if wParam == hotkeyToggle {
			toggleMain()
		} else if wParam == hotkeyPanic {
			panicHide()
		}
		return 0
	case WM_APP_SHOW:
		showMain()
		return 0
	case WM_APP_EXIT:
		app.exiting = true
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_APP_TRAY:
		handleTrayEvent(wParam, lParam)
		return 0
	case WM_CLOSE:
		if app.exiting {
			pDestroyWindow.Call(uintptr(hwnd))
		} else {
			show(hwnd, SW_HIDE)
			showBalloon("WorkMate 还在后台", "需要时双击托盘图标，或按 Ctrl + Alt + G。", false)
		}
		return 0
	case WM_DESTROY:
		app.exiting = true
		app.floatingWanted = false
		if floatingTaskbarParent != 0 && app.floating != 0 {
			pDestroyWindow.Call(uintptr(app.floating))
		}
		pKillTimer.Call(uintptr(hwnd), timerMain)
		pUnregisterHotKey.Call(uintptr(hwnd), hotkeyToggle)
		pUnregisterHotKey.Call(uintptr(hwnd), hotkeyPanic)
		removeTrayIcon()
		_ = app.store.SaveAll()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func toggleMain() {
	if app.main == 0 {
		return
	}
	v, _, _ := pIsWindowVisible.Call(uintptr(app.main))
	if v != 0 {
		show(app.main, SW_HIDE)
	} else {
		showMain()
	}
}
func showMain() {
	if app.main == 0 {
		return
	}
	show(app.main, SW_RESTORE)
	pSetForegroundWindow.Call(uintptr(app.main))
	invalidate(app.main)
}
func panicHide() {
	hideFloating()
	for _, hwnd := range []HWND{settingsWnd, calendarWnd, timelineWnd, dayEditWnd, purchaseWnd, ledgerWnd, app.main} {
		if hwnd != 0 {
			show(hwnd, SW_HIDE)
		}
	}
}

func handleMainClick(x, y int) {
	for _, h := range app.hits {
		if x >= int(h.R.Left) && x < int(h.R.Right) && y >= int(h.R.Top) && y < int(h.R.Bottom) {
			switch h.ID {
			case "panic":
				panicHide()
			case "buypro":
				openPurchaseWindow()
			case "theme":
				if app.store.Settings.Theme == "Dark" {
					app.store.Settings.Theme = "Light"
				} else {
					app.store.Settings.Theme = "Dark"
				}
				_ = app.store.SaveAll()
				invalidate(app.main)
				if app.floating != 0 {
					invalidate(app.floating)
				}
			case "settings":
				openSettingsWindow()
			case "overtime":
				if app.store.OvertimeRunning() {
					app.store.StopOvertime(time.Now())
				} else {
					app.store.StartOvertime(time.Now())
				}
				invalidate(app.main)
			case "calendar":
				openCalendarWindow()
			case "timeline":
				openTimelineWindow()
			case "ledger":
				openLedgerWindow()
			case "recordtoday":
				openDayEdit(time.Now())
			case "dailyshare":
				if requireFeature(featureDailyCard) {
					saveShareCard("daily")
				}
			case "monthshare":
				if requireFeature(featurePeriodReports) {
					saveShareCard("month")
				}
			case "yearshare":
				if requireFeature(featurePeriodReports) {
					saveShareCard("year")
				}
			case "floating":
				toggleFloating(false)
			case "backup":
				backupFromUI()
			case "restore":
				restoreFromUI()
			}
			return
		}
	}
}

var workMateTrayGUID = [16]byte{0x6b, 0x41, 0x93, 0x2d, 0x28, 0x7f, 0x4c, 0x4e, 0x9e, 0x51, 0x8b, 0x6c, 0x31, 0x02, 0x7a, 0xd5}

func addTrayIcon() {
	// Give the notification icon a stable GUID.  Explorer can otherwise keep
	// a ghost entry after a crash/restart because the old HWND no longer
	// exists.  Deleting the GUID identity before NIM_ADD makes tray creation
	// idempotent and prevents this build from accumulating duplicate icons.
	stale := NOTIFYICONDATA{CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATA{})), UFlags: NIF_GUID, GuidItem: workMateTrayGUID}
	pShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&stale)))

	// NIF_SHOWTIP keeps the normal hover tooltip when NOTIFYICON_VERSION_4
	// is enabled.  More importantly, VERSION_4 packs the callback event into
	// LOWORD(lParam); handleTrayEvent knows how to unpack that format.
	app.tray = NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
		HWnd:             app.main,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP | NIF_GUID | NIF_SHOWTIP,
		UCallbackMessage: WM_APP_TRAY,
		HIcon:            app.icon,
		GuidItem:         workMateTrayGUID,
	}
	copyUTF16(app.tray.SzTip[:], "打工搭子 WorkMate · 右键打开菜单")
	if ok, _, _ := pShellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&app.tray))); ok == 0 {
		return
	}
	app.tray.UTimeoutOrVersion = NOTIFYICON_VERSION_4
	if ok, _, _ := pShellNotifyIcon.Call(NIM_SETVERSION, uintptr(unsafe.Pointer(&app.tray))); ok == 0 {
		// Fallback to the legacy callback layout if VERSION_4 is not accepted.
		app.tray.UTimeoutOrVersion = 0
	}
}
func removeTrayIcon() {
	if app.tray.HWnd != 0 {
		pShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&app.tray)))
	}
}
func showBalloon(title, body string, warn bool) {
	n := app.tray
	n.UFlags = NIF_INFO
	copyUTF16(n.SzInfoTitle[:], title)
	copyUTF16(n.SzInfo[:], body)
	if warn {
		n.DwInfoFlags = NIIF_WARNING
	} else {
		n.DwInfoFlags = NIIF_INFO
	}
	pShellNotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&n)))
}
func handleTrayEvent(wp, lp uintptr) {
	// When NOTIFYICON_VERSION_4 is active, LOWORD(lParam) is the event and
	// HIWORD(lParam) is the icon ID.  The old code compared the whole lParam,
	// so a right click could never match WM_RBUTTONUP on modern Windows.
	event := uint32(lp)
	if app.tray.UTimeoutOrVersion == NOTIFYICON_VERSION_4 {
		event = uint32(loword(lp))
	}

	switch event {
	case WM_LBUTTONDBLCLK, NIN_SELECT, NIN_KEYSELECT:
		showMain()
	case WM_RBUTTONUP, WM_CONTEXTMENU:
		showTrayMenu(wp)
	}
}

const (
	trayCmdOpen     = 2101
	trayCmdFloating = 2102
	trayCmdOvertime = 2103
	trayCmdPanic    = 2104
	trayCmdSettings = 2105
	trayCmdExit     = 2199
)

func showTrayMenu(_ uintptr) {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)

	pAppendMenu.Call(m, MF_STRING, trayCmdOpen, uintptr(unsafe.Pointer(u16("打开 WorkMate"))))
	pAppendMenu.Call(m, MF_STRING, trayCmdFloating, uintptr(unsafe.Pointer(u16("显示 / 隐藏桌面挂件"))))

	overtimeLabel := "开始加班"
	if app.store != nil && app.store.OvertimeRunning() {
		overtimeLabel = "结束加班"
	}
	pAppendMenu.Call(m, MF_STRING, trayCmdOvertime, uintptr(unsafe.Pointer(u16(overtimeLabel))))
	pAppendMenu.Call(m, MF_SEPARATOR, 0, 0)
	pAppendMenu.Call(m, MF_STRING, trayCmdPanic, uintptr(unsafe.Pointer(u16("老板来了 · 立即隐藏"))))
	pAppendMenu.Call(m, MF_STRING, trayCmdSettings, uintptr(unsafe.Pointer(u16("设置"))))
	pAppendMenu.Call(m, MF_SEPARATOR, 0, 0)
	pAppendMenu.Call(m, MF_STRING, trayCmdExit, uintptr(unsafe.Pointer(u16("完全退出 WorkMate"))))

	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// Microsoft recommends making the owner foreground before TrackPopupMenu
	// for notification-area shortcut menus.  Posting WM_NULL afterwards fixes
	// the classic issue where a subsequent tray menu will not dismiss/open.
	pSetForegroundWindow.Call(uintptr(app.main))
	cmd, _, _ := pTrackPopupMenu.Call(
		m,
		TPM_RIGHTBUTTON|TPM_RETURNCMD,
		uintptr(pt.X), uintptr(pt.Y),
		0,
		uintptr(app.main),
		0,
	)
	pPostMessage.Call(uintptr(app.main), WM_NULL, 0, 0)

	switch cmd {
	case trayCmdOpen:
		showMain()
	case trayCmdFloating:
		toggleFloating(false)
	case trayCmdOvertime:
		if app.store != nil {
			if app.store.OvertimeRunning() {
				app.store.StopOvertime(time.Now())
			} else {
				app.store.StartOvertime(time.Now())
			}
			invalidate(app.main)
			if app.floating != 0 {
				invalidate(app.floating)
			}
		}
	case trayCmdPanic:
		panicHide()
	case trayCmdSettings:
		showMain()
		openSettingsWindow()
	case trayCmdExit:
		// Post the request to the main window so shutdown always follows the
		// same path: unregister hotkeys -> remove tray icon -> persist data ->
		// quit the message loop.  After this there is no WorkMate.exe process.
		pPostMessage.Call(uintptr(app.main), WM_APP_EXIT, 0, 0)
	}
}

func backupFromUI() {
	name := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", "WorkMate_Backup_"+time.Now().Format("2006-01-02")+".zip")
	p, ok := chooseFile(app.main, true, "备份 WorkMate 数据", "ZIP 备份 (*.zip)|*.zip|所有文件 (*.*)|*.*|", "zip", name)
	if !ok {
		return
	}
	if err := app.store.CreateBackup(p); err != nil {
		msgBox(app.main, "备份失败", err.Error(), MB_OK|MB_ICONERROR)
	} else {
		msgBox(app.main, "备份完成", "备份已保存到：\n"+p, MB_OK|MB_ICONINFORMATION)
	}
}
func restoreFromUI() {
	p, ok := chooseFile(app.main, false, "恢复 WorkMate 数据", "ZIP 备份 (*.zip)|*.zip|所有文件 (*.*)|*.*|", "zip", "")
	if !ok {
		return
	}
	if msgBox(app.main, "恢复数据", "恢复会覆盖当前本地数据；程序会先自动做一份安全备份。\n\n继续吗？", MB_YESNO|MB_ICONWARNING) != IDYES {
		return
	}
	if err := app.store.RestoreBackup(p); err != nil {
		msgBox(app.main, "恢复失败", err.Error(), MB_OK|MB_ICONERROR)
	} else {
		msgBox(app.main, "恢复完成", "数据已经恢复。", MB_OK|MB_ICONINFORMATION)
		applyFloatingSettings()
		if calendarWnd != 0 {
			invalidate(calendarWnd)
		}
		if ledgerWnd != 0 {
			refreshLedgerWindow(ledgerWnd)
		}
		invalidate(app.main)
	}
}

func parseFloatText(hwnd HWND) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(getText(hwnd)), 64)
	return v
}
func parseIntText(hwnd HWND) int { v, _ := strconv.Atoi(strings.TrimSpace(getText(hwnd))); return v }
