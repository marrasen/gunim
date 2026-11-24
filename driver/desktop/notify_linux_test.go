//go:build linux

package desktop

import (
	"context"
	"errors"
	"image"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

// privateBus starts a session bus of the test's own, with dbus-daemon, and points the session bus at it until the
// test ends. No service is there to start but those in the folder named, if any. It skips the test where dbus-daemon
// is missing.
func privateBus(t *testing.T, services ...string) string {
	t.Helper()
	addr, _ := startBus(t, filepath.Join(t.TempDir(), "bus"), services...)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	return addr
}

// startBus starts a bus at the socket sock, and returns its address and what stops it.
func startBus(t *testing.T, sock string, services ...string) (addr string, stop func()) {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("no dbus-daemon:", err)
	}
	addr = "unix:path=" + sock
	var conf strings.Builder
	conf.WriteString("<busconfig><type>session</type><listen>" + addr + "</listen>")
	for _, dir := range services {
		conf.WriteString("<servicedir>" + dir + "</servicedir>")
	}
	conf.WriteString(`<policy context="default"><allow send_destination="*" eavesdrop="true"/><allow own="*"/><allow eavesdrop="true"/></policy></busconfig>`)
	file := filepath.Join(t.TempDir(), "bus.conf")
	if err := os.WriteFile(file, []byte(conf.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), daemon, "--config-file="+file, "--nofork", "--nopidfile")
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop = func() {
		once.Do(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = os.Remove(sock)
		})
	}
	t.Cleanup(stop)
	until(t, "dbus-daemon to open its socket", func() bool {
		_, err := os.Stat(sock)
		return err == nil
	})
	return addr, stop
}

// until waits for ok, and fails t where it takes five seconds.
func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("waited in vain for " + what)
		}
	}
}

// fakeWatcher takes the tray's item, as a panel's watcher does.
type fakeWatcher struct{}

func (fakeWatcher) RegisterStatusNotifierItem(string) *dbus.Error { return nil }

// fakeNotifier is a desktop's notification server: it numbers each message and records what it was handed.
type fakeNotifier struct {
	mu    sync.Mutex
	next  uint32
	shown [][]any
	// caps are what GetCapabilities says after capsIn, and hold has Notify wait for it to close.
	caps   []string
	capsIn time.Duration
	hold   chan struct{}
}

// set runs fn with the fake held still.
func (f *fakeNotifier) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeNotifier) GetCapabilities() ([]string, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	time.Sleep(f.capsIn)
	return append([]string{}, f.caps...), nil
}

func (f *fakeNotifier) Notify(app string, replaces uint32, icon, summary, body string, actions []string,
	hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
	f.mu.Lock()
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.shown = append(f.shown, []any{app, replaces, icon, summary, body, actions, hints, timeout})
	return f.next, nil
}

// serve puts a watcher and a notification server on the bus at addr, on a connection of their own.
func serve(t *testing.T, addr string) (*dbus.Conn, *fakeNotifier) {
	t.Helper()
	return serveWith(t, addr, true)
}

// serveWith puts a notification server on the bus at addr, and a watcher where watcher is true, as a panel that
// shows icons has.
func serveWith(t *testing.T, addr string, watcher bool) (*dbus.Conn, *fakeNotifier) {
	t.Helper()
	conn := connect(t, addr)
	f := &fakeNotifier{}
	names := []string{notifyName}
	if watcher {
		if err := conn.Export(fakeWatcher{}, sniWatcherAt, sniWatcher); err != nil {
			t.Fatal(err)
		}
		names = append(names, sniWatcher)
	}
	if err := conn.Export(f, notifyPath, notifyName); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
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
		AppName:       "Test App",
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
		if args[0] != "Test App" {
			t.Fatalf("the message is from %q, want the tray's AppName", args[0])
		}
		if _, ok := args[6].(map[string]dbus.Variant)["image-data"]; !ok {
			t.Fatalf("the message's hints %v hold no image-data", args[6])
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
	// A message clicked leaves too, and that is no more news.
	if err := d.TrayNotifyTagged("title", "body", "three"); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(3), "default"); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 2)
	if err := server.Emit(notifyPath, notifyName+".NotificationClosed", uint32(3), uint32(2)); err != nil {
		t.Fatal(err)
	}
	h.quiet(t, "the message clicked left")
}

// sendDirect sends the signal member, from conn straight to the bus name dest, as any program on the bus can,
// past the match rules.
func sendDirect(t *testing.T, conn *dbus.Conn, dest, member string, body ...any) {
	t.Helper()
	msg := &dbus.Message{Type: dbus.TypeSignal, Headers: map[dbus.HeaderField]dbus.Variant{
		dbus.FieldPath:        dbus.MakeVariant(notifyPath),
		dbus.FieldInterface:   dbus.MakeVariant(notifyName),
		dbus.FieldMember:      dbus.MakeVariant(member),
		dbus.FieldDestination: dbus.MakeVariant(dest),
		dbus.FieldSignature:   dbus.MakeVariant(dbus.SignatureOf(body...)),
	}, Body: body}
	if call := conn.Send(msg, nil); call.Err != nil {
		t.Fatal(call.Err)
	}
}

