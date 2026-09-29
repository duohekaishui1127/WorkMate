package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const appVersion = "0.8.0"
const appDisplayName = "打工搭子 WorkMate"

type Settings struct {
	Theme                      string  `json:"Theme"`
	MonthlySalary              float64 `json:"MonthlySalary"`
	OvertimeBaseSalary         float64 `json:"OvertimeBaseSalary"`
	MonthlyWorkDays            float64 `json:"MonthlyWorkDays"`
	WorkStart                  string  `json:"WorkStart"`
	WorkEnd                    string  `json:"WorkEnd"`
	LunchStart                 string  `json:"LunchStart"`
	LunchEnd                   string  `json:"LunchEnd"`
	PaydayDay                  int     `json:"PaydayDay"`
	WorkOnSaturday             bool    `json:"WorkOnSaturday"`
	WorkOnSunday               bool    `json:"WorkOnSunday"`
	UseMainlandHolidayCalendar bool    `json:"UseMainlandHolidayCalendar"`
	CumulativeWorkStartDate    string  `json:"CumulativeWorkStartDate"`
	AutoStart                  bool    `json:"AutoStart"`
	StartHidden                bool    `json:"StartHidden"`
	ShowFloatingOnStartup      bool    `json:"ShowFloatingOnStartup"`
	ShowFloatingOnAutoStart    bool    `json:"ShowFloatingOnAutoStart"`
	FloatingOpacity            float64 `json:"FloatingOpacity"`
	FloatingLeft               int     `json:"FloatingLeft"`
	FloatingTop                int     `json:"FloatingTop"`
	FloatingMode               string  `json:"FloatingMode"`
	FloatingAutoHide           bool    `json:"FloatingAutoHide"`
	FloatingTopmost            bool    `json:"FloatingTopmost"`
	FloatingShowEarned         bool    `json:"FloatingShowEarned"`
	FloatingShowCountdown      bool    `json:"FloatingShowCountdown"`
	FloatingShowProgress       bool    `json:"FloatingShowProgress"`
	FloatingShowPhrase         bool    `json:"FloatingShowPhrase"`
	FloatingCompact            bool    `json:"FloatingCompact"`
	FloatingPalette            string  `json:"FloatingPalette"`
	FloatingSize               string  `json:"FloatingSize"`
	ReminderEnabled            bool    `json:"ReminderEnabled"`
	ReminderEveningHour        int     `json:"ReminderEveningHour"`
	ReminderEveningMinute      int     `json:"ReminderEveningMinute"`
}

type DailyRecord struct {
	Date            string  `json:"Date"`
	WorkMinutes     float64 `json:"WorkMinutes"`
	OvertimeMinutes float64 `json:"OvertimeMinutes"`
	FirstSeen       string  `json:"FirstSeen"`
	LastSeen        string  `json:"LastSeen"`
	LastOvertimeEnd string  `json:"LastOvertimeEnd"`
}

type LeaveRecord struct {
	Date       string  `json:"Date"`
	AnnualDays float64 `json:"AnnualDays,omitempty"`
	CompHours  float64 `json:"CompHours,omitempty"`
	LeaveDays  float64 `json:"LeaveDays,omitempty"`
	SickDays   float64 `json:"SickDays,omitempty"`
	Note       string  `json:"Note,omitempty"`
}

type TimelineEvent struct {
	Date string `json:"Date"`
	Time string `json:"Time"`
	Kind string `json:"Kind"`
	Text string `json:"Text"`
}

type RuntimeState struct {
	OvertimeStartedAt       string   `json:"OvertimeStartedAt"`
	OvertimeLastHeartbeatAt string   `json:"OvertimeLastHeartbeatAt"`
	NotificationKeys        []string `json:"NotificationKeys"`
}

type HolidaySpanConfig struct {
	Start string `json:"Start"`
	End   string `json:"End"`
	Name  string `json:"Name"`
}

type StatutoryHolidayConfig struct {
	Date string `json:"Date"`
	Name string `json:"Name"`
}

