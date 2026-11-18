package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var tabs = []string{"1  Transcript", "2  Triggers", "3  Suggestions & AI", "4  Appearance"}

type settingsWindow struct {
	hwnd, font, header uintptr
	buttons            [4]uintptr
	items              map[string]uintptr
	pageHandles        []uintptr
	page               int
	record             bool
	s                  settings
	deep, grok         string
}

func (a *app) openSettings() {
	a.hide()
	if a.settings != nil {
		u("ShowWindow", a.settings.hwnd, 9)
		u("SetForegroundWindow", a.settings.hwnd)
		return
	}
	s := &settingsWindow{s: a.s, items: map[string]uintptr{}}
	s.deep, _ = protect(s.s.DeepSeekSecret, false)
	s.grok, _ = protect(s.s.GrokSecret, false)
	work := monitorWork(foreground().root)
	width, height := px(734), px(575)
	s.hwnd = create(0, settingsClass, "AAC Link · Settings", 0x00cb0000, int(work.L)+(work.width()-width)/2, int(work.T)+(work.height()-height)/2, width, height, 0, 0, a.instance)
	if s.hwnd == 0 {
		return
	}
	a.settings = s
	s.font = font(16, 400)
	s.header = font(23, 600)
	h := control(a, s.hwnd, "STATIC", "AAC Link settings", 0, 0)
	send(h, 0x30, s.header, 1)
	place(h, 20*uiScale/96, px(12), px(670), px(33))
	for i, name := range tabs {
		s.buttons[i] = s.make(a, "BUTTON", name, 0x10000, 101+i, 20+172*i, 53, 165, 35, false)
	}
	s.make(a, "BUTTON", "Preview overlay", 0x10000, 110, 20, 480, 159, 40, false)
	s.make(a, "BUTTON", "Cancel", 0x10000, 111, 481, 480, 100, 40, false)
	s.make(a, "BUTTON", "Save settings", 0x10001, 112, 587, 480, 118, 40, false)
	s.openPage(a, 0)
	u("ShowWindow", s.hwnd, 5)
	u("SetForegroundWindow", s.hwnd)
}
func (s *settingsWindow) make(a *app, class, text string, style uintptr, id, x, y, w, h int, page bool) uintptr {
	hwnd := control(a, s.hwnd, class, text, style, id)
	send(hwnd, 0x30, s.font, 1)
	place(hwnd, px(x), px(y), px(w), px(h))
	if page {
		s.pageHandles = append(s.pageHandles, hwnd)
	}
	return hwnd
}
func (s *settingsWindow) label(a *app, text string, x, y, w int) {
	s.make(a, "STATIC", text, 0, 0, x, y, w, 25, true)
}
func (s *settingsWindow) note(a *app, text string, y, h int) {
	s.make(a, "STATIC", text, 0, 0, 22, y, 678, h, true)
}
func (s *settingsWindow) edit(a *app, key, label, val string, y, width int, style uintptr) {
	s.label(a, label, 22, y+4, 263)
	hwnd := s.make(a, "EDIT", val, 0x00810080|style, 0, 288, y, width, 31, true)
	send(hwnd, 0xc5, 4096, 0)
	s.items[key] = hwnd
}
func (s *settingsWindow) combo(a *app, key, label, value string, y int, options []string) {
	s.label(a, label, 22, y+4, 263)
	h := s.make(a, "COMBOBOX", "", 0x00210043, 0, 288, y, 413, 360, true)
	for _, o := range options {
		send(h, 0x143, 0, ptr(wp(o)))
	}
	send(h, 0x14e, max(0, slices.Index(options, value)), 0)
	s.items[key] = h
}
func (s *settingsWindow) check(a *app, key, label string, y int, value bool) {
	h := s.make(a, "BUTTON", label, 0x10003, 0, 22, y, 680, 27, true)
	if value {
		send(h, 0xf1, 1, 0)
	}
	s.items[key] = h
}
func (s *settingsWindow) track(a *app, key, label string, y, v, lo, hi int) {
	s.label(a, label, 22, y+5, 263)
	h := s.make(a, "msctls_trackbar32", "", 0x10000|0x10, 0, 288, y, 413, 34, true)
	send(h, 0x407, 0, lo)
	send(h, 0x408, 1, hi)
	send(h, 0x405, 1, v)
	s.items[key] = h
	s.items[key+"-label"] = s.make(a, "STATIC", strconv.Itoa(v), 0, 0, 662, y+5, 40, 24, true)
	place(h, px(288), px(y), px(360), px(34))
}
func (s *settingsWindow) openPage(a *app, page int) {
	for _, h := range s.pageHandles {
		u("DestroyWindow", h)
	}
	s.items = map[string]uintptr{}
	s.pageHandles = nil
	s.page = page
	s.record = false
	for i, h := range s.buttons {
		u("EnableWindow", h, map[bool]int{true: 0, false: 1}[i == page])
	}
	c := s.s
	switch page {
	case 0:
		s.combo(a, "source", "Transcript source", c.Source, 105, transcriptSources)
		s.edit(a, "path", "Single transcript .txt file", c.Path, 147, 324, 0)
		s.make(a, "BUTTON", "Browse…", 0x10000, 113, 618, 147, 82, 31, true)
		s.edit(a, "folder", "Daily transcript folder", c.Folder, 189, 324, 0)
		s.make(a, "BUTTON", "Browse…", 0x10000, 115, 618, 189, 82, 31, true)
		s.combo(a, "poll", "Check for changes", fmt.Sprintf("%d ms", c.Poll), 231, rates)
		s.combo(a, "unit", "Keep recent context in", c.Unit, 273, []string{"sentences", "words"})
		s.edit(a, "context", "Number of sentences or words", strconv.Itoa(c.Context), 315, 125, 0x2000)
		s.note(a, "Daily files mode opens today's file in that folder (for example, 2026-11-18.txt). It switches at midnight using this PC's local date and waits if the new file hasn't been created yet.", 367, 77)

	case 1:
		s.edit(a, "hotkey", "Summon / dismiss shortcut", c.Hotkey, 105, 273, 0)
		s.make(a, "BUTTON", "Record shortcut", 0x10000, 114, 568, 105, 133, 31, true)
		s.combo(a, "timeout", "Auto-hide if untouched", formatDuration(c.Timeout, "seconds"), 148, timeouts)
		s.combo(a, "delay", "Reopen cooldown", formatDuration(c.Cooldown, "ms"), 189, cooldowns)
		s.combo(a, "side", "Dismiss button side", c.CloseSide, 230, []string{"Left", "Right"})
		s.check(a, "large", "Extra-large red Dismiss button", 272, c.LargeClose)
		s.edit(a, "target", "AAC window title filter (optional)", c.Target, 314, 413, 0)
		s.note(a, "Set a title filter, such as TD Snap, to keep the shortcut from inserting into unrelated apps. AAC button macros can run AACLink.exe --toggle while the message bar still has focus.", 366, 67)
	case 2:
		s.combo(a, "mode", "Suggestion mode", c.Mode, 101, modes)
		s.combo(a, "inject", "Text insertion", c.Injection, 136, []string{"Simulated typing", "Clipboard paste"})
		s.check(a, "space", "Add trailing space after a word", 173, c.Space)
		s.check(a, "punct", "Add a period to replies without punctuation", 202, c.Punctuation)
		s.check(a, "hide", "Dismiss after choosing a suggestion", 231, c.CloseAfter)
		s.combo(a, "engine", "Prediction engine", c.Engine, 263, engines)
		s.edit(a, "model", "Model ID override (optional)", c.Model, 300, 413, 0)
		s.edit(a, "deep", "DeepSeek API key", s.deep, 336, 413, 0x20)
		s.edit(a, "grok", "Grok / xAI API key", s.grok, 372, 413, 0x20)
		s.check(a, "cloud", "Allow DeepSeek or Grok to receive recent speech and the readable draft", 407, c.Cloud)
		s.note(a, "Ollama models require setup once. Cloud is off by default. Paste replaces the clipboard.", 443, 31)
	case 3:
		s.combo(a, "position", "Overlay position", c.Position, 103, positions)
		s.edit(a, "keyboard", "Keyboard height in logical pixels", strconv.Itoa(c.KeyboardHeight), 143, 125, 0x2000)
		s.check(a, "lock", "Lock custom position", 180, c.Lock)
		s.combo(a, "theme", "High-contrast theme", c.Theme, 214, themes)
		s.track(a, "height", "Button height", 254, c.Height, 52, 140)
		s.track(a, "font", "Button text size", 291, c.Font, 14, 34)
		s.track(a, "gap", "Gap between buttons", 328, c.Gap, 4, 26)
		s.note(a, "For a custom position, unlock and save, then drag the grip at the top right of the overlay. Use Preview overlay after saving to adjust placement without sending text.", 382, 69)
	}
}
func formatDuration(n int, unit string) string {
	if n == 0 {
		return "Off"
	}
	return fmt.Sprintf("%d %s", n, unit)
}
func (s *settingsWindow) capture() {
	t := func(key string) string { return textWin(s.items[key]) }
	chosen := func(key string, options []string) string {
		i := int(send(s.items[key], 0x147, 0, 0))
		if i >= 0 && i < len(options) {
			return options[i]
		}
		return options[0]
	}
	n := func(k string, v int) int {
		i, e := strconv.Atoi(t(k))
		if e != nil {
			return v
		}
		return i
	}
	selected := func(k string) bool { return send(s.items[k], 0xf0, 0, 0) == 1 }
	duration := func(k string, opts []string) int { i, _ := strconv.Atoi(strings.Fields(chosen(k, opts))[0]); return i }
	p := &s.s
	switch s.page {
	case 0:
		p.Source, p.Folder = chosen("source", transcriptSources), t("folder")
		p.Path, p.Unit, p.Context, p.Poll = t("path"), chosen("unit", []string{"sentences", "words"}), n("context", p.Context), duration("poll", rates)
	case 1:
		p.Hotkey, p.Timeout, p.Cooldown, p.CloseSide, p.LargeClose, p.Target = t("hotkey"), duration("timeout", timeouts), duration("delay", cooldowns), chosen("side", []string{"Left", "Right"}), selected("large"), t("target")
	case 2:
		p.Mode = chosen("mode", modes)
		p.Injection = chosen("inject", []string{"Simulated typing", "Clipboard paste"})
		p.Engine = chosen("engine", engines)
		p.Space, p.Punctuation, p.CloseAfter, p.Cloud = selected("space"), selected("punct"), selected("hide"), selected("cloud")
		p.Model, s.deep, s.grok = t("model"), strings.TrimSpace(t("deep")), strings.TrimSpace(t("grok"))
	case 3:
		p.Position, p.Theme, p.Lock = chosen("position", positions), chosen("theme", themes), selected("lock")
		p.KeyboardHeight = n("keyboard", p.KeyboardHeight)
		p.Height, p.Font, p.Gap = int(send(s.items["height"], 0x400, 0, 0)), int(send(s.items["font"], 0x400, 0, 0)), int(send(s.items["gap"], 0x400, 0, 0))
	}
}

