package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim/widget"
)

func TestTheSpeedComesFromACopysProgress(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "big.bin")
	if err := os.WriteFile(src, make([]byte, 4*copyBuffer), 0o644); err != nil {
		t.Fatal(err)
	}
	var reports []progress
	e := env{report: func(p progress) { reports = append(reports, p) }, reportEvery: time.Nanosecond}
	if _, err := runJob(context.Background(), job{kind: OpCopy, srcs: []string{src}, dest: root}, e); err != nil {
		t.Fatal(err)
	}
	// Each report a quarter of a second after the one before.
	var m speedometer
	start := time.Now()
	var rates []float64
	for i, p := range reports {
		if m.add(start.Add(time.Duration(i)*250*time.Millisecond), p.bytes) {
			rates = append(rates, m.rate)
		}
	}
	last := reports[len(reports)-1]
	if last.bytes != 4*copyBuffer || last.bytesTotal != 4*copyBuffer {
		t.Fatalf("the copy reported %d of %d bytes at the end", last.bytes, last.bytesTotal)
	}
	var peak float64
	for _, r := range rates {
		peak = max(peak, r)
	}
	if want := float64(copyBuffer) * 4; len(rates) < 3 || peak != want {
		t.Fatalf("the samples are %v, want a megabyte a quarter second, %v, at their peak", rates, want)
	}
	if left := m.left(copyBuffer, 4*copyBuffer); left <= 0 {
		t.Fatalf("with three quarters to go the time left is %v", left)
	}
}

func TestACopyDrawsItsSpeedAndCheersAsItEnds(t *testing.T) {
	h := newHarness(t, "sub/")
	if err := os.WriteFile(filepath.Join(h.dir, "big.bin"), make([]byte, 3<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	h.a.ops.limit = 3 << 20
	h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "big.bin")}, dest: filepath.Join(h.dir, "sub")},
		"Copying big.bin")
	h.a.ops.limit = 0
	row := func() *opRow {
		r, _ := widget.RowOf[*opRow](h.b.ops.list, widget.Key(strconv.Itoa(h.a.ops.next)))
		return r
	}
	h.until("the graph has speeds", func() bool { return row() != nil && row().samples >= 2 })
	h.until("the copy ends with a cheer", func() bool { return row() != nil && row().badge.cheering })
	if row().detail.Text != "Done" || row().title.Text != "Copied 1 item to sub" {
		t.Fatalf("the finished row says %q and %q", row().title.Text, row().detail.Text)
	}
	h.until("the row leaves the panel", func() bool { return row() == nil })
}
