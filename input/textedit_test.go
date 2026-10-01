package input

import "testing"

func TestCommitReplacesTheCompositionOrTheSelection(t *testing.T) {
	s := TextState{Text: "hello world", Selection: [2]int{6, 11}, Composing: [2]int{11, 11}}
	got := s.Commit("there")
	want := TextEdit{Replace: [2]int{6, 11}, With: "there", Selection: [2]int{11, 11}, Composing: [2]int{11, 11}}
	if got != want {
		t.Fatalf("commit over the selection = %+v, want %+v", got, want)
	}

	s = TextState{Text: "hello wor", Selection: [2]int{9, 9}, Composing: [2]int{6, 9}}
	got = s.Commit("world")
	want = TextEdit{Replace: [2]int{6, 9}, With: "world", Selection: [2]int{11, 11}, Composing: [2]int{11, 11}}
	if got != want {
		t.Fatalf("commit over the composition = %+v, want %+v", got, want)
	}
}

func TestComposeKeepsItsHighlightInsideTheComposition(t *testing.T) {
	s := TextState{Text: "ab", Selection: [2]int{1, 1}, Composing: [2]int{1, 1}}
	got := s.Compose("にほ", [2]int{3, 99})
	want := TextEdit{Replace: [2]int{1, 1}, With: "にほ", Selection: [2]int{4, 7}, Composing: [2]int{1, 7}}
	if got != want {
		t.Fatalf("compose = %+v, want %+v", got, want)
	}
	s, _ = s.Apply(got)
	if s.Text != "aにほb" {
		t.Fatalf("text %q after composing, want aにほb", s.Text)
	}
	end := s.Compose("", [2]int{})
	if s, _ = s.Apply(end); s.Text != "ab" || s.Composing[0] != s.Composing[1] {
		t.Fatalf("an empty composition left %q composing %v, want ab with none", s.Text, s.Composing)
	}
}

func TestApplyWorksInTheStretchItHolds(t *testing.T) {
	// The copy holds bytes 100 to 105 of a longer text.
	s := TextState{Text: "hello", Start: 100, Selection: [2]int{105, 105}, Composing: [2]int{105, 105}}
	got, ok := s.Apply(TextEdit{Replace: [2]int{104, 105}, Selection: [2]int{104, 104}, Composing: [2]int{104, 104}})
	if !ok || got.Text != "hell" || got.Start != 100 || got.End() != 104 {
		t.Fatalf("backspace = %+v, %v; want hell from 100", got, ok)
	}
	if _, ok := s.Apply(TextEdit{Replace: [2]int{90, 101}}); ok {
		t.Fatal("an edit reaching outside the stretch applied")
	}
}
