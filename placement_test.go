package gunim

import (
	"context"
	"testing"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// optionsDriver is a pumpDriver that keeps the options of the last
// window it opened.
type optionsDriver struct {
	pumpDriver
	got driver.Options
}

func (d *optionsDriver) NewWindow(o driver.Options) (driver.Window, error) {
	d.got = o
	return d.pumpDriver.NewWindow(o)
}

func TestWindowOpensAtItsPlacementAndReportsIt(t *testing.T) {
	d := &optionsDriver{}
	saved := &driver.Placement{Bounds: geom.Rc(40, 50, 800, 600), Maximized: true}
	var got driver.Placement
	var ok bool
	err := runApp(context.Background(), d, func(a *App) error {
		w, err := a.NewWindow(WindowOptions{Title: "placed", Place: saved})
		if err != nil {
			return err
		}
		w.mustOffscreen(t).SetOrigin(geom.Pt(40, 50))
		got, ok = w.Placement()
		w.Close()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.got.Place != saved {
		t.Errorf("the driver was asked to place the window at %v, want %v", d.got.Place, saved)
	}
	if want := geom.Rc(40, 50, 800, 600); !ok || got.Bounds != want || got.Maximized {
		t.Errorf("Placement = %v, %v; want %v, not maximized, true", got, ok, want)
	}
}
