package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The commands that pick a theme.
const (
	CmdThemeDark  = "theme.dark"
	CmdThemeLight = "theme.light"
)

// sideTheme keeps the sidebar's rows as close in either theme.
func sideTheme() theme.Theme {
	return theme.Make("files.side", theme.Set(widget.ListSpacing, 4), theme.Set(widget.RowRadius, 7))
}

// handleTheme takes the commands that pick a theme.
func (a *app) handleTheme(in gunim.Intent) bool {
	v, ok := in.(Command)
	if !ok || v.Name != CmdThemeDark && v.Name != CmdThemeLight {
		return false
	}
	a.setLight(v.Name == CmdThemeLight)
	return true
}

// setLight switches to the light theme or the dark one, which the window
// animates, and saves the choice.
func (a *app) setLight(light bool) {
	if a.shell.Light == light {
		return
	}
	a.shell.Light = light
	a.prefs.Light = light
	a.publishShell()
	a.savePrefs()
}
