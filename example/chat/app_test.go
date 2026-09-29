package main

import (
	"context"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// harness runs the app half against the real views in an offscreen window, with no colleagues about.
type harness struct {
	t *testing.T
	w *gunim.Window
	a *app
	v *chatView
}

// grab is a patch that hands the test the chat view.
type grab struct{}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, w: gunimtest.New(t, geom.Sz(1100, 700), widget.NewSurface())}
	registerViews(h.w)
	gunim.RegisterPatch(h.w, "chat", func(v *chatView, _ grab, _ *gunim.UI) { h.v = v })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.a = newApp(ctx, h.w.Client(), 1, 200, 0)
	if err := h.w.Client().Mount(gunim.Root, "chat", "chat", h.a.state()); err != nil {
		t.Fatal(err)
	}
	if err := h.w.Client().Patch("chat", grab{}); err != nil {
		t.Fatal(err)
	}
	h.frames(30)
	return h
}

// frames draws n frames, handling the intents and the timers that come due meanwhile.
func (h *harness) frames(n int) {
	for range n {
		h.w.Frame(time.Second / 60)
		for more := true; more; {
			select {
			case ev := <-h.w.Client().Intents():
				h.a.handle(ev.Intent)
			case fn := <-h.a.later:
				fn()
			default:
				more = false
			}
		}
	}
}

// until draws frames until ok holds, for up to five seconds.
func (h *harness) until(what string, ok func() bool) {
	h.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			h.t.Fatalf("waited in vain until %s", what)
		}
		h.frames(1)
		time.Sleep(5 * time.Millisecond)
	}
}

// typeAndSend types s in the message box and presses Enter.
func (h *harness) typeAndSend(s string) {
	h.w.Input(input.TextInput{Text: s})
	h.frames(1)
	h.w.Input(input.KeyPress{Key: input.KeyEnter})
	h.frames(2)
}

// item returns the timeline's row for the user's message with body, if it shows.
func (h *harness) item(body string) (Item, bool) {
	for _, it := range h.v.items {
		if it.Mine && it.Body == body {
			return it, true
		}
	}
	return Item{}, false
}

func (h *harness) state(body string) State {
	it, ok := h.item(body)
	if !ok {
		h.t.Fatalf("no message %q in the timeline", body)
	}
	return it.State
}

func TestASentMessageShowsAtOnceAndArrives(t *testing.T) {
	h := newHarness(t)
	h.typeAndSend("Hello there")
	if s := h.state("Hello there"); s != Pending {
		t.Fatalf("state %v straight after sending, want pending", s)
	}
	if got := h.v.composer.Text(); got != "" {
		t.Fatalf("message box holds %q after sending, want it empty", got)
	}
	h.until("the message is sent", func() bool { return h.state("Hello there") == Sent })
	h.frames(60)
	if !h.v.list.AtEnd() {
		t.Fatal("the timeline left its end")
	}
}

func TestShiftEnterStartsALineInTheMessageBox(t *testing.T) {
	h := newHarness(t)
	h.w.Input(input.TextInput{Text: "one"})
	h.w.Input(input.KeyPress{Key: input.KeyEnter, Mods: input.ModShift})
	h.w.Input(input.TextInput{Text: "two"})
	h.frames(2)
	if got := h.v.composer.Text(); got != "one\ntwo" {
		t.Fatalf("message box holds %q, want two lines", got)
	}
}

func TestMessagesSentOfflineWaitForTheConnection(t *testing.T) {
	h := newHarness(t)
	h.a.handle(LinkToggled{})
	h.typeAndSend("Later")
	time.Sleep(1200 * time.Millisecond)
	h.frames(5)
	if s := h.state("Later"); s != Pending {
		t.Fatalf("state %v while offline, want pending", s)
	}
	h.a.handle(LinkToggled{})
	h.until("the message is sent", func() bool { return h.state("Later") == Sent })
}

func TestAFailedMessageGoesAgainOnAClick(t *testing.T) {
	h := newHarness(t)
	h.a.failRate = 1
	h.typeAndSend("Try me")
	h.until("the send fails", func() bool { return h.state("Try me") == Failed })
	h.a.failRate = 0
	it, _ := h.item("Try me")
	h.a.handle(RetryAsked{ID: it.ID})
	h.until("the message is sent", func() bool { return h.state("Try me") == Sent })
}

func TestAReplyQuotesItsMessage(t *testing.T) {
	h := newHarness(t)
	target := h.a.current.msgs[len(h.a.current.msgs)-3]
	h.a.handle(ReplyAsked{ID: target.ID})
	h.frames(2)
	h.typeAndSend("Replying")
	it, ok := h.item("Replying")
	if !ok || it.Reply.ID != target.ID || it.Reply.Text != target.Body {
		t.Fatalf("reply %+v, want a quote of %s", it.Reply, target.ID)
	}
}

func TestEditingReplacesTheText(t *testing.T) {
	h := newHarness(t)
	h.typeAndSend("Frist")
	it, _ := h.item("Frist")
	h.a.handle(EditAsked{ID: it.ID})
	h.frames(2)
	if got := h.v.composer.Text(); got != "Frist" {
		t.Fatalf("message box holds %q for the edit, want the message", got)
	}
	h.w.Input(input.KeyPress{Key: input.KeyA, Mods: input.ModControl})
	h.typeAndSend("First")
	it, ok := h.item("First")
	if !ok || !it.Edited || it.ID == "" {
		t.Fatalf("edited message %+v, want the new text marked edited", it)
	}
	if _, ok := h.item("Frist"); ok {
		t.Fatal("the old text still shows")
	}
}

func TestAnotherConversationOpensAtItsEnd(t *testing.T) {
	h := newHarness(t)
	before := h.v.list
	h.a.handle(ConversationChosen{ID: h.a.projects[0].convs[1].ID})
	h.frames(30)
	if h.v.list == before {
		t.Fatal("the timeline was not replaced")
	}
	if !h.v.list.AtEnd() {
		t.Fatal("the new timeline is not at its end")
	}
	if h.v.list.Offset() <= 0 {
		t.Fatalf("offset %v, want the timeline scrolled to its end", h.v.list.Offset())
	}
}

// height returns the laid-out height of the row for the user's message with body.
func (h *harness) height(body string) float32 {
	h.t.Helper()
	it, ok := h.item(body)
	if !ok {
		h.t.Fatalf("no message %q in the timeline", body)
	}
	n, ok := h.v.list.Row(widget.Key(it.Key))
	if !ok {
		h.t.Fatalf("message %q is not built", body)
	}
	return n.(*msgRow).height
}

func TestAMessageKeepsItsHeightAsItArrives(t *testing.T) {
	h := newHarness(t)
	h.typeAndSend("With a heading")
	h.typeAndSend("Sharing it")
	h.frames(5)
	if it, _ := h.item("Sharing it"); !it.Continued {
		t.Fatal("the second message has a heading of its own")
	}
	headed, shared := h.height("With a heading"), h.height("Sharing it")
	h.until("both are sent", func() bool { return h.state("With a heading") == Sent && h.state("Sharing it") == Sent })
	h.frames(5)
	if got := h.height("With a heading"); got != headed {
		t.Fatalf("the message with a heading went from %v to %v tall as it arrived", headed, got)
	}
	if got := h.height("Sharing it"); got != shared {
		t.Fatalf("the message sharing a heading went from %v to %v tall as it arrived", shared, got)
	}
}
