//go:build linux

package desktop

import (
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

// privateBus starts a session bus of the test's own, with dbus-daemon, and points the session bus at it until the
// test ends. It skips the test where dbus-daemon is missing.
func privateBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("no dbus-daemon:", err)
	}
	sock := filepath.Join(t.TempDir(), "bus")
	addr := "unix:path=" + sock
	cmd := exec.CommandContext(t.Context(), daemon, "--session", "--nofork", "--nopidfile", "--address="+addr)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dbus-daemon did not open its socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	return addr
}

// fakeWatcher takes the tray's item, as a panel's watcher does.
type fakeWatcher struct{}

func (fakeWatcher) RegisterStatusNotifierItem(string) *dbus.Error { return nil }

// fakeNotifier is a desktop's notification server: it numbers each message and records what it was handed.
type fakeNotifier struct {
	mu    sync.Mutex
	next  uint32
	shown [][]any
}

func (f *fakeNotifier) Notify(app string, replaces uint32, icon, summary, body string, actions []string,
	hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.shown = append(f.shown, []any{app, replaces, icon, summary, body, actions, hints, timeout})
	return f.next, nil
}

// serve puts a watcher and a notification server on the bus at addr, on a connection of their own.
func serve(t *testing.T, addr string) (*dbus.Conn, *fakeNotifier) {
	t.Helper()
	conn := connect(t, addr)
	f := &fakeNotifier{}
	if err := conn.Export(fakeWatcher{}, sniWatcherAt, sniWatcher); err != nil {
		t.Fatal(err)
	}
	if err := conn.Export(f, notifyPath, notifyName); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{sniWatcher, notifyName} {
		if reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
			t.Fatalf("could not take the name %s: %v", name, err)
		}
	}
	return conn, f
}

func connect(t *testing.T, addr string) *dbus.Conn {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// heard records what a tray's message callbacks hear, in order.
type heardLog struct {
	mu   sync.Mutex
	got  []string
	more chan struct{}
}

func newHeardLog() *heardLog { return &heardLog{more: make(chan struct{}, 64)} }

func (h *heardLog) add(s string) {
	h.mu.Lock()
	h.got = append(h.got, s)
	h.mu.Unlock()
	h.more <- struct{}{}
}

// wait waits for n more entries, and returns all heard so far.
func (h *heardLog) wait(t *testing.T, n int) []string {
	t.Helper()
	for range n {
		select {
		case <-h.more:
		case <-time.After(2 * time.Second):
			t.Fatalf("heard %q, and nothing more", h.all())
		}
	}
	return h.all()
}

func (h *heardLog) all() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.got...)
}

// quiet fails t where anything more is heard within a moment.
func (h *heardLog) quiet(t *testing.T, after string) {
	t.Helper()
	select {
	case <-h.more:
		t.Fatalf("after %s the tray heard %q", after, h.all())
	case <-time.After(200 * time.Millisecond):
	}
}

// upTray puts a tray up whose message callbacks write to h, and takes it down as the test ends.
func upTray(t *testing.T, d *Driver, h *heardLog) {
	t.Helper()
	err := d.SetTray(driver.Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}, Tooltip: "test",
		OnNotifyClick: func() { h.add("click") },
		OnNotifyTag:   func(tag string) { h.add("tag:" + tag) },
		OnNotifyGone:  func() { h.add("gone") }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.SetTray(driver.Tray{}) })
}

