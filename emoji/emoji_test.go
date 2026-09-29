package emoji

import (
	"slices"
	"testing"
)

func TestTheGroupsHoldEveryEmojiOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range Groups() {
		if len(g.Emoji) == 0 {
			t.Fatalf("group %q is empty", g.Name)
		}
		for _, e := range g.Emoji {
			if seen[e.Text] {
				t.Fatalf("%s %q is listed twice", e.Text, e.Name)
			}
			seen[e.Text] = true
		}
	}
	if len(seen) < 1500 {
		t.Fatalf("%d emoji, want the whole set", len(seen))
	}
}

func TestSearchFindsByTheStartsOfWords(t *testing.T) {
	got := Search("thumbs")
	if len(got) == 0 || got[0].Text != "\U0001F44D" {
		t.Fatalf("Search(thumbs) = %v, want thumbs up first", got)
	}
	if got := Search("face tears"); !slices.ContainsFunc(got, func(e Emoji) bool { return e.Text == "\U0001F602" }) {
		t.Fatal("Search(face tears) misses face with tears of joy")
	}
	if got := Search("umbs"); len(got) != 0 {
		t.Fatalf("Search(umbs) = %v, want none: words match from their start", got)
	}
	if Search("  ") != nil {
		t.Fatal("an empty search found emoji")
	}
}

func TestLookupFindsAnEmojiByItsText(t *testing.T) {
	if e, ok := Lookup("\U0001F44D"); !ok || e.Name != "thumbs up" {
		t.Fatalf("Lookup(thumbs up) = %+v, %v", e, ok)
	}
	if _, ok := Lookup("A"); ok {
		t.Fatal("Lookup(A) found an emoji")
	}
}

func TestSearchPutsWholeWordsFirst(t *testing.T) {
	got := Search("heart")
	if len(got) == 0 || got[0].Name != "red heart" {
		t.Fatalf("a search for heart starts with %q, want red heart", got[0].Name)
	}
	if got := Search("thumbs up"); got[0].Name != "thumbs up" {
		t.Fatalf("a search for thumbs up starts with %q", got[0].Name)
	}
}
