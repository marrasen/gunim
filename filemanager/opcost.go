package filemanager

import (
	"math"
	"time"
)

// What an operation's work costs before it has shown it, and how the
// costs it shows are kept.
const (
	// itemBytes is how many bytes take as long as an item does by itself:
	// opening it, making it, closing it and setting its time. It is what
	// holds until the operation shows otherwise, and the unit the bytes
	// are counted in.
	itemBytes = 512 << 10
	// costSpread is how many times more or less than itemBytes an item
	// may turn out to cost.
	costSpread = 32
	// costMemory is how many seconds a sample counts for before it has
	// mostly faded, so the costs follow a disk or a line that changes.
	costMemory = 30.0
	// costTie is how strongly the cost of an item is held to that of
	// itemBytes bytes where the samples do not tell the two apart.
	costTie = 0.02
	// costKnown is how many seconds of samples it takes to say how long
	// is left.
	costKnown = 1.0
	// removeShare is how much of an item's cost it is to take one away.
	removeShare = 0.25
)

// costs learns what an operation's work costs in time as it goes: so
// long for each item, whatever its size, and so long for each byte. Many
// small files cost by the item and a few big ones by the byte, and an
// operation of both is only as far along as the time it has left says,
// which neither its bytes nor its items alone tell.
//
// It fits the two costs to samples of the items and bytes done in a
// stretch of time, by least squares, the older samples fading. A sample
// weighs as little as it has work in it, so the costs are those of the
// work on average, not those of the stretches that did the most.
type costs struct {
	// at is when the last sample was taken, at steps items and bytes
	// bytes.
	at    time.Time
	steps float64
	bytes int64
	// The sums of the samples: of the items x, the bytes y in units of
	// itemBytes and the seconds t they took.
	xx, xy, yy, xt, yt float64
	// seen is the seconds sampled.
	seen float64
	// item and chunk are the seconds an item and itemBytes bytes take.
	item, chunk float64
}

// rest has the next sample start anew, after time that was no work.
func (c *costs) rest() { c.at = time.Time{} }

// add takes how far the operation has got by now: steps items, where
// taking one away is a share of one, and bytes bytes.
func (c *costs) add(now time.Time, steps float64, bytes int64) {
	if c.at.IsZero() {
		c.at, c.steps, c.bytes = now, steps, bytes
		return
	}
	t := now.Sub(c.at).Seconds()
	if t < speedEvery.Seconds() {
		return
	}
	x, y := steps-c.steps, float64(bytes-c.bytes)/itemBytes
	switch {
	case x < 0 || y < 0:
		// It went back, as an extraction that starts again does.
		c.at, c.steps, c.bytes = now, steps, bytes
		return
	case x == 0 && y == 0:
		// Nothing done yet: the time counts toward what is done next.
		return
	}
	c.at, c.steps, c.bytes = now, steps, bytes
	fade, w := math.Exp(-t/costMemory), 1/(x+y)
	c.xx, c.xy, c.yy = c.xx*fade+w*x*x, c.xy*fade+w*x*y, c.yy*fade+w*y*y
	c.xt, c.yt = c.xt*fade+w*x*t, c.yt*fade+w*y*t
	c.seen += t
	c.fit()
}

// fit works the costs out from the sums.
func (c *costs) fit() {
	// tied fits the one cost left when an item costs k times a chunk.
	tied := func(k float64) (item, chunk float64) {
		d := k*k*c.xx + 2*k*c.xy + c.yy
		if d <= 0 {
			return 0, 0
		}
		chunk = max((k*c.xt+c.yt)/d, 0)
		return k * chunk, chunk
	}
	// The tie holds the two costs together as far as the samples leave
	// them open, as they do while every sample has much the same share
	// of items and bytes.
	tie := costTie * min(c.xx, c.yy)
	a, b, d := c.xx+tie, c.xy-tie, c.yy+tie
	det := a*d - b*b
	if det <= 1e-9*a*d {
		c.item, c.chunk = tied(1)
		return
	}
	item, chunk := (c.xt*d-c.yt*b)/det, (c.yt*a-c.xt*b)/det
	switch {
	case item <= 0 && chunk <= 0:
		item, chunk = tied(1)
	case item > costSpread*chunk:
		item, chunk = tied(costSpread)
	case chunk > costSpread*item:
		item, chunk = tied(1.0 / costSpread)
	}
	c.item, c.chunk = item, chunk
}

// known reports whether the costs are known well enough to say how long
// is left.
func (c *costs) known() bool { return c.seen >= costKnown && (c.item > 0 || c.chunk > 0) }

// weights are the costs to weigh the work by: those learnt, or before
// any are, an item as itemBytes bytes.
func (c *costs) weights() (item, chunk float64) {
	if c.item <= 0 && c.chunk <= 0 {
		return 1, 1
	}
	return c.item, c.chunk
}

// share is how far an operation has got, from 0 to 1, with steps of
// stepsTotal items and bytes of bytesTotal bytes done: the share of the
// time all of it takes that what is done took.
func (c *costs) share(steps, stepsTotal float64, bytes, bytesTotal int64) float64 {
	item, chunk := c.weights()
	total := item*stepsTotal + chunk*float64(bytesTotal)/itemBytes
	if total <= 0 {
		return 0
	}
	done := item*min(steps, stepsTotal) + chunk*float64(min(bytes, bytesTotal))/itemBytes
	return min(done/total, 1)
}

// left is how many seconds the rest takes, or 0 while that is unknown.
func (c *costs) left(steps, stepsTotal float64, bytes, bytesTotal int64) float64 {
	if !c.known() {
		return 0
	}
	return c.item*max(stepsTotal-steps, 0) + c.chunk*float64(max(bytesTotal-bytes, 0))/itemBytes
}

// whole is the items of p that are done, as the costs count them: an
// original a move takes away is a share of an item.
func (p progress) whole() float64 { return float64(p.items) + removeShare*float64(p.removed) }

// steps is the items of p done and to do, as the costs count them, with
// the item p is on done as far as its bytes are: where the bytes are
// ahead of the items, by up to one item.
func (p progress) steps() (done, total float64) {
	done, total = p.whole(), float64(p.itemsTotal)+removeShare*float64(p.removeTotal)
	if p.bytesTotal > 0 && p.items < p.itemsTotal {
		ahead := float64(p.bytes)/float64(p.bytesTotal)*float64(p.itemsTotal) - float64(p.items)
		done += min(max(ahead, 0), 1)
	}
	return done, total
}