type HolidayYearConfig struct {
	Year              int                      `json:"Year"`
	AdjustedWorkdays  []string                 `json:"AdjustedWorkdays"`
	StatutoryHolidays []StatutoryHolidayConfig `json:"StatutoryHolidays"`
	HolidaySpans      []HolidaySpanConfig      `json:"HolidaySpans"`
}

type Store struct {
	mu sync.Mutex

	DataDir           string
	BackupDir         string
	SettingsPath      string
	RecordsPath       string
	LeavesPath        string
	TimelinePath      string
	CustomHolidayPath string
	StatePath         string

	Settings     Settings
	Records      []DailyRecord
	Leaves       []LeaveRecord
	Timeline     []TimelineEvent
	HolidayYears []HolidayYearConfig
	Runtime      RuntimeState

	overtimeStarted *time.Time
	lastTick        time.Time
	lastPersist     time.Time
}

func defaultSettings() Settings {
	return Settings{
		Theme: "Light", MonthlySalary: 8000, OvertimeBaseSalary: 0, MonthlyWorkDays: 21.75,
		WorkStart: "09:00", WorkEnd: "18:00", LunchStart: "12:00", LunchEnd: "13:00",
		PaydayDay: 10, UseMainlandHolidayCalendar: true,
		FloatingOpacity: 0.94, FloatingLeft: -1, FloatingTop: -1, FloatingMode: "Screen",
		FloatingShowEarned: true, FloatingShowCountdown: true, FloatingShowProgress: true,
		FloatingPalette: "Lavender", FloatingSize: "Medium", FloatingTopmost: true,
		ReminderEnabled: true, ReminderEveningHour: 16, ReminderEveningMinute: 30,
	}
}

func newStore() (*Store, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		if d, err := os.UserConfigDir(); err == nil {
			appData = d
		} else {
			appData = os.TempDir()
		}
	}
	dataDir := filepath.Join(appData, "WorkMate")
	backupDir := filepath.Join(appData, "WorkMate_Backups")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(backupDir, 0755)
	s := &Store{
		DataDir: dataDir, BackupDir: backupDir,
		SettingsPath:      filepath.Join(dataDir, "settings.json"),
		RecordsPath:       filepath.Join(dataDir, "records.json"),
		LeavesPath:        filepath.Join(dataDir, "leave-records.json"),
		TimelinePath:      filepath.Join(dataDir, "timeline.json"),
		CustomHolidayPath: filepath.Join(dataDir, "holiday-calendar.json"),
		StatePath:         filepath.Join(dataDir, "state.json"),
		Settings:          defaultSettings(),
		lastTick:          time.Now(), lastPersist: time.Now(),
	}
	_ = readJSON(s.SettingsPath, &s.Settings)
	normalizeSettings(&s.Settings)
	_ = readJSON(s.RecordsPath, &s.Records)
	_ = readJSON(s.LeavesPath, &s.Leaves)
	_ = readJSON(s.TimelinePath, &s.Timeline)
	_ = readJSON(s.CustomHolidayPath, &s.HolidayYears)
	_ = readJSON(s.StatePath, &s.Runtime)
	s.normalizeLegacyData()
	s.restoreOvertimeState()
	return s, nil
}

func normalizeSettings(v *Settings) {
	d := defaultSettings()
	if v.Theme == "" {
		v.Theme = d.Theme
	}
	if v.MonthlySalary <= 0 {
		v.MonthlySalary = d.MonthlySalary
	}
	if v.MonthlyWorkDays <= 0 {
		v.MonthlyWorkDays = d.MonthlyWorkDays
	}
	if v.WorkStart == "" {
		v.WorkStart = d.WorkStart
	}
	if v.WorkEnd == "" {
		v.WorkEnd = d.WorkEnd
	}
	if v.LunchStart == "" {
		v.LunchStart = d.LunchStart
	}
	if v.LunchEnd == "" {
		v.LunchEnd = d.LunchEnd
	}
	if v.PaydayDay <= 0 || v.PaydayDay > 31 {
		v.PaydayDay = d.PaydayDay
	}
	if v.FloatingOpacity <= 0 || v.FloatingOpacity > 1 {
		v.FloatingOpacity = d.FloatingOpacity
	}
	if v.FloatingMode != "Screen" && v.FloatingMode != "Taskbar" && v.FloatingMode != "TaskbarEmbed" {
		v.FloatingMode = d.FloatingMode
	}
	if v.FloatingPalette == "" {
		v.FloatingPalette = d.FloatingPalette
	}
	if v.FloatingSize == "" {
		v.FloatingSize = d.FloatingSize
	}
	if v.ReminderEveningHour < 0 || v.ReminderEveningHour > 23 {
		v.ReminderEveningHour = d.ReminderEveningHour
	}
	if v.ReminderEveningMinute < 0 || v.ReminderEveningMinute > 59 {
		v.ReminderEveningMinute = d.ReminderEveningMinute
	}
}

func readJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	return json.Unmarshal(b, dst)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) normalizeLegacyData() {
	for i := range s.Records {
		if t, err := parseAnyDate(s.Records[i].Date); err == nil {
			s.Records[i].Date = t.Format("2006-01-02")
		}
	}
	for i := range s.Leaves {
		if t, err := parseAnyDate(s.Leaves[i].Date); err == nil {
			s.Leaves[i].Date = t.Format("2006-01-02")
		}
	}
	for i := range s.Timeline {
		if t, err := parseAnyDate(s.Timeline[i].Date); err == nil {
			s.Timeline[i].Date = t.Format("2006-01-02")
		}
	}
}

func parseAnyDate(v string) (time.Time, error) {
	layouts := []string{"2006-01-02", time.RFC3339, "2006/01/02", "01/02/2006"}
	for _, l := range layouts {
		if t, err := time.Parse(l, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid date")
}

func (s *Store) restoreOvertimeState() {
	if strings.TrimSpace(s.Runtime.OvertimeStartedAt) == "" {
		return
	}
	started, err := time.Parse(time.RFC3339Nano, s.Runtime.OvertimeStartedAt)
	if err != nil {
		return
	}
	if hb, err := time.Parse(time.RFC3339Nano, s.Runtime.OvertimeLastHeartbeatAt); err == nil {
		// Abnormal shutdown: count only until the last heartbeat when the gap is obviously a shutdown gap.
		if time.Since(hb) > 15*time.Minute {
			s.addOvertimeDuration(started, hb)
			s.Runtime.OvertimeStartedAt = ""
			s.Runtime.OvertimeLastHeartbeatAt = ""
			_ = s.saveAllLocked()
			return
		}
	}
	s.overtimeStarted = &started
}

func (s *Store) saveAllLocked() error {
	if err := writeJSONAtomic(s.SettingsPath, s.Settings); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.RecordsPath, s.Records); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.LeavesPath, s.Leaves); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.TimelinePath, s.Timeline); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.CustomHolidayPath, s.HolidayYears); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.StatePath, s.Runtime); err != nil {
		return err
	}
	return nil
}

func (s *Store) SaveAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveAllLocked()
}

func (s *Store) recordFor(date time.Time) *DailyRecord {
	key := date.Format("2006-01-02")
	for i := range s.Records {
		if s.Records[i].Date == key {
			return &s.Records[i]
		}
	}
	s.Records = append(s.Records, DailyRecord{Date: key})
	return &s.Records[len(s.Records)-1]
}

func (s *Store) leaveFor(date time.Time) *LeaveRecord {
	key := date.Format("2006-01-02")
	for i := range s.Leaves {
		if s.Leaves[i].Date == key {
			return &s.Leaves[i]
		}
	}
	s.Leaves = append(s.Leaves, LeaveRecord{Date: key})
	return &s.Leaves[len(s.Leaves)-1]
}

