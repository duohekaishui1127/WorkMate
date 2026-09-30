//go:build windows

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

var settingsWnd HWND
var settingsFont HFONT
var dayEditFont HFONT
var dayEditWnd HWND
var dayEditDate time.Time
var calendarWnd HWND
var timelineWnd HWND

func createCtl(parent HWND, class, text string, style uintptr, x, y, w, h, id int) HWND {
	r, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(text))), style|WS_CHILD|WS_VISIBLE, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(parent), uintptr(id), uintptr(app.hInst), 0)
	hw := HWND(r)
	if settingsFont != 0 {
		pSendMessage.Call(uintptr(hw), WM_SETFONT, uintptr(settingsFont), 1)
	}
	return hw
}

type settingChoice struct{ Key, Label string }

var floatingModeChoices = []settingChoice{{"Screen", "桌面悬浮"}, {"Taskbar", "任务栏上沿"}, {"TaskbarEmbed", "底部任务栏内部"}}
var floatingSizeChoices = []settingChoice{{"Small", "小"}, {"Medium", "中"}, {"Large", "大"}}
var floatingPaletteChoices = []settingChoice{{"Lavender", "薰衣草"}, {"Peach", "杏色"}, {"Mint", "薄荷"}, {"Pink", "粉色"}, {"Sky", "天空蓝"}}
var applyAutoStart = setAutoStart

func openSettingsWindow() {
	if settingsWnd != 0 {
		show(settingsWnd, SW_RESTORE)
		pSetForegroundWindow.Call(uintptr(settingsWnd))
		return
	}
	settingsWnd = createOwnedWindow(settingsClass, "WorkMate 设置", 920, 666, app.main)
	show(settingsWnd, SW_SHOW)
}

func settingsWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		f, _, _ := pCreateFont.Call(uintptr(^uint32(13-1)), 0, 0, 0, FW_NORMAL, 0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY, 0, uintptr(unsafe.Pointer(u16("Microsoft YaHei UI"))))
		settingsFont = HFONT(f)
		s := app.store.Settings
		createCtl(hwnd, "STATIC", "工作与收入", 0, 24, 18, 400, 26, 0)
		createCtl(hwnd, "STATIC", "启动、提醒与桌面挂件", 0, 478, 18, 410, 26, 0)
		edit := func(x, y int, label string, id int, value string) {
			createCtl(hwnd, "STATIC", label, 0, x, y+2, 174, 26, 0)
			createCtl(hwnd, "EDIT", value, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, x+178, y, 232, 28, id)
		}
		rows := []struct {
			label string
			id    int
			value string
		}{
			{"月薪（元）", 100, fmt.Sprintf("%.2f", s.MonthlySalary)},
			{"加班基数（0=月薪）", 101, fmt.Sprintf("%.2f", s.OvertimeBaseSalary)},
			{"月计薪天数", 108, fmt.Sprintf("%.2f", s.MonthlyWorkDays)},
			{"上班时间 HH:MM", 102, s.WorkStart}, {"下班时间 HH:MM", 103, s.WorkEnd},
			{"午休开始 HH:MM", 104, s.LunchStart}, {"午休结束 HH:MM", 105, s.LunchEnd},
			{"发薪日（1-31）", 106, strconv.Itoa(s.PaydayDay)},
			{"参加工作日期", 107, s.CumulativeWorkStartDate},
		}
		for i, r := range rows {
			edit(24, 48+i*40, r.label, r.id, r.value)
		}
		edit(478, 48, "晚间提醒 HH:MM", 122, fmt.Sprintf("%02d:%02d", s.ReminderEveningHour, s.ReminderEveningMinute))
		edit(478, 88, "挂件不透明度（%）", 123, fmt.Sprintf("%.0f", s.FloatingOpacity*100))
		combo := func(y int, label string, id int, choices []settingChoice, value string) {
			createCtl(hwnd, "STATIC", label, 0, 478, y+2, 174, 26, 0)
			ctl := createCtl(hwnd, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_TABSTOP|WS_VSCROLL, 656, y, 232, 180, id)
			for i, c := range choices {
				pSendMessage.Call(uintptr(ctl), CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(c.Label))))
				if c.Key == value {
					pSendMessage.Call(uintptr(ctl), CB_SETCURSEL, uintptr(i), 0)
				}
			}
		}
		combo(128, "挂件尺寸", 124, floatingSizeChoices, s.FloatingSize)
		combo(168, "挂件配色", 125, floatingPaletteChoices, s.FloatingPalette)
		checks := []struct {
			id, x, y int
			label    string
			value    bool
		}{
			{110, 24, 412, "周六通常上班", s.WorkOnSaturday}, {111, 24, 442, "周日通常上班", s.WorkOnSunday},
			{112, 24, 472, "使用中国大陆节假日与调休", s.UseMainlandHolidayCalendar},
			{113, 478, 216, "登录 Windows 后自动静默启动", s.AutoStart}, {114, 478, 246, "手动启动先隐藏到托盘", s.StartHidden},
			{120, 478, 276, "手动启动时显示挂件", s.ShowFloatingOnStartup}, {115, 478, 306, "自动启动时显示挂件", s.ShowFloatingOnAutoStart},
			{116, 478, 336, "启用提醒（下班免费，提前通知 Pro）", s.ReminderEnabled}, {117, 478, 366, "挂件显示今日估算收入", s.FloatingShowEarned},
			{118, 478, 396, "挂件显示倒计时", s.FloatingShowCountdown}, {119, 478, 426, "挂件显示进度", s.FloatingShowProgress},
			{126, 478, 456, "挂件显示短句", s.FloatingShowPhrase}, {127, 478, 486, "挂件使用紧凑模式", s.FloatingCompact},
			{128, 478, 546, "靠边自动隐藏，鼠标靠近展开", s.FloatingAutoHide},
			{129, 478, 576, "挂件始终置顶", s.FloatingTopmost},
		}
		for _, c := range checks {
			ctl := createCtl(hwnd, "BUTTON", c.label, BS_AUTOCHECKBOX|WS_TABSTOP, c.x, c.y, 410, 26, c.id)
			if c.value {
				pSendMessage.Call(uintptr(ctl), BM_SETCHECK, BST_CHECKED, 0)
			}
		}
		combo(516, "挂件位置", 121, floatingModeChoices, s.FloatingMode)
		updateFloatingModeControls(hwnd)
		createCtl(hwnd, "STATIC", "任务栏内嵌随任务栏显示；空位不足时自动贴上沿。", 0, 24, 634, 410, 20, 130)
		createCtl(hwnd, "STATIC", "今日收入按日程估算；报告收入按记录工时估算。年假额度按参加工作日期估算。\r\n拖动挂件靠近屏幕边缘即可吸附。\r\nCtrl+Alt+Q 或右键挂件可立即隐藏全部窗口。", 0, 24, 530, 410, 102, 0)
		createCtl(hwnd, "BUTTON", "保存设置", BS_PUSHBUTTON|WS_TABSTOP, 674, 616, 100, 36, 190)
		createCtl(hwnd, "BUTTON", "取消", BS_PUSHBUTTON|WS_TABSTOP, 798, 616, 90, 36, 191)
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case 121:
			if hiword(wParam) == 1 {
				updateFloatingModeControls(hwnd)
			} // CBN_SELCHANGE
		case 190:
			saveSettingsFromWindow(hwnd)
		case 191:
			pDestroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		if settingsFont != 0 {
			pDeleteObject.Call(uintptr(settingsFont))
			settingsFont = 0
		}
		settingsWnd = 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func updateFloatingModeControls(hwnd HWND) {
	enabled := uintptr(1)
	if selectedChoice(hwnd, 121, floatingModeChoices) == "TaskbarEmbed" {
		enabled = 0
	}
	for _, id := range []int{126, 127, 128, 129} {
		pEnableWindow.Call(uintptr(getDlgItem(hwnd, id)), enabled)
	}
}

