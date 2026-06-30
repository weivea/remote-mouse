package main

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

// pipeInjector implements Injector by serialising each call as a newline-JSON
// In event onto a writer (the named pipe to the agent). The writer is swappable
// under a mutex so the service can rebind to a freshly spawned agent when the
// input desktop changes. The mutex is held across the write so concurrent
// client goroutines cannot interleave bytes on the pipe.
type pipeInjector struct {
	mu sync.Mutex
	w  io.Writer
}

func newPipeInjector(w io.Writer) *pipeInjector { return &pipeInjector{w: w} }

func (p *pipeInjector) setWriter(w io.Writer) {
	p.mu.Lock()
	p.w = w
	p.mu.Unlock()
}

func (p *pipeInjector) emit(m In) {
	b, err := encode(m)
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.w == nil {
		return
	}
	p.w.Write(b)
}

func (p *pipeInjector) MoveRel(dx, dy int) { p.emit(In{T: "move", Dx: dx, Dy: dy}) }
func (p *pipeInjector) Button(b string, down bool) {
	d := down
	p.emit(In{T: "button", B: b, Down: &d})
}
func (p *pipeInjector) Scroll(dx, dy int) { p.emit(In{T: "scroll", Dx: dx, Dy: dy}) }
func (p *pipeInjector) Text(s string)     { p.emit(In{T: "text", S: s}) }
func (p *pipeInjector) Key(code, mods int, down bool) {
	d := down
	p.emit(In{T: "key", Code: code, Mods: mods, Down: &d})
}
func (p *pipeInjector) Close() {}

// runAgentLoop reads newline-delimited In events from r and applies each to inj
// until r is exhausted. The agent process uses it to drive SendInput from
// events the service forwards over the pipe.
func runAgentLoop(r io.Reader, inj Injector) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var m In
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		applyEvent(inj, m)
	}
}