// A click on any of several messages hears the tag that message was shown with, after OnNotifyClick, and the
// server sees the title and body alone.
func TestAClickOnAMessageHearsItsOwnTag(t *testing.T) {
	addr := privateBus(t)
	server, f := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	odd := "<b>&amp;</b>\x00\"'\n日本 🎉 " + strings.Repeat("x", driver.MaxNotifyTag-40)
	for _, tag := range []string{"chat:1", odd, ""} {
		if err := d.TrayNotifyTagged("title", "body", tag); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		id  uint32
		tag string
	}{{2, odd}, {1, "chat:1"}, {3, ""}} {
		before := len(h.all())
		if err := server.Emit(notifyPath, notifyName+".ActionInvoked", c.id, "default"); err != nil {
			t.Fatal(err)
		}
		got := h.wait(t, 2)[before:]
		if got[0] != "click" || got[1] != "tag:"+c.tag {
			t.Fatalf("a click on message %d heard %q, want [click tag:%q]", c.id, got, c.tag)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.shown) != 3 {
		t.Fatalf("the server was handed %d messages, want 3", len(f.shown))
	}
	for _, args := range f.shown {
		for _, a := range args {
			if s, ok := a.(string); ok && (s == odd || s == "chat:1") {
				t.Fatalf("the server was handed the tag %q in %v", s, args)
			}
		}
		if actions := args[5].([]string); len(actions) != 2 || actions[0] != "default" {
			t.Fatalf("the message's actions are %q, want the default one", actions)
		}
	}
}

// The tray hears only the clicks it should: on its own messages' default action, from the server.
func TestATrayHearsOnlyClicksOnItsOwnMessagesFromTheServer(t *testing.T) {
	addr := privateBus(t)
	server, _ := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	if err := d.TrayNotifyTagged("title", "body", "mine"); err != nil {
		t.Fatal(err)
	}
	// Another action than the default one.
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "reply"); err != nil {
		t.Fatal(err)
	}
	// A message of another program's.
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(99), "default"); err != nil {
		t.Fatal(err)
	}
	// Another program on the bus, playing the server.
	if err := connect(t, addr).Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	h.quiet(t, "clicks it should skip")
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(t, 2); len(got) != 2 || got[1] != "tag:mine" {
		t.Fatalf("the server's click heard %q, want [click tag:mine]", got)
	}
}

// The message shown last leaving runs OnNotifyGone, and an older one leaving does nothing.
func TestTheLastMessageLeavingRunsOnNotifyGone(t *testing.T) {
	addr := privateBus(t)
	server, _ := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	for _, tag := range []string{"one", "two"} {
		if err := d.TrayNotifyTagged("title", "body", tag); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Emit(notifyPath, notifyName+".NotificationClosed", uint32(1), uint32(1)); err != nil {
		t.Fatal(err)
	}
	h.quiet(t, "an older message left")
	if err := server.Emit(notifyPath, notifyName+".NotificationClosed", uint32(2), uint32(2)); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(t, 1); got[0] != "gone" {
		t.Fatalf("the last message leaving heard %q, want [gone]", got)
	}
}

// The tray keeps the last notifyKept messages' tags, and a click on an older one goes unheard.
func TestTheTrayKeepsTheLastMessagesTags(t *testing.T) {
	addr := privateBus(t)
	server, _ := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	for i := range notifyKept + 1 {
		if err := d.TrayNotifyTagged("title", "body", string(rune('A'+i%26))); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	h.quiet(t, "a click on a message past the kept ones")
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(2), "default"); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(t, 2); got[1] != "tag:B" {
		t.Fatalf("a click on the oldest message kept heard %q, want [click tag:B]", got)
	}
}

// A tag too long, or a message with no tray up, fails before the server sees it.
func TestAMessageFailsWithATagTooLongOrNoTray(t *testing.T) {
	addr := privateBus(t)
	_, f := serve(t, addr)
	d := &Driver{}
	if err := d.TrayNotifyTagged("title", "body", "tag"); !errors.Is(err, driver.ErrNoTray) {
		t.Fatalf("a message with no tray up returned %v, want ErrNoTray", err)
	}
	if w := d.TrayNotifyWay(); w != driver.NotifyNone {
		t.Fatalf("with no tray up the way is %v, want none", w)
	}
	upTray(t, d, newHeardLog())
	if w := d.TrayNotifyWay(); w != driver.NotifyExact {
		t.Fatalf("with a tray up the way is %v, want exact", w)
	}
	if err := d.TrayNotifyTagged("title", "body", strings.Repeat("x", driver.MaxNotifyTag+1)); !errors.Is(err, driver.ErrNotifyTag) {
		t.Fatalf("a tag of MaxNotifyTag+1 bytes returned %v, want ErrNotifyTag", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.shown) != 0 {
		t.Fatalf("the server was handed %d messages, want none", len(f.shown))
	}
}
