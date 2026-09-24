package text

import (
	"errors"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
)

// The fonts installed on the system, as a last fallback. They are found
// on first need: the first character no face covers starts a scan. The
// first scan on a machine reads every font file, and caches an index on
// disk that makes later ones fast. All of it runs with mu held.
var (
	systemOn    = true
	systemTried bool
	system      *fontscan.FontMap
)

// UseSystemFonts turns the system fallback on or off. It is on to start
// with: a character that neither a face nor its fallbacks cover comes
// from a font installed on the system, so text in any script shows
// where a font for it is installed.
func UseSystemFonts(on bool) {
	mu.Lock()
	defer mu.Unlock()
	systemOn = on
}

// LoadSystemFonts finds the system's fonts now, rather than when a
// character first needs one. The first scan on a machine can take a
// second or more; an application that shows text in many scripts can
// call it from a goroutine at startup, so no frame waits on the scan.
func LoadSystemFonts() error {
	mu.Lock()
	defer mu.Unlock()
	if loadSystem() == nil {
		return errors.New("text: no system fonts found")
	}
	return nil
}

// loadSystem returns the system's fonts, scanning for them on the first
// call. It returns nil when they are turned off or none were found. It
// runs with mu held.
func loadSystem() *fontscan.FontMap {
	if !systemOn {
		return nil
	}
	if !systemTried {
		systemTried = true
		fm := fontscan.NewFontMap(quiet{})
		if err := fm.UseSystemFonts(""); err == nil {
			fm.SetQuery(fontscan.Query{Families: []string{"sans-serif"}})
			system = fm
		}
	}
	return system
}

// systemFace returns an installed font that covers r, or nil. It runs
// with mu held.
func systemFace(r rune) *font.Face {
	fm := loadSystem()
	if fm == nil {
		return nil
	}
	fm.SetScript(language.LookupScript(r))
	ff := fm.ResolveFace(r)
	if ff == nil {
		return nil
	}
	// The font map answers with some font even when none covers r.
	if _, ok := ff.NominalGlyph(r); !ok {
		return nil
	}
	faceOf(ff)
	return ff
}

// quiet drops the font scanner's log, which reports every font it
// passes over.
type quiet struct{}

// Printf implements [fontscan.Logger] by dropping the message.
func (quiet) Printf(string, ...any) {}
