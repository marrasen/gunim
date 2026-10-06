//go:build windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// An update goes through while the copy the last update moved aside
// still runs, as a program in the tray does for days: the running copy
// is moved out of the way, and goes once it has ended.
func TestReplacePastARunningOld(t *testing.T) {
	ping, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "PING.EXE"))
	if err != nil {
		t.Skip("no PING.EXE to run:", err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "studio.exe")
	if err = os.WriteFile(exe+".old", ping, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(exe, []byte("two"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe+".old", "-n", "60", "127.0.0.1")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}
	t.Cleanup(stop)
	if err = os.Remove(exe + ".old"); err == nil {
		t.Skip("this Windows let a running program's file go")
	}
	part := filepath.Join(dir, "part")
	if err = os.WriteFile(part, []byte("three"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = Replace(part, exe); err != nil {
		t.Fatalf("the update failed with the last one's copy running: %v", err)
	}
	for f, want := range map[string]string{exe: "three", exe + ".old": "two"} {
		if raw, _ := os.ReadFile(f); string(raw) != want {
			t.Errorf("%s holds %q, want %q", filepath.Base(f), raw, want)
		}
	}
	busy := leftovers(exe, ".old")
	if len(busy) != 1 {
		t.Fatalf("the running copy went to %v, want one name of its own", busy)
	}
	CleanOld(exe)
	if _, err = os.Stat(busy[0]); err != nil {
		t.Errorf("CleanOld took away the running copy, or it went: %v", err)
	}
	stop()
	CleanOld(exe)
	if left := leftovers(exe, ".old"); len(left) != 0 {
		t.Errorf("once it ended, CleanOld left %v", left)
	}
}
