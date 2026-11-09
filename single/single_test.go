package single

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startOne makes this the one running for dir, taking each handover with
// take, and returns what it took.
func startOne(t *testing.T, dir string, take bool) (took <-chan Handover, stop func()) {
	t.Helper()
	in, stop, err := Listen(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan Handover, 8)
	go func() {
		for h := range in {
			h.Take(take)
			got <- h
		}
	}()
	t.Cleanup(stop)
	return got, stop
}

// A copy started while one runs hands its command line over, and the
// one running hears it.
func TestACommandLineIsHandedOver(t *testing.T) {
	dir := t.TempDir()
	if ok, err := Hand(dir, Handover{Args: []string{"-ssh", "x"}}); ok || err != nil {
		t.Fatalf("with none running, the handover was taken %v, %v", ok, err)
	}
	got, _ := startOne(t, dir, true)
	ok, err := Hand(dir, Handover{Args: []string{"-ssh", "x"}, Dir: "/srv"})
	if !ok || err != nil {
		t.Fatalf("the handover was taken %v, %v", ok, err)
	}
	select {
	case h := <-got:
		if !slices.Equal(h.Args, []string{"-ssh", "x"}) || h.Dir != "/srv" {
			t.Fatalf("the one running heard %+v", h)
		}
	case <-time.After(time.Second):
		t.Fatal("the one running heard nothing")
	}
	// Windows reports every writable file as rw-rw-rw-: there the file
	// is private by being in the user's own profile.
	if fi, err := os.Stat(filepath.Join(dir, File)); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("the file is %v, %v", fi.Mode(), err)
	}
}

// A handover the one running does not take, as on its way out, is not
// said to be taken: the copy handing it over runs as the one.
func TestAHandoverNotTakenIsSaidSo(t *testing.T) {
	dir := t.TempDir()
	startOne(t, dir, false)
	if ok, _ := Hand(dir, Handover{Args: []string{"-e", "x"}}); ok {
		t.Fatal("a handover not taken was said to be")
	}
}

// A handover without the token is not heard.
func TestAHandoverWithoutTheTokenIsRefused(t *testing.T) {
	dir := t.TempDir()
	got, _ := startOne(t, dir, true)
	raw, _ := os.ReadFile(filepath.Join(dir, File))
	var r running
	_ = json.Unmarshal(raw, &r)
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", r.Addr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte(`{"token":"wrong","args":["-e","rm"]}` + "\n"))
	buf := make([]byte, 8)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	n, _ := conn.Read(buf)
	_ = conn.Close()
	if n != 0 {
		t.Fatalf("a wrong token was answered %q", buf[:n])
	}
	select {
	case h := <-got:
		t.Fatalf("a wrong token was heard: %+v", h)
	case <-time.After(100 * time.Millisecond):
	}
}

// The file is gone by the time stop returns, and a copy starting then
// runs as the one.
func TestTheFileGoesWithTheOneRunning(t *testing.T) {
	dir := t.TempDir()
	_, stop := startOne(t, dir, true)
	stop()
	if _, err := os.Stat(filepath.Join(dir, File)); !os.IsNotExist(err) {
		t.Fatalf("stopped, the file is still there: %v", err)
	}
	if ok, _ := Hand(dir, Handover{}); ok {
		t.Fatal("a handover was taken with none running")
	}
}

// A file left by a copy that ended is not believed, whatever listens
// on its port now.
func TestAFileLeftBehindIsNotBelieved(t *testing.T) {
	dir := t.TempDir()
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("ok\n"))
			_ = c.Close()
		}
	}()
	raw, _ := json.Marshal(running{Addr: l.Addr().String(), Token: "x", PID: 1 << 30})
	if err := os.WriteFile(filepath.Join(dir, File), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, _ := Hand(dir, Handover{Args: []string{"-e", "secret"}}); ok {
		t.Fatal("a handover went to whatever holds the port of a copy that ended")
	}
}

