//go:build linux

package desktop

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// The system's media controls on Linux are MPRIS, the D-Bus interface
// GNOME's and KDE's panels, and the media keys, speak to. The player
// shows there while SetNowPlaying has something to show, under a bus
// name of its own, and leaves as it is given nil.

const (
	mprisPath   = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisRoot   = "org.mpris.MediaPlayer2"
	mprisPlayer = "org.mpris.MediaPlayer2.Player"
	dbusProps   = "org.freedesktop.DBus.Properties"
)

// mpris is the application's player on the session bus.
type mpris struct {
	d  *Driver
	mu sync.Mutex
	// conn is the session bus while the player shows, and name its name
	// there.
	conn *dbus.Conn
	name string
	// np is what plays, as of since; track numbers what it is, to name
	// it, and key tells a new one.
	np    driver.NowPlaying
	since time.Time
	track int
	key   string
	// art is the file the cover is written to, for the panel to read,
	// and cover the cover it holds.
	art   string
	cover []byte
}

var player = &mpris{}

// SetNowPlaying implements [driver.NowPlayer]: the player shows in the
// desktop's media controls through MPRIS.
func (d *Driver) SetNowPlaying(np *driver.NowPlaying) error {
	player.mu.Lock()
	defer player.mu.Unlock()
	player.d = d
	if np == nil {
		player.leave()
		return nil
	}
	return player.show(*np)
}

// show puts np in the media controls, joining the bus first if it must.
func (m *mpris) show(np driver.NowPlaying) error {
	if m.conn == nil {
		if err := m.join(); err != nil {
			return err
		}
	}
	was, wasAt := m.np, positionNow(&m.np, m.since)
	m.np, m.since = np, time.Now()
	changed := map[string]dbus.Variant{}
	if key := np.Title + "\x00" + np.Artist + "\x00" + np.Album + "\x00" + np.Length.String(); key != m.key || m.track == 0 {
		m.key = key
		m.track++
		m.writeArt(np.Cover)
		changed["Metadata"] = dbus.MakeVariant(m.metadata())
		changed["CanSeek"] = dbus.MakeVariant(np.Length > 0)
	} else if !bytes.Equal(np.Cover, m.cover) {
		m.writeArt(np.Cover)
		changed["Metadata"] = dbus.MakeVariant(m.metadata())
	}
	if was.Playing != np.Playing || m.track == 1 {
		changed["PlaybackStatus"] = dbus.MakeVariant(status(np.Playing))
	}
	if len(changed) > 0 {
		_ = m.conn.Emit(mprisPath, dbusProps+".PropertiesChanged", mprisPlayer, changed, []string{})
	}
	// The panel works out the position from the rate as it plays, and
	// hears of a jump.
	if d := np.Position - wasAt; d > time.Second || d < -time.Second {
		_ = m.conn.Emit(mprisPath, mprisPlayer+".Seeked", micros(np.Position))
	}
	return nil
}

// join connects to the session bus, puts the player's objects there,
// and takes a name for it.
func (m *mpris) join() error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("desktop: media controls: %w", err)
	}
	if err = conn.Export(mprisRootObject{m}, mprisPath, mprisRoot); err != nil {
		_ = conn.Close()
		return err
	}
	o := mprisPlayerObject{m}
	methods := map[string]any{
		"Next": o.next, "Previous": o.previous, "Pause": o.pause, "PlayPause": o.playPause,
		"Stop": o.stop, "Play": o.play, "Seek": o.seekBy, "SetPosition": o.setPosition, "OpenUri": o.openURI,
	}
	if err = conn.ExportMethodTable(methods, mprisPath, mprisPlayer); err != nil {
		_ = conn.Close()
		return err
	}
	if err = conn.Export(mprisProps{m}, mprisPath, dbusProps); err != nil {
		_ = conn.Close()
		return err
	}
	if err = conn.Export(introspect.Introspectable(mprisIntrospection), mprisPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		_ = conn.Close()
		return err
	}
	name := fmt.Sprintf("%s.%s.instance%d", mprisRoot, busWord(identity()), os.Getpid())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		if err == nil {
			err = fmt.Errorf("the name %s is taken", name)
		}
		return fmt.Errorf("desktop: media controls: %w", err)
	}
	m.conn, m.name, m.track, m.key = conn, name, 0, ""
	return nil
}

// leave takes the player off the bus, and its cover off the disk.
func (m *mpris) leave() {
	if m.conn != nil {
		_, _ = m.conn.ReleaseName(m.name)
		_ = m.conn.Close()
		m.conn = nil
	}
	m.writeArt(nil)
	m.np = driver.NowPlaying{}
}

// writeArt keeps cover in a file for the panel to read, a new file for
// each, so the panel reads it again; nil takes the file away.
func (m *mpris) writeArt(cover []byte) {
	if m.art != "" {
		_ = os.Remove(m.art)
		m.art = ""
	}
	m.cover = cover
	if len(cover) == 0 {
		return
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "gunim")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	ext := ".png"
	if len(cover) > 2 && cover[0] == 0xff && cover[1] == 0xd8 {
		ext = ".jpg"
	}
	path := filepath.Join(dir, fmt.Sprintf("now-playing-%d-%d%s", os.Getpid(), m.track, ext))
	if os.WriteFile(path, cover, 0o644) == nil {
		m.art = path
	}
}

