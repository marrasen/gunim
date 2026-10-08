package theme

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
)

// Kind is what sort of value a token holds: what an editor shows for
// it, and how a theme file writes it.
type Kind uint8

const (
	// KindOther is a token declared with [New] and a codec of its own.
	KindOther Kind = iota
	// KindColor is a colour, from [Color].
	KindColor
	// KindForeground is a colour drawn on top of others, such as text,
	// from [Foreground].
	KindForeground
	// KindLength is a length in logical pixels, from [Length].
	KindLength
	// KindNumber is a plain number, from [Number].
	KindNumber
	// KindInsets is a padding or margin, from [Insets].
	KindInsets
	// KindSpring is a motion, from [Spring].
	KindSpring
	// KindChoice is a value that changes whole, such as a font, from
	// [Choice].
	KindChoice
)

// String returns the kind's name, such as "colour".
func (k Kind) String() string {
	switch k {
	case KindColor:
		return "colour"
	case KindForeground:
		return "foreground colour"
	case KindLength:
		return "length"
	case KindNumber:
		return "number"
	case KindInsets:
		return "insets"
	case KindSpring:
		return "spring"
	case KindChoice:
		return "choice"
	default:
		return "other"
	}
}

// Info describes a declared token, for a program that lists or edits
// tokens without knowing their Go types.
type Info struct {
	// Key names the token within a theme.
	Key string
	// Kind is what sort of value it holds.
	Kind Kind
	// Default is its value in a theme that leaves it out.
	Default any
	// Type is the Go type of its values.
	Type reflect.Type
}

// Lookup returns the token named key, and false where no token has that
// name.
func Lookup(key string) (Info, bool) {
	keysMu.Lock()
	defer keysMu.Unlock()
	d, ok := keys[key]
	if !ok {
		return Info{}, false
	}
	return Info{Key: key, Kind: d.kind, Default: d.def, Type: d.typ}, true
}

