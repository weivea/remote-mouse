package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestPipeInjectorRoundTripsThroughAgentLoop(t *testing.T) {
	// pipeInjector serialises calls as newline-JSON In events into a buffer;
	// runAgentLoop reads them back and drives a fakeInjector. Asserts events
	// survive the round trip, including click -> two buttons.
	var buf bytes.Buffer
	pi := newPipeInjector(&buf)
	applyEvent(pi, In{T: "move", Dx: 5, Dy: -7})
	applyEvent(pi, In{T: "click", B: "left"})
	applyEvent(pi, In{T: "text", S: "hi"})

	f := &fakeInjector{}
	runAgentLoop(&buf, f)

	want := []string{"move 5 -7", "button left true", "button left false", "text hi"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}

func TestPipeInjectorSetWriterSwapsTarget(t *testing.T) {
	var a, b bytes.Buffer
	pi := newPipeInjector(&a)
	pi.MoveRel(1, 1)
	pi.setWriter(&b)
	pi.MoveRel(2, 2)
	if a.Len() == 0 || b.Len() == 0 {
		t.Fatalf("expected writes to both buffers, a=%d b=%d", a.Len(), b.Len())
	}
}

func TestPipeInjectorNilWriterIsNoop(t *testing.T) {
	pi := newPipeInjector(nil)
	pi.MoveRel(1, 1) // must not panic
}
