package main

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/marrasen/gunim"
)

// serve is the application half.
//
// It runs on its own goroutine and reaches the window only through
// [gunim.Client], where every method takes data and returns at once.
// Whatever it does with an intent, the window keeps drawing.
func serve(ctx context.Context, c gunim.Client) error {
	s := &store{
		jobs: []Job{
			{ID: "1", Title: "Reindex archive", Status: JobPending},
			{ID: "2", Title: "Rebuild thumbnails", Status: JobPending},
			{ID: "3", Title: "Vacuum database", Status: JobPending},
		},
	}

	// The view watches a topic, so one publish reaches it and anything
	// else showing the same data.
	if err := c.Mount(gunim.Root, "jobs", "joblist", s.list(), JobsTopic); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			handle(ctx, c, s, ev)
		}
	}
}

func handle(ctx context.Context, c gunim.Client, s *store, ev gunim.Envelope) {
	switch {
	case is[RunRequested](ev):
		v, _ := gunim.As[RunRequested](ev)
		go runJob(ctx, c, s, v.ID)

	case is[DeleteRequested](ev):
		v, _ := gunim.As[DeleteRequested](ev)
		job, ok := s.get(v.ID)
		if !ok {
			return
		}
		s.pending = v.ID
		if err := c.Mount(gunim.Root, "confirm", "confirm", ConfirmState{Title: "Delete " + job.Title + "?"}); err != nil {
			log.Printf("mount confirm: %v", err)
			return
		}
		_ = c.Focus("confirm")

	case is[Confirmed](ev):
		// The dialog closed itself the moment OK was released, so it is
		// already fading while this runs. The publish that follows
		// lands on a list that is still animating, and the row it
		// removes collapses into the gap.
		s.remove(s.pending)
		_ = c.Publish(JobsTopic, s.list())

	case is[ThemeToggled](ev):
		// Which theme shows is application state, like any other; the
		// window animates the switch.
		s.light = !s.light
		name := darkTheme().Name
		if s.light {
			name = lightTheme().Name
		}
		_ = c.SetTheme(name)

	case is[Cancelled](ev):
		s.pending = ""

	case is[gunim.CommandFailed](ev):
		v, _ := gunim.As[gunim.CommandFailed](ev)
		log.Printf("command %s failed on %q%q: %s", v.Command, v.ID, v.Key, v.Reason)
	}
}

// is reports whether ev carries a T.
func is[T any](ev gunim.Envelope) bool {
	_, ok := gunim.As[T](ev)
	return ok
}

// runJob does the slow work, on a goroutine of its own.
//
// It sleeps, publishes and patches, and the window animates through all
// of it. Progress goes out as a patch, so each tick retargets one
// spring on one row. The two status changes go out as a publish,
// because those change what the row is rather than where a value sits.
func runJob(ctx context.Context, c gunim.Client, s *store, id string) {
	if ok := s.setStatus(id, JobRunning); !ok {
		return
	}
	_ = c.Publish(JobsTopic, s.list())

	for step := 1; step <= 20; step++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
		done := float32(step) / 20
		s.setProgress(id, done)
		_ = c.Patch(JobsTopic, JobProgress{ID: id, Progress: done})
	}

	s.setStatus(id, JobDone)
	_ = c.Publish(JobsTopic, s.list())
}

// store is the application's data, reached from the serve goroutine and
// from each running job.
type store struct {
	mu   sync.Mutex
	jobs []Job
	// pending is the job the open dialog is asking about, and light
	// whether the light theme is showing. Only serve touches them.
	pending string
	light   bool
}

func (s *store) list() JobList {
	s.mu.Lock()
	defer s.mu.Unlock()
	return JobList{Jobs: slices.Clone(s.jobs)}
}

func (s *store) get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}

func (s *store) setStatus(id string, status JobStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			s.jobs[i].Status = status
			return true
		}
	}
	return false
}

func (s *store) setProgress(id string, done float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			s.jobs[i].Progress = done
			return
		}
	}
}

func (s *store) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = slices.DeleteFunc(s.jobs, func(j Job) bool { return j.ID == id })
}
