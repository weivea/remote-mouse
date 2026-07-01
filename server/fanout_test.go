package main

import (
	"errors"
	"testing"
	"time"
)

// recWriter is a deadlineWriter test double that records bytes written and can
// be told to fail, so we can assert fan-out keeps writing past a bad target.
type recWriter struct {
	got      []byte
	deadline time.Time
	err      error
}

func (r *recWriter) Write(b []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	r.got = append(r.got, b...)
	return len(b), nil
}
func (r *recWriter) SetWriteDeadline(t time.Time) error { r.deadline = t; return nil }

func TestFanoutWriterBroadcastsToAll(t *testing.T) {
	a, b := &recWriter{}, &recWriter{}
	f := &fanoutWriter{ws: []deadlineWriter{a, b}, timeout: time.Second}
	n, err := f.Write([]byte("hello\n"))
	if err != nil || n != 6 {
		t.Fatalf("Write = %d,%v; want 6,nil", n, err)
	}
	if string(a.got) != "hello\n" || string(b.got) != "hello\n" {
		t.Fatalf("a=%q b=%q; want both hello\\n", a.got, b.got)
	}
	if a.deadline.IsZero() || b.deadline.IsZero() {
		t.Fatalf("expected a write deadline to be set on each target")
	}
}

func TestFanoutWriterContinuesPastError(t *testing.T) {
	bad := &recWriter{err: errors.New("broken pipe")}
	good := &recWriter{}
	f := &fanoutWriter{ws: []deadlineWriter{bad, good}, timeout: time.Second}
	if _, err := f.Write([]byte("x")); err != nil {
		t.Fatalf("Write returned err %v; a failing target must not fail the fan-out", err)
	}
	if string(good.got) != "x" {
		t.Fatalf("good target got %q; want x (must write past the failing target)", good.got)
	}
}

func TestFanoutWriterNoTargetsIsNoop(t *testing.T) {
	f := &fanoutWriter{}
	if n, err := f.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatalf("empty fan-out Write = %d,%v; want 3,nil", n, err)
	}
}
