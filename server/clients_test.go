package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRegistryRegisterSnapshot(t *testing.T) {
	r := NewClientRegistry()
	id1 := r.Register("iPhone", "10.0.0.2")
	r.Register("iPad", "10.0.0.3")
	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("want 2 clients, got %d", len(snap))
	}
	if snap[0].Name != "iPhone" { // sorted by Since ascending
		t.Errorf("want iPhone first, got %q", snap[0].Name)
	}
	r.Deregister(id1)
	if len(r.Snapshot()) != 1 {
		t.Errorf("want 1 after deregister, got %d", len(r.Snapshot()))
	}
}

func TestRegistryOnChange(t *testing.T) {
	r := NewClientRegistry()
	var n int32
	r.SetOnChange(func() { atomic.AddInt32(&n, 1) })
	id := r.Register("a", "1.1.1.1") // +1
	r.Deregister(id)                 // +1
	r.Deregister(id)                 // no-op, must not fire
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Errorf("want 2 onChange calls, got %d", got)
	}
}

func TestRegistrySnapshotIsolated(t *testing.T) {
	r := NewClientRegistry()
	r.Register("a", "1.1.1.1")
	snap := r.Snapshot()
	snap[0].Name = "mutated"
	if r.Snapshot()[0].Name != "a" {
		t.Error("Snapshot must return an isolated copy")
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewClientRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := r.Register("x", "1.1.1.1")
			_ = r.Snapshot()
			r.Deregister(id)
		}()
	}
	wg.Wait()
	if len(r.Snapshot()) != 0 {
		t.Errorf("want 0 after all deregister, got %d", len(r.Snapshot()))
	}
}
