package jobs

import (
	"fmt"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kncept/quesadilla/utils/qhttp"
)

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

// AddJob adds a new job with the given type-specific metadata and returns
// it. Returns an error if a job with the same type and name already exists
// (duplicate).
func (this *JobManager) AddJob(jobType JobType, name string, metadata any) (*Job, error) {
	this.mu.RLock()
	for _, j := range this.jobs {
		if j.Type == jobType && j.Name == name {
			this.mu.RUnlock()
			return nil, fmt.Errorf("duplicate job: type=%s, name=%s", jobType, name)
		}
	}
	this.mu.RUnlock()

	job := &Job{
		Type:     jobType,
		Name:     name,
		Metadata: metadata,
		Started:  time.Now(),
		Progress: -1,
	}
	fmt.Printf("Job added: %+v\n", job)
	this.mu.Lock()
	this.jobs = append(this.jobs, job)
	this.mu.Unlock()
	this.NotifyChange()
	return job, nil
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

// StartDownloadJob starts a file download job in a goroutine.
func (this *JobManager) StartDownloadJob(params DownloadParams) {
	job, err := this.AddJob(JobTypeDownload, params.Name, params)
	if err != nil {
		fmt.Printf("Failed to add job: %v\n", err)
		return
	}
	go func() {
		defer this.RemoveJob(job)

		directory, fileName := path.Split(params.Destination)
		if directory == "" {
			directory = "."
		}
		err := qhttp.DownloadFileWithProgress(params.URL, directory, fileName, func(current, total int64) {
			if total > 0 {
				job.Progress = int((current * 100) / total)
			} else {
				job.Progress = -1
			}
			job.Completed = false
			this.NotifyChange()
		})
		job.Error = err
		job.Completed = true
		job.Progress = 100
		this.NotifyChange()
		if params.OnComplete != nil {
			params.OnComplete(err)
		}
	}()
}

// StartMetadataRefreshJob starts a model metadata refresh job in a
// goroutine, running the given refresh function.
func (this *JobManager) StartMetadataRefreshJob(params ModelRefreshParams, refreshFunc func() error) {
	job, err := this.AddJob(JobTypeRefreshModelMetadata, params.Name, params)
	if err != nil {
		fmt.Printf("Failed to add job: %v\n", err)
		return
	}
	go func() {
		defer this.RemoveJob(job)

		err := refreshFunc()
		job.Error = err
		job.Completed = true
		job.Progress = 100
		this.NotifyChange()
		if params.OnComplete != nil {
			params.OnComplete(err)
		}
	}()
}

// StartJob starts a generic job with a work function that receives a
// progress callback. The work function should return an error.
// If a duplicate job is detected, the job is not started and the error is
// logged.
func (this *JobManager) StartJob(name string, work func(progress func(current, total int64)) error) {
	job, err := this.AddJob(JobTypeGeneric, name, GenericJobParams{})
	if err != nil {
		fmt.Printf("Failed to add job: %v\n", err)
		return
	}
	go func() {
		defer this.RemoveJob(job)

		err := work(func(current, total int64) {
			if total > 0 {
				job.Progress = int((current * 100) / total)
			} else {
				job.Progress = -1
			}
			job.Completed = false
			this.NotifyChange()
		})
		job.Error = err
		job.Completed = true
		job.Progress = 100
		this.NotifyChange()
	}()
}
