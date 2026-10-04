package vst3

import (
	"runtime"
	"sync"
	"time"
)

// A plugin's controller and editor live on one thread, as they expect:
// the package's own, locked, which also runs what plugins ask to be run
// there.

var uiOnce sync.Once

// uiWork is what the thread is asked to do.
var uiWork = make(chan func(), 64)

// tickers are run on the thread every few milliseconds: a plugin's
// output to its controller, and its editor's timers.
var (
	tickersMu sync.Mutex
	tickers   = map[any]func(){}
)

// onUI runs f on the plugins' thread and waits for it.
func onUI(f func()) {
	uiOnce.Do(func() {
		ready := make(chan struct{})
		go uiLoop(ready)
		<-ready
	})
	done := make(chan struct{})
	uiWork <- func() {
		defer close(done)
		f()
	}
	wake()
	<-done
}

// tick has f run on the plugins' thread every few milliseconds, under
// key, until untick.
func tick(key any, f func()) {
	tickersMu.Lock()
	defer tickersMu.Unlock()
	tickers[key] = f
}

func untick(key any) {
	tickersMu.Lock()
	defer tickersMu.Unlock()
	delete(tickers, key)
}

// tickEvery is how often the thread runs its tickers.
const tickEvery = 16 * time.Millisecond

func runTickers() {
	tickersMu.Lock()
	fs := make([]func(), 0, len(tickers))
	for _, f := range tickers {
		fs = append(fs, f)
	}
	tickersMu.Unlock()
	for _, f := range fs {
		f()
	}
}

func uiLoop(ready chan struct{}) {
	runtime.LockOSThread()
	uiRun(ready)
}
