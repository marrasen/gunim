package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
)

// dialogMasks mounts a dialog set up by set, lets it settle, and returns it and the masks it drew.
func dialogMasks(t *testing.T, set func(*Dialog)) (*Dialog, []*paint.MaskOp) {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	var d *Dialog
	gunim.RegisterView(w, "confirm", func(title string) *Dialog {
		d = NewDialog(title)
		set(d)
		return d
	}, nil)
	if err := w.Client().Mount(gunim.Root, "confirm", "confirm", "Delete the folder?"); err != nil {
		t.Fatal(err)
	}
	for range 120 {
		w.Frame(time.Second / 60)
	}
	return d, maskOps(w.Offscreen())
}

func TestADialogShowsItsIconBeforeItsTitle(t *testing.T) {
	d, ms := dialogMasks(t, func(d *Dialog) { d.Icon = icon.FolderInput })
	if len(ms) != 1 || strokeOf(t, ms[0]).Icon != icon.FolderInput || ms[0].Color != Ink.Default() {
		t.Fatalf("the dialog drew %d icons, want its icon in the ink", len(ms))
	}
	panel := d.panel(geom.Sz(800, 600), gunim.Frame{})
	pad, bar := DialogPadding.Default(), TitleBarCompactHeight.Default()
	at := ms[0].Transform.Apply(ms[0].Rect.Min)
	end := ms[0].Transform.Apply(ms[0].Rect.Max)
	if at.Y < panel.Min.Y || end.Y > panel.Min.Y+bar {
		t.Fatalf("the icon runs from %v to %v, want in the title bar, %v to %v down", at.Y, end.Y, panel.Min.Y, panel.Min.Y+bar)
	}
	// The icon and the title are centred together, the title after the icon.
	title := d.title(nil, panel.Size().W)
	want := panel.Min.X + (panel.Size().W-IconSize.Default()-IconGap.Default()-title.Size.W)/2
	if dx := at.X - want; dx < -1 || dx > 1 {
		t.Fatalf("the icon is at %v, want at %v, with the title after it centred in the bar", at.X, want)
	}
	if w := title.Size.W; w > panel.Size().W-2*pad-IconSize.Default()-IconGap.Default() {
		t.Fatalf("the title is %v wide, past the room the icon leaves", w)
	}
	if _, ms := dialogMasks(t, func(*Dialog) {}); len(ms) != 0 {
		t.Fatalf("a dialog with no icon drew %d", len(ms))
	}
}

func TestADangerDialogShowsAWarning(t *testing.T) {
	_, ms := dialogMasks(t, func(d *Dialog) { d.Danger = true })
	if len(ms) != 1 || strokeOf(t, ms[0]).Icon != icon.TriangleAlert || ms[0].Color != DialogDangerInk.Default() {
		t.Fatalf("the danger dialog drew %d icons, want a warning in the danger colour", len(ms))
	}
	_, ms = dialogMasks(t, func(d *Dialog) { d.Danger, d.Icon = true, icon.Trash2 })
	if len(ms) != 1 || strokeOf(t, ms[0]).Icon != icon.Trash2 || ms[0].Color != DialogDangerInk.Default() {
		t.Fatalf("the danger dialog with an icon drew %d, want its icon in the danger colour", len(ms))
	}
}
