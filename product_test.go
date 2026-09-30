package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFreeKeepsUsefulDailyToolsAndProGatesPaidWorkflows(t *testing.T) {
	now := testDate("2026-09-30 12:00")
	free := &LicenseManager{trialStart: now.Add(-25 * time.Hour), trialLastSeen: now}
	trial := &LicenseManager{trialStart: now.Add(-time.Hour), trialLastSeen: now}
	paid := &LicenseManager{pro: true}
	seen := map[featureID]bool{}
	for _, f := range featureCatalog {
		if f.Name == "" || f.Benefit == "" || seen[f.ID] {
			t.Fatalf("feature catalog entry is incomplete or repeated: %+v", f)
		}
		seen[f.ID] = true
		if canUseFeature(free, f.ID, now) == f.Pro {
			t.Errorf("free access mismatch for %s", f.ID)
		}
		if !canUseFeature(trial, f.ID, now) || !canUseFeature(paid, f.ID, now) {
			t.Errorf("trial or paid access mismatch for %s", f.ID)
		}
	}
	if canUseFeature(free, "unknown", now) {
		t.Fatal("unknown capability accepted")
	}
	for _, key := range []string{"off30", "off5", "off"} {
		if reminderFeature(key) != featureOffReminder {
			t.Fatal("basic reminder incorrectly gated")
		}
	}
	for _, key := range []string{"holiday", "adjusted", "payday"} {
		if reminderFeature(key) != featureAdvancedReminders {
			t.Fatal("advance reminder incorrectly free")
		}
	}
}

