//go:build windows

package main

import (
	"fmt"
	"math"
	"time"
	"unsafe"
)

type palette struct{ Bg, Surface, Text, Sub, Accent, Money, Success, Danger, Lavender, Peach, Mint, Pink, Sky, Border uint32 }

func currentPalette() palette {
	if app.store != nil && app.store.Settings.Theme == "Dark" {
		return palette{rgb(31, 28, 38), rgb(42, 38, 51), rgb(255, 247, 251), rgb(203, 187, 198), rgb(196, 181, 253), rgb(251, 198, 109), rgb(125, 211, 174), rgb(251, 141, 154), rgb(56, 49, 78), rgb(73, 55, 54), rgb(43, 69, 62), rgb(76, 48, 63), rgb(43, 59, 75), rgb(78, 70, 88)}
	}
	return palette{rgb(255, 249, 245), rgb(255, 253, 251), rgb(74, 63, 69), rgb(135, 116, 126), rgb(167, 139, 250), rgb(235, 156, 76), rgb(103, 189, 151), rgb(239, 127, 142), rgb(241, 235, 255), rgb(255, 236, 222), rgb(228, 247, 237), rgb(255, 232, 241), rgb(232, 244, 255), rgb(238, 220, 213)}
}

func withBrush(hdc HDC, c uint32, fn func()) {
	b, _, _ := pCreateSolidBrush.Call(uintptr(c))
	old, _, _ := pSelectObject.Call(uintptr(hdc), b)
	fn()
	pSelectObject.Call(uintptr(hdc), old)
	pDeleteObject.Call(b)
}
func withPen(hdc HDC, c uint32, width int, fn func()) {
	p, _, _ := pCreatePen.Call(PS_SOLID, uintptr(width), uintptr(c))
	old, _, _ := pSelectObject.Call(uintptr(hdc), p)
	fn()
	pSelectObject.Call(uintptr(hdc), old)
	pDeleteObject.Call(p)
}
func fill(hdc HDC, r RECT, c uint32) {
	b, _, _ := pCreateSolidBrush.Call(uintptr(c))
	pFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&r)), b)
	pDeleteObject.Call(b)
}
func roundBox(hdc HDC, r RECT, bg, border uint32, rad int32) {
	withBrush(hdc, bg, func() {
		withPen(hdc, border, 1, func() {
			pRoundRect.Call(uintptr(hdc), uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(rad), uintptr(rad))
		})
	})
}
func lineBox(hdc HDC, r RECT, bg, border uint32) {
	withBrush(hdc, bg, func() {
		withPen(hdc, border, 1, func() {
			pRectangle.Call(uintptr(hdc), uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
		})
	})
}

func drawText(hdc HDC, text string, r RECT, size int, weight int, color uint32, flags uint32) {
	face := u16("Microsoft YaHei UI")
	f, _, _ := pCreateFont.Call(uintptr(^uint32(size-1)), 0, 0, 0, uintptr(weight), 0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY, 0, uintptr(unsafe.Pointer(face)))
	old, _, _ := pSelectObject.Call(uintptr(hdc), f)
	pSetBkMode.Call(uintptr(hdc), TRANSPARENT)
	pSetTextColor.Call(uintptr(hdc), uintptr(color))
	u := syscallStringToUTF16(text)
	if len(u) > 0 {
		pDrawText.Call(uintptr(hdc), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), uintptr(flags))
	}
	pSelectObject.Call(uintptr(hdc), old)
	pDeleteObject.Call(f)
}
func syscallStringToUTF16(s string) []uint16 {
	u := make([]uint16, 0, len(s)+1)
	for _, r := range s {
		if r < 0x10000 {
			u = append(u, uint16(r))
		} else {
			r -= 0x10000
			u = append(u, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		}
	}
	u = append(u, 0)
	return u
}

func addHit(id string, r RECT) { app.hits = append(app.hits, hitRect{id, r}) }
func button(hdc HDC, id, label string, r RECT, bg, fg, border uint32) {
	roundBox(hdc, r, bg, border, 14)
	drawText(hdc, label, r, 14, FW_SEMIBOLD, fg, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	addHit(id, r)
}

func paintMain(hwnd HWND) {
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
	app.hits = app.hits[:0]
	now := time.Now()
	drawText(HDC(hdc), "打工搭子  ♡", RECT{42, 24, 300, 66}, 29, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(HDC(hdc), "今天的任务：平安活到下班。", RECT{44, 64, 430, 96}, 15, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	button(HDC(hdc), "panic", "老板来了", RECT{760, 34, 866, 78}, p.Surface, p.Text, p.Border)
	themeLabel := "☾ 深色"
	if app.store.Settings.Theme == "Dark" {
		themeLabel = "☀ 浅色"
	}
	button(HDC(hdc), "theme", themeLabel, RECT{878, 34, 978, 78}, p.Surface, p.Text, p.Border)
	button(HDC(hdc), "settings", "设置", RECT{990, 34, 1072, 78}, p.Lavender, p.Text, p.Border)
	if app.license != nil {
		status := app.license.StatusText(now)
		drawText(HDC(hdc), status, RECT{480, 34, 650, 60}, 12, FW_SEMIBOLD, p.Accent, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		if !app.license.IsPro() {
			button(HDC(hdc), "buypro", "买断 PRO", RECT{652, 34, 748, 78}, p.Peach, p.Text, p.Border)
		}
	}
	paintHero(HDC(hdc), now, p)
	paintMetrics(HDC(hdc), now, p)
	paintSummaries(HDC(hdc), now, p)
	paintActions(HDC(hdc), p)
}

func paintHero(hdc HDC, now time.Time, p palette) {
	r := RECT{42, 112, 1078, 334}
	info := app.store.DayInfo(now)
	bg := p.Lavender
	if info.IsRestDay {
		bg = p.Mint
	}
	roundBox(hdc, r, bg, p.Border, 24)
	tag := info.Name
	title := "今日收入（日程估算）"
	sub := "每一分钟都算数，至少工资别白算。"
	valueColor := p.Money
	earned, progress := app.store.TodayEarned(now)
	big := fmt.Sprintf("¥%.2f", earned)
	rightTitle, rightValue := workCountdown(now)
	if info.IsRestDay {
		title = "今天不用打工"
		sub = "请珍惜这短暂的自由。"
		big = "休息日"
		valueColor = p.Success
		next := app.store.NextWorkDay(now)
		rightTitle = "下次上班"
		rightValue = fmt.Sprintf("%d月%d日  %s", next.Month(), next.Day(), app.store.Settings.WorkStart)
		progress = 0
	}
	tom := app.store.DayInfo(now.AddDate(0, 0, 1))
	if tom.Type == "AdjustedWorkday" {
		sub = "明天虽然可能是周末，但它是调休工作日。"
	} else if n := app.store.HolidayStartName(now.AddDate(0, 0, 1)); n != "" {
		sub = "明天开始 " + n + "，今天再坚持一下。"
	}
	drawText(hdc, tag, RECT{72, 130, 390, 158}, 14, FW_SEMIBOLD, p.Accent, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, title, RECT{72, 161, 580, 202}, 26, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, big, RECT{70, 210, 600, 278}, 46, FW_BOLD, valueColor, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, sub, RECT{74, 288, 650, 318}, 14, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, rightTitle, RECT{760, 148, 1034, 176}, 14, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, rightValue, RECT{758, 178, 1038, 222}, 29, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "今日打工进度", RECT{760, 246, 930, 272}, 13, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, fmt.Sprintf("%d%%", int(math.Round(progress*100))), RECT{960, 246, 1035, 272}, 13, FW_BOLD, p.Accent, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
	bar := RECT{760, 288, 1038, 298}
	roundBox(hdc, bar, p.Surface, p.Surface, 8)
	if progress > 0 {
		w := int32(float64(bar.Right-bar.Left) * progress)
		if w < 8 {
			w = 8
		}
		roundBox(hdc, RECT{bar.Left, bar.Top, bar.Left + w, bar.Bottom}, p.Accent, p.Accent, 8)
	}
}

func workCountdown(now time.Time) (string, string) {
	info := app.store.DayInfo(now)
	if info.IsRestDay {
		return "今天", "不用打工"
	}
	ws, _ := hmToMinutes(app.store.Settings.WorkStart)
	we, _ := hmToMinutes(app.store.Settings.WorkEnd)
	ls, _ := hmToMinutes(app.store.Settings.LunchStart)
	le, _ := hmToMinutes(app.store.Settings.LunchEnd)
	cur := now.Hour()*60 + now.Minute()
	target := func(min int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), min/60, min%60, 0, 0, now.Location())
	}
	fmtDur := func(d time.Duration) string {
		if d < 0 {
			d = 0
		}
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	if cur < ws {
		return "距离上班", fmtDur(target(ws).Sub(now))
	}
	if ls < le && cur >= ls && cur < le {
		return "午休中 · 距继续打工", fmtDur(target(le).Sub(now))
	}
	if cur < we {
		return "距离下班", fmtDur(target(we).Sub(now))
	}
	return "今天", "已经下班"
}

func paintMetrics(hdc HDC, now time.Time, p palette) {
	cards := []struct {
		r        RECT
		bg       uint32
		cap, val string
		col      uint32
	}{
		{RECT{42, 350, 370, 460}, p.Peach, "发工资还有多久", fmt.Sprintf("%d 天", daysUntil(now, app.store.NextPayday(now))), p.Money},
		{RECT{385, 350, 713, 460}, p.Mint, "下一次自由", formatDateShort(app.store.NextRestDay(now)), p.Text},
		{RECT{728, 350, 1078, 460}, p.Pink, "今天什么属性", app.store.DayInfo(now).Name, p.Accent},
	}
	for _, c := range cards {
		roundBox(hdc, c.r, c.bg, p.Border, 20)
		drawText(hdc, c.cap, RECT{c.r.Left + 22, c.r.Top + 14, c.r.Right - 18, c.r.Top + 42}, 13, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, c.val, RECT{c.r.Left + 22, c.r.Top + 47, c.r.Right - 18, c.r.Top + 90}, 24, FW_BOLD, c.col, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	}
	info := app.store.DayInfo(now)
	drawText(hdc, fmt.Sprintf("加班参考 %.1f×", info.OvertimeMultiplier), RECT{750, 428, 1045, 452}, 12, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}
func daysUntil(a, b time.Time) int {
	aa := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, a.Location())
	bb := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, a.Location())
	d := int(bb.Sub(aa).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}
func formatDateShort(t time.Time) string {
	return fmt.Sprintf("%d月%d日 周%s", t.Month(), t.Day(), weekdayCN(t.Weekday()))
}
func weekdayCN(w time.Weekday) string {
	return []string{"日", "一", "二", "三", "四", "五", "六"}[int(w)]
}

func paintSummaries(hdc HDC, now time.Time, p palette) {
	wk := app.store.WeekSummary(now)
	mo := app.store.MonthSummaryAt(now.Year(), now.Month(), now)
	yr := app.store.YearSummaryAt(now.Year(), now)
	cards := []struct {
		r                        RECT
		bg                       uint32
		title, val, sub, id, btn string
	}{
		{RECT{42, 478, 370, 650}, p.Sky, "这周过得怎么样", fmt.Sprintf("%.1f h", wk.WorkMinutes/60), fmt.Sprintf("加班 %.1fh · 有记录 %d 天", wk.OvertimeMinutes/60, wk.RecordedWorkDays), "dailyshare", "今日分享卡"},
		{RECT{385, 478, 713, 650}, p.Peach, "这个月的工作记录", fmt.Sprintf("%d 天", mo.RecordedWorkDays), fmt.Sprintf("加班 %.1fh · 调休 %d 天", mo.OvertimeMinutes/60, mo.AdjustedWorkdays), "monthshare", "月度报告"},
		{RECT{728, 478, 1078, 650}, p.Lavender, "今年的工时估算收入", fmt.Sprintf("¥%.0f", yr.ReferenceIncome), fmt.Sprintf("记录 %d 天 · 加班 %.1fh", yr.RecordedWorkDays, yr.OvertimeMinutes/60), "yearshare", "年度打工报告"},
	}
	for _, c := range cards {
		roundBox(hdc, c.r, c.bg, p.Border, 20)
		drawText(hdc, c.title, RECT{c.r.Left + 20, c.r.Top + 13, c.r.Right - 15, c.r.Top + 42}, 13, FW_SEMIBOLD, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, c.val, RECT{c.r.Left + 20, c.r.Top + 49, c.r.Right - 15, c.r.Top + 91}, 27, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, c.sub, RECT{c.r.Left + 20, c.r.Top + 93, c.r.Right - 15, c.r.Top + 121}, 12, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		button(hdc, c.id, c.btn, RECT{c.r.Left + 20, c.r.Top + 128, c.r.Left + 155, c.r.Top + 164}, p.Surface, p.Text, p.Border)
	}
}

func paintActions(hdc HDC, p palette) {
	y := 674
	items := []struct {
		id, label string
		w         int
		primary   bool
	}{{"overtime", "开始加班", 118, true}, {"calendar", "劳动日历", 105, false}, {"timeline", "今日时间轴", 118, false}, {"floating", "桌面挂件", 105, false}, {"backup", "备份", 78, false}, {"restore", "恢复", 78, false}}
	if app.license != nil && !app.license.HasProAccess(time.Now()) {
		items[2].label = "时间轴 PRO"
		items[4].label = "备份 PRO"
		items[5].label = "恢复 PRO"
	}
	if app.store.OvertimeRunning() {
		items[0].label = "结束加班"
	}
	x := 42
	for _, it := range items {
		bg := p.Surface
		fg := p.Text
		if it.primary {
			bg = p.Accent
			fg = rgb(255, 255, 255)
		}
		button(hdc, it.id, it.label, RECT{int32(x), int32(y), int32(x + it.w), int32(y + 44)}, bg, fg, p.Border)
		x += it.w + 12
	}
	drawText(hdc, annualLeaveText(app.store.AnnualLeaveBalance(time.Now())), RECT{42, 764, 1078, 793}, 12, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "Ctrl + Alt + Q：老板来了 · Ctrl + Alt + G：呼出主界面", RECT{42, 732, 720, 764}, 12, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}

func checkReminders(now time.Time) {
	if app.license != nil && !app.license.HasProAccess(now) {
		return
	}
	if !app.store.Settings.ReminderEnabled {
		return
	}
	keyDate := now.Format("2006-01-02")
	mark := func(k, title, body string) {
		full := keyDate + ":" + k
		if app.reminderSeen[full] {
			return
		}
		app.reminderSeen[full] = true
		app.store.Runtime.NotificationKeys = append(app.store.Runtime.NotificationKeys, full)
		if len(app.store.Runtime.NotificationKeys) > 120 {
			app.store.Runtime.NotificationKeys = app.store.Runtime.NotificationKeys[len(app.store.Runtime.NotificationKeys)-120:]
		}
		_ = app.store.SaveAll()
		showBalloon(title, body, false)
	}
	for _, reminder := range app.store.DueReminders(now) {
		mark(reminder.Key, reminder.Title, reminder.Body)
	}
}
