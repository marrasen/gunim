package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// ping is a sample application intent.
type ping struct {
	N int
}

// tick is a sample patch: a value moved, the shape stayed put.
type tick struct {
	At float32
}

// panelState is a sample view state.
type panelState struct {
	Label string
}

func init() {
	RegisterType[ping]("test.ping")
	RegisterType[tick]("test.tick")
	RegisterType[panelState]("test.panel")
}

// probe is a view root that fades in and out and reports clicks.
type probe struct {
	anim.Group
	in    *anim.Float
	at    *anim.Float
	label string
	hits  int
}

func newProbe(label string) *probe {
	p := &probe{in: anim.NewFloat(0), at: anim.NewFloat(0), label: label}
	p.Add(p.in, p.at)
	return p
}

func (p *probe) Transition(s Presence, _ Frame) bool {
	switch s {
	case Entering:
		p.in.Animate(1, anim.Snappy)
	case Exiting:
		p.in.Animate(0, anim.Gentle)
	case Present:
		// Settled.
	}
	return !p.in.Active()
}

func (p *probe) Handle(e input.Event, u *UI) bool {
	if _, ok := e.(input.PointerDown); ok {
		p.hits++
		u.Send(p, ping{N: p.hits})
		return true
	}
	return false
}

func (p *probe) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (p *probe) Paint(pt *paint.Painter, _ Frame, box geom.Size, _ Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 8, paint.Fill{})
}

// offscreen returns the driver behind a test window.
func (w *Window) mustOffscreen(t *testing.T) *driver.OffscreenWindow {
	t.Helper()
	d, ok := w.dw.(*driver.OffscreenWindow)
	if !ok {
		t.Fatalf("window is backed by a %T, want an offscreen driver", w.dw)
	}
	return d
}

// probeOf returns the probe behind a node.
func probeOf(t *testing.T, n Node) *probe {
	t.Helper()
	p, ok := n.(*probe)
	if !ok {
		t.Fatalf("node is a %T, want a *probe", n)
	}
	return p
}

// take reads one intent, or fails.
func take(t *testing.T, c Client) Envelope {
	t.Helper()
	select {
	case ev := <-c.Intents():
		return ev
	default:
		t.Fatal("expected an intent, found none")
		return Envelope{}
	}
}

func newProbeWindow(t *testing.T) *Window {
	t.Helper()
	w := newTestWindow()
	RegisterView(w, "panel",
		func(s panelState) *probe { return newProbe(s.Label) },
		func(p *probe, s panelState, _ *UI) { p.label = s.Label })
	RegisterPatch(w, "panel", func(p *probe, v tick, _ *UI) {
		p.at.Animate(v.At, anim.Snappy)
	})
	return w
}

func TestCommandsAndIntentsSurviveTheWire(t *testing.T) {
	// The compiler stays quiet about serializability, so the rule is
	// enforced here. An application does the same over its own types.
	values := []any{
		Mount{Parent: Root, ID: "panel", View: "panel", Watch: []string{"topic"}, State: panelState{Label: "hi"}},
		Update{ID: "panel", State: panelState{Label: "bye"}},
		Publish{Key: "topic", State: panelState{Label: "all"}},
		Patch{Key: "topic", Data: tick{At: 0.5}},
		Unmount{ID: "panel"},
		Focus{ID: "panel"},
		SetTheme{Theme: "light"},
		Envelope{From: "panel", Intent: ping{N: 1}},
		ping{N: 3},
		tick{At: 0.25},
		CommandFailed{Command: "mount", ID: "panel", Reason: "view missing"},

		// Zero fields are left out of the encoding and come back zero,
		// with no struct tag in sight.
		Mount{Parent: Root, ID: "bare", View: "panel"},
		Envelope{From: "bare"},
		Publish{Key: "empty"},
		CommandFailed{},
	}
	if err := CheckWire(values...); err != nil {
		t.Fatal(err)
	}
}

func TestCheckWireCatchesAnUnregisteredType(t *testing.T) {
	type unnamed struct{ N int }
	if err := CheckWire(unnamed{N: 1}); err == nil {
		t.Fatal("CheckWire passed a type with no RegisterType")
	}
	if err := CheckWire(Publish{Key: "k", State: unnamed{N: 1}}); err == nil {
		t.Fatal("CheckWire passed a command carrying a type with no RegisterType")
	}
}

