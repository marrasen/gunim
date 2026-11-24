// Package toast builds what a Windows toast notification carries: its XML, and the launch argument that brings a
// message's tag back with a click. It is plain Go, so it builds and tests on every platform; driver/desktop shows
// the toasts on Windows.
//
// A tag travels in the toast's launch attribute, which Windows hands back as the click's arguments. The tag is
// percent-encoded first: every byte the XML would need to escape, or could not hold at all, becomes %XX, and so does
// the percent sign. Any tag survives the trip whole, NUL and broken UTF-8 included, and the encoded text is at most
// three times as long as the tag.
package toast

import (
	"crypto/sha1" //nolint:gosec // a name-based UUID, as RFC 9562's version 5 has it
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// launchPrefix starts every launch argument gunim writes, so a click on another program's toast, or a launch
// argument of the application's own, is told apart.
const launchPrefix = "gunim-tag:"

// MaxID is the most bytes a toast's ID holds in its launch argument.
const MaxID = 64

// Launch returns the launch argument for a toast with the ID id and the tag tag. The ID names the toast, so two
// reports of one click are heard once; it holds the bytes 0-9, a-z and '-' alone, at most MaxID of them.
func Launch(id, tag string) string {
	var b strings.Builder
	b.Grow(len(launchPrefix) + len(id) + 1 + len(tag))
	b.WriteString(launchPrefix)
	b.WriteString(id)
	b.WriteByte(':')
	escapeTag(&b, tag)
	return b.String()
}

// ParseLaunch reads a launch argument Launch wrote, and reports whether it was one.
func ParseLaunch(s string) (id, tag string, ok bool) {
	rest, found := strings.CutPrefix(s, launchPrefix)
	if !found {
		return "", "", false
	}
	id, enc, found := strings.Cut(rest, ":")
	if !found || !validID(id) {
		return "", "", false
	}
	tag, ok = unescapeTag(enc)
	return id, tag, ok
}

// validID reports whether id is an ID Launch takes.
func validID(id string) bool {
	if id == "" || len(id) > MaxID {
		return false
	}
	for i := range len(id) {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') && c != '-' {
			return false
		}
	}
	return true
}

// escapeTag writes tag with every byte the XML could not hold as it is written as %XX.
func escapeTag(b *strings.Builder, tag string) {
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(tag); {
		r, n := utf8.DecodeRuneInString(tag[i:])
		if (r == utf8.RuneError && n <= 1) || needsEscape(r) {
			for _, c := range []byte(tag[i : i+n]) {
				b.WriteByte('%')
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xf])
			}
		} else {
			b.WriteString(tag[i : i+n])
		}
		i += n
	}
}

// needsEscape reports whether r is escaped in a tag: XML's markup, the percent sign, and what XML cannot hold or
// would change, as an attribute's line breaks and tabs become spaces.
func needsEscape(r rune) bool {
	switch r {
	case '%', '&', '<', '>', '"', '\'':
		return true
	}
	return r < 0x20 || r == 0x7f || r == 0xfffe || r == 0xffff
}