func checked(hwnd HWND, id int) bool {
	r, _, _ := pSendMessage.Call(uintptr(getDlgItem(hwnd, id)), BM_GETCHECK, 0, 0)
	return r == BST_CHECKED
}
func selectedChoice(hwnd HWND, id int, choices []settingChoice) string {
	r, _, _ := pSendMessage.Call(uintptr(getDlgItem(hwnd, id)), CB_GETCURSEL, 0, 0)
	if int(r) < 0 || int(r) >= len(choices) {
		return ""
	}
	return choices[int(r)].Key
}
func saveSettingsFromWindow(hwnd HWND) {
	next := app.store.Settings
	fail := func(err error) { msgBox(hwnd, "设置有误", err.Error(), MB_OK|MB_ICONWARNING) }
	for _, item := range []struct {
		id    int
		label string
		dest  *float64
	}{
		{100, "月薪", &next.MonthlySalary}, {101, "加班工资基数", &next.OvertimeBaseSalary}, {108, "月计薪天数", &next.MonthlyWorkDays}, {123, "挂件不透明度", &next.FloatingOpacity},
	} {
		v, err := strconv.ParseFloat(strings.TrimSpace(getText(getDlgItem(hwnd, item.id))), 64)
		if err != nil {
			fail(fmt.Errorf("%s请输入有效数字。", item.label))
			return
		}
		*item.dest = v
	}
	next.FloatingOpacity /= 100
	next.WorkStart = strings.TrimSpace(getText(getDlgItem(hwnd, 102)))
	next.WorkEnd = strings.TrimSpace(getText(getDlgItem(hwnd, 103)))
	next.LunchStart = strings.TrimSpace(getText(getDlgItem(hwnd, 104)))
	next.LunchEnd = strings.TrimSpace(getText(getDlgItem(hwnd, 105)))
	pd, err := strconv.Atoi(strings.TrimSpace(getText(getDlgItem(hwnd, 106))))
	if err != nil {
		fail(fmt.Errorf("发薪日请输入 1-31 的整数。"))
		return
	}
	next.PaydayDay = pd
	next.CumulativeWorkStartDate = strings.TrimSpace(getText(getDlgItem(hwnd, 107)))
	minutes, err := hmToMinutes(getText(getDlgItem(hwnd, 122)))
	if err != nil {
		fail(fmt.Errorf("晚间提醒时间请使用 HH:MM 格式。"))
		return
	}
	next.ReminderEveningHour, next.ReminderEveningMinute = minutes/60, minutes%60
	next.FloatingSize = selectedChoice(hwnd, 124, floatingSizeChoices)
	next.FloatingPalette = selectedChoice(hwnd, 125, floatingPaletteChoices)
	next.WorkOnSaturday, next.WorkOnSunday = checked(hwnd, 110), checked(hwnd, 111)
	next.UseMainlandHolidayCalendar = checked(hwnd, 112)
	next.AutoStart, next.StartHidden = checked(hwnd, 113), checked(hwnd, 114)
	next.ShowFloatingOnAutoStart, next.ShowFloatingOnStartup = checked(hwnd, 115), checked(hwnd, 120)
	next.ReminderEnabled = checked(hwnd, 116)
	next.FloatingShowEarned, next.FloatingShowCountdown, next.FloatingShowProgress = checked(hwnd, 117), checked(hwnd, 118), checked(hwnd, 119)
	next.FloatingShowPhrase, next.FloatingCompact = checked(hwnd, 126), checked(hwnd, 127)
	next.FloatingAutoHide, next.FloatingTopmost = checked(hwnd, 128), checked(hwnd, 129)
	next.FloatingMode = selectedChoice(hwnd, 121, floatingModeChoices)
	if err := app.store.UpdateSettings(next, time.Now()); err != nil {
		fail(err)
		return
	}
	if err := applyAutoStart(next.AutoStart); err != nil {
		msgBox(hwnd, "开机启动设置失败", "其他设置已保存，但开机启动未更新："+err.Error(), MB_OK|MB_ICONWARNING)
		return
	}
	pDestroyWindow.Call(uintptr(hwnd))
	invalidate(app.main)
	if calendarWnd != 0 {
		invalidate(calendarWnd)
	}
	applyFloatingSettings()
}

