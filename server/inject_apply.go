package main

// applyEvent dispatches a single client event to an injector. It handles the
// pointer/keyboard event types only; control-plane messages (ping/bye) are the
// caller's responsibility. "click" expands to a press + release so that, when
// inj is a pipeInjector, the expansion happens on the service side and the
// agent only ever sees primitive button events. Shared by the in-process
// server (server.go) and the agent pipe loop (pipe_inject.go).
func applyEvent(inj Injector, m In) {
	switch m.T {
	case "move":
		inj.MoveRel(m.Dx, m.Dy)
	case "button":
		inj.Button(m.B, m.Down != nil && *m.Down)
	case "click":
		inj.Button(m.B, true)
		inj.Button(m.B, false)
	case "scroll":
		inj.Scroll(m.Dx, m.Dy)
	case "text":
		inj.Text(m.S)
	case "key":
		inj.Key(m.Code, m.Mods, m.Down != nil && *m.Down)
	}
}
