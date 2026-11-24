package driver

import (
	"errors"
	"image"
)

// ErrNoTray says the platform has no tray to show an icon in.
var ErrNoTray = errors.New("there is no system tray here")

// Tray is an icon in the system tray, the notification area on Windows,
// with a menu, for an application that runs without a window in front.
type Tray struct {
	// Icon is the icon at several sizes, for the tray to pick from, and
	// Tooltip what shows as the pointer rests on it.
	Icon    []image.Image
	Tooltip string
	// Items are the lines of its menu.
	Items []TrayItem
	// OnPick runs with the ID of the line picked from the menu, and
	// OnClick with a click on the icon itself, where the tray has one
	// apart from the menu's. Both run on a goroutine of the driver's,
	// and are to hand the work on rather than do it there.
	OnPick  func(id int)
	OnClick func()
	// OnNotifyClick runs with a click on a message [TrayNotifier.TrayNotify] showed, as for a message that opens the
	// page it is about. The program may take the foreground as the click comes, so a window raised from here comes
	// to the front. OnNotifyTag hears which message was clicked.
	//
	// OnNotifyGone runs when the message shown last leaves the screen, closed or timed out, as it may then wait in
	// the notification centre. Both run on a goroutine of the driver's, on Windows and Linux.
	OnNotifyClick func()
	OnNotifyGone  func()
	// OnNotifyTag runs with a click on a message [TagNotifier.TrayNotifyTagged] showed, with the tag it was shown
	// with, and with "" for one [TrayNotifier.TrayNotify] showed. It runs after OnNotifyClick, where both are set,
	// on a goroutine of the driver's. [NotifyWayer] says how sure the tag is.
	//
	// On Linux, and on Windows with toasts, the tag is that of the message clicked, of any of the last 64 shown on
	// Linux, and any still in the notification centre on Windows. Windows's tray balloons report only that a
	// message was clicked, so with those the tag is that of the message shown last.
	OnNotifyTag func(tag string)
	// AppID names the application to Windows's notifications, as its AppUserModelID: a few words with dots
	// between, as "Company.Product". The bytes A-Z, a-z, 0-9, '.', '-' and '_' are kept, and others become '-'.
	// Empty names it after the program file, as "gunim.<name>"; two programs of one file name then share it, so
	// an application sets its own. AppName is the name its messages show as from, and empty takes the program
	// file's name. Both are read at the first message.
	//
	// To show toasts, the driver writes both, at the first message, and the icon's picture, under the current user's
	// Software\Classes\AppUserModelId\<AppID> in the registry, with the class ID of the COM object that hears
	// clicks from the notification centre, and that class ID's LocalServer32, the program file. The picture goes in
	// the user's cache folder, under <AppID>. An uninstaller takes all three away. A click on a toast after the
	// program ended starts the program again, with the argument -Embedding, and the click is heard once it puts its
	// icon up.
	AppID, AppName string
}

// NotifyWay is how sure the tag [Tray.OnNotifyTag] hears is.
type NotifyWay uint8

const (
	// NotifyNone says there is no message to show: no icon is up, or the platform shows none.
	NotifyNone NotifyWay = iota
	// NotifyLast says a click names no message, so the tag heard is that of the message shown last, as with
	// Windows's tray balloons.
	NotifyLast
	// NotifyExact says a click names its message, so the tag heard is that message's, as with Windows's toasts
	// and Linux's notification server.
	NotifyExact
)

// String names w, for a log.
func (w NotifyWay) String() string {
	switch w {
	case NotifyNone:
		return "none"
	case NotifyLast:
		return "last"
	case NotifyExact:
		return "exact"
	}
	return "none"
}

// A NotifyWayer is a [TagNotifier] that says how sure a click's tag is.
type NotifyWayer interface {
	// TrayNotifyWay says how the next message shows: on Windows NotifyExact for a toast, and NotifyLast for a
	// balloon, where toasts cannot be shown.
	TrayNotifyWay() NotifyWay
}

// TrayItem is a line of a tray icon's menu: a command, a submenu, or a
// line between groups.
type TrayItem struct {
	// Title is what the line says. ID is what OnPick hears when it is
	// picked, and means nothing for a submenu or a separator.
	Title string
	ID    int
	// Items make the line a submenu of them.
	Items []TrayItem
	// Checked ticks the line, Disabled greys it out, and Separator makes
	// it a line between groups and nothing else.
	Checked   bool
	Disabled  bool
	Separator bool
	// Default draws the line in bold, as what a click on the icon does.
	Default bool
}

// A TrayNotifier is a [Driver] that shows a message from its tray icon,
// as the system shows one.
type TrayNotifier interface {
	// TrayNotify shows a message from the tray icon, as the system shows
	// one, and fails where no icon is up or the platform has none.
	TrayNotify(title, body string) error
}

// MaxNotifyTag is the most bytes a message's tag holds.
const MaxNotifyTag = 1024

// ErrNotifyTag says a message's tag is longer than [MaxNotifyTag] bytes.
var ErrNotifyTag = errors.New("the message's tag is longer than MaxNotifyTag bytes")

// CheckNotifyTag returns [ErrNotifyTag] for a tag longer than [MaxNotifyTag] bytes, and nil for any other.
func CheckNotifyTag(tag string) error {
	if len(tag) > MaxNotifyTag {
		return ErrNotifyTag
	}
	return nil
}

// A TagNotifier is a [TrayNotifier] that gives each message a tag, which [Tray.OnNotifyTag] hears back with a click
// on it.
type TagNotifier interface {
	// TrayNotifyTagged shows a message as TrayNotify does, with tag, any text the application picks, as an ID of
	// the message or the page it is about. The tag stays in the process. It fails with ErrNotifyTag for a tag
	// longer than MaxNotifyTag bytes.
	TrayNotifyTagged(title, body, tag string) error
}

// A Trayer is a [Driver] that can show a tray icon.
type Trayer interface {
	// SetTray shows t in place of the tray icon shown before, and a
	// Tray with no Icon takes it away. It returns an error where the
	// platform has no tray to show one in.
	SetTray(t Tray) error
}
