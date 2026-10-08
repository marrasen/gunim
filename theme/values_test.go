package theme

import (
	"errors"
	"image/color"
	"strings"
	"testing"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
)

// A choice of a kind JSON holds, and one it does not.
type shade string

var (
	text    = Foreground("test.values.ink", color.NRGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff})
	gap     = Number("test.values.gap", 2)
	margin  = Insets("test.values.margin", geom.Uniform(4))
	glide   = Spring("test.values.glide", anim.Snappy)
	tone    = Choice("test.values.tone", shade("warm"))
	face    = Choice("test.values.face", &struct{ name string }{"sans"})
	counted = New("test.values.count", 3, anim.Codec[int]{})
)

func TestTokensKnowTheirKind(t *testing.T) {
	for key, want := range map[string]Kind{
		fill.Key(): KindColor, text.Key(): KindForeground, pad.Key(): KindLength, gap.Key(): KindNumber,
		margin.Key(): KindInsets, glide.Key(): KindSpring, tone.Key(): KindChoice, counted.Key(): KindOther,
	} {
		info, ok := Lookup(key)
		if !ok {
			t.Fatalf("Lookup(%q) finds no token", key)
		}
		if info.Kind != want {
			t.Errorf("%q is a %s, want a %s", key, info.Kind, want)
		}
	}
	if _, ok := Lookup("test.values.nothing"); ok {
		t.Fatal("Lookup finds a token nobody declared")
	}
	info, _ := Lookup(pad.Key())
	if info.Default != float32(8) {
		t.Fatalf("pad's default = %v, want 8", info.Default)
	}
	all := Tokens()
	for i := 1; i < len(all); i++ {
		if all[i-1].Key >= all[i].Key {
			t.Fatalf("Tokens is out of order at %q, %q", all[i-1].Key, all[i].Key)
		}
	}
	if len(all) != len(Declared()) {
		t.Fatalf("Tokens lists %d, Declared %d", len(all), len(Declared()))
	}
}

