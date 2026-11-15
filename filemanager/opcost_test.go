package filemanager

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marrasen/gunim"
)

// acrossMove moves srcs into dest as across volumes, and returns every
// report of how far it got.
func acrossMove(t *testing.T, root string, ans answer, srcs []string, dest string) (record, []progress) {
	t.Helper()
	var reports []progress
	e := testEnv(root, ans, nil)
	e.across, e.reportEvery = true, time.Nanosecond
	e.report = func(p progress) { reports = append(reports, p) }
	rec, err := runJob(context.Background(), job{kind: OpMove, srcs: srcs, dest: dest}, e)
	if err != nil {
		t.Fatal(err)
	}
	return rec, reports
}

// steady checks that reports never count more done than there is, and
// that once the counting is over the totals stand, the progress never
// falls back, and it ends with all of it done.
func steady(t *testing.T, reports []progress) {
	t.Helper()
	var counted, totals *progress
	prev := float32(0)
	for i, p := range reports {
		if p.counting {
			counted = &reports[i]
			continue
		}
		if counted == nil {
			continue
		}
		if totals == nil {
			totals = &reports[i]
		}
		if p.itemsTotal != totals.itemsTotal || p.bytesTotal != totals.bytesTotal || p.removeTotal != totals.removeTotal {
			t.Fatalf("report %d: the totals went from %+v to %+v", i, *totals, p)
		}
		if p.items > p.itemsTotal || p.bytes > p.bytesTotal || p.removed > p.removeTotal {
			t.Fatalf("report %d counts more done than there is: %+v", i, p)
		}
		// As the costs weigh it before any is learnt, not held where it
		// was as a card holds it.
		steps, total := p.steps()
		done := float32(new(costs).share(steps, total, p.bytes, p.bytesTotal))
		if done < prev {
			t.Fatalf("report %d: the progress fell from %v to %v", i, prev, done)
		}
		prev = done
	}
	if counted == nil {
		t.Fatal("the move never said it was counting")
	}
	last := reports[len(reports)-1]
	if last.items != last.itemsTotal || last.bytes != last.bytesTotal || last.removed != last.removeTotal || prev != 1 {
		t.Fatalf("the move ended at %+v, %v of the way", last, prev)
	}
}

// A move across volumes counts all it has to move before it copies the
// first item, so the items done never pass the total and the progress
// never falls back as the next folder starts.
func TestAMoveAcrossVolumesCountsAllOfItFirst(t *testing.T) {
	root := t.TempDir()
	srcs := make([]string, 0, 3)
	for d := range 3 {
		dir := filepath.Join(root, "src", fmt.Sprintf("d%d", d))
		for i := range 20 {
			tree(t, root, fmt.Sprintf("src/d%d/%05d.jpg", d, i))
		}
		srcs = append(srcs, dir)
	}
	tree(t, root, "dst/")
	rec, reports := acrossMove(t, root, answer{}, srcs, at(root, "dst"))
	steady(t, reports)
	last := reports[len(reports)-1]
	// Three folders and the twenty files in each.
	if last.itemsTotal != 63 || last.removeTotal != 63 || len(rec.steps) != 3 {
		t.Fatalf("the move counted %d items and %d to remove, in %d steps", last.itemsTotal, last.removeTotal, len(rec.steps))
	}
	if last.leaps != 0 {
		t.Fatalf("%d times the move counted done what it had not worked on", last.leaps)
	}
	removing := false
	for _, p := range reports {
		removing = removing || p.removing
	}
	if !removing {
		t.Fatal("the move never said it was removing the originals")
	}
	if got := names(t, at(root, "src")); len(got) != 0 {
		t.Fatalf("the move left %v behind", got)
	}
	if contents(t, at(root, "dst/d2/00019.jpg")) != "src/d2/00019.jpg" {
		t.Fatal("the move lost a file")
	}
}

// What a move across volumes skips or merges still counts as done, so
// the move ends with all of it done.
func TestAMoveAcrossVolumesThatSkipsAndMergesEndsFull(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "src/a/one.txt", "src/a/two.txt", "src/a/deep/three.txt", "src/b.txt", "src/c/four.txt",
		"dst/a/one.txt", "dst/a/deep/", "dst/b.txt")
	// The first goes across volumes, so the two that clash are counted.
	srcs := []string{at(root, "src/c"), at(root, "src/a"), at(root, "src/b.txt")}
	for _, ans := range []answer{{choice: choiceSkip, all: true}, {choice: choiceReplace, all: true}} {
		t.Run(fmt.Sprint(ans.choice), func(t *testing.T) {
			sub := t.TempDir()
			if err := os.CopyFS(sub, os.DirFS(root)); err != nil {
				t.Fatal(err)
			}
			moved := make([]string, len(srcs))
			for i, s := range srcs {
				rel, _ := filepath.Rel(root, s)
				moved[i] = filepath.Join(sub, rel)
			}
			_, reports := acrossMove(t, sub, ans, moved, at(sub, "dst"))
			steady(t, reports)
			// What was skipped or merged is counted done without work,
			// and says so, to stay out of the speed.
			if reports[len(reports)-1].leaps == 0 {
				t.Fatal("the move never said it counted done what it had not worked on")
			}
			if contents(t, at(sub, "dst/c/four.txt")) != "src/c/four.txt" {
				t.Fatal("the move lost a file")
			}
		})
	}
}

