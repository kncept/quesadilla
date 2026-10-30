package gui

import (
	"fmt"

	"fyne.io/fyne/v2"

	"github.com/kncept/quesadilla/gui/jobs"
	"github.com/kncept/quesadilla/model/remoterepository"
)

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

// downloadHuggingFaceVariant downloads a single variant of the currently
// selected HuggingFace model, then refreshes the screen.
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

	this.jobManager.StartJob(fmt.Sprintf("Download %s (%s)", modelName, variantFileName), func(progress func(current, total int64)) error {
		remote := hfScanner.GetModel(modelID)
		if remote == nil {
			return fmt.Errorf("could not find model %s on HuggingFace", modelID)
		}

		err := this.QApp.LocalModels.DownloadVariant(remote, variantFileName, progress)
		if err != nil {
			return err
		}

		fyne.Do(func() {
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
		return nil
	})
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

	this.jobManager.StartMetadataRefreshJob(jobs.ModelRefreshParams{
		Name:      fmt.Sprintf("Refresh metadata %s", modelName),
		ModelName: modelName,
		OnComplete: func(err error) {
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
	}, func() error {
		return this.doRefreshModelMetadata(modelName)
	})
}
