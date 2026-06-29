//go:build !windows

package main

// Non-Windows builds have no tray: just block forever while listen runs.
func runUI(notray bool, pass string, port int) { select {} }
