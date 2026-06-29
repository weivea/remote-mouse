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
