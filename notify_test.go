package gunim

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/marrasen/gunim/driver"
)

// phoneDriver is a driver with messages and a network kept in memory, as an application's test builds one.
type phoneDriver struct {
	pumpDriver
	driver.OffscreenNotifier
	driver.OffscreenNetwork
}

// A message takes the place of the one with its tag, and a tap brings the tag back, also one from before the
// application listens.
func TestATapOnAMessageBringsItsTagBack(t *testing.T) {
	d := &phoneDriver{}
	var heard []string
	err := MainWith(context.Background(), d, func(a *App) error {
		for _, n := range []Notification{{Tag: "a", Title: "1"}, {Tag: "b <&\"'>", Title: "2"}, {Tag: "a", Title: "3"}} {
			if err := a.Notify(n); err != nil {
				return err
			}
		}
		if err := d.Tap("b <&\"'>"); err != nil {
			return err
		}
		a.OnNotification(func(tag string) { heard = append(heard, tag) })
		if s := d.Shown(); len(s) != 1 || s[0].Title != "3" {
			t.Errorf("%+v show", s)
		}
		if err := d.Tap("a"); err != nil {
			return err
		}
		if err := a.Notify(Notification{Tag: "c", Ongoing: true}); err != nil {
			return err
		}
		if err := d.Tap("c"); err != nil || len(d.Shown()) != 1 {
			t.Errorf("an ongoing message's tap: %v, and %d show", err, len(d.Shown()))
		}
		return errors.Join(a.EndNotify("c"), a.EndNotify("none"))
	})
	if err != nil || !slices.Equal(heard, []string{"b <&\"'>", "a", "c"}) || len(d.Shown()) != 0 {
		t.Fatalf("%v, heard %q, and %d show", err, heard, len(d.Shown()))
	}
}

func TestAMessageFailsWhereItCannotShow(t *testing.T) {
	d := &phoneDriver{}
	err := MainWith(context.Background(), d, func(a *App) error {
		if err := a.Notify(Notification{Tag: strings.Repeat("x", MaxNotifyTag+1)}); !errors.Is(err, ErrNotifyTag) {
			t.Errorf("a tag too long returned %v", err)
		}
		if err := a.Notify(Notification{Tag: "\xff"}); !errors.Is(err, driver.ErrNotifyText) {
			t.Errorf("a tag that is not UTF-8 returned %v", err)
		}
		if err := a.Notify(Notification{Body: "\xff"}); !errors.Is(err, driver.ErrNotifyText) {
			t.Errorf("a body that is not UTF-8 returned %v", err)
		}
		d.SetNotifyOff(true)
		if err := a.Notify(Notification{Tag: "a"}); !errors.Is(err, ErrNotifyOff) {
			t.Errorf("messages turned off returned %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = MainWith(context.Background(), &pumpDriver{}, func(a *App) error {
		a.OnNotification(func(string) {})
		if err := a.Notify(Notification{}); !errors.Is(err, ErrNoNotifier) {
			t.Errorf("a platform with no messages returned %v", err)
		}
		if err := a.EndNotify(""); !errors.Is(err, ErrNoNotifier) {
			t.Errorf("a platform with no messages returned %v", err)
		}
		if _, err := a.WatchNetwork(func(NetworkState) {}); !errors.Is(err, ErrNoNetworkWatch) {
			t.Errorf("a platform with no news returned %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProgressIsPartsOfAThousand(t *testing.T) {
	for _, c := range []struct {
		p    NotifyProgress
		want int
		ok   bool
	}{
		{NotifyProgress{Done: 0, Total: 10}, 0, true}, {NotifyProgress{Done: 5, Total: 10}, 500, true}, {NotifyProgress{Done: 10, Total: 10}, 1000, true},
		{NotifyProgress{Done: 11, Total: 10}, 1000, true}, {NotifyProgress{Done: -1, Total: 10}, 0, true}, {NotifyProgress{Done: 3, Total: 0}, 0, false},
		{NotifyProgress{Done: 1 << 62, Total: 1 << 62}, 1000, true}, {NotifyProgress{Done: 1 << 61, Total: 1 << 62}, 500, true},
		// Work that isn't done is not shown as done.
		{NotifyProgress{Done: 1<<40 - 1, Total: 1 << 40}, 999, true},
	} {
		if got, ok := c.p.Parts(); got != c.want || ok != c.ok {
			t.Errorf("%+v is %d, %v; want %d, %v", c.p, got, ok, c.want, c.ok)
		}
	}
}

// What was connected may be so no longer after another network, a block, or its end, and not after another price.
func TestTheNetworksChangesAreHeard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var d driver.OffscreenNetwork
		var heard []NetworkState
		stop, err := d.WatchNetwork(func(s NetworkState) { heard = append(heard, s) })
		if err != nil {
			t.Fatal(err)
		}
		wifi := NetworkState{Online: true, Internet: true, Network: 1}
		metered, blocked, mobile := wifi, wifi, NetworkState{Online: true, Metered: true, Network: 2}
		metered.Metered, blocked.Blocked = true, true
		for _, s := range []struct {
			state   NetworkState
			changed bool
		}{{wifi, false}, {metered, false}, {wifi, false}, {blocked, true}, {wifi, true}, {mobile, true}, {NetworkState{}, true}} {
			heard = nil
			d.SetNetwork(s.state)
			d.SetNetwork(s.state)
			synctest.Wait()
			want := s.state
			want.Changed = s.changed
			if !slices.Equal(heard, []NetworkState{want}) {
				t.Errorf("heard %+v, want %+v", heard, want)
			}
			if s.state.Usable() != (s.state != blocked && s.state.Online) {
				t.Errorf("%+v is usable: %v", s.state, s.state.Usable())
			}
		}
		heard = nil
		stop()
		stop()
		d.SetNetwork(wifi)
		synctest.Wait()
		if len(heard) > 0 {
			t.Errorf("heard %+v after stop", heard)
		}
		// One that starts to watch late hears how it is, with no change.
		late, err := d.WatchNetwork(func(s NetworkState) { heard = append(heard, s) })
		if err != nil {
			t.Fatal(err)
		}
		defer late()
		synctest.Wait()
		if !slices.Equal(heard, []NetworkState{wifi}) {
			t.Errorf("heard %+v", heard)
		}
	})
}

// A change that a slow listener misses is in the next it hears.
func TestAChangeASlowListenerMissesIsInTheNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var d driver.OffscreenNetwork
		var heard []NetworkState
		gate := make(chan bool)
		stop, _ := d.WatchNetwork(func(s NetworkState) { <-gate; heard = append(heard, s) })
		defer stop()
		wifi := NetworkState{Online: true, Network: 1}
		d.SetNetwork(wifi)
		synctest.Wait()
		d.SetNetwork(NetworkState{})
		d.SetNetwork(wifi)
		wifi.Metered = true
		d.SetNetwork(wifi)
		gate <- true
		gate <- true
		synctest.Wait()
		if len(heard) != 2 || !heard[1].Changed || !heard[1].Metered {
			t.Fatalf("heard %+v", heard)
		}
	})
}

