//go:build !windows

package main

// run on non-Windows platforms only supports the standalone server.
func run(cfg appConfig) { serveStandalone(cfg) }
