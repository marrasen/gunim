package main

import (
	"image/color"
	"path/filepath"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// background returns the window's background colour now.
func (h *harness) background() color.NRGBA {
	h.t.Helper()
	type ask struct{}
	var c color.NRGBA
	gunim.RegisterPatch(h.w, "browser", func(_ *browser, _ ask, u *gunim.UI) { c = widget.Background.Get(u.Theme()) })
	if err := h.w.Client().Patch(string(browserID), ask{}); err != nil {
		h.t.Fatal(err)
	}
	h.w.Frame(0)
	return c
}

func TestTheThemeGlidesToLightAndIsSaved(t *testing.T) {
	h := newHarness(t, "a.txt")
	dark := h.background()
	light := widget.Background.Get(theme.NewLive(lightTheme()))
	h.do(Command{Name: CmdThemeLight})
	h.frames(8)
	mid := h.background()
	if mid == dark || mid == light {
		t.Fatalf("eight frames in, the background is %v, want it between %v and %v", mid, dark, light)
	}
	h.frames(120)
	if got := h.background(); got != light {
		t.Fatalf("two seconds in, the background is %v, want %v", got, light)
	}
	p, err := loadPrefs(filepath.Join(h.root, "prefs.json"))
	if err != nil || !p.Light {
		t.Fatalf("the saved settings say light %v, or %v", p.Light, err)
	}
	for m, cmds := range h.b.title.cmds {
		for i, c := range cmds {
			if on := h.b.title.bar.Menus[m].Checked[i]; (c == CmdThemeLight || c == CmdThemeDark) && on != (c == CmdThemeLight) {
				t.Fatalf("the menu item for %s is ticked %v", c, on)
			}
		}
	}
}