// A click sent straight to the tray's connection by another program is dropped; one the server sends straight to
// it is heard.
func TestATrayDropsClicksSentStraightToItByAnotherProgram(t *testing.T) {
	addr := privateBus(t)
	server, _ := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	if err := d.TrayNotifyTagged("title", "body", "mine"); err != nil {
		t.Fatal(err)
	}
	notices.mu.Lock()
	dest := notices.n.conn.Names()[0]
	notices.mu.Unlock()
	sendDirect(t, connect(t, addr), dest, "ActionInvoked", uint32(1), "default")
	sendDirect(t, connect(t, addr), dest, "NotificationClosed", uint32(1), uint32(1))
	h.quiet(t, "signals another program sent straight to the tray")
	sendDirect(t, server, dest, "ActionInvoked", uint32(1), "default")
	if got := h.wait(t, 2); got[1] != "tag:mine" {
		t.Fatalf("the server's click heard %q, want [click tag:mine]", got)
	}
}

// Where no panel shows icons, SetTray fails, and the messages show and are heard all the same.
func TestMessagesShowWithNoPanelForTheIcon(t *testing.T) {
	addr := privateBus(t)
	server, f := serveWith(t, addr, false)
	d := &Driver{}
	h := newHeardLog()
	err := d.SetTray(driver.Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))},
		OnNotifyTag: func(tag string) { h.add("tag:" + tag) }})
	if !errors.Is(err, driver.ErrNoTray) {
		t.Fatalf("SetTray with no panel returned %v, want ErrNoTray", err)
	}
	t.Cleanup(func() { _ = d.SetTray(driver.Tray{}) })
	if w := d.TrayNotifyWay(); w != driver.NotifyExact {
		t.Fatalf("with no panel the way is %v, want exact", w)
	}
	if err := d.TrayNotifyTagged("title", "body", "alone"); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(t, 1); got[0] != "tag:alone" {
		t.Fatalf("a click heard %q, want [tag:alone]", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.shown) != 1 {
		t.Fatalf("the server was handed %d messages, want 1", len(f.shown))
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

// A message after the bus went and came back goes over a new connection.
func TestMessagesComeBackWithTheBus(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "bus")
	addr, stop := startBus(t, sock)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	if err := d.TrayNotifyTagged("title", "body", "before"); err != nil {
		t.Fatal(err)
	}
	old := theNotifier()
	stop()
	until(t, "the connection to close", func() bool { return !old.conn.Connected() })
	if w := d.TrayNotifyWay(); w != driver.NotifyNone {
		t.Fatalf("with the bus gone the way is %v, want none", w)
	}
	if err := d.TrayNotifyTagged("title", "body", "lost"); !errors.Is(err, driver.ErrNoTray) {
		t.Fatalf("with the bus gone a message returned %v, want ErrNoTray", err)
	}
	startBus(t, sock)
	server, _ := serve(t, addr)
	if w := d.TrayNotifyWay(); w != driver.NotifyExact {
		t.Fatalf("with the bus back the way is %v, want exact", w)
	}
	if err := d.TrayNotifyTagged("title", "body", "after"); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(t, 2); got[1] != "tag:after" {
		t.Fatalf("a click heard %q, want [click tag:after]", got)
	}
}

// muteBus points the session bus at a socket that takes connections and says nothing.
func muteBus(t *testing.T) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "mute")
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+sock)
}

func theNotifier() *notifier {
	notices.mu.Lock()
	defer notices.mu.Unlock()
	return notices.n
}

