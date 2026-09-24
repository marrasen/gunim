// Command dialog shows the split gunim is built around.
//
// The window and the application are two halves that speak only in
// values. protocol.go holds the vocabulary they share, ui.go is the
// window half, app.go is the application half, and main.go joins them
// over a channel in one process.
//
// Cutting between the halves is what a socket would replace: the
// application half could move to another machine and neither half would
// change. In this process the values cross as they are, and
// wire_test.go proves each one would also survive a socket.
//
// Running it opens a window with three jobs. Run one, and when it is
// done, delete it through the confirm dialog.
package main

import "github.com/marrasen/gunim"

// The shared vocabulary. Both halves import it, so one Go declaration
// serves the window and the application at once. This is the step aprot
// has to generate when the other half is TypeScript.

// JobsTopic is the key both halves use for the job list. Views watch
// it, and the application publishes to it.
const JobsTopic = "jobs"

// JobStatus is where a job has got to.
type JobStatus string

// The states a job moves through.
const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
)

// Job is one row's worth of data.
type Job struct {
	ID       string
	Title    string
	Status   JobStatus
	Progress float32
}

// JobList is the state of the whole list, published to [JobsTopic]
// whenever a job is added, removed or changes status.
type JobList struct {
	Jobs []Job
}

// JobProgress is a patch: one job moved along, and the shape of the
// list stayed put.
//
// Sending this instead of a whole JobList is the difference between a
// spring retargeting on one row and every row reconciling. It is the
// same split aprot draws between PatchSubscription and a refresh.
type JobProgress struct {
	ID       string
	Progress float32
}

// ConfirmState is what the confirm view renders.
type ConfirmState struct {
	Title string
}

// RunRequested travels when the user starts a job.
type RunRequested struct {
	ID string
}

// DeleteRequested travels when the user asks to remove a job.
type DeleteRequested struct {
	ID string
}

// Confirmed travels when the user confirms the dialog.
type Confirmed struct {
	What string
}

// Cancelled travels when the user backs out of the dialog.
type Cancelled struct{}

func init() {
	gunim.RegisterType[JobList]("job.list")
	gunim.RegisterType[ConfirmState]("confirm.state")
	gunim.RegisterType[JobProgress]("job.progress")
	gunim.RegisterType[RunRequested]("job.run")
	gunim.RegisterType[DeleteRequested]("job.delete")
	gunim.RegisterType[Confirmed]("confirm.accept")
	gunim.RegisterType[Cancelled]("confirm.dismiss")
}
