//go:build !windows || !(amd64 || arm64)

package filemanager

import "errors"

// errNoOpenWith says the system does not offer its programs for a file.
var errNoOpenWith = errors.New("this system does not offer its programs for a file")

func sysOpenWithWorks() bool                        { return false }
func sysOpenWithApps(string) ([]OpenWithApp, error) { return nil, errNoOpenWith }
func sysOpenWithApp(string, string) error           { return errNoOpenWith }
func sysOpenWithDialog(string) error                { return errNoOpenWith }
