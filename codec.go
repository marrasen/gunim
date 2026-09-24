package gunim

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// wireOptions are the encoding rules for everything crossing between a
// window and its application.
//
// OmitZeroStructFields is what keeps struct tags out of this package. A
// zero field is left out of the output and comes back zero, so a value
// survives the round trip with no annotation on it at all. The v1
// package needed `json:",omitempty"` on each field to manage the same
// thing, and even then a nil raw message returned as the four bytes
// "null".
//
// Deterministic fixes the order of map keys, so the same value always
// encodes to the same bytes. That makes a wire dump diffable and a
// golden test stable.
//
// Case-sensitive member matching is v2's default, which is the rule
// worth having: the Go field name is the wire name, exactly.
var wireOptions = json.JoinOptions(
	json.OmitZeroStructFields(true),
	json.Deterministic(true),
)

// encode turns a value into the bytes that cross the boundary.
func encode(v any) (jsontext.Value, error) {
	return json.Marshal(v, wireOptions)
}

// decode reads a value back. An empty input leaves out at its zero
// value, so a command carrying no state is the same as one carrying an
// empty one.
func decode(data jsontext.Value, out any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out, wireOptions)
}
