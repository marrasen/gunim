//go:build linux

package desktop

import (
	"testing"

	"github.com/marrasen/gunim/input"
)

// Letters, digits and F-keys have the keysyms X gives them.
func TestHotKeysHaveTheirKeysyms(t *testing.T) {
	for _, c := range []struct {
		k   input.Key
		sym uint32
	}{{input.KeyK, 'k'}, {input.Key5, '5'}, {input.KeyF12, 0xffc9}, {input.KeyEnter, 0xff0d}} {
		if sym, ok := keysym(c.k); !ok || sym != c.sym {
			t.Errorf("key %v is %#x, want %#x", c.k, sym, c.sym)
		}
	}
	if _, ok := keysym(input.KeyLeftShift); ok {
		t.Error("a modifier alone was taken as a hot key")
	}
}
