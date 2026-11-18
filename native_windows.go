package main

import (
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user   = syscall.NewLazyDLL("user32.dll")
	kernel = syscall.NewLazyDLL("kernel32.dll")
	shell  = syscall.NewLazyDLL("shell32.dll")
	gdi    = syscall.NewLazyDLL("gdi32.dll")
	common = syscall.NewLazyDLL("comctl32.dll")
	dialog = syscall.NewLazyDLL("comdlg32.dll")
	crypt  = syscall.NewLazyDLL("crypt32.dll")
)
var retainNativeArgs bool
var nativeArgs []any

// The Go objects behind pointer parameters must stay live and on the heap
// through the synchronous native call. No Go pointer is retained by Win32.
func win(d *syscall.LazyDLL, n string, a ...any) uintptr {
	if retainNativeArgs {
		nativeArgs = a
	} // escape analysis only
	b := make([]uintptr, len(a))
	for i, v := range a {
		switch x := v.(type) {
		case uintptr:
			b[i] = x
		case unsafe.Pointer:
			b[i] = uintptr(x)
		case int:
			b[i] = uintptr(x)
		case uint32:
			b[i] = uintptr(x)
		default:
			panic(fmt.Sprintf("unsupported native argument %T", v))
		}
	}
	r, _, _ := d.NewProc(n).Call(b...)
	runtime.KeepAlive(a)
	return r
}
func u(n string, a ...any) uintptr   { return win(user, n, a...) }
func ptr[T any](p *T) unsafe.Pointer { return unsafe.Pointer(p) }
func wp(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	return p
}
func signed(v int) uintptr { return uintptr(v) }
func textWin(hwnd uintptr) string {
	b := make([]uint16, 4096)
	n := u("GetWindowTextW", hwnd, ptr(&b[0]), uintptr(len(b)))
	return string(utf16.Decode(b[:min(n, uintptr(len(b)))]))
}
func setText(hwnd uintptr, s string) {
	if hwnd != 0 {
		u("SetWindowTextW", hwnd, ptr(wp(s)))
	}
}
func send(hwnd, msg uintptr, w, l any) uintptr { return u("SendMessageW", hwnd, msg, w, l) }
func msg(hwnd uintptr, title, body string) {
	u("MessageBoxW", hwnd, ptr(wp(body)), ptr(wp(title)), 0x30)
}

type point struct{ X, Y int32 }
type rect struct{ L, T, R, B int32 }

func (r rect) width() int            { return int(r.R - r.L) }
func (r rect) height() int           { return int(r.B - r.T) }
func windowRect(h uintptr) (r rect)  { u("GetWindowRect", h, ptr(&r)); return }
func cursor() (p point)              { u("GetCursorPos", ptr(&p)); return }
func (r rect) contains(p point) bool { return p.X >= r.L && p.X < r.R && p.Y >= r.T && p.Y < r.B }

type guiInfo struct {
	Size, Flags                               uint32
	Active, Focus, Capture, Menu, Move, Caret uintptr
	CaretRect                                 rect
}
type destination struct {
	root, focus uintptr
	pid         uint32
	title       string
}

func foreground() destination {
	h := u("GetForegroundWindow")
	if h == 0 {
		return destination{}
	}
	var pid uint32
	tid := u("GetWindowThreadProcessId", h, ptr(&pid))
	g := guiInfo{}
	g.Size = uint32(unsafe.Sizeof(g))
	u("GetGUIThreadInfo", tid, ptr(&g))
	return destination{h, g.Focus, pid, textWin(h)}
}
func (d destination) same(o destination) bool {
	return d.root != 0 && d.root == o.root && d.pid == o.pid && d.focus == o.focus
}
func boundedMessage(h, m uintptr, w, l any) (uintptr, bool) {
	var out uintptr
	r := u("SendMessageTimeoutW", h, m, w, l, 2, 60, ptr(&out))
	return out, r != 0
}

