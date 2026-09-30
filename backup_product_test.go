package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFreeBackupRoundTripAndRejectedRestoreKeepsCurrentData(t *testing.T) {
	s := testStore(t)
	s.Records = []DailyRecord{{Date: "2026-09-28", WorkMinutes: 120}}
	s.Leaves = []LeaveRecord{{Date: "2026-09-28", AnnualDays: 0.5, Note: "保留年假"}}
	path := filepath.Join(t.TempDir(), "backup.zip")
	if err := s.CreateBackup(path); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 6 {
		t.Fatalf("backup has %d files, want 6", len(archive.File))
	}
	archive.Close()
	s.Records[0].WorkMinutes = 240
	s.Leaves[0].Note = "现有记录"
	if err = s.SaveAll(); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"settings.json", "records.json", "leave-records.json", "timeline.json", "holiday-calendar.json", "state.json"} {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		data := []byte("[]")
		if name == "settings.json" || name == "state.json" {
			data = []byte("{}")
		}
		if name == "records.json" {
			data = []byte("123")
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bad, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(bad); err == nil {
		t.Fatal("structurally invalid backup accepted")
	}
	if s.Records[0].WorkMinutes != 240 || s.Leaves[0].Note != "现有记录" {
		t.Fatal("rejected restore changed live data")
	}
	if err = s.RestoreBackup(path); err != nil {
		t.Fatal(err)
	}
	if s.Records[0].WorkMinutes != 120 || s.Leaves[0].AnnualDays != 0.5 || s.Leaves[0].Note != "保留年假" {
		t.Fatal("backup did not restore records and paid leave")
	}
	matches, err := filepath.Glob(filepath.Join(s.BackupDir, "before-restore-*.zip"))
	if err != nil || len(matches) != 1 {
		t.Fatal("safety copy not retained", err)
	}
	safety, err := zip.OpenReader(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer safety.Close()
	for _, f := range safety.File {
		if f.Name != "records.json" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("240")) {
			t.Fatal("safety backup missed pre-restore data")
		}
		return
	}
	t.Fatal("safety backup has no records")
}

func TestBackupWriteFailurePreservesPreviousArchive(t *testing.T) {
	s := testStore(t)
	path := filepath.Join(t.TempDir(), "existing.zip")
	original := []byte("previous backup stays")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	s.SettingsPath = filepath.Join(t.TempDir(), "missing", "settings.json")
	if err := s.CreateBackup(path); err == nil {
		t.Fatal("backup succeeded although current records could not be saved")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatal("failed backup replaced prior archive", err)
	}
}
