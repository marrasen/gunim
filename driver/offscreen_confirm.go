package driver

import (
	"context"
	"sync"
)

// OffscreenConfirmer asks a test in the user's place: it implements [Confirmer], and a test's [Driver] embeds it. The
// zero value has no screen lock.
type OffscreenConfirmer struct {
	mu    sync.Mutex
	lock  bool
	asked chan ConfirmPrompt
	busy  bool
}

// ConfirmPrompt is a prompt an [OffscreenConfirmer] shows.
type ConfirmPrompt struct {
	ConfirmOptions
	// Gone is closed as the prompt goes without an answer, as its context ends.
	Gone   <-chan struct{}
	answer chan<- bool
}

// Answer answers the prompt, as confirmed or not. It does nothing for one that is gone.
func (p ConfirmPrompt) Answer(confirmed bool) {
	select {
	case p.answer <- confirmed:
	default:
	}
}

// SetScreenLock says whether the device has a screen lock.
func (o *OffscreenConfirmer) SetScreenLock(on bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lock = on
}

// HasScreenLock implements [Confirmer].
func (o *OffscreenConfirmer) HasScreenLock() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lock
}

// Prompts returns the prompts shown, for the test to answer.
func (o *OffscreenConfirmer) Prompts() <-chan ConfirmPrompt {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.shownLocked()
}

// shownLocked is Prompts with mu held.
func (o *OffscreenConfirmer) shownLocked() chan ConfirmPrompt {
	if o.asked == nil {
		o.asked = make(chan ConfirmPrompt)
	}
	return o.asked
}

// ConfirmPerson implements [Confirmer].
func (o *OffscreenConfirmer) ConfirmPerson(ctx context.Context, c ConfirmOptions) error {
	if err := CheckConfirm(c); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	lock, busy, asked := o.lock, o.busy, o.shownLocked()
	if lock && !busy {
		o.busy = true
	}
	o.mu.Unlock()
	switch {
	case !lock:
		return ErrNoScreenLock
	case busy:
		return ErrConfirming
	}
	gone, answer := make(chan struct{}), make(chan bool, 1)
	defer func() {
		close(gone)
		o.mu.Lock()
		defer o.mu.Unlock()
		o.busy = false
	}()
	select {
	case asked <- ConfirmPrompt{ConfirmOptions: c, Gone: gone, answer: answer}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case ok := <-answer:
		if !ok {
			return ErrNotConfirmed
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
