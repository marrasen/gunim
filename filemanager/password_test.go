package filemanager

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/marrasen/gunim/zipcrypt"
)

// copyTestZip copies an archive of zipcrypt's tests, protected with the
// password hunter2, into dir as name.
func copyTestZip(t *testing.T, from, dir, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "zipcrypt", "testdata", from))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// passwordShown waits for the window's password dialog, and returns it.
func (h *harness) passwordShown() PasswordPrompt {
	h.t.Helper()
	h.until("the password is asked for", func() bool {
		if len(h.a.ops.dialogs) == 0 {
			return false
		}
		_, ok := h.a.ops.dialogs[0].state.(PasswordPrompt)
		return ok
	})
	p, _ := h.a.ops.dialogs[0].state.(PasswordPrompt)
	return p
}

// extract starts extracting the archive called name into a folder
// called into.
func (h *harness) extract(name, into string) {
	h.t.Helper()
	h.do(Command{Name: CmdRefresh})
	h.until("the zip shows", func() bool { return slices.Contains(h.shown(), name) })
	h.pick(name)
	h.do(Command{Name: CmdExtract})
	h.until("the prompt shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	p, ok := h.a.ops.dialogs[0].state.(Prompt)
	if !ok {
		h.t.Fatalf("the dialog is %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(Prompted{Token: p.Token, Text: into, OK: true})
}

// Extract asks for the password of a zip that has one, again while the
// one given is wrong, and makes nothing until one is right.
func TestExtractAsksForThePasswordUntilItIsRight(t *testing.T) {
	for _, zipped := range []string{"legacy.zip", "aes256.zip"} {
		t.Run(zipped, func(t *testing.T) {
			h := newHarness(t, "keep.txt")
			copyTestZip(t, zipped, h.dir, "secret.zip")
			h.extract("secret.zip", "out")
			p := h.passwordShown()
			if p.Make || p.Problem != "" {
				t.Fatalf("the first question is %+v", p)
			}
			h.answer(PasswordGiven{Token: p.Token, Text: "wrong", OK: true})
			again := h.passwordShown()
			if again.Token == p.Token || again.Problem == "" {
				t.Fatalf("a wrong password was asked about as %+v", again)
			}
			if h.exists("out") {
				t.Fatal("the folder was made before the password was right")
			}
			h.answer(PasswordGiven{Token: again.Token, Text: "hunter2", OK: true})
			h.until("the zip is extracted", func() bool { return h.exists("out/tiny.txt") && len(h.a.ops.running) == 0 })
			if got := contents(t, filepath.Join(h.dir, "out", "tiny.txt")); got != "tiny" {
				t.Fatalf("tiny.txt came out holding %q", got)
			}
		})
	}
}

// Cancelling the password stops the extraction, and leaves nothing.
func TestCancellingThePasswordLeavesNothing(t *testing.T) {
	h := newHarness(t, "keep.txt")
	copyTestZip(t, "aes256.zip", h.dir, "secret.zip")
	h.extract("secret.zip", "out")
	p := h.passwordShown()
	h.answer(PasswordGiven{Token: p.Token})
	h.until("the operation ends", func() bool { return len(h.a.ops.running) == 0 })
	if h.exists("out") {
		t.Fatal("a folder was made for an archive that was not opened")
	}
	if len(h.a.ops.dialogs) != 0 {
		t.Fatalf("a dialog shows after the cancel: %+v", h.a.ops.dialogs[0].state)
	}
}

// Create zip with its box ticked asks for a password twice, and
// protects what the zip holds with it.
func TestCreateZipProtectsWithAPassword(t *testing.T) {
	h := newHarness(t, "notes.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.pick("notes.txt")
	h.do(Command{Name: CmdZip})
	h.until("the prompt shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	p, ok := h.a.ops.dialogs[0].state.(Prompt)
	if !ok || p.Check == "" {
		t.Fatal("Create zip offers no password")
	}
	h.answer(Prompted{Token: p.Token, Text: "locked", OK: true, Checked: true})
	pw := h.passwordShown()
	if !pw.Make {
		t.Fatalf("the password for a new zip is asked as %+v", pw)
	}
	h.answer(PasswordGiven{Token: pw.Token, Text: "s3cret", OK: true})
	h.until("the zip is made", func() bool { return h.exists("locked.zip") && len(h.a.ops.running) == 0 })
	zr, err := zip.OpenReader(filepath.Join(h.dir, "locked.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	f := zr.File[0]
	if !zipcrypt.Encrypted(f) {
		t.Fatal("the zip is not protected")
	}
	rc, err := zipcrypt.Open(f, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	if b, err := io.ReadAll(rc); err != nil || string(b) != "notes.txt" {
		t.Fatalf("the zip holds %q, %v", b, err)
	}
}

// A program that asks for passwords itself is asked, in place of the
// window's dialog, and told once the password worked.
func TestAProgramIsAskedForThePassword(t *testing.T) {
	var asked []PasswordAsk
	var worked atomic.Int32
	h := newHarnessWith(t, func(o *Options) {
		o.Password = func(_ context.Context, _ *Window, ask PasswordAsk) (Password, error) {
			asked = append(asked, ask)
			if !ask.Wrong {
				return Password{Text: "nope"}, nil
			}
			return Password{Text: "hunter2", Worked: func() { worked.Add(1) }}, nil
		}
	}, "keep.txt")
	copyTestZip(t, "legacy.zip", h.dir, "secret.zip")
	h.extract("secret.zip", "out")
	h.until("the zip is extracted", func() bool { return h.exists("out/fox.txt") && len(h.a.ops.running) == 0 })
	if len(asked) != 2 || asked[0].Wrong || !asked[1].Wrong || asked[0].Path != filepath.Join(h.dir, "secret.zip") || asked[0].Make {
		t.Fatalf("the program was asked %+v", asked)
	}
	if worked.Load() != 1 {
		t.Fatalf("the program was told %d times that the password worked", worked.Load())
	}
}