type openFilename struct {
	Size                      uint32
	Owner, Instance           uintptr
	Filter, Custom            *uint16
	MaxCustom, Index          uint32
	File                      *uint16
	MaxFile                   uint32
	TitleFile                 *uint16
	MaxTitle                  uint32
	Dir, Title                *uint16
	Flags                     uint32
	FileOffset, FileExtension uint16
	Ext                       *uint16
	Data, Hook                uintptr
	Template                  *uint16
	Reserved                  uintptr
	ReservedInt, FlagsEx      uint32
}

func (s *settingsWindow) browse() {
	b := make([]uint16, 4096)
	fill16(b, textWin(s.items["path"]))
	filter := utf16.Encode([]rune("Transcript text (*.txt)\x00*.txt\x00All files\x00*.*\x00\x00"))
	of := openFilename{Owner: s.hwnd, Filter: &filter[0], Index: 1, File: &b[0], MaxFile: uint32(len(b)), Title: wp("Select live transcript"), Flags: 0x1000 | 0x800 | 0x8 | 0x80000}
	of.Size = uint32(unsafe.Sizeof(of))
	if win(dialog, "GetOpenFileNameW", ptr(&of)) != 0 {
		i := slices.Index(b, uint16(0))
		if i < 0 {
			i = len(b)
		}
		setText(s.items["path"], string(utf16.Decode(b[:i])))
	}
}

