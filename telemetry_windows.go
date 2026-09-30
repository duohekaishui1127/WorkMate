//go:build windows

package main

import (
	"time"
)

// Reporting is off until the user explicitly accepts it. A declined choice is
// remembered, and Settings can change it at any time.
var telemetryState struct {
	busy   bool
	next   time.Time
	result chan error
}

func requestTelemetryConsent() {
	if onlinePurchase == nil || onlinePurchase.client == nil || app.store.Settings.TelemetryAsked {
		return
	}
	choice := msgBox(app.main, "设备使用统计", "是否允许发送设备使用统计？\n\n仅发送设备哈希码（可关联同设备订单）、程序版本、每天是否使用、试用和 Pro 状态，以及固定错误类别；不发送工资、工时、请假、截图或窗口内容。\n\n拒绝不影响使用；之后可在设置中随时更改。", MB_YESNO|MB_ICONINFORMATION)
	next := app.store.Settings
	next.TelemetryAsked = true
	next.TelemetryConsent = choice == IDYES
	if err := app.store.UpdateSettings(next, time.Now()); err != nil {
		// Failing to persist consent must never cause an upload.
		if choice == IDYES {
			msgBox(app.main, "统计设置未保存", "设置未能保存，此次不会发送统计；可稍后在设置中重新开启。", MB_OK|MB_ICONWARNING)
		}
	}
}

func syncTelemetry(now time.Time) {
	if telemetryState.result != nil {
		select {
		case err := <-telemetryState.result:
			telemetryState.busy = false
			if err != nil {
				telemetryState.next = now.Add(10 * time.Minute)
			} else {
				zone := time.FixedZone("China Standard Time", 8*60*60)
				local := now.In(zone)
				telemetryState.next = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, zone)
			}
		default:
		}
	}
	if !app.store.Settings.TelemetryConsent || onlinePurchase == nil || onlinePurchase.client == nil || telemetryState.busy || now.Before(telemetryState.next) {
		return
	}
	payload := map[string]any{
		"device_id":        app.license.deviceID,
		"version":          appVersion,
		"trial_started_at": app.license.trialStart.UTC().Format(time.RFC3339),
		"pro":              app.license.IsPro(),
	}
	telemetryState.busy = true
	if telemetryState.result == nil {
		telemetryState.result = make(chan error, 1)
	}
	client, result := onlinePurchase.client, telemetryState.result
	go func() { result <- client.sendTelemetry("/api/telemetry", payload) }()
}

func reportTelemetryError(code string) {
	if !app.store.Settings.TelemetryConsent || onlinePurchase == nil || onlinePurchase.client == nil {
		return
	}
	client := onlinePurchase.client
	payload := map[string]any{"device_id": app.license.deviceID, "version": appVersion, "code": code}
	go func() { _ = client.sendTelemetry("/api/telemetry/error", payload) }()
}
