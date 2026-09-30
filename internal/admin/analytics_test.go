package admin

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnalyticsCountsUniqueDevicesDaysAndVersionChanges(t *testing.T) {
	f := fixture(t)
	id := strings.Repeat("a", 64)
	beforeMidnight := time.Date(2026, 9, 30, 23, 55, 0, 0, analyticsZone)
	afterMidnight := beforeMidnight.Add(10 * time.Minute)
	report := TelemetryReport{DeviceID: id, Version: "0.8.0", TrialStartedAt: beforeMidnight.Add(-time.Hour).UTC().Format(time.RFC3339)}
	for i := 0; i < 2; i++ {
		if err := f.store.recordTelemetryAt(report, beforeMidnight); err != nil {
			t.Fatal(err)
		}
	}
	report.Version = "0.9.0"
	if err := f.store.recordTelemetryAt(report, afterMidnight); err != nil {
		t.Fatal(err)
	}
	v, err := f.store.analyticsOverviewAt(afterMidnight)
	if err != nil {
		t.Fatal(err)
	}
	if v.Day != "2026-10-01" || v.TodayNew != 0 || v.TodayFirstStarts != 0 || v.TodayActive != 1 || v.TotalKnownDevices != 1 || v.TrialDevices != 1 {
		t.Fatalf("unexpected overview: %+v", v)
	}
	if len(v.Trend) != 14 || v.Trend[12].Day != "2026-09-30" || v.Trend[12].New != 1 || v.Trend[12].Active != 1 || v.Trend[13].Active != 1 {
		t.Fatalf("wrong daily trend: %+v", v.Trend)
	}
	if len(v.VersionFlows) != 1 || v.VersionFlows[0] != (VersionFlow{From: "0.8.0", To: "0.9.0", Count: 1}) {
		t.Fatalf("wrong upgrade: %+v", v.VersionFlows)
	}
	users, total, err := f.store.AnalyticsUsers("0.9.0", 0)
	if err != nil || total != 1 || len(users) != 1 || users[0].FirstVersion != "0.8.0" || users[0].CurrentVersion != "0.9.0" {
		t.Fatalf("wrong user row: %+v %d %v", users, total, err)
	}
	if err := f.store.RecordError(ErrorReport{DeviceID: id, Version: "0.9.0", Code: "backup_failed"}); err != nil {
		t.Fatal(err)
	}
	v, err = f.store.analyticsOverviewAt(afterMidnight)
	if err != nil || len(v.VersionErrors) != 1 || v.VersionErrors[0].Reports != 1 {
		t.Fatalf("wrong error metric: %+v %v", v.VersionErrors, err)
	}
}

func TestAnalyticsConsentSurfaceAndActivationReceipt(t *testing.T) {
	f := fixture(t)
	f.ready(t)
	id := strings.Repeat("b", 64)
	bad := f.request("POST", "/api/telemetry", map[string]any{"device_id": id, "version": "0.8.0", "monthly_salary": 8000}, false)
	wantStatus(t, bad, 400) // Extra personal fields cannot enter the reporting API.
	wantStatus(t, f.request("GET", "/api/admin/analytics", nil, false), 401)
	wantStatus(t, f.request("GET", "/api/admin/users", nil, false), 401)
	wantStatus(t, f.request("POST", "/api/telemetry", TelemetryReport{DeviceID: id, Version: "0.8.0", TrialStartedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}, false), 204)
	o, token := f.pending(t, id)
	if err := f.store.AckActivation(o.ID, token); err == nil {
		t.Fatal("pending order accepted an activation receipt")
	}
	approved, err := f.store.Decide(o.ID, "approve", Decision{Revision: o.Revision, Confirmed: true, ReceivedCents: o.AmountCents, Receipt: "ANALYTICS-PAID-1"})
	if err != nil || approved.Status != "approved" {
		t.Fatalf("approval failed: %+v %v", approved, err)
	}
	if err := f.store.AckActivation(o.ID, "wrong"); err == nil {
		t.Fatal("wrong ticket acknowledged activation")
	}
	for i := 0; i < 2; i++ {
		// The public receipt requires the order's bearer token, not an admin cookie.
		if req := f.request("POST", "/api/orders/"+o.ID+"/activated", nil, false); req.Code == 200 {
			t.Fatal("receipt accepted without order token")
		}
		req := httptest.NewRequest("POST", "/api/orders/"+o.ID+"/activated", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, req)
		wantStatus(t, out, 200)
	}
	v, err := f.store.AnalyticsOverview()
	if err != nil || v.ApprovedOrders != 1 || v.ActivatedOrders != 1 || v.ActivationPercent != 100 || v.TrialToPaidPercent != 100 || v.TodayRevenueCents != o.AmountCents || v.TodayPaidOrders != 1 || v.KnownProDevices != 1 {
		t.Fatalf("wrong paid metrics: %+v %v", v, err)
	}
	response := f.request("GET", "/api/admin/analytics", nil, true)
	wantStatus(t, response, 200)
	var body AnalyticsOverview
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.DownloadClicks != nil {
		t.Fatalf("download count should remain unknown: %+v %v", body, err)
	}
	// Existing paid orders from before analytics was installed must not depress
	// the new client receipt rate, but still count as Pro entitlements.
	if _, err := f.store.db.Exec("UPDATE orders SET updated_at='2025-01-01T00:00:00Z' WHERE id=?", o.ID); err != nil {
		t.Fatal(err)
	}
	v, err = f.store.AnalyticsOverview()
	if err != nil || v.ApprovedOrders != 0 || v.ActivatedOrders != 0 || v.KnownProDevices != 1 {
		t.Fatalf("legacy order should be excluded only from receipt rate: %+v %v", v, err)
	}
}

func TestTrackedDownloadRedirectCountsClicks(t *testing.T) {
	f := fixture(t)
	h := NewHandler(f.store, HTTPOptions{DownloadURL: "https://example.com/workmate.zip"})
	for i := 0; i < 2; i++ {
		out := httptest.NewRecorder()
		h.ServeHTTP(out, httptest.NewRequest("GET", "/download", nil))
		if out.Code != 302 || out.Header().Get("Location") != "https://example.com/workmate.zip" {
			t.Fatalf("bad download redirect: %d %s", out.Code, out.Header().Get("Location"))
		}
	}
	v, err := f.store.AnalyticsOverview()
	if err != nil || v.DownloadClicks == nil || *v.DownloadClicks != 2 {
		t.Fatalf("wrong download clicks: %+v %v", v.DownloadClicks, err)
	}
}

func TestAnalyticsMigrationPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var started string
	if err := first.db.QueryRow("SELECT value FROM analytics_meta WHERE name='started_at'").Scan(&started); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("c", 64)
	if err := first.RecordTelemetry(TelemetryReport{DeviceID: id, Version: "0.9.0"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var again string
	if err := reopened.db.QueryRow("SELECT value FROM analytics_meta WHERE name='started_at'").Scan(&again); err != nil {
		t.Fatal(err)
	}
	v, err := reopened.AnalyticsOverview()
	if err != nil || again != started || v.TotalKnownDevices != 1 {
		t.Fatalf("analytics history changed after restart: %q %q %+v %v", started, again, v, err)
	}
}
