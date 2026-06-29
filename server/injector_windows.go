//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
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
	keyUnicode  = 0x0004
	keyUp       = 0x0002
	keyExtended = 0x0001

	wheelDelta = 120
)

// input matches Win32 INPUT; the union is sized to MOUSEINPUT (the largest
// variant we use). Keyboard fields are written via keybdInput overlay.
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
	_         uint32
}
type keybdInput struct {
	vk, scan uint16
	flags    uint32
	time     uint32
	extra    uintptr
	_, _     uint32
}

func sendMany(ins []input) {
	if len(ins) == 0 {
		return
	}
	procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
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

type winInjector struct{}

func newInjector() Injector { return &winInjector{} }

func (winInjector) MoveRel(dx, dy int) {
	sendMany([]input{mouse(mouseInput{dx: int32(dx), dy: int32(dy), flags: moveRelF})})
}
func (winInjector) Button(btn string, down bool) {
	var f uint32
	switch btn {
	case "right":
		f = map[bool]uint32{true: rightDown, false: rightUp}[down]
	case "middle":
		f = map[bool]uint32{true: midDown, false: midUp}[down]
	default:
		f = map[bool]uint32{true: leftDown, false: leftUp}[down]
	}
	sendMany([]input{mouse(mouseInput{flags: f})})
}
func (winInjector) Scroll(dx, dy int) {
	var ins []input
	if dy != 0 {
		ins = append(ins, mouse(mouseInput{mouseData: uint32(int32(dy) * wheelDelta), flags: wheel}))
	}
	if dx != 0 {
		ins = append(ins, mouse(mouseInput{mouseData: uint32(int32(dx) * wheelDelta), flags: hwheel}))
	}
	sendMany(ins)
}
func (winInjector) Text(s string) {
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
func (winInjector) Key(code, mods int, down bool) {
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
func (winInjector) Close() {}

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
