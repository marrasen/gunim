// Package inbox queues a window's input for the engine.
package inbox

import "sync"

// An Inbox carries input from the thread a platform delivers it on to
// the engine.
//
// It queues without bound, and a goroutine of its own feeds the
// channel. So the platform's thread, which pumps events for every
// window, always hands input over at once, and a window slow to read
// its input leaves the others running.
type Inbox struct {
	out  chan any
	quit <-chan struct{}
	wake chan struct{}

	mu     sync.Mutex
	items  []any
	closed bool
}

// New returns an inbox that stops feeding when quit closes.
func New(quit <-chan struct{}) *Inbox {
	q := &Inbox{out: make(chan any), quit: quit, wake: make(chan struct{}, 1)}
	go q.feed()
	return q
}

// Push queues ev.
func (q *Inbox) Push(ev any) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, ev)
	q.mu.Unlock()
	q.nudge()
}

// Close ends the stream once what is queued has been delivered.
func (q *Inbox) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.nudge()
}

func (q *Inbox) nudge() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Inbox) feed() {
	for {
		q.mu.Lock()
		if len(q.items) == 0 {
			closed := q.closed
			q.mu.Unlock()
			if closed {
				close(q.out)
				return
			}
			select {
			case <-q.wake:
			case <-q.quit:
				return
			}
			continue
		}
		ev := q.items[0]
		q.items = q.items[1:]
		q.mu.Unlock()
		select {
		case q.out <- ev:
		case <-q.quit:
			return
		}
	}
}

// Out is the channel the input arrives on, closed after Close once
// what was queued has been delivered.
func (q *Inbox) Out() <-chan any { return q.out }
