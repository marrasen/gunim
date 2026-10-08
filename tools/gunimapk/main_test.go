package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestTheSDKIsLookedForWhereAndroidStudioPutsIt(t *testing.T) {
	h := t.TempDir()
	env := map[string]string{"LOCALAPPDATA": `C:\Users\m\AppData\Local`}
	getenv := func(k string) string { return env[k] }
	cases := map[string][]string{
		"linux":   {filepath.Join(h, "Android", "Sdk"), filepath.Join(h, "Android", "sdk")},
		"darwin":  {filepath.Join(h, "Library", "Android", "sdk")},
		"windows": {filepath.Join(env["LOCALAPPDATA"], "Android", "Sdk")},
	}
	for goos, want := range cases {
		if got := sdkPlaces(getenv, goos, h); !slices.Equal(got, want) {
			t.Errorf("%s: looks in %q, want %q", goos, got, want)
		}
	}
}

func TestTheEnvironmentAndAndroidrcComeFirst(t *testing.T) {
	h := t.TempDir()
	if err := os.WriteFile(filepath.Join(h, ".androidrc"), []byte("--verbose\n--sdk=~/sdks/android\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"ANDROID_HOME": "/opt/android", "ANDROID_SDK_ROOT": "/opt/android"}
	got := sdkPlaces(func(k string) string { return env[k] }, "linux", h)
	want := []string{"/opt/android", filepath.Join(h, "sdks", "android"),
		filepath.Join(h, "Android", "Sdk"), filepath.Join(h, "Android", "sdk")}
	if !slices.Equal(got, want) {
		t.Fatalf("looks in %q, want %q", got, want)
	}
}

func TestPasswordsComeFromTheEnvironmentOrTheTerminal(t *testing.T) {
	none := func(string) string { return "" }
	got := signArgs("k.jks", "gunim", none, "o.apk", "a.apk")
	want := []string{"sign", "--ks", "k.jks", "--ks-key-alias", "gunim", "--out", "o.apk", "a.apk"}
	if !slices.Equal(got, want) {
		t.Fatalf("with no passwords set, signs with %q, want %q, so apksigner asks", got, want)
	}
	both := func(k string) string { return "secret" }
	got = signArgs("k.jks", "gunim", both, "o.apk", "a.apk")
	want = []string{"sign", "--ks", "k.jks", "--ks-key-alias", "gunim",
		"--ks-pass", "env:GUNIMAPK_STORE_PASS", "--key-pass", "env:GUNIMAPK_KEY_PASS", "--out", "o.apk", "a.apk"}
	if !slices.Equal(got, want) {
		t.Fatalf("with passwords set, signs with %q, want %q", got, want)
	}
}

func TestADebugBuildIsDebuggableAndAReleaseBuildIsNot(t *testing.T) {
	if (options{}).release() {
		t.Error("a build with no keystore is a release build")
	}
	if !(options{keystore: "k.jks"}).release() {
		t.Error("a build with a keystore is a debug build")
	}
}

func TestTheActivityHoldsTheScreenOneWayOnlyWhenAsked(t *testing.T) {
	const attr = `android:screenOrientation=`
	turns := manifestFor("org.gunim.calc", "Calculator", false, nil, "")
	if strings.Contains(turns, attr) {
		t.Errorf("with no orientation, the manifest holds the screen one way:\n%s", turns)
	}
	upright := manifestFor("org.gunim.calc", "Calculator", false, nil, "portrait")
	if !strings.Contains(upright, attr+`"portrait"`) {
		t.Errorf("asked for portrait, the manifest lets the screen turn:\n%s", upright)
	}
	for _, m := range []string{turns, upright} {
		if err := xml.Unmarshal([]byte(m), new(struct{})); err != nil {
			t.Errorf("the manifest is not XML: %v\n%s", err, m)
		}
	}
}

func TestAnOutputEndingAabIsABundle(t *testing.T) {
	for out, want := range map[string]bool{"calc.apk": false, "calc.aab": true, "dist/Calc.AAB": true, "calc": false} {
		if got := (options{out: out}).bundle(); got != want {
			t.Errorf("-o %s: bundle is %v, want %v", out, got, want)
		}
	}
}

func TestABundlesModuleHoldsTheManifestWhereBundletoolLooks(t *testing.T) {
	for name, want := range map[string]string{
		"AndroidManifest.xml":                   "manifest/AndroidManifest.xml",
		"resources.pb":                          "resources.pb",
		"res/mipmap-anydpi-v26/ic_launcher.xml": "res/mipmap-anydpi-v26/ic_launcher.xml",
		"kotlin/stray.txt":                      "root/kotlin/stray.txt",
	} {
		if got := moduleName(name); got != want {
			t.Errorf("%s goes to %s, want %s", name, got, want)
		}
	}
}

func TestJarsignerTakesThePasswordsAsApksignerDoes(t *testing.T) {
	none := func(string) string { return "" }
	got := jarsignerArgs("k.jks", "upload", none, "o.aab", "u.aab")
	want := []string{"-keystore", "k.jks", "-signedjar", "o.aab", "u.aab", "upload"}
	if !slices.Equal(got, want) {
		t.Fatalf("with no passwords set, signs with %q, want %q, so jarsigner asks", got, want)
	}
	both := func(string) string { return "secret" }
	got = jarsignerArgs("k.jks", "upload", both, "o.aab", "u.aab")
	want = []string{"-keystore", "k.jks", "-storepass:env", "GUNIMAPK_STORE_PASS", "-keypass:env", "GUNIMAPK_KEY_PASS",
		"-signedjar", "o.aab", "u.aab", "upload"}
	if !slices.Equal(got, want) {
		t.Fatalf("with passwords set, signs with %q, want %q", got, want)
	}
}

func TestTheActivityHearsBackThroughTheCallback(t *testing.T) {
	m := manifestFor("org.gunim.calc", "Calculator", false, nil, "")
	if !strings.Contains(m, `android:enableOnBackInvokedCallback="true"`) {
		t.Errorf("the manifest leaves the back callback off, so Android 13 to 15 send Back as a key and 16 sends nothing:\n%s", m)
	}
}

func TestALibraryAlignedTo4KBIsRefused(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the test's own binary is an ELF aligned to 4 KB on linux/amd64 alone")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPages(exe); err == nil {
		t.Fatalf("%s loads at 4 KB boundaries, and passes as fit for 16 KB pages", exe)
	}
}

func TestAFetchThatDiffersLeavesNoFile(t *testing.T) {
	body := []byte("bundletool")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	sum := sha256.Sum256(body)
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.jar")
	if err := fetch(context.Background(), srv.URL, bad, strings.Repeat("0", 64)); err == nil {
		t.Error("a download with the wrong checksum is taken")
	}
	good := filepath.Join(dir, "good.jar")
	if err := fetch(context.Background(), srv.URL, good, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "good.jar" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the cache holds %q, want only good.jar", names)
	}
}

func TestTheMediaServiceComesWithPlaybackAlone(t *testing.T) {
	const service, fgs = `gunim.android.GunimService`, `android.permission.FOREGROUND_SERVICE"`
	plain := manifestFor("org.gunim.quiz", "Quiz", false, []string{"music"}, "")
	if strings.Contains(plain, service) || strings.Contains(plain, fgs) {
		t.Errorf("without playback, the manifest holds the media service or its permission:\n%s", plain)
	}
	player := manifestFor("org.gunim.player", "Player", false, []string{"music", "playback"}, "")
	for _, want := range []string{service, fgs, `android.permission.FOREGROUND_SERVICE_MEDIA_PLAYBACK"`, `android:foregroundServiceType="mediaPlayback"`} {
		if !strings.Contains(player, want) {
			t.Errorf("with playback, the manifest lacks %s:\n%s", want, player)
		}
	}
	for _, m := range []string{plain, player} {
		if err := xml.Unmarshal([]byte(m), new(struct{})); err != nil {
			t.Errorf("the manifest is not XML: %v\n%s", err, m)
		}
	}
}
