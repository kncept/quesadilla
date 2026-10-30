package gui

import (
	"fmt"
	"sync/atomic"

	"fyne.io/fyne/v2"

	"github.com/kncept/quesadilla/gui/jobs"
	"github.com/kncept/quesadilla/model/remoterepository"
)

// progressTracker bridges the byte-based progress callbacks of the local
// repository (current, total) to the percentage-based progress function the
// JobManager polls while a job runs.
type progressTracker struct {
	current atomic.Int64
	total   atomic.Int64
}

// update records the latest download progress. It matches the
// func(current, total int64) callback the local repository expects.
func (this *progressTracker) update(current, total int64) {
	this.current.Store(current)
	this.total.Store(total)
}

// percent returns the download's progress as a percentage, or -1 when the
// total size is not known.
func (this *progressTracker) percent() int {
	total := this.total.Load()
	if total <= 0 {
		return -1
	}
	return int(this.current.Load() * 100 / total)
}

// jobsPage renders the Jobs screen using the new jobs package.
func (this *QGUI) jobsPage() fyne.CanvasObject {
	if this.jobsScreen == nil {
		this.jobsScreen = jobs.NewJobsScreen(this.jobManager)
	}
	return this.jobsScreen.Content()
}

// doRefreshModelMetadata performs the actual metadata refresh.
func (this *QGUI) doRefreshModelMetadata(modelName string) error {
	model := this.QApp.LocalModels.GetModel(modelName)
	if model == nil {
		return fmt.Errorf("model not found: %s", modelName)
	}

	// Only HuggingFace models can have their metadata refreshed from remote
	if !isHuggingFaceModel(*model) {
		return fmt.Errorf("metadata refresh is only available for HuggingFace models")
	}

	modelID := huggingFaceModelID(*model)
	scanner := this.QApp.RemoteModels.GetScanner("HuggingFace")
	hfScanner, ok := scanner.(*remoterepository.HuggingFaceScanner)
	if !ok {
		return fmt.Errorf("HuggingFace scanner not available")
	}

	remote := hfScanner.GetModel(modelID)
	if remote == nil {
		return fmt.Errorf("could not find model %s on HuggingFace", modelID)
	}

	// Update the local model with fresh metadata from remote
	err := this.QApp.LocalModels.UpdateModelMetadata(remote)
	if err != nil {
		return err
	}

	// Refresh the model details screen to show updated metadata
	fyne.Do(func() {
		if this.modelDetailsSelected == modelName {
			this.showModelDetailsContent(modelName)
		}
		if this.modelsTable != nil {
			this.modelsTable.Refresh()
		}
	})
	return nil
}

// downloadHuggingFaceVariant downloads the single variant the user picked of
// the currently selected HuggingFace model, then refreshes the screen.
func (this *QGUI) downloadHuggingFaceVariant(variantFileName string) {
	model := this.QApp.LocalModels.GetModel(this.modelDetailsSelected)
	if model == nil {
		return
	}
	modelID := huggingFaceModelID(*model)
	modelName := model.FullModelName()

	scanner := this.QApp.RemoteModels.GetScanner("HuggingFace")
	hfScanner, ok := scanner.(*remoterepository.HuggingFaceScanner)
	if !ok {
		this.notifyError("HuggingFace scanner not available")
		return
	}

	progress := &progressTracker{}
	err := this.jobManager.StartJob(
		fmt.Sprintf("Download %s (%s)", modelName, variantFileName),
		jobs.JobTypeDownload,
		fmt.Sprintf("Download %s for %s", variantFileName, modelName),
		func() error {
			remote := hfScanner.GetModel(modelID)
			if remote == nil {
				return fmt.Errorf("could not find model %s on HuggingFace", modelID)
			}
			return this.QApp.LocalModels.DownloadVariant(remote, variantFileName, progress.update)
		},
		func(err error) {
			fyne.Do(func() {
				if err != nil {
					this.notifyError(fmt.Sprintf("Failed to download %s: %v", variantFileName, err))
					return
				}
				if this.modelsTable != nil {
					this.modelsTable.Refresh()
				}
				// Rebuild the screen: the new variant appears in the left menu,
				// and its download button disappears.
				if this.modelDetailsSelected == modelName {
					this.showModelDetailsContent(modelName)
					if this.modelDetailsList != nil && len(this.modelDetailsRows) > 1 {
						this.modelDetailsList.Select(1) // the Download row
					}
				}
			})
		},
		progress.percent,
	)
	if err != nil {
		this.notifyError(fmt.Sprintf("Could not start download: %v", err))
	}
}

// refreshModelMetadata starts a job to refresh the metadata of the currently
// selected model from its source.
func (this *QGUI) refreshModelMetadata() {
	modelName := this.modelDetailsSelected
	if modelName == "" {
		return
	}

	model := this.QApp.LocalModels.GetModel(modelName)
	if model == nil {
		this.notifyError("Model not found: " + modelName)
		return
	}

	// Only HuggingFace models can have their metadata refreshed from remote
	if !isHuggingFaceModel(*model) {
		this.notifyError("Metadata refresh is only available for HuggingFace models")
		return
	}

	err := this.jobManager.StartJob(
		fmt.Sprintf("Refresh metadata %s", modelName),
		jobs.JobTypeRefreshModelMetadata,
		fmt.Sprintf("Refresh metadata for %s", modelName),
		func() error {
			return this.doRefreshModelMetadata(modelName)
		},
		func(err error) {
			fyne.Do(func() {
				if err != nil {
					this.notifyError(fmt.Sprintf("Failed to refresh metadata: %v", err))
					return
				}
				// Refresh the model details screen to show updated metadata
				if this.modelDetailsSelected == modelName {
					this.showModelDetailsContent(modelName)
				}
				if this.modelsTable != nil {
					this.modelsTable.Refresh()
				}
			})
		},
		nil,
	)
	if err != nil {
		this.notifyError(fmt.Sprintf("Could not start metadata refresh: %v", err))
	}
}