func (s *Store) Tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delta := now.Sub(s.lastTick)
	if delta < 0 || delta > 5*time.Minute {
		delta = 0
	}
	s.lastTick = now

	info := s.DayInfo(now)
	if info.IsWorkday && inWorkWindow(now, s.Settings) {
		r := s.recordFor(now)
		r.WorkMinutes += delta.Minutes()
		if r.FirstSeen == "" {
			r.FirstSeen = now.Format("15:04:05")
		}
		r.LastSeen = now.Format("15:04:05")
	}
	if s.overtimeStarted != nil {
		s.Runtime.OvertimeStartedAt = s.overtimeStarted.Format(time.RFC3339Nano)
		s.Runtime.OvertimeLastHeartbeatAt = now.Format(time.RFC3339Nano)
	}
	if now.Sub(s.lastPersist) >= 30*time.Second {
		_ = s.saveAllLocked()
		s.lastPersist = now
	}
}

func (s *Store) StartOvertime(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overtimeStarted != nil {
		return
	}
	t := now
	s.overtimeStarted = &t
	s.Runtime.OvertimeStartedAt = t.Format(time.RFC3339Nano)
	s.Runtime.OvertimeLastHeartbeatAt = t.Format(time.RFC3339Nano)
	s.addTimelineLocked(t, "overtime-start", "开始加班")
	_ = s.saveAllLocked()
}

func (s *Store) StopOvertime(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overtimeStarted == nil {
		return
	}
	s.addOvertimeDuration(*s.overtimeStarted, now)
	s.addTimelineLocked(now, "overtime-end", "结束加班")
	s.overtimeStarted = nil
	s.Runtime.OvertimeStartedAt = ""
	s.Runtime.OvertimeLastHeartbeatAt = ""
	_ = s.saveAllLocked()
}

func (s *Store) addOvertimeDuration(start, end time.Time) {
	if end.Before(start) {
		return
	}
	cur := start
	for cur.Before(end) {
		nextDay := time.Date(cur.Year(), cur.Month(), cur.Day()+1, 0, 0, 0, 0, cur.Location())
		segEnd := end
		if nextDay.Before(end) {
			segEnd = nextDay
		}
		mins := segEnd.Sub(cur).Minutes()
		if mins > 0 {
			s.recordFor(cur).OvertimeMinutes += mins
			s.recordFor(cur).LastOvertimeEnd = segEnd.Format("15:04:05")
		}
		cur = segEnd
	}
}

func (s *Store) OvertimeRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overtimeStarted != nil
}
func (s *Store) OvertimeStartedAt() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overtimeStarted == nil {
		return time.Time{}, false
	}
	return *s.overtimeStarted, true
}

func (s *Store) addTimelineLocked(t time.Time, kind, text string) {
	s.Timeline = append(s.Timeline, TimelineEvent{Date: t.Format("2006-01-02"), Time: t.Format("15:04:05"), Kind: kind, Text: text})
}

func (s *Store) AddTimeline(t time.Time, kind, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addTimelineLocked(t, kind, text)
	_ = writeJSONAtomic(s.TimelinePath, s.Timeline)
}

func inWorkWindow(now time.Time, set Settings) bool {
	min := now.Hour()*60 + now.Minute()
	ws, _ := hmToMinutes(set.WorkStart)
	we, _ := hmToMinutes(set.WorkEnd)
	ls, _ := hmToMinutes(set.LunchStart)
	le, _ := hmToMinutes(set.LunchEnd)
	if min < ws || min >= we {
		return false
	}
	if ls < le && min >= ls && min < le {
		return false
	}
	return true
}

func hmToMinutes(v string) (int, error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 || len(parts[0]) < 1 || len(parts[0]) > 2 || len(parts[1]) != 2 {
		return 0, errors.New("bad time")
	}
	for _, part := range parts {
		for _, c := range part {
			if c < '0' || c > '9' {
				return 0, errors.New("bad time")
			}
		}
	}
	h, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || h > 23 || m > 59 {
		return 0, errors.New("bad time")
	}
	return h*60 + m, nil
}

func workMinutesPerDay(set Settings) int {
	ws, _ := hmToMinutes(set.WorkStart)
	we, _ := hmToMinutes(set.WorkEnd)
	ls, _ := hmToMinutes(set.LunchStart)
	le, _ := hmToMinutes(set.LunchEnd)
	total := we - ws
	if ls >= ws && le <= we && le > ls {
		total -= le - ls
	}
	if total < 1 {
		return 480
	}
	return total
}