type browseInfo struct {
	owner, root     uintptr
	display, title  *uint16
	flags           uint32
	callback, param uintptr
	image           int32
}

func (s *settingsWindow) browseFolder() {
	ole := syscall.NewLazyDLL("ole32.dll")
	hr := int32(win(ole, "CoInitializeEx", 0, 2))
	flags := uint32(1) // BIF_RETURNONLYFSDIRS
	if hr >= 0 {
		defer win(ole, "CoUninitialize")
		flags |= 0x40 | 0x10 | 0x200 // Modern dialog, edit field, no New Folder button.
	}
	var display, path [260]uint16
	details := browseInfo{owner: s.hwnd, display: &display[0], title: wp("Select the folder containing the dated transcript files"), flags: flags}
	item := win(shell, "SHBrowseForFolderW", ptr(&details))
	if item == 0 {
		return
	}
	defer win(ole, "CoTaskMemFree", item)
	if win(shell, "SHGetPathFromIDListW", item, ptr(&path[0])) != 0 {
		end := slices.Index(path[:], uint16(0))
		if end >= 0 {
			setText(s.items["folder"], string(utf16.Decode(path[:end])))
		}
	}
}

func (a *app) persist() {
	s := a.settings
	s.capture()
	if _, _, err := parseHotkey(s.s.Hotkey); err != nil {
		msg(s.hwnd, "Check shortcut", err.Error())
		return
	}
	next := normalize(s.s)
	next.X, next.Y, next.HasPosition = a.s.X, a.s.Y, a.s.HasPosition
	var e error
	next.DeepSeekSecret, e = protect(s.deep, true)
	if e == nil {
		next.GrokSecret, e = protect(s.grok, true)
	}
	if e != nil {
		msg(s.hwnd, "Could not save API keys", e.Error())
		return
	}
	changedKey := !a.hotkey || !strings.EqualFold(a.s.Hotkey, next.Hotkey)
	if changedKey {
		if e = a.setHotkey(next.Hotkey); e != nil {
			_ = a.setHotkey(a.s.Hotkey)
			msg(s.hwnd, "Shortcut not saved", e.Error())
			return
		}
	}
	if e = saveSettings(next); e != nil {
		if changedKey {
			_ = a.setHotkey(a.s.Hotkey)
		}
		msg(s.hwnd, "Settings not saved", e.Error())
		return
	}
	sourceChanged := next.Source != a.s.Source || (next.Source == sourceDaily && next.Folder != a.s.Folder) || (next.Source != sourceDaily && next.Path != a.s.Path)
	a.hide()
	a.s = next
	a.updateFonts()
	if sourceChanged && !a.paused {
		a.watch = transcriptMonitor{}
		a.transcript = ""
	}
	a.startPolling()
	a.poll()
	u("DestroyWindow", s.hwnd)
}
func (a *app) recordMessage(m windowMessage) bool {
	s := a.settings
	if s == nil || !s.record || m.Msg != 0x100 && m.Msg != 0x104 {
		return false
	}
	if m.Hwnd != s.hwnd && u("IsChild", s.hwnd, m.Hwnd) == 0 {
		return false
	}
	vk := m.W
	if vk == 16 || vk == 17 || vk == 18 || vk == 91 || vk == 92 {
		return true
	}
	var label []string
	for i, k := range []uintptr{17, 18, 16, 91} {
		if u("GetAsyncKeyState", k)&0x8000 != 0 || (k == 91 && u("GetAsyncKeyState", 92)&0x8000 != 0) {
			label = append(label, []string{"Ctrl", "Alt", "Shift", "Win"}[i])
		}
	}
	if vk >= 112 && vk <= 135 {
		label = append(label, fmt.Sprintf("F%d", vk-111))
	} else if vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9' {
		label = append(label, string(rune(vk)))
	} else {
		label = append(label, map[uintptr]string{32: "Space", 8: "Backspace", 45: "Insert", 46: "Delete", 36: "Home", 35: "End", 33: "PageUp", 34: "PageDown"}[vk])
	}
	s.record = false
	u("SetWindowTextW", u("GetDlgItem", s.hwnd, 114), ptr(wp("Record shortcut")))
	if vk == 27 {
		return true
	}
	if value := strings.Join(label, "+"); len(label) > 0 {
		if _, _, e := parseHotkey(value); e == nil {
			setText(s.items["hotkey"], value)
		} else {
			msg(s.hwnd, "Shortcut not recorded", e.Error())
		}
	}
	return true
}
func settingsWnd(hwnd, message, w, l uintptr) uintptr {
	a := appState
	if a == nil || a.settings == nil {
		return u("DefWindowProcW", hwnd, message, w, l)
	}
	s := a.settings
	switch message {
	case 0x0111:
		id := uint16(w)
		if w>>16 != 0 {
			break
		}
		switch id {
		case 101, 102, 103, 104:
			s.capture()
			s.openPage(a, int(id)-101)
			return 0
		case 110:
			if a.visible {
				a.hide()
			} else {
				a.closed = time.Time{}
				a.toggle(true)
			}
			return 0
		case 111:
			u("DestroyWindow", hwnd)
			return 0
		case 112:
			a.persist()
			return 0
		case 113:
			s.browse()
			return 0
		case 115:
			s.browseFolder()
			return 0
		case 114:
			s.record = true
			setText(l, "Press shortcut…")
			return 0
		}
	case 0x0114:
		for _, k := range []string{"height", "font", "gap"} {
			if h := s.items[k]; h != 0 && l == h {
				setText(s.items[k+"-label"], strconv.Itoa(int(send(h, 0x400, 0, 0))))
				return 0
			}
		}
	case 0x10:
		u("DestroyWindow", hwnd)
		return 0
	case 2:
		if s.hwnd == hwnd {
			a.settings = nil
			win(gdi, "DeleteObject", s.font)
			win(gdi, "DeleteObject", s.header)
		}
		return 0
	}
	return u("DefWindowProcW", hwnd, message, w, l)
}
