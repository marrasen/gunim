//go:build linux && (arm64 || amd64)

package kms

import (
	"context"
	"errors"
	"slices"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// idleDriver is a driver whose render thread has ended and that holds
// nothing, for what Run and Close do around it.
func idleDriver(t *testing.T) *Driver {
	t.Helper()
	d := &Driver{quit: make(chan struct{}), done: make(chan struct{}), ended: make(chan struct{}),
		wake: -1, stop: -1, stats: newTimeline(nil)}
	close(d.done)
	return d
}

// SIGHUP, as an SSH session dropping sends, ends Run, which gives the
// screen back, where it would otherwise end the process with the VT left
// in graphics mode. SIGINT and SIGTERM go the same way.
func TestASignalEndsRunAndGivesTheScreenBack(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			d := idleDriver(t)
			d.catchSignals()
			ran := make(chan error, 1)
			go func() { ran <- d.Run(context.Background(), func() {}) }()
			if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-ran:
				if err != nil {
					t.Fatalf("Run returned %v, want nil for a signal", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("Run did not end on %v", sig)
			}
			select {
			case <-d.Closed():
			default:
				t.Fatal("Run ended with the screen not given back")
			}
		})
	}
}

// Close gives the screen back once, however often it is called, before or
// after Run, and returns the error the render thread ended on.
func TestCloseIsSafeTwiceAndAfterRun(t *testing.T) {
	d := idleDriver(t)
	d.err = errors.New("the GPU lost its context")
	if err := d.Close(); !errors.Is(err, d.err) {
		t.Fatalf("Close returned %v", err)
	}
	if err := d.Close(); !errors.Is(err, d.err) {
		t.Fatalf("a second Close returned %v", err)
	}
	if err := d.Run(context.Background(), func() {}); !errors.Is(err, d.err) {
		t.Fatalf("Run after Close returned %v", err)
	}
	<-d.Closed()
}

// A frame handed over a refresh or more after the flip before is late: its
// missed vblanks count apart, and its time enters All, where Frame, for
// frames handed over at once, leaves it out.
func TestLateFramesAreCountedApart(t *testing.T) {
	tl := newTimeline(nil)
	t0 := time.Now()
	ms := func(n float64) time.Time { return t0.Add(time.Duration(n * float64(time.Millisecond))) }
	tl.shown(2, ms(0), ms(1), ms(3), ms(16.7), time.Time{}, true, 0)
	// Laid out in 25 ms: handed over after the next vblank, and shown a
	// vblank late.
	tl.shown(4, ms(16.7), ms(41.7), ms(43), ms(50), time.Time{}, false, 1)
	s := tl.stats()
	if s.Missed != 0 || s.Late != 1 || s.LateMissed != 1 {
		t.Fatalf("missed %d, late %d with %d missed", s.Missed, s.Late, s.LateMissed)
	}
	if s.Frame.N != 1 || s.All.N != 2 || s.All.Max < 26*time.Millisecond {
		t.Fatalf("frame %+v, all %+v", s.Frame, s.All)
	}
}

// After SYN_DROPPED the kernel sends no ABS_MT_SLOT again, so the decoder
// reads the slot the events go to now: a move for finger 1 after the drop
// moves finger 1, not finger 0.
func TestTheSlotIsReadAgainAfterADrop(t *testing.T) {
	var s stream
	s.down(0, 1, 10, 10)
	s.down(1, 2, 20, 20)
	s.abs(absMTSlot, 0)
	s.syn(synReport)
	s.syn(synDropped)
	s.abs(absMTSlot, 1) // lost with the rest
	s.syn(synReport)
	s.abs(absMTPositionX, 500)
	s.syn(synReport)
	m := newMTDecoder(2)
	m.resync = func(code uint16) []int32 {
		switch code {
		case absMTSlot:
			return []int32{1}
		case absMTTrackingID:
			return []int32{1, 2}
		case absMTPositionX:
			return []int32{10, 20}
		default:
			return []int32{10, 20}
		}
	}
	got := decode(m, s.evs)
	last := got[len(got)-1]
	if len(last) != 1 || last[0] != (contact{slot: 1, id: 2, x: 500, y: 20, phase: phaseMove}) {
		t.Fatalf("after the drop: %v, want finger 1 at 500", last)
	}
}

func TestLiftAllLiftsEveryFingerDown(t *testing.T) {
	var s stream
	s.down(0, 1, 10, 10)
	s.down(3, 4, 30, 30)
	s.syn(synReport)
	m := newMTDecoder(5)
	decode(m, s.evs)
	got := m.liftAll()
	want := []contact{{slot: 0, id: 1, x: 10, y: 10, phase: phaseUp}, {slot: 3, id: 4, x: 30, y: 30, phase: phaseUp}}
	if !slices.Equal(got, want) {
		t.Fatalf("lifted %v, want %v", got, want)
	}
	if again := m.liftAll(); again != nil {
		t.Fatalf("lifted %v again", again)
	}
}

// A stamp from before the clock was set lies decades ahead; it, and every
// stamp from a device left on the wall clock, is taken as now.
func TestEventTimesFromAnotherClockAreTakenAsNow(t *testing.T) {
	now := time.Now()
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		t.Fatal(err)
	}
	mono := time.Duration(ts.Nano()) - 5*time.Millisecond
	fresh := event{sec: int64(mono / time.Second), usec: int64(mono % time.Second / time.Microsecond)}
	if got := eventTime(fresh, false, now); got.Equal(now) || now.Sub(got) > time.Second {
		t.Fatalf("a fresh monotonic stamp came out %v before now", now.Sub(got))
	}
	wall := event{sec: now.Unix(), usec: int64(now.Nanosecond() / 1000)}
	if got := eventTime(wall, false, now); !got.Equal(now) {
		t.Fatalf("a wall-clock stamp came out %v from now", got.Sub(now))
	}
	if got := eventTime(fresh, true, now); !got.Equal(now) {
		t.Fatal("a device on the wall clock was not stamped as read")
	}
}

