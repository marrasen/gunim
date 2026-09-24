// Package theme controls how widgets look and move, and animates every
// value when the theme changes.
//
// A widget declares each value it wants themed as a [Token], with a
// default: a colour, a length, a padding, a font size, even the spring
// its hover animates with. A [Theme] gives tokens values; anything it
// leaves out keeps its default. A window keeps one animated value per
// token, in a [Live], and switching themes retargets them all, so a
// switch from dark to light moves colours, paddings, radii and motion
// together.
//
// For that to work, a widget reads tokens every time it lays out or
// paints, never copying one into a field. The rule of thumb is to
// animate state and look up style: a button animates how hovered it is,
// from 0 to 1, and blends the theme's idle and hover colours by that
// each frame.
//
//	var ButtonFill = theme.Color("button.fill", color.NRGBA{...})
//
//	fill := ButtonFill.Get(f.Theme)
package theme

import (
	"fmt"
	"image/color"
	"reflect"
	"sync"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
)

// A Token names one themeable value of type T and gives its default.
// Declare tokens once, as package variables, next to the widget that
// reads them.
type Token[T any] struct {
	key   string
	def   T
	codec anim.Codec[T]
}

var (
	keysMu sync.Mutex
	keys   = map[string]reflect.Type{}
)

// New declares a token with its own codec. The key names it within a
// [Theme], and must be unique; two tokens with one key and different
// types panic.
func New[T any](key string, def T, c anim.Codec[T]) Token[T] {
	t := reflect.TypeFor[T]()
	keysMu.Lock()
	defer keysMu.Unlock()
	if prev, ok := keys[key]; ok && prev != t {
		panic(fmt.Sprintf("theme: %q is already a %s token", key, prev))
	}
	keys[key] = t
	return Token[T]{key: key, def: def, codec: c}
}

// Color declares a colour token.
func Color(key string, def color.NRGBA) Token[color.NRGBA] {
	return New(key, def, anim.ColorCodec)
}

// Length declares a length token in logical pixels: a radius, a gap, a
// font size.
func Length(key string, def float32) Token[float32] { return New(key, def, anim.FloatCodec) }

// Insets declares a padding or margin token.
func Insets(key string, def geom.Insets) Token[geom.Insets] {
	return New(key, def, anim.InsetsCodec)
}

// Spring declares a motion token.
func Spring(key string, def anim.Spring) Token[anim.Spring] {
	return New(key, def, anim.SpringCodec)
}

// Key returns the token's name.
func (t Token[T]) Key() string { return t.key }

// Default returns the value the token has in a theme that leaves it out.
func (t Token[T]) Default() T { return t.def }

// Get returns the token's value in l right now, part way to its target
// while a theme switch is running. A nil Live gives the default.
func (t Token[T]) Get(l *Live) T {
	if l == nil {
		return t.def
	}
	s, ok := l.slots[t.key]
	if !ok {
		s = &slot[T]{a: anim.New(t.in(l.active), t.codec), token: t}
		l.slots[t.key] = s
	}
	typed, ok := s.(*slot[T])
	if !ok {
		panic(fmt.Sprintf("theme: %q read as the wrong type", t.key))
	}
	return typed.a.Value()
}

// in returns the token's value in th, or its default.
func (t Token[T]) in(th Theme) T {
	if v, ok := th.values[t.key]; ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}
	return t.def
}

// A Theme is a named set of token values.
type Theme struct {
	Name   string
	values map[string]any
}

// With returns a copy of th with entries added, replacing any value th
// already gives the same token. It is how an application adds its own
// tokens to a widget library's theme.
//
//	light := widget.Light().With(theme.Set(Background, white))
func (th Theme) With(entries ...Entry) Theme {
	out := Theme{Name: th.Name, values: make(map[string]any, len(th.values)+len(entries))}
	for k, v := range th.values {
		out.values[k] = v
	}
	for _, e := range entries {
		out.values[e.key] = e.value
	}
	return out
}

// An Entry is one token's value in a theme, made by [Set].
type Entry struct {
	key   string
	value any
}

// Set gives token t the value v.
func Set[T any](t Token[T], v T) Entry { return Entry{key: t.key, value: v} }

// Make builds a theme from entries. A later entry for the same token
// wins.
func Make(name string, entries ...Entry) Theme {
	th := Theme{Name: name, values: make(map[string]any, len(entries))}
	for _, e := range entries {
		th.values[e.key] = e.value
	}
	return th
}

// Switch is the motion a theme switch runs with. A theme can set it too:
// the new theme's value is the one used.
var Switch = Spring("theme.switch", anim.Spring{Response: 0.6, Damping: 1})

// A Live is the theme in force in one window: every token read so far,
// each an animated value. It belongs to the window's UI goroutine.
type Live struct {
	active Theme
	slots  map[string]stepper
}

type stepper interface {
	step(dt time.Duration) bool
	retarget(th Theme, m anim.Motion)
}

type slot[T any] struct {
	a     *anim.Animated[T]
	token Token[T]
}

func (s *slot[T]) step(dt time.Duration) bool { return s.a.Step(dt) }

func (s *slot[T]) retarget(th Theme, m anim.Motion) { s.a.Animate(s.token.in(th), m) }

// NewLive returns the live form of th, at rest.
func NewLive(th Theme) *Live {
	return &Live{active: th, slots: map[string]stepper{}}
}

// Active returns the theme the values are heading for.
func (l *Live) Active() Theme { return l.active }

// Use switches to th, animating every token from where it is now. Tokens
// first read after the switch start at th's values.
func (l *Live) Use(th Theme) {
	l.active = th
	m := Switch.in(th)
	for _, s := range l.slots {
		s.retarget(th, m)
	}
}

// Step advances every value by dt and reports whether any is still
// moving. The engine calls it once a frame.
func (l *Live) Step(dt time.Duration) bool {
	moving := false
	for _, s := range l.slots {
		if s.step(dt) {
			moving = true
		}
	}
	return moving
}
