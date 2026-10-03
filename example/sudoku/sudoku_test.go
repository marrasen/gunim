package main

import (
	"testing"
	"time"
)

func TestUnitsAndPeersCoverTheGrid(t *testing.T) {
	for c := range 81 {
		seen := map[int]bool{}
		for _, p := range peers[c] {
			if p == c || seen[p] {
				t.Fatalf("cell %d's peers repeat or hold itself: %v", c, peers[c])
			}
			seen[p] = true
		}
	}
	for u, cells := range units {
		seen := map[int]bool{}
		for _, c := range cells {
			seen[c] = true
		}
		if len(seen) != 9 {
			t.Fatalf("unit %d holds %v", u, cells)
		}
	}
}

func TestAGeneratedPuzzleHasOneSolutionAndItsDifficulty(t *testing.T) {
	for _, d := range []Difficulty{Easy, Medium, Hard, Expert} {
		start := time.Now()
		p := generate(uint64(d)*101, d)
		took := time.Since(start)
		var found Grid
		if n := p.Givens.solutions(2, &found); n != 1 {
			t.Fatalf("%v: %d solutions, want 1", d, n)
		}
		if found != p.Solution {
			t.Fatalf("%v: the solution found is not the one kept", d)
		}
		givens := 0
		for c := range 81 {
			if p.Givens[c] != 0 {
				givens++
				if p.Givens[c] != p.Solution[c] {
					t.Fatalf("%v: given %d is not the solution's", d, c)
				}
			}
		}
		t.Logf("%v: rated %v, %d givens, in %v", d, p.Difficulty, givens, took)
		if p.Difficulty != d {
			t.Errorf("asked for %v, made %v", d, p.Difficulty)
		}
		if took > 3*time.Second {
			t.Errorf("%v took %v to make", d, took)
		}
	}
}

func TestTheSameSeedMakesTheSamePuzzle(t *testing.T) {
	a, b := generate(42, Medium), generate(42, Medium)
	if a != b {
		t.Fatal("seed 42 made two puzzles")
	}
}
