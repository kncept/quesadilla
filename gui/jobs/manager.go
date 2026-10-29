package jobs

import (
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kncept/quesadilla/utils/qhttp"
)

// JobManager manages the list of active jobs.
type JobManager struct {
	mu      sync.RWMutex
	jobs    []*Job
	onChange atomic.Value
}

// NewJobManager creates a new job manager.
func NewJobManager() *JobManager {
	return &JobManager{}
}

// SetOnChange sets the callback to be called when the jobs list changes.
func (m *JobManager) SetOnChange(fn func()) {
	m.onChange.Store(fn)
}

// NotifyChange calls the onChange callback if set.
func (m *JobManager) NotifyChange() {
	if fn, ok := m.onChange.Load().(func()); ok && fn != nil {
		fn()
	}
}

// AddJob adds a new job and returns it.
func (m *JobManager) AddJob(jobType JobType, name, source, destination, modelName string) *Job {
	job := &Job{
		Type:        jobType,
		Name:        name,
		Source:      source,
		Destination: destination,
		ModelName:   modelName,
		Started:     time.Now(),
		Progress:    -1,
	}
	m.mu.Lock()
	m.jobs = append(m.jobs, job)
	m.mu.Unlock()
	m.NotifyChange()
	return job
}

// RemoveJob removes a job from the list.
func (m *JobManager) RemoveJob(job *Job) {
	if job == nil {
		return
	}
	m.mu.Lock()
	for i, j := range m.jobs {
		if j == job {
			m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	m.NotifyChange()
}

// ActiveJobs returns a snapshot of current jobs.
func (m *JobManager) ActiveJobs() []Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	jobs := make([]Job, len(m.jobs))
	for i, job := range m.jobs {
		jobs[i] = *job
	}
	return jobs
}

// StartDownloadJob starts a download job in a goroutine.
func (m *JobManager) StartDownloadJob(request JobRequest) {
	job := m.AddJob(request.Type, request.Name, request.URL, request.Destination, request.ModelName)
	go func() {
		defer m.RemoveJob(job)

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
			m.NotifyChange()
		})
		job.Error = err
		job.Completed = true
		job.Progress = 100
		m.NotifyChange()
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}

// StartMetadataRefreshJob starts a metadata refresh job in a goroutine.
func (m *JobManager) StartMetadataRefreshJob(request JobRequest, refreshFunc func() error) {
	job := m.AddJob(request.Type, request.Name, request.URL, request.Destination, request.ModelName)
	go func() {
		defer m.RemoveJob(job)

		err := refreshFunc()
		job.Error = err
		job.Completed = true
		job.Progress = 100
		m.NotifyChange()
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}

// StartJob starts a generic job with a work function that receives a progress callback.
// The work function should return an error.
func (m *JobManager) StartJob(request JobRequest, work func(progress func(current, total int64)) error) {
	job := m.AddJob(request.Type, request.Name, request.URL, request.Destination, request.ModelName)
	go func() {
		defer m.RemoveJob(job)

		err := work(func(current, total int64) {
			if total > 0 {
				job.Progress = int((current * 100) / total)
			} else {
				job.Progress = -1
			}
			job.Completed = false
			m.NotifyChange()
		})
		job.Error = err
		job.Completed = true
		job.Progress = 100
		m.NotifyChange()
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}