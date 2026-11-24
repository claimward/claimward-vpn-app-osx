//go:build !darwin

package main

// activate has nothing to do off macOS: the app is built for darwin, and this
// stub only lets the platform-independent lint (mvvmlint) type-check main.
func activate() {}
