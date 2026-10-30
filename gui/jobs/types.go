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
	// JobTypeGeneric represents a job running an arbitrary work function.
	JobTypeGeneric JobType = "generic"
)

// DownloadParams describes a file download job.
type DownloadParams struct {
	// Name is what the job is called on the Jobs screen.
	Name string
	// URL is the source to download from.
	URL string
	// Destination is the file to write the download to.
	Destination string
	// ModelName is the model the download belongs to (may be empty).
	ModelName string
	// OnComplete runs after the job finishes. It receives the
	// job's error, or nil when it succeeded.
	OnComplete func(error)
}

// ModelRefreshParams describes a model metadata refresh job.
type ModelRefreshParams struct {
	// Name is what the job is called on the Jobs screen.
	Name string
	// ModelName is the model whose metadata is being refreshed.
	ModelName string
	// OnComplete runs after the job finishes. It receives the
	// job's error, or nil when it succeeded.
	OnComplete func(error)
}

// GenericJobParams is the metadata for a generic job started with StartJob.
// Generic jobs carry no type-specific parameters; the type exists so the
// Job's Metadata field always holds a known struct type the screen can cast
// to.
type GenericJobParams struct{}

// Job represents a job currently in progress or completed.
type Job struct {
	Type      JobType
	Name      string
	Metadata  any // type-specific params: DownloadParams, ModelRefreshParams, or GenericJobParams
	Started   time.Time
	Progress  int   // 0-100, or -1 if unknown
	Error     error // set when job completes with error
	Completed bool  // true when job has finished
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
