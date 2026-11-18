package main

import (
	"fmt"
	"time"
	"unicode/utf16"
	"unsafe"
)

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

func monitorWork(h uintptr) rect {
	mon := u("MonitorFromWindow", h, 2)
	m := monitorInfo{Size: 40}
	if u("GetMonitorInfoW", mon, ptr(&m)) != 0 && m.Work.width() > 0 {
		return m.Work
	}
	return rect{0, 0, int32(u("GetSystemMetrics", 0)), int32(u("GetSystemMetrics", 1))}
}
func (a *app) colors() (bg, fg, button, buttonText, accent uintptr) {
	switch a.s.Theme {
	case themes[1]:
		return rgb(240, 244, 249), rgb(20, 31, 52), rgb(221, 233, 250), rgb(16, 35, 64), rgb(55, 91, 135)
	case themes[2]:
		return rgb(11, 37, 63), rgb(230, 246, 255), rgb(198, 238, 255), rgb(12, 39, 66), rgb(98, 198, 238)
	default:
		return rgb(8, 8, 8), rgb(255, 229, 90), rgb(255, 224, 59), rgb(0, 0, 0), rgb(255, 224, 59)
	}
}
func (a *app) layout() {
	work := monitorWork(a.target.root)
	width := min(px(1060), max(px(280), work.width()-2*px(12)))
	height := px(a.s.Height + 75)
	x := int(work.L) + (work.width()-width)/2
	y := int(work.T) + px(12)
	if a.s.Position == positions[1] || a.s.Position == positions[2] {
		y = int(work.B) - px(a.s.KeyboardHeight) - height - px(7)
	}
	if a.s.Position == positions[2] && a.s.HasPosition {
		x, y = px(a.s.X), px(a.s.Y)
	}
	x = min(max(x, int(work.L)), max(int(work.L), int(work.R)-width))
	y = min(max(y, int(work.T)), max(int(work.T), int(work.B)-height))
	u("SetWindowPos", a.hwnd, ^uintptr(0), signed(x), signed(y), signed(width), signed(height), 0x10)
	pad, gap := px(13), px(a.s.Gap)
	barY := px(38)
	closeWidth := px(87)
	if a.s.LargeClose {
		closeWidth = px(130)
	}
	show(a.grip, a.s.Position == positions[2] && !a.s.Lock)
	setText(a.close, map[bool]string{true: "×  Dismiss", false: "× Close"}[a.s.LargeClose])
	place(a.caption, pad, px(11), width-2*pad-px(113), px(24))
	place(a.grip, width-pad-px(110), px(8), px(110), px(25))
	place(a.status, pad, height-px(29), width-2*pad, px(24))
	start := pad
	if a.s.CloseSide == "Left" {
		place(a.close, pad, barY, closeWidth, px(a.s.Height))
		start += closeWidth + gap
	} else {
		place(a.close, width-pad-closeWidth, barY, closeWidth, px(a.s.Height))
	}
	buttonWidth := max(1, (width-2*pad-closeWidth-gap*5)/5)
	for i, h := range a.buttons {
		place(h, start+i*(buttonWidth+gap), barY, buttonWidth, px(a.s.Height))
	}
	u("InvalidateRect", a.hwnd, 0, 1)
}

type drawItem struct {
	Kind, ID, Item, Action, State uint32
	H, DC                         uintptr
	Rect                          rect
	Data                          uintptr
}

func drawFill(dc uintptr, r rect, color uintptr) {
	b := win(gdi, "CreateSolidBrush", color)
	u("FillRect", dc, ptr(&r), b)
	win(gdi, "DeleteObject", b)
}

