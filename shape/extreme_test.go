package shape

import (
	"fmt"
	"strings"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/vecpath"
	"github.com/marrasen/gunim/paint"
)

func TestAPathThatReachesPastFloat32IsAnError(t *testing.T) {
	for _, d := range []string{
		// The bounds are finite, but not their width.
		"M-3e38 0L3e38 0L0 10z",
		// Each number is finite, but not the point they add up to.
		"M3e38 0l3e38 0l0 10z",
	} {
		if _, err := NewPath(d); err == nil {
			t.Errorf("%q read, want an error", d)
		}
		if _, err := ParseSVG([]byte(`<svg><path d="` + d + `"/></svg>`)); err == nil {
			t.Errorf("an SVG with %q read, want an error", d)
		}
	}
	if _, err := ParseSVG([]byte(`<svg><g transform="scale(1e30)"><rect width="1e30" height="1"/></g></svg>`)); err == nil {
		t.Error("a rect scaled past float32 read, want an error")
	}
}

func TestAFillOfAPathPastFloat32DrawsWithoutPanicking(t *testing.T) {
	// A path made without NewPath's check, as a fill of points past float32 is drawn.
	subs, err := vecpath.Parse("M-3e38 0L3e38 0L0 10z")
	if err != nil {
		t.Fatal(err)
	}
	lo, hi, _ := vecpath.Bounds(subs)
	p := &Path{subs: subs, lo: lo, hi: hi}
	if m := p.Fill().Coverage(16, 16); len(m) != 16*16 {
		t.Fatalf("%d bytes", len(m))
	}
	p.Stroke(1).Coverage(16, 16)
}

// extremes are numbers at and past the ends of float32, and ones that are not numbers.
var extremes = []string{
	"0", "-0", "1e-45", "-1e-45", "1e-38", "3e38", "-3e38", "3.4028234e38", "-3.4028234e38", "3.5e38", "1e39",
	"-1e39", "1e308", "1e309", "NaN", "nan", "Inf", "-Inf", "+Inf", "infinity", "1e30", "-1e30",
}

// extremeSVGs are SVG files with n in each place a number goes.
func extremeSVGs(n string) []string {
	r := strings.NewReplacer("N", n)
	out := make([]string, 0, 12)
	for _, src := range []string{
		`<svg viewBox="0 0 10 10"><path d="MN 0LN,N L0 Nz" fill="red" stroke="blue"/></svg>`,
		`<svg viewBox="0 0 10 10"><path d="M0 0lN,0lN,NcN,N,N,N,N,Nz" stroke-width="2" stroke="#000"/></svg>`,
		`<svg viewBox="0 0 10 10"><path d="M1 1aN,N,N,0,1,5,5qN,N,2,2tN,N"/></svg>`,
		`<svg viewBox="N,N,N,N"><rect x="N" y="N" width="N" height="N" rx="N"/></svg>`,
		`<svg width="N" height="N"><circle cx="N" cy="N" r="N"/><ellipse rx="N" ry="N"/></svg>`,
		`<svg><line x1="N" y1="0" x2="1" y2="N" stroke="red" stroke-width="N"/></svg>`,
		`<svg><polygon points="0,0 N,0 N,N"/><polyline points="N,N 1,1 2,N" fill="red"/></svg>`,
		`<svg><g transform="scale(N)"><rect width="10" height="10"/></g></svg>`,
		`<svg><g transform="matrix(N,0,0,N,N,N) rotate(N 1 1) skewX(N)"><circle r="5"/></g></svg>`,
		`<svg><g opacity="N" fill-opacity="N"><rect width="5" height="5" stroke="red" stroke-width="N"/></g></svg>`,
		`<svg><defs><linearGradient id="g" x1="N" y1="N" x2="N" y2="N" gradientTransform="scale(N)">` +
			`<stop offset="N"/><stop offset="N%" stop-opacity="N"/></linearGradient></defs>` +
			`<rect width="5" height="5" fill="url(#g)"/></svg>`,
		`<svg><defs><radialGradient id="g" cx="N" cy="N" r="N" gradientUnits="userSpaceOnUse">` +
			`<stop offset="0" stop-color="red"/><stop offset="1"/></radialGradient></defs>` +
			`<circle r="5" fill="url(#g)"/></svg>`,
	} {
		out = append(out, r.Replace(src))
	}
	return out
}

// drawAll reads src and, where it reads, draws each of its parts' masks at a few sizes and paints the figure.
func drawAll(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s: panic: %v", src, r)
		}
	}()
	f, err := ParseSVG([]byte(src))
	if err != nil {
		return
	}
	var p paint.Painter
	p.Reset()
	f.Paint(&p, geom.Rc(0, 0, 64, 64))
	for _, pt := range f.Parts {
		for _, sz := range [][2]int{{1, 1}, {16, 16}, {37, 5}} {
			fill := pt.Path.Fill()
			fill.EvenOdd = pt.EvenOdd
			fill.Coverage(sz[0], sz[1])
			pt.Path.Stroke(pt.Width).Coverage(sz[0], sz[1])
			pt.Path.Stroke(1).Coverage(sz[0], sz[1])
		}
	}
}

func TestAnSVGOfExtremeNumbersNeverPanics(t *testing.T) {
	for _, n := range extremes {
		for _, src := range extremeSVGs(n) {
			drawAll(t, src)
		}
	}
	// Two different numbers side by side, where one alone is fine.
	for _, a := range extremes {
		for _, b := range []string{"3e38", "-3e38", "1e-45"} {
			drawAll(t, fmt.Sprintf(`<svg><path d="M%s %s L%s %s L%s %s z" stroke="red"/></svg>`, a, b, b, a, a, a))
		}
	}
}

func FuzzParseSVG(f *testing.F) {
	for _, n := range extremes {
		for _, src := range extremeSVGs(n) {
			f.Add(src)
		}
	}
	f.Add(art)
	f.Fuzz(drawAll)
}
