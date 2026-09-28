// Package icon holds the Lucide icon set as vector strokes, and draws an icon as a coverage mask a driver tints.
//
// Each icon is a variable named after Lucide's name for it, such as [Funnel] for "funnel", with Lucide's older names
// as aliases, such as [Filter]. A program links only the icons it uses. Package byname looks icons up by name.
//
// An icon draws the way Lucide draws it: strokes 2 units wide on a 24 by 24 grid, with round caps and joins, scaled
// to the size it is drawn at.
package icon

//go:generate go run ../tools/genicons -src $LUCIDE_REACT

import (
	"sync"
)

// Icon is a vector icon: SVG path data on a 24 by 24 grid, drawn as strokes with round caps and joins. It must not
// change once drawn.
type Icon struct {
	// Name is the icon's name in Lucide, such as "circle-alert".
	Name string
	// Path is the SVG path data drawn as strokes.
	Path string
	// Fill is SVG path data filled as well as stroked, or empty.
	Fill string

	// parsed holds the outline, parsed on first use and freed with the icon.
	parsed     *outline
	parsedOnce sync.Once
}

// Check reports the first error in the icon's path data.
func (ic *Icon) Check() error {
	if _, err := parse(ic.Path); err != nil {
		return err
	}
	_, err := parse(ic.Fill)
	return err
}

// outline returns the icon's parsed outline. A path with an error draws as far as it parsed.
func (ic *Icon) outline() *outline {
	ic.parsedOnce.Do(func() {
		o := &outline{}
		o.strokes, _ = parse(ic.Path)
		o.fills, _ = parse(ic.Fill)
		ic.parsed = o
	})
	return ic.parsed
}

// Stroke is an icon as a [paint.Shape]: stroked Width units wide on its 24-unit grid, and drawn on as far as
// Progress, from 0 for nothing to 1 for all of it. The zero Stroke draws nothing; see [Icon.Stroke].
type Stroke struct {
	Icon     *Icon
	Width    float32
	Progress float32
}

// Stroke returns the icon stroked 2 units wide, as Lucide draws it, and drawn in full.
func (ic *Icon) Stroke() Stroke { return Stroke{Icon: ic, Width: 2, Progress: 1} }

// Settled implements [paint.Shape]: a stroke drawn in full stays as it is. A width that is not a number is redrawn
// every frame, as it never equals the last one's.
func (s Stroke) Settled() bool { return s.Progress >= 1 && s.Width == s.Width }

// Coverage implements [paint.Shape]. It fits the 24-unit grid into w by h pixels, centred.
func (s Stroke) Coverage(w, h int) []byte {
	if w <= 0 || h <= 0 {
		return nil
	}
	if s.Icon == nil || s.Width <= 0 || s.Progress <= 0 {
		return make([]byte, w*h)
	}
	return rasterize(s.Icon.outline(), w, h, s.Width, min(s.Progress, 1))
}
