package main

import (
	"errors"
	"math"
	"strings"
	"time"
)

func validateSettings(s Settings, now time.Time) error {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(s.MonthlySalary) || s.MonthlySalary <= 0 {
		return errors.New("月薪必须是大于 0 的有效数值。")
	}
	if !finite(s.OvertimeBaseSalary) || s.OvertimeBaseSalary < 0 {
		return errors.New("加班工资基数必须是大于或等于 0 的有效数值。")
	}
	if !finite(s.MonthlyWorkDays) || s.MonthlyWorkDays <= 0 || s.MonthlyWorkDays > 31 {
		return errors.New("月计薪天数必须大于 0 且不超过 31。")
	}
	ws, e1 := hmToMinutes(s.WorkStart)
	we, e2 := hmToMinutes(s.WorkEnd)
	ls, e3 := hmToMinutes(s.LunchStart)
	le, e4 := hmToMinutes(s.LunchEnd)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return errors.New("上下班及午休时间请使用 HH:MM 格式。")
	}
	if we <= ws {
		return errors.New("下班时间必须晚于上班时间；暂不支持跨午夜班次。")
	}
	if ls < ws || le > we || ls > le {
		return errors.New("午休必须位于工作时段内，结束时间不能早于开始时间；不午休可填写相同时间。")
	}
	if s.PaydayDay < 1 || s.PaydayDay > 31 {
		return errors.New("发薪日请输入 1-31。")
	}
	if strings.TrimSpace(s.CumulativeWorkStartDate) != "" {
		d, err := parseAnyDate(s.CumulativeWorkStartDate)
		if err != nil || d.Format("2006-01-02") > now.Format("2006-01-02") {
			return errors.New("参加工作日期请输入有效的过去或当天日期，例如 2018-06-01。")
		}
	}
	if s.ReminderEveningHour < 0 || s.ReminderEveningHour > 23 || s.ReminderEveningMinute < 0 || s.ReminderEveningMinute > 59 {
		return errors.New("晚间提醒时间请使用 HH:MM 格式。")
	}
	if !finite(s.FloatingOpacity) || s.FloatingOpacity < 0.4 || s.FloatingOpacity > 1 {
		return errors.New("挂件不透明度请输入 40-100%。")
	}
	if s.FloatingSize != "Small" && s.FloatingSize != "Medium" && s.FloatingSize != "Large" {
		return errors.New("请选择挂件尺寸。")
	}
	if s.FloatingPalette != "Lavender" && s.FloatingPalette != "Peach" && s.FloatingPalette != "Mint" && s.FloatingPalette != "Pink" && s.FloatingPalette != "Sky" {
		return errors.New("请选择挂件配色。")
	}
	return nil
}

// Persist first, then publish the new settings to the running UI.
func (s *Store) UpdateSettings(next Settings, now time.Time) error {
	if err := validateSettings(next, now); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeJSONAtomic(s.SettingsPath, next); err != nil {
		return err
	}
	s.Settings = next
	return nil
}
