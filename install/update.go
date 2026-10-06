package install

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Source is where newer releases of a program come from.
type Source interface {
	// Latest is the newest release with a program for this system.
	Latest(ctx context.Context) (Release, error)
}

// Release is one release of a program, with its program for this
// system.
type Release struct {
	// Version is the release's version, as "v1.3.0".
	Version string
	// Page is the release's page on the web, and Notes what it says of
	// the release.
	Page, Notes string
	// Name is the file the program comes in: the program itself, or a
	// .zip or .tar.gz holding it. Program is where it downloads from,
	// Sums where the SHA256SUMS file that checks it does, and Signature
	// where SHA256SUMS.sig, its signature, does. Each is an https URL.
	Name, Program, Sums, Signature string
}

// GitHub finds a program's releases on GitHub.
//
// Each release holds the program for each system as a file named
// <repository>_<version>_<goos>_<goarch>, with ".exe" on Windows, as
// "mastering-studio_1.3.0_windows_amd64.exe", or that name with ".zip"
// or ".tar.gz" for the program in an archive, a SHA256SUMS file of
// their checksums, as sha256sum writes it, and SHA256SUMS.sig, its
// signature, as "gunimsign SHA256SUMS" writes it.
type GitHub struct {
	// Repo is the repository, as "marrasen/mastering-studio".
	Repo string
	// Prerelease takes pre-releases too, for a program's beta testers.
	Prerelease bool
	// Asset, when set, names the file of the program for a version, as
	// "1.3.0", and a system, in place of the usual names.
	Asset func(version, goos, goarch string) string
	// API is GitHub's API, for a test to stand in for it.
	API string
}

// apiPatience is how long GitHub is given to say what it has, and
// fetchPatience how long a program takes to download.
const (
	apiPatience   = 15 * time.Second
	fetchPatience = 10 * time.Minute
	// readLimit is the most of an answer read, and programLimit the
	// largest program fetched.
	readLimit    = 4 << 20
	programLimit = 1 << 30
)

