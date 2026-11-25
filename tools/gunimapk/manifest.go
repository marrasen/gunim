package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// androidNS is the namespace of Android's attributes in a manifest.
const androidNS = "http://schemas.android.com/apk/res/android"

// parts are a program's own parts of the manifest, from -manifest's
// file: the elements beside the application, the application's own
// attributes, each after a space, and the elements in it.
type parts struct{ beside, attrs, inside string }

// readParts reads -manifest's file for the application ID id, or nothing
// for no file.
func readParts(path, id string) (parts, error) {
	if path == "" {
		return parts{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return parts{}, fmt.Errorf("-manifest: %w", err)
	}
	p, err := parseParts(data, id)
	if err != nil {
		return parts{}, fmt.Errorf("-manifest: %s: %w", path, err)
	}
	return p, nil
}

// taken are the application's attributes gunimapk writes itself, with
// what sets each.
var taken = map[string]string{
	"label":                       "-name sets it",
	"icon":                        "-icon sets it",
	"appCategory":                 "-category sets it",
	"allowBackup":                 "-backup sets it",
	"dataExtractionRules":         "-backup sets it",
	"fullBackupContent":           "-backup sets it",
	"hasFragileUserData":          "-keep-data sets it",
	"debuggable":                  "a build without -keystore is debuggable",
	"hasCode":                     "gunimapk sets it",
	"extractNativeLibs":           "gunimapk sets it",
	"enableOnBackInvokedCallback": "gunimapk sets it",
}

// parseParts reads a program's own parts from a manifest: a file in
// Android's own form, as an editor checks it, that holds the program's
// parts alone. ${applicationId} in it stands for id, as with Gradle, so
// that a provider's authority can differ between a debug build and a
// release build installed side by side. The elements go into the APK's
// manifest as they are written.
func parseParts(data []byte, id string) (parts, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	data = bytes.ReplaceAll(data, []byte("${applicationId}"), []byte(id))
	var p parts
	d := xml.NewDecoder(bytes.NewReader(data))
	var beside strings.Builder
	depth, apps := 0, 0
	// from is where the element at depth 2, or what the application
	// holds, starts.
	from, app, started := int64(0), false, false
	for {
		at := d.InputOffset()
		tok, err := d.Token()
		if errors.Is(err, io.EOF) && started {
			break
		}
		if err != nil {
			return parts{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			started = true
			if err := checkElement(t, depth); err != nil {
				return parts{}, err
			}
			if depth != 2 {
				continue
			}
			from, app = at, t.Name.Local == "application"
			if app {
				apps++
				if apps > 1 {
					return parts{}, errors.New("more than one application")
				}
				from = d.InputOffset()
				for _, a := range t.Attr {
					if p.attrs, err = addAttr(p.attrs, a); err != nil {
						return parts{}, err
					}
				}
			}
		case xml.EndElement:
			depth--
			switch {
			case depth != 1:
			case app:
				p.inside = line(data[from:at])
			default:
				beside.WriteString(line(data[from:d.InputOffset()]))
			}
		}
	}
	p.beside = beside.String()
	return p, nil
}

// line returns what is written in a manifest as whole lines, or "" for
// space alone.
func line(b []byte) string {
	s := strings.Trim(string(b), "\r\n")
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "\t" + strings.TrimLeft(s, " \t") + "\n"
}

// addAttr adds an attribute of the program's application element to
// attrs.
func addAttr(attrs string, a xml.Attr) (string, error) {
	if by, ok := taken[a.Name.Local]; ok {
		return "", fmt.Errorf("the application's %s is not the program's to write: %s", a.Name.Local, by)
	}
	return attrs + " android:" + a.Name.Local + `="` + xmlEscape(a.Value) + `"`, nil
}

// checkElement reports what does not belong in a program's own parts:
// another root than manifest, an attribute of the root's, which are
// gunimapk's, a namespace the APK's manifest does not declare, and what
// gunimapk has a flag for.
func checkElement(e xml.StartElement, depth int) error {
	if depth == 1 {
		if e.Name.Local != "manifest" {
			return fmt.Errorf("the root is %s, not manifest", e.Name.Local)
		}
		for _, a := range e.Attr {
			switch {
			case a.Name.Space == "xmlns" && a.Name.Local == "android" && a.Value == androidNS:
			case a.Name.Space == "" && a.Name.Local == "package":
				return errors.New("-id sets the package; leave it out")
			default:
				return fmt.Errorf("the root's %s has no place; it takes xmlns:android=%q alone", a.Name.Local, androidNS)
			}
		}
		return nil
	}
	if e.Name.Space != "" {
		return fmt.Errorf("%s is in a namespace of its own", e.Name.Local)
	}
	if depth == 2 && e.Name.Local == "uses-sdk" {
		return errors.New("-min-sdk sets uses-sdk; leave it out")
	}
	for _, a := range e.Attr {
		if a.Name.Space != androidNS {
			return fmt.Errorf("%s has an attribute, %s, that is not Android's; the root declares xmlns:android=%q",
				e.Name.Local, a.Name.Local, androidNS)
		}
		if depth == 3 && a.Name.Local == "name" && strings.HasPrefix(a.Value, "gunim.android.") {
			return fmt.Errorf("%s is gunim's own", a.Value)
		}
	}
	return nil
}

// rulesName is the resource that holds noBackupRules.
const rulesName = "gunim_extraction_rules"

// domains are the places Android's backup takes a program's files from,
// by its own names: the program's data and its folders in it, the same
// in the storage open before the first unlock, and its folder on the
// shared storage. Its cache and its no-backup folder are never taken.
var domains = []string{
	"root", "file", "database", "sharedpref", "external",
	"device_root", "device_file", "device_database", "device_sharedpref",
}

// noBackupRules returns the extraction rules that leave a program's
// files out of each way Android carries them off the device. The
// manifest's allowBackup="false" alone lets a transfer to another device
// through from Android 12, and a way with no rules of its own takes
// everything. A transfer to another kind of device, from Android 16
// QPR2, also needs the program to name its other half there, with
// platform-specific-params, which the rules leave out.
func noBackupRules() string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<data-extraction-rules>\n")
	for _, way := range []string{"cloud-backup", "device-transfer", `cross-platform-transfer platform="ios"`} {
		fmt.Fprintf(&b, "\t<%s>\n", way)
		for _, d := range domains {
			fmt.Fprintf(&b, "\t\t<exclude domain=%q path=\".\"/>\n", d)
		}
		name, _, _ := strings.Cut(way, " ")
		fmt.Fprintf(&b, "\t</%s>\n", name)
	}
	b.WriteString("</data-extraction-rules>\n")
	return b.String()
}

// writeRules writes noBackupRules into res, the resources directory.
func writeRules(res string) error {
	dir := filepath.Join(res, "xml")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, rulesName+".xml"), []byte(noBackupRules()), 0o644)
}

// permissionNames returns -permissions' names, in order.
func permissionNames() []string {
	names := make([]string, 0, len(permissions))
	for n := range permissions {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// appID returns the application ID of a build: debugID for a debug build
// where it names one, and id otherwise.
func appID(id, debugID string, release bool) string {
	if debugID != "" && !release {
		return debugID
	}
	return id
}
