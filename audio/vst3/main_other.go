//go:build !darwin

package vst3

// onMain reports that the package's own thread runs f: on this system a
// plugin's editor needs no thread of its own.
func onMain(func()) bool { return false }

// ticksElsewhere says the package's own thread runs the tickers.
func ticksElsewhere() bool { return false }

// onMainThread is for macOS.
func onMainThread() bool { return false }
