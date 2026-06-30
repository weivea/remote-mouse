package main

import (
	"sort"
	"sync"
	"time"
)

// Client is one authenticated, currently-connected device.
type Client struct {
	Name  string
	Addr  string
	Since time.Time

	seq uint64 // insertion order; tiebreaker when Since is equal (coarse clocks)
}

// ClientRegistry tracks online connections and notifies a subscriber on change.
// Safe for concurrent use.
type ClientRegistry struct {
	mu       sync.RWMutex
	clients  map[uint64]Client
	nextID   uint64
	onChange func()
}

func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{clients: make(map[uint64]Client)}
}

// Register adds a connection and returns its id. Triggers onChange.
func (r *ClientRegistry) Register(name, addr string) uint64 {
	r.mu.Lock()
	r.nextID++
	id := r.nextID
	r.clients[id] = Client{Name: name, Addr: addr, Since: time.Now(), seq: id}
	r.mu.Unlock()
	r.notify()
	return id
}

// Deregister removes a connection by id. Triggers onChange only if it existed.
func (r *ClientRegistry) Deregister(id uint64) {
	r.mu.Lock()
	_, ok := r.clients[id]
	delete(r.clients, id)
	r.mu.Unlock()
	if ok {
		r.notify()
	}
}

// Snapshot returns an isolated copy of current clients, sorted by Since ascending.
func (r *ClientRegistry) Snapshot() []Client {
	r.mu.RLock()
	out := make([]Client, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, c)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Since.Equal(out[j].Since) {
			return out[i].seq < out[j].seq
		}
		return out[i].Since.Before(out[j].Since)
	})
	return out
}

// SetOnChange registers the change callback (UI). Called once at setup.
func (r *ClientRegistry) SetOnChange(f func()) {
	r.mu.Lock()
	r.onChange = f
	r.mu.Unlock()
}

func (r *ClientRegistry) notify() {
	r.mu.RLock()
	f := r.onChange
	r.mu.RUnlock()
	if f != nil {
		f()
	}
}
