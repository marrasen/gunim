package asio

import "testing"

func TestFitBuffer(t *testing.T) {
	for _, c := range []struct{ want, lo, hi, pref, gran, out int }{
		{0, 64, 2048, 256, -1, 256},
		{1000, 64, 2048, 256, -1, 1024},
		{5000, 64, 2048, 256, -1, 2048},
		{10, 64, 2048, 256, -1, 64},
		{700, 64, 2048, 256, 0, 256},
		{700, 96, 1024, 192, 32, 704},
		{700, 512, 512, 512, 1, 512},
		{1100, 100, 1000, 500, 100, 1000},
	} {
		if got := fitBuffer(c.want, c.lo, c.hi, c.pref, c.gran); got != c.out {
			t.Errorf("fitBuffer(%d, %d, %d, %d, %d) = %d, want %d", c.want, c.lo, c.hi, c.pref, c.gran, got, c.out)
		}
	}
}
