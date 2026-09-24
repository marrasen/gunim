package text

import (
	"os"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

const cjkCollection = "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc"

func TestACollectionHoldsSeveralFaces(t *testing.T) {
	data, err := os.ReadFile(cjkCollection)
	if err != nil {
		t.Skipf("no %s on this machine", cjkCollection)
	}
	faces, err := ParseCollection(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) < 2 {
		t.Fatalf("%d faces, want the collection's several", len(faces))
	}
	r := faces[0].Shape("日本語", 16)
	for _, g := range r.Glyphs {
		if g.ID == 0 {
			t.Fatal("the collection's first face drew a missing glyph for Japanese")
		}
	}
}

// bare returns a Latin face of its own, with no fallbacks.
func bare(t *testing.T) *Face {
	t.Helper()
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestInstalledFontsFillInWhatAFaceLacks(t *testing.T) {
	start := time.Now()
	if err := LoadSystemFonts(); err != nil {
		t.Skip("no system fonts on this machine")
	}
	t.Logf("system fonts ready in %v", time.Since(start))

	f := bare(t)
	r := f.Shape("a日本語", 16)
	if len(r.Glyphs) != 4 {
		t.Fatalf("%d glyphs, want 4", len(r.Glyphs))
	}
	if r.Glyphs[0].Face != f.id {
		t.Fatal("the Latin letter left the face that has it")
	}
	for _, g := range r.Glyphs[1:] {
		if g.Face == f.id {
			t.Skip("no installed font covers Japanese on this machine")
		}
		if g.ID == 0 {
			t.Fatal("an installed font drew a missing glyph")
		}
	}
}

func TestTurningOffInstalledFontsLeavesMissingGlyphs(t *testing.T) {
	UseSystemFonts(false)
	defer UseSystemFonts(true)
	f := bare(t)
	for _, g := range f.Shape("日", 16).Glyphs {
		if g.Face != f.id || g.ID != 0 {
			t.Fatal("with installed fonts off, Japanese still found a font")
		}
	}
}
