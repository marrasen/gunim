package gunim

import (
	"context"
	"testing"
)

// stayDriver is a pumpDriver that hears StayOpen.
type stayDriver struct {
	pumpDriver
	stay []bool
}

func (d *stayDriver) StayOpen(on bool) { d.stay = append(d.stay, on) }

// StayOpen reaches a driver that can stay open, and is quietly nothing
// for one that cannot.
func TestStayOpenReachesTheDriver(t *testing.T) {
	d := &stayDriver{}
	if err := runApp(context.Background(), d, func(a *App) error {
		a.StayOpen(true)
		a.StayOpen(false)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(d.stay) != 2 || !d.stay[0] || d.stay[1] {
		t.Fatalf("the driver heard %v", d.stay)
	}
	if err := runApp(context.Background(), &pumpDriver{}, func(a *App) error {
		a.StayOpen(true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
