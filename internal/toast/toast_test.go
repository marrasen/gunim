package toast

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf8"
)

// odd are tags with every kind of text that could break the XML or the launch argument.
var odd = []string{
	"",
	"chat:42",
	`<toast launch="x">&amp;</toast>`,
	`"quoted" and 'single' & <angle> %41 %% %`,
	"line\nbreak\ttab\rreturn",
	"nul\x00inside\x01\x1f\x7f",
	"日本語 🎉 ünïcödé \u200d\ufeff",
	"\uffff\ufffe",
	"broken \xff\xfe utf-8 \xc3",
	"gunim-tag:other:%ZZ",
	strings.Repeat("🎉", 256),
}

// parsed is what a toast's XML says, as an XML reader reads it.
type parsed struct {
	Launch string   `xml:"launch,attr"`
	Texts  []string `xml:"visual>binding>text"`
}

func parse(t *testing.T, s string) parsed {
	t.Helper()
	var p parsed
	if err := xml.Unmarshal([]byte(s), &p); err != nil {
		t.Fatalf("the toast's XML does not parse: %v\n%s", err, s)
	}
	return p
}

// Every tag comes back whole from the launch argument, read out of the toast's XML by an XML reader.
func TestATagComesBackWholeThroughTheToastsXML(t *testing.T) {
	for _, tag := range odd {
		launch := Launch("1a-2", tag)
		p := parse(t, XML("title", "body", launch))
		if p.Launch != launch {
			t.Fatalf("the XML's launch attribute reads %q, want %q", p.Launch, launch)
		}
		id, got, ok := ParseLaunch(p.Launch)
		if !ok || id != "1a-2" || got != tag {
			t.Fatalf("ParseLaunch(%q) = %q, %q, %v; want 1a-2, %q, true", p.Launch, id, got, ok, tag)
		}
	}
}

// The launch argument holds no byte XML would escape or change, and is at most three times as long as the tag.
func TestALaunchArgumentIsPlainText(t *testing.T) {
	for _, tag := range odd {
		launch := Launch("7", tag)
		if !utf8.ValidString(launch) {
			t.Fatalf("Launch(%q) = %q, broken UTF-8", tag, launch)
		}
		for _, r := range launch {
			if needsEscape(r) && r != '%' {
				t.Fatalf("Launch(%q) = %q holds %q", tag, launch, r)
			}
		}
		if n := len(launch) - len(launchPrefix+"7:"); n > 3*len(tag) {
			t.Fatalf("Launch(%q) encodes %d bytes as %d", tag, len(tag), n)
		}
	}
}

// Text the application shows reaches the toast as it is, less what XML cannot hold.
func TestTheTitleAndBodyReachTheToast(t *testing.T) {
	title := `Tom & "Jerry" <b>bold</b> 'quote' 🎉`
	body := "line one\nline two\ttab\x00\x01gone\uffff"
	p := parse(t, XML(title, body, Launch("1", "x")))
	if len(p.Texts) != 2 || p.Texts[0] != title || p.Texts[1] != "line one\nline two\ttabgone" {
		t.Fatalf("the toast's text reads %q", p.Texts)
	}
	if s := XML("a", "b", "c"); !strings.Contains(s, `template="ToastGeneric"`) || !strings.Contains(s, `activationType="foreground"`) {
		t.Fatalf("the toast is %s", s)
	}
}

// A launch argument of anything but Launch's reads as none.
func TestParseLaunchTurnsAwayOtherArguments(t *testing.T) {
	for _, s := range []string{
		"", "plain", "gunim-tag:", "gunim-tag:noseparator", "gunim-tag::empty-id", "gunim-tag:UPPER:x",
		"gunim-tag:a b:x", "gunim-tag:" + strings.Repeat("1", MaxID+1) + ":x", "gunim-tag:1:%", "gunim-tag:1:%4",
		"gunim-tag:1:%GG", "other:1:x",
	} {
		if id, tag, ok := ParseLaunch(s); ok {
			t.Errorf("ParseLaunch(%q) = %q, %q, true; want false", s, id, tag)
		}
	}
	if _, tag, ok := ParseLaunch("gunim-tag:1:%3c%3E"); !ok || tag != "<>" {
		t.Errorf("ParseLaunch with lower-case escapes = %q, %v; want <>, true", tag, ok)
	}
}