// Many AAC apps draw their own message bar. Never scrape unrelated windows,
// password fields, or an entire document via the clipboard.
func readDraft(d destination) (string, bool) {
	if d.focus == 0 {
		return "", false
	}
	b := make([]uint16, 128)
	n := u("GetClassNameW", d.focus, ptr(&b[0]), uintptr(len(b)))
	class := strings.ToLower(string(utf16.Decode(b[:min(n, uintptr(len(b)))])))
	if class != "edit" {
		return "", false
	}
	if u("GetWindowLongPtrW", d.focus, ^uintptr(15))&0x20 != 0 {
		return "", false
	} // GWL_STYLE, ES_PASSWORD
	end, ok := boundedMessage(d.focus, 0x00b0, 0, 0) // EM_GETSEL, packed start/end
	if !ok || uint32(end) == 0xffffffff {
		return "", false
	}
	start := int(end & 0xffff)
	size, ok := boundedMessage(d.focus, 0x000e, 0, 0)
	if !ok || size > 8192 || uintptr(start) > size {
		return "", false
	}
	if start == 0 {
		return "", true
	}
	t := make([]uint16, size+1)
	copied, ok := boundedMessage(d.focus, 0x000d, uintptr(len(t)), ptr(&t[0]))
	if !ok || uintptr(start) > copied {
		return "", false
	}
	i := max(0, start-280)
	if i > 0 && t[i] >= 0xdc00 && t[i] <= 0xdfff {
		i++
	}
	return string(utf16.Decode(t[i:start])), true
}

type input struct {
	Kind        uint32
	_           uint32
	VK, Scan    uint16
	Flags, Time uint32
	_           uint32
	Extra       uintptr
	_           uint64
}

func keyInput(vk, scan uint16, flags uint32) input {
	return input{Kind: 1, VK: vk, Scan: scan, Flags: flags}
}
func sendInputs(a []input) bool {
	if len(a) == 0 {
		return false
	}
	n := u("SendInput", uintptr(len(a)), ptr(&a[0]), unsafe.Sizeof(a[0]))
	runtime.KeepAlive(a)
	return n == uintptr(len(a))
}
func typeText(v string) bool {
	var a []input
	for _, c := range utf16.Encode([]rune(v)) {
		a = append(a, keyInput(0, c, 4), keyInput(0, c, 6))
	}
	return sendInputs(a)
}
func pasteText(hwnd uintptr, v string, target destination) bool {
	buf := utf16.Encode([]rune(v + "\x00"))
	if u("OpenClipboard", hwnd) == 0 {
		return false
	}
	h := win(kernel, "GlobalAlloc", 2, uintptr(len(buf)*2))
	if h == 0 {
		u("CloseClipboard")
		return false
	}
	p := win(kernel, "GlobalLock", h)
	if p == 0 {
		win(kernel, "GlobalFree", h)
		u("CloseClipboard")
		return false
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(buf)), buf)
	win(kernel, "GlobalUnlock", h)
	success := false
	if u("EmptyClipboard") != 0 {
		success = u("SetClipboardData", 13, h) != 0
	}
	if !success {
		win(kernel, "GlobalFree", h)
	}
	u("CloseClipboard")
	return success && target.same(foreground()) && sendInputs([]input{keyInput(0x11, 0, 0), keyInput('V', 0, 0), keyInput('V', 0, 2), keyInput(0x11, 0, 2)})
}
func modifiersDown() bool {
	for _, k := range []uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c} {
		if u("GetAsyncKeyState", k)&0x8000 != 0 {
			return true
		}
	}
	return false
}
func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

type dataBlob struct {
	Len  uint32
	Data *byte
}

func protect(text string, encrypt bool) (string, error) {
	if text == "" {
		return "", nil
	}
	b := []byte(text)
	if !encrypt {
		var e error
		b, e = base64.StdEncoding.DecodeString(text)
		if e != nil {
			return "", e
		}
	}
	if len(b) == 0 {
		return "", fmt.Errorf("invalid credential")
	}
	in := dataBlob{uint32(len(b)), &b[0]}
	var out dataBlob
	n := "CryptProtectData"
	if !encrypt {
		n = "CryptUnprotectData"
	}
	ok := win(crypt, n, ptr(&in), 0, 0, 0, 0, 1, ptr(&out))
	runtime.KeepAlive(b)
	if ok == 0 || out.Data == nil {
		return "", fmt.Errorf("Windows could not access the saved API key")
	}
	defer win(kernel, "LocalFree", ptr(out.Data))
	ob := unsafe.Slice(out.Data, out.Len)
	if encrypt {
		return base64.StdEncoding.EncodeToString(ob), nil
	}
	return string(ob), nil
}
