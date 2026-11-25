// Package pace posts messages no faster than a system takes their updates.
//
// Android drops an update of a message that shows, without a word, where the application posts more than about five a
// second, by an average over the posts before. The last update says that work is done, so it must not be dropped.
package pace

import (
	"sync"
	"time"
)

// least is the average time between posts that an update waits for. Android drops one under 0.2 s.
const least = 280 * time.Millisecond

// weight is how much of the average stays as a post comes, as Android counts it.
const weight = 0.7

// maxKnown is how many tags are kept of those posted.
const maxKnown = 512

// Pace posts messages of type T by their tags. The first with a tag goes at once. One after it waits where it must, and
// a later one takes the place of one that waits.
type Pace[T any] struct {
	// Post posts v. It is called for one message at a time.
	Post func(tag string, v T) error

	mu      sync.Mutex
	known   map[string]bool
	waiting map[string]T
	// order is the tags that wait, the first to go first.
	order []string
	// last is when the last was posted, and mean the average time between posts then.
	last time.Time
	mean time.Duration
	// done is closed as the goroutine that posts what waits ends, and is nil with none.
	done chan struct{}
}

// Update posts v under tag, or has it wait. It returns what Post did, or nil for one that waits.
func (p *Pace[T]) Update(tag string, v T) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.waiting[tag]; ok {
		p.waiting[tag] = v
		return nil
	}
	if !p.known[tag] || p.wait() <= 0 && len(p.order) == 0 {
		return p.post(tag, v)
	}
	if p.waiting == nil {
		p.waiting = map[string]T{}
	}
	p.waiting[tag] = v
	p.order = append(p.order, tag)
	if p.done == nil {
		p.done = make(chan struct{})
		go p.run(p.done)
	}
	return nil
}

// End leaves out what waits under tag, and runs f with nothing posted meanwhile.
func (p *Pace[T]) End(tag string, f func() error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.known, tag)
	if _, ok := p.waiting[tag]; ok {
		delete(p.waiting, tag)
		for i, t := range p.order {
			if t == tag {
				p.order = append(p.order[:i], p.order[i+1:]...)
				break
			}
		}
	}
	return f()
}

// Drain returns once all that waits has been posted.
func (p *Pace[T]) Drain() {
	for {
		p.mu.Lock()
		done := p.done
		p.mu.Unlock()
		if done == nil {
			return
		}
		<-done
	}
}

// run posts what waits, each in its time.
func (p *Pace[T]) run(done chan struct{}) {
	for {
		p.mu.Lock()
		if len(p.order) == 0 {
			p.done = nil
			close(done)
			p.mu.Unlock()
			return
		}
		if d := p.wait(); d > 0 {
			p.mu.Unlock()
			time.Sleep(d)
			continue
		}
		tag := p.order[0]
		p.order = p.order[1:]
		v := p.waiting[tag]
		delete(p.waiting, tag)
		// An update that fails has no one to tell.
		_ = p.post(tag, v)
		p.mu.Unlock()
	}
}

// wait returns how long an update waits from now. It runs with mu held.
func (p *Pace[T]) wait() time.Duration {
	if p.last.IsZero() {
		return 0
	}
	need := time.Duration((float64(least) - weight*float64(p.mean)) / (1 - weight))
	return need - time.Since(p.last)
}

// post posts v now. It runs with mu held.
func (p *Pace[T]) post(tag string, v T) error {
	if p.last.IsZero() {
		// What an earlier run posted is not known, and counts as just within.
		p.mean = least
	} else {
		// A long time with no post counts for no more than a second, as it would let many through at once.
		since := min(time.Since(p.last), time.Second)
		p.mean = min(time.Duration(weight*float64(p.mean)+(1-weight)*float64(since)), least)
	}
	p.last = time.Now()
	err := p.Post(tag, v)
	if err == nil {
		if p.known == nil || len(p.known) >= maxKnown {
			p.known = map[string]bool{}
		}
		p.known[tag] = true
	}
	return err
}
