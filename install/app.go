package install

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"io/fs"
	"strings"
	"unicode"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/theme"
)

// App describes a program to install. Name and Version are all it
// needs; every other field has a default, and each one an application
// sets changes that part of the installer and leaves the rest.
type App struct {
	// Name is what the user sees: the title of the installer, the
	// shortcut in the Start menu or the applications menu, and the entry
	// in the system's list of installed programs.
	Name string
	// Version is this build's version, as "v1.2.0" or "1.2.0", usually
	// set as the release is built with -ldflags "-X main.version=…". An
	// empty Version, "dev", or anything else that is not a release says
	// the program was built from a working tree: [Run] then lets it run
	// as it is, with no installer, and it looks for no updates.
	Version string

	// ID names the program to the system: its folder, its registry key,
	// its desktop file. It is made from Name when empty, in lower case
	// with dashes for spaces, as "marras-mastering-studio". Keep it the
	// same from one release to the next, as the installed copy is found
	// by it.
	ID string
	// Exe is the program's file name in its folder, with no ".exe". It is
	// ID when empty.
	Exe string
	// Publisher is who made the program, as the list of installed
	// programs shows it.
	Publisher string
	// Description says in a sentence what the program does. The
	// installer shows it under the name, and the shortcuts carry it.
	Description string
	// URL is the program's home page.
	URL string
	// Icon is the program's icon. The installer shows it large, and on
	// Linux it becomes the icon of the program's desktop file. On
	// Windows the shortcuts take the icon built into the program. When
	// nil, the installer draws the first letter of Name.
	Icon image.Image
	// IconFunc, when set and Icon is not, makes the icon, for one that
	// takes time to make: it runs only when the installer needs the
	// icon, and not as the installed program starts, but for its first
	// start after an update, which writes the icon again.
	IconFunc func() image.Image
	// Categories are the desktop file's categories on Linux, as
	// "AudioVideo;Audio;". "Utility;" when empty.
	Categories string

	// Files are more files to install beside the program, such as a
	// library it loads, kept in the program with go:embed. Installed,
	// they are where [Dir] says, under the same names. An update puts
	// the new program in place first, and the program writes its new
	// files as it first starts, in [Run]: load them only after Run, never
	// as the program loads or in an init.
	Files fs.FS
	// Space is how much more room, in bytes, the program takes once it
	// runs, as for the data it downloads. The installer counts it when it
	// checks there is room.
	Space int64

	// FileTypes are the kinds of file the program opens. The installer
	// offers each, and the system then lists the program among those
	// that open such a file.
	FileTypes []FileType
	// Autostart, when set, offers to start the program with the
	// computer.
	Autostart *Autostart
	// NoDesktop leaves out the offer of a shortcut on the desktop, and
	// Desktop ticks it to begin with.
	NoDesktop bool
	Desktop   bool
	// Choices are offers of the application's own, shown under the
	// installer's, whose answers reach [App.Installed] and are kept for
	// the next install.
	Choices []Choice

	// Updates is where newer releases come from, such as [GitHub]. With
	// one, the installer offers to keep the program up to date, and an
	// installed release then looks for a newer one a minute after it
	// starts and once a day: as the user's [UpdateMode] says, it puts it
	// in place for the next start by itself, tells the program, or does
	// nothing. Nil looks for none.
	Updates Source
	// UpdateKey is the public key the program's releases are signed
	// with, as "gunimsign -keygen" prints it. Updates needs it: an update
	// runs only when its SHA256SUMS carries a signature this key checks,
	// so a release no one with the private key made never runs, wherever
	// it comes from.
	UpdateKey string
	// UpdateMode is how updates go until the user chooses: UpdatesInstall
	// when empty. A program that kept a setting of its own for it before
	// it used this package gives that setting here, so an install of it
	// taken on keeps what the user chose.
	UpdateMode UpdateMode
	// Available, when set, hears of a newer release, with the updates
	// set to UpdatesNotify, on a goroutine of its own: for the program to
	// ask the user, and on a yes put it in place with [Stage] and offer
	// to [Restart] into it. Without it, UpdatesNotify does nothing.
	Available func(Release)
	// Updated, when set, hears of each release put in place for the next
	// start by itself, with the updates set to UpdatesInstall, on a
	// goroutine of its own, so the program can offer to restart into it
	// with [Restart].
	Updated func(Release)

	// Data are the folders the program keeps the user's settings and
	// work in. Uninstalling offers to take them away too, and leaves
	// them by default.
	Data []string
	// Dir, when set, says where to install, in place of the system's
	// usual place for a program installed for one user.
	Dir func() (string, error)
	// Formerly are the places older versions put the program, as full
	// paths, for a program that installed itself before it used this
	// package. Started from one, the installed program moves itself to
	// where it goes now, with what it had on the system, and runs on;
	// and the installer, started from a download, updates it there.
	Formerly []string

	// Installed, when set, runs once the program is in place and
	// registered, on a fresh install, on an update put in place, and on
	// an install over an older copy, with what the user chose. An error
	// fails the install and is shown.
	Installed func(ctx context.Context, in Installation) error
	// Uninstalling, when set, runs before the program is taken away. An
	// error stops the uninstall and is shown.
	Uninstalling func(ctx context.Context, in Installation) error
	// Quit, when set, asks the copies of the program still running to
	// end, as before an uninstall. Without it the installer asks the
	// user to close them.
	Quit func(ctx context.Context) error

	// Look is how the installer's window looks.
	Look Look
	// Words are what the installer's window says, where the defaults
	// will not do.
	Words Words
	// Window, when set, replaces the installer's window with one of the
	// application's own, which drives s.
	Window func(ctx context.Context, a *gunim.App, s *Session) error
}

