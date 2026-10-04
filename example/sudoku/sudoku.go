package main

import (
	"math/bits"
	"math/rand/v2"
)

// A Grid is a sudoku's 81 cells, row by row, each 1 to 9 or 0 for
// empty.
type Grid [81]int8

// Units are the 27 groups that must each hold 1 to 9 once: the rows,
// then the columns, then the boxes, each its cells in order.
var units [27][9]int

// unitsOf holds the row, column and box each cell lies in, and peers
// the 20 other cells sharing one with it.
var (
	unitsOf [81][3]int
	peers   [81][20]int
)

func init() {
	for i := range 9 {
		for j := range 9 {
			units[i][j] = i*9 + j
			units[9+i][j] = j*9 + i
			units[18+i][j] = (i/3*3+j/3)*9 + i%3*3 + j%3
		}
	}
	for u, cells := range &units {
		for _, c := range cells {
			unitsOf[c][u/9] = u
		}
	}
	for c := range 81 {
		n := 0
		for _, u := range unitsOf[c] {
			for _, p := range units[u] {
				if p == c || contains(peers[c][:n], p) {
					continue
				}
				peers[c][n] = p
				n++
			}
		}
	}
}

func contains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// allDigits is the candidates 1 to 9 as bits 1 to 9.
const allDigits = 0x3fe

// candidates returns the digits cell c could hold, as bits.
func (g *Grid) candidates(c int) uint16 {
	used := uint16(0)
	for _, p := range peers[c] {
		used |= 1 << g[p]
	}
	return allDigits &^ used
}

// solutions counts g's solutions, stopping at limit, and fills first
// with the first it finds. It leaves g as it was.
func (g *Grid) solutions(limit int, first *Grid) int {
	work := *g
	n := 0
	work.search(limit, &n, first)
	return n
}

// search fills g by trying the digits of the cell with fewest choices
// first.
func (g *Grid) search(limit int, n *int, first *Grid) {
	best, bestCount := -1, 10
	var bestCands uint16
	for c := range 81 {
		if g[c] != 0 {
			continue
		}
		cands := g.candidates(c)
		k := bits.OnesCount16(cands)
		if k == 0 {
			return
		}
		if k < bestCount {
			best, bestCount, bestCands = c, k, cands
			if k == 1 {
				break
			}
		}
	}
	if best < 0 {
		if *n == 0 && first != nil {
			*first = *g
		}
		*n++
		return
	}
	for d := int8(1); d <= 9; d++ {
		if bestCands&(1<<d) == 0 {
			continue
		}
		g[best] = d
		g.search(limit, n, first)
		g[best] = 0
		if *n >= limit {
			return
		}
	}
}

// fill fills g with a random solved grid.
func fill(rng *rand.Rand) Grid {
	var g Grid
	var place func(c int) bool
	place = func(c int) bool {
		if c == 81 {
			return true
		}
		cands := g.candidates(c)
		digits := []int8{1, 2, 3, 4, 5, 6, 7, 8, 9}
		rng.Shuffle(len(digits), func(i, j int) { digits[i], digits[j] = digits[j], digits[i] })
		for _, d := range digits {
			if cands&(1<<d) == 0 {
				continue
			}
			g[c] = d
			if place(c + 1) {
				return true
			}
		}
		g[c] = 0
		return false
	}
	place(0)
	return g
}

// Difficulty is how hard a puzzle is: the hardest way of thinking a
// person needs to solve it.
type Difficulty int

// The difficulties, easiest first.
const (
	// Easy needs only a cell that can hold one digit alone.
	Easy Difficulty = iota + 1
	// Medium needs a digit that fits one cell alone of a row, column
	// or box.
	Medium
	// Hard needs a digit kept to one line in a box, or to one box in a
	// line, to rule it out elsewhere.
	Hard
	// Expert needs two cells of a group that share the same two
	// digits, to rule them out of the group's other cells.
	Expert
	// Fiendish needs more than these: a guess, or a cleverer way.
	Fiendish
)

func (d Difficulty) String() string {
	switch d {
	case Easy:
		return "Easy"
	case Medium:
		return "Medium"
	case Hard:
		return "Hard"
	case Expert:
		return "Expert"
	case Fiendish:
		return "Fiendish"
	}
	return "Unknown"
}

