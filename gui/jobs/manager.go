package jobs

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// progressPollInterval is how often a running job's progress function is
// sampled to update the percentage shown on the Jobs screen.
const progressPollInterval = 100 * time.Millisecond

// JobManager manages the list of active jobs.
type JobManager struct {
	mu       sync.RWMutex
	jobs     []*Job
	onChange atomic.Value
}

// NewJobManager creates a new job manager.
func NewJobManager() *JobManager {
	return &JobManager{}
}

// SetOnChange sets the callback to be called when the jobs list changes.
func (this *JobManager) SetOnChange(fn func()) {
	this.onChange.Store(fn)
}

// NotifyChange calls the onChange callback if set.
func (this *JobManager) NotifyChange() {
	if fn, ok := this.onChange.Load().(func()); ok && fn != nil {
		fn()
	}
}

// addJob adds a job to the active list. It returns an error when a job with
// the same name and type is already active; in that case the job is not added.
func (this *JobManager) addJob(job *Job) error {
	this.mu.Lock()
	defer this.mu.Unlock()
	for _, existing := range this.jobs {
		if existing.Type == job.Type && existing.Name == job.Name {
			return fmt.Errorf("duplicate job: type=%s, name=%s", job.Type, job.Name)
		}
	}
	this.jobs = append(this.jobs, job)
	return nil
}

// RemoveJob removes a job from the list.
func (this *JobManager) RemoveJob(job *Job) {
	if job == nil {
		return
	}
	this.mu.Lock()
	for i, j := range this.jobs {
		if j == job {
			this.jobs = append(this.jobs[:i], this.jobs[i+1:]...)
			break
		}
	}
	this.mu.Unlock()
	this.NotifyChange()
}

// ActiveJobs returns a snapshot of current jobs.
func (this *JobManager) ActiveJobs() []Job {
	this.mu.RLock()
	defer this.mu.RUnlock()
	jobs := make([]Job, len(this.jobs))
	for i, job := range this.jobs {
		jobs[i] = *job
	}
	return jobs
}

// StartJob starts a job in a goroutine and adds it to the active list.
//
// jobName and jobType identify the job: if a job with the same name and type
// is already active, StartJob returns an error and the new job does not start.
// description is shown on the Jobs screen.
//
// job does the work and returns its result. onComplete, when non-nil, runs in
// its own goroutine after the job finishes, and the job stays listed until it
// returns. progress, when non-nil, is polled while the job runs and returns
// the job's percentage (0-100), or -1 when the progress is unknown; it
// defaults to always reporting -1.
func (this *JobManager) StartJob(jobName string, jobType string, description string, job func() error, onComplete func(err error), progress func() int) error {
	if progress == nil {
		progress = func() int { return -1 }
	}
	entry := &Job{
		Type:        jobType,
		Name:        jobName,
		Description: description,
		Started:     time.Now(),
		Progress:    -1,
	}
	if err := this.addJob(entry); err != nil {
		return err
	}
	this.NotifyChange()

	go func() {
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(progressPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					this.setProgress(entry, progress())
				}
			}
		}()

		err := job()
		close(done)

		this.mu.Lock()
		entry.Error = err
		entry.Completed = true
		entry.Progress = 100
		this.mu.Unlock()
		this.NotifyChange()

		if onComplete == nil {
			this.RemoveJob(entry)
			return
		}
		go func() {
			onComplete(err)
			this.RemoveJob(entry)
		}()
	}()
	return nil
}

// setProgress updates a running job's progress, unless the job has already
// finished (so a late sample cannot overwrite the final 100%).
func (this *JobManager) setProgress(job *Job, progress int) {
	this.mu.Lock()
	if job.Completed {
		this.mu.Unlock()
		return
	}
	job.Progress = progress
	this.mu.Unlock()
	this.NotifyChange()
}
