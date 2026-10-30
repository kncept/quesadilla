package jobs

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// JobsScreen renders the Jobs screen with a list on the left and details on the right.
type JobsScreen struct {
	manager     *JobManager
	list        *widget.List
	detailsView *fyne.Container
	selectedJob *Job
}

// NewJobsScreen creates a new jobs screen.
func NewJobsScreen(manager *JobManager) *JobsScreen {
	s := &JobsScreen{
		manager: manager,
	}
	s.buildUI()
	return s
}

func (s *JobsScreen) buildUI() {
	// Job list on the left
	s.list = widget.NewList(
		func() int {
			return len(s.manager.ActiveJobs())
		},
		func() fyne.CanvasObject {
			return container.NewPadded(widget.NewLabel("Job Name"))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			jobs := s.manager.ActiveJobs()
			if id < 0 || id >= len(jobs) {
				return
			}
			job := jobs[id]
			label := obj.(*fyne.Container).Objects[0].(*widget.Label)
			label.SetText(job.Name)
		},
	)
	s.list.OnSelected = func(id widget.ListItemID) {
		jobs := s.manager.ActiveJobs()
		if id < 0 || id >= len(jobs) {
			return
		}
		s.showJobDetails(jobs[id])
	}

	// Details view on the right
	s.detailsView = container.NewVBox(
		widget.NewLabel("Select a job to view details"),
	)

	// Set up change callback
	s.manager.SetOnChange(func() {
		fyne.Do(func() {
			s.list.Refresh()
			if s.selectedJob != nil {
				// Re-find the selected job in the updated list
				jobs := s.manager.ActiveJobs()
				found := false
				for _, j := range jobs {
					if j.Name == s.selectedJob.Name && j.Type == s.selectedJob.Type && j.Started.Equal(s.selectedJob.Started) {
						s.showJobDetails(j)
						found = true
						break
					}
				}
				if !found {
					s.selectedJob = nil
					s.detailsView.RemoveAll()
					s.detailsView.Add(widget.NewLabel("Select a job to view details"))
					s.detailsView.Refresh()
				}
			}
		})
	})
}

func (s *JobsScreen) showJobDetails(job Job) {
	s.selectedJob = &job
	s.detailsView.RemoveAll()

	var status string
	if job.Completed {
		if job.Error != nil {
			status = "Failed"
		} else {
			status = "Completed"
		}
	} else {
		status = "Running"
	}

	progressText := "—"
	if job.Progress >= 0 {
		progressText = fmt.Sprintf("%d%%", job.Progress)
	}

	elapsed := time.Since(job.Started).Round(time.Second)

	s.detailsView.Add(widget.NewLabelWithStyle("Job Details", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	s.detailsView.Add(widget.NewSeparator())
	s.detailsView.Add(widget.NewLabel("Name: " + job.Name))
	s.detailsView.Add(widget.NewLabel("Type: " + string(job.Type)))
	s.detailsView.Add(widget.NewLabel("Status: " + status))
	switch metadata := job.Metadata.(type) {
	case DownloadParams:
		if metadata.URL != "" {
			s.detailsView.Add(widget.NewLabel("Source: " + metadata.URL))
		}
		if metadata.Destination != "" {
			s.detailsView.Add(widget.NewLabel("Destination: " + metadata.Destination))
		}
		if metadata.ModelName != "" {
			s.detailsView.Add(widget.NewLabel("Model: " + metadata.ModelName))
		}
	case ModelRefreshParams:
		if metadata.ModelName != "" {
			s.detailsView.Add(widget.NewLabel("Model: " + metadata.ModelName))
		}
	}
	s.detailsView.Add(widget.NewLabel("Started: " + job.Started.Format(time.RFC3339)))
	s.detailsView.Add(widget.NewLabel("Elapsed: " + elapsed.String()))
	s.detailsView.Add(widget.NewLabel("Progress: " + progressText))
	if job.Error != nil {
		s.detailsView.Add(widget.NewLabel("Error: " + job.Error.Error()))
	}
	s.detailsView.Refresh()
}

// Content returns the main content of the jobs screen.
func (s *JobsScreen) Content() fyne.CanvasObject {
	split := container.NewHSplit(
		container.NewPadded(s.list),
		container.NewPadded(s.detailsView),
	)
	split.SetOffset(0.3)
	return split
}
