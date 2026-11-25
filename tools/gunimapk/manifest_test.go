package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// manifestDoc is what the tests read from a manifest.
type manifestDoc struct {
	Package string `xml:"package,attr"`
	Uses    []struct {
		Name string `xml:"name,attr"`
	} `xml:"uses-permission"`
	Features []struct {
		Name     string `xml:"name,attr"`
		Required string `xml:"required,attr"`
	} `xml:"uses-feature"`
	App struct {
		Attrs    []xml.Attr `xml:",any,attr"`
		Services []struct {
			Name   string `xml:"name,attr"`
			Filter []struct {
				Action struct {
					Name string `xml:"name,attr"`
				} `xml:"action"`
			} `xml:"intent-filter"`
		} `xml:"service"`
		Receivers []struct {
			Name string `xml:"name,attr"`
		} `xml:"receiver"`
		Providers []struct {
			Name        string `xml:"name,attr"`
			Authorities string `xml:"authorities,attr"`
		} `xml:"provider"`
	} `xml:"application"`
}

// attr returns the application's Android attribute name, or "".
func (d *manifestDoc) attr(name string) string {
	for _, a := range d.App.Attrs {
		if a.Name.Space == androidNS && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func readManifest(t *testing.T, o options) *manifestDoc {
	t.Helper()
	m := manifestFor(o)
	d := &manifestDoc{}
	if err := xml.Unmarshal([]byte(m), d); err != nil {
		t.Fatalf("the manifest is not XML: %v\n%s", err, m)
	}
	return d
}

// ownManifest is a program's own parts, of every kind.
const ownManifest = "\xef\xbb\xbf" + `<?xml version="1.0" encoding="utf-8"?>
<!-- The program's own. -->
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
	<uses-permission android:name="android.permission.NFC"/>
	<queries><package android:name="com.example.other"/></queries>
	<application android:manageSpaceActivity="com.example.notes.Space" android:description='A "note" &amp; more'>
		<!-- A job's service. -->
		<service android:name="com.example.notes.Sync" android:exported="false"
			android:permission="android.permission.BIND_JOB_SERVICE">
			<intent-filter><action android:name="com.example.notes.SYNC"/></intent-filter>
		</service>
		<receiver android:name="com.example.notes.Boot" android:exported="false"/>
		<provider android:name="com.example.notes.Pages" android:exported="false" android:authorities="${applicationId}.pages"/>
	</application>
	<uses-feature android:name="android.hardware.nfc" android:required="false"/>
</manifest>
`

func TestAProgramsOwnPartsJoinTheManifest(t *testing.T) {
	p, err := parseParts([]byte(ownManifest), "com.example.notes.debug")
	if err != nil {
		t.Fatal(err)
	}
	d := readManifest(t, options{id: "com.example.notes.debug", name: "Notes", own: p, perms: []string{"internet", "playback"}})
	uses := make([]string, 0, len(d.Uses))
	for _, u := range d.Uses {
		uses = append(uses, strings.TrimPrefix(u.Name, "android.permission."))
	}
	if want := []string{"VIBRATE", "INTERNET", "FOREGROUND_SERVICE", "FOREGROUND_SERVICE_MEDIA_PLAYBACK", "NFC"}; !slices.Equal(uses, want) {
		t.Errorf("permissions %q, want %q", uses, want)
	}
	if len(d.Features) != 1 || d.Features[0].Name != "android.hardware.nfc" {
		t.Errorf("features %+v", d.Features)
	}
	if got := d.attr("manageSpaceActivity"); got != "com.example.notes.Space" {
		t.Errorf("manageSpaceActivity is %q", got)
	}
	if got, want := d.attr("description"), `A "note" & more`; got != want {
		t.Errorf("description is %q, want %q", got, want)
	}
	if got := d.attr("label"); got != "Notes" {
		t.Errorf("the label is %q", got)
	}
	services := make([]string, 0, len(d.App.Services))
	for _, s := range d.App.Services {
		services = append(services, s.Name)
		if s.Name == "com.example.notes.Sync" && (len(s.Filter) != 1 || s.Filter[0].Action.Name != "com.example.notes.SYNC") {
			t.Errorf("the service's filter is %+v", s.Filter)
		}
	}
	if want := []string{"gunim.android.GunimService", "com.example.notes.Sync"}; !slices.Equal(services, want) {
		t.Errorf("services %q, want %q", services, want)
	}
	if len(d.App.Receivers) != 1 {
		t.Errorf("receivers %+v", d.App.Receivers)
	}
	authorities := make([]string, 0, len(d.App.Providers))
	for _, p := range d.App.Providers {
		authorities = append(authorities, p.Authorities)
	}
	if want := []string{"com.example.notes.debug.pages", "com.example.notes.debug.gunim.files"}; !slices.Equal(authorities, want) {
		t.Errorf("authorities %q, want %q", authorities, want)
	}
}

func TestOwnPartsMayHoldOneKindAlone(t *testing.T) {
	const head = `<manifest xmlns:android="` + androidNS + `">`
	for name, c := range map[string]struct {
		in   string
		want parts
	}{
		"nothing":          {head + `</manifest>`, parts{}},
		"a closed app":     {head + `<application android:manageSpaceActivity="a.B"/></manifest>`, parts{attrs: ` android:manageSpaceActivity="a.B"`}},
		"an empty app":     {head + "<application>\n\t</application></manifest>", parts{}},
		"a permission":     {head + `<uses-permission android:name="a.P"/></manifest>`, parts{beside: "\t<uses-permission android:name=\"a.P\"/>\n"}},
		"an activity":      {head + `<application><activity android:name="a.B"/></application></manifest>`, parts{inside: "\t<activity android:name=\"a.B\"/>\n"}},
		"one after an app": {head + `<application/><uses-permission android:name="a.P"/></manifest>`, parts{beside: "\t<uses-permission android:name=\"a.P\"/>\n"}},
	} {
		got, err := parseParts([]byte(c.in), "a.b")
		if err != nil || got != c.want {
			t.Errorf("%s: %+v, %v, want %+v", name, got, err, c.want)
		}
	}
}

func TestOwnPartsThatAreNotTheProgramsAreRefused(t *testing.T) {
	const head = `<manifest xmlns:android="` + androidNS + `">`
	for name, c := range map[string]struct{ in, says string }{
		"no file's worth":      {"", "EOF"},
		"a file cut short":     {head + `<application>`, "unexpected EOF"},
		"another root":         {`<application xmlns:android="` + androidNS + `"/>`, "not manifest"},
		"a package":            {`<manifest xmlns:android="` + androidNS + `" package="a.b"/>`, "-id"},
		"a version":            {`<manifest xmlns:android="` + androidNS + `" android:versionCode="1"/>`, "versionCode"},
		"another namespace":    {`<manifest xmlns:android="` + androidNS + `" xmlns:tools="http://schemas.android.com/tools"/>`, "tools"},
		"android, misspelt":    {`<manifest xmlns:android="http://example.com"/>`, "android"},
		"no namespace":         {`<manifest><uses-permission android:name="a.P"/></manifest>`, "not Android's"},
		"a bare attribute":     {head + `<uses-permission name="a.P"/></manifest>`, "not Android's"},
		"a namespace inside":   {head + `<uses-permission xmlns:android="http://example.com" android:name="a.P"/></manifest>`, "not Android's"},
		"two applications":     {head + `<application/><application/></manifest>`, "more than one"},
		"uses-sdk":             {head + `<uses-sdk android:minSdkVersion="30"/></manifest>`, "-min-sdk"},
		"backup":               {head + `<application android:allowBackup="true"/></manifest>`, "-backup"},
		"rules":                {head + `<application android:dataExtractionRules="@xml/r"/></manifest>`, "-backup"},
		"the data kept":        {head + `<application android:hasFragileUserData="true"/></manifest>`, "-keep-data"},
		"a debuggable release": {head + `<application android:debuggable="true"/></manifest>`, "-keystore"},
		"gunim's own activity": {head + `<application><activity android:name="gunim.android.GunimActivity"/></application></manifest>`, "gunim's own"},
		"the label":            {head + `<application android:label="A"/></manifest>`, "-name"},
	} {
		if p, err := parseParts([]byte(c.in), "a.b"); err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %+v, %v, want an error that says %q", name, p, err, c.says)
		}
	}
}

func TestEachPermissionAsksForItsOwnAndNoMore(t *testing.T) {
	want := map[string][]string{
		"music":         {"READ_MEDIA_AUDIO", "READ_EXTERNAL_STORAGE"},
		"microphone":    {"RECORD_AUDIO"},
		"camera":        {"CAMERA"},
		"notifications": {"POST_NOTIFICATIONS"},
		"internet":      {"INTERNET"},
		"network-state": {"ACCESS_NETWORK_STATE"},
		"jobs":          {"RUN_USER_INITIATED_JOBS"},
		"boot":          {"RECEIVE_BOOT_COMPLETED"},
		"hide-overlays": {"HIDE_OVERLAY_WINDOWS"},
		"playback":      {"FOREGROUND_SERVICE", "FOREGROUND_SERVICE_MEDIA_PLAYBACK"},
	}
	for _, name := range permissionNames() {
		d := readManifest(t, options{id: "org.gunim.calc", name: "Calculator", perms: []string{name}})
		var got []string
		for _, u := range d.Uses {
			got = append(got, strings.TrimPrefix(u.Name, "android.permission."))
		}
		if w := append([]string{"VIBRATE"}, want[name]...); !slices.Equal(got, w) {
			t.Errorf("%s asks for %q, want %q", name, got, w)
		}
		for _, f := range d.Features {
			if f.Required != "false" {
				t.Errorf("%s needs a device with %s", name, f.Name)
			}
		}
	}
	if len(want) != len(permissions) {
		t.Errorf("%d permissions, and the test knows %d", len(permissions), len(want))
	}
}

func TestADebugBuildHasItsOwnIDWhereItIsGivenOne(t *testing.T) {
	for _, c := range []struct {
		id, debugID string
		release     bool
		want        string
	}{
		{"a.b", "", false, "a.b"},
		{"a.b", "", true, "a.b"},
		{"a.b", "a.b.debug", false, "a.b.debug"},
		{"a.b", "a.b.debug", true, "a.b"},
	} {
		if got := appID(c.id, c.debugID, c.release); got != c.want {
			t.Errorf("-id %s -debug-id %q, release %v: %s, want %s", c.id, c.debugID, c.release, got, c.want)
		}
	}
}

func TestTheCompilerIsTheNewestTheNDKHasForAnAndroid(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"x-android26-clang", "x-android34-clang", "x-android35-clang"} {
		if err := os.WriteFile(filepath.Join(bin, name), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for level, want := range map[int]string{26: "x-android26-clang", 34: "x-android34-clang", 36: "x-android35-clang"} {
		if got := filepath.Base(clang(bin, "x-android", level)); got != want {
			t.Errorf("for Android %d: %s, want %s", level, got, want)
		}
	}
}

func TestBackupsAreLeftOnUnlessTurnedOff(t *testing.T) {
	d := readManifest(t, options{id: "org.gunim.calc", name: "Calculator"})
	for _, a := range []string{"allowBackup", "dataExtractionRules", "hasFragileUserData"} {
		if got := d.attr(a); got != "" {
			t.Errorf("with no flag, %s is %q", a, got)
		}
	}
	d = readManifest(t, options{id: "org.gunim.calc", name: "Calculator", noBackup: true, keepData: true})
	for a, want := range map[string]string{"allowBackup": "false", "dataExtractionRules": "@xml/" + rulesName, "hasFragileUserData": "true"} {
		if got := d.attr(a); got != want {
			t.Errorf("%s is %q, want %q", a, got, want)
		}
	}
}

// Each way Android carries files off the device needs rules of its own,
// as one without them takes everything, and no rule may include
// anything, as Android then takes that. A transfer to another kind of
// device is off while the rules name no program there.
func TestTheRulesLeaveOutEverythingFromEveryWay(t *testing.T) {
	type rule struct {
		XMLName xml.Name
		Domain  string `xml:"domain,attr"`
		Path    string `xml:"path,attr"`
	}
	var doc struct {
		XMLName xml.Name
		Ways    []struct {
			XMLName  xml.Name
			Platform string `xml:"platform,attr"`
			Rules    []rule `xml:",any"`
		} `xml:",any"`
	}
	if err := xml.Unmarshal([]byte(noBackupRules()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.XMLName.Local != "data-extraction-rules" {
		t.Errorf("the root is %s", doc.XMLName.Local)
	}
	platforms := map[string]string{"cloud-backup": "", "device-transfer": "", "cross-platform-transfer": "ios"}
	if len(doc.Ways) != len(platforms) {
		t.Errorf("%d ways, want %d", len(doc.Ways), len(platforms))
	}
	for _, w := range doc.Ways {
		name := w.XMLName.Local
		if platform, ok := platforms[name]; !ok || platform != w.Platform {
			t.Errorf("%s to platform %q", name, w.Platform)
		}
		left := slices.Clone(domains)
		for _, r := range w.Rules {
			if r.XMLName.Local != "exclude" || r.Path != "." {
				t.Errorf("%s has %s of %s", name, r.XMLName.Local, r.Path)
			}
			left = slices.DeleteFunc(left, func(d string) bool { return d == r.Domain })
		}
		if len(left) > 0 {
			t.Errorf("%s leaves in %q", name, left)
		}
	}
}

// element is an element of a manifest or of a file of rules, read back
// from an APK.
type element struct {
	name   string
	parent string
	attrs  map[string]string
}

// readXML reads file back from apk with aapt2, which reads Android's own
// form of it.
func readXML(t *testing.T, b *builder, apk, file string) []element {
	t.Helper()
	out, err := exec.CommandContext(b.ctx, filepath.Join(b.buildTools, "aapt2"), "dump", "xmltree", "--file", file, apk).Output()
	if err != nil {
		t.Fatalf("reading %s from %s: %v", file, apk, err)
	}
	var all []element
	// open is the elements above, with how far in each stands.
	type above struct {
		in   int
		name string
	}
	var open []above
	for l := range strings.Lines(string(out)) {
		l = strings.TrimRight(l, "\r\n")
		text := strings.TrimLeft(l, " ")
		in := len(l) - len(text)
		kind, rest, _ := strings.Cut(text, ": ")
		switch kind {
		case "E":
			name, _, _ := strings.Cut(rest, " ")
			for len(open) > 0 && open[len(open)-1].in >= in {
				open = open[:len(open)-1]
			}
			e := element{name: name, attrs: map[string]string{}}
			if len(open) > 0 {
				e.parent = open[len(open)-1].name
			}
			open = append(open, above{in, name})
			all = append(all, e)
		case "A":
			rest = strings.TrimPrefix(rest, androidNS+":")
			name, value, _ := strings.Cut(rest, "=")
			name, _, _ = strings.Cut(name, "(")
			value, _, _ = strings.Cut(value, " (Raw: ")
			all[len(all)-1].attrs[name] = strings.Trim(value, `"`)
		}
	}
	return all
}

// find returns the element name whose attribute a is v, or the first
// named so where a is "".
func find(all []element, name, a, v string) *element {
	for i := range all {
		if all[i].name == name && (a == "" || all[i].attrs[a] == v) {
			return &all[i]
		}
	}
	return nil
}

// sdk returns a builder, or skips the test where there is no SDK to
// build with. GUNIMAPK_TEST=build fails it instead.
func sdk(t *testing.T) *builder {
	t.Helper()
	b, err := newBuilder(context.Background())
	if err != nil {
		if os.Getenv("GUNIMAPK_TEST") == "build" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(b.tmp) })
	return b
}

// checkAPK reports what apk's own manifest and rules lack of o, whose
// own parts are ownManifest's.
func checkAPK(t *testing.T, b *builder, apk string, o options) {
	t.Helper()
	m := readXML(t, b, apk, "AndroidManifest.xml")
	check := func(element, key, value, attr, want string) {
		t.Helper()
		e := find(m, element, key, value)
		if e == nil {
			t.Errorf("the APK has no %s with %s %q", element, key, value)
			return
		}
		if got := e.attrs[attr]; got != want {
			t.Errorf("the APK's %s %s has %s %q, want %q", element, value, attr, got, want)
		}
	}
	check("manifest", "", "", "package", o.id)
	check("uses-sdk", "", "", "minSdkVersion", "34")
	check("uses-sdk", "", "", "targetSdkVersion", "36")
	check("application", "", "", "allowBackup", "false")
	check("application", "", "", "hasFragileUserData", "true")
	check("application", "", "", "manageSpaceActivity", "com.example.notes.Space")
	debuggable := "true"
	if o.release() {
		debuggable = ""
	}
	check("application", "", "", "debuggable", debuggable)
	for _, p := range []string{"VIBRATE", "INTERNET", "RUN_USER_INITIATED_JOBS", "NFC"} {
		check("uses-permission", "name", "android.permission."+p, "name", "android.permission."+p)
	}
	check("service", "name", "com.example.notes.Sync", "exported", "false")
	check("service", "name", "com.example.notes.Sync", "permission", "android.permission.BIND_JOB_SERVICE")
	check("provider", "name", "com.example.notes.Pages", "authorities", o.id+".pages")
	check("provider", "name", "gunim.android.GunimFiles", "authorities", o.id+".gunim.files")
	if e := find(m, "service", "name", "com.example.notes.Sync"); e != nil && e.parent != "application" {
		t.Errorf("the program's service is in %s", e.parent)
	}
	if app := find(m, "application", "", ""); app != nil && !strings.HasPrefix(app.attrs["dataExtractionRules"], "@0x") {
		t.Errorf("the APK's dataExtractionRules is %q, and no resource", app.attrs["dataExtractionRules"])
	}

	rules := readXML(t, b, apk, "res/xml/"+rulesName+".xml")
	ways := map[string][]string{}
	for _, e := range rules {
		switch {
		case e.name == "exclude" && e.attrs["path"] == ".":
			ways[e.parent] = append(ways[e.parent], e.attrs["domain"])
		case e.parent == "" || e.parent == "data-extraction-rules":
		default:
			t.Errorf("the APK's rules hold %s in %s", e.name, e.parent)
		}
	}
	for _, way := range []string{"cloud-backup", "device-transfer", "cross-platform-transfer"} {
		if !slices.Equal(ways[way], domains) {
			t.Errorf("the APK's rules leave %q out of %s, want %q", ways[way], way, domains)
		}
	}
	if e := find(rules, "cross-platform-transfer", "", ""); e == nil || e.attrs["platform"] != "ios" {
		t.Errorf("the APK's rules for another kind of device are %+v", e)
	}
}

// ownJava writes a program's own service, which boots the program as a
// component of its own does, and returns its files.
func ownJava(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "com", "example", "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const src = `package com.example.notes;

public class Sync extends android.app.Service {
	@Override
	public void onCreate() {
		gunim.android.Native.boot(this);
	}

	@Override
	public android.os.IBinder onBind(android.content.Intent i) {
		return null;
	}
}
`
	for name, text := range map[string]string{"Sync.java": src, "notes.txt": "not Java"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := javaFiles(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestJavaFilesAreFoundWhereverTheyStand(t *testing.T) {
	files := ownJava(t)
	if len(files) != 1 || filepath.Base(files[0]) != "Sync.java" {
		t.Errorf("found %q", files)
	}
	if files, err := javaFiles(""); err != nil || files != nil {
		t.Errorf("with no directory: %q, %v", files, err)
	}
	if _, err := javaFiles(t.TempDir()); err == nil {
		t.Error("a directory with no Java passes")
	}
	if _, err := javaFiles(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("a directory that is gone passes")
	}
}

// askingOptions asks for every part checkAPK looks for.
func askingOptions(t *testing.T, id string) options {
	t.Helper()
	own, err := parseParts([]byte(strings.Replace(ownManifest, ` android:description='A "note" &amp; more'`, "", 1)), id)
	if err != nil {
		t.Fatal(err)
	}
	return options{pkg: "github.com/marrasen/gunim/example/backup", id: id, name: "Notes", version: "1", own: own, minSDK: 34,
		noBackup: true, keepData: true, perms: []string{"internet", "jobs"}, abis: []string{"x86_64"}}
}

func TestTheManifestAnAPKHoldsSaysWhatWasAsked(t *testing.T) {
	for _, keystore := range []string{"", "k.jks"} {
		b := sdk(t)
		o := askingOptions(t, "com.example.notes")
		o.keystore = keystore
		args, err := b.linkArgs(o)
		if err != nil {
			t.Fatal(err)
		}
		apk := filepath.Join(b.tmp, "linked.apk")
		if err := b.tool(filepath.Join(b.buildTools, "aapt2"), append([]string{"link", "-o", apk}, args...)...); err != nil {
			t.Fatal(err)
		}
		checkAPK(t, b, apk, o)
	}
}

// A whole APK takes a minute to build, so this runs only with
// GUNIMAPK_TEST=build, as CI sets it.
func TestAWholeAPKHoldsTheProgramAndItsManifest(t *testing.T) {
	if os.Getenv("GUNIMAPK_TEST") != "build" {
		t.Skip("GUNIMAPK_TEST=build builds a whole APK")
	}
	b := sdk(t)
	o := askingOptions(t, "com.example.notes.debug")
	o.out = filepath.Join(t.TempDir(), "notes.apk")
	o.java = ownJava(t)
	if err := buildAPK(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	checkAPK(t, b, o.out, o)
	z, err := zip.OpenReader(o.out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = z.Close() }()
	names := make([]string, 0, len(z.File))
	for _, f := range z.File {
		names = append(names, f.Name)
		if f.Name != "classes.dex" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		dex, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, class := range []string{"Lgunim/android/GunimActivity;", "Lcom/example/notes/Sync;"} {
			if !bytes.Contains(dex, []byte(class)) {
				t.Errorf("the APK's classes have no %s", class)
			}
		}
	}
	for _, want := range []string{"classes.dex", "lib/x86_64/libgunim.so", "resources.arsc"} {
		if !slices.Contains(names, want) {
			t.Errorf("the APK holds %q, and no %s", names, want)
		}
	}
	out, err := exec.CommandContext(b.ctx, filepath.Join(b.buildTools, "zipalign"), "-c", "-P", "16", "4", o.out).CombinedOutput()
	if err != nil {
		t.Errorf("the APK is not aligned for 16 KB pages: %v\n%s", err, bytes.TrimSpace(out))
	}
}
