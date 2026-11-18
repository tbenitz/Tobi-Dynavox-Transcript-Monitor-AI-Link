package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unsafe"
)

const overlayClass = "AACLink_Overlay_01"
const settingsClass = "AACLink_Settings_01"
const hotkeyID = 1
const wmTray = 0x8010
const wmMacro = 0x8011
const wmPrediction = 0x8012
const pollTimer = 1
const tickTimer = 2

type wndClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type windowMessage struct {
	Hwnd    uintptr
	Msg     uint32
	W, L    uintptr
	T       uint32
	P       point
	Private uint32
}
type app struct {
	s                                                  settings
	hwnd, instance, font, small, bg, icon, restartTray uintptr
	settings                                           *settingsWindow
	caption, status, close, grip                       uintptr
	buttons                                            [5]uintptr
	visible, preview, paused, ready, hotkey            bool
	manualRefresh                                      bool
	target                                             destination
	draft                                              string
	draftReadable                                      bool
	transcript, transcriptStatus                       string
	shownDraft, pendingDraft                           string
	current, pending                                   []suggestion
	pendingSet                                         bool
	providerStatus, toast                              string
	closed, opened, lastRequest, toastUntil            time.Time
	generation                                         uint32
	pendingAI                                          bool
	cancel                                             context.CancelFunc
	results                                            chan result
	watch                                              transcriptMonitor
	moving                                             bool
	drag                                               point
	dragFrom                                           rect
	tray                                               notifyData
}
type result struct {
	number uint32
	out    predictions
	err    error
}