// TrayNotifyWay is none with no bus, one that does not answer, or no server on it, and exact with a server there or
// one the bus can start.
func TestTrayNotifyWayLooksForAServer(t *testing.T) {
	d := &Driver{}
	tray := driver.Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}}
	way := func(want driver.NotifyWay, when string) {
		t.Helper()
		setNoticeTray(tray)
		defer setNoticeTray(driver.Tray{})
		start := time.Now()
		if w := d.TrayNotifyWay(); w != want {
			t.Errorf("with %s the way is %v, want %v", when, w, want)
		}
		if took := time.Since(start); took > 2*notifyTimeout {
			t.Errorf("with %s the way took %v", when, took)
		}
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(t.TempDir(), "none"))
	way(driver.NotifyNone, "no bus")
	way(driver.NotifyNone, "no bus, asked again")

	muteBus(t)
	way(driver.NotifyNone, "a bus that does not answer")

	addr := privateBus(t)
	way(driver.NotifyNone, "no server on the bus")
	serve(t, addr)
	way(driver.NotifyExact, "a server on the bus")

	services := t.TempDir()
	if err := os.WriteFile(filepath.Join(services, "n.service"),
		[]byte("[D-BUS Service]\nName="+notifyName+"\nExec=/bin/false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	privateBus(t, services)
	way(driver.NotifyExact, "a server the bus can start")
}

// A bus that does not answer holds a message up for a moment, and fails it.
func TestAMessageFailsOnABusThatDoesNotAnswer(t *testing.T) {
	muteBus(t)
	setNoticeTray(driver.Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}})
	t.Cleanup(func() { setNoticeTray(driver.Tray{}) })
	start := time.Now()
	if err := (&Driver{}).TrayNotify("title", "body"); !errors.Is(err, driver.ErrNoTray) {
		t.Fatalf("a message returned %v, want ErrNoTray", err)
	}
	if took := time.Since(start); took > 2*notifyTimeout {
		t.Fatalf("a message took %v to fail", took)
	}
}

// A server slow to answer has the caller go on with ErrNotifyTimeout, and a click on the message is heard once its ID
// has come, though it was clicked before.
func TestAClickOnAMessageAnsweredLateIsHeard(t *testing.T) {
	addr := privateBus(t)
	server, f := serve(t, addr)
	hold := make(chan struct{})
	f.set(func() { f.hold = hold })
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	err := d.TrayNotifyTagged("title", "body", "late")
	if !errors.Is(err, driver.ErrNotifyTimeout) || errors.Is(err, driver.ErrNoTray) {
		t.Fatalf("a message not answered returned %v, want ErrNotifyTimeout", err)
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	h.quiet(t, "a click on a message with no ID yet")
	close(hold)
	if got := h.wait(t, 2); got[1] != "tag:late" {
		t.Fatalf("the click heard %q, want [click tag:late]", got)
	}
}

// A late answer keeps the tag of its message, and leaves the message shown last the last.
func TestALateAnswerLeavesTheNewestMessageTheLast(t *testing.T) {
	n := &notifier{tags: map[uint32]string{}, clicked: map[uint32]bool{}, inflight: 2, owner: ":1.1"}
	click := func(sender string) {
		n.heard(&dbus.Signal{Sender: sender, Path: notifyPath, Name: notifyName + ".ActionInvoked", Body: []any{uint32(99), "default"}})
	}
	click(":1.1")
	click(":1.9") // a forger's, dropped with nothing asked of the bus: there is none here to ask
	n.sent(2, 20, "new", nil)
	if len(n.early) != 1 {
		t.Fatalf("%d early signals are kept with a message in flight, want 1", len(n.early))
	}
	n.sent(1, 10, "late", nil)
	if n.last != 20 || n.tags[10] != "late" || n.tags[20] != "new" || n.inflight != 0 {
		t.Fatalf("the last is %d, the tags %v, with %d in flight", n.last, n.tags, n.inflight)
	}
	click(":1.1")
	if len(n.early) != 0 {
		t.Fatalf("%d early signals are kept with no message in flight, want none", len(n.early))
	}
}

// A click reported with no message in flight is dropped, and stays so for a message whose ID comes to be the same.
func TestAClickWithNoMessageInFlightIsDropped(t *testing.T) {
	addr := privateBus(t)
	server, f := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	if err := d.TrayNotifyTagged("title", "body", "first"); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(2), "default"); err != nil {
		t.Fatal(err)
	}
	// One of its own after it, so the first has been heard.
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 2)
	if err := d.TrayNotifyTagged("title", "body", "second"); err != nil {
		t.Fatal(err)
	}
	f.set(func() {
		if f.next != 2 {
			t.Fatalf("the second message is %d, want 2", f.next)
		}
	})
	h.quiet(t, "a message with the ID of an old click")
}

// The bus's answer to a question asked before it reported a new owner is dropped.
func TestAnOldAnswerLeavesTheNewOwner(t *testing.T) {
	n := &notifier{tags: map[uint32]string{}, clicked: map[uint32]bool{}}
	changed := func(to string) {
		n.heard(&dbus.Signal{Sender: busName, Name: busName + ".NameOwnerChanged", Body: []any{notifyName, "", to}})
	}
	gen := n.gen
	changed(":1.2")
	if got := n.keepOwner(":1.1", gen); got != ":1.2" {
		t.Fatalf("after an old answer the owner is %q, want :1.2", got)
	}
	if got := n.keepOwner(":1.3", n.gen); got != ":1.3" {
		t.Fatalf("after a new answer the owner is %q, want :1.3", got)
	}
	gen = n.gen
	changed("")
	changed("")
	if got := n.keepOwner(":1.3", gen); got != "" {
		t.Fatalf("after an old answer with the server gone the owner is %q, want none", got)
	}
}

