package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDate(v string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", v, time.FixedZone("test", 8*3600))
	if err != nil {
		panic(err)
	}
	return t
}

func testStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	s, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	s.Settings.UseMainlandHolidayCalendar = false
	return s
}

func TestSummaryExcludesFutureAndUsesRecordedIncome(t *testing.T) {
	s := testStore(t)
	s.Records = []DailyRecord{
		{Date: "2026-01-01", WorkMinutes: 480}, {Date: "2026-01-02", WorkMinutes: 240},
		{Date: "2026-01-04", OvertimeMinutes: 60}, {Date: "2026-01-07", WorkMinutes: 120},
		{Date: "2026-01-08", WorkMinutes: 480, OvertimeMinutes: 600},
	}
	s.Leaves = []LeaveRecord{{Date: "2026-01-03", AnnualDays: 1}, {Date: "2026-01-08", AnnualDays: 2}}
	now := testDate("2026-01-07 15:00")
	sm := s.MonthSummaryAt(2026, time.January, now)
	if sm.WorkDays != 5 || sm.RestDays != 2 || sm.RecordedWorkDays != 4 || sm.WorkMinutes != 840 || sm.OvertimeMinutes != 60 || sm.AnnualDays != 1 || sm.LongestStreak != 2 {
		t.Fatalf("unexpected summary: %+v", sm)
	}
	want := 840.0 / 480 * (8000.0 / 21.75)
	if math.Abs(sm.ReferenceIncome-want) > 1e-9 {
		t.Fatalf("income = %v, want %v", sm.ReferenceIncome, want)
	}
	if sm.End.Format("2006-01-02") != "2026-01-08" {
		t.Fatalf("cutoff: %v", sm.End)
	}
	if yr := s.YearSummaryAt(2026, now); yr.WorkDays != sm.WorkDays || yr.ReferenceIncome != sm.ReferenceIncome {
		t.Fatalf("year includes future: %+v", yr)
	}
	if wk := s.WeekSummary(now); wk.WorkDays != 3 || wk.WorkMinutes != 120 {
		t.Fatalf("week includes future: %+v", wk)
	}
	if future := s.MonthSummaryAt(2026, time.February, now); future.WorkDays != 0 || future.ReferenceIncome != 0 || future.End.Before(future.Start) {
		t.Fatalf("future range: %+v", future)
	}
	if past := s.MonthSummaryAt(2025, time.December, now); past.WorkDays != 23 || past.RestDays != 8 {
		t.Fatalf("past range truncated: %+v", past)
	}
}

func TestLiveOvertimeAcrossMonthWithoutDoubleCounting(t *testing.T) {
	s := testStore(t)
	start := testDate("2026-01-31 23:30")
	now := testDate("2026-02-01 00:30")
	s.StartOvertime(start)
	jan, feb := s.MonthSummaryAt(2026, time.January, now), s.MonthSummaryAt(2026, time.February, now)
	if jan.OvertimeMinutes != 30 || feb.OvertimeMinutes != 30 || jan.RecordedWorkDays != 1 || feb.RecordedWorkDays != 1 {
		t.Fatalf("live split: jan=%+v feb=%+v", jan, feb)
	}
	if len(s.Records) != 0 {
		t.Fatal("reading summary mutated persisted overtime")
	}
	s.StopOvertime(now)
	if after := s.YearSummaryAt(2026, now); after.OvertimeMinutes != 60 || after.LongestStreak != 2 {
		t.Fatalf("stopped session counted twice: %+v", after)
	}
}

func TestTodayIncomeAndLunchBoundaries(t *testing.T) {
	s := testStore(t)
	for _, tc := range []struct {
		at       string
		progress float64
	}{
		{"2026-01-07 08:59", 0}, {"2026-01-07 12:00", 3.0 / 8}, {"2026-01-07 12:30", 3.0 / 8},
		{"2026-01-07 13:30", 3.5 / 8}, {"2026-01-07 18:00", 1}, {"2026-01-10 15:00", 0},
	} {
		earned, progress := s.TodayEarned(testDate(tc.at))
		if progress != tc.progress || math.Abs(earned-progress*8000/21.75) > 1e-9 {
			t.Errorf("%s: income %v progress %v", tc.at, earned, progress)
		}
	}
}

