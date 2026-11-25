package pace

import (
	"errors"
	"strconv"
	"testing"
	"testing/synctest"
	"time"
)

// android drops updates as Android does: where the average time between posts, this one included, is under 0.2 s.
type android struct {
	shown   map[string]int
	last    time.Time
	mean    float64
	dropped int
	posts   int
}

func (a *android) post(tag string, v int) error {
	a.posts++
	if !a.last.IsZero() {
		a.mean = weight*a.mean + (1-weight)*max(time.Since(a.last).Seconds(), 0.0005)
	} else {
		a.mean = 1000
	}
	a.last = time.Now()
	if _, ok := a.shown[tag]; ok && a.mean < 0.2 {
		a.dropped++
		return nil
	}
	a.shown[tag] = v
	return nil
}

// A hundred updates a second, for ten seconds, drop none at the system, and the last shows.
func TestManyUpdatesDropNoneAndTheLastShows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &android{shown: map[string]int{}}
		p := Pace[int]{Post: a.post}
		for i := range 1000 {
			if err := p.Update("work", i); err != nil {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		p.Drain(time.Minute)
		if a.dropped > 0 || a.shown["work"] != 999 {
			t.Fatalf("%d of %d dropped, and %d shows", a.dropped, a.posts, a.shown["work"])
		}
		if a.posts < 25 {
			t.Fatalf("only %d posts in ten seconds", a.posts)
		}
	})
}

// Many new messages go at once, and an update after them waits until the system takes it.
func TestNewMessagesGoAtOnceAndAnUpdateAfterThemWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &android{shown: map[string]int{}}
		p := Pace[int]{Post: a.post}
		for i := range 30 {
			_ = p.Update(strconv.Itoa(i), i)
		}
		if len(a.shown) != 30 {
			t.Fatalf("%d of 30 went at once", len(a.shown))
		}
		for i := range 5 {
			_ = p.Update(strconv.Itoa(i), 100+i)
		}
		start := time.Now()
		p.Drain(time.Minute)
		for i := range 5 {
			if a.shown[strconv.Itoa(i)] != 100+i {
				t.Fatalf("message %d shows %d", i, a.shown[strconv.Itoa(i)])
			}
		}
		if a.dropped > 0 || time.Since(start) > 4*time.Second {
			t.Fatalf("%d dropped, in %v", a.dropped, time.Since(start))
		}
	})
}

// After a rest, a burst of updates drops none.
func TestABurstAfterARestDropsNone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &android{shown: map[string]int{}}
		p := Pace[int]{Post: a.post}
		_ = p.Update("a", 0)
		for round := range 5 {
			time.Sleep(time.Hour)
			for i := range 200 {
				_ = p.Update("a", 1000*round+i)
				time.Sleep(time.Millisecond)
			}
			p.Drain(time.Minute)
			if a.dropped > 0 || a.shown["a"] != 1000*round+199 {
				t.Fatalf("%d dropped, and %d shows", a.dropped, a.shown["a"])
			}
		}
	})
}

// One taken away as its update waits does not come back.
func TestOneTakenAwayDoesNotComeBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &android{shown: map[string]int{}}
		p := Pace[int]{Post: a.post}
		for i := range 10 {
			_ = p.Update("a", i)
			_ = p.Update("b", i)
		}
		if err := p.End("a", func() error { delete(a.shown, "a"); return nil }); err != nil {
			t.Fatal(err)
		}
		p.Drain(time.Minute)
		if _, ok := a.shown["a"]; ok || a.shown["b"] != 9 {
			t.Fatalf("%v show", a.shown)
		}
	})
}

// What Post fails with comes back for one that goes at once, and the next goes at once too.
func TestAFailureComesBack(t *testing.T) {
	want := errors.New("off")
	p := Pace[int]{Post: func(string, int) error { return want }}
	for range 2 {
		if err := p.Update("a", 1); !errors.Is(err, want) {
			t.Fatal(err)
		}
	}
}

// Draining ends where more goes on coming.
func TestDrainingEndsWhereMoreGoesOnComing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &android{shown: map[string]int{}}
		p := Pace[int]{Post: a.post}
		stop := make(chan struct{})
		ended := make(chan struct{})
		go func() {
			defer close(ended)
			for {
				select {
				case <-stop:
					return
				case <-time.After(time.Millisecond):
					_ = p.Update("a", 1)
				}
			}
		}()
		time.Sleep(time.Second)
		start := time.Now()
		p.Drain(5 * time.Second)
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("draining took %v", d)
		}
		close(stop)
		<-ended
		p.Drain(time.Minute)
	})
}
