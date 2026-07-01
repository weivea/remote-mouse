package main

import (
	"testing"
	"time"
)

func TestClientsCodecRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	in := []Client{
		{Name: "iPhone", Addr: "192.168.1.9", Since: now},
		{Name: "", Addr: "192.168.1.10", Since: now.Add(-time.Minute)},
	}
	b := encodeClients(in)
	out, err := decodeClients(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].Name != in[i].Name || out[i].Addr != in[i].Addr || !out[i].Since.Equal(in[i].Since) {
			t.Errorf("row %d = %+v, want %+v", i, out[i], in[i])
		}
	}
}

func TestDecodeClientsEmpty(t *testing.T) {
	out, err := decodeClients(encodeClients(nil))
	if err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %d", len(out))
	}
}