// FileType is a kind of file the program opens.
type FileType struct {
	// Name is what the kind of file is called, as "Mastering album".
	Name string
	// Exts are the kind's file name extensions, each with its dot.
	Exts []string
	// MIME is the kind's media type, as "audio/flac", for one the
	// system knows already. When empty, the kind is the program's own,
	// and Linux learns it as application/x-<ID>-<first extension>.
	MIME string
	// Default makes the program the one that opens the kind when the
	// user has not picked another, as is right for a kind of its own.
	// Without it the program is only offered among the others.
	Default bool
	// Off leaves the offer unticked to begin with.
	Off bool
}

// Autostart is the offer to start the program with the computer.
type Autostart struct {
	// Args are what the program is started with then, as "-tray".
	Args []string
	// On ticks the offer to begin with.
	On bool
	// Label is what the offer says. "Start <Name> with the computer"
	// when empty.
	Label string
}

// Choice is an offer of the application's own in the installer.
type Choice struct {
	// Key names the choice in [Installation.Chose] and in what is kept.
	Key string
	// Label is what the offer says, and Detail a line under it.
	Label  string
	Detail string
	// On ticks the offer to begin with.
	On bool
	// Current says On is how the program stands now, as for a choice
	// the program's own settings also change: the installer starts from
	// it every time, in place of what the user chose at the last
	// install.
	Current bool
}

// Look is how the installer's window looks. Its zero value is the
// default look: a dark window in the colours of the program's icon.
type Look struct {
	// Accent is the colour the installer glows and moves in. When unset,
	// it is the most vivid colour of the icon.
	Accent color.NRGBA
	// Theme replaces the installer's theme, for an application that
	// wants its own widgets' look.
	Theme *theme.Theme
	// Width and Height are the window's size. 640 by 520 when zero.
	Width, Height float32
}

// Words are what the installer's window says. An empty field says the
// default.
type Words struct {
	// Welcome is the line over the choices.
	Welcome string
	// Done is said once the program is installed.
	Done string
	// Removed is said once the program is uninstalled.
	Removed string
}

// Installation is an installed copy of the program, as the installer
// hands it to [App.Installed] and [App.Uninstalling].
type Installation struct {
	// Dir is the folder the program is in, and Exe the program.
	Dir, Exe string
	// Version is the version installed.
	Version string
	// Updates is how the installed program takes newer releases.
	Updates UpdateMode
	// Picks are the user's answers to the offers, by key: the program's
	// own [Choice] keys, and the installer's own, which are [PickDesktop],
	// [PickAutostart], [PickUpdates] and, for each file type, the result
	// of [FileTypeKey].
	Picks map[string]bool
}

// Chose reports whether the user ticked the offer named key.
func (in Installation) Chose(key string) bool { return in.Picks[key] }

// The keys of the installer's own offers in [Installation.Picks].
const (
	PickDesktop   = "desktop"
	PickAutostart = "autostart"
	PickUpdates   = "updates"
)

// FileTypeKey is the key of the offer to open files of kind t.
func FileTypeKey(t FileType) string {
	if len(t.Exts) == 0 {
		return "type:"
	}
	return "type:" + strings.ToLower(strings.TrimPrefix(t.Exts[0], "."))
}

// UpdateMode is how an installed program takes newer releases.
type UpdateMode string

// The update modes.
const (
	// UpdatesInstall puts a newer release in place for the next start
	// by itself.
	UpdatesInstall UpdateMode = "install"
	// UpdatesNotify tells the program of a newer release, through
	// [App.Available], to ask the user.
	UpdatesNotify UpdateMode = "notify"
	// UpdatesOff looks for none.
	UpdatesOff UpdateMode = "off"
)