// rate solves g as a person would and returns the hardest way of
// thinking it needed.
func rate(g Grid) Difficulty {
	var cand [81]uint16
	for c := range 81 {
		if g[c] == 0 {
			cand[c] = g.candidates(c)
		}
	}
	set := func(c int, d int8) {
		g[c] = d
		cand[c] = 0
		for _, p := range peers[c] {
			cand[p] &^= 1 << d
		}
	}
	hardest := Easy
	for {
		empty := 0
		for c := range 81 {
			if g[c] == 0 {
				empty++
			}
		}
		if empty == 0 {
			return hardest
		}
		// A cell that can hold one digit alone.
		progress := false
		for c := range 81 {
			if g[c] == 0 && bits.OnesCount16(cand[c]) == 1 {
				set(c, int8(bits.TrailingZeros16(cand[c])))
				progress = true
			}
		}
		if progress {
			continue
		}
		// A digit that fits one cell alone of a group.
		for _, u := range &units {
			for d := int8(1); d <= 9; d++ {
				at, count := -1, 0
				for _, c := range u {
					if cand[c]&(1<<d) != 0 {
						at, count = c, count+1
					}
				}
				if count == 1 {
					set(at, d)
					progress = true
				}
			}
		}
		if progress {
			hardest = max(hardest, Medium)
			continue
		}
		if lockedCandidates(&cand) {
			hardest = max(hardest, Hard)
			continue
		}
		if nakedPairs(&cand) {
			hardest = max(hardest, Expert)
			continue
		}
		return Fiendish
	}
}

// lockedCandidates rules out digits that a box keeps to one line, or a
// line to one box, from the rest of the line or box. It reports
// whether it ruled any out.
func lockedCandidates(cand *[81]uint16) bool {
	progress := false
	for _, a := range &units {
		for _, b := range &units {
			// Only a box with a line, either way.
			if (a[0] == b[0] && a == b) || !crosses(a, b) {
				continue
			}
			for d := uint16(1); d <= 9; d++ {
				bit := uint16(1) << d
				inside, outside := false, false
				for _, c := range a {
					if cand[c]&bit == 0 {
						continue
					}
					if contains(b[:], c) {
						inside = true
					} else {
						outside = true
					}
				}
				if !inside || outside {
					continue
				}
				// d lies in a only where a meets b: so in b it lies
				// there too.
				for _, c := range b {
					if !contains(a[:], c) && cand[c]&bit != 0 {
						cand[c] &^= bit
						progress = true
					}
				}
			}
		}
	}
	return progress
}

// crosses reports whether a box and a line meet, in three cells.
func crosses(a, b [9]int) bool {
	n := 0
	for _, c := range a {
		if contains(b[:], c) {
			n++
		}
	}
	return n == 3
}

// nakedPairs rules out the two digits two cells of a group alone can
// hold from the group's other cells. It reports whether it ruled any
// out.
func nakedPairs(cand *[81]uint16) bool {
	progress := false
	for _, u := range &units {
		for i, a := range u {
			if bits.OnesCount16(cand[a]) != 2 {
				continue
			}
			for _, b := range u[i+1:] {
				if cand[b] != cand[a] {
					continue
				}
				for _, c := range u {
					if c != a && c != b && cand[c]&cand[a] != 0 {
						cand[c] &^= cand[a]
						progress = true
					}
				}
			}
		}
	}
	return progress
}

// A Puzzle is a grid to solve and its solution.
type Puzzle struct {
	Givens, Solution Grid
	Difficulty       Difficulty
}

// generate makes a puzzle of difficulty d from seed: the same seed
// makes the same puzzle. It takes the closest it finds in a few tries.
func generate(seed uint64, d Difficulty) Puzzle {
	var best Puzzle
	bestGap := 99
	for try := range uint64(60) {
		rng := rand.New(rand.NewPCG(seed, try*0x9e3779b97f4a7c15+1))
		p := carve(rng, d)
		gap := int(p.Difficulty) - int(d)
		if gap < 0 {
			gap = -gap
		}
		if gap < bestGap {
			best, bestGap = p, gap
		}
		if gap == 0 {
			break
		}
	}
	return best
}

// givensFor is how many givens a puzzle of each difficulty keeps at
// least: an easy one keeps more, to be kind.
var givensFor = map[Difficulty]int{Easy: 38, Medium: 30, Hard: 25, Expert: 23, Fiendish: 22}

// carve fills a grid and takes cells out of it in pairs, opposite each
// other, while the puzzle keeps one solution and stays no harder than
// d, down to the givens d keeps.
func carve(rng *rand.Rand, d Difficulty) Puzzle {
	sol := fill(rng)
	g := sol
	order := rng.Perm(41)
	left := 81
	for _, c := range order {
		if left <= givensFor[d] {
			break
		}
		o := 80 - c
		keepC, keepO := g[c], g[o]
		g[c], g[o] = 0, 0
		removed := 2
		if c == o {
			removed = 1
		}
		if g.solutions(2, nil) != 1 || rate(g) > d {
			g[c], g[o] = keepC, keepO
			continue
		}
		left -= removed
	}
	return Puzzle{Givens: g, Solution: sol, Difficulty: rate(g)}
}
