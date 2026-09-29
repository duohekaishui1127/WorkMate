package main

import (
	"fmt"
	"time"
)

// Background launches stay quiet; manual launches respect StartHidden.
func startupPresentation(s Settings, background bool) (hidden, floating bool) {
	hidden = background || s.StartHidden
	floating = s.ShowFloatingOnStartup
	if background {
		floating = s.ShowFloatingOnAutoStart
	}
	return
}

func floatingDimensions(s Settings) (w, h int) {
	w, h = 340, 102
	switch s.FloatingSize {
	case "Small":
		w, h = 260, 78
	case "Large":
		w, h = 420, 126
	}
	if s.FloatingCompact {
		h = 60
	}
	if s.FloatingShowPhrase {
		h += 24
	}
	return
}

type Reminder struct {
	Key, Title, Body string
}

// Reminder eligibility is independent of Windows notifications and Pro gating.
func (s *Store) DueReminders(now time.Time) []Reminder {
	if !s.Settings.ReminderEnabled {
		return nil
	}
	var out []Reminder
	add := func(key, title, body string) {
		out = append(out, Reminder{key, title, body})
	}
	if s.DayInfo(now).IsWorkday {
		we, _ := hmToMinutes(s.Settings.WorkEnd)
		target := time.Date(now.Year(), now.Month(), now.Day(), we/60, we%60, 0, 0, now.Location())
		left := target.Sub(now)
		if left > 0 && left <= 30*time.Minute {
			add("off30", "还有 30 分钟左右下班", "今天也快熬过去了，再坚持一下。")
		}
		if left > 0 && left <= 5*time.Minute {
			add("off5", "还有 5 分钟下班", "请把灵魂慢慢从工位收回来。")
		}
		if left <= 0 && left > -2*time.Minute {
			add("off", "下班时间到", "今天辛苦了。")
		}
	}
	if now.Hour()*60+now.Minute() >= s.Settings.ReminderEveningHour*60+s.Settings.ReminderEveningMinute {
		tomorrow := now.AddDate(0, 0, 1)
		if s.DayInfo(tomorrow).Type == "AdjustedWorkday" {
			add("adjusted", "明天是调休工作日", "明天可能是周末，但还是要上班。")
		}
		if name := s.HolidayStartName(tomorrow); name != "" {
			add("holiday", "明天开始 "+name, "今天可以开始期待自由了。")
		}
		if s.NextPayday(now).Format("2006-01-02") == tomorrow.Format("2006-01-02") {
			add("payday", "明天可能发工资", "钱还没到，精神先到账。")
		}
	}
	return out
}

func annualLeaveText(b AnnualLeaveBalance) string {
	if !b.Configured {
		return "设置参加工作日期后，可查看年假估算额度与余额。"
	}
	return fmt.Sprintf("年假估算：额度 %.1f 天 · 已用 %.1f 天 · 已预留 %.1f 天 · 可安排 %.1f 天", b.Entitlement, b.Used, b.Planned, b.Available)
}
