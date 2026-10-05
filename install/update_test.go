package install

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v1.2.0", "1.2.0", 0},
		{"v1.2.0", "v1.10.0", -1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.0.0-beta.1", "v1.0.0", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.10", -1},
		{"v1.0.0-beta.2", "v1.0.0-alpha.9", 1},
		{"v1.0.0-1", "v1.0.0-beta", -1},
		{"v1.0.0-beta", "v1.0.0-beta.1", -1},
		{"v1.0.0+build.5", "v1.0.0", 0},
		{"dev", "v1.0.0", 0},
		{"v1.0", "v1.0.0", 0},
	} {
		if got := compare(c.a, c.b); got != c.want {
			t.Errorf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	for v, want := range map[string]bool{"v1.2.3": true, "1.2.3-rc.1": true, "dev": false, "": false, "v01.2.3": false, "v1.2.3-": false} {
		if IsRelease(v) != want {
			t.Errorf("IsRelease(%q) = %v", v, !want)
		}
	}
}

// release is a release as the test's GitHub serves it.
type release struct {
	tag        string
	prerelease bool
	files      map[string][]byte
}

// fakeGitHub serves releases as GitHub's API does, each with a
// SHA256SUMS of its files.
func fakeGitHub(t *testing.T, rels ...release) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	var list []map[string]any
	for _, r := range rels {
		var sums strings.Builder
		var assets []map[string]string
		for name, body := range r.files {
			sum := sha256.Sum256(body)
			fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
			url := "/dl/" + r.tag + "/" + name
			mux.HandleFunc(url, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) })
			assets = append(assets, map[string]string{"name": name, "browser_download_url": srv.URL + url})
		}
		sumsURL := "/dl/" + r.tag + "/SHA256SUMS"
		body := sums.String()
		mux.HandleFunc(sumsURL, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
		assets = append(assets, map[string]string{"name": "SHA256SUMS", "browser_download_url": srv.URL + sumsURL})
		list = append(list, map[string]any{"tag_name": r.tag, "prerelease": r.prerelease, "html_url": "https://example.com/" + r.tag, "assets": assets})
	}
	mux.HandleFunc("/repos/marrasen/studio/releases", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(list)
	})
	return srv
}

func asset(version string) string {
	return "studio_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + exeSuffix
}

// The newest release with a program for this system is the latest; a
// pre-release only for a source that takes them.
func TestGitHubLatest(t *testing.T) {
	srv := fakeGitHub(t,
		release{tag: "v1.3.0-beta.1", prerelease: true, files: map[string][]byte{asset("1.3.0-beta.1"): []byte("beta")}},
		release{tag: "v1.2.0", files: map[string][]byte{asset("1.2.0"): []byte("one two")}},
		release{tag: "v1.4.0", files: map[string][]byte{"studio_1.4.0_plan9_mips" + exeSuffix: []byte("elsewhere")}},
		release{tag: "v1.1.0", files: map[string][]byte{asset("1.1.0"): []byte("one one")}},
	)
	g := GitHub{Repo: "marrasen/studio", API: srv.URL}
	r, err := g.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "v1.2.0" || r.Name != asset("1.2.0") {
		t.Fatalf("latest %+v, want v1.2.0", r)
	}
	g.Prerelease = true
	r, err = g.Latest(context.Background())
	if err != nil || r.Version != "v1.3.0-beta.1" {
		t.Fatalf("latest with pre-releases %+v, %v", r, err)
	}
	a := App{Name: "studio", Version: "v1.2.0", Updates: GitHub{Repo: "marrasen/studio", API: srv.URL}}
	if _, newer, err := Check(context.Background(), a); err != nil || newer {
		t.Fatalf("Check says newer %v, %v; want the same", newer, err)
	}
	a.Version = "dev"
	if _, newer, _ := Check(context.Background(), a); newer {
		t.Error("a build from a working tree was behind")
	}
}