func (s *Store) HourlyBase() float64 {
	base := s.Settings.OvertimeBaseSalary
	if base <= 0 {
		base = s.Settings.MonthlySalary
	}
	return base / 21.75 / 8.0
}

func (s *Store) TodayEarned(now time.Time) (float64, float64) {
	info := s.DayInfo(now)
	if !info.IsWorkday {
		return 0, 0
	}
	ws, _ := hmToMinutes(s.Settings.WorkStart)
	we, _ := hmToMinutes(s.Settings.WorkEnd)
	ls, _ := hmToMinutes(s.Settings.LunchStart)
	le, _ := hmToMinutes(s.Settings.LunchEnd)
	cur := now.Hour()*60 + now.Minute()
	elapsed := 0
	if cur <= ws {
		elapsed = 0
	} else if cur >= we {
		elapsed = workMinutesPerDay(s.Settings)
	} else {
		elapsed = cur - ws
		if cur > ls {
			elapsed -= minInt(cur, le) - ls
		}
		if elapsed < 0 {
			elapsed = 0
		}
	}
	total := workMinutesPerDay(s.Settings)
	progress := float64(elapsed) / float64(total)
	if progress > 1 {
		progress = 1
	}
	daily := s.Settings.MonthlySalary / s.Settings.MonthlyWorkDays
	return daily * progress, progress
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Store) NextPayday(now time.Time) time.Time {
	day := s.Settings.PaydayDay
	y, m := now.Year(), now.Month()
	mk := func(y int, m time.Month) time.Time {
		max := daysInMonth(y, m)
		d := day
		if d > max {
			d = max
		}
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	}
	t := mk(y, m)
	if !now.Before(t.Add(24 * time.Hour)) {
		m++
		if m > 12 {
			m = 1
			y++
		}
		t = mk(y, m)
	}
	return t
}
func daysInMonth(y int, m time.Month) int { return time.Date(y, m+1, 0, 0, 0, 0, 0, time.Local).Day() }

func (s *Store) AnnualLeaveEntitlement(now time.Time) float64 {
	if strings.TrimSpace(s.Settings.CumulativeWorkStartDate) == "" {
		return 0
	}
	t, err := parseAnyDate(s.Settings.CumulativeWorkStartDate)
	if err != nil {
		return 0
	}
	years := now.Year() - t.Year()
	if now.Month() < t.Month() || (now.Month() == t.Month() && now.Day() < t.Day()) {
		years--
	}
	if years < 1 {
		return 0
	}
	if years < 10 {
		return 5
	}
	if years < 20 {
		return 10
	}
	return 15
}

func (s *Store) YearSummary(year int) Summary {
	return s.YearSummaryAt(year, time.Now())
}
func (s *Store) YearSummaryAt(year int, now time.Time) Summary {
	return s.summaryAt(time.Date(year, 1, 1, 0, 0, 0, 0, now.Location()), time.Date(year+1, 1, 1, 0, 0, 0, 0, now.Location()), now)
}
func (s *Store) MonthSummary(year int, month time.Month) Summary {
	return s.MonthSummaryAt(year, month, time.Now())
}
func (s *Store) MonthSummaryAt(year int, month time.Month, now time.Time) Summary {
	return s.summaryAt(time.Date(year, month, 1, 0, 0, 0, 0, now.Location()), time.Date(year, month+1, 1, 0, 0, 0, 0, now.Location()), now)
}
func (s *Store) WeekSummary(now time.Time) Summary {
	off := (int(now.Weekday()) + 6) % 7
	start := time.Date(now.Year(), now.Month(), now.Day()-off, 0, 0, 0, 0, now.Location())
	return s.summaryAt(start, start.AddDate(0, 0, 7), now)
}

type Summary struct {
	Start, End           time.Time
	WorkDays             int // Scheduled workdays through today, not attendance.
	RecordedWorkDays     int
	WorkMinutes          float64
	OvertimeMinutes      float64
	AnnualDays           float64
	CompHours            float64
	LeaveDays            float64
	RestDays             int
	AdjustedWorkdays     int
	StatutoryDays        int
	ReferenceIncome      float64 // Estimated from recorded regular work minutes.
	ReferenceOvertimePay float64
	BusiestMonth         int
	BusiestMonthOvertime float64
	LatestEnd            string
	LongestStreak        int // Consecutive calendar days with recorded work.
}