func TestAnnualLeaveAnniversaryAndBalances(t *testing.T) {
	s := testStore(t)
	s.Settings.CumulativeWorkStartDate = "2019-03-01"
	for _, tc := range []struct {
		at   string
		want float64
	}{
		{"2020-02-29 12:00", 0}, {"2020-03-01 12:00", 5}, {"2029-02-28 12:00", 5},
		{"2029-03-01 12:00", 10}, {"2039-03-01 12:00", 15},
	} {
		if got := s.AnnualLeaveEntitlement(testDate(tc.at)); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.at, got, tc.want)
		}
	}
	s.Leaves = []LeaveRecord{{Date: "2026-01-01", AnnualDays: 1.5}, {Date: "2026-09-30", AnnualDays: 2}, {Date: "2025-12-31", AnnualDays: 9}, {Date: "bad", AnnualDays: 9}}
	b := s.AnnualLeaveBalance(testDate("2026-09-29 12:00"))
	if !b.Configured || b.Entitlement != 5 || b.Used != 1.5 || b.Planned != 2 || b.Remaining != 3.5 || b.Available != 1.5 {
		t.Fatalf("balance: %+v", b)
	}
	s.Leaves = append(s.Leaves, LeaveRecord{Date: "2026-10-01", AnnualDays: 10})
	if s.AnnualLeaveBalance(testDate("2026-09-29 12:00")).Available != 0 {
		t.Fatal("negative available leave")
	}
	s.Settings.CumulativeWorkStartDate = "2026-09-29"
	if !s.AnnualLeaveBalance(testDate("2026-09-29 00:01")).Configured {
		t.Fatal("same-day start date rejected due to timezone")
	}
	s.Settings.CumulativeWorkStartDate = ""
	if s.AnnualLeaveBalance(time.Now()).Configured {
		t.Fatal("missing start date treated as configured")
	}
}

func TestStartupPresentation(t *testing.T) {
	for _, background := range []bool{false, true} {
		for _, hidden := range []bool{false, true} {
			for _, manualFloat := range []bool{false, true} {
				for _, autoFloat := range []bool{false, true} {
					s := defaultSettings()
					s.StartHidden = hidden
					s.ShowFloatingOnStartup = manualFloat
					s.ShowFloatingOnAutoStart = autoFloat
					h, f := startupPresentation(s, background)
					wantF := manualFloat
					if background {
						wantF = autoFloat
					}
					if h != (background || hidden) || f != wantF {
						t.Fatalf("startup: %+v background=%v => %v,%v", s, background, h, f)
					}
				}
			}
		}
	}
}

func TestEveningReminderRespectsMinutes(t *testing.T) {
	s := testStore(t)
	s.Settings.PaydayDay = 30
	has := func(at string, key string) bool {
		for _, r := range s.DueReminders(testDate(at)) {
			if r.Key == key {
				return true
			}
		}
		return false
	}
	if has("2026-09-29 16:29", "payday") || !has("2026-09-29 16:30", "payday") || !has("2026-09-29 17:01", "payday") {
		t.Fatal("16:30 boundary is incorrect")
	}
	if !has("2026-09-29 17:30", "off30") || !has("2026-09-29 17:55", "off5") || !has("2026-09-29 18:00", "off") || has("2026-09-29 18:02", "off") {
		t.Fatal("off-work boundaries incorrect")
	}
	s.Settings.ReminderEnabled = false
	if len(s.DueReminders(testDate("2026-09-29 17:55"))) != 0 {
		t.Fatal("disabled reminder delivered")
	}
}

