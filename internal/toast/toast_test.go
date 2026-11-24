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

// An AppUserModelID keeps the safe bytes of the one asked for, and is named after the program where none is.
func TestAppIDIsSafeAndNamedAfterTheProgram(t *testing.T) {
	for _, c := range []struct{ id, exe, want string }{
		{"Example.Notes", `C:\x\notes.exe`, "Example.Notes"},
		{"My App/Ü 2", "", "My-App---2"},
		{"", `C:\Program Files\My Tool\my tool.exe`, "gunim.my-tool"},
		{"", "", "gunim.gunim"},
		{"  ", "/tmp/go-build/b001/exe/demo", "gunim.demo"},
		{strings.Repeat("a", 200), "", strings.Repeat("a", MaxAppID)},
	} {
		if got := AppID(c.id, c.exe); got != c.want {
			t.Errorf("AppID(%q, %q) = %q, want %q", c.id, c.exe, got, c.want)
		}
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