// Stage puts a release checked against its sums in place of the
// installed program, from the program itself or from an archive.
func TestStage(t *testing.T) {
	dir := t.TempDir()
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	f, _ := zw.Create("studio/studio" + exeSuffix)
	_, _ = f.Write([]byte("from the zip"))
	_ = zw.Close()
	srv := fakeGitHub(t,
		release{tag: "v2.0.0", files: map[string][]byte{asset("2.0.0"): []byte("two")}},
	)
	a := App{Name: "studio", Version: "v1.0.0", Dir: func() (string, error) { return dir, nil },
		Updates: GitHub{Repo: "marrasen/studio", API: srv.URL}}
	exe := filepath.Join(dir, "studio"+exeSuffix)
	if err := os.WriteFile(exe, []byte("one"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, newer, err := Check(context.Background(), a)
	if err != nil || !newer {
		t.Fatalf("Check %+v, %v, %v", r, newer, err)
	}
	if err := Stage(context.Background(), a, r); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(exe); string(raw) != "two" {
		t.Fatalf("the program holds %q after the update", raw)
	}

	zsrv := fakeGitHub(t, release{tag: "v3.0.0", files: map[string][]byte{
		"studio_3.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip": zipped.Bytes()}})
	a.Updates = GitHub{Repo: "marrasen/studio", API: zsrv.URL}
	r, _, err = Check(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if err := Stage(context.Background(), a, r); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(exe); string(raw) != "from the zip" {
		t.Fatalf("the program holds %q after the update from a zip", raw)
	}

	// A program that is not what its sums say is not put in place.
	bad := r
	bad.Name = "nothing"
	if err := Stage(context.Background(), a, bad); err == nil {
		t.Fatal("staged a release whose sums say nothing of it")
	}
	if raw, _ := os.ReadFile(exe); string(raw) != "from the zip" {
		t.Fatalf("a failed update left %q", raw)
	}
}

// The ID is made from the name, and mistakes in an App are said.
func TestCheckApp(t *testing.T) {
	a := App{Name: "Marras Mastering Studio!"}
	if a.id() != "marras-mastering-studio" {
		t.Errorf("id %q", a.id())
	}
	for _, bad := range []App{
		{},
		{Name: "x", ID: "a/b"},
		{Name: "x", FileTypes: []FileType{{Exts: []string{"album"}}}},
		{Name: "x", Choices: []Choice{{Key: PickDesktop}}},
	} {
		if err := bad.check(); err == nil {
			t.Errorf("%+v passed", bad)
		}
	}
}

// The updater does what the mode says at each look: installs and says
// so, tells the program, or does nothing.
func TestKeepUpToDateFollowsTheMode(t *testing.T) {
	was, wasEvery := updateFirst, updateEvery
	updateFirst, updateEvery = time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { updateFirst, updateEvery = was, wasEvery })
	srv := fakeGitHub(t, release{tag: "v2.0.0", files: map[string][]byte{asset("2.0.0"): []byte("two")}})
	dir := t.TempDir()
	available, updated := make(chan Release, 4), make(chan Release, 4)
	a := App{Name: "studio", Version: "v1.0.0", Dir: func() (string, error) { return dir, nil },
		Updates:   GitHub{Repo: "marrasen/studio", API: srv.URL},
		Available: func(r Release) { available <- r }, Updated: func(r Release) { updated <- r }}
	exe := filepath.Join(dir, "studio"+exeSuffix)
	if err := os.WriteFile(exe, []byte("one"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := manifest{ID: "studio", Version: "v1.0.0", Exe: "studio" + exeSuffix, Updates: UpdatesNotify}
	if err := m.write(dir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go keepUpToDate(ctx, a)
	select {
	case r := <-available:
		if r.Version != "v2.0.0" {
			t.Fatalf("told of %s", r.Version)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("set to notify, the program was not told")
	}
	if raw, _ := os.ReadFile(exe); string(raw) != "one" {
		t.Fatal("set to notify, the release was put in place unasked")
	}
	cancel()

	if err := SetUpdates(a, UpdatesInstall); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go keepUpToDate(ctx, a)
	select {
	case <-updated:
	case <-time.After(5 * time.Second):
		t.Fatal("set to install, the release was not put in place")
	}
	if raw, _ := os.ReadFile(exe); string(raw) != "two" {
		t.Fatalf("set to install, the program holds %q", raw)
	}
}