func (s *Store) summaryAt(start, end, now time.Time) Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if end.After(cutoff) {
		end = cutoff
	}
	if end.Before(start) {
		end = start
	}
	sm := Summary{Start: start, End: end}
	overtimeByMonth := map[int]float64{}
	recByDate := map[string]DailyRecord{}
	for _, r := range s.Records {
		recByDate[r.Date] = r
	}
	streak := 0
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		info := s.DayInfo(d)
		if info.IsWorkday {
			sm.WorkDays++
		} else {
			sm.RestDays++
		}
		if info.Type == "AdjustedWorkday" {
			sm.AdjustedWorkdays++
		}
		if info.Type == "StatutoryHoliday" {
			sm.StatutoryDays++
		}
		r := recByDate[d.Format("2006-01-02")]
		// Include live overtime without mutating saved records.
		if s.overtimeStarted != nil {
			a, b := *s.overtimeStarted, now
			if a.Before(d) {
				a = d
			}
			if next := d.AddDate(0, 0, 1); b.After(next) {
				b = next
			}
			if b.After(a) {
				r.OvertimeMinutes += b.Sub(a).Minutes()
			}
		}
		sm.WorkMinutes += r.WorkMinutes
		sm.OvertimeMinutes += r.OvertimeMinutes
		overtimeByMonth[int(d.Month())] += r.OvertimeMinutes
		if r.LastOvertimeEnd > sm.LatestEnd {
			sm.LatestEnd = r.LastOvertimeEnd
		}
		sm.ReferenceOvertimePay += r.OvertimeMinutes / 60 * s.hourlyBaseUnlocked() * info.OvertimeMultiplier
		if r.WorkMinutes > 0 || r.OvertimeMinutes > 0 {
			sm.RecordedWorkDays++
			streak++
			if streak > sm.LongestStreak {
				sm.LongestStreak = streak
			}
		} else {
			streak = 0
		}
		for _, l := range s.Leaves {
			if l.Date == d.Format("2006-01-02") {
				sm.AnnualDays += l.AnnualDays
				sm.CompHours += l.CompHours
				sm.LeaveDays += l.LeaveDays + l.SickDays
			}
		}
	}
	sm.ReferenceIncome = sm.WorkMinutes / float64(workMinutesPerDay(s.Settings)) * (s.Settings.MonthlySalary / s.Settings.MonthlyWorkDays)
	for m := 1; m <= 12; m++ {
		if v := overtimeByMonth[m]; v > sm.BusiestMonthOvertime {
			sm.BusiestMonth, sm.BusiestMonthOvertime = m, v
		}
	}
	return sm
}

type AnnualLeaveBalance struct {
	Configured                                       bool
	Entitlement, Used, Planned, Remaining, Available float64
}

func (s *Store) AnnualLeaveBalance(now time.Time) AnnualLeaveBalance {
	s.mu.Lock()
	defer s.mu.Unlock()
	start, err := parseAnyDate(s.Settings.CumulativeWorkStartDate)
	if err != nil || start.Format("2006-01-02") > now.Format("2006-01-02") {
		return AnnualLeaveBalance{}
	}
	b := AnnualLeaveBalance{Configured: true, Entitlement: s.AnnualLeaveEntitlement(now)}
	today := now.Format("2006-01-02")
	for _, l := range s.Leaves {
		d, err := parseAnyDate(l.Date)
		if err != nil || d.Year() != now.Year() {
			continue
		}
		if d.Format("2006-01-02") <= today {
			b.Used += l.AnnualDays
		} else {
			b.Planned += l.AnnualDays
		}
	}
	b.Remaining = b.Entitlement - b.Used
	if b.Remaining < 0 {
		b.Remaining = 0
	}
	b.Available = b.Remaining - b.Planned
	if b.Available < 0 {
		b.Available = 0
	}
	return b
}

