package admin

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$`)
var analyticsZone = time.FixedZone("China Standard Time", 8*60*60)

func (s *Store) initAnalytics() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS telemetry_devices (
 device_id TEXT PRIMARY KEY, first_seen TEXT NOT NULL, first_day TEXT NOT NULL,
 last_seen TEXT NOT NULL, first_version TEXT NOT NULL, current_version TEXT NOT NULL,
 trial_started_at TEXT NOT NULL DEFAULT '', pro_activated_at TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS telemetry_days (
 day TEXT NOT NULL, device_id TEXT NOT NULL REFERENCES telemetry_devices(device_id),
 version TEXT NOT NULL, PRIMARY KEY(day,device_id));
CREATE INDEX IF NOT EXISTS telemetry_days_version ON telemetry_days(version,day);
CREATE TABLE IF NOT EXISTS telemetry_version_changes (
 id INTEGER PRIMARY KEY AUTOINCREMENT, device_id TEXT NOT NULL REFERENCES telemetry_devices(device_id),
 from_version TEXT NOT NULL, to_version TEXT NOT NULL, changed_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS telemetry_errors (
 day TEXT NOT NULL, device_id TEXT NOT NULL REFERENCES telemetry_devices(device_id),
 version TEXT NOT NULL, code TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 1,
 PRIMARY KEY(day,device_id,version,code));
CREATE TABLE IF NOT EXISTS order_activations (
 order_id TEXT PRIMARY KEY REFERENCES orders(id), activated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS download_clicks (day TEXT PRIMARY KEY, count INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS analytics_meta (name TEXT PRIMARY KEY, value TEXT NOT NULL);
`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT OR IGNORE INTO analytics_meta(name,value) VALUES('started_at',?)", stamp())
	return err
}

func (s *Store) RecordDownloadClick() error {
	day := time.Now().In(analyticsZone).Format("2006-01-02")
	_, err := s.db.Exec("INSERT INTO download_clicks(day,count) VALUES(?,1) ON CONFLICT(day) DO UPDATE SET count=count+1", day)
	return err
}

func validTelemetryDevice(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 32 && strings.ToLower(id) == id
}

// TelemetryReport contains only an opaque device ID and product lifecycle data.
// The client sends it only after the user explicitly opts in.
type TelemetryReport struct {
	DeviceID       string `json:"device_id"`
	Version        string `json:"version"`
	TrialStartedAt string `json:"trial_started_at"`
	Pro            bool   `json:"pro"`
}

func (s *Store) RecordTelemetry(v TelemetryReport) error {
	return s.recordTelemetryAt(v, time.Now())
}
func (s *Store) recordTelemetryAt(v TelemetryReport, now time.Time) error {
	if !validTelemetryDevice(v.DeviceID) || !versionPattern.MatchString(v.Version) {
		return errors.New("匿名统计事件无效")
	}
	if v.TrialStartedAt != "" {
		started, err := time.Parse(time.RFC3339, v.TrialStartedAt)
		if err != nil || started.After(now.Add(24*time.Hour)) || started.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
			return errors.New("试用开始时间无效")
		}
		v.TrialStartedAt = started.UTC().Format(time.RFC3339)
	}
	stampAt := now.UTC().Format(time.RFC3339Nano)
	day := now.In(analyticsZone).Format("2006-01-02")
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRow("SELECT current_version FROM telemetry_devices WHERE device_id=?", v.DeviceID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		activated := ""
		if v.Pro {
			activated = stampAt
		}
		_, err = tx.Exec("INSERT INTO telemetry_devices(device_id,first_seen,first_day,last_seen,first_version,current_version,trial_started_at,pro_activated_at) VALUES(?,?,?,?,?,?,?,?)", v.DeviceID, stampAt, day, stampAt, v.Version, v.Version, v.TrialStartedAt, activated)
	} else if err == nil {
		if previous != v.Version {
			_, err = tx.Exec("INSERT INTO telemetry_version_changes(device_id,from_version,to_version,changed_at) VALUES(?,?,?,?)", v.DeviceID, previous, v.Version, stampAt)
		}
		if err == nil {
			activated := ""
			if v.Pro {
				activated = stampAt
			}
			_, err = tx.Exec(`UPDATE telemetry_devices SET last_seen=?,current_version=?,
 trial_started_at=CASE WHEN trial_started_at='' THEN ? ELSE trial_started_at END,
 pro_activated_at=CASE WHEN pro_activated_at='' THEN ? ELSE pro_activated_at END WHERE device_id=?`, stampAt, v.Version, v.TrialStartedAt, activated, v.DeviceID)
		}
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO telemetry_days(day,device_id,version) VALUES(?,?,?) ON CONFLICT(day,device_id) DO UPDATE SET version=excluded.version", day, v.DeviceID, v.Version); err != nil {
		return err
	}
	return tx.Commit()
}

