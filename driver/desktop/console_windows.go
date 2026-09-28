//go:build !gunimconsole

package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// A Go program built for Windows is a console program: started from
// Explorer or a shortcut, Windows makes a console window for it, beside
// the program's own windows, and closing that console ends the program.
// A program with windows has no use for one it was given alone, so the
// driver lets it go as the program starts. A console the program shares,
// as when it was started from a terminal, stays, so what it prints
// there, and a program run with its output piped, are as they were.
//
// The build tag gunimconsole keeps the console in every case, for a
// program that wants one. A release build that wants no console at all,
// not even the instant before this lets it go, is linked with
// -ldflags -H=windowsgui.

var (
	consoleKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleProcessList = consoleKernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = consoleKernel32.NewProc("FreeConsole")
)

func init() { freeOwnConsole() }

// freeOwnConsole lets go of the console when this process is the only
// one attached to it.
func freeOwnConsole() {
	if procGetConsoleProcessList.Find() != nil || procFreeConsole.Find() != nil {
		return
	}
	var pids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 {
		// No console at all, or one shared with the shell that started
		// the program.
		return
	}
	_, _, _ = procFreeConsole.Call()
}