func openCalendarWindow() {
	if calendarWnd != 0 {
		show(calendarWnd, SW_RESTORE)
		pSetForegroundWindow.Call(uintptr(calendarWnd))
		return
	}
	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU)
	r, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(u16(calendarClass))), uintptr(unsafe.Pointer(u16("劳动日历 · 单击日期可记录"))), style, 350, 130, 850, 690, uintptr(app.main), 0, uintptr(app.hInst), 0)
	calendarWnd = HWND(r)
	show(calendarWnd, SW_SHOW)
}
func calendarWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		paintCalendar(hwnd)
		return 0
	case WM_LBUTTONUP:
		x := int(signed16(loword(lParam)))
		y := int(signed16(hiword(lParam)))
		handleCalendarClick(x, y)
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		calendarWnd = 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func paintCalendar(hwnd HWND) {
	var ps PAINTSTRUCT
	hdc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	p := currentPalette()
	var cr RECT
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&cr)))
	fill(HDC(hdc), cr, p.Bg)
	app.calendarHits = app.calendarHits[:0]
	m := app.calendarMonth
	balance := app.store.AnnualLeaveBalance(time.Now())
	drawText(HDC(hdc), annualLeaveText(balance), RECT{28, 578, 790, 606}, 12, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(HDC(hdc), fmt.Sprintf("%d 年 %d 月", m.Year(), m.Month()), RECT{250, 22, 590, 58}, 25, FW_BOLD, p.Text, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	calButton(HDC(hdc), "prev", "‹ 上月", RECT{28, 22, 110, 58}, p)
	calButton(HDC(hdc), "next", "下月 ›", RECT{700, 22, 782, 58}, p)
	calButton(HDC(hdc), "today", "今天", RECT{610, 22, 682, 58}, p)
	calButton(HDC(hdc), "import", "导入节假日", RECT{28, 616, 132, 652}, p)
	calButton(HDC(hdc), "export", "导出模板", RECT{144, 616, 236, 652}, p)
	days := []string{"一", "二", "三", "四", "五", "六", "日"}
	left, top, cellW, cellH := 28, 85, 108, 82
	for i, d := range days {
		drawText(HDC(hdc), "周"+d, RECT{int32(left + i*cellW), 68, int32(left + (i+1)*cellW), 90}, 12, FW_SEMIBOLD, p.Sub, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}
	first := time.Date(m.Year(), m.Month(), 1, 0, 0, 0, 0, time.Local)
	offset := (int(first.Weekday()) + 6) % 7
	dim := daysInMonth(m.Year(), m.Month())
	today := time.Now().Format("2006-01-02")
	for day := 1; day <= dim; day++ {
		idx := offset + day - 1
		row, col := idx/7, idx%7
		x := left + col*cellW
		y := top + row*cellH
		r := RECT{int32(x), int32(y), int32(x + cellW - 8), int32(y + cellH - 8)}
		dt := time.Date(m.Year(), m.Month(), day, 0, 0, 0, 0, time.Local)
		info := app.store.DayInfo(dt)
		bg := p.Surface
		if info.IsRestDay {
			bg = p.Mint
		}
		if info.Type == "AdjustedWorkday" {
			bg = p.Peach
		}
		if dt.Format("2006-01-02") == today {
			bg = p.Lavender
		}
		roundBox(HDC(hdc), r, bg, p.Border, 14)
		drawText(HDC(hdc), strconv.Itoa(day), RECT{r.Left + 8, r.Top + 4, r.Right - 8, r.Top + 30}, 14, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(HDC(hdc), info.Name, RECT{r.Left + 8, r.Top + 29, r.Right - 6, r.Top + 50}, 10, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		badge := calendarBadge(dt)
		if badge != "" {
			drawText(HDC(hdc), badge, RECT{r.Left + 8, r.Top + 51, r.Right - 6, r.Bottom - 4}, 10, FW_SEMIBOLD, p.Accent, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		}
		app.calendarHits = append(app.calendarHits, hitRect{"day:" + dt.Format("2006-01-02"), r})
	}
	drawText(HDC(hdc), "绿色=休息 · 杏色=调休工作日 · 紫色=今天。单击补记加班（免费）；年假 / 补休 / 请假管理为 Pro。", RECT{260, 616, 790, 652}, 11, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}
func calButton(hdc HDC, id, label string, r RECT, p palette) {
	roundBox(hdc, r, p.Surface, p.Border, 12)
	drawText(hdc, label, r, 12, FW_SEMIBOLD, p.Text, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	app.calendarHits = append(app.calendarHits, hitRect{id, r})
}
func calendarBadge(dt time.Time) string {
	key := dt.Format("2006-01-02")
	var ot float64
	var parts []string
	app.store.mu.Lock()
	defer app.store.mu.Unlock()
	for _, r := range app.store.Records {
		if r.Date == key {
			ot = r.OvertimeMinutes / 60
		}
	}
	if ot > 0 {
		parts = append(parts, fmt.Sprintf("加班%.1fh", ot))
	}
	for _, l := range app.store.Leaves {
		if l.Date == key {
			if l.AnnualDays > 0 {
				parts = append(parts, fmt.Sprintf("年假%.1f天", l.AnnualDays))
			}
			if l.CompHours > 0 {
				parts = append(parts, fmt.Sprintf("补休%.1fh", l.CompHours))
			}
			if l.LeaveDays > 0 {
				parts = append(parts, fmt.Sprintf("请假%.1f天", l.LeaveDays))
			}
		}
	}
	return strings.Join(parts, " · ")
}
func handleCalendarClick(x, y int) {
	for _, h := range app.calendarHits {
		if x >= int(h.R.Left) && x < int(h.R.Right) && y >= int(h.R.Top) && y < int(h.R.Bottom) {
			switch h.ID {
			case "prev":
				app.calendarMonth = app.calendarMonth.AddDate(0, -1, 0)
				invalidate(calendarWnd)
			case "next":
				app.calendarMonth = app.calendarMonth.AddDate(0, 1, 0)
				invalidate(calendarWnd)
			case "today":
				n := time.Now()
				app.calendarMonth = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.Local)
				invalidate(calendarWnd)
			case "import":
				if requireFeature(featureHolidayConfig) {
					importHolidayUI()
				}
			case "export":
				if requireFeature(featureHolidayConfig) {
					exportHolidayUI()
				}
			default:
				if strings.HasPrefix(h.ID, "day:") {
					d, _ := time.ParseInLocation("2006-01-02", strings.TrimPrefix(h.ID, "day:"), time.Local)
					openDayEdit(d)
				}
			}
			return
		}
	}
}

func importHolidayUI() {
	p, ok := chooseFile(calendarWnd, false, "导入节假日配置", "JSON 配置 (*.json)|*.json|所有文件 (*.*)|*.*|", "json", "")
	if !ok {
		return
	}
	if err := app.store.ImportHolidayFile(p); err != nil {
		msgBox(calendarWnd, "导入失败", err.Error(), MB_OK|MB_ICONERROR)
	} else {
		msgBox(calendarWnd, "导入成功", "节假日配置已更新，本地立即生效。", MB_OK|MB_ICONINFORMATION)
		invalidate(calendarWnd)
		invalidate(app.main)
	}
}
func exportHolidayUI() {
	year := app.calendarMonth.Year() + 1
	name := fmt.Sprintf("WorkMate-Holiday-%d-Template.json", year)
	p, ok := chooseFile(calendarWnd, true, "导出节假日模板", "JSON 配置 (*.json)|*.json|所有文件 (*.*)|*.*|", "json", name)
	if !ok {
		return
	}
	if err := ExportHolidayTemplate(p, year); err != nil {
		msgBox(calendarWnd, "导出失败", err.Error(), MB_OK|MB_ICONERROR)
	} else {
		msgBox(calendarWnd, "导出成功", fmt.Sprintf("已生成 %d 年模板。", year), MB_OK|MB_ICONINFORMATION)
	}
}

func openDayEdit(d time.Time) {
	if dayEditWnd != 0 {
		pDestroyWindow.Call(uintptr(dayEditWnd))
	}
	dayEditDate = d
	dayEditWnd = createOwnedWindow(dayEditClass, "记录 "+d.Format("2006-01-02"), 470, 434, app.main)
	show(dayEditWnd, SW_SHOW)
}
func dayEditWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		dayEditFont = newUIFont(14)
		ctl := func(class, text string, style uintptr, x, y, w, h, id int) HWND {
			child := createCtl(hwnd, class, text, style, x, y, w, h, id)
			pSendMessage.Call(uintptr(child), WM_SETFONT, uintptr(dayEditFont), 1)
			return child
		}
		info := app.store.DayInfo(dayEditDate)
		ctl("STATIC", "日期性质："+info.Name, 0, 22, 18, 420, 26, 300)
		entry := app.store.DayEntryAt(dayEditDate)
		rows := []struct {
			label string
			id    int
			value string
		}{
			{"加班（小时）", 301, ledgerNumber(entry.OvertimeHours)},
			{"年假（天，Pro）", 302, ledgerNumber(entry.AnnualDays)},
			{"补休（小时，Pro）", 303, ledgerNumber(entry.CompHours)},
			{"请假（天，Pro）", 304, ledgerNumber(entry.LeaveDays)},
			{"备注（可选）", 305, entry.Note},
		}
		for i, row := range rows {
			y := 58 + i*44
			ctl("STATIC", row.label, 0, 22, y, 128, 26, 0)
			edit := ctl("EDIT", row.value, WS_BORDER|WS_TABSTOP|ES_AUTOHSCROLL, 158, y-2, 284, 28, row.id)
			if row.id >= 302 && row.id <= 304 && !canUseFeature(app.license, featureLeave, time.Now()) {
				pEnableWindow.Call(uintptr(edit), 0)
			}
		}
		ctl("STATIC", "加班补记与备注免费；已有休假记录会保留。", 0, 22, 286, 420, 28, 306)
		ctl("BUTTON", "了解休假管理 Pro", BS_PUSHBUTTON|WS_TABSTOP, 22, 326, 188, 34, 392)
		ctl("BUTTON", "保存记录", BS_PUSHBUTTON|WS_TABSTOP, 232, 376, 100, 36, 390)
		ctl("BUTTON", "取消", BS_PUSHBUTTON|WS_TABSTOP, 346, 376, 96, 36, 391)
		return 0
	case WM_COMMAND:
		switch loword(wParam) {
		case 392:
			openPurchaseWindowFor(featureLeave)
		case 390:
			saveDayEdit(hwnd)
		case 391:
			pDestroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		if dayEditFont != 0 {
			pDeleteObject.Call(uintptr(dayEditFont))
			dayEditFont = 0
		}
		dayEditWnd = 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}
func saveDayEdit(hwnd HWND) {
	values := make([]float64, 4)
	for i, id := range []int{301, 302, 303, 304} {
		v, err := strconv.ParseFloat(strings.TrimSpace(getText(getDlgItem(hwnd, id))), 64)
		if err != nil {
			msgBox(hwnd, "输入有误", "加班与休假请输入有效数字，例如 1.5。", MB_OK|MB_ICONWARNING)
			return
		}
		values[i] = v
	}
	entry := DayEntry{OvertimeHours: values[0], AnnualDays: values[1], CompHours: values[2], LeaveDays: values[3], Note: getText(getDlgItem(hwnd, 305))}
	err := app.store.UpdateDayEntry(dayEditDate, entry, canUseFeature(app.license, featureLeave, time.Now()), time.Now())
	if err == errLeaveRequiresPro {
		openPurchaseWindowFor(featureLeave)
		return
	}
	if err != nil {
		msgBox(hwnd, "记录未保存", err.Error(), MB_OK|MB_ICONWARNING)
		return
	}
	pDestroyWindow.Call(uintptr(hwnd))
	if calendarWnd != 0 {
		invalidate(calendarWnd)
	}
	if ledgerWnd != 0 {
		refreshLedgerWindow(ledgerWnd)
	}
	invalidate(app.main)
}

func openTimelineWindow() {
	if timelineWnd != 0 {
		show(timelineWnd, SW_RESTORE)
		pSetForegroundWindow.Call(uintptr(timelineWnd))
		return
	}
	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU)
	r, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(u16(timelineClass))), uintptr(unsafe.Pointer(u16("今日工作时间轴"))), style, 500, 200, 620, 560, uintptr(app.main), 0, uintptr(app.hInst), 0)
	timelineWnd = HWND(r)
	show(timelineWnd, SW_SHOW)
}
func timelineWndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		paintTimeline(hwnd)
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		timelineWnd = 0
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}
func paintTimeline(hwnd HWND) {
	var ps PAINTSTRUCT
	hdc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	p := currentPalette()
	var cr RECT
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&cr)))
	fill(HDC(hdc), cr, p.Bg)
	drawText(HDC(hdc), "今天的工作时间轴", RECT{30, 22, 560, 60}, 24, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(HDC(hdc), time.Now().Format("2006-01-02")+" · 系统计划 + 实际加班记录", RECT{32, 60, 560, 90}, 12, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	events := app.store.BuildAutoTimeline(time.Now())
	y := 110
	for i, e := range events {
		if y > 485 {
			break
		}
		dot := p.Accent
		if strings.Contains(e.Kind, "overtime") {
			dot = p.Money
		}
		roundBox(HDC(hdc), RECT{36, int32(y + 7), 48, int32(y + 19)}, dot, dot, 12)
		drawText(HDC(hdc), e.Time, RECT{62, int32(y), 145, int32(y + 28)}, 13, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(HDC(hdc), e.Text, RECT{152, int32(y), 540, int32(y + 28)}, 13, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		if i < len(events)-1 {
			fill(HDC(hdc), RECT{41, int32(y + 21), 43, int32(y + 43)}, p.Border)
		}
		y += 46
	}
}