// trackID names the track playing for MPRIS.
func (m *mpris) trackID() dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("/org/gunim/track/%d", m.track))
}

// metadata is the track playing as MPRIS describes it.
func (m *mpris) metadata() map[string]dbus.Variant {
	md := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(m.trackID()),
		"xesam:title":   dbus.MakeVariant(m.np.Title),
	}
	if m.np.Artist != "" {
		md["xesam:artist"] = dbus.MakeVariant([]string{m.np.Artist})
	}
	if m.np.Album != "" {
		md["xesam:album"] = dbus.MakeVariant(m.np.Album)
	}
	if m.np.Length > 0 {
		md["mpris:length"] = dbus.MakeVariant(micros(m.np.Length))
	}
	if m.art != "" {
		md["mpris:artUrl"] = dbus.MakeVariant("file://" + m.art)
	}
	return md
}

// props returns the properties of iface, as they are now.
func (m *mpris) props(iface string) (map[string]dbus.Variant, bool) {
	switch iface {
	case mprisRoot:
		return map[string]dbus.Variant{
			"CanQuit":             dbus.MakeVariant(false),
			"CanRaise":            dbus.MakeVariant(true),
			"HasTrackList":        dbus.MakeVariant(false),
			"Identity":            dbus.MakeVariant(identity()),
			"SupportedUriSchemes": dbus.MakeVariant([]string{}),
			"SupportedMimeTypes":  dbus.MakeVariant([]string{}),
		}, true
	case mprisPlayer:
		return map[string]dbus.Variant{
			"PlaybackStatus": dbus.MakeVariant(status(m.np.Playing)),
			"Rate":           dbus.MakeVariant(1.0),
			"MinimumRate":    dbus.MakeVariant(1.0),
			"MaximumRate":    dbus.MakeVariant(1.0),
			"Volume":         dbus.MakeVariant(1.0),
			"Metadata":       dbus.MakeVariant(m.metadata()),
			"Position":       dbus.MakeVariant(micros(positionNow(&m.np, m.since))),
			"CanGoNext":      dbus.MakeVariant(true),
			"CanGoPrevious":  dbus.MakeVariant(true),
			"CanPlay":        dbus.MakeVariant(true),
			"CanPause":       dbus.MakeVariant(true),
			"CanSeek":        dbus.MakeVariant(m.np.Length > 0),
			"CanControl":     dbus.MakeVariant(true),
		}, true
	}
	return nil, false
}

func status(playing bool) string {
	if playing {
		return "Playing"
	}
	return "Paused"
}

// micros is d in microseconds, as MPRIS counts time.
func micros(d time.Duration) int64 { return d.Microseconds() }

// identity is the application's name, from its program's.
func identity() string {
	exe, err := os.Executable()
	if err != nil {
		return "gunim"
	}
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	if name == "" {
		return "gunim"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// busWord makes s fit as one element of a bus name: letters, digits
// and underscores, starting with a letter.
func busWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9' && b.Len() > 0, r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || b.String()[0] == '_' {
		return "gunim" + b.String()
	}
	return b.String()
}

// mprisRootObject is org.mpris.MediaPlayer2.
type mprisRootObject struct{ m *mpris }

// Raise brings the window the controls speak to forward.
func (o mprisRootObject) Raise() *dbus.Error {
	o.m.mu.Lock()
	d := o.m.d
	o.m.mu.Unlock()
	if d != nil {
		d.post(func() {
			if w := d.mediaWindow(); w != nil {
				_ = w.gw.Restore()
				_ = w.gw.Focus()
			}
		})
	}
	return nil
}

// Quit is refused: CanQuit is false.
func (mprisRootObject) Quit() *dbus.Error { return nil }

// mprisPlayerObject is org.mpris.MediaPlayer2.Player: its buttons reach
// the window as media keys, and its seeks as input.MediaSeek.
type mprisPlayerObject struct{ m *mpris }

func (o mprisPlayerObject) key(k input.Key) *dbus.Error {
	o.m.mu.Lock()
	d := o.m.d
	o.m.mu.Unlock()
	if d != nil {
		d.mediaKey(k)
	}
	return nil
}

// The buttons: each reaches the window as its media key.
func (o mprisPlayerObject) next() *dbus.Error      { return o.key(input.KeyMediaNext) }
func (o mprisPlayerObject) previous() *dbus.Error  { return o.key(input.KeyMediaPrevious) }
func (o mprisPlayerObject) pause() *dbus.Error     { return o.key(input.KeyMediaPause) }
func (o mprisPlayerObject) playPause() *dbus.Error { return o.key(input.KeyMediaPlayPause) }
func (o mprisPlayerObject) stop() *dbus.Error      { return o.key(input.KeyMediaStop) }
func (o mprisPlayerObject) play() *dbus.Error      { return o.key(input.KeyMediaPlay) }

