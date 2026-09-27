package match

import (
	"slices"
	"testing"
)

func titles(items []Item, found []Found) []string {
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = items[f.Index].Title
	}
	return out
}

func TestAWordTypedWholeFindsTheItemNamedByIt(t *testing.T) {
	items := []Item{
		{Title: "Write a starting keyboard shortcuts file"},
		{Title: "New pane on Ubuntu (WSL)"},
	}
	got := titles(items, Rank(items, "wsl"))
	if want := []string{"New pane on Ubuntu (WSL)", "Write a starting keyboard shortcuts file"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWordStartsRankAboveLettersInTheMiddle(t *testing.T) {
	items := []Item{{Title: "Close pane"}, {Title: "Split right"}, {Title: "Paste"}}
	found := Rank(items, "sr")
	if len(found) != 1 || items[found[0].Index].Title != "Split right" {
		t.Fatalf("got %q, want Split right alone", titles(items, found))
	}
	if !slices.Equal(found[0].At, []int{0, 6}) {
		t.Fatalf("matched letters %v, want the S and the r", found[0].At)
	}
}

func TestOtherWordsFindAnItemBelowTitleMatches(t *testing.T) {
	items := []Item{{Title: "Connect to window", Also: []string{"take over"}}, {Title: "Take a screenshot"}}
	got := titles(items, Rank(items, "take"))
	if want := []string{"Take a screenshot", "Connect to window"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAnEmptyQueryFindsEverythingInOrder(t *testing.T) {
	items := []Item{{Title: "bbb"}, {Title: "a"}, {Title: "cc"}}
	if got := titles(items, Rank(items, "")); !slices.Equal(got, []string{"bbb", "a", "cc"}) {
		t.Fatalf("got %q", got)
	}
}

func TestAQueryTypedWholeMarksTheRunItNames(t *testing.T) {
	at, _, ok := Find("report", "report.txt")
	if !ok || !slices.Equal(at, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("report marks %v in report.txt, want the first six letters", at)
	}
}
