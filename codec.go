package gunim

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"reflect"
)

// wireOptions are the encoding rules for everything a socket transport
// carries between a window and its application.
//
// OmitZeroStructFields is what keeps struct tags out of this package. A
// zero field is left out of the output and comes back zero, so a value
// survives the round trip with no annotation on it at all.
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

// encode turns a value into bytes.
func encode(v any) (jsontext.Value, error) {
	return json.Marshal(v, wireOptions)
}

// decode reads a value back. An empty input leaves out at its zero
// value.
func decode(data jsontext.Value, out any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out, wireOptions)
}

// wireValue is a state, patch or intent on the wire: its registered
// name, so the far end knows what to decode it into, and its encoding.
type wireValue struct {
	Kind string
	Data jsontext.Value
}

func packValue(v any) (wireValue, error) {
	if v == nil {
		return wireValue{}, nil
	}
	kind, ok := TypeName(v)
	if !ok {
		return wireValue{}, fmt.Errorf("gunim: %T needs RegisterType to cross the wire", v)
	}
	data, err := encode(v)
	if err != nil {
		return wireValue{}, fmt.Errorf("gunim: encode %T: %w", v, err)
	}
	return wireValue{Kind: kind, Data: data}, nil
}

func (w wireValue) unpack() (any, error) {
	if w.Kind == "" {
		return nil, nil
	}
	t, ok := typeNamed(w.Kind)
	if !ok {
		return nil, fmt.Errorf("gunim: %q has no RegisterType in this process", w.Kind)
	}
	out := reflect.New(t)
	if err := decode(w.Data, out.Interface()); err != nil {
		return nil, fmt.Errorf("gunim: decode %s: %w", w.Kind, err)
	}
	return out.Elem().Interface(), nil
}

// wireCommand is every command in one shape, named by Command.
type wireCommand struct {
	Command string
	Parent  ID
	ID      ID
	View    string
	Watch   []string
	Key     string
	Value   wireValue
}

// MarshalCommand encodes a command for a socket transport.
//
// In one process [Client] hands the window the command as it is, so
// encoding is for a socket alone. The value a command carries needs
// [RegisterType], so the far end knows what to decode it into.
func MarshalCommand(c Command) ([]byte, error) {
	w := wireCommand{Command: c.Name()}
	var v any
	switch c := c.(type) {
	case Mount:
		w.Parent, w.ID, w.View, w.Watch, v = c.Parent, c.ID, c.View, c.Watch, c.State
	case Update:
		w.ID, v = c.ID, c.State
	case Publish:
		w.Key, v = c.Key, c.State
	case Patch:
		w.Key, v = c.Key, c.Data
	case Unmount:
		w.ID = c.ID
	case Focus:
		w.ID = c.ID
	default:
		return nil, fmt.Errorf("gunim: unknown command %T", c)
	}
	var err error
	if w.Value, err = packValue(v); err != nil {
		return nil, err
	}
	return encode(w)
}

// UnmarshalCommand decodes what [MarshalCommand] produced.
func UnmarshalCommand(data []byte) (Command, error) {
	var w wireCommand
	if err := decode(data, &w); err != nil {
		return nil, fmt.Errorf("gunim: decode command: %w", err)
	}
	v, err := w.Value.unpack()
	if err != nil {
		return nil, err
	}
	switch w.Command {
	case "mount":
		return Mount{Parent: w.Parent, ID: w.ID, View: w.View, Watch: w.Watch, State: v}, nil
	case "update":
		return Update{ID: w.ID, State: v}, nil
	case "publish":
		return Publish{Key: w.Key, State: v}, nil
	case "patch":
		return Patch{Key: w.Key, Data: v}, nil
	case "unmount":
		return Unmount{ID: w.ID}, nil
	case "focus":
		return Focus{ID: w.ID}, nil
	}
	return nil, fmt.Errorf("gunim: unknown command %q", w.Command)
}

// wireEnvelope is an envelope on the wire.
type wireEnvelope struct {
	From  ID
	Value wireValue
}

// MarshalEnvelope encodes an intent for a socket transport. The intent
// needs [RegisterType].
func MarshalEnvelope(e Envelope) ([]byte, error) {
	v, err := packValue(e.Intent)
	if err != nil {
		return nil, err
	}
	return encode(wireEnvelope{From: e.From, Value: v})
}

// UnmarshalEnvelope decodes what [MarshalEnvelope] produced.
func UnmarshalEnvelope(data []byte) (Envelope, error) {
	var w wireEnvelope
	if err := decode(data, &w); err != nil {
		return Envelope{}, fmt.Errorf("gunim: decode envelope: %w", err)
	}
	v, err := w.Value.unpack()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{From: w.From, Intent: v}, nil
}