// A flood of forged signals costs no goroutines, and the server's own click comes through it.
func TestForgedSignalsCostNoGoroutines(t *testing.T) {
	addr := privateBus(t)
	server, _ := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	if err := d.TrayNotifyTagged("title", "body", "mine"); err != nil {
		t.Fatal(err)
	}
	dest := theNotifier().conn.Names()[0]
	forger := connect(t, addr)
	before, most := runtime.NumGoroutine(), 0
	for range 5000 {
		sendDirect(t, forger, dest, "ActionInvoked", uint32(1), "default")
		most = max(most, runtime.NumGoroutine())
	}
	sendDirect(t, server, dest, "ActionInvoked", uint32(1), "default")
	if got := h.wait(t, 2); len(got) != 2 || got[1] != "tag:mine" {
		t.Fatalf("after a flood the server's click heard %q, want [click tag:mine]", got)
	}
	// sendDirect's own sends take a goroutine or two at a time.
	if most > before+50 {
		t.Fatalf("5000 forged signals took the goroutines from %d to %d", before, most)
	}
}

// Broken UTF-8 and NUL in a message are cleaned, and a body is escaped for a server that reads markup, and only for
// one.
func TestAMessageIsCleanedAndEscapedForItsServer(t *testing.T) {
	for _, c := range []struct {
		caps []string
		body string
	}{
		{[]string{"actions"}, "<b>a</b> & b\n\ufffd"},
		{[]string{"actions", "body-markup"}, "&lt;b&gt;a&lt;/b&gt; &amp; b\n\ufffd"},
	} {
		addr := privateBus(t)
		_, f := serve(t, addr)
		f.set(func() { f.caps = c.caps })
		d := &Driver{}
		if err := d.SetTray(driver.Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}, AppName: "A\x00pp\xff"}); err != nil {
			t.Fatal(err)
		}
		if err := d.TrayNotify("T & <i>\x00\xff", "<b>a</b> & b\n\x00\xff"); err != nil {
			t.Fatal(err)
		}
		_ = d.SetTray(driver.Tray{})
		f.mu.Lock()
		args := f.shown[0]
		f.mu.Unlock()
		if args[0] != "App\ufffd" || args[3] != "T & <i>\ufffd" || args[4] != c.body {
			t.Fatalf("with %v the server was handed app %q, title %q, body %q", c.caps, args[0], args[3], args[4])
		}
	}
}

// A server slow to say what it reads is handed an escaped body, as it may read markup.
func TestABodyIsEscapedForAServerSlowToSayWhatItReads(t *testing.T) {
	addr := privateBus(t)
	_, f := serve(t, addr)
	f.set(func() { f.capsIn = notifyTimeout/2 + 300*time.Millisecond })
	d := &Driver{}
	upTray(t, d, newHeardLog())
	if err := d.TrayNotify("title", "<b>x</b>"); err != nil {
		t.Fatal(err)
	}
	f.set(func() {
		if got := f.shown[0][4]; got != "&lt;b&gt;x&lt;/b&gt;" {
			t.Fatalf("the server was handed %q", got)
		}
	})
}

// A question to the bus that failed leaves the server and its messages known.
func TestAFailedQuestionLeavesTheOwner(t *testing.T) {
	addr := privateBus(t)
	server, _ := serve(t, addr)
	d := &Driver{}
	h := newHeardLog()
	upTray(t, d, h)
	if err := d.TrayNotifyTagged("title", "body", "mine"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if theNotifier().learnOwner(ctx) == "" {
		t.Fatal("a question that failed forgot the server")
	}
	if err := server.Emit(notifyPath, notifyName+".ActionInvoked", uint32(1), "default"); err != nil {
		t.Fatal(err)
	}
	if got := h.wait(t, 2); got[1] != "tag:mine" {
		t.Fatalf("the click heard %q, want [click tag:mine]", got)
	}
}

// A click kept for a message's ID counts as soon as the ID is known, so the message leaving then is no news.
func TestAnEarlyClickCountsOnceItsIDIsKnown(t *testing.T) {
	n := &notifier{tags: map[uint32]string{}, clicked: map[uint32]bool{}, inflight: 2, owner: ":1.1"}
	for _, action := range []string{"default", "other"} {
		id := uint32(7)
		if action == "other" {
			id = 8
		}
		n.heard(&dbus.Signal{Sender: ":1.1", Path: notifyPath, Name: notifyName + ".ActionInvoked", Body: []any{id, action}})
	}
	n.sent(1, 7, "a", nil)
	n.sent(2, 8, "b", nil)
	if !n.clicked[7] || n.clicked[8] {
		t.Fatalf("clicked is %v, want 7 alone", n.clicked)
	}
}
