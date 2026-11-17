package maximizer

import (
	"math"
	"testing"
)

func TestTheTruePeakFindsPeaksBetweenTheSamples(t *testing.T) {
	// A sine at a quarter of the rate, its samples where it is at 45°:
	// every sample at 0.707 of its peak, which lies between them. Read
	// once the filter is full of it.
	var tp truePeak
	most := 0.0
	for i := range 2000 {
		v := math.Sin(2*math.Pi*float64(i)/4 + math.Pi/4)
		if p := tp.push(v); i >= tpTaps {
			most = max(most, p)
		}
	}
	if math.Abs(20*math.Log10(most)) > 0.01 {
		t.Errorf("samples at 0.707 of a sine of 1 read a true peak of %.4f, want 1", most)
	}
	// A low sine reads as its samples do.
	tp.reset()
	most = 0
	for i := range 4800 {
		most = max(most, tp.push(0.5*math.Sin(2*math.Pi*100*float64(i)/48000)))
	}
	if math.Abs(most-0.5) > 0.001 {
		t.Errorf("a low sine of 0.5 reads a true peak of %.4f", most)
	}
}

func TestTheTruePeakReadsHighTonesBetweenTheSamples(t *testing.T) {
	// Tones up to 20 kHz, peaking between the samples and between the
	// phases too, read once the filter is full of them.
	for _, f := range []float64{1000, 8000, 12000, 16000, 18000, 20000} {
		for _, off := range []float64{0.5, 1.0 / 16, 0.1, 0.3} {
			w := 2 * math.Pi * f / 48000
			var tp truePeak
			most := 0.0
			for i := range 4 * tpTaps {
				if p := tp.push(math.Cos(w * (float64(i) - off))); i >= tpTaps {
					most = max(most, p)
				}
			}
			if db := 20 * math.Log10(most); math.Abs(db) > 0.01 {
				t.Errorf("a %.0f Hz tone of full scale, peaking %.3f of a frame off, reads %.4f dBTP", f, off, db)
			}
		}
	}
}

func TestTheTruePeakIsTheFiltersEveryPhase(t *testing.T) {
	// Phase 0 is the middle sample, and each other phase is its mirror's
	// backwards, as phases takes it to be.
	for k := range tpTaps {
		want := 0.0
		if k == tpTaps/2-1 {
			want = 1
		}
		if math.Abs(tpFilter[0][k]-want) > 1e-15 {
			t.Fatalf("phase 0, tap %d: %g, want %g", k, tpFilter[0][k], want)
		}
	}
	for p := 1; p < tpPhases; p++ {
		for k := range tpTaps {
			a, b := tpFilter[p][k], tpFilter[tpPhases-p][tpTaps-1-k]
			if math.Abs(a-b) > 1e-15 {
				t.Fatalf("phase %d, tap %d: %g, its mirror %g", p, k, a, b)
			}
		}
	}
	// And phases finds the phases' sums, written out, between the frame
	// before's last phase and the next sample.
	var tp truePeak
	w := make([]float64, 0, 3000)
	last := 0.0
	for i := range 3000 {
		x := math.Sin(float64(i)*0.37) * math.Cos(float64(i)*0.011)
		w = append(w, x)
		got := tp.phases(x)
		want := [tpPhases + 2]float64{last}
		for p := range tpPhases + 1 {
			for k := range tpTaps {
				if j := len(w) - tpTaps + k + p/tpPhases; j >= 0 && j < len(w) {
					want[p+1] += tpFilter[p%tpPhases][k] * w[j]
				}
			}
		}
		last = want[tpPhases]
		for p := range want {
			if math.Abs(got[p]-want[p]) > 1e-14 {
				t.Fatalf("sample %d, phase %d: %g, not %g", i, p-1, got[p], want[p])
			}
		}
	}
}

// TestOverAndMostFindTheTruePeaksRunFinds checks that the runs that skip quiet frames give run's true peaks to the
// bit wherever they are read: over ceiling, and the most of a run.
func TestOverAndMostFindTheTruePeaksRunFinds(t *testing.T) {
	m := music(48000, 3)
	x := make([]float64, len(m))
	for i, v := range m {
		// Fade in and out, so runs start quiet and grow loud, as after a skipped stretch.
		x[i] = float64(v) * math.Abs(math.Sin(2*math.Pi*float64(i)/9000))
	}
	var all, over, most truePeak
	want, got := make([]float64, len(x)), make([]float64, len(x))
	all.run(x, want)
	skipped := 0
	for i := 0; i < len(x); i += 777 {
		e := min(i+777, len(x))
		ceiling := 0.05 + 0.3*float64(i%5)
		over.over(x[i:e], got[i:e], ceiling)
		floor := 0.1 * float64(i%7)
		wantMost := floor
		for j := i; j < e; j++ {
			if want[j] > ceiling && math.Float64bits(got[j]) != math.Float64bits(want[j]) {
				t.Fatalf("frame %d over %v: %v, run's %v", j, ceiling, got[j], want[j])
			}
			if got[j] == 0 && want[j] != 0 {
				skipped++
			}
			wantMost = max(wantMost, want[j])
		}
		if g := most.most(x[i:e], floor); math.Float64bits(g) != math.Float64bits(wantMost) {
			t.Fatalf("frames %d to %d from %v: most %v, run's %v", i, e, floor, g, wantMost)
		}
	}
	if skipped == 0 {
		t.Fatal("over skipped no frame")
	}
}
