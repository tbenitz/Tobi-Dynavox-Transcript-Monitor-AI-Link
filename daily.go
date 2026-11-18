package main

import (
	"path/filepath"
	"time"
)

const (
	sourceFixed = "Single file"
	sourceDaily = "Daily files (YYYY-MM-DD.txt)"
)

var transcriptSources = []string{sourceFixed, sourceDaily}

func (s settings) transcriptPath(now time.Time) string {
	if s.Source != sourceDaily {
		return s.Path
	}
	if s.Folder == "" {
		return ""
	}
	return filepath.Join(s.Folder, now.Format(time.DateOnly)+".txt")
}

type transcriptMonitor struct {
	path           string
	privateThrough string
	reader         follower
}

func (m *transcriptMonitor) read(s settings, now time.Time) (string, error, bool) {
	path := s.transcriptPath(now)
	changed := path != m.path
	if changed {
		m.path, m.reader = path, follower{}
		if path != "" && s.Source == sourceDaily && now.Format(time.DateOnly) <= m.privateThrough {
			m.reader.resume(path)
		}
	}
	if path == "" {
		return "", nil, changed
	}
	text, err := m.reader.read(path)
	return text, err, changed
}

func (m *transcriptMonitor) resume(s settings, now time.Time) {
	m.path = s.transcriptPath(now)
	if s.Source == sourceDaily {
		m.privateThrough = max(m.privateThrough, now.Format(time.DateOnly))
	}
	if m.path == "" {
		m.reader = follower{}
		return
	}
	m.reader.resume(m.path)
}

func (m *transcriptMonitor) reload(s settings, now time.Time) {
	// Reset only the resolved current reader. Never clear a privacy cutoff.
	if m.reader.protected && m.path == s.transcriptPath(now) {
		m.resume(s, now)
	} else {
		m.path, m.reader = "", follower{}
	}
}
