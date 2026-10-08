package install

import (
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/widget"
)

// The toasts below tell of updates in the program's own window, as a
// [widget.Toasts] shows them, in the same words in every program. Each
// stays until the user answers it or closes it, and each takes the
// place of the one before, by its key, "update".

// AvailableToast asks whether to fetch version, a newer release told of
// through [App.Available]. Update sends fetch, for the program to put
// it in place with [Stage].
func AvailableToast(a App, version string, fetch gunim.Intent) widget.Toast {
	return widget.Toast{Title: a.Name + " " + bare(version) + " is out",
		Body: "Fetch it now? It starts the next time you open " + a.Name + ".",
		Key:  "update", Icon: icon.Download, Buttons: []widget.ToastButton{
			{Label: "Update", OnClick: func(bool, *gunim.UI) gunim.Intent { return fetch }},
			{Label: "Not Now"},
		}}
}

// ReadyToast tells of version, put in place for the next start, as
// [App.Updated] tells of it. Restart Now sends restart, for the program
// to [Restart] into it.
func ReadyToast(a App, version string, restart gunim.Intent) widget.Toast {
	return widget.Toast{Title: a.Name + " " + bare(version) + " is ready",
		Body: "Restart now, or it starts the next time you open " + a.Name + ".",
		Key:  "update", Icon: icon.RefreshCw, Buttons: []widget.ToastButton{
			{Label: "Restart Now", OnClick: func(bool, *gunim.UI) gunim.Intent { return restart }},
			{Label: "Later"},
		}}
}

// UpdatedToast tells, on the first start of a release an update put in
// place by itself, that the program runs it now, in place of version
// from, as [UpdatedFrom] names it. What's New sends whatsNew, for the
// program to show what changed with [ShowWhatsNew].
func UpdatedToast(a App, from string, whatsNew gunim.Intent) widget.Toast {
	return widget.Toast{Title: a.Name + " is updated to " + bare(a.Version),
		Body: "It updated itself from " + bare(from) + ".",
		Key:  "update", Kind: widget.ToastSuccess, Buttons: []widget.ToastButton{
			{Label: "What's New", OnClick: func(bool, *gunim.UI) gunim.Intent { return whatsNew }},
			{Label: "OK"},
		}}
}

// bare is a version without its v.
func bare(version string) string { return strings.TrimPrefix(version, "v") }
