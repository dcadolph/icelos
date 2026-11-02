// Package queue runs conversion jobs in order with bounded parallelism.
package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/dcadolph/icelos/internal/engine"
)

// State is where a job is in its life.
type State string

// Job states.
const (
	// StateQueued means the job waits for a worker.
	StateQueued State = "queued"
	// StateRunning means a worker is encoding the job.
	StateRunning State = "running"
	// StateDone means the output is in place.
	StateDone State = "done"
	// StateFailed means the encode stopped with an error.
	StateFailed State = "failed"
	// StateCanceled means the job was stopped before it finished.
	StateCanceled State = "canceled"
)

// Finished reports whether s is a final state.
func (s State) Finished() bool {
	return s == StateDone || s == StateFailed || s == StateCanceled
}

// Status is a snapshot of one job.
type Status struct {
	// ID is the queue-assigned job identifier.
	ID string `json:"id"`
	// Job is the conversion request with its output path resolved.
	Job engine.Job `json:"job"`
	// State is the job state.
	State State `json:"state"`
	// Progress is the most recent progress report.
	Progress engine.Progress `json:"progress"`
	// Error is the failure message for failed jobs.
	Error string `json:"error,omitempty"`
}

// NotifyFunc receives a status snapshot after every change. It may be called from several
// goroutines at once and must return quickly.
type NotifyFunc func(Status)

// item is one job inside the queue.
type item struct {
	// status is the current snapshot.
	status Status
	// cancel stops a running encode.
	cancel context.CancelFunc
	// stopped is set when the user asked to cancel.
	stopped bool
}

// Queue holds jobs and hands them to workers.
type Queue struct {
	// enc runs each job.
	enc engine.Encoder
	// workers is how many jobs run at once.
	workers int
	// notify receives every status change.
	notify NotifyFunc
	// mu guards every field below.
	mu sync.Mutex
	// items holds jobs in submission order.
	items []*item
	// next is the last ID handed out.
	next int
	// reserved holds output paths claimed by unfinished jobs.
	reserved map[string]bool
	// changed is closed and replaced on every change to wake idle workers.
	changed chan struct{}
}

// New returns a Queue that runs one job at a time through enc.
func New(enc engine.Encoder, notify NotifyFunc) *Queue {
	return WithWorkers(enc, 1, notify)
}

// WithWorkers returns a Queue that runs up to workers jobs at once through enc. A nil notify
// discards status changes.
func WithWorkers(enc engine.Encoder, workers int, notify NotifyFunc) *Queue {
	if enc == nil {
		panic("queue.WithWorkers: Encoder required")
	}
	if workers < 1 {
		panic("queue.WithWorkers: workers must be at least 1")
	}
	if notify == nil {
		notify = func(Status) {}
	}
	return &Queue{enc: enc, workers: workers, notify: notify, reserved: map[string]bool{},
		changed: make(chan struct{})}
}

// Add queues job. When job.Output is empty the queue picks a free name in outDir, or next to
// the input when outDir is empty, and no two unfinished jobs ever share an output path.
func (q *Queue) Add(job engine.Job, outDir string) (Status, error) {
	if err := job.Preset.Validate(); err != nil {
		return Status{}, err
	}
	fi, err := os.Stat(job.Input)
	if err != nil {
		return Status{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	if !fi.Mode().IsRegular() {
		return Status{}, fmt.Errorf("%w: %s: not a regular file", ErrInput, job.Input)
	}
	if job.Input, err = filepath.Abs(job.Input); err != nil {
		return Status{}, fmt.Errorf("%w: %w", ErrInput, err)
	}

	q.mu.Lock()
	if job.Output == "" {
		job.Output, err = engine.OutputPath(job.Input, job.Preset.Ext, outDir,
			func(p string) bool { return q.reserved[p] })
	} else {
		job.Output, err = filepath.Abs(job.Output)
		if err == nil && q.reserved[job.Output] {
			err = fmt.Errorf("%w: %s", ErrDuplicateOutput, job.Output)
		}
	}
	if err != nil {
		q.mu.Unlock()
		return Status{}, err
	}
	q.next++
	it := &item{status: Status{ID: strconv.Itoa(q.next), Job: job, State: StateQueued}}
	q.items = append(q.items, it)
	q.reserved[job.Output] = true
	st := it.status
	q.signal()
	q.mu.Unlock()

	q.notify(st)
	return st, nil
}

// Cancel stops the job with id. A queued job is marked canceled at once. A running job is
// canceled when its encoder returns. Canceling a finished job is a no-op.
func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	it := q.find(id)
	if it == nil {
		q.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrUnknownJob, id)
	}
	var st Status
	notify := false
	switch it.status.State {
	case StateQueued:
		it.status.State = StateCanceled
		delete(q.reserved, it.status.Job.Output)
		st, notify = it.status, true
		q.signal()
	case StateRunning:
		it.stopped = true
		it.cancel()
	}
	q.mu.Unlock()
	if notify {
		q.notify(st)
	}
	return nil
}

