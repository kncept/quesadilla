package gui

import (
	"fmt"
	"log"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/kncept/quesadilla/app/jobs"
	"github.com/kncept/quesadilla/model/definitions"
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
				this.updateRefreshAllButtonState()
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

// deleteModelDetailsVariant deletes a single variant from the currently
// selected model in the Model Details view.
func (this *QGUI) deleteModelDetailsVariant(modelName string, fileName string) {
	model := this.QApp.LocalModels.GetModel(modelName)
	if model == nil {
		this.notifyError("Model not found: " + modelName)
		return
	}

	// Show confirmation dialog
	dialog.ShowConfirm("Delete Variant",
		fmt.Sprintf("Delete variant %q from %s?\nThis will remove the file from disk.", fileName, modelName),
		func(confirmed bool) {
			if !confirmed {
				return
			}
			err := this.QApp.LocalModels.RemoveVariant(modelName, fileName)
			if err != nil {
				this.notifyError(fmt.Sprintf("Failed to delete variant: %v", err))
				return
			}
			// Refresh the model details screen
			if this.modelDetailsSelected == modelName {
				this.showModelDetailsContent(modelName)
				if this.modelDetailsList != nil && len(this.modelDetailsRows) > 1 {
					this.modelDetailsList.Select(1) // the Download row
				}
			}
			if this.modelsTable != nil {
				this.modelsTable.Refresh()
			}
			this.updateRefreshAllButtonState()
		},
		this.mainWin)
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
				this.updateRefreshAllButtonState()
			})
		},
		nil,
	)
	if err != nil {
		this.notifyError(fmt.Sprintf("Could not start metadata refresh: %v", err))
	}
}

// refreshAllMetadata starts a job to refresh metadata for all HuggingFace models
// one at a time, waiting for each to complete before starting the next.
func (this *QGUI) refreshAllMetadata() {
	models := this.QApp.LocalModels.Models()
	if len(models) == 0 {
		this.notifyError("No models installed")
		return
	}

	// Filter to only HuggingFace models
	var hfModels []definitions.Model
	for _, m := range models {
		if isHuggingFaceModel(m) {
			hfModels = append(hfModels, m)
		}
	}

	if len(hfModels) == 0 {
		this.notifyError("No HuggingFace models installed - metadata refresh only available for HuggingFace models")
		return
	}

	// Disable the button while the job is running
	if this.modelsRefreshAllButton != nil {
		this.modelsRefreshAllButton.Disable()
	}

	err := this.jobManager.StartJob(
		fmt.Sprintf("Refresh All Metadata (%d models)", len(hfModels)),
		jobs.JobTypeGeneric,
		fmt.Sprintf("Refresh metadata for %d HuggingFace models", len(hfModels)),
		func() error {
			for _, model := range hfModels {
				// Update parent job progress
				this.jobManager.NotifyChange()

				// Start a sub-job for this model's metadata refresh and wait for completion
				modelName := model.FullModelName()
				err := this.startAndWaitForMetadataRefresh(modelName)
				if err != nil {
					// Log error but continue with other models
					fmt.Printf("Failed to refresh metadata for %s: %v\n", modelName, err)
					// Could collect errors and report at the end
				}
			}
			return nil
		},
		func(err error) {
			fyne.Do(func() {
				if err != nil {
					this.notifyError(fmt.Sprintf("Failed to refresh all metadata: %v", err))
				} else {
					log.Printf("Metadata refresh completed for all models")
				}
				// Re-enable the button
				if this.modelsRefreshAllButton != nil {
					this.modelsRefreshAllButton.Enable()
				}
				// Refresh the models table
				if this.modelsTable != nil {
					this.modelsTable.Refresh()
				}
				this.updateRefreshAllButtonState()
				// Refresh model details if a model is selected
				if this.modelDetailsSelected != "" {
					this.showModelDetailsContent(this.modelDetailsSelected)
				}
			})
		},
		func() int {
			// Could calculate overall progress here
			return -1
		},
	)
	if err != nil {
		this.notifyError(fmt.Sprintf("Could not start refresh all metadata: %v", err))
		if this.modelsRefreshAllButton != nil {
			this.modelsRefreshAllButton.Enable()
		}
	}
}

// startAndWaitForMetadataRefresh starts a metadata refresh job for the given model
// using StartJob and waits for it to complete before returning.
func (this *QGUI) startAndWaitForMetadataRefresh(modelName string) error {
	done := make(chan error, 1)

	err := this.jobManager.StartJob(
		fmt.Sprintf("Refresh metadata %s", modelName),
		jobs.JobTypeRefreshModelMetadata,
		fmt.Sprintf("Refresh metadata for %s", modelName),
		func() error {
			return this.doRefreshModelMetadata(modelName)
		},
		func(err error) {
			done <- err
		},
		nil,
	)
	if err != nil {
		return err
	}

	// Wait for the job to complete
	return <-done
}

// doRefreshModelMetadataWithProgress performs the actual metadata refresh with progress callback.
func (this *QGUI) doRefreshModelMetadataWithProgress(modelName string, progress func(int)) error {
	// For now, just call the existing function since it doesn't support progress
	// In the future, this could be enhanced to report progress
	return this.doRefreshModelMetadata(modelName)
}