func TestValuesBySetAndGetByKey(t *testing.T) {
	th, err := Make("t").WithValue(pad.Key(), 12)
	if err != nil {
		t.Fatal(err)
	}
	if got := pad.Get(NewLive(th)); got != 12 {
		t.Fatalf("pad = %v after WithValue of an int 12, want 12", got)
	}
	if v, ok := th.Value(pad.Key()); !ok || v != float32(12) {
		t.Fatalf("Value = %v, %v, want float32 12", v, ok)
	}
	if _, ok := th.Value(fill.Key()); ok {
		t.Fatal("Value reports a value the theme leaves out")
	}
	th, err = th.WithValue(glide.Key(), anim.Spring{Response: 0, Damping: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := glide.Get(NewLive(th)); got != (anim.Spring{Damping: 1}) {
		t.Fatalf("glide = %v, want the instant spring", got)
	}

	// The wrong type leaves the theme as it was.
	same, err := th.WithValue(fill.Key(), "red")
	var ke *KeyError
	if !errors.As(err, &ke) || ke.Key != fill.Key() {
		t.Fatalf("a string for a colour gives %v, want a KeyError for its key", err)
	}
	if same.Has(fill.Key()) {
		t.Fatal("a value of the wrong type was set")
	}
	if _, err := th.WithValue("test.values.nothing", 1); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("an unknown key gives %v, want ErrUnknownKey", err)
	}

	// Without and Over.
	if th.Without(pad.Key()).Has(pad.Key()) || !th.Without(pad.Key()).Has(glide.Key()) {
		t.Fatal("Without takes out other than the keys it is given")
	}
	if !th.Has(pad.Key()) {
		t.Fatal("Without changed the theme it was called on")
	}
	base := Make("base", Set(pad, 3), Set(fill, color.NRGBA{R: 1, A: 0xff}))
	over := th.Over(base)
	if over.Name != "base" || pad.Get(NewLive(over)) != 12 || fill.Get(NewLive(over)).R != 1 {
		t.Fatalf("Over gives %+v, want base's name and fill with th's pad", over)
	}
	if got := over.Keys(); strings.Join(got, ",") != "test.fill,test.pad,test.values.glide" {
		t.Fatalf("Keys = %v", got)
	}
}

func TestValuesRoundTripThroughJSON(t *testing.T) {
	th := Make("t",
		Set(fill, color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff}),
		Set(text, color.NRGBA{R: 1, G: 2, B: 3, A: 0x80}),
		Set(pad, 12.5),
		Set(margin, geom.Insets{Top: 1, Right: 2, Bottom: 3, Left: 4}),
		Set(glide, anim.Spring{Response: 0.25, Damping: 0.85}),
		Set(tone, "cool"),
		Set(face, &struct{ name string }{"serif"}),
	)
	data, err := MarshalValues(th)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "test.fill": "#5e9cff",
  "test.pad": 12.5,
  "test.values.glide": {"response": 0.25, "damping": 0.85},
  "test.values.ink": "#01020380",
  "test.values.margin": [1, 2, 3, 4],
  "test.values.tone": "cool"
}
`
	if string(data) != want {
		t.Fatalf("MarshalValues wrote\n%s\nwant\n%s", data, want)
	}
	back, err := UnmarshalValues(Make("empty"), data)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{fill.Key(), text.Key(), pad.Key(), margin.Key(), glide.Key(), tone.Key()} {
		a, _ := th.Value(key)
		b, ok := back.Value(key)
		if !ok || a != b {
			t.Errorf("%q came back as %v, want %v", key, b, a)
		}
	}
	if back.Has(face.Key()) {
		t.Error("a font came back from JSON it was never written to")
	}
	if empty, _ := MarshalValues(Make("none")); string(empty) != "{}\n" {
		t.Fatalf("an empty theme writes %q", empty)
	}
}

func TestUnmarshalValuesKeepsTheGoodAndListsTheBad(t *testing.T) {
	base := Make("base", Set(gap, 9))
	data := `{"test.pad": 4, "test.fill": "#abc", "test.values.margin": {"top": 2, "left": 1},
		"test.values.nothing": 1, "test.values.glide": {"response": "fast"}, "test.values.ink": 12,
		"test.values.face": "serif"}`
	th, err := UnmarshalValues(base, []byte(data))
	if err == nil {
		t.Fatal("bad values give no error")
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("%v lists no errors", err)
	}
	bad := make([]string, 0, len(joined.Unwrap()))
	for _, e := range joined.Unwrap() {
		var ke *KeyError
		if !errors.As(e, &ke) {
			t.Fatalf("%v is no KeyError", e)
		}
		bad = append(bad, ke.Key)
	}
	if got := strings.Join(bad, ","); got != "test.values.face,test.values.glide,test.values.ink,test.values.nothing" {
		t.Fatalf("the error lists %s", got)
	}
	if !strings.Contains(err.Error(), ErrUnknownKey.Error()) {
		t.Fatalf("the error does not say the key is unknown: %v", err)
	}
	l := NewLive(th)
	if pad.Get(l) != 4 || fill.Get(l) != (color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff}) || gap.Get(l) != 9 {
		t.Fatalf("the good values and the base's are not all there: pad %v fill %v gap %v", pad.Get(l), fill.Get(l), gap.Get(l))
	}
	if got := margin.Get(l); got != (geom.Insets{Top: 2, Left: 1}) {
		t.Fatalf("margin = %v", got)
	}
	if base.Has(pad.Key()) {
		t.Fatal("UnmarshalValues changed the base theme")
	}
	if _, err := UnmarshalValues(base, []byte("[1, 2]")); err == nil {
		t.Fatal("a JSON array gives no error")
	}
}

func TestHexReadsAndWritesColours(t *testing.T) {
	for in, want := range map[string]color.NRGBA{
		"#fff":     {R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		"#5E9CFF":  {R: 0x5e, G: 0x9c, B: 0xff, A: 0xff},
		"12345678": {R: 0x12, G: 0x34, B: 0x56, A: 0x78},
		"#000000":  {A: 0xff},
	} {
		got, err := ParseHex(in)
		if err != nil || got != want {
			t.Errorf("ParseHex(%q) = %v, %v, want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "#ff", "#ggg", "#1234567"} {
		if _, err := ParseHex(in); err == nil {
			t.Errorf("ParseHex(%q) reads a colour", in)
		}
	}
	if got := Hex(color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x22}); got != "#5e9cff22" {
		t.Fatalf("Hex = %s", got)
	}
}
