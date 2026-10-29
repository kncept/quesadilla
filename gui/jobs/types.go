package jobs

import (
	"fmt"
	"time"
)

// JobType represents the type of job.
type JobType string

const (
	// JobTypeDownload represents a file download job.
	JobTypeDownload JobType = "download"
	// JobTypeRefreshModelMetadata represents a model metadata refresh job.
	JobTypeRefreshModelMetadata JobType = "refresh model metadata"
)

// JobRequest describes a job to be started.
type JobRequest struct {
	// Type is the type of job.
	Type JobType
	// Name is what the job is called on the Jobs screen.
	Name string
	// URL is the source to download from (for download jobs).
	URL string
	// Destination is the file to write the download to (for download jobs).
	Destination string
	// ModelName is the model name for metadata refresh jobs.
	ModelName string
	// OnComplete runs after the job finishes. It receives the
	// job's error, or nil when it succeeded.
	OnComplete func(error)
}

// Job represents a job currently in progress or completed.
type Job struct {
	Type        JobType
	Name        string
	Source      string
	Destination string
	ModelName   string
	Started     time.Time
	Progress    int       // 0-100, or -1 if unknown
	Error       error     // set when job completes with error
	Completed   bool      // true when job has finished
}

// JobColumns are the table headers shown on the Jobs screen.
var JobColumns = []string{"Name", "Source", "Destination", "Progress", "Elapsed"}

// JobName returns the name shown on the Jobs screen for a model download job,
// including the single variant when one is being fetched.
func JobName(verb, modelName, variantFile string) string {
	if variantFile != "" {
		return fmt.Sprintf("%s %s (%s)", verb, modelName, variantFile)
	}
	return fmt.Sprintf("%s %s", verb, modelName)
}