func TestFloatingDimensions(t *testing.T) {
	s := defaultSettings()
	for _, tc := range []struct {
		size          string
		width, height int
	}{{"Small", 260, 78}, {"Medium", 340, 102}, {"Large", 420, 126}} {
		s.FloatingSize = tc.size
		w, h := floatingDimensions(s)
		if w != tc.width || h != tc.height {
			t.Fatalf("%s: %d x %d", tc.size, w, h)
		}
		s.FloatingCompact = true
		s.FloatingShowPhrase = true
		w, h = floatingDimensions(s)
		if w != tc.width || h != 84 {
			t.Fatalf("compact phrase: %d x %d", w, h)
		}
		s.FloatingCompact = false
		s.FloatingShowPhrase = false
	}
}

func TestSettingsValidationAndPersistence(t *testing.T) {
	s := testStore(t)
	now := testDate("2026-09-29 12:00")
	for _, tc := range []struct {
		name   string
		change func(*Settings)
	}{
		{"salary NaN", func(v *Settings) { v.MonthlySalary = math.NaN() }}, {"salary negative", func(v *Settings) { v.MonthlySalary = -1 }},
		{"base Infinity", func(v *Settings) { v.OvertimeBaseSalary = math.Inf(1) }}, {"days zero", func(v *Settings) { v.MonthlyWorkDays = 0 }},
		{"reversed shift", func(v *Settings) { v.WorkEnd = "08:00" }}, {"lunch outside", func(v *Settings) { v.LunchStart = "08:00" }},
		{"malformed time", func(v *Settings) { v.WorkStart = "09:00junk" }}, {"future employment", func(v *Settings) { v.CumulativeWorkStartDate = "2027-01-01" }},
		{"invalid employment", func(v *Settings) { v.CumulativeWorkStartDate = "2026-02-30" }}, {"opacity out of range", func(v *Settings) { v.FloatingOpacity = 0.1 }},
		{"unknown mode", func(v *Settings) { v.FloatingMode = "Unknown" }},
		{"unknown size", func(v *Settings) { v.FloatingSize = "Huge" }}, {"minute out of range", func(v *Settings) { v.ReminderEveningMinute = 60 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := s.Settings
			tc.change(&v)
			if err := s.UpdateSettings(v, now); err == nil {
				t.Fatal("invalid settings accepted")
			}
			if s.Settings != defaultTestSettings() {
				t.Fatal("invalid save changed in-memory settings")
			}
		})
	}
	v := s.Settings
	v.StartHidden = true
	v.FloatingCompact = true
	v.FloatingShowPhrase = true
	v.FloatingOpacity = .65
	v.FloatingPalette = "Mint"
	v.FloatingSize = "Large"
	v.ReminderEveningMinute = 45
	if err := s.UpdateSettings(v, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings != v {
		t.Fatalf("settings round trip: %+v", loaded.Settings)
	}
	s.SettingsPath = filepath.Join(t.TempDir(), "missing", "settings.json")
	v.MonthlySalary = 9000
	if err := s.UpdateSettings(v, now); err == nil || s.Settings.MonthlySalary != 8000 {
		t.Fatal("failed write changed running settings")
	}
}

func defaultTestSettings() Settings {
	s := defaultSettings()
	s.UseMainlandHolidayCalendar = false
	return s
}

func TestTimeParserRejectsTrailingGarbage(t *testing.T) {
	for _, v := range []string{"24:00", "09:60", "09:00junk", "09:00:00", "-1:00", "9:0", "09:+1"} {
		if _, err := hmToMinutes(v); err == nil {
			t.Errorf("accepted %q", v)
		}
	}
	if got, err := hmToMinutes(" 9:00 "); err != nil || got != 540 {
		t.Fatalf("legacy time rejected: %v %v", got, err)
	}
}

func TestTrialExpiryNoticeOnceAndRollback(t *testing.T) {
	now := testDate("2026-09-29 12:00")
	lm := LicenseManager{trialStart: now, trialLastSeen: now, lastPersist: time.Now()}
	if lm.ConsumeExpiryNotice(now.Add(trialDuration - time.Second)) {
		t.Fatal("premature expiry")
	}
	if !lm.ConsumeExpiryNotice(now.Add(trialDuration)) || lm.HasProAccess(now.Add(trialDuration)) {
		t.Fatal("expiry boundary not enforced")
	}
	if lm.ConsumeExpiryNotice(now.Add(trialDuration + time.Second)) {
		t.Fatal("repeated purchase prompt")
	}
	lm = LicenseManager{trialStart: now, trialLastSeen: now, lastPersist: time.Now()}
	lm.Tick(now.Add(-11 * time.Minute))
	if lm.HasProAccess(now) || lm.TrialRemaining(now) != 0 || !strings.Contains(lm.StatusText(now), "系统时间异常") {
		t.Fatal("rollback status misleading")
	}
	lm.pro = true
	if !lm.HasProAccess(now) || lm.ConsumeExpiryNotice(now) {
		t.Fatal("Pro user gated")
	}
}

func TestTrialSealRejectsTampering(t *testing.T) {
	st := trialState{DeviceID: "test", StartedAt: "2026-09-29T00:00:00Z", LastSeenAt: "2026-09-29T01:00:00Z"}
	token := sealTrial(st)
	got, err := unsealTrial(token)
	if err != nil || got != st {
		t.Fatal("trial round trip failed")
	}
	parts := strings.Split(token, ".")
	parts[0] = base64.RawURLEncoding.EncodeToString([]byte(`{"device_id":"other"}`))
	if _, err := unsealTrial(strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestActivationSignatureAndDeviceBinding(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(pub)
	p := licensePayload{Version: 1, LicenseID: "TEST-1", Edition: "pro", DeviceID: "test-device", IssuedAt: "2026-09-29T00:00:00Z"}
	sign := func(p licensePayload) string {
		b, _ := json.Marshal(p)
		return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, b))
	}
	code := sign(p)
	if err := verifyActivationCode(code, p.DeviceID, key); err != nil {
		t.Fatal(err)
	}
	if err := verifyActivationCode(code, "other-device", key); err == nil {
		t.Fatal("foreign device accepted")
	}
	for _, tc := range []struct {
		name   string
		change func(*licensePayload)
	}{
		{"edition", func(p *licensePayload) { p.Edition = "free" }}, {"version", func(p *licensePayload) { p.Version = 99 }},
		{"id", func(p *licensePayload) { p.LicenseID = "" }}, {"issued time", func(p *licensePayload) { p.IssuedAt = "bad" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := p
			tc.change(&q)
			if verifyActivationCode(sign(q), p.DeviceID, key) == nil {
				t.Fatal("invalid payload accepted")
			}
		})
	}
	parts := strings.Split(code, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[1])
	sig[0] ^= 1
	parts[1] = base64.RawURLEncoding.EncodeToString(sig)
	if verifyActivationCode(strings.Join(parts, "."), p.DeviceID, key) == nil {
		t.Fatal("bad signature accepted")
	}
	lm := LicenseManager{dataDir: t.TempDir(), deviceID: p.DeviceID}
	if lm.Activate("bad-code") == nil || lm.IsPro() {
		t.Fatal("invalid activation unlocked client")
	}
}

func TestCommerceSetupFailsClosed(t *testing.T) {
	dir := t.TempDir()
	cfg, err := loadCommerceConfig(dir)
	if err != nil || cfg.PurchaseReady(dir) == nil {
		t.Fatal("missing config opens purchases")
	}
	cfg = CommerceConfig{Enabled: true, Price: "¥19.90", Contact: "开发者联系入口"}
	if cfg.PurchaseReady(dir) == nil {
		t.Fatal("missing QR accepted")
	}
	b, err := os.ReadFile(filepath.Join("assets", "payment_qr.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "payment_qr.png"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if cfg.PurchaseReady(dir) == nil {
		t.Fatal("placeholder QR accepted")
	}
	f, err := os.Create(filepath.Join(dir, "payment_qr.png"))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err = cfg.PurchaseReady(dir); err != nil {
		t.Fatal(err)
	}
	cfg.Contact = ""
	if cfg.PurchaseReady(dir) == nil {
		t.Fatal("missing contact accepted")
	}
	if err = os.WriteFile(filepath.Join(dir, "commerce.json"), []byte("bad json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadCommerceConfig(dir); err == nil {
		t.Fatal("malformed commerce config ignored")
	}
}
