package main

import "github.com/marrasen/gunim/text"

// runs holds text shaped already, for nodes that paint every frame.
// It is reached from the UI goroutine alone.
var runs = map[runKey]text.Run{}

type runKey struct {
	s    string
	size float32
	bold bool
}

// shaped is s shaped at size, from the cache.
func shaped(s string, size float32, bold bool) text.Run {
	k := runKey{s, size, bold}
	if r, ok := runs[k]; ok {
		return r
	}
	if len(runs) > 4000 {
		clear(runs)
	}
	r := text.GoSans(bold, false).Shape(s, size)
	runs[k] = r
	return r
}
