//go:build linux || darwin

package desktop

import (
	"runtime"

	"github.com/marrasen/gunim/text"
)

// systemText returns how text is drawn when the application leaves it
// to the system: greyscale, and hinted except on macOS, which draws
// glyphs as their fonts shape them.
func systemText() (text.Rendering, error) {
	tr := text.Rendering{Smoothing: text.Greyscale, Hinting: text.HintingLight, Gamma: 1.8}
	if runtime.GOOS == "darwin" {
		tr.Hinting = text.HintingNone
	}
	return tr, nil
}
