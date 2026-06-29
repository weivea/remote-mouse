package main

import "testing"

func TestKeyCodes(t *testing.T) {
	if ModCtrl != 1 || ModAlt != 2 || ModShift != 4 || ModMeta != 8 {
		t.Fatal("mod bits wrong")
	}
	if KeyEnter != 1 || KeyArrowUp != 10 || KeyF1 != 30 || KeyVolUp != 201 {
		t.Fatal("keycode constants wrong")
	}
}
