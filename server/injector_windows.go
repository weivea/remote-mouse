//go:build windows

package main

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	moveRelF    = 0x0001
	leftDown    = 0x0002
	leftUp      = 0x0004
	rightDown   = 0x0008
	rightUp     = 0x0010
	midDown     = 0x0020
	midUp       = 0x0040
	wheel       = 0x0800
	hwheel      = 0x1000
	absoluteF   = 0x8000 // MOUSEEVENTF_ABSOLUTE
	virtualDesk = 0x4000 // MOUSEEVENTF_VIRTUALDESK
	keyUnicode  = 0x0004
	keyUp       = 0x0002
	keyExtended = 0x0001

	wheelDelta = 120

	smCXScreen        = 0
	smCYScreen        = 1
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
)

// input matches Win32 INPUT exactly (x64: 40 bytes). The mi field doubles as
// the union, sized to MOUSEINPUT (the largest variant we use); keyboard events
// are written over it via a keybdInput overlay. The struct sizes must match the
// Win32 types exactly, because SendInput rejects calls whose cbSize differs from
// sizeof(INPUT) and then injects nothing.
type input struct {
	typ uint32
	mi  mouseInput
}
type mouseInput struct {
	dx, dy    int32
	mouseData uint32
	flags     uint32
	time      uint32
	extra     uintptr
}
type keybdInput struct {
	vk, scan uint16
	flags    uint32
	time     uint32
	extra    uintptr
}

var sendInputWarned int32

// verboseInject, when set, makes sendMany log the result of every injection
// (event type, key vk / mouse flags, and the SendInput count/err). It is turned
// on for the secure-desktop agent so we get ground truth about which events the
// lock/logon desktop accepts vs. silently drops. Pure mouse-move spam is
// rate-limited; keyboard, buttons, scrolls and any short-count are always logged.
var (
	verboseInject int32
	lastMoveLogNs int64
)

func setVerboseInject(on bool) {
	if on {
		atomic.StoreInt32(&verboseInject, 1)
	} else {
		atomic.StoreInt32(&verboseInject, 0)
	}
}

// RM_INJECTLOG=1 turns on per-event injection logging for any mode (used to
// diagnose the standalone process's behavior on the lock screen); it also
// redirects logging to %ProgramData%\RemoteMouse\standalone.log so the evidence
// survives a lock. The secure-desktop agent enables verbose logging on its own.
func init() {
	if os.Getenv("RM_INJECTLOG") != "" {
		setupFileLogger("standalone")
		setVerboseInject(true)
	}
}

func sendMany(ins []input) {
	if len(ins) == 0 {
		return
	}
	n, _, err := procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
	short := int(n) != len(ins)
	if atomic.LoadInt32(&verboseInject) == 1 {
		logInject(ins, int(n), err, short)
	}
	// SendInput returns the number of events actually inserted; a short count
	// means the injection was blocked (e.g. wrong cbSize, or UIPI when the
	// foreground window is elevated). Log once so it isn't a silent no-op.
	if short && atomic.CompareAndSwapInt32(&sendInputWarned, 0, 1) {
		log.Printf("SendInput injected %d/%d events (input blocked): %v", int(n), len(ins), err)
	}
}

// logInject emits one line per injection batch for lock-screen diagnostics. It
// deliberately never logs typed CONTENT: a Unicode TEXT event carries the actual
// character in wScan (this is the PIN-entry path on the secure desktop), so only
// its shape (flags/count/result) is logged, never the code point.
func logInject(ins []input, n int, err error, short bool) {
	first := ins[0]
	if first.typ == inputKeyboard {
		k := *(*keybdInput)(unsafe.Pointer(&first.mi))
		if k.flags&keyUnicode != 0 {
			log.Printf("inject TEXT flags=0x%X count=%d -> n=%d err=%v", k.flags, len(ins), n, err)
			return
		}
		log.Printf("inject KEY vk=0x%02X scan=0x%04X flags=0x%X count=%d -> n=%d err=%v", k.vk, k.scan, k.flags, len(ins), n, err)
		return
	}
	isMove := len(ins) == 1 && first.mi.flags&moveRelF != 0
	if isMove && !short {
		now := time.Now().UnixNano()
		if now-atomic.LoadInt64(&lastMoveLogNs) < int64(time.Second) {
			return
		}
		atomic.StoreInt64(&lastMoveLogNs, now)
	}
	log.Printf("inject MOUSE dx=%d dy=%d flags=0x%X count=%d -> n=%d err=%v", first.mi.dx, first.mi.dy, first.mi.flags, len(ins), n, err)
}