// Tokens returns every token declared so far, sorted by key.
func Tokens() []Info {
	keysMu.Lock()
	out := make([]Info, 0, len(keys))
	for k, d := range keys {
		out = append(out, Info{Key: k, Kind: d.kind, Default: d.def, Type: d.typ})
	}
	keysMu.Unlock()
	slices.SortFunc(out, func(a, b Info) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// ErrUnknownKey is the error for a key no declared token has.
var ErrUnknownKey = errors.New("no token has this key")

// A KeyError is a value that could not be set for one token: an unknown
// key, or a value of the wrong type or form.
type KeyError struct {
	Key string
	Err error
}

// Error implements error.
func (e *KeyError) Error() string { return fmt.Sprintf("theme: %q: %v", e.Key, e.Err) }

// Unwrap returns the reason.
func (e *KeyError) Unwrap() error { return e.Err }

// Value returns the value th gives the token named key, and false where
// th leaves it out. It is th's own value, never the default.
func (th Theme) Value(key string) (any, bool) {
	v, ok := th.values[key]
	return v, ok
}

// Keys returns the keys of the tokens th gives values, sorted.
func (th Theme) Keys() []string {
	out := make([]string, 0, len(th.values))
	for k := range th.values {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Len returns how many tokens th gives values.
func (th Theme) Len() int { return len(th.values) }

// WithValue returns a copy of th that gives the token named key the
// value v. The value must suit the token: a [color.NRGBA] for a colour,
// an [anim.Spring] for a spring, and so on. A number of any Go type
// does for a token of another number type, such as an int for a
// length. A key no token has, or a value of the wrong type, gives an
// error and th unchanged.
func (th Theme) WithValue(key string, v any) (Theme, error) {
	info, ok := Lookup(key)
	if !ok {
		return th, &KeyError{Key: key, Err: ErrUnknownKey}
	}
	cv, err := convert(v, info.Type)
	if err != nil {
		return th, &KeyError{Key: key, Err: err}
	}
	return th.With(Entry{key: key, value: cv}), nil
}

// convert returns v as a value of type t, or an error where it cannot
// be one.
func convert(v any, t reflect.Type) (any, error) {
	if v == nil {
		return nil, fmt.Errorf("no value for a %s", t)
	}
	rv := reflect.ValueOf(v)
	if rv.Type() == t {
		return v, nil
	}
	if isNumber(rv.Kind()) && isNumber(t.Kind()) {
		return rv.Convert(t).Interface(), nil
	}
	return nil, fmt.Errorf("a %s does not suit a %s token", rv.Type(), t)
}

// isNumber reports whether k is one of Go's number kinds.
func isNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// Without returns a copy of th that leaves the tokens named keys out, so
// they take their defaults, or the values of a theme th is laid over.
func (th Theme) Without(keys ...string) Theme {
	out := th.With()
	for _, k := range keys {
		delete(out.values, k)
	}
	return out
}

// Over returns base with th's values laid over it: every value th gives,
// and base's for the tokens th leaves out. It keeps base's name. It is
// how a program applies a user's changes to the theme they started
// from.
func (th Theme) Over(base Theme) Theme {
	out := base.With()
	for k, v := range th.values {
		out.values[k] = v
	}
	return out
}

// MarshalValues writes the values th gives as a JSON object keyed by
// token key, sorted, one key to a line. A colour is written "#rrggbb",
// or "#rrggbbaa" when it is not opaque; a length or a number as a
// number; insets as [top, right, bottom, left]; a spring as
// {"response": seconds, "damping": ratio}. Any other value is written
// when it is a string, a number or a bool, and left out otherwise, such
// as a font: that is no error.
func MarshalValues(th Theme) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{")
	first := true
	for _, key := range th.Keys() {
		raw, ok, err := marshalValue(th.values[key])
		if err != nil {
			return nil, &KeyError{Key: key, Err: err}
		}
		if !ok {
			continue
		}
		if !first {
			b.WriteString(",")
		}
		first = false
		k, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		b.WriteString("\n  ")
		b.Write(k)
		b.WriteString(": ")
		b.Write(raw)
	}
	if !first {
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

// marshalValue writes one value as JSON, and reports false for one JSON
// cannot hold.
func marshalValue(v any) (raw []byte, ok bool, err error) {
	switch v := v.(type) {
	case color.NRGBA:
		return []byte(strconv.Quote(Hex(v))), true, nil
	case float32:
		return []byte(formatNumber(v)), true, nil
	case geom.Insets:
		return fmt.Appendf(nil, "[%s, %s, %s, %s]", formatNumber(v.Top), formatNumber(v.Right), formatNumber(v.Bottom), formatNumber(v.Left)), true, nil
	case anim.Spring:
		return fmt.Appendf(nil, `{"response": %s, "damping": %s}`, formatNumber(v.Response), formatNumber(v.Damping)), true, nil
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, false, nil
	}
	switch k := rv.Kind(); {
	case k == reflect.String, k == reflect.Bool, isNumber(k):
		raw, err = json.Marshal(v)
		return raw, err == nil, err
	default:
		return nil, false, nil
	}
}

// formatNumber writes v as briefly as it reads back the same.
func formatNumber(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }

// Hex writes c as "#rrggbb", or "#rrggbbaa" when it is not opaque.
func Hex(c color.NRGBA) string {
	if c.A == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// ParseHex reads a colour written "#rgb", "#rrggbb" or "#rrggbbaa", with
// or without the "#", in either case.
func ParseHex(s string) (color.NRGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "ff"
	}
	if len(h) != 8 {
		return color.NRGBA{}, fmt.Errorf("%q is no colour: write it #rgb, #rrggbb or #rrggbbaa", s)
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("%q is no colour: write it #rgb, #rrggbb or #rrggbbaa", s)
	}
	return color.NRGBA{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, nil
}

// UnmarshalValues reads values written by [MarshalValues] and returns
// base with them set. Values that cannot be set, for a key no token has
// or in a form that does not suit the token, are left out, and the error
// lists each as a [KeyError]: the theme returned still holds every value
// that could be read. Data that is no JSON object gives base and an
// error.
//
// Insets may also be written as one number for all four sides, or as
// an object of "top", "right", "bottom" and "left", where a side left
// out is nought.
func UnmarshalValues(base Theme, data []byte) (Theme, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return base, fmt.Errorf("theme: reading values: %w", err)
	}
	if raw == nil {
		return base, errors.New("theme: reading values: not a JSON object")
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := base.With()
	var errs []error
	for _, key := range keys {
		info, ok := Lookup(key)
		if !ok {
			errs = append(errs, &KeyError{Key: key, Err: ErrUnknownKey})
			continue
		}
		v, err := unmarshalValue(info, raw[key])
		if err != nil {
			errs = append(errs, &KeyError{Key: key, Err: err})
			continue
		}
		out.values[key] = v
	}
	return out, errors.Join(errs...)
}

// unmarshalValue reads one value for the token info describes.
func unmarshalValue(info Info, raw json.RawMessage) (any, error) {
	switch info.Type {
	case reflect.TypeFor[color.NRGBA]():
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("a colour is written as a string such as \"#5e9cff\", not %s", raw)
		}
		return ParseHex(s)
	case reflect.TypeFor[float32]():
		var f float32
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("a %s is written as a number, not %s", info.Kind, raw)
		}
		return f, nil
	case reflect.TypeFor[geom.Insets]():
		return unmarshalInsets(raw)
	case reflect.TypeFor[anim.Spring]():
		var s struct {
			Response *float32 `json:"response"`
			Damping  *float32 `json:"damping"`
		}
		if err := json.Unmarshal(raw, &s); err != nil || s.Response == nil || s.Damping == nil {
			return nil, fmt.Errorf("a spring is written as {\"response\": seconds, \"damping\": ratio}, not %s", raw)
		}
		return anim.Spring{Response: *s.Response, Damping: *s.Damping}, nil
	}
	switch k := info.Type.Kind(); {
	case k == reflect.String, k == reflect.Bool, isNumber(k):
		p := reflect.New(info.Type)
		if err := json.Unmarshal(raw, p.Interface()); err != nil {
			return nil, fmt.Errorf("a %s does not read from %s", info.Type, raw)
		}
		return p.Elem().Interface(), nil
	default:
		return nil, fmt.Errorf("a %s cannot be read from a file", info.Type)
	}
}

// unmarshalInsets reads insets as [top, right, bottom, left], one
// number, or an object of sides.
func unmarshalInsets(raw json.RawMessage) (geom.Insets, error) {
	var all float32
	if json.Unmarshal(raw, &all) == nil {
		return geom.Uniform(all), nil
	}
	var four []float32
	if json.Unmarshal(raw, &four) == nil && len(four) == 4 {
		return geom.Insets{Top: four[0], Right: four[1], Bottom: four[2], Left: four[3]}, nil
	}
	var sides struct {
		Top, Right, Bottom, Left float32
	}
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) && json.Unmarshal(raw, &sides) == nil {
		return geom.Insets{Top: sides.Top, Right: sides.Right, Bottom: sides.Bottom, Left: sides.Left}, nil
	}
	return geom.Insets{}, fmt.Errorf("insets are written as [top, right, bottom, left], not %s", raw)
}
