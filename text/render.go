package text

// Rendering says how a window draws text. Its zero value follows the
// system's settings.
type Rendering struct {
	Smoothing Smoothing
	Hinting   Hinting
	// Gamma is the gamma text is blended for, from 1 to 2.2. Zero
	// follows the system.
	Gamma float32
}

// Smoothing is how glyph edges are antialiased.
type Smoothing uint8

const (
	// SmoothingSystem follows the system's setting.
	SmoothingSystem Smoothing = iota
	// Greyscale antialiases each pixel as a whole.
	Greyscale
	// SubpixelRGB antialiases each third of a pixel on its own, as
	// ClearType does, for a panel whose subpixels run red, green and
	// blue from the left. Text falls back to greyscale where subpixels
	// would show wrong: moving under a scale or rotation, fading in a
	// layer, or over a transparent window.
	SubpixelRGB
	// SubpixelBGR is [SubpixelRGB] for a panel whose subpixels run
	// blue, green and red from the left.
	SubpixelBGR
)

// Subpixel reports whether s antialiases each third of a pixel.
func (s Smoothing) Subpixel() bool { return s == SubpixelRGB || s == SubpixelBGR }

// Hinting is whether glyph edges snap to the pixel grid.
type Hinting uint8

const (
	// HintingSystem follows the system's setting.
	HintingSystem Hinting = iota
	// HintingNone draws glyphs as the font shapes them.
	HintingNone
	// HintingLight snaps the heights of glyphs to whole pixels at small
	// sizes: the baseline, the x-height, the cap height, and the tops
	// and bottoms of strokes. Widths and positions across stay as
	// shaped.
	HintingLight
)

// Or returns r with each setting it leaves to the system taken from sys.
func (r Rendering) Or(sys Rendering) Rendering {
	if r.Smoothing == SmoothingSystem {
		r.Smoothing = sys.Smoothing
	}
	if r.Hinting == HintingSystem {
		r.Hinting = sys.Hinting
	}
	if r.Gamma == 0 {
		r.Gamma = sys.Gamma
	}
	return r
}