//go:nocheckptr
func (a *app) draw(l uintptr) {
	d := (*drawItem)(unsafe.Pointer(l))
	_, _, bg, fg, accent := a.colors()
	if d.ID == 200 {
		bg, fg, accent = rgb(161, 28, 40), rgb(255, 255, 255), rgb(255, 165, 165)
	}
	if d.State&1 != 0 {
		bg, fg = fg, bg
	}
	drawFill(d.DC, d.Rect, accent)
	r := d.Rect
	r.L += 2
	r.T += 2
	r.R -= 2
	r.B -= 2
	drawFill(d.DC, r, bg)
	r.L += 5
	r.R -= 5
	r.T += 3
	r.B -= 3
	win(gdi, "SetTextColor", d.DC, fg)
	win(gdi, "SetBkMode", d.DC, 1)
	orig := win(gdi, "SelectObject", d.DC, a.font)
	s := textWin(d.H)
	if s == "" {
		win(gdi, "SelectObject", d.DC, orig)
		return
	}
	p := wp(s)
	n := len(utf16.Encode([]rune(s)))
	size := r
	const fmtFlags = 0x10 | 1 | 0x800 // WRAP, CENTER, NOPREFIX
	u("DrawTextW", d.DC, ptr(p), n, ptr(&size), fmtFlags|0x400)
	if size.height() > r.height() || size.width() > r.width() {
		win(gdi, "SelectObject", d.DC, a.small)
		size = r
		u("DrawTextW", d.DC, ptr(p), n, ptr(&size), fmtFlags|0x400)
	}
	if size.height() < r.height() {
		r.T += int32((r.height() - size.height()) / 2)
	}
	u("DrawTextW", d.DC, ptr(p), n, ptr(&r), fmtFlags|0x40000)
	win(gdi, "SelectObject", d.DC, orig)
}
func overlayWnd(hwnd, message, w, l uintptr) uintptr {
	a := appState
	if message == 0x21 {
		return 3
	} // MA_NOACTIVATE (deliver the click)
	if a == nil {
		return u("DefWindowProcW", hwnd, message, w, l)
	}
	if a.restartTray != 0 && message == a.restartTray {
		a.addTray()
		return 0
	}
	switch message {
	case 0x0014: // WM_ERASEBKGND
		if a.bg != 0 {
			var r rect
			u("GetClientRect", hwnd, ptr(&r))
			u("FillRect", w, ptr(&r), a.bg)
			return 1
		}
	case 0x0138: // WM_CTLCOLORSTATIC
		_, c, _, _, _ := a.colors()
		if a.bg != 0 {
			win(gdi, "SetTextColor", w, c)
			win(gdi, "SetBkMode", w, 1)
			return a.bg
		}
	case 0x002b:
		if l != 0 {
			a.draw(l)
			return 1
		}
	case 0x0111:
		id := uint16(w)
		if w>>16 == 0 && id == 200 {
			a.hide()
			return 0
		}
		if w>>16 == 0 && id >= 201 && id < 206 {
			a.selectSuggestion(int(id - 201))
			return 0
		}
	case 0x0312:
		if w == hotkeyID {
			a.toggle(false)
			return 0
		}
	case 0x0113:
		if w == pollTimer {
			a.poll()
			return 0
		}
		if w == tickTimer {
			a.tick()
			return 0
		}
	case 0x0201:
		if a.visible && a.s.Position == positions[2] && !a.s.Lock && windowRect(a.grip).contains(cursor()) {
			a.moving = true
			a.drag = cursor()
			a.dragFrom = windowRect(hwnd)
			u("SetCapture", hwnd)
			return 0
		}
	case 0x0200:
		if a.moving {
			p := cursor()
			work := monitorWork(hwnd)
			d := a.dragFrom
			x := min(max(d.L+p.X-a.drag.X, work.L), max(work.L, work.R-int32(d.width())))
			y := min(max(d.T+p.Y-a.drag.Y, work.T), max(work.T, work.B-int32(d.height())))
			u("SetWindowPos", hwnd, 0, signed(int(x)), signed(int(y)), 0, 0, 0x15)
			return 0
		}
	case 0x0202, 0x0215:
		if a.moving {
			a.moving = false
			u("ReleaseCapture")
			r := windowRect(hwnd)
			a.s.X, a.s.Y, a.s.HasPosition = logical(int(r.L)), logical(int(r.T)), true
			if err := saveSettings(a.s); err != nil {
				a.flash("Position could not be saved.")
			}
			return 0
		}
	case wmTray:
		switch uint16(l) {
		case 0x0205, 0x007b:
			a.trayMenu()
		case 0x0203:
			a.openSettings()
		}
		return 0
	case wmPrediction:
		a.receive()
		return 0
	case wmMacro:
		switch w {
		case 1:
			a.toggle(false)
		case 2:
			a.pause()
		case 3:
			a.openSettings()
		}
		return 0
	case 0x0010:
		u("DestroyWindow", hwnd)
		return 0
	case 0x0002:
		a.hide()
		a.deleteTray()
		u("UnregisterHotKey", hwnd, hotkeyID)
		u("PostQuitMessage", 0)
		return 0
	}
	return u("DefWindowProcW", hwnd, message, w, l)
}

type notifyData struct {
	Size                uint32
	Hwnd                uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	Title               [64]uint16
	InfoFlags           uint32
	Guid                [16]byte
	BalloonIcon         uintptr
}

