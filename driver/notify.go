package driver

import (
	"errors"
	"image"
	"unicode/utf8"
)

// Notification is a message of an application's that the system shows apart from its window, as a phone does in its
// notification shade and on its lock screen.
type Notification struct {
	// Tag names the message: one with the tag of one that shows takes its place, quietly, and a tap on it brings the
	// tag back; see [Notifier.OnNotification]. It is any UTF-8 text of at most [MaxNotifyTag] bytes, such as the ID of
	// what the message is about. The system keeps it with the message, so it holds no secret.
	Tag string
	// Channel is the kind of message this is, which the user can turn off or quieten in the system's settings.
	Channel NotifyChannel
	// Title and Body are what the message says.
	Title, Body string
	// Lock is how much of it the lock screen shows, and PublicTitle and PublicBody are what stands there in place of
	// Title and Body with [LockPrivate]. With no PublicTitle the application's name does.
	Lock                    NotifyLock
	PublicTitle, PublicBody string
	// Progress shows a bar of how far work has come, unless it is nil. Set it as often as it changes: the driver leaves
	// out what comes faster than the system takes it, and always shows the last.
	Progress *NotifyProgress
	// Ongoing keeps the message as the user clears the rest, as for work that goes on, where the system lets it. The
	// application takes it away, or replaces it, as the work ends.
	Ongoing bool
	// Icon is the message's small icon, of which the system shows the shape alone, its alpha, in a colour of its own.
	// nil takes a plain one. The picture is read after Notify returns too, so it stays as it is.
	Icon image.Image
}

// NotifyChannel is a kind of message. The system shows it to the user by its name and what it is for, and keeps what
// the user chose for it by its ID.
type NotifyChannel struct {
	// ID is the same from run to run. "" is a kind named after the application.
	ID string
	// Name and Description are in the user's language. An empty Name takes the application's.
	Name, Description string
	// Importance is how much it disturbs. The user can change it, and it can only be lowered once the kind has been
	// used.
	Importance NotifyImportance
}

// NotifyImportance is how much a kind of message disturbs the user.
type NotifyImportance uint8

const (
	// NotifyNormal makes a sound.
	NotifyNormal NotifyImportance = iota
	// NotifyQuiet makes no sound, as for progress.
	NotifyQuiet
	// NotifyUrgent makes a sound, and shows over what the user does.
	NotifyUrgent
)

// NotifyLock is how much of a message the lock screen shows, at the most: the user may have the system show less. A
// user who has the system show sensitive messages whole there sees those of [LockPrivate] whole, so what must never
// show with the phone locked is in no message.
type NotifyLock uint8

const (
	// LockPrivate shows that there is a message, with its PublicTitle and PublicBody.
	LockPrivate NotifyLock = iota
	// LockPublic shows it whole.
	LockPublic
	// LockSecret shows nothing of it.
	LockSecret
)

// NotifyProgress is how far work has come: Done of Total, in any unit, as bytes. A Total of 0 or less is work of
// unknown length.
type NotifyProgress struct{ Done, Total int64 }

// A Notifier is a [Driver] whose system shows an application's messages.
type Notifier interface {
	// Notify shows n, or puts it in the place of the one with its tag. It fails with [ErrNotifyOff] where the user, or
	// the system, shows no such message.
	Notify(n Notification) error
	// EndNotify takes the message with tag away, where one shows.
	EndNotify(tag string) error
	// OnNotification has fn hear the tag of each message the user taps, on a goroutine of the driver's, one at a time.
	// A tap brings the application to the front, or starts it. Those that came before are heard at once.
	OnNotification(fn func(tag string))
}

// ErrNoNotifier is returned where the platform shows no such messages. A desktop shows them from a tray icon; see
// [TrayNotifier].
var ErrNoNotifier = errors.New("driver: no notifications on this platform")

// ErrNotifyOff is returned where the user, or the system, shows no message of the kind: the application has no
// permission, or the user turned it or the kind off.
var ErrNotifyOff = errors.New("driver: the user has the application's notifications off")

// ErrNotifyText is returned for a message whose tag or text is not UTF-8.
var ErrNotifyText = errors.New("driver: a notification's tag or text is not UTF-8")

// CheckNotification returns [ErrNotifyTag] for a message whose tag is too long, and [ErrNotifyText] for one that is
// not UTF-8.
func CheckNotification(n Notification) error {
	if err := CheckNotifyTag(n.Tag); err != nil {
		return err
	}
	for _, s := range []string{n.Tag, n.Channel.ID, n.Channel.Name, n.Channel.Description, n.Title, n.Body, n.PublicTitle, n.PublicBody} {
		if !utf8.ValidString(s) {
			return ErrNotifyText
		}
	}
	return nil
}

// Parts returns how far p has come in parts of a thousand, and false for work of unknown length.
func (p NotifyProgress) Parts() (int, bool) {
	if p.Total <= 0 {
		return 0, false
	}
	done := min(max(p.Done, 0), p.Total)
	return int(float64(done) / float64(p.Total) * 1000), true
}
