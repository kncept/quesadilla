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

// AddJob adds a new job and returns it.
func (this *JobManager) AddJob(jobType JobType, name, source, destination, modelName string) *Job {
	job := &Job{
		Type:        jobType,
		Name:        name,
		Source:      source,
		Destination: destination,
		ModelName:   modelName,
		Started:     time.Now(),
		Progress:    -1,
	}
	fmt.Printf("Job added: %+v\n", job)
	this.mu.Lock()
	this.jobs = append(this.jobs, job)
	this.mu.Unlock()
	this.NotifyChange()
	return job
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

// StartDownloadJob starts a download job in a goroutine.
func (this *JobManager) StartDownloadJob(request JobRequest) {
	job := this.AddJob(request.Type, request.Name, request.URL, request.Destination, request.ModelName)
	go func() {
		defer this.RemoveJob(job)

		directory, fileName := path.Split(request.Destination)
		if directory == "" {
			directory = "."
		}
		err := qhttp.DownloadFileWithProgress(request.URL, directory, fileName, func(current, total int64) {
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
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}

// StartMetadataRefreshJob starts a metadata refresh job in a goroutine.
func (this *JobManager) StartMetadataRefreshJob(request JobRequest, refreshFunc func() error) {
	job := this.AddJob(request.Type, request.Name, request.URL, request.Destination, request.ModelName)
	go func() {
		defer this.RemoveJob(job)

		err := refreshFunc()
		job.Error = err
		job.Completed = true
		job.Progress = 100
		this.NotifyChange()
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}

// StartJob starts a generic job with a work function that receives a progress callback.
// The work function should return an error.
func (this *JobManager) StartJob(request JobRequest, work func(progress func(current, total int64)) error) {
	job := this.AddJob(request.Type, request.Name, request.URL, request.Destination, request.ModelName)
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
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}