// The undo of a move goes back across volumes as the move went, counted
// the same.
func TestTheUndoOfAMoveAcrossVolumesCountsAllOfItFirst(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "src/a/one.txt", "src/a/two.txt", "src/b/three.txt", "dst/")
	rec, _ := acrossMove(t, root, answer{}, []string{at(root, "src/a"), at(root, "src/b")}, at(root, "dst"))
	var reports []progress
	e := testEnv(root, answer{}, nil)
	e.across, e.reportEvery = true, time.Nanosecond
	e.report = func(p progress) { reports = append(reports, p) }
	if _, err := runJob(context.Background(), job{kind: OpUndo, undo: &rec}, e); err != nil {
		t.Fatal(err)
	}
	steady(t, reports)
	if contents(t, at(root, "src/a/two.txt")) != "src/a/two.txt" || len(names(t, at(root, "dst"))) != 0 {
		t.Fatal("the undo did not move both folders back")
	}
}

// A move the file system does by a rename counts each item as one,
// whatever it holds, and never counts what is in it.
func TestAMoveByRenameCountsEachItemAsOne(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "src/a/one.txt", "src/a/two.txt", "src/b.txt", "dst/")
	var reports []progress
	e := testEnv(root, answer{}, nil)
	e.reportEvery = time.Nanosecond
	e.report = func(p progress) { reports = append(reports, p) }
	srcs := []string{at(root, "src/a"), at(root, "src/b.txt")}
	if _, err := runJob(context.Background(), job{kind: OpMove, srcs: srcs, dest: at(root, "dst")}, e); err != nil {
		t.Fatal(err)
	}
	for _, p := range reports {
		if p.counting || p.itemsTotal != 2 || p.bytesTotal != 0 {
			t.Fatalf("a move by rename reported %+v", p)
		}
	}
	if last := reports[len(reports)-1]; last.items != 2 {
		t.Fatalf("the move ended with %d of 2 items", last.items)
	}
}

// A copy of many files reads into one buffer, not one for each file.
func TestACopyReadsEveryFileIntoOneBuffer(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "src/a.txt", "src/b.txt", "src/c.txt", "dst/")
	r := &runner{ctx: context.Background(), env: testEnv(root, answer{}, nil)}
	var first *byte
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := r.copyItem(at(root, "src/"+name), at(root, "dst/"+name), false, false, true); err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = &r.buf[0]
		}
		if &r.buf[0] != first || len(r.buf) != copyBuffer {
			t.Fatal("the copy made a buffer anew")
		}
	}
}

// feed gives c samples of an operation that does so many items and so
// many bytes in each of n stretches of a fifth of a second, from now on,
// and returns when they end.
func feed(c *costs, now time.Time, n int, items float64, bytes int64) time.Time {
	c.add(now, c.steps, c.bytes)
	for range n {
		now = now.Add(200 * time.Millisecond)
		c.add(now, c.steps+items, c.bytes+bytes)
	}
	return now
}

// Small files cost by the item and big ones by the byte: once an
// operation has done both, it knows each cost.
func TestTheCostsOfItemsAndBytesAreToldApart(t *testing.T) {
	var c costs
	// Thirty small files a second, then a hundred megabytes a second.
	now := feed(&c, time.Now(), 50, 6, 6*4<<10)
	if !c.known() || math.Abs(c.item-1.0/30) > 0.004 {
		t.Fatalf("small files alone cost %v s each, want about a thirtieth", c.item)
	}
	feed(&c, now, 50, 0, 20<<20)
	if math.Abs(c.item-1.0/30) > 0.004 {
		t.Fatalf("an item costs %v s, want about a thirtieth", c.item)
	}
	if rate := itemBytes / c.chunk; math.Abs(rate-100<<20) > 10<<20 {
		t.Fatalf("the bytes go at %.1f MB/s, want about 100", rate/(1<<20))
	}
	// 3,000 small files and 4 GB left: 100 s of files and 41 s of bytes.
	if left := c.left(0, 3000, 0, 4<<30); math.Abs(left-141) > 12 {
		t.Fatalf("the time left is %.0f s, want about 141", left)
	}
	// By bytes alone the files would be nothing, and by items the 4 GB.
	if share := c.share(3000, 3001, 0, 4<<30); math.Abs(share-100.0/141) > 0.06 {
		t.Fatalf("with the files done it is %.2f of the way, want about 0.71", share)
	}
}

