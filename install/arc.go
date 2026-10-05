package install

import "math"

// arc is part of a ring, as a coverage mask: the ring's progress round
// the program's icon. It starts start round from the top and runs
// clockwise, with round ends. Its sizes are in thousandths of the box it is drawn into,
// so the mask is the same at every pixel density.
type arc struct {
	// start is where the arc starts, in tenths of a degree clockwise
	// from the top, and sweep how much of the ring it draws, from 0 to
	// 3600.
	start int16
	sweep int16
	// width is the ring's thickness, in thousandths of the box's width.
	width int16
}

// Settled implements [paint.Shape]: a whole ring stays as it is, and a
// part one changes as the work goes on.
func (a arc) Settled() bool { return a.sweep >= 3600 }

// Coverage implements [paint.Shape].
func (a arc) Coverage(w, h int) []byte {
	out := make([]byte, w*h)
	if a.sweep <= 0 || w <= 0 || h <= 0 {
		return out
	}
	size := float64(min(w, h))
	half := float64(a.width) / 1000 * size / 2
	mid := size/2 - half - 1 // the ring's middle line
	cx, cy := float64(w)/2, float64(h)/2
	sweep := float64(min(a.sweep, 3600)) / 10 * math.Pi / 180
	start := float64(a.start) / 10 * math.Pi / 180
	// The ends' centres, for their round caps.
	endX, endY := cx+mid*math.Sin(start+sweep), cy-mid*math.Cos(start+sweep)
	startX, startY := cx+mid*math.Sin(start), cy-mid*math.Cos(start)
	for y := range h {
		for x := range w {
			px, py := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(px, py)
			cov := 0.0
			// The angle from the arc's start, clockwise, from 0 to 2π.
			theta := math.Mod(math.Atan2(px, -py)-start+4*math.Pi, 2*math.Pi)
			if theta <= sweep || a.sweep >= 3600 {
				cov = clamp01(half - math.Abs(d-mid) + 0.5)
			}
			if a.sweep < 3600 {
				for _, c := range [2][2]float64{{startX, startY}, {endX, endY}} {
					e := math.Hypot(float64(x)+0.5-c[0], float64(y)+0.5-c[1])
					cov = math.Max(cov, clamp01(half-e+0.5))
				}
			}
			out[y*w+x] = uint8(cov*255 + 0.5)
		}
	}
	return out
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