var appState *app
var uiScale = 96
var wProc, settingProc, buttonProc uintptr
var httpClient = &http.Client{
	Timeout:       18 * time.Second,
	Transport:     &http.Transport{}, // No environment proxy, including for localhost.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func px(n int) int      { return (n*uiScale + 48) / 96 }
func logical(n int) int { return n * 96 / uiScale }
func font(size, weight int) uintptr {
	return win(gdi, "CreateFontW", signed(-px(size)), 0, 0, 0, weight, 0, 0, 0, 1, 0, 0, 5, 0, ptr(wp("Segoe UI")))
}
func registerClass(name string, cb, instance, cursor, brush, icon uintptr) bool {
	k := wndClass{Size: uint32(unsafe.Sizeof(wndClass{})), Proc: cb, Instance: instance, Cursor: cursor, Background: brush, Name: wp(name), Icon: icon, SmallIcon: icon}
	return u("RegisterClassExW", ptr(&k)) != 0
}
func create(ex uintptr, class, title string, style uintptr, x, y, w, h int, parent, id, instance uintptr) uintptr {
	return u("CreateWindowExW", ex, ptr(wp(class)), ptr(wp(title)), style, signed(x), signed(y), signed(w), signed(h), parent, id, instance, 0)
}
func control(a *app, parent uintptr, kind, name string, style uintptr, id int) uintptr {
	return create(0, kind, name, 0x50000000|style, 0, 0, 0, 0, parent, uintptr(id), a.instance)
}
func place(h uintptr, x, y, w, d int) {
	u("MoveWindow", h, signed(x), signed(y), signed(w), signed(d), 1)
}
func show(h uintptr, on bool) {
	if on {
		u("ShowWindow", h, 8)
	} else {
		u("ShowWindow", h, 0)
	}
}
func main() {
	runtime.LockOSThread()
	instance := win(kernel, "GetModuleHandleW", 0)
	if len(os.Args) == 2 && (os.Args[1] == "--toggle" || os.Args[1] == "--pause") {
		h := u("FindWindowW", ptr(wp(overlayClass)), 0)
		if h != 0 {
			mode := 1
			if os.Args[1] == "--pause" {
				mode = 2
			}
			u("PostMessageW", h, wmMacro, mode, 0)
			return
		}
		msg(0, "AAC Link", "Start AAC Link from its normal shortcut first.")
		return
	}
	existing := u("FindWindowW", ptr(wp(overlayClass)), 0)
	if existing != 0 {
		u("PostMessageW", existing, wmMacro, 3, 0)
		return
	}
	u("SetProcessDPIAware")
	if p := user.NewProc("GetDpiForSystem"); p.Find() == nil {
		if n, _, _ := p.Call(); n >= 96 && n <= 384 {
			uiScale = int(n)
		}
	}
	win(common, "InitCommonControls")
	ico := speechIcon(instance)
	if ico == 0 {
		ico = u("LoadIconW", 0, 32516)
	}
	wProc = sysCallback(overlayWnd)
	settingProc = sysCallback(settingsWnd)
	buttonProc = sysSubclass(passiveButton)
	cursor := u("LoadCursorW", 0, 32512)
	if !registerClass(overlayClass, wProc, instance, cursor, 0, ico) || !registerClass(settingsClass, settingProc, instance, cursor, 16, ico) {
		msg(0, "AAC Link", "Could not initialize Windows controls.")
		return
	}
	a := &app{s: readSettings(), instance: instance, icon: ico, results: make(chan result, 4)}
	if len(os.Args) == 2 && os.Args[1] == "--self-test" {
		if err := nativeSmoke(a); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	appState = a
	a.hwnd = create(0x08000088, overlayClass, "AAC Link suggestions", 0x80000000, 0, 0, px(700), px(160), 0, 0, a.instance)
	if a.hwnd == 0 {
		msg(0, "AAC Link", "Could not create the suggestions window.")
		return
	}
	a.caption = control(a, a.hwnd, "STATIC", "AAC LINK    /    RESPONSE SUGGESTIONS", 0, 0)
	a.status = control(a, a.hwnd, "STATIC", "", 0x4000, 0) // SS_ENDELLIPSIS
	a.grip = control(a, a.hwnd, "STATIC", "Drag to place", 2, 90)
	a.close = control(a, a.hwnd, "BUTTON", "Dismiss", 0x000b, 200)
	for i := range a.buttons {
		a.buttons[i] = control(a, a.hwnd, "BUTTON", "", 0x000b, 201+i)
	}
	for _, b := range append([]uintptr{a.close}, a.buttons[:]...) {
		win(common, "SetWindowSubclass", b, buttonProc, 1, 0)
	}
	a.updateFonts()
	a.restartTray = u("RegisterWindowMessageW", ptr(wp("TaskbarCreated")))
	a.addTray()
	if err := a.setHotkey(a.s.Hotkey); err != nil {
		a.balloon("Shortcut unavailable", "Choose another shortcut in AAC Link settings.")
	}
	a.startPolling()
	u("SetTimer", a.hwnd, tickTimer, 100, 0)
	a.poll()
	if a.s.transcriptPath(time.Now()) == "" {
		a.openSettings()
	}
	a.ready = true
	for {
		var m windowMessage
		n := int32(u("GetMessageW", ptr(&m), 0, 0, 0))
		if n <= 0 {
			break
		}
		if a.recordMessage(m) {
			continue
		}
		if a.settings != nil && u("IsDialogMessageW", a.settings.hwnd, ptr(&m)) != 0 {
			continue
		}
		u("TranslateMessage", ptr(&m))
		u("DispatchMessageW", ptr(&m))
	}
}
func (a *app) updateFonts() {
	for _, f := range []uintptr{a.font, a.small, a.bg} {
		if f != 0 {
			win(gdi, "DeleteObject", f)
		}
	}
	a.font = font(a.s.Font, 600)
	a.small = font(14, 500)
	bg, _, _, _, _ := a.colors()
	a.bg = win(gdi, "CreateSolidBrush", bg)
	for _, h := range []uintptr{a.caption, a.status, a.grip} {
		send(h, 0x30, a.small, 1)
	}
}
func (a *app) setHotkey(s string) error {
	mods, key, e := parseHotkey(s)
	if e != nil {
		return e
	}
	if a.hotkey {
		u("UnregisterHotKey", a.hwnd, hotkeyID)
		a.hotkey = false
	}
	if u("RegisterHotKey", a.hwnd, hotkeyID, mods|0x4000, key) == 0 {
		return fmt.Errorf("%s is reserved or already in use. Choose a different shortcut.", s)
	}
	a.hotkey = true
	return nil
}
func (a *app) startPolling() {
	u("KillTimer", a.hwnd, pollTimer)
	u("SetTimer", a.hwnd, pollTimer, a.s.Poll, 0)
}
func (a *app) poll() {
	if a.paused {
		return
	}
	now := time.Now()
	previous := a.watch.path
	raw, err, switched := a.watch.read(a.s, now)
	if switched && previous != "" && !a.preview {
		// Do not leave yesterday's buttons available under a resting pointer.
		a.hide()
	}
	var heard, status string
	switch {
	case a.watch.path == "":
		status = "No transcript selected"
	case err != nil:
		status = "Waiting for " + filepath.Base(a.watch.path)
	default:
		heard = recent(raw, a.s.Unit, a.s.Context)
		if heard == "" {
			status = "Waiting for new speech"
		}
	}
	changed := heard != a.transcript || status != a.transcriptStatus
	a.transcript, a.transcriptStatus = heard, status
	if !a.visible || a.preview {
		return
	}
	if !a.target.same(foreground()) {
		a.hide()
		return
	}
	d, ok := readDraft(a.target)
	if changed || d != a.draft || ok != a.draftReadable {
		a.draft, a.draftReadable = d, ok
		a.refresh()
	}
}
func (a *app) sourceRolledOver() bool {
	if !a.preview && !a.paused && a.s.transcriptPath(time.Now()) != a.watch.path {
		a.poll()
		return true
	}
	return false
}

func (a *app) tick() {
	if !a.visible {
		return
	}
	if a.sourceRolledOver() {
		return
	}
	if !a.preview && !a.target.same(foreground()) {
		a.hide()
		return
	}
	if a.s.Timeout > 0 && time.Since(a.opened) >= time.Duration(a.s.Timeout)*time.Second && !a.moving {
		a.hide()
		return
	}
	if a.pendingSet && !windowRect(a.hwnd).contains(cursor()) && !a.moving {
		a.applySuggestions(a.pending, a.pendingDraft)
	}
	if a.pendingAI && time.Since(a.lastRequest) > 1650*time.Millisecond {
		a.requestAI()
	}
	if !a.toastUntil.IsZero() && time.Now().After(a.toastUntil) {
		a.toast = ""
		a.toastUntil = time.Time{}
		a.updateStatus()
	}
}
func (a *app) toggle(preview bool) {
	if a.visible {
		a.hide()
		return
	}
	if !mayOpen(time.Now(), a.closed, a.s.Cooldown) {
		return
	}
	if !preview {
		a.poll()
	}
	d := foreground()
	if !preview && (d.root == 0 || d.pid == uint32(win(kernel, "GetCurrentProcessId")) || d.focus == 0 ||
		(a.s.Target != "" && !strings.Contains(strings.ToLower(d.title), strings.ToLower(a.s.Target)))) {
		a.balloon("No matching AAC window", "Focus the AAC message bar, then use your shortcut.")
		return
	}
	a.preview, a.target, a.visible = preview, d, true
	a.opened = time.Now()
	a.draft, a.draftReadable = "", false
	if !preview {
		a.draft, a.draftReadable = readDraft(d)
	}
	a.current = nil
	a.pending = nil
	a.pendingSet = false
	a.manualRefresh = true
	a.refresh()
	a.manualRefresh = false
	a.layout()
	show(a.hwnd, true)
}
func (a *app) hide() {
	if !a.visible {
		return
	}
	a.visible = false
	a.closed = time.Now()
	a.current = nil
	a.pending = nil
	a.pendingSet = false
	a.toast = ""
	a.toastUntil = time.Time{}
	a.draft = ""
	a.draftReadable = false
	a.target = destination{}
	a.invalidateAI()
	if a.moving {
		a.moving = false
		u("ReleaseCapture")
	}
	show(a.hwnd, false)
}
func (a *app) invalidateAI() {
	a.generation++
	a.pendingAI = false
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}
func (a *app) refresh() {
	a.invalidateAI()
	heard, draft := a.transcript, a.draft
	if a.preview {
		heard = "Would you like coffee or tea?"
		draft = ""
	}
	if a.paused && !a.preview {
		heard = ""
	}
	a.providerStatus = ""
	a.queueSuggestions(offline(heard, draft, a.s.Mode))
	provider, _, _ := a.s.endpoint()
	if !a.preview && !a.paused && provider != "" && (heard != "" || draft != "") {
		if provider == "ollama" || a.s.Cloud {
			a.pendingAI = true
			a.providerStatus = "AI updating…"
		} else {
			a.providerStatus = "Cloud sharing is off"
		}
	}
	a.updateStatus()
}
func (a *app) requestAI() {
	a.pendingAI = false
	if !a.visible || a.preview || a.paused {
		return
	}
	provider, model, url := a.s.endpoint()
	if provider == "" || provider != "ollama" && !a.s.Cloud {
		return
	}
	key := ""
	if provider != "ollama" {
		secret := a.s.DeepSeekSecret
		env := "DEEPSEEK_API_KEY"
		if provider == "grok" {
			secret = a.s.GrokSecret
			env = "XAI_API_KEY"
		}
		key, _ = protect(secret, false)
		if key == "" {
			key = os.Getenv(env)
		}
		if key == "" {
			a.providerStatus = "API key missing • offline suggestions"
			a.updateStatus()
			return
		}
	}
	a.lastRequest = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	num, heard, draft, mode, h := a.generation, a.transcript, a.draft, a.s.Mode, a.hwnd
	go func() {
		out, e := predict(ctx, httpClient, url, provider, model, key, heard, draft, mode)
		if ctx.Err() != nil {
			return
		}
		select {
		case a.results <- result{num, out, e}:
			u("PostMessageW", h, wmPrediction, 0, 0)
		default:
		}
	}()
}
func (a *app) receive() {
	if a.visible {
		a.sourceRolledOver()
	}
	for {
		select {
		case r := <-a.results:
			if r.number != a.generation || !a.visible || a.paused {
				continue
			}
			if r.err != nil {
				a.providerStatus = "AI unavailable (" + classifyAI(r.err) + ") • offline suggestions"
			} else {
				a.providerStatus = a.s.Engine
				a.queueSuggestions(mergeAI(r.out, offline(a.transcript, a.draft, a.s.Mode), a.s.Mode))
			}
			a.updateStatus()
		default:
			return
		}
	}
}
func classifyAI(e error) string {
	s := e.Error()
	if strings.HasPrefix(s, "provider returned HTTP ") {
		return strings.TrimPrefix(s, "provider returned ")
	}
	if strings.Contains(s, "timeout") || strings.Contains(s, "deadline") {
		return "timeout"
	}
	if strings.Contains(s, "no usable") {
		return "no usable output"
	}
	return "check provider setup"
}
func (a *app) queueSuggestions(v []suggestion) {
	if !a.manualRefresh && a.visible && windowRect(a.hwnd).contains(cursor()) {
		a.pending = v
		a.pendingDraft = a.draft
		a.pendingSet = true
		return
	}
	a.applySuggestions(v, a.draft)
}
func (a *app) applySuggestions(v []suggestion, shownDraft string) {
	a.pending = nil
	a.pendingSet = false
	a.shownDraft = shownDraft
	if slices.Equal(a.current, v) {
		return
	}
	a.current = v
	for i, h := range a.buttons {
		next := i < len(v)
		if next {
			setText(h, v[i].Text)
		}
		show(h, next)
		u("InvalidateRect", h, 0, 1)
	}
	if len(v) == 0 {
		a.updateStatus()
	}
}
func (a *app) flash(s string) {
	a.toast = s
	a.toastUntil = time.Now().Add(3 * time.Second)
	a.updateStatus()
}
func (a *app) updateStatus() {
	s := a.target.title
	if s == "" {
		s = "Active Windows app"
	}
	d := []rune(s)
	if len(d) > 38 {
		s = string(d[:37]) + "…"
	}
	parts := []string{s}
	if a.preview {
		parts = []string{"Preview only • taps do not insert text"}
	} else if a.paused {
		parts = append(parts, "Watching paused • generic suggestions")
	} else {
		if a.transcriptStatus != "" {
			parts = append(parts, a.transcriptStatus)
		}
		if a.s.Mode != quick && !a.draftReadable {
			parts = append(parts, "Context words only • AAC draft not exposed")
		}
		if a.providerStatus != "" {
			parts = append(parts, a.providerStatus)
		}
	}
	if len(a.current) == 0 && a.draftReadable {
		parts = append(parts, "No matching word completions")
	}
	if a.toast != "" {
		parts = []string{a.toast}
	}
	setText(a.status, strings.Join(parts, "   /   "))
}
func (a *app) selectSuggestion(i int) {
	if !a.visible || i < 0 || i >= len(a.current) {
		return
	}
	if a.sourceRolledOver() {
		return
	}
	if a.preview {
		a.flash("Preview • " + a.current[i].Text)
		a.opened = time.Now()
		return
	}
	if !a.target.same(foreground()) {
		a.hide()
		return
	}
	if modifiersDown() {
		a.flash("Release the shortcut keys, then choose again.")
		return
	}
	draft, ok := readDraft(a.target)
	if a.current[i].Word && (draft != a.shownDraft || ok != a.draftReadable) {
		a.draft, a.draftReadable = draft, ok
		a.flash("Draft changed. Move off the bar to refresh, then choose again.")
		a.refresh()
		return
	}
	v := insertion(a.current[i], draft, a.s.Space, a.s.Punctuation)
	if v == "" {
		a.flash("No matching completion. Try another suggestion.")
		return
	}
	if !a.target.same(foreground()) {
		a.hide()
		return
	}
	inserted := false
	if a.s.Injection == "Clipboard paste" {
		inserted = pasteText(a.hwnd, v, a.target)
	} else {
		inserted = typeText(v)
	}
	if !inserted {
		a.flash("Windows did not accept the text. Try the other insertion method.")
		return
	}
	if a.s.CloseAfter {
		a.hide()
		return
	}
	a.opened = time.Now()
	a.flash("Text sent • check the AAC message bar")
}
func (a *app) pause() {
	a.hide()
	a.paused = !a.paused
	a.transcript = ""
	a.transcriptStatus = ""
	if !a.paused {
		a.watch.resume(a.s, time.Now())
		a.poll()
	}
	a.balloon("AAC Link", map[bool]string{true: "Transcript and AI paused.", false: "Watching only newly appended transcript text."}[a.paused])
}
func (a *app) reload() {
	a.hide()
	if a.paused {
		return
	}
	a.watch.reload(a.s, time.Now())
	a.poll()
	a.balloon("AAC Link", "Transcript reader reset.")
}
