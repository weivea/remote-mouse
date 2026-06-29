# Windows Server (Full Input + Tray + Autostart) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Windows Go server run for real (mouse/scroll/text), add keyboard/shortcut/media-key input, a tray icon with registry autostart, a build script, and wire the iOS client to send keys.

**Architecture:** Add a shared neutral keycode table (`keys.go`) and extend the `Injector` interface with `Key(code, mods, down)`. Windows/macOS/stub implement it. A windows-only `tray_windows.go` adds systray + autostart; `main.go` gains `-notray`. iOS gains a key bar.

**Tech Stack:** Go 1.26 (cgo on macOS), `getlantern/systray`, golang.org/x/sys/windows/registry, SwiftUI.

---

## File Structure
- `server/keys.go` — neutral keycode + mods constants (cross-platform).
- `server/keys_test.go` — tests for VK mapping (windows) / parity.
- `server/injector.go` — add `Key`.
- `server/injector_windows.go` — fix INPUT sizing, batch SendInput, code→VK, media.
- `server/injector_darwin.go` — Key via CGKeyCode + flags.
- `server/injector_other.go` — stub Key.
- `server/tray_windows.go` — systray + HKCU Run autostart.
- `server/tray_other.go` — no-op runTray for non-windows.
- `server/main.go` — `-notray`, hand off to tray on windows.
- `server/build.ps1` — build rmserver.exe.
- `proto/protocol-mvp.md`, `docs/12-progress.md` — docs.
- `ios/Sources/Views/TouchpadView.swift`, `ios/Sources/Models/Client.swift` — key bar + send.

---

### Task 1: Shared keycode table

**Files:**
- Create: `server/keys.go`
- Test: `server/keys_test.go`

- [ ] **Step 1: Write the failing test**
```go
package main

import "testing"

func TestKeyCodes(t *testing.T) {
	if ModCtrl != 1 || ModAlt != 2 || ModShift != 4 || ModMeta != 8 {
		t.Fatal("mod bits wrong")
	}
	if KeyEnter != 1 || KeyArrowUp != 10 || KeyF1 != 30 || KeyVolUp != 51 {
		t.Fatal("keycode constants wrong")
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — `go test ./... -run TestKeyCodes` → undefined constants.

- [ ] **Step 3: Implement**
```go
package main

const (
	ModCtrl  = 1
	ModAlt   = 2
	ModShift = 4
	ModMeta  = 8
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

	KeyVolDown    = 50
	KeyVolUp      = 51
	KeyMute       = 52
	KeyPlayPause  = 53
	KeyNext       = 54
	KeyPrev       = 55
)
```

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestKeyCodes`.
- [ ] **Step 5: Commit** — `git add server/keys.go server/keys_test.go && git commit -m "feat(server): shared neutral keycode table"`

### Task 2: Extend Injector interface + stub + handler

**Files:** Modify `server/injector.go`, `server/injector_other.go`, `server/server.go`

- [ ] **Step 1:** Add to `injector.go` interface: `Key(code, mods int, down bool)`.
- [ ] **Step 2:** Add to `injector_other.go`: `func (stubInjector) Key(c, m int, d bool){ log.Printf("key %d mods %d down=%v", c, m, d) }`.
- [ ] **Step 3:** In `server.go` switch add:
```go
case "key":
	s.inj.Key(m.Code, m.Mods, m.Down != nil && *m.Down)
```
- [ ] **Step 4:** `go build ./...` on a non-darwin/non-win machine still ok (stub). Commit: `feat(server): Key in injector interface + handler`.

### Task 3: Windows injector — fix sizing + Key + media

**Files:** Modify `server/injector_windows.go`; add cases to `keys_test.go` (windows build).

- [ ] **Step 1: Test (windows-only)** append:
```go
//go:build windows
func TestVK(t *testing.T){ if vkFor(KeyEnter)!=0x0D||vkFor(KeyArrowUp)!=0x26||vkFor(KeyVolUp)!=0xAF{t.Fatal("vk map")} }
```
- [ ] **Step 2:** `go test -run TestVK` → FAIL (no vkFor).
- [ ] **Step 3:** Add `vkFor(code) uint16` map (enter=0x0D, back=0x08, tab=0x09, esc=0x1B, del=0x2E, arrows 0x25-0x28, home/end 0x24/0x23, pgup/pgdn 0x21/0x22, F1-F12 0x70+, vol 0xAE-0xB0, media 0xB0-0xB3, letters/digits = code). `Key`: press mod VKs, main, release reverse; flags KEYEVENTF_KEYUP on up.
- [ ] **Step 4:** `go test -run TestVK` PASS; `go vet`.
- [ ] **Step 5: Commit** — `fix(server): windows SendInput sizing + key/media via VK`.

### Task 4: macOS Key parity

**Files:** Modify `server/injector_darwin.go` (add C KeyTap + flags; arrows/edit only, media as log). Build: `GOOS=darwin go build` (verify on mac later). Commit.

### Task 5: Tray + autostart (windows) + main flag

**Files:** Create `server/tray_windows.go`, `server/tray_other.go`; modify `main.go` (`-notray`, registry path); `go get github.com/getlantern/systray`. systray menu: show pass/port, toggle autostart (HKCU Run), quit. Commit `feat(server): tray + registry autostart`.

### Task 6: build.ps1 + docs

**Files:** Create `server/build.ps1` (`$env:GOOS='windows'; go build -o rmserver.exe .`); update `proto/protocol-mvp.md` key table + `docs/12-progress.md`. Commit.

### Task 7: iOS key bar (build later on mac)

**Files:** `TouchpadView.swift` add arrows/enter/back/esc/copy/paste/media buttons; `Client.swift` add `sendKey(code,mods)`. Commit `feat(ios): key bar (untested, mac build pending)`.