var errorCodes = map[string]bool{
	"purchase_sync": true, "license_save": true, "backup_failed": true,
	"restore_failed": true, "window_create": true, "unexpected": true,
}

type ErrorReport struct {
	DeviceID string `json:"device_id"`
	Version  string `json:"version"`
	Code     string `json:"code"`
}

func (s *Store) RecordError(v ErrorReport) error {
	if !validTelemetryDevice(v.DeviceID) || !versionPattern.MatchString(v.Version) || !errorCodes[v.Code] {
		return errors.New("错误统计事件无效")
	}
	day := time.Now().In(analyticsZone).Format("2006-01-02")
	result, err := s.db.Exec(`INSERT INTO telemetry_errors(day,device_id,version,code,count)
 SELECT ?,?,?,?,1 WHERE EXISTS(SELECT 1 FROM telemetry_devices WHERE device_id=?)
 ON CONFLICT(day,device_id,version,code) DO UPDATE SET count=min(count+1,5)`, day, v.DeviceID, v.Version, v.Code, v.DeviceID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return errors.New("设备尚未启用匿名统计")
	}
	return nil
}

func (s *Store) AckActivation(id, token string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow("SELECT status FROM orders WHERE id=? AND token_hash=?", id, tokenHash(token)).Scan(&status); err != nil {
		return err
	}
	if status != "approved" {
		return errors.New("订单尚未开通")
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO order_activations(order_id,activated_at) VALUES(?,?)", id, stamp()); err != nil {
		return err
	}
	return tx.Commit()
}

type DailyPoint struct {
	Day    string `json:"day"`
	Active int    `json:"active"`
	New    int    `json:"new"`
}
type VersionFlow struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}
type VersionError struct {
	Version string `json:"version"`
	Reports int    `json:"reports"`
}
type AnalyticsOverview struct {
	Day                string         `json:"day"`
	TodayNew           int            `json:"today_new"`
	TodayFirstStarts   int            `json:"today_first_starts"`
	TodayActive        int            `json:"today_active"`
	TotalKnownDevices  int            `json:"total_known_devices"`
	TodayPaidOrders    int            `json:"today_paid_orders"`
	TodayRevenueCents  int            `json:"today_revenue_cents"`
	KnownProDevices    int            `json:"known_pro_devices"`
	TrialDevices       int            `json:"trial_devices"`
	TrialToPaidPercent float64        `json:"trial_to_paid_percent"`
	ApprovedOrders     int            `json:"approved_orders"`
	ActivatedOrders    int            `json:"activated_orders"`
	ActivationPercent  float64        `json:"activation_percent"`
	DownloadClicks     *int           `json:"download_clicks"`
	Trend              []DailyPoint   `json:"trend"`
	VersionFlows       []VersionFlow  `json:"version_flows"`
	VersionErrors      []VersionError `json:"version_errors"`
}

