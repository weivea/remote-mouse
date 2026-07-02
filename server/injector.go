package main

// Injector abstracts OS input injection for an already-logged-in session.
// Platform implementations live in injector_<goos>.go.
type Injector interface {
	MoveRel(dx, dy int)
	// MoveAbs positions the pointer at a normalized 0..65535 coordinate mapped
	// onto the primary screen (0,0 = top-left, 65535,65535 = bottom-right). Used
	// by the air-mouse absolute/calibrated mode.
	MoveAbs(nx, ny int)
	Button(btn string, down bool)
	Scroll(dx, dy int)
	Text(s string)
	Key(code, mods int, down bool)
	Close()
}
