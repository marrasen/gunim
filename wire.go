package gunim

import (
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
// Commands carry plain Go values. In one process they cross over a
// queue exactly as they are, with nothing encoded or copied, so a
// state holding an image hands the window the image itself. A socket
// transport turns them into bytes with [MarshalCommand] and back with
// [UnmarshalCommand]; [CheckWire] proves in a test that a value will
// make that trip.
//
// Sending a value hands it over. The window reads it on its own
// goroutine, possibly after Send has returned, so leave everything
// reachable from it unchanged from then on. Build fresh state for each
// send, and share large read-only data such as pixels by pointer.
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
	// State is the value the view renders, of the type its
	// [RegisterView] build function takes. Nil means that type's zero
	// value.
	State any
}

// Update hands fresh state to one mounted view.
//
// It is [Publish] to the topic named after the view's own ID, so one
// delivery path serves both.
type Update struct {
	ID    ID
	State any
}

// Publish hands fresh state to every view watching Key.
//
// One call reaches the list, the sidebar count and the status line
// together, and each animates the difference in its own way. This is
// the shape an aprot refresh trigger arrives in: the handler fires the
// trigger, the transport turns the pushed result into a Publish.
type Publish struct {
	Key   string
	State any
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
	// Data is the patch. Its type picks the [RegisterPatch] handler
	// that applies it.
	Data any
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
// Read it with [As].
type Envelope struct {
	// From is the mounted view that raised the intent.
	From ID
	// Intent is the value the widget sent, as it sent it.
	Intent Intent
}

// An Intent is what a widget reports when the user does something the
// application cares about.
//
// Go has no way to say "this type is plain data", so Intent is any
// value and [CheckWire] enforces the rule in a test. Keeping the
// application's reach down to plain data is the whole point: a value
// can cross a socket, and application logic stays off the UI goroutine
// where it would stall every animation in the window.
type Intent any

var (
	nameByType sync.Map // reflect.Type -> string
	typeByName sync.Map // string -> reflect.Type
)

// RegisterType names a type for the wire.
//
// In one process nothing needs a name, because values cross as they
// are. A socket carries a name alongside each state, patch and intent
// so the far end knows what to decode it into, so register every type
// your application sends either way, and let [CheckWire] catch the ones
// you missed. Call it from an init function in the package that
// declares the type.
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

// typeNamed returns the type registered under name.
func typeNamed(name string) (reflect.Type, bool) {
	t, ok := typeByName.Load(name)
	if !ok {
		return nil, false
	}
	rt, ok := t.(reflect.Type)
	return rt, ok
}

// As returns e's intent when it is a T.
//
//	if v, ok := gunim.As[DeleteJob](ev); ok { ... }
func As[T any](e Envelope) (T, bool) {
	v, ok := e.Intent.(T)
	return v, ok
}

// CheckWire reports the first value that would fail to cross a socket.
//
// Each value needs a [RegisterType] name and has to come back from its
// wire encoding equal to what went in. A [Command] or an [Envelope] is
// checked whole, along with the value it carries. Call it from a test
// over every command, state, patch and intent your application uses,
// so the compiler's silence about serializability turns into a failing
// build.
//
//	func TestWire(t *testing.T) {
//	    if err := gunim.CheckWire(DeleteJob{ID: "7"}, Confirmed{}); err != nil {
//	        t.Fatal(err)
//	    }
//	}
func CheckWire(values ...any) error {
	for _, v := range values {
		if err := checkWire(v); err != nil {
			return err
		}
	}
	return nil
}

func checkWire(v any) error {
	var (
		got any
		err error
	)
	switch v := v.(type) {
	case nil:
		return errors.New("gunim: CheckWire got a nil value, which has no type to send")
	case Command:
		var data []byte
		if data, err = MarshalCommand(v); err == nil {
			got, err = UnmarshalCommand(data)
		}
	case Envelope:
		var data []byte
		if data, err = MarshalEnvelope(v); err == nil {
			got, err = UnmarshalEnvelope(data)
		}
	default:
		var w wireValue
		if w, err = packValue(v); err == nil {
			got, err = w.unpack()
		}
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(v, got) {
		return fmt.Errorf("gunim: %T changed across the wire: %#v became %#v", v, v, got)
	}
	return nil
}
