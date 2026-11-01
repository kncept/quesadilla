package jobs

import (
	"fmt"
	"time"
)

// The job types the application starts. A job's type groups it on the Jobs
// screen; together with its name it identifies the job, so the same job
// cannot run twice at once.
const (
	// JobTypeDownload is a job that downloads or installs model files.
	JobTypeDownload = "download"
	// JobTypeRefreshModelMetadata is a job that refreshes a model's metadata.
	JobTypeRefreshModelMetadata = "refresh model metadata"
	// JobTypeGeneric is any other job.
	JobTypeGeneric = "generic"
)

// Job represents a job currently in progress or completed.
type Job struct {
	Type        string
	Name        string
	Description string
	Started     time.Time
	Progress    int   // 0-100, or -1 if unknown
	Error       error // set when job completes with error
	Completed   bool  // true when job has finished
}

// JobName returns the name shown on the Jobs screen for a model job,
// including the single variant when one is being fetched.
func JobName(verb, modelName, variantFile string) string {
	if variantFile != "" {
		return fmt.Sprintf("%s %s (%s)", verb, modelName, variantFile)
	}
	return fmt.Sprintf("%s %s", verb, modelName)
}
