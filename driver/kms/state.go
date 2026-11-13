//go:build linux && (arm64 || amd64)

package kms

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// state is what a run took that outlives the process: the VT it switched
// to, the VT that showed before, and the backlight's level as it found it.
// The driver writes it to a file once it has taken them, and removes the
// file once it has put them back. A run that finds the file was preceded
// by one that ended without putting them back, as on SIGKILL or a crash
// in Mesa's C code, and puts them back first, so the state it then finds
// is the machine's own.
type state struct {
	VT        int    `json:"vt,omitempty"`
	Prev      int    `json:"prev,omitempty"`
	Opened    bool   `json:"opened,omitempty"`
	Backlight string `json:"backlight,omitempty"`
	Level     int    `json:"level,omitempty"`
}

// statePath returns where the state file goes: under /run for root, under
// the user's runtime directory otherwise, or "" where there is neither.
// A test points it elsewhere.
var statePath = func() string {
	if os.Geteuid() == 0 {
		return "/run/gunim-kms.json"
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "gunim-kms.json")
	}
	return ""
}

// readState reads a state file a run before left, and whether there was
// one.
func readState() (state, bool) {
	p := statePath()
	if p == "" {
		return state{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return state{}, false
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		log.Printf("gunim: kms: %s: %v", p, err)
		return state{}, true
	}
	return s, true
}

// writeState keeps s for a run after this one, should this one end
// without putting it back.
func writeState(s state) error {
	p := statePath()
	if p == "" || s == (state{}) {
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return fmt.Errorf("state file: %w", err)
	}
	return nil
}

// removeState removes the state file, once what it holds is put back.
func removeState() {
	p := statePath()
	if p == "" {
		return
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("gunim: kms: %v", err)
	}
}

// repair puts back what a run before left, as its state file says.
func repair(s state) error {
	errs := []error{repairVT(consoles, s)}
	if s.Backlight != "" && s.Level > 0 {
		b := &backlight{dir: s.Backlight}
		errs = append(errs, b.setLevel(s.Level))
	}
	return errors.Join(errs...)
}
