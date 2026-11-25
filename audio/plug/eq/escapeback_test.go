package eq

import (
	"testing"

	"github.com/marrasen/gunim"
)

// The graph says it takes Escape while a band is chosen, for a phone's Back.
func TestAGraphWithABandChosenTakesEscape(t *testing.T) {
	for _, sel := range []bool{false, true} {
		g := &graph{}
		if sel {
			g.sel = 1
		}
		if got := gunim.EscapeTaker(g).TakesEscape(nil); got != sel {
			t.Errorf("with a band chosen %v, the graph takes Escape: %v", sel, got)
		}
	}
}
