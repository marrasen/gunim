package filemanager

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
	// The copy moves a megabyte each quarter of a second.
	var m speedometer
	start := time.Now()
	var rates []float64
	for _, p := range reports {
		if m.add(start.Add(time.Duration(p.bytes/copyBuffer)*250*time.Millisecond), p.bytes) {
			rates = append(rates, m.rate)
		}
	}
	last := reports[len(reports)-1]
	if last.bytes != 4*copyBuffer || last.bytesTotal != 4*copyBuffer {
		t.Fatalf("the copy reported %d of %d bytes at the end", last.bytes, last.bytesTotal)
	}
	if len(rates) < 2 {
		t.Fatalf("%d samples from %d reports", len(rates), len(reports))
	}
	for _, r := range rates {
		if want := float64(copyBuffer) * 4; r != want {
			t.Fatalf("the samples are %v, want each a megabyte a quarter second, %v", rates, want)
		}
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