// A touch screen that goes away mid-touch lifts its fingers, so no window
// keeps a press that never ends.
func TestATouchScreenGoneLiftsItsFingers(t *testing.T) {
	if liveEventSize != 24 {
		t.Skip("the events are written as 64-bit Linux's")
	}
	d := testDriver(0, 1)
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	d.stop = stop
	w, _ := d.NewWindow(driver.Options{})
	win := w.(*Window)
	defer func() { _ = win.Close() }()
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(p[0]) }()
	var s stream
	s.down(0, 1, 100, 100)
	s.syn(synReport)
	buf := make([]byte, 24*len(s.evs))
	for i, e := range s.evs {
		encodeEvent(buf[24*i:], e)
	}
	if _, err := unix.Write(p[1], buf); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(p[1]); err != nil {
		t.Fatal(err)
	}
	dev := &inputDev{fd: p[0], path: "a pipe", slots: 2, x: absInfo{maximum: 719}, y: absInfo{maximum: 1279}}
	done := make(chan struct{})
	go func() {
		d.readTouch(dev)
		close(done)
	}()
	var got []string
	for len(got) < 4 {
		switch nextInput(t, win).(type) {
		case input.PointerMove:
			got = append(got, "move")
		case input.PointerDown:
			got = append(got, "down")
		case input.PointerUp:
			got = append(got, "up")
		case input.PointerLeave:
			got = append(got, "leave")
		}
	}
	if want := []string{"move", "down", "up", "leave"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the reader did not end")
	}
}

// The GPU's attempt failing leaves the CPU to draw, in the same process;
// both failing is an error that holds both reasons; software asks for the
// CPU at once.
func TestTheCPUDrawsWhereTheGPUFails(t *testing.T) {
	gpuErr, cpuErr := errors.New("no V3D"), errors.New("no llvmpipe")
	for _, c := range []struct {
		name     string
		software bool
		fail     map[bool]error
		tries    []bool
		onCPU    bool
		err      []error
	}{
		{"the GPU draws", false, nil, []bool{false}, false, nil},
		{"the GPU fails", false, map[bool]error{false: gpuErr}, []bool{false, true}, true, nil},
		{"both fail", false, map[bool]error{false: gpuErr, true: cpuErr}, []bool{false, true}, true, []error{gpuErr, cpuErr}},
		{"asked for the CPU", true, nil, []bool{true}, true, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var tries []bool
			onCPU, err := startDrawing(c.software, func(software bool) error {
				tries = append(tries, software)
				return c.fail[software]
			})
			if !slices.Equal(tries, c.tries) || onCPU != c.onCPU {
				t.Fatalf("tried %v, on the CPU %v", tries, onCPU)
			}
			for _, want := range c.err {
				if !errors.Is(err, want) {
					t.Fatalf("error %v, want it to hold %v", err, want)
				}
			}
			if c.err == nil && err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Collecting a frame allocates nothing once the slices have grown.
func TestCollectingAFrameAllocatesNothing(t *testing.T) {
	d := testDriver(90, 1.25)
	w, _ := d.NewWindow(driver.Options{})
	win := w.(*Window)
	defer func() { _ = win.Close() }()
	r := &renderThread{d: d}
	step := func() {
		d.mu.Lock()
		win.next, win.hasNext = frame{}, true
		d.mu.Unlock()
		r.collect()
		if len(r.jobs) != 1 || len(r.stack) != 1 {
			t.Fatalf("collected %d jobs over %d layers", len(r.jobs), len(r.stack))
		}
	}
	step()
	if n := testing.AllocsPerRun(100, step); n != 0 {
		t.Fatalf("%v allocations a frame", n)
	}
}

// The pacer starts by telling windows of a flip at once, shortens its lead
// a step at a time while frames make their vblank, and on a miss lengthens
// it at once and holds it. Frames drawn after a rest teach it nothing.
func TestThePacerLearnsHowLateAFrameCanStart(t *testing.T) {
	const period = 16667 * time.Microsecond
	var p pacer
	if got := p.now(0, period); got != period {
		t.Fatalf("starts with a lead of %v, want a whole refresh", got)
	}
	for range paceEvery {
		p.flipped(true, false, 0, period)
	}
	if got := p.now(0, period); got != period-paceStep {
		t.Fatalf("after %d frames made: %v, want a step shorter", paceEvery, got)
	}
	for range 100 * paceEvery {
		p.flipped(true, true, 0, period)
	}
	if got := p.now(0, period); got != paceMinLead {
		t.Fatalf("after many frames made: %v, want the floor, %v", got, paceMinLead)
	}
	p.flipped(false, true, 3, period)
	if got := p.now(0, period); got != paceMinLead {
		t.Fatalf("a frame after a rest moved the lead to %v", got)
	}
	p.flipped(true, true, 1, period)
	if got := p.now(0, period); got != paceMinLead+paceBack {
		t.Fatalf("after a miss: %v, want %v", got, paceMinLead+paceBack)
	}
	for range paceHold {
		p.flipped(true, true, 0, period)
	}
	if got := p.now(0, period); got != paceMinLead+paceBack {
		t.Fatalf("shortened to %v while holding after a miss", got)
	}
	if got := p.now(5*time.Millisecond, period); got != 5*time.Millisecond {
		t.Fatalf("a fixed lead came out %v", got)
	}
}
