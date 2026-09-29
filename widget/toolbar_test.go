package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestToolbarTakesItsItemsSizeWithPadding(t *testing.T) {
	a, b := newSpot(28, 28), newSpot(28, 28)
	bar := NewToolbar(a, b)
	stage(t, &frame{child: bar, size: geom.Sz(400, 100)})
	at(t, a, geom.Pt(2, 2))
	at(t, b, geom.Pt(30, 2))
}
