package widget

import (
	"image/color"
	"slices"
	"testing"

	"github.com/marrasen/gunim/theme"
)

// bothThemes are the colour tokens whose dark default suits the light theme too.
var bothThemes = map[string]bool{
	// Strong fills with white on them.
	"button.primary": true, "button.primary.hover": true, "button.danger": true, "button.danger.hover": true,
	"button.strong.ink": true, "check.mark": true, "window.close.hot": true,
	// Tints over content, the same on dark and light.
	"grid.mark": true, "table.mark": true,
	// Rings drawn on the desktop around the window, not on the window's background.
	"echo.problem": true, "echo.done": true, "echo.call": true, "echo.wait": true,
}

func TestTheLightThemeGivesEveryColourALightValue(t *testing.T) {
	light := Light()
	var missing []string
	for key, def := range theme.Declared() {
		if _, isColour := def.(color.NRGBA); !isColour || light.Has(key) || bothThemes[key] {
			continue
		}
		missing = append(missing, key)
	}
	slices.Sort(missing)
	for _, key := range missing {
		t.Errorf("Light leaves %q at its dark default; give it a light value, or add it to bothThemes", key)
	}
}