func TestWireFormatUsesGoFieldNames(t *testing.T) {
	// The Go field name is the wire name, and a zero field is absent.
	got, err := MarshalCommand(Mount{Parent: Root, ID: "jobs", View: "joblist", Watch: []string{"jobs"}, State: panelState{Label: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"Command":"mount","Parent":"root","ID":"jobs","View":"joblist","Watch":["jobs"],"Value":{"Kind":"test.panel","Data":{"Label":"x"}}}`
	if string(got) != want {
		t.Fatalf("encoded as\n  %s\nwant\n  %s", got, want)
	}
}

// frameState holds a large buffer, the way an image would.
type frameState struct {
	Pixels []byte
}

func TestStateCrossesByReference(t *testing.T) {
	// In one process nothing is encoded, so a view sees the very bytes
	// the application sent.
	w := newTestWindow()
	var seen []byte
	RegisterView(w, "image",
		func(frameState) *Box { return &Box{} },
		func(_ *Box, s frameState, _ *UI) { seen = s.Pixels })
	pixels := make([]byte, 4<<20)
	if err := w.Client().Mount(Root, "image", "image", frameState{Pixels: pixels}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	if len(seen) != len(pixels) || &seen[0] != &pixels[0] {
		t.Fatal("the view got a copy of the pixels rather than the pixels")
	}
}

func TestStateOfTheWrongTypeComesBackAsAFailure(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", tick{At: 1}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	v, ok := As[CommandFailed](take(t, c))
	if !ok || v.Command != "mount" {
		t.Fatal("a mount with the wrong state type should fail")
	}
}

func TestMountBuildsAndAnimatesIn(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()

	if err := c.Mount(Root, "panel", "panel", panelState{Label: "hello"}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)

	s, ok := w.ui.ids["panel"]
	if !ok {
		t.Fatal("mounted view is missing from the id index")
	}
	if got := probeOf(t, s.node).label; got != "hello" {
		t.Fatalf("label = %q, want %q", got, "hello")
	}
	if got := w.ui.Presence(s.node); got != Present {
		t.Fatalf("Presence = %v, want present", got)
	}
}

func TestUpdateIsPublishToTheViewsOwnTopic(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{Label: "hello"}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)

	if err := c.Update("panel", panelState{Label: "goodbye"}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	if got := probeOf(t, w.ui.ids["panel"].node).label; got != "goodbye" {
		t.Fatalf("label = %q, want %q", got, "goodbye")
	}
}

func TestOnePublishReachesEveryWatcher(t *testing.T) {
	// This is the property aprot's refresh triggers buy: one call, and
	// every view showing that data updates.
	w := newProbeWindow(t)
	c := w.Client()
	for _, id := range []ID{"a", "b", "c"} {
		if err := c.Mount(Root, id, "panel", panelState{Label: "start"}, "jobs"); err != nil {
			t.Fatal(err)
		}
	}
	run(w, 60)

	if err := c.Publish("jobs", panelState{Label: "fresh"}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)

	for _, id := range []ID{"a", "b", "c"} {
		if got := probeOf(t, w.ui.ids[id].node).label; got != "fresh" {
			t.Fatalf("view %q label = %q, want %q", id, got, "fresh")
		}
	}
}

func TestPatchRetargetsWithoutReplacingState(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{Label: "hello"}, "jobs"); err != nil {
		t.Fatal(err)
	}
	run(w, 60)

	if err := c.Patch("jobs", tick{At: 1}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)

	p := probeOf(t, w.ui.ids["panel"].node)
	if !p.at.Active() {
		t.Fatal("patch left the value at rest; it should have started a spring")
	}
	if p.label != "hello" {
		t.Fatalf("label = %q, want the patch to leave the rest of the state alone", p.label)
	}
	run(w, 120)
	if got := p.at.Value(); got != 1 {
		t.Fatalf("at = %v, want 1 once the spring settles", got)
	}
}

func TestUnwatchedPublishComesBackAsAFailure(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Publish("nobody", panelState{}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)

	v, ok := As[CommandFailed](take(t, c))
	if !ok {
		t.Fatal("expected a CommandFailed")
	}
	if v.Key != "nobody" {
		t.Fatalf("CommandFailed.Key = %q, want the topic that had no watchers", v.Key)
	}
}

func TestUnmountDropsTheSubscription(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{}, "jobs"); err != nil {
		t.Fatal(err)
	}
	run(w, 60)
	if err := c.Unmount("panel"); err != nil {
		t.Fatal(err)
	}
	run(w, 200)

	if len(w.ui.topics["jobs"]) != 0 {
		t.Fatal("topic kept a subscriber for an unmounted view")
	}
}

func TestClickTravelsBackAsAnIntent(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{Label: "hello"}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)

	w.ui.handlePlatform(input.PointerDown{Pos: geom.Pt(10, 10), Time: time.Now()})

	ev := take(t, c)
	if ev.From != "panel" {
		t.Fatalf("From = %q, want the mounted view's id", ev.From)
	}
	v, ok := As[ping](ev)
	if !ok {
		t.Fatalf("As[ping] failed on %T", ev.Intent)
	}
	if v.N != 1 {
		t.Fatalf("N = %d, want 1", v.N)
	}
}

func TestUnmountLetsTheViewAnimateOut(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{Label: "hello"}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)
	node := w.ui.ids["panel"].node

	// The application says go away and carries on. The view keeps its
	// place in the tree for as long as its exit takes.
	if err := c.Unmount("panel"); err != nil {
		t.Fatal(err)
	}
	run(w, 3)

	if _, still := w.ui.index[node]; !still {
		t.Fatal("view left the tree before its exit animation ran")
	}
	if v := probeOf(t, node).in.Value(); v <= 0 || v >= 1 {
		t.Fatalf("mid-exit value = %v, want something between 0 and 1", v)
	}

	run(w, 120)
	if _, still := w.ui.index[node]; still {
		t.Fatal("view stayed in the tree after its exit settled")
	}
	if _, still := w.ui.ids["panel"]; still {
		t.Fatal("id index kept an entry for an unmounted view")
	}
}

func TestFailedCommandTravelsBack(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "nosuchview", panelState{}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)

	v, ok := As[CommandFailed](take(t, c))
	if !ok {
		t.Fatal("expected a CommandFailed")
	}
	if v.Command != "mount" || v.ID != "panel" {
		t.Fatalf("CommandFailed = %+v, want a failed mount of panel", v)
	}
}

func TestCompositeArrivesWhole(t *testing.T) {
	w := newTestWindow()
	parent := newComposite()
	w.ui.Insert(w.ui.Root(), parent)
	for _, kid := range parent.Children() {
		if _, ok := w.ui.index[kid]; !ok {
			t.Fatal("a composite's child was left out of the tree")
		}
	}
}

// composite is a node that owns two children.
type composite struct {
	kids []Node
}

func newComposite() *composite {
	return &composite{kids: []Node{newProbe("a"), newProbe("b")}}
}

func (c *composite) Children() []Node { return c.kids }
func (c *composite) Layout(cs Constraints, _ Frame, kids Children) geom.Size {
	for kid := range kids.All {
		kid.Layout(cs)
		kid.Place(geom.Point{})
	}
	return cs.Max
}
func (c *composite) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}

func TestMountingALeavingViewBringsItBack(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{Label: "first"}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)
	node := w.ui.ids["panel"].node

	if err := c.Unmount("panel"); err != nil {
		t.Fatal(err)
	}
	run(w, 6)
	mid := probeOf(t, node).in.Value()

	if err := c.Mount(Root, "panel", "panel", panelState{Label: "again"}, "extra"); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	select {
	case ev := <-c.Intents():
		t.Fatalf("remount failed: %+v", ev)
	default:
	}
	if w.ui.ids["panel"].node != node {
		t.Fatal("remount built a new node instead of reversing the old one")
	}
	p := probeOf(t, node)
	if p.label != "again" {
		t.Fatalf("label = %q, want the remount's state", p.label)
	}
	// The spring keeps its velocity, so the fade may dip a little
	// further before it turns, but it carries on from part way.
	if v := p.in.Value(); v <= 0 || v >= 1 {
		t.Fatalf("in = %v, want it to carry on from about %v", v, mid)
	}
	if w.ui.Presence(node) != Entering {
		t.Fatalf("Presence = %v, want entering", w.ui.Presence(node))
	}

	// The new watch list replaced the old one.
	if err := c.Publish("extra", panelState{Label: "published"}); err != nil {
		t.Fatal(err)
	}
	run(w, 120)
	if p.label != "published" {
		t.Fatalf("label = %q, want the publish on the new topic", p.label)
	}
	if !inTree(w.ui, node) {
		t.Fatal("revived view was reaped anyway")
	}
}

func TestMountingADifferentViewOnALeavingIDReplacesIt(t *testing.T) {
	w := newProbeWindow(t)
	RegisterView(w, "box", func(struct{}) *Box { return &Box{} }, nil)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)
	old := w.ui.ids["panel"].node

	if err := c.Unmount("panel"); err != nil {
		t.Fatal(err)
	}
	if err := c.Mount(Root, "panel", "box", nil); err != nil {
		t.Fatal(err)
	}
	run(w, 3)

	if _, ok := w.ui.ids["panel"].node.(*Box); !ok {
		t.Fatal("the new view did not take the id")
	}
	if !inTree(w.ui, old) {
		t.Fatal("the old view was cut off instead of finishing its exit")
	}

	// Reaping the old view must leave the new one's id alone.
	run(w, 120)
	if inTree(w.ui, old) {
		t.Fatal("the old view never left")
	}
	if _, ok := w.ui.ids["panel"]; !ok {
		t.Fatal("reaping the old view dropped the new view's id")
	}
}

func TestMountingAPresentIDStillFails(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	for range 2 {
		if err := c.Mount(Root, "panel", "panel", panelState{}); err != nil {
			t.Fatal(err)
		}
	}
	run(w, 1)
	if v, ok := As[CommandFailed](take(t, c)); !ok || v.Command != "mount" {
		t.Fatal("a second mount of a present id should fail")
	}
}

func TestFocusOnALeavingViewFails(t *testing.T) {
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{}); err != nil {
		t.Fatal(err)
	}
	run(w, 60)
	if err := c.Unmount("panel"); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus("panel"); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	if v, ok := As[CommandFailed](take(t, c)); !ok || v.Command != "focus" {
		t.Fatal("focusing a leaving view should come back as a failure")
	}
}

func TestCheckWireRejectsNil(t *testing.T) {
	if err := CheckWire(nil); err == nil {
		t.Fatal("CheckWire(nil) = nil, want an error")
	}
}

func TestSetThemeAnimatesEveryReader(t *testing.T) {
	pad := theme.Length("test.wire.pad", 8)
	w := newTestWindow()
	w.RegisterTheme(theme.Make("roomy", theme.Set(pad, 20)))
	if got := pad.Get(w.ui.Theme()); got != 8 {
		t.Fatalf("setup: pad = %v, want 8", got)
	}
	run(w, 1)
	if w.ui.needsFrame() {
		t.Fatal("setup: the window should be idle")
	}

	if err := w.Client().SetTheme("roomy"); err != nil {
		t.Fatal(err)
	}
	run(w, 3)
	if got := pad.Get(w.ui.Theme()); got <= 8 || got >= 20 {
		t.Fatalf("three frames into the switch, pad = %v, want between 8 and 20", got)
	}
	if !w.ui.needsFrame() {
		t.Fatal("the window went idle in the middle of a theme switch")
	}
	run(w, 300)
	if got := pad.Get(w.ui.Theme()); got != 20 {
		t.Fatalf("once settled, pad = %v, want 20", got)
	}
}

func TestSetThemeWithAnUnknownNameComesBackAsAFailure(t *testing.T) {
	w := newTestWindow()
	c := w.Client()
	if err := c.SetTheme("nope"); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	v, ok := As[CommandFailed](take(t, c))
	if !ok || v.Command != "set-theme" || v.Key != "nope" {
		t.Fatalf("got %+v, want a failed set-theme naming nope", v)
	}
}
