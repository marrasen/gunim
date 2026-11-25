package widget

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/theme"
)

// colourNear reports whether a and b differ by no more than by in each channel.
func colourNear(a, b color.Color, by int) bool {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	d := func(x, y uint32) int { return max(int(x>>8)-int(y>>8), int(y>>8)-int(x>>8)) }
	return d(r1, r2) <= by && d(g1, g2) <= by && d(b1, b2) <= by
}

// phoneShot saves img as name in the folder GUNIM_SHOTS names, where it names one.
func phoneShot(t *testing.T, name string, img image.Image) {
	if dir := os.Getenv("GUNIM_SHOTS"); dir != "" {
		gunimtest.WritePNG(t, filepath.Join(dir, name), img)
	}
}

// The real renderer draws the bar's ground and its pill, the disc of a refresh under way, and a sheet's panel over
// the shaded page, each in its own colour.
func TestThePhoneWidgetsDrawInTheirColours(t *testing.T) {
	th := Dark().With(Touch()...)
	w := gunimtest.NewWith(t, gunim.WindowOptions{Size: geom.Sz(390, 800), Root: NewSurface(), Theme: th})
	rows := make([]gunim.Node, 0, 30)
	for i := range 30 {
		rows = append(rows, NewLabel("Message "+string(rune('A'+i%26))))
	}
	pull := NewPullRefresh(NewScroll(NewPad(Column(rows...))))
	places := []BarItem{
		{Label: "Inbox", Icon: icon.Inbox, Count: 3},
		{Label: "Notes", Icon: icon.NotebookPen},
		{Label: "You", Icon: icon.User, Dot: true},
	}
	b := NewBottomBar(places, pull, NewLabel("Notes"), NewLabel("You"))
	g := &grab{child: b}
	gunim.RegisterView(w, "phone", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "phone", "phone", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.TimeByFrames(true)
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	pull.SetRefreshing(true, g.ui)
	run(60)
	img := gunimtest.Pixels(t, w)
	phoneShot(t, "bar.png", img)
	live := theme.NewLive(th)
	strip, _ := g.ui.Bounds(b.strip)
	ground := img.At(int(strip.Max.X)-4, int(strip.Max.Y)-3)
	if !colourNear(ground, MenubarFill.Get(live), 6) {
		t.Fatalf("the bar's ground is %v, want %v", ground, MenubarFill.Get(live))
	}
	pill := pillRect(b.strip.slot(0, b.strip.th), b.strip.th).Add(strip.Min)
	inPill := img.At(int(pill.Min.X)+4, int(pill.Center().Y))
	if colourNear(inPill, ground, 2) {
		t.Fatalf("the pill is %v, the colour of the ground", inPill)
	}
	disc := img.At(195, int(PullRest.Get(live))-3)
	if !colourNear(disc, MenuFill.Get(live), 8) {
		t.Fatalf("the disc's edge is %v, want %v", disc, MenuFill.Get(live))
	}

	s := NewBottomSheet("Share a note", NewPad(Column(NewLabel("To"), NewLabel("Friends"))))
	g.ui.Insert(g.ui.Root(), s)
	run(90)
	img = gunimtest.Pixels(t, w)
	phoneShot(t, "sheet.png", img)
	panel := img.At(int(s.panel.Center().X), int(s.panel.Max.Y)-20)
	if !colourNear(panel, DialogFill.Get(live), 6) {
		t.Fatalf("the sheet's panel is %v, want %v", panel, DialogFill.Get(live))
	}
	shaded := img.At(10, 10)
	if colourNear(shaded, Background.Get(live), 2) {
		t.Fatalf("the page over the sheet is %v, unshaded", shaded)
	}
}
