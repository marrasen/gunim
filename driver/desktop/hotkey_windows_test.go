//go:build windows

package desktop

import (
	"testing"

	"github.com/marrasen/gunim/input"
)

// Letters, digits and F-keys have the virtual keys Windows gives them.
func TestHotKeysHaveTheirVirtualKeys(t *testing.T) {
	for _, c := range []struct {
		k  input.Key
		vk uint32
	}{{input.KeyK, 'K'}, {input.Key5, '5'}, {input.KeyF12, 0x7b}, {input.KeySpace, 0x20}} {
		if vk, ok := virtualKey(c.k); !ok || vk != c.vk {
			t.Errorf("key %v is %#x, want %#x", c.k, vk, c.vk)
		}
	}
	if _, ok := virtualKey(input.KeyLeftShift); ok {
		t.Error("a modifier alone was taken as a hot key")
	}
}
