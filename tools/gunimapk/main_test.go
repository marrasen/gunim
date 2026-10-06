package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
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
