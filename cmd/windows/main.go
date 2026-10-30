package main

import (
	"encoding/json"
	"os"
	"path"

	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/gui"
	"github.com/kncept/quesadilla/utils/qenv"
)

func main() {
	g := gui.CreateGui(SystemAgnosticOperations(), app.New())
	g.Start()
}

func SystemAgnosticOperations() gui.SystemAgnosticOperations {
	return &windowsOperations{}
}

type windowsOperations struct{}

// GetLocation implements [gui.SystemAgnosticOperations].
func (this *windowsOperations) GetLocation() (any, error) {
	// Location services not typically available on Windows desktops
	return nil, nil
}

// GetPreference implements [gui.SystemAgnosticOperations].
func (this *windowsOperations) GetPreference(dotTreeKey string) (string, error) {
	prefs, err := loadPreferences()
	if err != nil {
		return "", err
	}
	return prefs[dotTreeKey], nil
}

// LocationEnabled implements [gui.SystemAgnosticOperations].
func (this *windowsOperations) LocationEnabled() bool {
	return false
}

// SetPreference implements [gui.SystemAgnosticOperations].
func (this *windowsOperations) SetPreference(dotTreeKey string, value string) error {
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	prefs[dotTreeKey] = value
	return savePreferences(prefs)
}

func preferencesPath() string {
	return path.Join(qenv.QDir(), "preferences.json")
}

func loadPreferences() (map[string]string, error) {
	p := preferencesPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}
	var prefs map[string]string
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, err
	}
	if prefs == nil {
		prefs = make(map[string]string)
	}
	return prefs, nil
}

func savePreferences(prefs map[string]string) error {
	p := preferencesPath()
	// Ensure directory exists
	if err := os.MkdirAll(path.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}