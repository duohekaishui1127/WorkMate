package main

import "sort"

// Use only the gap after the app buttons and before the notification area.
// No Explorer windows are resized to make room for the widget.
func taskbarEmbedRect(bar, buttons, tray desktopRect, occupied []desktopRect, width int) (desktopRect, bool) {
	if bar.width() <= bar.height() || bar.height() < 24 || buttons.width() <= 0 || tray.width() <= 0 {
		return desktopRect{}, false
	}
	left, right := max(bar.Left, buttons.Right)+8, min(bar.Right, tray.Left)-8
	if right-left < 160 {
		return desktopRect{}, false
	}
	blocks := append([]desktopRect(nil), occupied...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Left < blocks[j].Left })
	start, bestLeft, bestRight := left, 0, 0
	for _, r := range blocks {
		if r.Bottom <= bar.Top || r.Top >= bar.Bottom || r.Right <= left || r.Left >= right {
			continue
		}
		end := min(right, r.Left-8)
		if end-start >= 160 {
			bestLeft, bestRight = start, end
		}
		start = max(start, r.Right+8)
	}
	if right-start >= 160 {
		bestLeft, bestRight = start, right
	}
	if bestRight-bestLeft < 160 {
		return desktopRect{}, false
	}
	width = min(max(160, width), bestRight-bestLeft)
	return desktopRect{bestRight - width, bar.Top + 4, bestRight, bar.Bottom - 4}, true
}

func taskbarWidgetWidth(size string) int {
	switch size {
	case "Small":
		return 180
	case "Large":
		return 280
	default:
		return 240
	}
}
