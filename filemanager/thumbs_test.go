package filemanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
)

func TestThumbnailsComeForTheTilesInViewAndStayCached(t *testing.T) {
	pics := []string{"a.png", "b.jpg", "c.gif", "d.bmp"}
	h := newPicHarness(t, pics, "notes.txt", "folder/")
	h.do(Command{Name: CmdViewIcons})
	have := func() bool {
		for _, n := range pics {
			if h.icons().thumbs[n].img == nil {
				return false
			}
		}
		return true
	}
	h.until("each picture's thumbnail arrives", have)
	if _, ok := h.icons().thumbs["notes.txt"]; ok {
		t.Fatal("a text file was sent a thumbnail")
	}
	if h.a.thumbs.made != len(pics) {
		t.Fatalf("the workers made %d thumbnails for %d pictures", h.a.thumbs.made, len(pics))
	}
	w, hh := h.icons().thumbs["a.png"].img.Size()
	if max(w, hh) > thumbBucket(int(h.icons().grid.Size.W)) || w != 3*hh/2 {
		t.Fatalf("a 60 by 40 picture has a %d by %d thumbnail", w, hh)
	}
	// A page opened again is served from the cache.
	h.do(Navigate{Path: filepath.Join(h.dir, "folder")})
	h.until("the other folder shows", func() bool { return len(h.shown()) == 0 && h.b.listing.path != h.dir })
	h.do(Command{Name: CmdBack})
	h.until("the thumbnails are back", func() bool { return len(h.shown()) == 6 && have() })
	if h.a.thumbs.made != len(pics) {
		t.Fatalf("coming back made %d thumbnails again", h.a.thumbs.made-len(pics))
	}
}

func TestADecodeErrorReachesTheTile(t *testing.T) {
	h := newPicHarness(t, []string{"good.png"}, "broken.png")
	tileOf := h.recordTiles()
	h.do(Command{Name: CmdViewIcons})
	h.until("the broken picture says why", func() bool { return h.icons().thumbs["broken.png"].err != "" })
	msg := h.icons().thumbs["broken.png"].err
	if !strings.Contains(msg, "broken.png") {
		t.Fatalf("the error %q does not name the file", msg)
	}
	h.frames(2)
	tile := tileOf("broken.png")
	if tile == nil || !tile.mark.on || tile.tip.Text != msg {
		t.Fatalf("the tile does not show the error: %+v", tile)
	}
}

func TestAPictureThatCannotBeOpenedSaysSo(t *testing.T) {
	dir := t.TempDir()
	_, _, err := makeThumb(filepath.Join(dir, "gone.png"), 128, false)
	if err == nil || !os.IsNotExist(unwrapAll(err)) {
		t.Fatalf("a missing picture gave %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cut.png"), []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := makeThumb(filepath.Join(dir, "cut.png"), 128, false); err == nil {
		t.Fatal("a picture cut short made a thumbnail")
	}
}

func unwrapAll(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
}

func TestThumbnailSizesCoverTheTile(t *testing.T) {
	for px, want := range map[int]int{50: 96, 96: 96, 97: 128, 200: 256, 5000: 512} {
		if got := thumbBucket(px); got != want {
			t.Errorf("thumbBucket(%d) = %d, want %d", px, got, want)
		}
	}
}

// recordTiles keeps each tile the icon view builds from now on, and returns a lookup by name.
func (h *harness) recordTiles() func(name string) *iconTile {
	iv := h.icons()
	var built []*iconTile
	iv.grid.Tile = func(i int) gunim.Node {
		t := newIconTile(iv, i)
		built = append(built, t)
		return t
	}
	return func(name string) *iconTile {
		for _, t := range built {
			if t.name == name {
				return t
			}
		}
		return nil
	}
}

func TestTheWireCarriesTheIconViewAndTheViewer(t *testing.T) {
	err := gunim.CheckWire(
		ViewMode{Path: "/a", Icons: true, Tile: 128},
		NeedThumbs{Gen: 2, Size: 128, Rows: []int{1, 2}},
		Thumb{Dir: "/a", Name: "b.png", Size: 128, Err: "no"},
		TileSized{Size: 140},
		OpenViewer{Gen: 2, Row: 1},
		ViewerStep{Dir: -1},
		ViewerWants{Seq: 1, W: 800, H: 600},
		ViewerClosed{Seq: 1},
		Viewing{Seq: 1, Travel: 1, Path: "/a/b.png", Name: "b.png", Index: 2, Count: 3, W: 60, H: 40, Loading: true},
	)
	if err != nil {
		t.Fatal(err)
	}
}