func TestFreeCanCorrectOvertimeWithoutLosingPaidLeaveData(t *testing.T) {
	s := testStore(t)
	d := testDate("2026-09-28 12:00")
	s.Records = []DailyRecord{{Date: "2026-09-28", WorkMinutes: 180, OvertimeMinutes: 90, FirstSeen: "09:00:00"}}
	s.Leaves = []LeaveRecord{{Date: "2026-09-28", AnnualDays: 1, CompHours: 4, SickDays: 0.5, Note: "原备注"}}
	entry := s.DayEntryAt(d)
	entry.OvertimeHours = 2
	entry.Note = "修正加班时长"
	if err := s.UpdateDayEntry(d, entry, false, d.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if s.Records[0].WorkMinutes != 180 || s.Records[0].FirstSeen != "09:00:00" || s.Records[0].OvertimeMinutes != 120 {
		t.Fatal("time correction changed unrelated work history")
	}
	if leave := s.Leaves[0]; leave.AnnualDays != 1 || leave.CompHours != 4 || leave.SickDays != 0.5 || leave.Note != "修正加班时长" {
		t.Fatalf("free edit lost paid leave: %+v", leave)
	}
	entry.AnnualDays = 2
	if err := s.UpdateDayEntry(d, entry, false, d.Add(3*time.Hour)); !errors.Is(err, errLeaveRequiresPro) {
		t.Fatalf("free leave edit was accepted: %v", err)
	}
	if s.Leaves[0].AnnualDays != 1 {
		t.Fatal("rejected leave edit mutated data")
	}
	entry.AnnualDays = 1
	entry.OvertimeHours = math.NaN()
	if err := s.UpdateDayEntry(d, entry, false, d.Add(3*time.Hour)); err == nil {
		t.Fatal("NaN hours were saved")
	}
	reloaded, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Records[0].OvertimeMinutes != 120 || reloaded.Leaves[0].AnnualDays != 1 || reloaded.Leaves[0].Note != "修正加班时长" {
		t.Fatal("free edit was not saved across restart")
	}
}

func TestMonthlyLedgerSplitsLiveOvertimeAndDoesNotInventFutureHistory(t *testing.T) {
	s := testStore(t)
	start := testDate("2026-01-31 23:30")
	now := testDate("2026-02-01 00:30")
	s.Records = []DailyRecord{{Date: "2026-01-31", WorkMinutes: 120, OvertimeMinutes: 60}, {Date: "2026-02-02", WorkMinutes: 480}}
	s.Leaves = []LeaveRecord{{Date: "2026-01-31", AnnualDays: 0.5, Note: "2026春节前"}, {Date: "2026-02-02", AnnualDays: 1, Note: "未来计划"}}
	s.StartOvertime(start)
	jan, err := s.MonthlyLedgerAt(2026, time.January, now)
	if err != nil {
		t.Fatal(err)
	}
	feb, err := s.MonthlyLedgerAt(2026, time.February, now)
	if err != nil {
		t.Fatal(err)
	}
	if jan.Totals.OvertimeMinutes != 90 || jan.Totals.WorkMinutes != 120 || jan.Totals.AnnualDays != 0.5 || jan.Rows[len(jan.Rows)-1].Status != "包含进行中加班" {
		t.Fatalf("January ledger wrong: %+v", jan.Totals)
	}
	if len(feb.Rows) != 1 || feb.Totals.OvertimeMinutes != 30 || feb.Totals.WorkMinutes != 0 || feb.Totals.AnnualDays != 0 || feb.Totals.RecordedDays != 1 {
		t.Fatalf("February includes future records or misses live time: %+v", feb.Totals)
	}
	s.StopOvertime(now)
	janAfter, _ := s.MonthlyLedgerAt(2026, time.January, now)
	febAfter, _ := s.MonthlyLedgerAt(2026, time.February, now)
	if janAfter.Totals.OvertimeMinutes != 90 || febAfter.Totals.OvertimeMinutes != 30 {
		t.Fatal("overtime doubled after stopping")
	}
	future, err := s.MonthlyLedgerAt(2026, time.March, now)
	if err != nil || len(future.Rows) != 0 || future.HasEntries() {
		t.Fatal("future month invented history", err)
	}
}

func TestLedgerCSVPreservesChineseAndTreatsSpreadsheetFormulasAsText(t *testing.T) {
	s := testStore(t)
	date := testDate("2026-09-28 17:00")
	s.Records = []DailyRecord{{Date: "2026-09-28", WorkMinutes: 480, OvertimeMinutes: 90}}
	s.Leaves = []LeaveRecord{{Date: "2026-09-28", AnnualDays: 0.5, Note: "=HYPERLINK(\"https://example.test\",\"点我\")\n中文备注"}}
	ledger, err := s.MonthlyLedgerAt(2026, time.September, date)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = writeLedgerCSV(&out, ledger); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("Excel UTF-8 marker missing")
	}
	reader := csv.NewReader(bytes.NewReader(out.Bytes()[3:]))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][0] != "日期" || rows[len(rows)-1][0] != "合计" || rows[len(rows)-1][4] != "1.50" {
		t.Fatal("CSV headings or totals lost")
	}
	found := false
	for _, row := range rows[1 : len(rows)-1] {
		if row[0] != "2026-09-28" {
			continue
		}
		found = true
		if row[4] != "1.50" || row[5] != "0.50" || !strings.HasPrefix(row[10], "\t=HYPERLINK") || !strings.Contains(row[10], "中文备注") {
			t.Fatalf("CSV row corrupted: %q", row)
		}
	}
	if !found {
		t.Fatal("recorded day absent from CSV")
	}
	path := filepath.Join(t.TempDir(), "工时账本.csv")
	if err = s.ExportMonthlyLedger(path, 2026, time.September, date); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, out.Bytes()) {
		t.Fatal("exported file differs from verified CSV", err)
	}
	if err = s.ExportMonthlyLedger(path, 2026, time.December, date); err == nil {
		t.Fatal("empty future month overwrote export")
	}
	data, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(data, out.Bytes()) {
		t.Fatal("failed export changed the old file", err)
	}
	for _, unsafe := range []string{"=1+1", " +1+1", "-1+1", "@SUM(1)", "\ufeff=1+1"} {
		if !strings.HasPrefix(spreadsheetText(unsafe), "\t") {
			t.Fatalf("formula not escaped: %q", unsafe)
		}
	}
}
