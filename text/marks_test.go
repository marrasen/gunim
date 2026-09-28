package text

import "testing"

func TestADecomposedLetterDrawsAsItsComposedForm(t *testing.T) {
	f := Default()
	for _, s := range []string{"Det är sant", "Ställverk"} {
		composed := map[string]string{"Det är sant": "Det är sant", "Ställverk": "Ställverk"}[s]
		p := f.Layout(s, Style{Size: 14}, 0)
		q := f.Layout(composed, Style{Size: 14}, 0)
		if got, want := p.Size.W, q.Size.W; got != want {
			t.Errorf("%q is %v wide, want %v, as wide as %q", s, got, want, composed)
		}
	}
}
