package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type LedgerRow struct {
	Date                                       time.Time
	DayName, Note, Status                      string
	WorkMinutes, OvertimeMinutes               float64
	AnnualDays, CompHours, LeaveDays, SickDays float64
}

func (r LedgerRow) HasEntry() bool {
	return r.WorkMinutes > 0 || r.OvertimeMinutes > 0 || r.AnnualDays > 0 || r.CompHours > 0 || r.LeaveDays > 0 || r.SickDays > 0 || r.Note != ""
}

type LedgerTotals struct {
	RecordedDays                                                             int
	WorkMinutes, OvertimeMinutes, AnnualDays, CompHours, LeaveDays, SickDays float64
}
type MonthlyLedger struct {
	Month, AsOf time.Time
	Rows        []LedgerRow
	Totals      LedgerTotals
}

func (l MonthlyLedger) HasEntries() bool {
	for _, row := range l.Rows {
		if row.HasEntry() {
			return true
		}
	}
	return false
}

// Future plans stay in the calendar; a historical ledger only contains elapsed days.
func (s *Store) MonthlyLedgerAt(year int, month time.Month, now time.Time) (MonthlyLedger, error) {
	if year < 1970 || year > 2100 || month < time.January || month > time.December {
		return MonthlyLedger{}, errors.New("请选择 1970–2100 年之间的有效月份。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l := MonthlyLedger{Month: time.Date(year, month, 1, 0, 0, 0, 0, now.Location()), AsOf: now}
	end := l.Month.AddDate(0, 1, 0)
	cutoff := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if end.After(cutoff) {
		end = cutoff
	}
	records := map[string]DailyRecord{}
	for _, record := range s.Records {
		records[record.Date] = record
	}
	leaves := map[string][]LeaveRecord{}
	for _, leave := range s.Leaves {
		leaves[leave.Date] = append(leaves[leave.Date], leave)
	}
	for date := l.Month; date.Before(end); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		record := records[key]
		row := LedgerRow{Date: date, DayName: s.DayInfo(date).Name, WorkMinutes: record.WorkMinutes, OvertimeMinutes: record.OvertimeMinutes, Status: "无记录"}
		for _, leave := range leaves[key] {
			row.AnnualDays += leave.AnnualDays
			row.CompHours += leave.CompHours
			row.LeaveDays += leave.LeaveDays
			row.SickDays += leave.SickDays
			if leave.Note != "" {
				if row.Note != "" {
					row.Note += " / "
				}
				row.Note += leave.Note
			}
		}
		if row.HasEntry() {
			row.Status = "已记录"
		}
		if s.overtimeStarted != nil {
			a, b := *s.overtimeStarted, now
			if a.Before(date) {
				a = date
			}
			if next := date.AddDate(0, 0, 1); b.After(next) {
				b = next
			}
			if b.After(a) {
				row.OvertimeMinutes += b.Sub(a).Minutes()
				row.Status = "包含进行中加班"
			}
		}
		if row.WorkMinutes > 0 || row.OvertimeMinutes > 0 {
			l.Totals.RecordedDays++
		}
		l.Totals.WorkMinutes += row.WorkMinutes
		l.Totals.OvertimeMinutes += row.OvertimeMinutes
		l.Totals.AnnualDays += row.AnnualDays
		l.Totals.CompHours += row.CompHours
		l.Totals.LeaveDays += row.LeaveDays
		l.Totals.SickDays += row.SickDays
		l.Rows = append(l.Rows, row)
	}
	return l, nil
}

// User notes must remain text when opened in a spreadsheet.
func spreadsheetText(value string) string {
	start := strings.TrimLeftFunc(value, func(r rune) bool { return unicode.IsSpace(r) || r == 0 || r == '\ufeff' })
	if strings.HasPrefix(start, "=") || strings.HasPrefix(start, "+") || strings.HasPrefix(start, "-") || strings.HasPrefix(start, "@") || strings.HasPrefix(start, "＝") || strings.HasPrefix(start, "＋") || strings.HasPrefix(start, "－") || strings.HasPrefix(start, "＠") {
		return "\t" + start
	}
	return value
}
func ledgerNumber(value float64) string { return fmt.Sprintf("%.2f", value) }
func weekdayText(date time.Time) string {
	return []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[date.Weekday()]
}

func writeLedgerCSV(w io.Writer, l MonthlyLedger) error {
	// Excel recognizes Chinese UTF-8 CSV directly when the file includes a BOM.
	if _, err := io.WriteString(w, "\xef\xbb\xbf"); err != nil {
		return err
	}
	csvWriter := csv.NewWriter(w)
	csvWriter.UseCRLF = true
	if err := csvWriter.Write([]string{"日期", "星期", "日期性质", "记录工时（小时）", "加班（小时）", "年假（天）", "补休（小时）", "请假（天）", "病假（天）", "记录状态", "备注", "统计截止"}); err != nil {
		return err
	}
	for _, r := range l.Rows {
		if err := csvWriter.Write([]string{r.Date.Format("2006-01-02"), weekdayText(r.Date), spreadsheetText(r.DayName), ledgerNumber(r.WorkMinutes / 60), ledgerNumber(r.OvertimeMinutes / 60), ledgerNumber(r.AnnualDays), ledgerNumber(r.CompHours), ledgerNumber(r.LeaveDays), ledgerNumber(r.SickDays), r.Status, spreadsheetText(r.Note), l.AsOf.Format("2006-01-02 15:04")}); err != nil {
			return err
		}
	}
	t := l.Totals
	if err := csvWriter.Write([]string{"合计", "", "已发生记录", ledgerNumber(t.WorkMinutes / 60), ledgerNumber(t.OvertimeMinutes / 60), ledgerNumber(t.AnnualDays), ledgerNumber(t.CompHours), ledgerNumber(t.LeaveDays), ledgerNumber(t.SickDays), fmt.Sprintf("有工时记录 %d 天", t.RecordedDays), "工时来自本机运行和手动记录，不是考勤证明；未来休假计划未计入。", l.AsOf.Format("2006-01-02 15:04")}); err != nil {
		return err
	}
	csvWriter.Flush()
	return csvWriter.Error()
}
func (s *Store) ExportMonthlyLedger(path string, year int, month time.Month, now time.Time) error {
	l, err := s.MonthlyLedgerAt(year, month, now)
	if err != nil {
		return err
	}
	if !l.HasEntries() {
		return errors.New("这个月还没有记录，可先开始加班计时，或在日历补记。")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".workmate-ledger-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = writeLedgerCSV(f, l); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
