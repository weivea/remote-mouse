//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	moveRelF   = 0x0001
	leftDown   = 0x0002
	leftUp     = 0x0004
	rightDown  = 0x0008
	rightUp    = 0x0010
	midDown    = 0x0020
	midUp      = 0x0040
	wheel      = 0x0800
	hwheel     = 0x1000
	keyUnicode = 0x0004
	keyUp      = 0x0002
)

// INPUT union sized for the mouse variant (largest fields we use).
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
	vk, scan  uint16
	flags     uint32
	time      uint32
	extra     uintptr
	_, _      uint32
}

func send(in *input) { procSendInput.Call(1, uintptr(unsafe.Pointer(in)), unsafe.Sizeof(*in)) }

type winInjector struct{}

func newInjector() Injector { return &winInjector{} }

func (winInjector) MoveRel(dx, dy int) {
	send(&input{typ: inputMouse, mi: mouseInput{dx: int32(dx), dy: int32(dy), flags: moveRelF}})
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
	send(&input{typ: inputMouse, mi: mouseInput{flags: f}})
}
func (winInjector) Scroll(dx, dy int) {
	if dy != 0 {
		send(&input{typ: inputMouse, mi: mouseInput{mouseData: uint32(int32(dy)), flags: wheel}})
	}
	if dx != 0 {
		send(&input{typ: inputMouse, mi: mouseInput{mouseData: uint32(int32(dx)), flags: hwheel}})
	}
}
func (winInjector) Text(s string) {
	for _, r := range s {
		k := keybdInput{scan: uint16(r), flags: keyUnicode}
		var down input
		down.typ = inputKeyboard
		*(*keybdInput)(unsafe.Pointer(&down.mi)) = k
		send(&down)
		k.flags = keyUnicode | keyUp
		var up input
		up.typ = inputKeyboard
		*(*keybdInput)(unsafe.Pointer(&up.mi)) = k
		send(&up)
	}
}
func (winInjector) Close() {}
