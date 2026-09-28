package widget

import (
	"testing"

	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/theme"
)

func TestAnIconButtonAtASizeOfItsOwnFitsItsIcon(t *testing.T) {
	b := NewIconButton(icon.Funnel, "Filter")
	b.IconSize = theme.Length("test.icon.small", 12)
	_, run := stage(t, Row(b))
	run(2)
	want := 12 + 2*IconPadding.Default()
	if b.size.W != want || b.size.H != want {
		t.Fatalf("the button is %v, want %v square", b.size, want)
	}
}

func TestAnActiveButtonFadesToTheAccent(t *testing.T) {
	b := NewIconButton(icon.Columns3, "Show as a column")
	_, run := stage(t, Row(b))
	run(2)
	if b.lit.Value() != 0 {
		t.Fatal("a button that is not active is lit")
	}
	b.Active = true
	run(60)
	if b.lit.Value() < 0.99 {
		t.Fatalf("an active button is lit %v after a second, want 1", b.lit.Value())
	}
	b.Active = false
	run(60)
	if b.lit.Value() > 0.01 {
		t.Fatalf("a button no longer active is lit %v after a second, want 0", b.lit.Value())
	}
}
