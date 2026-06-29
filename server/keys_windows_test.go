//go:build windows

package main

import "testing"

func TestVK(t *testing.T) {
	if vkFor(KeyEnter) != 0x0D || vkFor(KeyArrowUp) != 0x26 || vkFor(KeyVolUp) != 0xAF {
		t.Fatal("vk map")
	}
	if vkFor(67) != 67 || vkFor(KeyF1) != 0x70 || vkFor(KeyF1+11) != 0x7B {
		t.Fatal("vk passthrough/F-keys")
	}
}
