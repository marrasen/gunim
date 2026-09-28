package gunim

import (
	"fmt"
	"os"
	"testing"
)

// made are the offscreen windows the tests made, and TestMain fails the
// run for any stray they kept that a test did not take.
var made []*Window

func TestMain(m *testing.M) {
	offscreenMade = func(w *Window) { made = append(made, w) }
	code := m.Run()
	for _, w := range made {
		for _, s := range w.Strays() {
			fmt.Fprintln(os.Stderr, "stray:", s)
			code = 1
		}
	}
	os.Exit(code)
}
