package main

import "time"

type DayInfo struct {
	Date               time.Time
	Type               string
	Name               string
	IsWorkday          bool
	IsRestDay          bool
	OvertimeMultiplier float64
	CanCompensate      bool
}

var adjustedWorkdays = map[string]bool{
	"2025-01-26": true, "2025-02-08": true, "2025-04-27": true, "2025-09-28": true, "2025-10-11": true,
	"2026-01-04": true, "2026-02-14": true, "2026-02-28": true, "2026-05-09": true, "2026-09-20": true, "2026-10-10": true,
}
var statutory = map[string]string{
	"2025-01-01": "元旦", "2025-01-28": "春节·除夕", "2025-01-29": "春节·正月初一", "2025-01-30": "春节·正月初二", "2025-01-31": "春节·正月初三",
	"2025-04-04": "清明节", "2025-05-01": "劳动节", "2025-05-02": "劳动节", "2025-05-31": "端午节", "2025-10-01": "国庆节", "2025-10-02": "国庆节", "2025-10-03": "国庆节", "2025-10-06": "中秋节",
	"2026-01-01": "元旦", "2026-02-16": "春节·除夕", "2026-02-17": "春节·正月初一", "2026-02-18": "春节·正月初二", "2026-02-19": "春节·正月初三",
	"2026-04-05": "清明节", "2026-05-01": "劳动节", "2026-05-02": "劳动节", "2026-06-19": "端午节", "2026-09-25": "中秋节", "2026-10-01": "国庆节", "2026-10-02": "国庆节", "2026-10-03": "国庆节",
}
var holidaySpans = []struct{ Start, End, Name string }{
	{"2025-01-01", "2025-01-01", "元旦假期"}, {"2025-01-28", "2025-02-04", "春节假期"}, {"2025-04-04", "2025-04-06", "清明节假期"}, {"2025-05-01", "2025-05-05", "劳动节假期"}, {"2025-05-31", "2025-06-02", "端午节假期"}, {"2025-10-01", "2025-10-08", "国庆/中秋假期"},
	{"2026-01-01", "2026-01-03", "元旦假期"}, {"2026-02-15", "2026-02-23", "春节假期"}, {"2026-04-04", "2026-04-06", "清明节假期"}, {"2026-05-01", "2026-05-05", "劳动节假期"}, {"2026-06-19", "2026-06-21", "端午节假期"}, {"2026-09-25", "2026-09-27", "中秋节假期"}, {"2026-10-01", "2026-10-07", "国庆节假期"},
}

func (s *Store) customYear(year int) *HolidayYearConfig {
	for i := range s.HolidayYears {
		if s.HolidayYears[i].Year == year {
			return &s.HolidayYears[i]
		}
	}
	return nil
}

func (s *Store) DayInfo(date time.Time) DayInfo {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	key := d.Format("2006-01-02")
	if s.Settings.UseMainlandHolidayCalendar {
		if cfg := s.customYear(d.Year()); cfg != nil {
			for _, x := range cfg.AdjustedWorkdays {
				if x == key {
					return DayInfo{Date: d, Type: "AdjustedWorkday", Name: "调休工作日", IsWorkday: true, OvertimeMultiplier: 1.5}
				}
			}
			for _, x := range cfg.StatutoryHolidays {
				if x.Date == key {
					return DayInfo{Date: d, Type: "StatutoryHoliday", Name: x.Name, IsRestDay: true, OvertimeMultiplier: 3}
				}
			}
			for _, sp := range cfg.HolidaySpans {
				st, _ := time.ParseInLocation("2006-01-02", sp.Start, d.Location())
				en, _ := time.ParseInLocation("2006-01-02", sp.End, d.Location())
				if !d.Before(st) && !d.After(en) {
					return DayInfo{Date: d, Type: "HolidayRestDay", Name: sp.Name, IsRestDay: true, OvertimeMultiplier: 2, CanCompensate: true}
				}
			}
		}
		if adjustedWorkdays[key] {
			return DayInfo{Date: d, Type: "AdjustedWorkday", Name: "调休工作日", IsWorkday: true, OvertimeMultiplier: 1.5}
		}
		if n, ok := statutory[key]; ok {
			return DayInfo{Date: d, Type: "StatutoryHoliday", Name: n, IsRestDay: true, OvertimeMultiplier: 3}
		}
		for _, sp := range holidaySpans {
			st, _ := time.ParseInLocation("2006-01-02", sp.Start, d.Location())
			en, _ := time.ParseInLocation("2006-01-02", sp.End, d.Location())
			if !d.Before(st) && !d.After(en) {
				return DayInfo{Date: d, Type: "HolidayRestDay", Name: sp.Name, IsRestDay: true, OvertimeMultiplier: 2, CanCompensate: true}
			}
		}
	}
	switch d.Weekday() {
	case time.Saturday:
		if s.Settings.WorkOnSaturday {
			return DayInfo{Date: d, Type: "NormalWorkday", Name: "周六工作日", IsWorkday: true, OvertimeMultiplier: 1.5}
		}
		return DayInfo{Date: d, Type: "RestDay", Name: "休息日", IsRestDay: true, OvertimeMultiplier: 2, CanCompensate: true}
	case time.Sunday:
		if s.Settings.WorkOnSunday {
			return DayInfo{Date: d, Type: "NormalWorkday", Name: "周日工作日", IsWorkday: true, OvertimeMultiplier: 1.5}
		}
		return DayInfo{Date: d, Type: "RestDay", Name: "休息日", IsRestDay: true, OvertimeMultiplier: 2, CanCompensate: true}
	default:
		return DayInfo{Date: d, Type: "NormalWorkday", Name: "普通工作日", IsWorkday: true, OvertimeMultiplier: 1.5}
	}
}

func (s *Store) NextRestDay(from time.Time) time.Time {
	for i := 0; i < 370; i++ {
		d := from.AddDate(0, 0, i)
		if s.DayInfo(d).IsRestDay {
			return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		}
	}
	return from
}
func (s *Store) NextWorkDay(from time.Time) time.Time {
	for i := 1; i < 370; i++ {
		d := from.AddDate(0, 0, i)
		if s.DayInfo(d).IsWorkday {
			return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		}
	}
	return from
}
func (s *Store) HolidayStartName(date time.Time) string {
	if cfg := s.customYear(date.Year()); cfg != nil {
		key := date.Format("2006-01-02")
		for _, sp := range cfg.HolidaySpans {
			if sp.Start == key {
				return sp.Name
			}
		}
	}
	for _, sp := range holidaySpans {
		if sp.Start == date.Format("2006-01-02") {
			return sp.Name
		}
	}
	return ""
}
