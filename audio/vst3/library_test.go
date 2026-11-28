//go:build !js

package vst3

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// readJournal reads the journal at path.
func readJournal(t *testing.T, path string) journalFile {
	t.Helper()
	var j journalFile
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	return j
}

// TestAModuleThatCrashedIsSkipped scans a folder of a bundle that lists
// nothing, so the scan loads its module. The journal names the module
// while it loads, and nothing after. A journal still naming it as the
// next library starts, as a crash leaves it, has that library skip it
// and list it as failed, until Retry loads it again.
func TestAModuleThatCrashedIsSkipped(t *testing.T) {
	// The library scans the system's folders too: it scans none here, so
	// the plugins installed on the machine stay out of the test.
	systemDirs = func() []string { return nil }
	t.Cleanup(func() { systemDirs = Dirs })
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Crashy.vst3")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(t.TempDir(), "journal.json")
	var loaded []string
	beforeLoad = func(path string) {
		loaded = append(loaded, path)
		if j := readJournal(t, journal); j.Loading != path {
			t.Errorf("loading %s, the journal names %q", path, j.Loading)
		}
	}
	t.Cleanup(func() { beforeLoad = nil })

	lib := NewLibraryAt(journal, dir)
	if len(lib.Failed()) != 0 || !slices.Equal(loaded, []string{bundle}) {
		t.Fatalf("a first scan failed %v, and loaded %v", lib.Failed(), loaded)
	}
	if j := readJournal(t, journal); j.Loading != "" || len(j.Crashed) != 0 {
		t.Fatalf("after the scan, the journal holds %+v", j)
	}

	// The program dies loading it.
	b, _ := json.Marshal(journalFile{Loading: bundle})
	if err := os.WriteFile(journal, b, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded = nil
	lib = NewLibraryAt(journal, dir)
	if !slices.Equal(lib.Failed(), []string{bundle}) || len(loaded) != 0 {
		t.Fatalf("after a crash, the scan failed %v, and loaded %v", lib.Failed(), loaded)
	}
	// Still skipped by the library after.
	if lib := NewLibraryAt(journal, dir); !slices.Equal(lib.Failed(), []string{bundle}) {
		t.Fatalf("a library after that fails %v", lib.Failed())
	}
	// Tried again: it loads, and fails as other than a crash.
	if err := lib.Retry(bundle); err == nil || errors.Is(err, ErrCrashed) || !slices.Equal(loaded, []string{bundle}) {
		t.Fatalf("retried, it loaded %v: %v", loaded, err)
	}
	if len(lib.Failed()) != 0 {
		t.Fatalf("retried, it is listed as failed: %v", lib.Failed())
	}
	if j := readJournal(t, journal); j.Loading != "" || len(j.Crashed) != 0 {
		t.Fatalf("after the retry, the journal holds %+v", j)
	}
}

// TestAPluginWhoseModuleCrashedIsNotFound has plug look up a plugin of
// LSP's, whose bundle lists it, so the scan loads nothing: a journal
// naming LSP's module as crashed keeps Find from loading it, with the
// crash as the reason.
func TestAPluginWhoseModuleCrashedIsNotFound(t *testing.T) {
	m := lsp(t)
	journal := filepath.Join(t.TempDir(), "journal.json")
	b, _ := json.Marshal(journalFile{Crashed: []string{m.Path}})
	if err := os.WriteFile(journal, b, 0o644); err != nil {
		t.Fatal(err)
	}
	lib := NewLibraryAt(journal, filepath.Dir(m.Path))
	var id string
	for _, f := range lib.Plugins() {
		if f.Name == "Sampler Mono" {
			id = f.ID
		}
	}
	if id == "" || lib.Find(id) || !errors.Is(lib.Err(id), ErrCrashed) {
		t.Fatalf("the sampler %q was found, or would not load for %v", id, lib.Err(id))
	}
}

// fakeBundle makes a bundle in dir named name that lists nothing, its
// binary holding body, and returns its path and its binary's.
func fakeBundle(t *testing.T, dir, name, body string) (bundle, bin string) {
	t.Helper()
	bundle = filepath.Join(dir, name+".vst3")
	sub := map[string]string{"darwin": "MacOS", "windows": "x86_64-win"}[runtime.GOOS]
	if sub == "" {
		sub = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH] + "-linux"
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		sub = "arm64-win"
	}
	ext := map[string]string{"linux": ".so", "windows": ".vst3"}[runtime.GOOS]
	bin = filepath.Join(bundle, "Contents", sub, name+ext)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return bundle, bin
}