func fill16(buf []uint16, s string) {
	clear(buf)
	copy(buf[:len(buf)-1], utf16.Encode([]rune(s)))
}
func (a *app) addTray() {
	n := notifyData{Hwnd: a.hwnd, ID: 1, Flags: 7, Callback: wmTray, Icon: a.icon}
	n.Size = uint32(unsafe.Sizeof(n))
	fill16(n.Tip[:], "AAC Link • Transcript response suggestions")
	a.tray = n
	win(shell, "Shell_NotifyIconW", 0, ptr(&n))
}
func (a *app) deleteTray() { win(shell, "Shell_NotifyIconW", 2, ptr(&a.tray)) }
func (a *app) balloon(title, body string) {
	n := a.tray
	n.Flags = 0x10
	n.InfoFlags = 1
	fill16(n.Title[:], title)
	fill16(n.Info[:], body)
	win(shell, "Shell_NotifyIconW", 1, ptr(&n))
}
func (a *app) trayMenu() {
	m := u("CreatePopupMenu")
	if m == 0 {
		return
	}
	defer u("DestroyMenu", m)
	toggle := "Show overlay (preview)"
	if a.visible {
		toggle = "Hide overlay"
	}
	pause := "Pause transcript watching"
	if a.paused {
		pause = "Resume transcript watching"
	}
	labels := []string{toggle, pause, "Settings…", "Reload transcript file", "Exit"}
	for i, s := range labels {
		u("AppendMenuW", m, 0, uintptr(i+1), ptr(wp(s)))
	}
	owner := a.hwnd
	if a.settings != nil {
		owner = a.settings.hwnd
	}
	// The shell owns foreground activation for a tray menu. The real overlay is
	// summoned through the hotkey or macro, never by capturing the menu's focus.
	u("SetForegroundWindow", owner)
	p := cursor()
	id := u("TrackPopupMenu", m, 0x100|2, signed(int(p.X)), signed(int(p.Y)), 0, owner, 0)
	u("PostMessageW", owner, 0, 0, 0)
	switch id {
	case 1:
		if a.visible {
			a.hide()
		} else {
			a.closed = time.Time{}
			a.toggle(true)
		}
	case 2:
		a.pause()
	case 3:
		a.openSettings()
	case 4:
		a.reload()
	case 5:
		u("DestroyWindow", a.hwnd)
	}
}
func nativeSmoke(a *app) error {
	if unsafe.Sizeof(input{}) != 40 || unsafe.Sizeof(notifyData{}) != 976 {
		return fmt.Errorf("unexpected Windows struct ABI")
	}
	a.hwnd = create(0x08000088, overlayClass, "AAC Link native smoke test", 0x80000000, 0, 0, 500, 125, 0, 0, a.instance)
	if a.hwnd == 0 {
		return fmt.Errorf("overlay creation failed")
	}
	defer u("DestroyWindow", a.hwnd)
	const exStyle = ^uintptr(19)
	ex := u("GetWindowLongPtrW", a.hwnd, exStyle)
	if ex&0x08000000 == 0 {
		return fmt.Errorf("missing WS_EX_NOACTIVATE")
	}
	if send(a.hwnd, 0x21, 0, 0) != 3 {
		return fmt.Errorf("mouse activation not blocked")
	}
	h := control(a, a.hwnd, "BUTTON", "Dismiss", 0xb, 1)
	if h == 0 || textWin(h) != "Dismiss" {
		return fmt.Errorf("button creation failed")
	}
	token, e := protect("local-test-token", true)
	if e != nil {
		return e
	}
	raw, e := protect(token, false)
	if e != nil || raw != "local-test-token" {
		return fmt.Errorf("DPAPI round trip failed: %v", e)
	}
	return nil
}

// Use native Button controls for accessible names and Invoke. On mouse/touch
// activation handle clicks without calling Button's focus-setting default path.
var downButton uintptr

func passiveButton(h, m, w, l, id, ref uintptr) uintptr {
	switch m {
	case 0x21:
		return 3
	case 0x201, 0x203: // WM_LBUTTONDOWN / double click
		downButton = h
		u("SetCapture", h)
		send(h, 0xf3, 1, 0)
		return 0
	case 0x202:
		was := downButton == h
		if was {
			downButton = 0
			u("ReleaseCapture")
		}
		send(h, 0xf3, 0, 0)
		if !was || !windowRect(h).contains(cursor()) {
			return 0
		}
		fallthrough
	case 0xf5: // BM_CLICK, including assistive technology invocation
		parent := u("GetParent", h)
		u("PostMessageW", parent, 0x111, u("GetDlgCtrlID", h), h)
		return 0
	case 0x215, 0x001f:
		if downButton == h {
			downButton = 0
			send(h, 0xf3, 0, 0)
		}
	case 0x0082:
		win(common, "RemoveWindowSubclass", h, buttonProc, id)
	}
	return win(common, "DefSubclassProc", h, m, w, l)
}
func speechIcon(instance uintptr) uintptr {
	const size = 32
	color := make([]byte, size*size*4)
	mask := make([]byte, size*size/8)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			bubble := (x >= 3 && x < 29 && y >= 5 && y < 24 && !(x == 3 || x == 28) && (y > 5 && y < 23)) || (x >= 5 && x < 27 && y >= 5 && y < 24)
			tail := y >= 23 && y <= 28 && x >= 7 && x <= 13 && x < 37-y
			if !bubble && !tail {
				mask[y*4+x/8] |= byte(1 << uint(7-x%8))
				continue
			}
			i := (y*size + x) * 4
			color[i], color[i+1], color[i+2], color[i+3] = 64, 231, 255, 255
			if y >= 12 && y < 16 && ((x >= 8 && x < 12) || (x >= 14 && x < 18) || (x >= 20 && x < 24)) {
				color[i], color[i+1], color[i+2] = 12, 15, 22
			}
		}
	}
	return u("CreateIcon", instance, size, size, 1, 32, ptr(&mask[0]), ptr(&color[0]))
}
