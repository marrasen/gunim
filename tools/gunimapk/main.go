// Command gunimapk builds a gunim program into an Android APK.
//
//	go run ./tools/gunimapk -o calculator.apk ./example/calculator
//
// It builds the program as libgunim.so for each ABI asked for, with cgo
// and the NDK's clang, compiles the Java half of the Android driver,
// links a manifest that starts gunim's activity, and signs the APK with
// the debug key. -icon gives it a launcher icon from a PNG, and -name a
// label. The APK is a debug build, named for its commit and the
// time it was built, which it shows as it starts. -install installs it on the device adb sees, and -run
// starts it there too.
//
// It finds the Android SDK at $ANDROID_HOME, $ANDROID_SDK_ROOT or
// ~/Android/sdk, and takes the newest NDK, build tools and platform
// installed there. Java comes from $JAVA_HOME, or the PATH.
package main

import (
	"archive/zip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim/driver/android/java"
)

// minSDK is the oldest Android the APK runs on: 8.0. targetSDK is the
// one it is built for. Android 15 makes an app built for it draw under
// the system bars, which the driver leaves to the system for now.
const (
	minSDK    = 26
	targetSDK = 34
)

// abis maps an Android ABI to its GOARCH and its clang target.
var abis = map[string]struct{ goarch, clang string }{
	"arm64-v8a":   {"arm64", "aarch64-linux-android"},
	"x86_64":      {"amd64", "x86_64-linux-android"},
	"armeabi-v7a": {"arm", "armv7a-linux-androideabi"},
	"x86":         {"386", "i686-linux-android"},
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("gunimapk: ")
	out := flag.String("o", "", "the APK to write; the package's name and .apk by default")
	id := flag.String("id", "", "the application ID; org.gunim.<name> by default")
	name := flag.String("name", "", "the application's label; the package's name by default")
	iconPNG := flag.String("icon", "", "a PNG file for the launcher's icon, square, 192 pixels or more")
	abiList := flag.String("abi", "arm64-v8a,x86_64", "the ABIs to build for, comma separated")
	install := flag.Bool("install", false, "install the APK with adb")
	run := flag.Bool("run", false, "install the APK with adb and start it")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "usage: gunimapk [flags] package\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	pkg := flag.Arg(0)
	base := filepath.Base(pkg)
	if base == "." || base == "/" {
		wd, _ := os.Getwd()
		base = filepath.Base(wd)
	}
	if *out == "" {
		*out = base + ".apk"
	}
	if *name == "" {
		*name = base
	}
	if *id == "" {
		*id = "org.gunim." + regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(strings.ToLower(base), "_")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := buildAPK(ctx, pkg, *out, *id, *name, *iconPNG, strings.Split(*abiList, ","), *install || *run, *run)
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// buildAPK builds the APK for pkg into out, installs it when install
// is set and starts it when start is, and removes its working directory
// whatever happens. Interrupting ctx stops the tool running.
func buildAPK(ctx context.Context, pkg, out, id, name, iconPNG string, abiNames []string, install, start bool) error {
	b, err := newBuilder(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(b.tmp) }()
	if err := b.build(pkg, out, id, name, iconPNG, abiNames); err != nil {
		return err
	}
	adb := filepath.Join(b.sdk, "platform-tools", "adb")
	if install {
		if err := b.tool(adb, "install", "-r", out); err != nil {
			return err
		}
	}
	if start {
		return b.tool(adb, "shell", "am", "start", "-n", id+"/gunim.android.GunimActivity")
	}
	return nil
}

// builder holds where the tools are, and the directory it works in.
// Cancelling ctx stops the tool running.
type builder struct {
	ctx                                       context.Context
	sdk, ndk, buildTools, androidJar, javaBin string
	tmp                                       string
}

func newBuilder(ctx context.Context) (*builder, error) {
	b := &builder{ctx: ctx}
	for _, dir := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"), home("Android", "sdk")} {
		if dir != "" && exists(filepath.Join(dir, "platforms")) {
			b.sdk = dir
			break
		}
	}
	if b.sdk == "" {
		return nil, errors.New("no Android SDK: set ANDROID_HOME")
	}
	var err error
	if b.ndk = os.Getenv("ANDROID_NDK_HOME"); b.ndk == "" {
		if b.ndk, err = newest(filepath.Join(b.sdk, "ndk")); err != nil {
			return nil, fmt.Errorf("no NDK: %w", err)
		}
	}
	if b.buildTools, err = newest(filepath.Join(b.sdk, "build-tools")); err != nil {
		return nil, fmt.Errorf("no build tools: %w", err)
	}
	platform, err := newest(filepath.Join(b.sdk, "platforms"))
	if err != nil {
		return nil, fmt.Errorf("no platform: %w", err)
	}
	b.androidJar = filepath.Join(platform, "android.jar")
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		b.javaBin = filepath.Join(jh, "bin")
	} else if javac, lookErr := exec.LookPath("javac"); lookErr == nil {
		b.javaBin = filepath.Dir(javac)
	} else {
		return nil, errors.New("no Java: set JAVA_HOME")
	}
	if b.tmp, err = os.MkdirTemp("", "gunimapk"); err != nil {
		return nil, err
	}
	return b, nil
}