// TestAModuleScannedBeforeIsNotLoaded has a journal keep what the module
// of a bundle that lists nothing makes. A library lists it from the
// journal and loads nothing, so no module is held loaded that no song
// uses. Once the binary changes, the library loads it again.
func TestAModuleScannedBeforeIsNotLoaded(t *testing.T) {
	systemDirs = func() []string { return nil }
	t.Cleanup(func() { systemDirs = Dirs })
	dir := t.TempDir()
	bundle, bin := fakeBundle(t, dir, "Cached", "a module")
	size, modTime, ok := stamp(bundle)
	if !ok {
		t.Fatal("the fake bundle's binary is not found")
	}
	class := Class{ID: uid{1, 2, 3}, Name: "Cached EQ", Category: "Audio Module Class", SubCategories: "Fx|EQ"}
	journal := filepath.Join(t.TempDir(), "journal.json")
	b, _ := json.Marshal(journalFile{Scanned: map[string]scannedModule{
		bundle:                          {Size: size, ModTime: modTime, Classes: []Class{class}},
		filepath.Join(dir, "Gone.vst3"): {Size: 1, ModTime: 1, Classes: []Class{class}},
	}})
	if err := os.WriteFile(journal, b, 0o644); err != nil {
		t.Fatal(err)
	}
	var loaded []string
	beforeLoad = func(path string) { loaded = append(loaded, path) }
	t.Cleanup(func() { beforeLoad = nil })

	lib := NewLibraryAt(journal, dir)
	if ps := lib.Plugins(); len(loaded) != 0 || len(ps) != 1 || ps[0].Name != "Cached EQ" || ps[0].Path != bundle {
		t.Fatalf("the library loaded %v and found %+v", loaded, ps)
	}
	if j := readJournal(t, journal); len(j.Scanned) != 1 || len(j.Scanned[bundle].Classes) != 1 {
		t.Fatalf("after the scan, the journal keeps %+v, not the bundle alone", j.Scanned)
	}

	// The plugin is updated: its binary is another size.
	if err := os.WriteFile(bin, []byte("a newer module"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib = NewLibraryAt(journal, dir)
	if ps := lib.Plugins(); !slices.Equal(loaded, []string{bundle}) || len(ps) != 0 {
		t.Fatalf("after an update, the library loaded %v and found %+v", loaded, ps)
	}
}

// TestAModuleLoadedInAScanIsKept scans a bundle that lists nothing,
// whose binary is LSP's module: the first library loads it, and the
// next lists its plugins without loading it.
func TestAModuleLoadedInAScanIsKept(t *testing.T) {
	m := lsp(t)
	systemDirs = func() []string { return nil }
	t.Cleanup(func() { systemDirs = Dirs })
	bin, err := binaryPath(m.Path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bundle, fake := fakeBundle(t, dir, "Unlisted", "")
	if err := os.Remove(fake); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin, fake); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(t.TempDir(), "journal.json")
	var loaded []string
	beforeLoad = func(path string) { loaded = append(loaded, path) }
	t.Cleanup(func() { beforeLoad = nil })

	first := NewLibraryAt(journal, dir).Plugins()
	if !slices.Equal(loaded, []string{bundle}) || len(first) == 0 {
		t.Fatalf("the first scan loaded %v and found %d plugins", loaded, len(first))
	}
	loaded = nil
	next := NewLibraryAt(journal, dir).Plugins()
	if len(loaded) != 0 || len(next) != len(first) {
		t.Fatalf("the next scan loaded %v and found %d plugins, not %d", loaded, len(next), len(first))
	}
}
