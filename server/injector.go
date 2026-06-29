package main

// Injector abstracts OS input injection for an already-logged-in session.
// Platform implementations live in injector_<goos>.go.
type Injector interface {
	MoveRel(dx, dy int)
	Button(btn string, down bool)
	Scroll(dx, dy int)
	Text(s string)
	Key(code, mods int, down bool)
	Close()
}
