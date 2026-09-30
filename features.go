package main

import "time"

type featureID string

const (
	featureBasics            featureID = "basics"
	featureRecords           featureID = "records"
	featureTimeline          featureID = "timeline"
	featureBackup            featureID = "backup"
	featureDailyCard         featureID = "daily-card"
	featureOffReminder       featureID = "off-reminder"
	featureLeave             featureID = "leave"
	featureLedgerExport      featureID = "ledger-export"
	featurePeriodReports     featureID = "period-reports"
	featureAdvancedReminders featureID = "advanced-reminders"
	featureHolidayConfig     featureID = "holiday-config"
)

type featureSpec struct {
	ID            featureID
	Name, Benefit string
	Pro           bool
}

// One catalog defines both access and the benefits shown before purchase.
var featureCatalog = []featureSpec{
	{featureBasics, "工资进度与桌面挂件", "看收入估算、下班倒计时，使用桌面或任务栏挂件。", false},
	{featureRecords, "加班计时与补记", "一键记录加班，在日历修正时长和补充备注。", false},
	{featureTimeline, "今日时间轴", "查看今天的工作安排和实际加班记录。", false},
	{featureBackup, "本地备份与恢复", "自己的记录随时备份，换电脑时可以迁移。", false},
	{featureDailyCard, "今日分享卡", "把今天的打工进度保存成图片。", false},
	{featureOffReminder, "基础下班提醒", "下班前和下班时提醒，减少盯着时钟。", false},
	{featureLeave, "年假、补休与请假管理", "记录已休和计划休假，查看年假估算余额。", true},
	{featureLedgerExport, "按月导出工时账本", "把每日工时、加班、休假和备注整理成 Excel 可打开的 CSV，省去月底抄表。", true},
	{featurePeriodReports, "月度与年度图文报告", "一键整理长期工时和加班记录，保存成图片。", true},
	{featureAdvancedReminders, "调休、假期与发薪日提醒", "提前知道明天是否调休上班、开始放假或临近发薪日。", true},
	{featureHolidayConfig, "自定义节假日配置", "导入和导出自己的节假日与调休日历。", true},
}

func findFeature(id featureID) (featureSpec, bool) {
	for _, f := range featureCatalog {
		if f.ID == id {
			return f, true
		}
	}
	return featureSpec{}, false
}
func canUseFeature(lm *LicenseManager, id featureID, now time.Time) bool {
	f, ok := findFeature(id)
	if !ok {
		return false
	}
	return !f.Pro || (lm != nil && lm.HasProAccess(now))
}
func reminderFeature(key string) featureID {
	switch key {
	case "off30", "off5", "off":
		return featureOffReminder
	default:
		return featureAdvancedReminders
	}
}
