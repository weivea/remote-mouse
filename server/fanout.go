package main

import (
	"io"
	"time"
)

// deadlineWriter is a writer whose blocking can be bounded with a write
// deadline. *net.Conn (the pipe to an agent) satisfies it. Kept as an interface
// so fan-out is unit-testable without real pipes.
type deadlineWriter interface {
	io.Writer
	SetWriteDeadline(t time.Time) error
}

// fanoutWriter broadcasts each write to every target, bounding each target's
// write with timeout so one hung agent cannot wedge injection for the others.
// Per-target errors are swallowed: a dead/slow agent's pipe must not stop the
// event reaching the agent on the desktop that will actually accept it. The
// caller (pipeInjector.emit) holds its mutex across the whole Write, so writes
// to a single target never interleave.
type fanoutWriter struct {
	ws      []deadlineWriter
	timeout time.Duration
}

func (f *fanoutWriter) Write(b []byte) (int, error) {
	for _, w := range f.ws {
		if f.timeout > 0 {
			_ = w.SetWriteDeadline(time.Now().Add(f.timeout))
		}
		_, _ = w.Write(b)
	}
	return len(b), nil
}