// An AppUserModelID keeps the safe bytes of the one asked for, and is named after the program where none is, with
// a hash of its path, so two programs of one name in two folders get two.
func TestAppIDIsSafeAndNamedAfterTheProgram(t *testing.T) {
	for _, c := range []struct{ id, exe, want string }{
		{"Example.Notes", `C:\x\notes.exe`, "Example.Notes"},
		{"My App/Ü 2", "", "My-App---2"},
		{strings.Repeat("a", 200), "", strings.Repeat("a", MaxAppID)},
	} {
		if got := AppID(c.id, c.exe); got != c.want {
			t.Errorf("AppID(%q, %q) = %q, want %q", c.id, c.exe, got, c.want)
		}
	}
	a := AppID("", `C:\Program Files\My Tool\my tool.exe`)
	if !strings.HasPrefix(a, "gunim.my-tool.") || len(a) != len("gunim.my-tool.")+8 {
		t.Fatalf("the default AppID is %q, want gunim.my-tool.<8 hex>", a)
	}
	if b := AppID("  ", `C:\Other\my tool.exe`); b == a || !strings.HasPrefix(b, "gunim.my-tool.") {
		t.Fatalf("two folders' programs of one name got %q and %q", a, b)
	}
	if b := AppID("", `c:\program files\my tool\MY TOOL.EXE`); b[len(b)-8:] != a[len(a)-8:] {
		t.Fatalf("one path in another case got %q and %q", a, b)
	}
	if b := AppID("", ""); !strings.HasPrefix(b, "gunim.gunim.") {
		t.Fatalf("with no program file the AppID is %q", b)
	}
}

// COM's -Embedding is taken out of the arguments, wherever it is and in any case, and the program's name stays.
func TestTakeEmbeddingTakesOutCOMsArgument(t *testing.T) {
	args, found := TakeEmbedding([]string{"prog.exe", "-v", "-EMBEDDING", "file", "/Embedding"})
	if !found || strings.Join(args, " ") != "prog.exe -v file" {
		t.Fatalf("TakeEmbedding = %q, %v", args, found)
	}
	if args, found := TakeEmbedding([]string{"-Embedding", "-x"}); found || len(args) != 2 {
		t.Fatalf("the program's own name was taken: %q, %v", args, found)
	}
	if args, found := TakeEmbedding(nil); found || args != nil {
		t.Fatalf("no arguments gave %q, %v", args, found)
	}
}

// A toast's XML stays within MaxXML however long the title and body, with the launch argument whole, the tag at its
// longest and its worst.
func TestAToastsXMLFitsWithTheLaunchArgumentWhole(t *testing.T) {
	worst := strings.Repeat("\x01", 1024)
	launch := Launch(strings.Repeat("a", MaxID), worst)
	for _, c := range []struct{ title, body string }{
		{strings.Repeat(`"`, 5000), strings.Repeat(`&`, 5000)},
		{strings.Repeat("🎉", 5000), strings.Repeat("日", 5000)},
		{"short", strings.Repeat("x", 100000)},
	} {
		s := XML(c.title, c.body, launch)
		if len(s) > MaxXML {
			t.Fatalf("the XML is %d bytes, over %d", len(s), MaxXML)
		}
		p := parse(t, s)
		if p.Launch != launch || len(p.Texts) != 2 || p.Texts[0] == "" {
			t.Fatalf("the launch argument or the title was cut: %d bytes of launch, texts %d", len(p.Launch), len(p.Texts))
		}
	}
	if p := parse(t, XML("title", "body", launch)); p.Texts[1] != "body" {
		t.Fatalf("a short body was cut to %q", p.Texts[1])
	}
}

// The activator's class ID is a version 5 UUID of the AppUserModelID, the same each run and apart for each ID.
func TestActivatorCLSIDIsStablePerAppID(t *testing.T) {
	a, b := ActivatorCLSID("Example.One"), ActivatorCLSID("Example.Two")
	if a != ActivatorCLSID("Example.One") || a == b {
		t.Fatalf("class IDs %s and %s", a, b)
	}
	if len(a) != 36 || a[14] != '5' || !strings.ContainsRune("89AB", rune(a[19])) {
		t.Fatalf("class ID %s is no version 5 UUID", a)
	}
}
