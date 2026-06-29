package main

// Neutral, cross-platform keycode + modifier table. The client sends
// {t:"key", code, down, mods}; each platform injector maps code→native key.
// Printable text still flows via {t:"text"}; key is for editing/nav/shortcut/media.

const (
	ModCtrl  = 1
	ModAlt   = 2
	ModShift = 4
	ModMeta  = 8 // Win key / Cmd
)

const (
	KeyEnter = 1
	KeyBack  = 2
	KeyTab   = 3
	KeyEsc   = 4
	KeyDel   = 5

	KeyArrowUp    = 10
	KeyArrowDown  = 11
	KeyArrowLeft  = 12
	KeyArrowRight = 13

	KeyHome = 20
	KeyEnd  = 21
	KeyPgUp = 22
	KeyPgDn = 23

	KeyF1 = 30 // F1..F12 = 30..41

	KeyVolDown   = 200
	KeyVolUp     = 201
	KeyMute      = 202
	KeyPlayPause = 203
	KeyNext      = 204
	KeyPrev      = 205
)

// Letters use uppercase ASCII A–Z (65–90) and digits use '0'–'9' (48–57),
// combined with mods for shortcuts (e.g. Ctrl+C = code 67, mods ModCtrl).
