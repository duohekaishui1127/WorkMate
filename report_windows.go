//go:build windows

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

func saveShareCard(kind string) {
	now := time.Now()
	name := "WorkMate-" + now.Format("20060102") + ".png"
	title := "保存今日打工卡"
	if kind == "month" {
		name = fmt.Sprintf("WorkMate-%d-%02d-月报.png", now.Year(), now.Month())
		title = "保存月度打工报告"
	} else if kind == "year" {
		name = fmt.Sprintf("WorkMate-%d-年度打工报告.png", now.Year())
		title = "保存年度打工报告"
	}
	initial := filepath.Join(os.Getenv("USERPROFILE"), "Desktop", name)
	path, ok := chooseFile(app.main, true, title, "PNG 图片 (*.png)|*.png|所有文件 (*.*)|*.*|", "png", initial)
	if !ok {
		return
	}
	if err := renderShareCard(path, kind, now); err != nil {
		msgBox(app.main, "生成失败", err.Error(), MB_OK|MB_ICONERROR)
	} else {
		msgBox(app.main, "生成完成", "图片已保存到：\n"+path, MB_OK|MB_ICONINFORMATION)
	}
}

func renderShareCard(path, kind string, now time.Time) error {
	w, h := 1080, 1380
	if kind == "month" {
		h = 1780
	} else if kind == "year" {
		h = 2200
	}
	mem, _, _ := pCreateCompatibleDC.Call(0)
	if mem == 0 {
		return fmt.Errorf("无法创建绘图缓冲")
	}
	defer pDeleteDC.Call(mem)
	bmi := BITMAPINFO{Header: BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var bits unsafe.Pointer
	hb, _, _ := pCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hb == 0 || bits == nil {
		return fmt.Errorf("无法创建图片画布")
	}
	old, _, _ := pSelectObject.Call(mem, hb)
	defer func() { pSelectObject.Call(mem, old); pDeleteObject.Call(hb) }()
	hdc := HDC(mem)
	p := currentPalette()
	fill(hdc, RECT{0, 0, int32(w), int32(h)}, p.Bg)
	// report frame
	roundBox(hdc, RECT{50, 50, int32(w - 50), int32(h - 50)}, p.Surface, p.Border, 32)
	drawText(hdc, "打工搭子  WorkMate", RECT{95, 85, 760, 135}, 22, FW_SEMIBOLD, p.Accent, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	switch kind {
	case "daily":
		drawDailyReport(hdc, now, p, w, h)
	case "month":
		drawMonthReport(hdc, now, p, w, h)
	default:
		drawYearReport(hdc, now, p, w, h)
	}
	// copy BGRA DIB memory into RGBA image
	raw := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			b, g, r, a := raw[i], raw[i+1], raw[i+2], raw[i+3]
			if a == 0 {
				a = 255
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func drawDailyReport(hdc HDC, now time.Time, p palette, w, h int) {
	info := app.store.DayInfo(now)
	earned, progress := app.store.TodayEarned(now)
	drawText(hdc, "今天的打工小票", RECT{95, 160, 950, 225}, 42, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, now.Format("2006-01-02")+" · "+info.Name, RECT{98, 230, 950, 270}, 18, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	bg := p.Lavender
	if info.IsRestDay {
		bg = p.Mint
	}
	roundBox(hdc, RECT{95, 315, 985, 620}, bg, p.Border, 28)
	if info.IsRestDay {
		drawText(hdc, "今天不用打工", RECT{140, 350, 900, 430}, 44, FW_BOLD, p.Success, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, "请珍惜这短暂的自由。", RECT{142, 440, 900, 490}, 22, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		next := app.store.NextWorkDay(now)
		drawText(hdc, "下次上班  "+formatDateShort(next), RECT{142, 520, 900, 565}, 22, FW_SEMIBOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	} else {
		drawText(hdc, fmt.Sprintf("¥ %.2f", earned), RECT{140, 350, 900, 445}, 52, FW_BOLD, p.Money, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		a, b := workCountdown(now)
		drawText(hdc, a+"  "+b, RECT{142, 455, 900, 505}, 22, FW_SEMIBOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, fmt.Sprintf("今日打工进度  %d%%", int(progress*100)), RECT{142, 525, 900, 570}, 20, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	}
	sm := app.store.MonthSummary(now.Year(), now.Month())
	reportStatRow(hdc, 95, 690, 985, "本月加班", fmt.Sprintf("%.1f 小时", sm.OvertimeMinutes/60), p.Peach, p)
	reportStatRow(hdc, 95, 820, 985, "本月工作日", fmt.Sprintf("%d 天", sm.WorkDays), p.Sky, p)
	reportStatRow(hdc, 95, 950, 985, "下一个休息日", formatDateShort(app.store.NextRestDay(now)), p.Mint, p)
	drawText(hdc, "今天的任务完成标准：活到下班。", RECT{95, 1130, 985, 1200}, 24, FW_BOLD, p.Text, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "所有数据仅保存在本机 · WorkMate V"+appVersion, RECT{95, 1250, 985, 1300}, 14, FW_NORMAL, p.Sub, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}
func reportStatRow(hdc HDC, x1, y, x2 int32, label, val string, bg uint32, p palette) {
	roundBox(hdc, RECT{x1, y, x2, y + 105}, bg, p.Border, 22)
	drawText(hdc, label, RECT{x1 + 30, y + 15, x2 - 30, y + 50}, 17, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, val, RECT{x1 + 30, y + 48, x2 - 30, y + 90}, 27, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}

func drawMonthReport(hdc HDC, now time.Time, p palette, w, h int) {
	sm := app.store.MonthSummary(now.Year(), now.Month())
	drawText(hdc, fmt.Sprintf("%d 年 %d 月打工报告", now.Year(), now.Month()), RECT{95, 160, 950, 230}, 42, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "这个月你又稳定地把时间换成了工资。", RECT{98, 235, 950, 280}, 19, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	reportBigGrid(hdc, sm, p, 330)
	reportStatRow(hdc, 95, 930, 985, "参考加班工资", fmt.Sprintf("¥ %.2f", sm.ReferenceOvertimePay), p.Peach, p)
	reportStatRow(hdc, 95, 1060, 985, "年假 / 补休", fmt.Sprintf("%.1f 天 / %.1f 小时", sm.AnnualDays, sm.CompHours), p.Mint, p)
	reportStatRow(hdc, 95, 1190, 985, "调休工作日", fmt.Sprintf("%d 天", sm.AdjustedWorkdays), p.Pink, p)
	phrase := "原来不是没努力，是月份真的很长。"
	if sm.OvertimeMinutes > 20*60 {
		phrase = "这个月加班有点多，工位已经快认识你了。"
	} else if sm.OvertimeMinutes < 5*60 {
		phrase = "恭喜，这个月老板少拥有了你一点。"
	}
	drawText(hdc, phrase, RECT{100, 1390, 980, 1470}, 26, FW_BOLD, p.Text, DT_CENTER|DT_VCENTER|DT_WORDBREAK)
	drawText(hdc, "WorkMate · 本地离线生成", RECT{100, 1620, 980, 1670}, 14, FW_NORMAL, p.Sub, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func drawYearReport(hdc HDC, now time.Time, p palette, w, h int) {
	sm := app.store.YearSummary(now.Year())
	drawText(hdc, fmt.Sprintf("你的 %d 打工报告", now.Year()), RECT{95, 155, 950, 230}, 44, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "这一年，你把很多清醒时间交给了工作，也顺利把工资领到了手。", RECT{98, 238, 950, 300}, 20, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_WORDBREAK)
	reportBigGrid(hdc, sm, p, 340)
	y := 940
	reportStatRow(hdc, 95, int32(y), 985, "参考收入", fmt.Sprintf("¥ %.0f", sm.ReferenceIncome), p.Lavender, p)
	y += 130
	reportStatRow(hdc, 95, int32(y), 985, "参考加班工资", fmt.Sprintf("¥ %.2f", sm.ReferenceOvertimePay), p.Peach, p)
	y += 130
	busiest := "暂无"
	if sm.BusiestMonth > 0 {
		busiest = fmt.Sprintf("%d 月 · %.1f 小时", sm.BusiestMonth, sm.BusiestMonthOvertime/60)
	}
	reportStatRow(hdc, 95, int32(y), 985, "今年最忙的月份", busiest, p.Pink, p)
	y += 130
	late := sm.LatestEnd
	if late == "" {
		late = "暂无记录"
	}
	reportStatRow(hdc, 95, int32(y), 985, "最晚一次加班结束", late, p.Sky, p)
	y += 130
	reportStatRow(hdc, 95, int32(y), 985, "最长连续工作", fmt.Sprintf("%d 天", sm.LongestStreak), p.Mint, p)
	phrase := "恭喜，你又平安打完一年工。"
	if sm.OvertimeMinutes > 120*60 {
		phrase = "这一年你加了不少班，请记得工资可以买东西，时间买不回来。"
	}
	drawText(hdc, phrase, RECT{110, 1745, 970, 1860}, 29, FW_BOLD, p.Text, DT_CENTER|DT_VCENTER|DT_WORDBREAK)
	drawText(hdc, "你今年最擅长的事情：把“再坚持一下”坚持了很多次。", RECT{110, 1880, 970, 1960}, 21, FW_NORMAL, p.Sub, DT_CENTER|DT_VCENTER|DT_WORDBREAK)
	drawText(hdc, "WorkMate · 所有记录均在本机生成", RECT{100, 2070, 980, 2120}, 14, FW_NORMAL, p.Sub, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func reportBigGrid(hdc HDC, sm Summary, p palette, top int32) {
	items := []struct {
		label, val string
		bg         uint32
	}{{"工作天数", fmt.Sprintf("%d 天", sm.WorkDays), p.Sky}, {"累计工作", fmt.Sprintf("%.1f h", sm.WorkMinutes/60), p.Lavender}, {"累计加班", fmt.Sprintf("%.1f h", sm.OvertimeMinutes/60), p.Peach}, {"休息天数", fmt.Sprintf("%d 天", sm.RestDays), p.Mint}}
	for i, it := range items {
		col, row := i%2, i/2
		x := int32(95 + col*455)
		y := top + int32(row*225)
		roundBox(hdc, RECT{x, y, x + 425, y + 195}, it.bg, p.Border, 25)
		drawText(hdc, it.label, RECT{x + 30, y + 25, x + 390, y + 65}, 17, FW_NORMAL, p.Sub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, it.val, RECT{x + 30, y + 78, x + 390, y + 145}, 37, FW_BOLD, p.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	}
}
