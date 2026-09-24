//go:build linux || darwin

package desktop

// raiseThread leaves the thread's priority as it is. Raising it on
// Linux and macOS needs privileges an application seldom has.
func raiseThread() {}