// Latest implements [Source].
func (g GitHub) Latest(ctx context.Context) (Release, error) {
	if !strings.Contains(g.Repo, "/") {
		return Release{}, fmt.Errorf("install: GitHub.Repo %q is not owner/name", g.Repo)
	}
	api := g.API
	if api == "" {
		api = "https://api.github.com"
	}
	ctx, cancel := context.WithTimeout(ctx, apiPatience)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/repos/"+g.Repo+"/releases?per_page=30", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gunim-install")
	resp, err := do(req)
	if err != nil {
		return Release{}, fmt.Errorf("ask GitHub for releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var said []struct {
		Tag        string `json:"tag_name"`
		Page       string `json:"html_url"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, readLimit)).Decode(&said); err != nil {
		return Release{}, fmt.Errorf("read what GitHub answered: %w", err)
	}
	var best Release
	for _, r := range said {
		if r.Draft || r.Prerelease && !g.Prerelease || !IsRelease(r.Tag) {
			continue
		}
		if best.Version != "" && compare(r.Tag, best.Version) <= 0 {
			continue
		}
		assets := map[string]string{}
		for _, a := range r.Assets {
			assets[a.Name] = a.URL
		}
		sums, ok := assets["SHA256SUMS"]
		if !ok {
			continue
		}
		for _, name := range g.names(strings.TrimPrefix(r.Tag, "v")) {
			if at, ok := assets[name]; ok {
				best = Release{Version: r.Tag, Page: r.Page, Notes: r.Body, Name: name, Program: at, Sums: sums,
					Signature: assets["SHA256SUMS.sig"]}
				break
			}
		}
	}
	if best.Version == "" {
		return Release{}, fmt.Errorf("%s has no release with a program for %s/%s", g.Repo, runtime.GOOS, runtime.GOARCH)
	}
	return best, nil
}

// names are the names the program for this system may have in the
// release of version.
func (g GitHub) names(version string) []string {
	if g.Asset != nil {
		return []string{g.Asset(version, runtime.GOOS, runtime.GOARCH)}
	}
	base := path.Base(g.Repo) + "_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH
	return []string{base + exeSuffix, base + ".zip", base + ".tar.gz"}
}

// Check asks a's source for its newest release, and reports whether it
// is newer than this build. A build that is no release has no order
// against one, and is never behind.
func Check(ctx context.Context, a App) (Release, bool, error) {
	if a.Updates == nil {
		return Release{}, false, errors.New("install: App.Updates is not set")
	}
	r, err := a.Updates.Latest(ctx)
	if err != nil {
		return Release{}, false, err
	}
	return r, IsRelease(a.Version) && Newer(r.Version, a.Version), nil
}

// Stage downloads release r, checks it against its SHA256SUMS and their
// signature, and puts
// it in place of the installed program, for the next time the program
// starts; the program running goes on as it is. The rest of r's files
// are put in place as it first starts. A copy installed already that is
// as new as r, or newer, stays, as when a copy started before it was
// installed still runs.
func Stage(ctx context.Context, a App, r Release) error {
	if err := a.check(); err != nil {
		return err
	}
	dir, err := a.dir()
	if err != nil {
		return err
	}
	m, err := readManifest(dir, a.id())
	if err != nil {
		return err
	}
	if m != nil && !newerThanInstalled(r, m.Version) {
		return fmt.Errorf("install: %s is installed already, and %s is no newer", m.Version, r.Version)
	}
	return StageTo(ctx, a, r, filepath.Join(dir, a.exe()))
}

// StageTo is [Stage] for the program at exe, as a copy that is not
// installed updates itself where it is.
func StageTo(ctx context.Context, a App, r Release, exe string) error {
	if err := a.check(); err != nil {
		return err
	}
	key, err := a.updateKey()
	if err != nil {
		return err
	}
	body, err := fetch(ctx, a.exe(), key, r)
	if err != nil {
		return err
	}
	return writeFile(exe, body, 0o755)
}

// newerThanInstalled reports whether r is newer than the version
// installed: always, where the install kept no version that is a
// release.
func newerThanInstalled(r Release, installed string) bool {
	return !IsRelease(installed) || Newer(r.Version, installed)
}

// fetch downloads r's program, checked against its SHA256SUMS, whose
// signature key checks, taking the file named program out of an
// archive.
func fetch(ctx context.Context, program string, key ed25519.PublicKey, r Release) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchPatience)
	defer cancel()
	if r.Sums == "" {
		return nil, fmt.Errorf("%s has no SHA256SUMS to check it against", r.Version)
	}
	if r.Signature == "" {
		return nil, fmt.Errorf("%s has no SHA256SUMS.sig, the signature that says who made it", r.Version)
	}
	sums, err := get(ctx, r.Sums, readLimit)
	if err != nil {
		return nil, fmt.Errorf("fetch SHA256SUMS: %w", err)
	}
	sig, err := get(ctx, r.Signature, readLimit)
	if err != nil {
		return nil, fmt.Errorf("fetch SHA256SUMS.sig: %w", err)
	}
	if !verify(key, sums, sig) {
		return nil, fmt.Errorf("the SHA256SUMS of %s is not signed with App.UpdateKey", r.Version)
	}
	want, err := sumOf(sums, r.Name)
	if err != nil {
		return nil, err
	}
	body, err := get(ctx, r.Program, programLimit)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", r.Name, err)
	}
	got := sha256.Sum256(body)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%s is not what SHA256SUMS says it is", r.Name)
	}
	switch {
	case strings.HasSuffix(r.Name, ".zip"), strings.HasSuffix(r.Name, ".tar.gz"):
		body, err = unpack(body, r.Name, program)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

// verify reports whether sig, as gunimsign writes it, is key's
// signature of sums.
func verify(key ed25519.PublicKey, sums, sig []byte) bool {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	return err == nil && len(raw) == ed25519.SignatureSize && ed25519.Verify(key, sums, raw)
}

// client fetches over https only, redirects too, so no one on the way
// can change what comes; plain http only from this computer, as from a
// test's server.
var client = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	return secure(req.URL)
}}

// secure says what is wrong with fetching u, or nothing.
func secure(u *url.URL) error {
	if u.Scheme == "https" || u.Scheme == "http" && loopback(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("install: %s is not https", u.Redacted())
}

// loopback reports whether host is this computer.
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// do sends req through client, once secure has passed its URL.
func do(req *http.Request) (*http.Response, error) {
	if err := secure(req.URL); err != nil {
		return nil, err
	}
	return client.Do(req)
}

// get fetches addr, at most limit bytes of it.
func get(ctx context.Context, addr string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gunim-install")
	resp, err := do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("it is larger than any program fetched")
	}
	return body, nil
}

// sumOf is the checksum SHA256SUMS gives name.
func sumOf(sums []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS says nothing of %s", name)
}

// unpack is the file called program in archive, at any depth.
func unpack(archive []byte, name, program string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if path.Base(f.Name) == program && f.Mode().IsRegular() {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer func() { _ = rc.Close() }()
				return readProgram(rc)
			}
		}
		return nil, fmt.Errorf("%s holds no %s", name, program)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s holds no %s", name, program)
		}
		if path.Base(h.Name) == program && h.Typeflag == tar.TypeReg {
			return readProgram(tr)
		}
	}
}

// readProgram reads a program out of an archive, and fails on one
// larger than any program fetched, in place of cutting it short.
func readProgram(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, programLimit+1))
	if err == nil && int64(len(body)) > programLimit {
		err = errors.New("the program in the archive is larger than any program fetched")
	}
	return body, err
}

// Restart starts the installed program with args, for a program to call
// just before it ends, to come back as the release [Stage] put in place.
func Restart(a App, args ...string) error {
	dir, err := a.dir()
	if err != nil {
		return err
	}
	return launch(filepath.Join(dir, a.exe()), args)
}

// How soon after it starts an installed program first looks for a newer
// release, and how often after that.
var (
	updateFirst = time.Minute
	updateEvery = 24 * time.Hour
)

// keepUpToDate looks for newer releases of a while ctx lasts, as the
// install's update mode says at each look: it puts one in place for the
// next start, or tells the program of it, once a release.
func keepUpToDate(ctx context.Context, a App) {
	told := ""
	wait := updateFirst
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = updateEvery
		in, err := Find(a)
		if err != nil || in.Updates == UpdatesOff || in.Updates == UpdatesNotify && a.Available == nil {
			continue
		}
		r, newer, err := Check(ctx, a)
		if err != nil || !newer || r.Version == told {
			continue
		}
		if !newerThanInstalled(r, in.Version) {
			// Installed since this copy started, as a newer version.
			continue
		}
		if in.Updates == UpdatesNotify {
			told = r.Version
			a.Available(r)
			continue
		}
		if err := Stage(ctx, a, r); err != nil {
			continue
		}
		told = r.Version
		if a.Updated != nil {
			a.Updated(r)
		}
	}
}