// News of a run of listening that has stopped, which was on its way, is heard by no one, and starts no change.
func TestNewsOfAListeningThatStoppedIsLeftOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var listens []int64
		n := driver.NetworkNews{Listen: func(run int64, on bool) error {
			if !on {
				run = -run
			}
			listens = append(listens, run)
			return nil
		}}
		var heard []NetworkState
		hear := func(s NetworkState) { heard = append(heard, s) }
		wifi, mobile := NetworkState{Online: true, Network: 1}, NetworkState{Online: true, Network: 2}
		first, err := n.Watch(hear)
		if err != nil {
			t.Fatal(err)
		}
		n.Set(1, wifi, false)
		synctest.Wait()
		first()
		n.Set(1, mobile, true)
		second, err := n.Watch(hear)
		if err != nil {
			t.Fatal(err)
		}
		n.Set(1, mobile, true)
		synctest.Wait()
		n.Set(2, wifi, false)
		synctest.Wait()
		if !slices.Equal(heard, []NetworkState{wifi, wifi}) {
			t.Fatalf("heard %+v", heard)
		}
		third, err := n.Watch(hear)
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		second()
		n.Set(2, mobile, false)
		synctest.Wait()
		third()
		if want := []int64{1, -1, 2, -2}; !slices.Equal(listens, want) {
			t.Fatalf("it listened %v, want %v", listens, want)
		}
		if l := heard[len(heard)-1]; l.Network != 2 || !l.Changed || len(heard) != 4 {
			t.Fatalf("heard %+v", heard)
		}
	})
}

// Where the driver cannot listen, no one watches, and the next to try may.
func TestAWatchFailsWhereTheDriverCannotListen(t *testing.T) {
	want := errors.New("no")
	fail := true
	n := driver.NetworkNews{Listen: func(int64, bool) error {
		if fail {
			return want
		}
		return nil
	}}
	if _, err := n.Watch(func(NetworkState) {}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	fail = false
	stop, err := n.Watch(func(NetworkState) {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
}
