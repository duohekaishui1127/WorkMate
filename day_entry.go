package main

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var errLeaveRequiresPro = errors.New("休假管理需要 Pro")

type DayEntry struct {
	OvertimeHours, AnnualDays, CompHours, LeaveDays float64
	Note                                            string
}

func (s *Store) DayEntryAt(date time.Time) DayEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dayEntryLocked(date)
}
func (s *Store) dayEntryLocked(date time.Time) DayEntry {
	key := date.Format("2006-01-02")
	var entry DayEntry
	for _, record := range s.Records {
		if record.Date == key {
			entry.OvertimeHours = record.OvertimeMinutes / 60
		}
	}
	for _, leave := range s.Leaves {
		if leave.Date == key {
			entry.AnnualDays, entry.CompHours, entry.LeaveDays, entry.Note = leave.AnnualDays, leave.CompHours, leave.LeaveDays, leave.Note
		}
	}
	return entry
}
func (s *Store) UpdateDayEntry(date time.Time, entry DayEntry, allowLeave bool, now time.Time) error {
	for _, v := range []float64{entry.OvertimeHours, entry.AnnualDays, entry.CompHours, entry.LeaveDays} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return errors.New("请输入大于或等于 0 的有效数字。")
		}
	}
	if entry.OvertimeHours > 24 || entry.CompHours > 24 {
		return errors.New("一天的加班或补休不能超过 24 小时。")
	}
	entry.Note = strings.TrimSpace(entry.Note)
	if len([]rune(entry.Note)) > 500 {
		return errors.New("备注请控制在 500 字以内。")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.dayEntryLocked(date)
	if !allowLeave && (entry.AnnualDays != old.AnnualDays || entry.CompHours != old.CompHours || entry.LeaveDays != old.LeaveDays) {
		return errLeaveRequiresPro
	}
	if date.Format("2006-01-02") > now.Format("2006-01-02") && entry.OvertimeHours != old.OvertimeHours {
		return errors.New("加班只能记录已发生的时长；未来休假可以在 Pro 中提前安排。")
	}
	if s.overtimeStarted != nil && entry.OvertimeHours != old.OvertimeHours {
		startDate := s.overtimeStarted.In(date.Location()).Format("2006-01-02")
		endDate := now.In(date.Location()).Format("2006-01-02")
		key := date.Format("2006-01-02")
		if key >= startDate && key <= endDate {
			return errors.New("这一天仍有加班计时，请先结束计时再修正时长，避免重复记录。")
		}
	}
	oldRecords := append([]DailyRecord(nil), s.Records...)
	oldLeaves := append([]LeaveRecord(nil), s.Leaves...)
	s.recordFor(date).OvertimeMinutes = entry.OvertimeHours * 60
	leave := s.leaveFor(date)
	leave.AnnualDays, leave.CompHours, leave.LeaveDays, leave.Note = entry.AnnualDays, entry.CompHours, entry.LeaveDays, entry.Note
	// Persist both files and restore the earlier file if the second write fails.
	if err := writeJSONAtomic(s.LeavesPath, s.Leaves); err != nil {
		s.Records, s.Leaves = oldRecords, oldLeaves
		return err
	}
	if err := writeJSONAtomic(s.RecordsPath, s.Records); err != nil {
		s.Records, s.Leaves = oldRecords, oldLeaves
		if rollbackErr := writeJSONAtomic(s.LeavesPath, oldLeaves); rollbackErr != nil {
			return fmt.Errorf("记录未保存，恢复休假文件也失败：%v；原错误：%w", rollbackErr, err)
		}
		return err
	}
	return nil
}
