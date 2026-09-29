package main

import (
	"testing"
	"time"
)

func TestTaskbarLayoutKeepsButtonsAndTrayClear(t *testing.T) {
	bar := desktopRect{0, 0, 2560, 48}
	buttons, tray := desktopRect{815, 0, 1348, 48}, desktopRect{2162, 0, 2560, 48}
	r, ok := taskbarEmbedRect(bar, buttons, tray, nil, 240)
	if !ok || r != (desktopRect{1914, 4, 2154, 44}) {
		t.Fatalf("unexpected taskbar slot: %+v, ok=%v", r, ok)
	}
	if r.Left < buttons.Right+8 || r.Right > tray.Left-8 || r.Top < bar.Top || r.Bottom > bar.Bottom {
		t.Fatal("widget overlaps shell controls")
	}
	// Another small toolbar occupies the right side of the available gap.
	r, ok = taskbarEmbedRect(bar, buttons, tray, []desktopRect{{1950, 0, 2150, 48}}, 240)
	if !ok || r.Right > 1942 || r.Left < buttons.Right+8 {
		t.Fatalf("occupied controls were covered: %+v", r)
	}
}

func TestTaskbarLayoutNarrowSpaceAndUnsupportedBars(t *testing.T) {
	bar := desktopRect{0, 0, 1000, 48}
	buttons, tray := desktopRect{0, 0, 610, 48}, desktopRect{800, 0, 1000, 48}
	r, ok := taskbarEmbedRect(bar, buttons, tray, nil, 280)
	if !ok || r.width() != 174 {
		t.Fatalf("widget did not adapt to available width: %+v", r)
	}
	buttons.Right = 630
	if _, ok := taskbarEmbedRect(bar, buttons, tray, nil, 240); ok {
		t.Fatal("accepted less than the minimum safe space")
	}
	for _, tc := range []struct{ bar, buttons, tray desktopRect }{
		{desktopRect{0, 0, 48, 1000}, buttons, tray},
		{desktopRect{0, 0, 1000, 16}, buttons, tray},
		{bar, desktopRect{}, tray},
		{bar, buttons, desktopRect{}},
		{bar, desktopRect{0, 0, 800, 48}, tray},
	} {
		if _, ok := taskbarEmbedRect(tc.bar, tc.buttons, tc.tray, nil, 240); ok {
			t.Fatal("unsafe or unknown taskbar layout accepted")
		}
	}
}

func TestTaskbarLayoutOverlappingControls(t *testing.T) {
	bar := desktopRect{0, 0, 1600, 48}
	buttons, tray := desktopRect{0, 0, 600, 48}, desktopRect{1500, 0, 1600, 48}
	occupied := []desktopRect{{1200, 0, 1400, 48}, {700, 0, 900, 48}, {800, 0, 1000, 48}, {0, 100, 1600, 200}}
	r, ok := taskbarEmbedRect(bar, buttons, tray, occupied, 240)
	if !ok || r != (desktopRect{1008, 4, 1192, 44}) {
		t.Fatalf("merged occupied spans incorrectly: %+v, ok=%v", r, ok)
	}
}

func TestTaskbarModeSettingsRoundTrip(t *testing.T) {
	s := testStore(t)
	for _, mode := range []string{"Screen", "Taskbar", "TaskbarEmbed"} {
		next := s.Settings
		next.FloatingMode = mode
		if err := s.UpdateSettings(next, time.Now()); err != nil {
			t.Fatal(err)
		}
		loaded, err := newStore()
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Settings.FloatingMode != mode {
			t.Fatalf("mode %s not persisted", mode)
		}
	}
}
