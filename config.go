package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const builtin = "Built-in offline"

var engines = []string{builtin, "Qwen 0.5B (8 GB, fastest)", "Qwen 1.5B (8 GB, balanced)", "Qwen 3B (16 GB, balanced)", "Qwen 7B (16 GB, heavier)", "DeepSeek cloud", "Grok cloud"}
var models = []string{"", "qwen2.5:0.5b-instruct", "qwen2.5:1.5b-instruct", "qwen2.5:3b-instruct", "qwen2.5:7b-instruct", "deepseek-flash", "grok-4.3"}
var themes = []string{"Yellow on black", "Light", "TD Snap-inspired"}
var positions = []string{"Top of monitor", "Above keyboard (set height)", "Custom drag position"}
var modes = []string{quick, words, hybrid}
var rates = []string{"250 ms", "500 ms", "1000 ms"}
var timeouts = []string{"Off", "10 seconds", "15 seconds", "30 seconds"}
var cooldowns = []string{"Off", "250 ms", "500 ms", "1000 ms"}

type settings struct {
	Source, Folder                                                                         string
	Path, Unit, Hotkey, Mode, Injection, Position, Theme, CloseSide, Target, Engine, Model string
	Poll, Context, Timeout, Cooldown, Height, Font, Gap, KeyboardHeight                    int
	X, Y                                                                                   int
	HasPosition, LargeClose, Space, Punctuation, CloseAfter, Lock, Cloud                   bool
	DeepSeekSecret, GrokSecret                                                             string
}

func defaults() settings {
	return settings{Unit: "sentences", Hotkey: "F9", Mode: hybrid, Injection: "Simulated typing", Position: positions[1], Theme: themes[0], CloseSide: "Right", Poll: 500, Context: 2, Timeout: 15, Cooldown: 500, Height: 82, Font: 18, Gap: 10, KeyboardHeight: 310, LargeClose: true, Space: true, CloseAfter: true, Lock: true, Engine: builtin}
}
func normalize(s settings) settings {
	d := defaults()
	selectString := func(v, def string, list []string) string {
		if slices.Contains(list, v) {
			return v
		}
		return def
	}
	selectNumber := func(v, def int, list []int) int {
		if slices.Contains(list, v) {
			return v
		}
		return def
	}
	s.Unit = selectString(s.Unit, d.Unit, []string{"sentences", "words"})
	s.Mode = selectString(s.Mode, d.Mode, modes)
	s.Injection = selectString(s.Injection, d.Injection, []string{"Simulated typing", "Clipboard paste"})
	s.Position = selectString(s.Position, d.Position, positions)
	s.Theme = selectString(s.Theme, d.Theme, themes)
	s.CloseSide = selectString(s.CloseSide, d.CloseSide, []string{"Left", "Right"})
	s.Engine = selectString(s.Engine, d.Engine, engines)
	s.Poll = selectNumber(s.Poll, d.Poll, []int{250, 500, 1000})
	s.Timeout = selectNumber(s.Timeout, d.Timeout, []int{0, 10, 15, 30})
	s.Cooldown = selectNumber(s.Cooldown, d.Cooldown, []int{0, 250, 500, 1000})
	s.Context = min(max(s.Context, 1), 100)
	s.Height = min(max(s.Height, 52), 140)
	s.Font = min(max(s.Font, 14), 34)
	s.Gap = min(max(s.Gap, 4), 26)
	s.KeyboardHeight = min(max(s.KeyboardHeight, 0), 1000)
	if _, _, e := parseHotkey(s.Hotkey); e != nil {
		s.Hotkey = d.Hotkey
	}
	s.Path = strings.Trim(strings.TrimSpace(s.Path), "\"")
	s.Folder = strings.Trim(strings.TrimSpace(s.Folder), "\"")
	s.Source = selectString(s.Source, sourceFixed, transcriptSources)
	s.Target = strings.TrimSpace(s.Target)
	s.Model = strings.TrimSpace(s.Model)
	return s
}
func configPath() (string, error) {
	dir, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(dir, "AACLink", "settings.json"), nil
}
func readSettings() settings {
	s := defaults()
	if path, e := configPath(); e == nil {
		if b, e := os.ReadFile(path); e == nil {
			if json.Unmarshal(b, &s) != nil {
				s = defaults()
			}
		}
	}
	return normalize(s)
}
func saveSettings(s settings) error {
	p, e := configPath()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), "settings-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	e = enc.Encode(s)
	if c := f.Close(); e == nil {
		e = c
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}
func (s settings) endpoint() (provider, model, url string) {
	i := slices.Index(engines, s.Engine)
	if i < 1 {
		return "", "", ""
	}
	model = models[i]
	if s.Model != "" {
		model = s.Model
	}
	switch i {
	case 5:
		return "deepseek", model, "https://api.deepseek.com/chat/completions"
	case 6:
		return "grok", model, "https://api.x.ai/v1/chat/completions"
	default:
		return "ollama", model, "http://127.0.0.1:11434/api/chat"
	}
}