// Where every stretch holds the same share of items and bytes, the two
// costs cannot be told apart, and the time left is still right for more
// of the same.
func TestCostsThatCannotBeToldApartStillSayTheTimeLeft(t *testing.T) {
	var c costs
	// Ten files of 200 KB a second.
	feed(&c, time.Now(), 60, 2, 2*200<<10)
	if c.item <= 0 || c.chunk <= 0 {
		t.Fatalf("the costs are %v an item and %v a chunk", c.item, c.chunk)
	}
	if left := c.left(0, 500, 0, 500*200<<10); math.Abs(left-50) > 2 {
		t.Fatalf("500 more files take %.1f s, want 50", left)
	}
}

// Before an operation has shown what its work costs, an item weighs as
// much as itemBytes bytes, and a single file is as far as its bytes are.
func TestTheShareBeforeAnyCostIsKnown(t *testing.T) {
	var c costs
	if c.known() || c.left(0, 10, 0, 1<<20) != 0 {
		t.Fatal("the time left is known before any sample")
	}
	one := progress{itemsTotal: 1, bytes: 50, bytesTotal: 100}
	steps, total := one.steps()
	if got := c.share(steps, total, one.bytes, one.bytesTotal); got != 0.5 {
		t.Fatalf("half of one file is %v of the way", got)
	}
	// A thousand empty files and one of a megabyte: the files are most of it.
	many := progress{items: 1000, itemsTotal: 1001, bytesTotal: 1 << 20}
	steps, total = many.steps()
	if got := c.share(steps, total, many.bytes, many.bytesTotal); got < 0.9 || got >= 1 {
		t.Fatalf("with every small file done it is %v of the way", got)
	}
}

// The card's numbers: the name of the item holds long enough to read,
// the count shows while it counts, and a move says when it removes the
// originals.
func TestATickSaysWhatTheOperationIsAt(t *testing.T) {
	now := time.Now()
	r := &opRun{id: 1}
	r.last = progress{counting: true, counted: 1234, itemsTotal: 3}
	if tick := r.tick(now); !tick.Unknown || tick.Counted != 1234 {
		t.Fatalf("counting shows %+v", tick)
	}
	r.last = progress{items: 1, itemsTotal: 10, bytes: 10, bytesTotal: 100, current: "/a/one.jpg"}
	if tick := r.tick(now); tick.File != "one.jpg" || tick.Unknown {
		t.Fatalf("the first item shows %+v", tick)
	}
	r.last.current = "/a/two.jpg"
	if tick := r.tick(now.Add(nameEvery / 2)); tick.File != "one.jpg" {
		t.Fatalf("the name changed to %q too soon to read", tick.File)
	}
	if tick := r.tick(now.Add(nameEvery)); tick.File != "two.jpg" {
		t.Fatalf("the name stayed %q", tick.File)
	}
	r.last.removing = true
	r.meter.shown = 1 << 20
	if tick := r.tick(now); tick.Doing != removingOriginals || tick.Speed != 0 {
		t.Fatalf("removing shows %+v", tick)
	}
	// Totals that grow hold the progress where it was said to be.
	was := r.tick(now).Done
	r.last.bytesTotal, r.last.itemsTotal = 10000, 1000
	if tick := r.tick(now); tick.Done != was {
		t.Fatalf("the progress went from %v to %v as the totals grew", was, tick.Done)
	}
}

func TestTheCardSaysCountingAndRemoving(t *testing.T) {
	h := newDndHarness(t)
	view := OpView{ID: 1, Title: "Moving 15 items", Kind: OpMove,
		OpProgress: OpProgress{Unknown: true, Counted: 1234}}
	h.ui(func(b *browser, u *gunim.UI) { b.ops.set(Ops{Ops: []OpView{view}}, u) })
	h.ui(func(b *browser, u *gunim.UI) {
		c := b.ops.card(1)
		if c.big.Text != "Counting…" || c.file.Text != "1,234 items found" {
			t.Errorf("counting, the card says %q and %q", c.big.Text, c.file.Text)
		}
		b.ops.tick(OpTick{ID: 1, OpProgress: OpProgress{Done: 0.5, Items: 114, ItemsTotal: 1515, Bytes: 50, BytesTotal: 100,
			File: "00002.jpg", Doing: removingOriginals}}, u)
		if c.big.Text != "114 of 1,515" || c.file.Text != removingOriginals || c.ratesRow.on {
			t.Errorf("removing, the card says %q and %q, with its speeds %v", c.big.Text, c.file.Text, c.ratesRow.on)
		}
	})
}
