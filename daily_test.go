package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyPathUsesLocalDateAndKeepsLegacyFixedPath(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 11, 19, 6, 59, 59, 0, time.UTC).In(time.FixedZone("Mountain", -7*3600))
	old := settings{Path: filepath.Join(dir, "live.txt")}
	if got := normalize(old).transcriptPath(clock); got != old.Path {
		t.Fatal(got)
	}
	s := settings{Source: sourceDaily, Folder: dir, Path: old.Path}
	if got := s.transcriptPath(clock); got != filepath.Join(dir, "2026-11-18.txt") {
		t.Fatal(got)
	}
	if got := s.transcriptPath(clock.Add(time.Second)); got != filepath.Join(dir, "2026-11-19.txt") {
		t.Fatal(got)
	}
	s.Folder = ""
	if got := s.transcriptPath(clock); got != "" {
		t.Fatalf("daily mode fell back to fixed file: %q", got)
	}
}

func TestDailyRolloverWaitsForNewFile(t *testing.T) {
	s := settings{Source: sourceDaily, Folder: t.TempDir()}
	now := time.Date(2026, 11, 18, 23, 59, 59, 0, time.Local)
	then := now.Add(time.Second)
	var m transcriptMonitor
	if err := os.WriteFile(s.transcriptPath(now), []byte("Yesterday"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err, changed := m.read(s, now); got != "Yesterday" || err != nil || !changed {
		t.Fatal(got, err, changed)
	}
	if got, err, changed := m.read(s, then); got != "" || !os.IsNotExist(err) || !changed {
		t.Fatal(got, err, changed)
	}
	if err := os.WriteFile(s.transcriptPath(then), []byte("Today"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err, changed := m.read(s, then); got != "Today" || err != nil || changed {
		t.Fatal(got, err, changed)
	}
}

func TestDailyPrivacyPauseAcrossMidnight(t *testing.T) {
	s := settings{Source: sourceDaily, Folder: t.TempDir()}
	now := time.Date(2026, 11, 18, 23, 59, 0, 0, time.Local)
	then := now.Add(time.Minute)
	write := func(at time.Time, v string) {
		t.Helper()
		if e := os.WriteFile(s.transcriptPath(at), []byte(v), 0600); e != nil {
			t.Fatal(e)
		}
	}
	var m transcriptMonitor
	write(now, "old day")
	_, _, _ = m.read(s, now)
	write(then, "private\n")
	m.resume(s, then)
	if got, err, _ := m.read(s, then); got != "" || err != nil {
		t.Fatal(got, err)
	}
	write(then, "private\npublic")
	if got, err, _ := m.read(s, then); got != "public" || err != nil {
		t.Fatal(got, err)
	}
	tomorrow := then.AddDate(0, 0, 1)
	write(tomorrow, "first speech")
	if got, err, changed := m.read(s, tomorrow); got != "first speech" || err != nil || !changed {
		t.Fatal(got, err, changed)
	}
	if got, err, changed := m.read(s, then); got != "" || err != nil || !changed {
		t.Fatal("clock rollback exposed private text", got, err, changed)
	}
}
