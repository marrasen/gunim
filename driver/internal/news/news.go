// Package news hands on news from a thread a platform tells it on to functions of an application's, on goroutines of
// their own, so that a slow one keeps no platform waiting.
package news

import "sync"

// Latest is news of which only the last counts. The zero value has none yet.
type Latest[T any] struct {
	// Merge, where it is set, returns what stands in the place of old, which fn has not heard yet, and next. Without
	// it next does.
	Merge func(old, next T) T
	// First, where it is set, returns what one that starts to watch hears of v, set before.
	First func(v T) T

	mu       sync.Mutex
	now      T
	has      bool
	watchers map[*watcher[T]]bool
}

// watcher is one that hears the news. pending is what it has yet to hear.
type watcher[T any] struct {
	fn      func(T)
	pending *T
	running bool
}

// Set tells v to each that watches, and to those that start to.
func (l *Latest[T]) Set(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now, l.has = v, true
	for w := range l.watchers {
		l.tell(w, v)
	}
}

// Forget has those that start to watch hear nothing until the next Set.
func (l *Latest[T]) Forget() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.has = false
}

// Watch has fn hear the news there is, and each from then on, one call at a time, until stop. A call
// that has begun as stop is called goes on. stop may be called from fn, and more than once.
func (l *Latest[T]) Watch(fn func(T)) (stop func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := &watcher[T]{fn: fn}
	if l.watchers == nil {
		l.watchers = map[*watcher[T]]bool{}
	}
	l.watchers[w] = true
	if l.has {
		v := l.now
		if l.First != nil {
			v = l.First(v)
		}
		l.tell(w, v)
	}
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.watchers, w)
		w.pending = nil
	}
}

// tell has w hear v. It runs with mu held.
func (l *Latest[T]) tell(w *watcher[T], v T) {
	if w.pending != nil && l.Merge != nil {
		v = l.Merge(*w.pending, v)
	}
	w.pending = &v
	if !w.running {
		w.running = true
		go l.run(w)
	}
}

// run calls w's function until there is no more for it to hear.
func (l *Latest[T]) run(w *watcher[T]) {
	for {
		l.mu.Lock()
		if w.pending == nil {
			w.running = false
			l.mu.Unlock()
			return
		}
		v := *w.pending
		w.pending = nil
		l.mu.Unlock()
		w.fn(v)
	}
}

// Queue is news all of which counts, for one function, told in order. It keeps the last max until there is a function
// to hear them. The zero value with a Max is ready.
type Queue[T any] struct {
	Max int

	mu      sync.Mutex
	fn      func(T)
	waiting []T
	running bool
}

// Add tells v.
func (q *Queue[T]) Add(v T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.waiting = append(q.waiting, v)
	if over := len(q.waiting) - q.Max; over > 0 {
		q.waiting = append([]T(nil), q.waiting[over:]...)
	}
	q.start()
}

// Hear has fn hear what waits and what comes, in the place of the one before; nil has the news wait.
func (q *Queue[T]) Hear(fn func(T)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.fn = fn
	q.start()
}

// start runs run where there is something to tell. It runs with mu held.
func (q *Queue[T]) start() {
	if q.fn != nil && len(q.waiting) > 0 && !q.running {
		q.running = true
		go q.run()
	}
}

// run tells what waits.
func (q *Queue[T]) run() {
	for {
		q.mu.Lock()
		if q.fn == nil || len(q.waiting) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		v, fn := q.waiting[0], q.fn
		q.waiting = q.waiting[1:]
		q.mu.Unlock()
		fn(v)
	}
}
