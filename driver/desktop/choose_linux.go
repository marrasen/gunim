//go:build linux

package desktop

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

// portalFilter is a FileChooser filter: a name and its rules, each a kind
// (0 for a glob) and a pattern.
type portalFilter struct {
	Name  string
	Rules []portalRule
}

type portalRule struct {
	Kind    uint32
	Pattern string
}

// ChooseFiles implements [driver.FileChooser] with the desktop portal's
// FileChooser, which draws the desktop's own dialog.
func (w *Window) ChooseFiles(o driver.ChooseOptions) ([]string, error) {
	var xid uintptr
	if err := w.d.call(func() error {
		x, err := w.gw.GetX11Window()
		xid = x
		return err
	}); err != nil {
		return nil, fmt.Errorf("desktop: finding the window for a file dialog: %w", err)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("desktop: reaching the session bus for a file dialog: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// The portal answers on a request object named from the caller and a
	// token, so the answer is watched for before the question is asked.
	token := fmt.Sprintf("gunim%d", rand.Uint32())
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	request := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
	answers := make(chan *dbus.Signal, 4)
	conn.Signal(answers)
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return nil, fmt.Errorf("desktop: listening for the file dialog: %w", err)
	}

	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"multiple":     dbus.MakeVariant(o.Multiple),
		"directory":    dbus.MakeVariant(o.Folders),
		"modal":        dbus.MakeVariant(true),
	}
	if len(o.Filters) > 0 && !o.Folders {
		filters := make([]portalFilter, len(o.Filters))
		for i, f := range o.Filters {
			filters[i].Name = f.Name
			for _, p := range f.Patterns {
				filters[i].Rules = append(filters[i].Rules, portalRule{Pattern: p})
			}
		}
		opts["filters"] = dbus.MakeVariant(filters)
	}
	var handle dbus.ObjectPath
	portal := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	if err := portal.Call("org.freedesktop.portal.FileChooser.OpenFile", 0,
		fmt.Sprintf("x11:%x", xid), o.Title, opts).Store(&handle); err != nil {
		return nil, fmt.Errorf("desktop: opening the file dialog: %w", err)
	}
	// An older portal names the request its own way, and says which.
	if handle != "" {
		request = handle
	}
	for sig := range answers {
		if sig.Path != request || len(sig.Body) < 2 {
			continue
		}
		code, _ := sig.Body[0].(uint32)
		switch code {
		case 0:
			return portalPaths(sig.Body[1])
		case 1:
			return nil, nil
		default:
			return nil, fmt.Errorf("desktop: the file dialog ended with response %d", code)
		}
	}
	return nil, errors.New("desktop: the session bus closed before the file dialog answered")
}

// portalPaths returns the paths of the file URIs in a FileChooser answer.
func portalPaths(results any) ([]string, error) {
	fields, ok := results.(map[string]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("desktop: the file dialog answered with %T", results)
	}
	var uris []string
	if v, ok := fields["uris"]; ok {
		if err := v.Store(&uris); err != nil {
			return nil, fmt.Errorf("desktop: reading the file dialog's answer: %w", err)
		}
	}
	out := make([]string, 0, len(uris))
	for _, s := range uris {
		u, err := url.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("desktop: the file dialog chose %q: %w", s, err)
		}
		if u.Scheme != "file" {
			return nil, fmt.Errorf("desktop: the file dialog chose %q, which is not a local file", s)
		}
		out = append(out, u.Path)
	}
	return out, nil
}
