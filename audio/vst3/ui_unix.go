//go:build !windows

package vst3

import "time"

// uiRun runs the plugins' thread: the work it is given, and its tickers.
func uiRun(ready chan struct{}) {
	t := time.NewTicker(tickEvery)
	close(ready)
	for {
		select {
		case f := <-uiWork:
			f()
		case <-t.C:
			runTickers()
		}
	}
}

// wake tells the thread work waits; it waits on the work itself.
func wake() {}