func (s *Store) hourlyBaseUnlocked() float64 {
	base := s.Settings.OvertimeBaseSalary
	if base <= 0 {
		base = s.Settings.MonthlySalary
	}
	return base / 21.75 / 8
}

func (s *Store) TimelineFor(date time.Time) []TimelineEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := date.Format("2006-01-02")
	var out []TimelineEvent
	for _, e := range s.Timeline {
		if e.Date == key {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func (s *Store) BuildAutoTimeline(date time.Time) []TimelineEvent {
	out := []TimelineEvent{{Date: date.Format("2006-01-02"), Time: s.Settings.WorkStart, Kind: "work-start", Text: "上班"}}
	if s.Settings.LunchStart != "" {
		out = append(out, TimelineEvent{Date: date.Format("2006-01-02"), Time: s.Settings.LunchStart, Kind: "lunch", Text: "午休"})
	}
	if s.Settings.LunchEnd != "" {
		out = append(out, TimelineEvent{Date: date.Format("2006-01-02"), Time: s.Settings.LunchEnd, Kind: "work-resume", Text: "继续打工"})
	}
	out = append(out, TimelineEvent{Date: date.Format("2006-01-02"), Time: s.Settings.WorkEnd, Kind: "work-end", Text: "下班"})
	out = append(out, s.TimelineFor(date)...)
	sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func (s *Store) CreateBackup(dest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.saveAllLocked()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	files := []string{s.SettingsPath, s.RecordsPath, s.LeavesPath, s.TimelinePath, s.CustomHolidayPath, s.StatePath}
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		w, err := zw.Create(filepath.Base(p))
		if err != nil {
			return err
		}
		if _, err = w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RestoreBackup(src string) error {
	// safety backup before restore
	_ = os.MkdirAll(s.BackupDir, 0755)
	safety := filepath.Join(s.BackupDir, "before-restore-"+time.Now().Format("20060102-150405")+".zip")
	_ = s.CreateBackup(safety)
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	allowed := map[string]string{"settings.json": s.SettingsPath, "records.json": s.RecordsPath, "leave-records.json": s.LeavesPath, "timeline.json": s.TimelinePath, "holiday-calendar.json": s.CustomHolidayPath, "state.json": s.StatePath}
	for _, zf := range r.File {
		target, ok := allowed[filepath.Base(zf.Name)]
		if !ok {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 16<<20))
		rc.Close()
		if err != nil {
			return err
		}
		if !json.Valid(data) {
			return fmt.Errorf("%s 不是有效 JSON", zf.Name)
		}
		if err = os.WriteFile(target, data, 0644); err != nil {
			return err
		}
	}
	ns, err := newStore()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Settings = ns.Settings
	s.Records = ns.Records
	s.Leaves = ns.Leaves
	s.Timeline = ns.Timeline
	s.HolidayYears = ns.HolidayYears
	s.Runtime = ns.Runtime
	s.overtimeStarted = ns.overtimeStarted
	return nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (s *Store) ImportHolidayFile(path string) error {
	var cfg []HolidayYearConfig
	if err := readJSON(path, &cfg); err != nil {
		return err
	}
	if len(cfg) == 0 {
		return errors.New("配置为空")
	}
	years := map[int]HolidayYearConfig{}
	for _, x := range s.HolidayYears {
		years[x.Year] = x
	}
	for _, x := range cfg {
		if x.Year < 2020 || x.Year > 2100 {
			return fmt.Errorf("年份无效: %d", x.Year)
		}
		years[x.Year] = x
	}
	var merged []HolidayYearConfig
	for _, x := range years {
		merged = append(merged, x)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Year < merged[j].Year })
	s.mu.Lock()
	s.HolidayYears = merged
	err := writeJSONAtomic(s.CustomHolidayPath, merged)
	s.mu.Unlock()
	return err
}

func ExportHolidayTemplate(path string, year int) error {
	cfg := []HolidayYearConfig{{Year: year, AdjustedWorkdays: []string{}, StatutoryHolidays: []StatutoryHolidayConfig{}, HolidaySpans: []HolidaySpanConfig{}}}
	return writeJSONAtomic(path, cfg)
}
