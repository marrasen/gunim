package gunim

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// An ID names a mounted view so that a [Command] can address it and an
// [Envelope] can say where it came from. The application picks these,
// and they stay stable for as long as the view is mounted.
type ID string

// Root is the ID of the window's root node, and the parent to mount a
// top-level view under.
const Root ID = "root"

// A Command travels from the application into a window.
//
// Every Command is a value that survives encoding/json, which is what
// keeps the application and the window separable: the same command
// works over a channel in one process and over a socket between two.
type Command interface {
	isCommand()
	// Name identifies the command on the wire.
	Name() string
}

// Mount builds the named view and inserts it under Parent, where it
// animates in.
//
// Mounting an ID whose view is still animating out brings it back. With
// the same View, the node reverses from wherever its exit had got to
// and takes the new Parent, Watch and State. With a different View, the
// old node finishes its exit without the ID and a new one is built.
//
// The view's update function runs straight after the build with the
// same state, so a view can build an empty shell and fill it in one
// place.
type Mount struct {
	Parent ID
	ID     ID
	View   string
	// Watch names the topics this view follows. A [Publish] or [Patch]
	// on any of them reaches this view.
	//
	// Every view also watches a topic named after its own ID, which is
	// what [Update] addresses.
	Watch []string
	State jsontext.Value
}

// Update hands fresh state to one mounted view.
//
// It is [Publish] to the topic named after the view's own ID, so one
// delivery path serves both.
type Update struct {
	ID    ID
	State jsontext.Value
}

// Publish hands fresh state to every view watching Key.
//
// One call reaches the list, the sidebar count and the status line
// together, and each animates the difference in its own way. This is
// the shape an aprot refresh trigger arrives in: the handler fires the
// trigger, the transport turns the pushed result into a Publish.
type Publish struct {
	Key   string
	State jsontext.Value
}

// Patch hands a typed partial change to every view watching Key.
//
// Reach for it when a value changed and the shape stayed put: a rating,
// a progress bar, a row's status. A patch lands as a spring retargeting
// mid-flight, where a whole [Publish] would reconcile the list. It is
// the counterpart of aprot's PatchSubscription, and the reason to tell
// the two apart is the same reason aprot does: one animates a value,
// the other animates structure.
type Patch struct {
	Key string
	// Kind names the patch type, registered with [RegisterType].
	Kind string
	Data jsontext.Value
}

// Unmount starts a view's exit.
//
// The node stays in the tree, animating out, and leaves when its
// [Transitioner] says it is done. The application carries on the moment
// the command is queued.
type Unmount struct {
	ID ID
}

// Focus moves keyboard focus to a view. An empty ID drops focus.
type Focus struct {
	ID ID
}

func (Mount) isCommand()   {}
func (Update) isCommand()  {}
func (Publish) isCommand() {}
func (Patch) isCommand()   {}
func (Unmount) isCommand() {}
func (Focus) isCommand()   {}

// Name identifies the command on the wire.
func (Mount) Name() string { return "mount" }

// Name identifies the command on the wire.
func (Update) Name() string { return "update" }

// Name identifies the command on the wire.
func (Publish) Name() string { return "publish" }

// Name identifies the command on the wire.
func (Patch) Name() string { return "patch" }

// Name identifies the command on the wire.
func (Unmount) Name() string { return "unmount" }

// Name identifies the command on the wire.
func (Focus) Name() string { return "focus" }

// An Envelope carries one intent from a window to the application.
//
// Kind names the intent type, registered with [RegisterType] in both
// processes, and Data holds it encoded. Read it with [As].
type Envelope struct {
	// From is the mounted view that raised the intent.
	From ID
	// Kind is the registered name of the intent type.
	Kind string
	// Data is the intent, encoded.
	Data jsontext.Value
}

// An Intent is what a widget reports when the user does something the
// application cares about.
//
// Go has no way to say "this type survives a round trip", so Intent is
// any value and [CheckWire] enforces the rule in a test. Keeping the
// application's reach down to plain data is the whole point: a value
// crosses a socket, and application logic stays off the UI goroutine
// where it would stall every animation in the window.
type Intent any

var (
	nameByType sync.Map // reflect.Type -> string
	typeByName sync.Map // string -> reflect.Type
)

// RegisterType names a type for the wire.
//
// Intents travelling out and patches travelling in both need one, so
// that the two ends agree on what a name decodes to. Call it from an
// init function in the package that declares the type.
//
//	func init() { gunim.RegisterType[DeleteJob]("job.delete") }
func RegisterType[T any](name string) {
	t := reflect.TypeFor[T]()
	if prev, loaded := nameByType.LoadOrStore(t, name); loaded && prev != name {
		panic(fmt.Sprintf("gunim: %s is already registered as %q", t, prev))
	}
	if prev, loaded := typeByName.LoadOrStore(name, t); loaded && prev != any(t) {
		panic(fmt.Sprintf("gunim: %q is already registered for %s", name, prev))
	}
}

// TypeName returns the registered name of v's type.
func TypeName(v any) (string, bool) {
	name, ok := nameByType.Load(reflect.TypeOf(v))
	if !ok {
		return "", false
	}
	s, _ := name.(string)
	return s, true
}

// As decodes e into T when e carries that type.
//
//	if v, ok := gunim.As[DeleteJob](ev); ok { ... }
func As[T any](e Envelope) (T, bool) {
	var v T
	name, ok := nameByType.Load(reflect.TypeFor[T]())
	if !ok || name != e.Kind {
		return v, false
	}
	if err := decode(e.Data, &v); err != nil {
		return v, false
	}
	return v, true
}

// envelope packs an intent for the wire.
func envelope(from ID, v Intent) (Envelope, error) {
	name, ok := TypeName(v)
	if !ok {
		return Envelope{}, fmt.Errorf("gunim: %T needs RegisterType", v)
	}
	data, err := encode(v)
	if err != nil {
		return Envelope{}, fmt.Errorf("gunim: encode %T: %w", v, err)
	}
	return Envelope{From: from, Kind: name, Data: data}, nil
}

// CheckWire reports the first value that fails to survive a round trip
// through JSON. Call it from a test over every command, intent and
// patch your application uses, so the compiler's silence about
// serializability turns into a failing build.
//
//	func TestWire(t *testing.T) {
//	    if err := gunim.CheckWire(DeleteJob{ID: "7"}, Confirmed{}); err != nil {
//	        t.Fatal(err)
//	    }
//	}
func CheckWire(values ...any) error {
	for _, v := range values {
		if v == nil {
			return errors.New("gunim: CheckWire got a nil value, which has no type to round-trip")
		}
		data, err := encode(v)
		if err != nil {
			return fmt.Errorf("gunim: %T fails to encode: %w", v, err)
		}
		out := reflect.New(reflect.TypeOf(v))
		if err := decode(data, out.Interface()); err != nil {
			return fmt.Errorf("gunim: %T fails to decode: %w", v, err)
		}
		if got := out.Elem().Interface(); !reflect.DeepEqual(v, got) {
			return fmt.Errorf("gunim: %T changed across a round trip: %#v became %#v", v, v, got)
		}
	}
	return nil
}