// Clear drops every finished job from the list.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = slices.DeleteFunc(q.items, func(it *item) bool { return it.status.State.Finished() })
}

// List returns a snapshot of every job in submission order.
func (q *Queue) List() []Status {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Status, len(q.items))
	for i, it := range q.items {
		out[i] = it.status
	}
	return out
}

// Serve runs workers until ctx is done, waiting for new jobs when the queue is empty.
func (q *Queue) Serve(ctx context.Context) {
	q.run(ctx, false)
}

// Drain runs workers until no job is queued, then waits for running jobs and returns.
func (q *Queue) Drain(ctx context.Context) {
	q.run(ctx, true)
}

// run starts the workers and waits for them.
func (q *Queue) run(ctx context.Context, drain bool) {
	var wg sync.WaitGroup
	for range q.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.work(ctx, drain)
		}()
	}
	wg.Wait()
}

// work claims and encodes jobs until ctx is done, or until the queue is empty in drain mode.
func (q *Queue) work(ctx context.Context, drain bool) {
	for ctx.Err() == nil {
		it, jobCtx, wake := q.claim(ctx)
		if it != nil {
			q.process(ctx, jobCtx, it)
			continue
		}
		if drain {
			return
		}
		select {
		case <-ctx.Done():
		case <-wake:
		}
	}
}

// claim marks the oldest queued job running and returns it with its own cancelable context.
// When nothing is queued it returns the channel that closes on the next change.
func (q *Queue) claim(ctx context.Context) (*item, context.Context, <-chan struct{}) {
	q.mu.Lock()
	for _, it := range q.items {
		if it.status.State != StateQueued {
			continue
		}
		jobCtx, cancel := context.WithCancel(ctx)
		it.cancel = cancel
		it.status.State = StateRunning
		st := it.status
		q.mu.Unlock()
		q.notify(st)
		return it, jobCtx, nil
	}
	wake := q.changed
	q.mu.Unlock()
	return nil, nil, wake
}

// process encodes one claimed job under jobCtx and records how it ended. A job counts as
// canceled when the user stopped it or the parent ctx ended, and as failed otherwise.
func (q *Queue) process(ctx, jobCtx context.Context, it *item) {
	q.mu.Lock()
	job, cancel := it.status.Job, it.cancel
	q.mu.Unlock()
	defer cancel()

	err := q.enc.Encode(jobCtx, job, func(p engine.Progress) {
		q.mu.Lock()
		it.status.Progress = p
		st := it.status
		q.mu.Unlock()
		q.notify(st)
	})

	q.mu.Lock()
	switch {
	case err == nil:
		it.status.State = StateDone
		it.status.Progress.Percent = 1
	case it.stopped || ctx.Err() != nil:
		it.status.State = StateCanceled
	default:
		it.status.State = StateFailed
		it.status.Error = err.Error()
	}
	delete(q.reserved, job.Output)
	st := it.status
	q.signal()
	q.mu.Unlock()
	q.notify(st)
}

// find returns the item with id, or nil. The caller holds mu.
func (q *Queue) find(id string) *item {
	for _, it := range q.items {
		if it.status.ID == id {
			return it
		}
	}
	return nil
}

// signal wakes every idle worker. The caller holds mu.
func (q *Queue) signal() {
	close(q.changed)
	q.changed = make(chan struct{})
}