// valid reports whether m is one of the modes.
func (m UpdateMode) valid() bool {
	return m == UpdatesInstall || m == UpdatesNotify || m == UpdatesOff
}

// unticked is the mode the installer's offer to keep the program up to
// date gives unticked: the program is told, where it can be, and
// otherwise nothing is done.
func (a *App) unticked() UpdateMode {
	if a.Available != nil {
		return UpdatesNotify
	}
	return UpdatesOff
}

// startMode is the mode before the user chooses one.
func (a *App) startMode() UpdateMode {
	if a.UpdateMode.valid() {
		return a.UpdateMode
	}
	return UpdatesInstall
}

// ErrUnsupported says installing is not done on this system yet.
var ErrUnsupported = errors.New("install: not done on this system yet")

// id is the program's ID: as set, or made from its name.
func (a *App) id() string {
	if a.ID != "" {
		return a.ID
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(a.Name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	if b.Len() == 0 {
		return "app"
	}
	return b.String()
}

// exe is the program's file name, with ".exe" on Windows.
func (a *App) exe() string {
	name := a.Exe
	if name == "" {
		name = a.id()
	}
	return name + exeSuffix
}

// check says what is missing from a.
func (a *App) check() error {
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("install: App.Name is empty")
	}
	id := a.id()
	if strings.ContainsAny(id, `/\:*?"<>|`) || strings.HasPrefix(id, ".") {
		return errors.New("install: App.ID " + id + " is not a name a folder can take")
	}
	if a.Exe != "" && strings.ContainsAny(a.Exe, `/\:*?"<>|`) {
		return errors.New("install: App.Exe " + a.Exe + " is not a name a file can take")
	}
	for _, t := range a.FileTypes {
		if len(t.Exts) == 0 {
			return errors.New("install: file type " + t.Name + " has no extension")
		}
		for _, e := range t.Exts {
			if !strings.HasPrefix(e, ".") || len(e) < 2 || strings.ContainsAny(e, `/\ "`) {
				return errors.New("install: file type " + t.Name + " has extension " + e + ", which wants a dot and a name")
			}
		}
	}
	if a.Updates != nil {
		if _, err := a.updateKey(); err != nil {
			return err
		}
	}
	for _, c := range a.Choices {
		switch c.Key {
		case "", PickDesktop, PickAutostart, PickUpdates:
			return errors.New("install: choice " + c.Label + " needs a key of its own")
		}
		if strings.HasPrefix(c.Key, "type:") {
			return errors.New("install: choice key " + c.Key + " starts as a file type's does")
		}
	}
	return nil
}

// updateKey is App.UpdateKey, read.
func (a *App) updateKey() (ed25519.PublicKey, error) {
	if a.UpdateKey == "" {
		return nil, errors.New("install: App.Updates needs App.UpdateKey, the key its releases are signed with; gunimsign -keygen makes one")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(a.UpdateKey))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("install: App.UpdateKey is not a key gunimsign -keygen prints")
	}
	return ed25519.PublicKey(raw), nil
}

// offers are the installer's offers to a, with how each starts: as the
// application set it, unless an installed copy kept what the user
// chose.
func (a *App) offers(kept map[string]bool) []Offer {
	var out []Offer
	pick := func(key string, on bool) bool {
		if v, ok := kept[key]; ok {
			return v
		}
		return on
	}
	if !a.NoDesktop {
		out = append(out, Offer{Key: PickDesktop, Label: "A shortcut on the desktop", On: pick(PickDesktop, a.Desktop)})
	}
	if a.Autostart != nil {
		label := a.Autostart.Label
		if label == "" {
			label = "Start " + a.Name + " with the computer"
		}
		out = append(out, Offer{Key: PickAutostart, Label: label, On: pick(PickAutostart, a.Autostart.On)})
	}
	for _, t := range a.FileTypes {
		label := "Open " + strings.Join(t.Exts, " ") + " files with " + a.Name
		if t.Name != "" {
			label = "Open " + t.Name + " files (" + strings.Join(t.Exts, " ") + ")"
		}
		out = append(out, Offer{Key: FileTypeKey(t), Label: label, On: pick(FileTypeKey(t), !t.Off)})
	}
	if a.Updates != nil {
		detail := "New releases install by themselves, for the next start."
		if a.Available != nil {
			detail = "New releases install by themselves; unticked, " + a.Name + " asks first."
		}
		out = append(out, Offer{Key: PickUpdates, Label: "Keep " + a.Name + " up to date", Detail: detail,
			On: pick(PickUpdates, a.startMode() == UpdatesInstall)})
	}
	for _, c := range a.Choices {
		on := c.On
		if !c.Current {
			on = pick(c.Key, c.On)
		}
		out = append(out, Offer{Key: c.Key, Label: c.Label, Detail: c.Detail, On: on})
	}
	return out
}
