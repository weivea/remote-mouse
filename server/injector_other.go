//go:build !darwin && !windows

package main

import "log"

type stubInjector struct{}

func newInjector() Injector { return &stubInjector{} }

func (stubInjector) MoveRel(dx, dy int)        { log.Printf("move %d,%d", dx, dy) }
func (stubInjector) Button(btn string, d bool) { log.Printf("button %s down=%v", btn, d) }
func (stubInjector) Scroll(dx, dy int)         { log.Printf("scroll %d,%d", dx, dy) }
func (stubInjector) Text(s string)             { log.Printf("text %q", s) }
func (stubInjector) Close()                    {}
