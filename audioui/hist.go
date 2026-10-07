package audioui

import (
	"math"
	"slices"
)

// The loudness histogram's bins: a tenth of a LU each, from histLow LUFS up, the last holding everything above.
const (
	histLow  = -80
	histStep = 0.1
	histBins = 900
)

// loudHist holds the mean weighted powers of blocks of sound by their loudness, in bins of a tenth of a LU, each bin
// with its powers, how many and their sum. Gating and percentiles then cost a pass over the bins and a look into
// one or two of them, however long the sound, and read as a pass over all the powers would: the gate's bin is
// read power by power. Powers under histLow LUFS are left out; no gate lets them through.
type loudHist struct {
	bins [histBins]histBin
}

type histBin struct {
	n      int
	sum    float64
	powers []float64
}

// add adds a block's mean weighted power.
func (h *loudHist) add(power float64) {
	if power <= 0 {
		return
	}
	l := LUFSOf(power)
	if l < histLow {
		return
	}
	b := &h.bins[min(int((l-histLow)/histStep), histBins-1)]
	b.n++
	b.sum += power
	b.powers = append(b.powers, power)
}

// side says where bin i lies against floor: 1 all above it, -1 all below, and 0 across it, where its powers are
// read one by one. A hair of slack keeps a power that rounding put in the next bin on the right side.
func side(i int, floor float64) int {
	const slack = 1e-6
	lo := histLow + float64(i)*histStep
	hi := lo + histStep
	if i == histBins-1 {
		hi = math.Inf(1)
	}
	switch {
	case lo-slack > floor:
		return 1
	case hi+slack <= floor:
		return -1
	}
	return 0
}

// over returns the mean and the number of the powers louder than floor, or as loud too where even says so.
func (h *loudHist) over(floor float64, even bool) (mean float64, n int) {
	var sum float64
	for i := range h.bins {
		b := &h.bins[i]
		if b.n == 0 {
			continue
		}
		switch side(i, floor) {
		case 1:
			sum += b.sum
			n += b.n
		case 0:
			for _, p := range b.powers {
				if l := LUFSOf(p); l > floor || even && l == floor {
					sum += p
					n++
				}
			}
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// integrated is the integrated loudness of the blocks added, as [audio.Integrated] reads it, and false where there
// is none: those under -70 LUFS left out, and then those more than 10 LU under the rest.
func (h *loudHist) integrated() (float64, bool) {
	mean, n := h.over(-70, false)
	if n == 0 {
		return math.Inf(-1), false
	}
	if mean, n = h.over(LUFSOf(mean)-10, false); n == 0 {
		return math.Inf(-1), false
	}
	return LUFSOf(mean), true
}

// span is where the loudness of the three-second windows added ranges, as [audio.LoudnessRange] reads it: those
// under -70 LUFS left out, and those more than 20 LU under the rest, from the tenth percentile of the rest to the
// 95th.
func (h *loudHist) span() (low, high float64, ok bool) {
	mean, n := h.over(-70, false)
	if n == 0 {
		return 0, 0, false
	}
	floor := LUFSOf(mean) - 20
	_, n = h.over(floor, true)
	if n == 0 {
		return 0, 0, false
	}
	at := func(q float64) float64 { return h.nth(floor, int(float64(n-1)*q+0.5)) }
	return at(0.10), at(0.95), true
}

// nth returns the loudness k places up from the quietest of the powers at least floor.
func (h *loudHist) nth(floor float64, k int) float64 {
	for i := range h.bins {
		b := &h.bins[i]
		s := side(i, floor)
		if b.n == 0 || s < 0 {
			continue
		}
		if s > 0 && k >= b.n {
			k -= b.n
			continue
		}
		var ls []float64
		for _, p := range b.powers {
			if l := LUFSOf(p); l >= floor {
				ls = append(ls, l)
			}
		}
		if k < len(ls) {
			slices.Sort(ls)
			return ls[k]
		}
		k -= len(ls)
	}
	return math.Inf(-1)
}