func mouse(mi mouseInput) input { return input{typ: inputMouse, mi: mi} }

func keyVK(vk uint16, up bool) input {
	k := keybdInput{vk: vk}
	if vk == 0xAD || (vk >= 0xAE && vk <= 0xB3) {
		k.flags |= keyExtended
	}
	if up {
		k.flags |= keyUp
	}
	in := input{typ: inputKeyboard}
	*(*keybdInput)(unsafe.Pointer(&in.mi)) = k
	return in
}

// winInjector injects via SendInput. In relative mode (the default, used on the
// ordinary Default desktop) it forwards raw MOUSEEVENTF_MOVE deltas so the OS
// pointer-acceleration curve applies. In absolute mode it tracks a virtual
// cursor position and emits MOUSEEVENTF_ABSOLUTE moves: the Windows secure
// desktop (lock / logon / UAC) silently ignores relative mouse moves, so
// absolute positioning is the only way to drive the pointer there. Keyboard
// SendInput is accepted on the secure desktop regardless, which is why text
// injection works on the lock screen but relative mouse moves do not.
type winInjector struct {
	absolute bool

	mu     sync.Mutex
	seeded bool
	x, y   int32 // tracked pointer position in virtual-desktop pixels
}

func newInjector() Injector       { return &winInjector{} }
func newSecureInjector() Injector { return &winInjector{absolute: true} }

func (w *winInjector) MoveRel(dx, dy int) {
	if !w.absolute {
		sendMany([]input{mouse(mouseInput{dx: int32(dx), dy: int32(dy), flags: moveRelF})})
		return
	}
	sendMany([]input{w.absMove(dx, dy)})
}
func (w *winInjector) Button(btn string, down bool) {
	f := buttonFlag(btn, down)
	if !w.absolute {
		sendMany([]input{mouse(mouseInput{flags: f})})
		return
	}
	// Re-assert the tracked absolute position so the click lands where the user
	// last moved to; the secure desktop discards our relative moves otherwise.
	sendMany([]input{w.absMove(0, 0), mouse(mouseInput{flags: f})})
}

// absMove accumulates a relative delta into the tracked pointer position and
// returns an absolute MOUSEEVENTF_ABSOLUTE|VIRTUALDESK move for it. The position
// is seeded from the live cursor on first use so control begins where the
// pointer already is.
func (w *winInjector) absMove(dx, dy int) input {
	w.mu.Lock()
	defer w.mu.Unlock()
	ox, oy, cw, ch := virtualScreen()
	if !w.seeded {
		if px, py, ok := cursorPos(); ok {
			w.x, w.y = px, py
		} else {
			w.x, w.y = ox+cw/2, oy+ch/2
		}
		w.seeded = true
	}
	w.x = clampI32(w.x+int32(dx), ox, ox+cw-1)
	w.y = clampI32(w.y+int32(dy), oy, oy+ch-1)
	nx, ny := normAbs(w.x, w.y, ox, oy, cw, ch)
	return mouse(mouseInput{dx: nx, dy: ny, flags: moveRelF | absoluteF | virtualDesk})
}

func buttonFlag(btn string, down bool) uint32 {
	switch btn {
	case "right":
		if down {
			return rightDown
		}
		return rightUp
	case "middle":
		if down {
			return midDown
		}
		return midUp
	default:
		if down {
			return leftDown
		}
		return leftUp
	}
}

// normAbs maps a virtual-desktop pixel position to SendInput's 0..65535 absolute
// coordinate space, relative to the virtual-screen origin (ox,oy) and size
// (cw,ch). Used with MOUSEEVENTF_VIRTUALDESK so it spans all monitors.
func normAbs(x, y, ox, oy, cw, ch int32) (int32, int32) {
	nx := int32(int64(clampI32(x, ox, ox+cw-1)-ox) * 65535 / int64(maxI32(cw-1, 1)))
	ny := int32(int64(clampI32(y, oy, oy+ch-1)-oy) * 65535 / int64(maxI32(ch-1, 1)))
	return nx, ny
}

func clampI32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxI32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

