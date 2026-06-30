package main

import (
	"fmt"
	"reflect"
	"testing"
)

// fakeInjector records each Injector call as a readable string. Shared across
// test files in package main.
type fakeInjector struct{ calls []string }

func (f *fakeInjector) MoveRel(dx, dy int)            { f.calls = append(f.calls, fmt.Sprintf("move %d %d", dx, dy)) }
func (f *fakeInjector) Button(b string, down bool)    { f.calls = append(f.calls, fmt.Sprintf("button %s %v", b, down)) }
func (f *fakeInjector) Scroll(dx, dy int)             { f.calls = append(f.calls, fmt.Sprintf("scroll %d %d", dx, dy)) }
func (f *fakeInjector) Text(s string)                 { f.calls = append(f.calls, "text "+s) }
func (f *fakeInjector) Key(code, mods int, down bool) { f.calls = append(f.calls, fmt.Sprintf("key %d %d %v", code, mods, down)) }
func (f *fakeInjector) Close()                        {}

// bptr returns a pointer to b, for the *bool fields on In.
func bptr(b bool) *bool { return &b }

func TestApplyEventClickExpandsToTwoButtons(t *testing.T) {
	f := &fakeInjector{}
	applyEvent(f, In{T: "click", B: "left"})
	want := []string{"button left true", "button left false"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("click calls = %v, want %v", f.calls, want)
	}
}

func TestApplyEventCoversAllPointerAndKeyTypes(t *testing.T) {
	f := &fakeInjector{}
	applyEvent(f, In{T: "move", Dx: 3, Dy: -4})
	applyEvent(f, In{T: "button", B: "right", Down: bptr(true)})
	applyEvent(f, In{T: "scroll", Dx: 0, Dy: 2})
	applyEvent(f, In{T: "text", S: "hi"})
	applyEvent(f, In{T: "key", Code: 65, Mods: 2, Down: bptr(false)})
	want := []string{"move 3 -4", "button right true", "scroll 0 2", "text hi", "key 65 2 false"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}
