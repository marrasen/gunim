package gunim

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/marrasen/gunim/driver"
)

// lockDriver is a driver with a screen lock kept in memory, as an application's test builds one.
type lockDriver struct {
	pumpDriver
	driver.OffscreenConfirmer
}

// A prompt that is confirmed returns nil, and any other end an error.
func TestOnlyAConfirmedPromptReturnsNil(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := &lockDriver{}
		d.SetScreenLock(true)
		a := &App{drv: d}
		for _, yes := range []bool{true, false} {
			go func() {
				p := <-d.Prompts()
				if p.Title != "Open the notes" || p.Text != "They are locked" {
					t.Errorf("the prompt asks %+v", p.ConfirmOptions)
				}
				p.Answer(yes)
				p.Answer(!yes)
			}()
			err := a.ConfirmPerson(t.Context(), ConfirmOptions{Title: "Open the notes", Text: "They are locked"})
			if yes && err != nil || !yes && !errors.Is(err, ErrNotConfirmed) {
				t.Errorf("answered %v, it returned %v", yes, err)
			}
		}
		// One whose time is out goes away, and an answer after that does nothing.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		late := make(chan driver.ConfirmPrompt, 1)
		go func() { late <- <-d.Prompts() }()
		if err := a.ConfirmPerson(ctx, ConfirmOptions{}); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("one whose time is out returned %v", err)
		}
		p := <-late
		<-p.Gone
		p.Answer(true)
		if err := a.ConfirmPerson(ctx, ConfirmOptions{}); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("one with a context that has ended returned %v", err)
		}
	})
}

// There is one prompt at a time, and the next is shown once it has gone.
func TestOnePromptToConfirmAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := &lockDriver{}
		d.SetScreenLock(true)
		a := &App{drv: d}
		first := make(chan error, 1)
		go func() { first <- a.ConfirmPerson(t.Context(), ConfirmOptions{Title: "1"}) }()
		p := <-d.Prompts()
		if err := a.ConfirmPerson(t.Context(), ConfirmOptions{Title: "2"}); !errors.Is(err, ErrConfirming) {
			t.Errorf("a second prompt returned %v", err)
		}
		p.Answer(false)
		if err := <-first; !errors.Is(err, ErrNotConfirmed) {
			t.Errorf("the first returned %v", err)
		}
		go func() { (<-d.Prompts()).Answer(true) }()
		if err := a.ConfirmPerson(t.Context(), ConfirmOptions{Title: "3"}); err != nil {
			t.Errorf("the next returned %v", err)
		}
	})
}

func TestNoPromptWithNoScreenLockOrNoSystemForIt(t *testing.T) {
	d := &lockDriver{}
	a := &App{drv: d}
	if on, err := a.HasScreenLock(); on || err != nil {
		t.Errorf("with no lock it says %v, %v", on, err)
	}
	if err := a.ConfirmPerson(t.Context(), ConfirmOptions{}); !errors.Is(err, ErrNoScreenLock) {
		t.Errorf("with no lock it returned %v", err)
	}
	d.SetScreenLock(true)
	if on, err := a.HasScreenLock(); !on || err != nil {
		t.Errorf("with a lock it says %v, %v", on, err)
	}
	if err := a.ConfirmPerson(t.Context(), ConfirmOptions{Title: "\xff"}); !errors.Is(err, driver.ErrConfirmText) {
		t.Errorf("a title that is not UTF-8 returned %v", err)
	}
	a = &App{drv: &pumpDriver{}}
	if _, err := a.HasScreenLock(); !errors.Is(err, ErrNoConfirmer) {
		t.Errorf("a platform that cannot tell returned %v", err)
	}
	if err := a.ConfirmPerson(t.Context(), ConfirmOptions{}); !errors.Is(err, ErrNoConfirmer) {
		t.Errorf("a platform with no prompt returned %v", err)
	}
}
