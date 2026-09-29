package main

import (
	"os"
	"testing"
	"time"
)

func TestFloatingDockEdgesAndHover(t *testing.T) {
	work := desktopRect{0, 0, 1920, 1040}
	for _, tc := range []struct {
		name string
		r    desktopRect
		edge dockEdge
		x, y int
	}{
		{"left", desktopRect{16, 200, 356, 302}, dockLeft, 6, 240},
		{"right", desktopRect{1564, 200, 1904, 302}, dockRight, 1913, 240},
		{"top", desktopRect{500, 16, 840, 118}, dockTop, 600, 6},
		{"bottom", desktopRect{500, 922, 840, 1024}, dockBottom, 600, 1033},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var d floatingDock
			d.place(tc.r, work, true)
			if d.edge != tc.edge {
				t.Fatalf("edge=%v, want %v", d.edge, tc.edge)
			}
			normal := d.normal
			now := testDate("2026-09-29 10:00")
			if d.update(now, 1000, 500) || d.update(now.Add(floatingDockDelay-time.Millisecond), 1000, 500) {
				t.Fatal("collapsed before the delay")
			}
			if !d.update(now.Add(floatingDockDelay), 1000, 500) || !d.collapsed {
				t.Fatal("did not collapse after the delay")
			}
			strip := d.visibleRect()
			if strip.Left < work.Left || strip.Top < work.Top || strip.Right > work.Right || strip.Bottom > work.Bottom {
				t.Fatalf("collapsed strip escaped the work area: %+v", strip)
			}
			if tc.edge == dockLeft || tc.edge == dockRight {
				if strip.width() != floatingDockStrip || strip.height() != normal.height() {
					t.Fatal("invalid vertical strip")
				}
			} else if strip.height() != floatingDockStrip || strip.width() != normal.width() {
				t.Fatal("invalid horizontal strip")
			}
			if d.update(now.Add(time.Second), 1000, 500) {
				t.Fatal("expanded away from the strip")
			}
			if !d.update(now.Add(2*time.Second), tc.x, tc.y) || d.collapsed || d.visibleRect() != normal {
				t.Fatal("hover did not restore the full window")
			}
		})
	}
}

func TestFloatingDockThresholdAndUndockedWindow(t *testing.T) {
	work := desktopRect{0, 0, 1920, 1040}
	var d floatingDock
	d.place(desktopRect{17, 200, 357, 302}, work, true)
	if d.edge != dockNone {
		t.Fatal("snapped beyond the threshold")
	}
	now := testDate("2026-09-29 10:00")
	d.update(now, 1000, 500)
	if d.update(now.Add(time.Hour), 1000, 500) || d.collapsed {
		t.Fatal("undocked window collapsed")
	}
	d.place(desktopRect{0, 0, 340, 102}, work, true)
	if d.edge != dockLeft {
		t.Fatal("corner docking is not deterministic")
	}
}

func TestFloatingDockCancelAndDrag(t *testing.T) {
	var d floatingDock
	d.place(desktopRect{0, 200, 340, 302}, desktopRect{0, 0, 1920, 1040}, true)
	now := testDate("2026-09-29 10:00")
	d.update(now, 1000, 500)
	d.update(now.Add(500*time.Millisecond), 100, 250)
	if !d.outsideSince.IsZero() {
		t.Fatal("entering the widget did not cancel hiding")
	}
	if d.update(now.Add(time.Second), 1000, 500) {
		t.Fatal("old leave time reused")
	}
	d.dragging = true
	if d.update(now.Add(time.Hour), 1000, 500) || d.collapsed {
		t.Fatal("collapsed during dragging")
	}
	d.dragging = false
	d.update(now.Add(2*time.Hour), 1000, 500)
	if !d.update(now.Add(2*time.Hour+floatingDockDelay), 1000, 500) {
		t.Fatal("hiding did not resume after dragging")
	}
	d.place(d.normal, d.work, false)
	if d.collapsed || d.edge != dockNone || d.visibleRect().width() != 340 {
		t.Fatal("disabling auto-hide did not expand")
	}
}

func TestFloatingDockNegativeMonitorAndOffscreenRestore(t *testing.T) {
	work := desktopRect{-1920, -200, 0, 840}
	var d floatingDock
	d.place(desktopRect{-345, 200, -5, 302}, work, true)
	if d.edge != dockRight || d.normal.Left != -340 {
		t.Fatalf("negative monitor coordinates lost: %+v", d)
	}
	now := testDate("2026-09-29 10:00")
	d.update(now, -1000, 500)
	d.update(now.Add(time.Second), -1000, 500)
	if !d.collapsed {
		t.Fatal("did not collapse on the negative monitor")
	}
	if d.update(now.Add(2*time.Second), 1, 250) {
		t.Fatal("hover on adjacent monitor expanded the window")
	}
	if !d.update(now.Add(3*time.Second), -1, 250) {
		t.Fatal("hover inside the docked monitor did not expand")
	}
	d.place(desktopRect{3000, 3000, 3340, 3102}, work, false)
	if d.normal != (desktopRect{-340, 738, 0, 840}) {
		t.Fatalf("offscreen position not recovered: %+v", d.normal)
	}
}

func TestFloatingSettingsMigrationAndPersistence(t *testing.T) {
	s := testStore(t)
	if s.Settings.FloatingAutoHide || !s.Settings.FloatingTopmost {
		t.Fatal("unexpected defaults")
	}
	if err := os.WriteFile(s.SettingsPath, []byte(`{"FloatingLeft":-900,"FloatingTop":200}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStore()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Settings.FloatingTopmost || loaded.Settings.FloatingAutoHide || loaded.Settings.FloatingLeft != -900 {
		t.Fatal("legacy settings did not preserve defaults and coordinates")
	}
	next := loaded.Settings
	next.FloatingAutoHide, next.FloatingTopmost = true, false
	if err := loaded.UpdateSettings(next, time.Now()); err != nil {
		t.Fatal(err)
	}
	loaded, err = newStore()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Settings.FloatingAutoHide || loaded.Settings.FloatingTopmost {
		t.Fatal("new settings not persisted")
	}
}