// dialOne connects to the one running for dir, as any local program
// can, without the token.
func dialOne(t *testing.T, dir string) net.Conn {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		t.Fatal(err)
	}
	var r running
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", r.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// flood writes bytes with no newline to conn until the one running
// drops it, or until the time given.
func flood(conn net.Conn, until time.Time) {
	chunk := bytes.Repeat([]byte("a"), 64<<10)
	_ = conn.SetWriteDeadline(until)
	for {
		if _, err := conn.Write(chunk); err != nil {
			return
		}
	}
}

// dropped waits up to within for the one running to close conn, and
// reports whether it did.
func dropped(conn net.Conn, within time.Duration) bool {
	_ = conn.SetReadDeadline(time.Now().Add(within))
	buf := make([]byte, 64)
	for {
		_, err := conn.Read(buf)
		if err == nil {
			continue
		}
		var ne net.Error
		return !errors.As(err, &ne) || !ne.Timeout()
	}
}

// A line longer than maxLine is dropped as soon as it passes the
// limit, not read to its end.
func TestAnOverLongLineIsDropped(t *testing.T) {
	dir := t.TempDir()
	startOne(t, dir, true)
	conn := dialOne(t, dir)
	go flood(conn, time.Now().Add(10*time.Second))
	if !dropped(conn, readTime/2) {
		t.Fatalf("a line longer than %d bytes was still read after %v", maxLine, readTime/2)
	}
}

// A handover with long arguments, under maxLine, is still taken.
func TestALongHandoverIsTaken(t *testing.T) {
	dir := t.TempDir()
	got, _ := startOne(t, dir, true)
	long := strings.Repeat("x", maxLine/2)
	if ok, err := Hand(dir, Handover{Args: []string{long}}); !ok || err != nil {
		t.Fatalf("the handover was taken %v, %v", ok, err)
	}
	if h := <-got; len(h.Args) != 1 || h.Args[0] != long {
		t.Fatal("the one running heard other arguments")
	}
}

// A connection that sends nothing, or a byte at a time, is dropped
// after readTime.
func TestASlowSenderIsDropped(t *testing.T) {
	was := readTime
	readTime = 200 * time.Millisecond
	t.Cleanup(func() { readTime = was })
	dir := t.TempDir()
	startOne(t, dir, true)
	silent := dialOne(t, dir)
	slow := dialOne(t, dir)
	go func() {
		for {
			if _, err := slow.Write([]byte("a")); err != nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	for name, conn := range map[string]net.Conn{"silent": silent, "slow": slow} {
		if !dropped(conn, 2*time.Second) {
			t.Errorf("the %s connection was not dropped after %v", name, readTime)
		}
	}
}

// Many connections that flood the one running hold no more memory
// than maxServing lines, and a handover after them is still taken.
func TestManyConnectionsKeepMemoryBounded(t *testing.T) {
	dir := t.TempDir()
	got, _ := startOne(t, dir, true)
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapInuse
	var peak atomic.Uint64
	done := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		var ms runtime.MemStats
		for {
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peak.Load() {
				peak.Store(ms.HeapInuse)
			}
			select {
			case <-done:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()
	const conns = 64
	var wg sync.WaitGroup
	until := time.Now().Add(2 * time.Second)
	for range conns {
		conn := dialOne(t, dir)
		wg.Go(func() { flood(conn, until) })
	}
	wg.Wait()
	close(done)
	<-sampled
	// A line served is read into a buffer up to about twice maxLine, and
	// the lines of dropped connections stay in the heap until the next
	// collection, which comes when the heap has doubled.
	const bound = 8 * maxServing * maxLine
	if grew := peak.Load() - min(base, peak.Load()); grew > bound {
		t.Errorf("%d connections grew the heap by %d MiB, over %d MiB", conns, grew>>20, bound>>20)
	}
	if ok, err := Hand(dir, Handover{Args: []string{"-e", "x"}}); !ok || err != nil {
		t.Fatalf("after the flood, the handover was taken %v, %v", ok, err)
	}
	<-got
}
