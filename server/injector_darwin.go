//go:build darwin

package main

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>

static void moveRel(int dx, int dy) {
    CGEventRef e = CGEventCreate(NULL);
    CGPoint p = CGEventGetLocation(e);
    CFRelease(e);
    CGPoint np = CGPointMake(p.x + dx, p.y + dy);
    CGEventRef m = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, np, kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, m);
    CFRelease(m);
}

static void button(int btn, int down) {
    CGEventRef e = CGEventCreate(NULL);
    CGPoint p = CGEventGetLocation(e);
    CFRelease(e);
    CGMouseButton b = kCGMouseButtonLeft;
    CGEventType t;
    if (btn == 1) { b = kCGMouseButtonRight; t = down ? kCGEventRightMouseDown : kCGEventRightMouseUp; }
    else if (btn == 2) { b = kCGMouseButtonCenter; t = down ? kCGEventOtherMouseDown : kCGEventOtherMouseUp; }
    else { b = kCGMouseButtonLeft; t = down ? kCGEventLeftMouseDown : kCGEventLeftMouseUp; }
    CGEventRef m = CGEventCreateMouseEvent(NULL, t, p, b);
    CGEventPost(kCGHIDEventTap, m);
    CFRelease(m);
}

static void scroll(int dx, int dy) {
    CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitPixel, 2, dy, dx);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}

static void rmTypeText(unsigned short* chars, int n) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    CGEventKeyboardSetUnicodeString(down, n, chars);
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    CGEventKeyboardSetUnicodeString(up, n, chars);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);
}

static void keyEvent(int vk, unsigned long flags, int down) {
    CGEventRef e = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)vk, down != 0);
    CGEventSetFlags(e, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}
*/
import "C"

import "unsafe"

type macInjector struct{}

func newInjector() Injector { return &macInjector{} }

func (m *macInjector) MoveRel(dx, dy int) { C.moveRel(C.int(dx), C.int(dy)) }

func (m *macInjector) Button(btn string, down bool) {
	d := C.int(0)
	if down {
		d = 1
	}
	C.button(C.int(btnCode(btn)), d)
}

func (m *macInjector) Scroll(dx, dy int) { C.scroll(C.int(dx), C.int(dy)) }

func (m *macInjector) Text(s string) {
	u := []uint16{}
	for _, r := range s {
		if r > 0xFFFF { // surrogate pair for emoji
			r -= 0x10000
			u = append(u, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			u = append(u, uint16(r))
		}
	}
	if len(u) == 0 {
		return
	}
	C.rmTypeText((*C.ushort)(unsafe.Pointer(&u[0])), C.int(len(u)))
}

func (m *macInjector) Close() {}

// macKey maps neutral codes to macOS CGKeyCodes (-1 = unmapped). Letters cover
// common shortcut keys; arrows/edit/nav/F-keys are full. Media handled separately.
func macKey(code int) int {
	switch code {
	case KeyEnter:
		return 36
	case KeyBack:
		return 51
	case KeyTab:
		return 48
	case KeyEsc:
		return 53
	case KeyDel:
		return 117
	case KeyArrowLeft:
		return 123
	case KeyArrowRight:
		return 124
	case KeyArrowDown:
		return 125
	case KeyArrowUp:
		return 126
	case KeyHome:
		return 115
	case KeyEnd:
		return 119
	case KeyPgUp:
		return 116
	case KeyPgDn:
		return 121
	case 'A':
		return 0
	case 'C':
		return 8
	case 'V':
		return 9
	case 'X':
		return 7
	case 'Z':
		return 6
	}
	if code >= KeyF1 && code <= KeyF1+11 {
		fk := []int{122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111}
		return fk[code-KeyF1]
	}
	return -1
}

func macFlags(mods int) uint64 {
	var f uint64
	if mods&ModMeta != 0 {
		f |= 0x100000
	}
	if mods&ModShift != 0 {
		f |= 0x20000
	}
	if mods&ModCtrl != 0 {
		f |= 0x40000
	}
	if mods&ModAlt != 0 {
		f |= 0x80000
	}
	return f
}

func (m *macInjector) Key(code, mods int, down bool) {
	vk := macKey(code)
	if vk < 0 {
		return // media/unmapped: stubbed until later milestone
	}
	d := C.int(0)
	if down {
		d = 1
	}
	C.keyEvent(C.int(vk), C.ulong(macFlags(mods)), d)
}

func btnCode(b string) int {
	switch b {
	case "right":
		return 1
	case "middle":
		return 2
	default:
		return 0
	}
}
