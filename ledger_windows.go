//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const ledgerClass = "WorkMateLedgerWindow"
const (
	lvmFirst            = 0x1000
	lvmInsertColumnW    = lvmFirst + 97
	lvmInsertItemW      = lvmFirst + 77
	lvmSetItemTextW     = lvmFirst + 116
	lvmDeleteAllItems   = lvmFirst + 9
	lvmGetNextItem      = lvmFirst + 12
	lvmSetItemState     = lvmFirst + 43
	lvmSetExtendedStyle = lvmFirst + 54
)

type listViewColumn struct {
	Mask                                                               uint32
	Format, Width                                                      int32
	Text                                                               *uint16
	TextMax, SubItem, Image, Order, MinWidth, DefaultWidth, IdealWidth int32
}
type listViewItem struct {
	Mask             uint32
	Item, SubItem    int32
	State, StateMask uint32
	Text             *uint16
	TextMax, Image   int32
	Param            uintptr
	Indent, GroupID  int32
	Columns          uint32
	ColumnIndices    *uint32
	ColumnFormats    *int32
	Group            int32
}
type notifyHeader struct {
	Window HWND
	ID     uintptr
	Code   uint32
}

var ledgerWnd HWND
var ledgerFont HFONT
var ledgerMonth time.Time
var ledgerVisibleRows []LedgerRow
var ledgerSnapshot MonthlyLedger