// build writes the APK for pkg to out.
func (b *builder) build(pkg, out, id, name, iconPNG string, abiNames []string) error {
	var libs []string
	for _, abi := range abiNames {
		lib, err := b.goLib(pkg, abi)
		if err != nil {
			return err
		}
		libs = append(libs, lib)
	}
	dex, dexErr := b.dex()
	if dexErr != nil {
		return dexErr
	}
	manifest := filepath.Join(b.tmp, "AndroidManifest.xml")
	if err := os.WriteFile(manifest, []byte(manifestFor(id, name, iconPNG != "")), 0o644); err != nil {
		return err
	}
	var res []string
	if iconPNG != "" {
		compiled, iconErr := b.icon(iconPNG)
		if iconErr != nil {
			return iconErr
		}
		res = append(res, compiled)
	}
	linked := filepath.Join(b.tmp, "linked.apk")
	args := append([]string{"link", "-o", linked, "-I", b.androidJar,
		"--manifest", manifest, "--min-sdk-version", strconv.Itoa(minSDK),
		"--target-sdk-version", strconv.Itoa(targetSDK), "--version-code", strconv.FormatInt(time.Now().Unix()/60, 10),
		"--version-name", versionOf(b.ctx, pkg), "--debug-mode"}, res...)
	if err := b.tool(filepath.Join(b.buildTools, "aapt2"), args...); err != nil {
		return err
	}
	unaligned := filepath.Join(b.tmp, "unaligned.apk")
	if err := pack(unaligned, linked, dex, libs, b.tmp); err != nil {
		return err
	}
	aligned := filepath.Join(b.tmp, "aligned.apk")
	if err := b.tool(filepath.Join(b.buildTools, "zipalign"), "-f", "-p", "4", unaligned, aligned); err != nil {
		return err
	}
	ks, keyErr := b.debugKey()
	if keyErr != nil {
		return keyErr
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return b.tool(filepath.Join(b.buildTools, "apksigner"), "sign", "--ks", ks, "--ks-pass", "pass:android",
		"--key-pass", "pass:android", "--out", out, aligned)
}

// goLib builds pkg as lib/<abi>/libgunim.so under the working directory.
func (b *builder) goLib(pkg, abi string) (string, error) {
	a, ok := abis[abi]
	if !ok {
		return "", fmt.Errorf("unknown ABI %q", abi)
	}
	host := runtime.GOOS + "-x86_64"
	cc := filepath.Join(b.ndk, "toolchains", "llvm", "prebuilt", host, "bin", fmt.Sprintf("%s%d-clang", a.clang, minSDK))
	lib := filepath.Join(b.tmp, "lib", abi, "libgunim.so")
	cmd := exec.CommandContext(b.ctx, "go", "build", "-buildmode=c-shared", "-trimpath", "-ldflags=-s -w", "-o", lib, pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GOOS=android", "GOARCH="+a.goarch, "CC="+cc, "CXX="+cc+"++")
	if a.goarch == "arm" {
		cmd.Env = append(cmd.Env, "GOARM=7")
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build %s for %s: %w", pkg, abi, err)
	}
	return lib, nil
}

// dex compiles the Java half of the driver into classes.dex.
func (b *builder) dex() (string, error) {
	src := filepath.Join(b.tmp, "java")
	classes := filepath.Join(b.tmp, "classes")
	dexDir := filepath.Join(b.tmp, "dex")
	for _, d := range []string{src, classes, dexDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	files, globErr := fs.Glob(java.Sources, "*.java")
	if globErr != nil {
		return "", globErr
	}
	args := []string{"--release", "11", "-nowarn", "-classpath", b.androidJar, "-d", classes}
	for _, f := range files {
		data, readErr := java.Sources.ReadFile(f)
		if readErr != nil {
			return "", readErr
		}
		path := filepath.Join(src, f)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
		args = append(args, path)
	}
	if err := b.tool(filepath.Join(b.javaBin, "javac"), args...); err != nil {
		return "", err
	}
	var compiled []string
	walkErr := filepath.WalkDir(classes, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".class") {
			compiled = append(compiled, p)
		}
		return err
	})
	if walkErr != nil {
		return "", walkErr
	}
	args = append([]string{"--release", "--min-api", strconv.Itoa(minSDK), "--lib", b.androidJar, "--output", dexDir}, compiled...)
	if err := b.tool(filepath.Join(b.buildTools, "d8"), args...); err != nil {
		return "", err
	}
	return filepath.Join(dexDir, "classes.dex"), nil
}

// icon compiles the launcher's icon from a PNG, as a resource for aapt2
// to link, at the screen density a large icon is drawn for.
func (b *builder) icon(png string) (string, error) {
	dir := filepath.Join(b.tmp, "res", "mipmap-xxxhdpi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, readErr := os.ReadFile(png)
	if readErr != nil {
		return "", fmt.Errorf("icon: %w", readErr)
	}
	if err := os.WriteFile(filepath.Join(dir, "ic_launcher.png"), data, 0o644); err != nil {
		return "", err
	}
	out := filepath.Join(b.tmp, "res.zip")
	return out, b.tool(filepath.Join(b.buildTools, "aapt2"), "compile", "--dir", filepath.Join(b.tmp, "res"), "-o", out)
}

// versionOf names a build: the commit of the package's repository,
// with -dirty for edits since, and the time it was built, as
// "fd8f5fa-dirty 2026-10-03 11:30". A debug build shows it as it
// starts, so a phone says which build it runs.
func versionOf(ctx context.Context, pkg string) string {
	at := time.Now().Format("2006-01-02 15:04")
	dir, err := exec.CommandContext(ctx, "go", "list", "-f", "{{.Dir}}", pkg).Output()
	if err != nil {
		return at
	}
	commit, err := exec.CommandContext(ctx, "git", "-C", strings.TrimSpace(string(dir)), "describe", "--always", "--dirty").Output()
	if err != nil {
		return at
	}
	return strings.TrimSpace(string(commit)) + " " + at
}

// debugKey returns the debug keystore, which it makes as Android
// Studio would where there is none.
func (b *builder) debugKey() (string, error) {
	ks := home(".android", "debug.keystore")
	if exists(ks) {
		return ks, nil
	}
	if err := os.MkdirAll(filepath.Dir(ks), 0o755); err != nil {
		return "", err
	}
	err := b.tool(filepath.Join(b.javaBin, "keytool"), "-genkeypair", "-keystore", ks, "-storepass", "android",
		"-alias", "androiddebugkey", "-keypass", "android", "-keyalg", "RSA", "-keysize", "2048",
		"-validity", "10000", "-dname", "CN=Android Debug,O=Android,C=US")
	return ks, err
}

// tool runs a tool, with Java on the PATH for the tools that are Java
// programs.
func (b *builder) tool(name string, args ...string) error {
	cmd := exec.CommandContext(b.ctx, name, args...)
	cmd.Env = append(os.Environ(), "PATH="+b.javaBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if filepath.IsAbs(b.javaBin) {
		cmd.Env = append(cmd.Env, "JAVA_HOME="+filepath.Dir(b.javaBin))
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return nil
}

// pack writes the APK: what aapt2 linked, the dex, and the libraries.
func pack(out, linked, dex string, libs []string, root string) (err error) {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	zw := zip.NewWriter(f)
	zr, err := zip.OpenReader(linked)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, e := range zr.File {
		if err := zw.Copy(e); err != nil {
			return err
		}
	}
	add := func(name, path string) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		r, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()
		_, err = io.Copy(w, r)
		return err
	}
	if err := add("classes.dex", dex); err != nil {
		return err
	}
	for _, lib := range libs {
		rel, err := filepath.Rel(root, lib)
		if err != nil {
			return err
		}
		if err := add(filepath.ToSlash(rel), lib); err != nil {
			return err
		}
	}
	return zw.Close()
}

// manifestFor returns the manifest of an APK that starts gunim's
// activity, with the launcher's icon from the resources when icon is
// set. The activity keeps itself across rotation and a keyboard
// coming and going, and slides up as the soft keyboard opens, to keep the
// text caret above it.
func manifestFor(id, name string, icon bool) string {
	iconAttr := ""
	if icon {
		iconAttr = ` android:icon="@mipmap/ic_launcher"`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="` + id + `">
	<application android:label="` + xmlEscape(name) + `"` + iconAttr + ` android:hasCode="true" android:extractNativeLibs="true">
		<activity android:name="gunim.android.GunimActivity" android:exported="true"
			android:configChanges="orientation|screenSize|screenLayout|smallestScreenSize|keyboard|keyboardHidden|navigation|uiMode|density"
			android:windowSoftInputMode="adjustPan|stateAlwaysHidden"
			android:theme="@android:style/Theme.DeviceDefault.NoActionBar">
			<intent-filter>
				<action android:name="android.intent.action.MAIN"/>
				<category android:name="android.intent.category.LAUNCHER"/>
			</intent-filter>
		</activity>
	</application>
</manifest>
`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// newest returns the subdirectory of dir whose name sorts last as a
// version.
func newest(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("%s is empty", dir)
	}
	slices.SortFunc(names, compareVersions)
	return filepath.Join(dir, names[len(names)-1]), nil
}

// compareVersions orders names such as 29.0.14206865 and android-36 by
// the numbers in them.
func compareVersions(a, b string) int {
	num := regexp.MustCompile(`\d+`)
	x, y := num.FindAllString(a, -1), num.FindAllString(b, -1)
	for i := 0; i < len(x) && i < len(y); i++ {
		n, _ := strconv.Atoi(x[i])
		m, _ := strconv.Atoi(y[i])
		if n != m {
			return n - m
		}
	}
	return len(x) - len(y)
}

func home(parts ...string) string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(append([]string{h}, parts...)...)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