// unescapeTag undoes escapeTag, and reports whether s was what it writes.
func unescapeTag(s string) (string, bool) {
	if !strings.Contains(s, "%") {
		return s, true
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			out = append(out, s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", false
		}
		hi, ok1 := unhex(s[i+1])
		lo, ok2 := unhex(s[i+2])
		if !ok1 || !ok2 {
			return "", false
		}
		out = append(out, hi<<4|lo)
		i += 2
	}
	return string(out), true
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

// MaxXML is the most bytes a toast's XML holds: Windows turns away a toast of more than 5 KB.
const MaxXML = 5000

// The most runes of a title and a body a toast shows; a toast shows a few lines of each, and longer text is cut.
const (
	maxTitle = 128
	maxBody  = 1024
)

// XML returns the toast's content: title and body as its two lines of text, and launch as what a click on it hands
// back. Text XML cannot hold is left out of title and body, and the rest is escaped. Title and body are cut, the body
// first, so the whole stays within MaxXML bytes; the launch argument stays whole, and one Launch wrote always fits.
func XML(title, body, launch string) string {
	t, b := []rune(title), []rune(body)
	t, b = t[:min(len(t), maxTitle)], b[:min(len(b), maxBody)]
	for {
		s := xmlOf(string(t), string(b), launch)
		if len(s) <= MaxXML || (len(t) == 0 && len(b) == 0) {
			return s
		}
		// Cut by the bytes over, at least one rune at a time.
		over := max(1, (len(s)-MaxXML)/6)
		if len(b) > 0 {
			b = b[:max(0, len(b)-over)]
		} else {
			t = t[:max(0, len(t)-over)]
		}
	}
}

func xmlOf(title, body, launch string) string {
	var b strings.Builder
	b.WriteString(`<toast launch="`)
	escapeXML(&b, launch)
	b.WriteString(`" activationType="foreground"><visual><binding template="ToastGeneric"><text>`)
	escapeXML(&b, title)
	b.WriteString(`</text><text>`)
	escapeXML(&b, body)
	b.WriteString(`</text></binding></visual></toast>`)
	return b.String()
}

// Clean returns s less what neither XML nor a message bus can hold: control characters other than tab and line
// breaks, U+FFFE and U+FFFF go, and broken UTF-8 becomes U+FFFD.
func Clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 0x20 || r == '\t' || r == '\n' || r == '\r') && r != 0xfffe && r != 0xffff {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeXML writes s, cleaned, escaped for XML text or an attribute in double quotes.
func escapeXML(b *strings.Builder, s string) {
	for _, r := range Clean(s) {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t', '\n', '\r':
			fmt.Fprintf(b, "&#x%X;", r)
		default:
			b.WriteRune(r)
		}
	}
}

// MaxAppID is the most characters an AppUserModelID holds.
const MaxAppID = 128

// AppID returns id made fit to be an AppUserModelID: the bytes A-Z, a-z, 0-9, '.', '-' and '_' kept, every other
// rune made '-', and cut at MaxAppID. An id empty after that is named after the program file exe, as
// "gunim.<name>.<hash>", where hash is 8 hex digits of a hash of exe's full path: two programs of one file name in
// two folders get two IDs, and a program moved to another folder gets a new one. A long name is cut, and the hash
// stays whole.
func AppID(id, exe string) string {
	out := cleanID(id)
	if strings.Trim(out, "-_") == "" {
		sum := sha256.Sum256([]byte(strings.ToLower(exe)))
		name := cleanID(ProgramName(exe))
		name = name[:min(len(name), MaxAppID-len("gunim..01234567"))]
		out = fmt.Sprintf("gunim.%s.%x", name, sum[:4])
	}
	if len(out) > MaxAppID {
		out = out[:MaxAppID]
	}
	return out
}

// cleanID keeps the bytes an AppUserModelID may hold, and makes every other rune '-'.
func cleanID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), ".")
}

// TakeEmbedding returns args without COM's "-Embedding" or "/Embedding", in any case, and reports whether it was there.
// COM starts a program with it, for a click on one of its toasts after it ended, and a program that parses its flags
// would stop at it. COM's is the sole argument, as the command registered has none, so a line with more is the user's
// own and stays whole.
func TakeEmbedding(args []string) ([]string, bool) {
	if len(args) == 2 && (strings.EqualFold(args[1], "-Embedding") || strings.EqualFold(args[1], "/Embedding")) {
		return args[:1], true
	}
	return args, false
}

// ProgramName is the program file exe's name with no folder or extension, or "gunim" where that leaves nothing.
func ProgramName(exe string) string {
	name := filepath.Base(strings.ReplaceAll(exe, `\`, "/"))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" || name == "." || name == "/" {
		return "gunim"
	}
	return name
}

// ActivatorCLSID returns the class ID of the COM object that hears clicks on appID's toasts, as text with no
// braces. It is a name-based UUID of appID, so it stays the same from run to run.
func ActivatorCLSID(appID string) string {
	// The namespace is gunim's own, a random UUID picked once.
	ns := []byte{0x6b, 0x1f, 0x3e, 0x52, 0x9a, 0x4d, 0x4c, 0x7e, 0x8f, 0x21, 0x5d, 0x0c, 0x93, 0x7a, 0xe4, 0x10}
	h := sha1.New() //nolint:gosec // a name-based UUID, as RFC 9562's version 5 has it
	h.Write(ns)
	h.Write([]byte(appID))
	u := h.Sum(nil)[:16]
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
