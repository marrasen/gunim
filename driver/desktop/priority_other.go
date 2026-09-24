//go:build linux || darwin

package desktop

// raiseProcess and raiseThread leave priorities as they are. Raising
// them on Linux and macOS needs privileges an application seldom has.
func raiseProcess() {}

func raiseThread() {}
