package glfw

import "testing"

func TestTheAccentSettingReads(t *testing.T) {
	c, on, err := accentBorder()
	if err != nil {
		t.Fatal(err)
	}
	if on && c.A != 0xff {
		t.Fatalf("the accent colour %v is not opaque", c)
	}
}