// seekBy is Seek: it moves the track by offset microseconds.
func (o mprisPlayerObject) seekBy(offset int64) *dbus.Error {
	o.m.mu.Lock()
	d, at := o.m.d, positionNow(&o.m.np, o.m.since)
	o.m.mu.Unlock()
	if d != nil {
		d.mediaSeek(at + time.Duration(offset)*time.Microsecond)
	}
	return nil
}

// setPosition is SetPosition: it moves the track named to pos
// microseconds in.
func (o mprisPlayerObject) setPosition(track dbus.ObjectPath, pos int64) *dbus.Error {
	o.m.mu.Lock()
	d, ok := o.m.d, track == o.m.trackID()
	o.m.mu.Unlock()
	if d != nil && ok {
		d.mediaSeek(time.Duration(pos) * time.Microsecond)
	}
	return nil
}

// openURI is OpenUri, which the player refuses: it takes no URIs.
func (mprisPlayerObject) openURI(string) *dbus.Error { return nil }

// mprisProps is org.freedesktop.DBus.Properties, worked out as asked,
// so the position is the position now.
type mprisProps struct{ m *mpris }

// Get returns the property name of iface, as it is now.
func (p mprisProps) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()
	all, ok := p.m.props(iface)
	if !ok {
		return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("no interface %s", iface))
	}
	v, ok := all[name]
	if !ok {
		return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("no property %s", name))
	}
	return v, nil
}

// GetAll returns the properties of iface, as they are now.
func (p mprisProps) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	p.m.mu.Lock()
	defer p.m.mu.Unlock()
	all, ok := p.m.props(iface)
	if !ok {
		return nil, dbus.MakeFailedError(fmt.Errorf("no interface %s", iface))
	}
	return all, nil
}

// Set changes nothing: the player's volume and rate are its own.
func (mprisProps) Set(string, string, dbus.Variant) *dbus.Error { return nil }

// positionNow returns how far np has played now, given that it had
// played np.Position at since.
func positionNow(np *driver.NowPlaying, since time.Time) time.Duration {
	at := np.Position
	if np.Playing {
		at += time.Since(since)
	}
	if np.Length > 0 {
		at = min(at, np.Length)
	}
	return max(at, 0)
}

const mprisIntrospection = `<node>
  <interface name="org.mpris.MediaPlayer2">
    <method name="Raise"/>
    <method name="Quit"/>
    <property name="CanQuit" type="b" access="read"/>
    <property name="CanRaise" type="b" access="read"/>
    <property name="HasTrackList" type="b" access="read"/>
    <property name="Identity" type="s" access="read"/>
    <property name="SupportedUriSchemes" type="as" access="read"/>
    <property name="SupportedMimeTypes" type="as" access="read"/>
  </interface>
  <interface name="org.mpris.MediaPlayer2.Player">
    <method name="Next"/>
    <method name="Previous"/>
    <method name="Pause"/>
    <method name="PlayPause"/>
    <method name="Stop"/>
    <method name="Play"/>
    <method name="Seek"><arg name="Offset" type="x" direction="in"/></method>
    <method name="SetPosition">
      <arg name="TrackId" type="o" direction="in"/>
      <arg name="Position" type="x" direction="in"/>
    </method>
    <method name="OpenUri"><arg name="Uri" type="s" direction="in"/></method>
    <signal name="Seeked"><arg name="Position" type="x"/></signal>
    <property name="PlaybackStatus" type="s" access="read"/>
    <property name="Rate" type="d" access="readwrite"/>
    <property name="Metadata" type="a{sv}" access="read"/>
    <property name="Volume" type="d" access="readwrite"/>
    <property name="Position" type="x" access="read"/>
    <property name="MinimumRate" type="d" access="read"/>
    <property name="MaximumRate" type="d" access="read"/>
    <property name="CanGoNext" type="b" access="read"/>
    <property name="CanGoPrevious" type="b" access="read"/>
    <property name="CanPlay" type="b" access="read"/>
    <property name="CanPause" type="b" access="read"/>
    <property name="CanSeek" type="b" access="read"/>
    <property name="CanControl" type="b" access="read"/>
  </interface>
  <interface name="org.freedesktop.DBus.Properties">
    <method name="Get">
      <arg name="interface" type="s" direction="in"/>
      <arg name="property" type="s" direction="in"/>
      <arg name="value" type="v" direction="out"/>
    </method>
    <method name="GetAll">
      <arg name="interface" type="s" direction="in"/>
      <arg name="properties" type="a{sv}" direction="out"/>
    </method>
    <method name="Set">
      <arg name="interface" type="s" direction="in"/>
      <arg name="property" type="s" direction="in"/>
      <arg name="value" type="v" direction="in"/>
    </method>
    <signal name="PropertiesChanged">
      <arg name="interface" type="s"/>
      <arg name="changed" type="a{sv}"/>
      <arg name="invalidated" type="as"/>
    </signal>
  </interface>
` + introspect.IntrospectDataString + `</node>`