// virtualScreen returns the bounding box of all monitors in pixels, falling back
// to the primary monitor if the virtual-screen metrics are unavailable.
func virtualScreen() (x, y, w, h int32) {
	x = sysMetric(smXVirtualScreen)
	y = sysMetric(smYVirtualScreen)
	w = sysMetric(smCXVirtualScreen)
	h = sysMetric(smCYVirtualScreen)
	if w <= 0 {
		w = sysMetric(smCXScreen)
	}
	if h <= 0 {
		h = sysMetric(smCYScreen)
	}
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return
}

func sysMetric(i int) int32 {
	r, _, _ := procGetSystemMetrics.Call(uintptr(i))
	return int32(r)
}

func cursorPos() (x, y int32, ok bool) {
	var p struct{ x, y int32 }
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p.x, p.y, r != 0
}
func (w *winInjector) Scroll(dx, dy int) {
	var ins []input
	if dy != 0 {
		ins = append(ins, mouse(mouseInput{mouseData: uint32(int32(dy) * wheelDelta), flags: wheel}))
	}
	if dx != 0 {
		ins = append(ins, mouse(mouseInput{mouseData: uint32(int32(dx) * wheelDelta), flags: hwheel}))
	}
	sendMany(ins)
}
func (w *winInjector) Text(s string) {
	var ins []input
	for _, r := range s {
		for _, u := range utf16Units(r) {
			d := keybdInput{scan: u, flags: keyUnicode}
			down := input{typ: inputKeyboard}
			*(*keybdInput)(unsafe.Pointer(&down.mi)) = d
			d.flags = keyUnicode | keyUp
			up := input{typ: inputKeyboard}
			*(*keybdInput)(unsafe.Pointer(&up.mi)) = d
			ins = append(ins, down, up)
		}
	}
	sendMany(ins)
}

// Key presses/releases a non-text key with modifiers. On down we press mods
// then the key; on up we release the key then mods (reverse), so chords work.
func (w *winInjector) Key(code, mods int, down bool) {
	vk := vkFor(code)
	if vk == 0 {
		return
	}
	var ins []input
	if down {
		ins = append(ins, modKeys(mods, false)...)
		ins = append(ins, keyVK(vk, false))
	} else {
		ins = append(ins, keyVK(vk, true))
		ins = append(ins, modKeys(mods, true)...)
	}
	sendMany(ins)
}
func (w *winInjector) Close() {}

func modKeys(mods int, up bool) []input {
	var ins []input
	if mods&ModCtrl != 0 {
		ins = append(ins, keyVK(0x11, up))
	}
	if mods&ModShift != 0 {
		ins = append(ins, keyVK(0x10, up))
	}
	if mods&ModAlt != 0 {
		ins = append(ins, keyVK(0x12, up))
	}
	if mods&ModMeta != 0 {
		ins = append(ins, keyVK(0x5B, up))
	}
	return ins
}

func utf16Units(r rune) []uint16 {
	if r > 0xFFFF {
		r -= 0x10000
		return []uint16{uint16(0xD800 + (r >> 10)), uint16(0xDC00 + (r & 0x3FF))}
	}
	return []uint16{uint16(r)}
}

func vkFor(code int) uint16 {
	switch {
	case code >= 65 && code <= 90, code >= 48 && code <= 57:
		return uint16(code)
	case code >= KeyF1 && code <= KeyF1+11:
		return uint16(0x70 + (code - KeyF1))
	}
	switch code {
	case KeyEnter:
		return 0x0D
	case KeyBack:
		return 0x08
	case KeyTab:
		return 0x09
	case KeyEsc:
		return 0x1B
	case KeyDel:
		return 0x2E
	case KeyArrowLeft:
		return 0x25
	case KeyArrowUp:
		return 0x26
	case KeyArrowRight:
		return 0x27
	case KeyArrowDown:
		return 0x28
	case KeyHome:
		return 0x24
	case KeyEnd:
		return 0x23
	case KeyPgUp:
		return 0x21
	case KeyPgDn:
		return 0x22
	case KeyMute:
		return 0xAD
	case KeyVolDown:
		return 0xAE
	case KeyVolUp:
		return 0xAF
	case KeyNext:
		return 0xB0
	case KeyPrev:
		return 0xB1
	case KeyPlayPause:
		return 0xB3
	}
	return 0
}