func (s *Store) AnalyticsOverview() (AnalyticsOverview, error) {
	return s.analyticsOverviewAt(time.Now())
}
func (s *Store) analyticsOverviewAt(now time.Time) (AnalyticsOverview, error) {
	day := now.In(analyticsZone).Format("2006-01-02")
	v := AnalyticsOverview{Day: day, Trend: []DailyPoint{}, VersionFlows: []VersionFlow{}, VersionErrors: []VersionError{}}
	counts := []struct {
		query string
		out   *int
	}{
		{"SELECT count(*) FROM telemetry_devices WHERE first_day=?", &v.TodayNew},
		{"SELECT count(*) FROM telemetry_devices WHERE trial_started_at<>'' AND date(trial_started_at,'+8 hours')=?", &v.TodayFirstStarts},
		{"SELECT count(*) FROM telemetry_days WHERE day=?", &v.TodayActive},
		{"SELECT count(*) FROM (SELECT device_id FROM telemetry_devices UNION SELECT device_id FROM orders)", &v.TotalKnownDevices},
		{"SELECT count(*) FROM orders WHERE status='approved' AND date(updated_at,'+8 hours')=?", &v.TodayPaidOrders},
		{"SELECT COALESCE(sum(amount_cents),0) FROM orders WHERE status='approved' AND date(updated_at,'+8 hours')=?", &v.TodayRevenueCents},
		{"SELECT count(*) FROM (SELECT device_id FROM orders WHERE status='approved' UNION SELECT device_id FROM telemetry_devices WHERE pro_activated_at<>'')", &v.KnownProDevices},
		{"SELECT count(*) FROM telemetry_devices WHERE trial_started_at<>''", &v.TrialDevices},
		{"SELECT count(*) FROM orders WHERE status='approved' AND updated_at>=(SELECT value FROM analytics_meta WHERE name='started_at')", &v.ApprovedOrders},
		{"SELECT count(*) FROM order_activations a JOIN orders o ON o.id=a.order_id WHERE o.updated_at>=(SELECT value FROM analytics_meta WHERE name='started_at')", &v.ActivatedOrders},
	}
	for i, q := range counts {
		var err error
		if i == 0 || i == 1 || i == 2 || i == 4 || i == 5 {
			err = s.db.QueryRow(q.query, day).Scan(q.out)
		} else {
			err = s.db.QueryRow(q.query).Scan(q.out)
		}
		if err != nil {
			return v, err
		}
	}
	var clickCount int
	if err := s.db.QueryRow("SELECT count FROM download_clicks WHERE day=?", day).Scan(&clickCount); err == nil {
		v.DownloadClicks = &clickCount
	} else if !errors.Is(err, sql.ErrNoRows) {
		return v, err
	}
	if v.TrialDevices > 0 {
		var paidTrial int
		if err := s.db.QueryRow(`SELECT count(*) FROM telemetry_devices t WHERE trial_started_at<>'' AND
 (pro_activated_at<>'' OR EXISTS(SELECT 1 FROM orders o WHERE o.device_id=t.device_id AND o.status='approved'))`).Scan(&paidTrial); err != nil {
			return v, err
		}
		v.TrialToPaidPercent = float64(paidTrial) * 100 / float64(v.TrialDevices)
	}
	if v.ApprovedOrders > 0 {
		v.ActivationPercent = float64(v.ActivatedOrders) * 100 / float64(v.ApprovedOrders)
	}
	start := now.In(analyticsZone).AddDate(0, 0, -13)
	pointByDay := map[string]int{}
	v.Trend = make([]DailyPoint, 14)
	for i := range v.Trend {
		d := start.AddDate(0, 0, i).Format("2006-01-02")
		v.Trend[i] = DailyPoint{Day: d}
		pointByDay[d] = i
	}
	for _, item := range []struct {
		query string
		set   func(*DailyPoint, int)
	}{
		{"SELECT day,count(*) FROM telemetry_days WHERE day>=? GROUP BY day", func(p *DailyPoint, n int) { p.Active = n }},
		{"SELECT first_day,count(*) FROM telemetry_devices WHERE first_day>=? GROUP BY first_day", func(p *DailyPoint, n int) { p.New = n }},
	} {
		rows, err := s.db.Query(item.query, v.Trend[0].Day)
		if err != nil {
			return v, err
		}
		for rows.Next() {
			var d string
			var n int
			if err = rows.Scan(&d, &n); err != nil {
				break
			}
			if index, ok := pointByDay[d]; ok {
				item.set(&v.Trend[index], n)
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return v, err
		}
	}
	flows, err := s.db.Query("SELECT from_version,to_version,count(*) FROM telemetry_version_changes GROUP BY from_version,to_version ORDER BY count(*) DESC LIMIT 12")
	if err != nil {
		return v, err
	}
	for flows.Next() {
		var f VersionFlow
		if err = flows.Scan(&f.From, &f.To, &f.Count); err != nil {
			break
		}
		v.VersionFlows = append(v.VersionFlows, f)
	}
	if err == nil {
		err = flows.Err()
	}
	flows.Close()
	if err != nil {
		return v, err
	}
	bugs, err := s.db.Query("SELECT version,sum(count) FROM telemetry_errors GROUP BY version ORDER BY sum(count) DESC LIMIT 12")
	if err != nil {
		return v, err
	}
	for bugs.Next() {
		var b VersionError
		if err = bugs.Scan(&b.Version, &b.Reports); err != nil {
			break
		}
		v.VersionErrors = append(v.VersionErrors, b)
	}
	if err == nil {
		err = bugs.Err()
	}
	bugs.Close()
	return v, err
}

type AnalyticsUser struct {
	DeviceID       string `json:"device_id"`
	FirstSeen      string `json:"first_seen"`
	LastSeen       string `json:"last_seen"`
	FirstVersion   string `json:"first_version"`
	CurrentVersion string `json:"current_version"`
	TrialStartedAt string `json:"trial_started_at"`
	ProActivatedAt string `json:"pro_activated_at"`
	Edition        string `json:"edition"`
	AuthStatus     string `json:"auth_status"`
}

func (s *Store) AnalyticsUsers(search string, offset int) ([]AnalyticsUser, int, error) {
	search = strings.TrimSpace(search)
	if len(search) > 64 || offset < 0 || offset > 10000000 {
		return nil, 0, errors.New("查询条件无效")
	}
	filter := " WHERE (?='' OR instr(t.device_id,?)>0 OR instr(t.current_version,?)>0)"
	args := []any{search, search, search}
	var total int
	if err := s.db.QueryRow("SELECT count(*) FROM telemetry_devices t"+filter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT t.device_id,t.first_seen,t.last_seen,t.first_version,t.current_version,t.trial_started_at,t.pro_activated_at,
 CASE WHEN t.pro_activated_at<>'' OR EXISTS(SELECT 1 FROM orders o WHERE o.device_id=t.device_id AND o.status='approved') THEN 'pro' ELSE 'free' END,
 CASE WHEN EXISTS(SELECT 1 FROM order_activations a JOIN orders o ON o.id=a.order_id WHERE o.device_id=t.device_id) THEN 'received'
 WHEN EXISTS(SELECT 1 FROM orders o WHERE o.device_id=t.device_id AND o.status='approved') THEN 'pending'
 WHEN t.pro_activated_at<>'' THEN 'offline' ELSE 'none' END
 FROM telemetry_devices t`+filter+` ORDER BY t.last_seen DESC LIMIT 50 OFFSET ?`, append(args, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []AnalyticsUser{}
	for rows.Next() {
		var u AnalyticsUser
		if err := rows.Scan(&u.DeviceID, &u.FirstSeen, &u.LastSeen, &u.FirstVersion, &u.CurrentVersion, &u.TrialStartedAt, &u.ProActivatedAt, &u.Edition, &u.AuthStatus); err != nil {
			return nil, 0, err
		}
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