func newUIFont(size int) HFONT {
	font, _, _ := pCreateFont.Call(uintptr(^uint32(size-1)), 0, 0, 0, FW_NORMAL, 0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY, 0, uintptr(unsafe.Pointer(u16("Microsoft YaHei UI"))))
	return HFONT(font)
}
func openLedgerWindow() {
	if ledgerWnd != 0 {
		show(ledgerWnd, SW_RESTORE)
		pSetForegroundWindow.Call(uintptr(ledgerWnd))
		refreshLedgerWindow(ledgerWnd)
		return
	}
	now := time.Now()
	ledgerMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	ledgerWnd = createOwnedWindow(ledgerClass, "工时账本 · 查看记录 / 按月导出", 1010, 664, app.main)
	show(ledgerWnd, SW_SHOW)
}
func ledgerWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		init := struct{ Size, Classes uint32 }{8, 1} // ICC_LISTVIEW_CLASSES
		syscall.NewLazyDLL("comctl32.dll").NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&init)))
		ledgerFont = newUIFont(14)
		ctl := func(class, text string, style uintptr, x, y, w, h, id int) HWND {
			child := createCtl(hwnd, class, text, style, x, y, w, h, id)
			pSendMessage.Call(uintptr(child), WM_SETFONT, uintptr(ledgerFont), 1)
			return child
		}
		ctl("BUTTON", "上个月", BS_PUSHBUTTON|WS_TABSTOP, 24, 20, 84, 34, 601)
		ctl("BUTTON", "本月", BS_PUSHBUTTON|WS_TABSTOP, 120, 20, 84, 34, 603)
		ctl("BUTTON", "下个月", BS_PUSHBUTTON|WS_TABSTOP, 216, 20, 84, 34, 602)
		ctl("STATIC", "", 0, 320, 24, 270, 28, 604)
		ctl("STATIC", "查看免费 · 导出为 Pro 功能", 0, 640, 24, 340, 28, 605)
		ctl("STATIC", "", 0, 24, 68, 960, 32, 606)
		ctl("STATIC", "", 0, 24, 104, 750, 30, 607)
		filter := ctl("BUTTON", "只看有记录的日期", BS_AUTOCHECKBOX|WS_TABSTOP, 800, 104, 184, 28, 608)
		pSendMessage.Call(uintptr(filter), BM_SETCHECK, BST_CHECKED, 0)
		list := ctl("SysListView32", "", WS_BORDER|WS_TABSTOP|WS_VSCROLL|0x1|0x4|0x8, 24, 146, 960, 398, 610) // report, single select, show selection
		pSendMessage.Call(uintptr(list), lvmSetExtendedStyle, 0, 0x1|0x20|0x10000)                            // grid, full row, double buffering
		columns := []struct {
			title string
			width int32
		}{{"日期", 104}, {"星期", 54}, {"日期性质", 110}, {"工时 h", 76}, {"加班 h", 76}, {"年假 天", 76}, {"补休 h", 76}, {"请假 天", 76}, {"记录状态", 132}, {"备注", 210}}
		for index, column := range columns {
			c := listViewColumn{Mask: 1 | 2 | 4 | 8, Width: column.width, Text: u16(column.title), SubItem: int32(index)}
			pSendMessage.Call(uintptr(list), lvmInsertColumnW, uintptr(index), uintptr(unsafe.Pointer(&c)))
		}
		ctl("BUTTON", "日历 / 休假计划", BS_PUSHBUTTON|WS_TABSTOP, 24, 562, 162, 38, 612)
		ctl("BUTTON", "刷新", BS_PUSHBUTTON|WS_TABSTOP, 198, 562, 84, 38, 613)
		ctl("BUTTON", "编辑选中日期", BS_PUSHBUTTON|WS_TABSTOP, 294, 562, 162, 38, 614)
		ctl("BUTTON", "导出 Excel 表格（Pro）", BS_PUSHBUTTON|WS_TABSTOP, 742, 562, 242, 38, 611)
		ctl("STATIC", "工时来自本机运行与手动记录，不是考勤证明。只统计已发生记录，未来休假计划请在日历查看。\r\n加班计时中的时长也会统计；导出时读取最新数据。记录和文件始终留在本机。", 0, 24, 612, 960, 44, 615)
		refreshLedgerWindow(hwnd)
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case 601:
			ledgerMonth = ledgerMonth.AddDate(0, -1, 0)
			refreshLedgerWindow(hwnd)
		case 602:
			ledgerMonth = ledgerMonth.AddDate(0, 1, 0)
			refreshLedgerWindow(hwnd)
		case 603:
			now := time.Now()
			ledgerMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			refreshLedgerWindow(hwnd)
		case 608, 613:
			refreshLedgerWindow(hwnd)
		case 612:
			app.calendarMonth = ledgerMonth
			openCalendarWindow()
			invalidate(calendarWnd)
		case 614:
			editLedgerSelection(hwnd)
		case 611:
			if requireFeature(featureLedgerExport) {
				exportLedgerFromUI(hwnd)
			}
		}
		return 0
	case 0x004e: // WM_NOTIFY
		if lParam == 0 {
			return 0
		}
		var header notifyHeader
		pRtlMoveMemory.Call(uintptr(unsafe.Pointer(&header)), lParam, unsafe.Sizeof(header))
		if header.ID == 610 && int32(header.Code) == -3 { // NM_DBLCLK
			editLedgerSelection(hwnd)
		}
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		if ledgerFont != 0 {
			pDeleteObject.Call(uintptr(ledgerFont))
			ledgerFont = 0
		}
		ledgerWnd = 0
		ledgerVisibleRows = nil
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}
func refreshLedgerWindow(hwnd HWND) {
	if hwnd == 0 {
		return
	}
	if ledgerMonth.IsZero() {
		now := time.Now()
		ledgerMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	l, err := app.store.MonthlyLedgerAt(ledgerMonth.Year(), ledgerMonth.Month(), time.Now())
	if err != nil {
		setText(getDlgItem(hwnd, 607), err.Error())
		return
	}
	ledgerSnapshot = l
	setText(getDlgItem(hwnd, 604), fmt.Sprintf("%d 年 %d 月 · 工时账本", ledgerMonth.Year(), ledgerMonth.Month()))
	t := l.Totals
	setText(getDlgItem(hwnd, 606), fmt.Sprintf("有工时记录 %d 天 · 工作 %.1fh · 加班 %.1fh · 年假 %.1f天 · 补休 %.1fh · 请假/病假 %.1f天", t.RecordedDays, t.WorkMinutes/60, t.OvertimeMinutes/60, t.AnnualDays, t.CompHours, t.LeaveDays+t.SickDays))
	hint := "双击日期可补记。查看记录免费；Pro 可按月导出，省去手动抄表。"
	if !l.HasEntries() {
		hint = "这个月还没有记录：可以开始加班计时，或到日历补记。"
	}
	setText(getDlgItem(hwnd, 607), hint)
	label := "导出 Excel 表格（Pro）"
	if canUseFeature(app.license, featureLedgerExport, time.Now()) {
		label = "导出 Excel 表格（CSV）"
	}
	setText(getDlgItem(hwnd, 611), label)
	list := getDlgItem(hwnd, 610)
	selected, _, _ := pSendMessage.Call(uintptr(list), lvmGetNextItem, ^uintptr(0), 2)
	selectedDate := ""
	if int32(selected) >= 0 && int(selected) < len(ledgerVisibleRows) {
		selectedDate = ledgerVisibleRows[selected].Date.Format("2006-01-02")
	}
	pSendMessage.Call(uintptr(list), lvmDeleteAllItems, 0, 0)
	ledgerVisibleRows = nil
	for _, row := range l.Rows {
		if checked(hwnd, 608) && !row.HasEntry() {
			continue
		}
		index := len(ledgerVisibleRows)
		ledgerVisibleRows = append(ledgerVisibleRows, row)
		item := listViewItem{Mask: 1, Item: int32(index), Text: u16(row.Date.Format("2006-01-02"))}
		pSendMessage.Call(uintptr(list), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))
		values := []string{weekdayText(row.Date), row.DayName, ledgerNumber(row.WorkMinutes / 60), ledgerNumber(row.OvertimeMinutes / 60), ledgerNumber(row.AnnualDays), ledgerNumber(row.CompHours), ledgerNumber(row.LeaveDays + row.SickDays), row.Status, strings.NewReplacer("\r", " ", "\n", " / ").Replace(row.Note)}
		for column, value := range values {
			item := listViewItem{SubItem: int32(column + 1), Text: u16(value)}
			pSendMessage.Call(uintptr(list), lvmSetItemTextW, uintptr(index), uintptr(unsafe.Pointer(&item)))
		}
		if row.Date.Format("2006-01-02") == selectedDate {
			item := listViewItem{State: 2, StateMask: 2}
			pSendMessage.Call(uintptr(list), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&item)))
		}
	}
	previous, next := uintptr(1), uintptr(1)
	if ledgerMonth.Year() <= 1970 && ledgerMonth.Month() == time.January {
		previous = 0
	}
	now := time.Now()
	current := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, ledgerMonth.Location())
	if !ledgerMonth.Before(current) {
		next = 0
	}
	pEnableWindow.Call(uintptr(getDlgItem(hwnd, 601)), previous)
	pEnableWindow.Call(uintptr(getDlgItem(hwnd, 602)), next)
}
func editLedgerSelection(hwnd HWND) {
	index, _, _ := pSendMessage.Call(uintptr(getDlgItem(hwnd, 610)), lvmGetNextItem, ^uintptr(0), 2)
	if int32(index) < 0 || int(index) >= len(ledgerVisibleRows) {
		setText(getDlgItem(hwnd, 607), "请先选择一个日期，或打开日历补记。 ")
		return
	}
	openDayEdit(ledgerVisibleRows[index].Date)
}
func exportLedgerFromUI(hwnd HWND) {
	name := fmt.Sprintf("WorkMate-工时账本-%d-%02d.csv", ledgerMonth.Year(), ledgerMonth.Month())
	path, ok := chooseFile(hwnd, true, "导出工时账本（Excel 可打开）", "CSV 表格 (*.csv)|*.csv|所有文件 (*.*)|*.*|", "csv", filepath.Join(os.Getenv("USERPROFILE"), "Desktop", name))
	if !ok {
		return
	}
	// A file dialog can remain open across trial expiry; check before writing.
	if !requireFeature(featureLedgerExport) {
		return
	}
	if err := app.store.ExportMonthlyLedger(path, ledgerMonth.Year(), ledgerMonth.Month(), time.Now()); err != nil {
		msgBox(hwnd, "导出失败", err.Error(), MB_OK|MB_ICONWARNING)
		return
	}
	refreshLedgerWindow(hwnd)
	msgBox(hwnd, "工时账本已导出", "已保存到：\n"+path+"\n\n可用 Excel 或 WPS 打开，中文列名与备注已保留。", MB_OK|MB_ICONINFORMATION)
}
