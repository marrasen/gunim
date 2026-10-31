//go:build !darwin

package vst3

// onMain reports that the package's own thread runs f: on this system a
// plugin's editor needs no thread of its own.
func onMain(func()) bool { return false }
