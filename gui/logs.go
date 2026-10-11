package gui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// viewLogsButton builds the "view logs" button for a running-models table
// row. Tapping it opens a log viewer window for that model.
func (this *QGUI) viewLogsButton(m runnerDefinitions.RunningModel) *widget.Button {
	return widget.NewButtonWithIcon("View Logs", theme.FileTextIcon(), func() {
		this.showLogs(m)
	})
}

// logTick is how often an open log viewer checks its model for new output
// while the Follow checkbox is on.
var logTick = 2 * time.Second

// logViewer is the state of an open log window: the model it shows and the
// widgets it renders.
type logViewer struct {
	model  runnerDefinitions.RunningModel
	win    fyne.Window
	text   *widget.Label
	scroll *container.Scroll
	follow *widget.Check
}

// newLogViewer creates the content of a log window for the model's captured
// log lines, plus the viewer state that drives it. The content is a heading
// with the model name, a scrollable area with the log lines, and a bottom
// bar with a Back button (closes the window), a Refresh button (reloads the
// captured log lines), and a Follow checkbox (auto-reloads the log lines and
// keeps the view pinned to the newest one). Follow starts checked, so the
// viewer tracks new output as soon as it opens. An auto-update loop is started
// that runs until the window closes; onClosed (when set) runs once, after the
// window closes and the loop is stopped.
func newLogViewer(m runnerDefinitions.RunningModel, win fyne.Window, onClosed func()) (fyne.CanvasObject, *logViewer) {
	v := &logViewer{model: m, win: win}

	title := headingLabel(fmt.Sprintf("Logs - %s", m.ModelName()))

	v.text = widget.NewLabel(strings.Join(m.Logs(), "\n"))
	v.text.Wrapping = fyne.TextWrapWord
	v.scroll = container.NewScroll(v.text)

	// Follow starts on: set the state before wiring OnChanged so the initial
	// state does not queue an update through the checkbox.
	v.follow = widget.NewCheck("Follow", nil)
	v.follow.SetChecked(true)
	v.follow.OnChanged = func(checked bool) {
		if checked {
			fyne.Do(v.update)
		}
	}

	// Refresh reloads the captured log lines on demand.
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		fyne.Do(v.update)
	})

	back := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		win.Close()
	})

	content := container.NewBorder(
		container.NewPadded(title),                                      // top
		container.NewPadded(container.NewHBox(back, refresh, v.follow)), // bottom
		nil, nil,
		v.scroll,
	)

	// Keep the log text up to date while the viewer is open and Follow is
	// on: the model may still be running and emitting output. The interval is
	// captured up front so the goroutine never reads logTick, and the
	// goroutine stops when the window closes.
	var closeOnce sync.Once
	done := make(chan struct{})
	win.SetOnClosed(func() {
		closeOnce.Do(func() {
			close(done)
			if onClosed != nil {
				onClosed()
			}
		})
	})
	tick := logTick
	go func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fyne.Do(v.tick)
			}
		}
	}()

	return content, v
}

// update reloads the model's captured log lines into the viewer and, when
// Follow is on, keeps the view pinned to the newest lines.
func (v *logViewer) update() {
	v.text.SetText(strings.Join(v.model.Logs(), "\n"))
	if v.follow.Checked {
		v.scroll.ScrollToBottom()
	}
}

// tick performs one auto-update of the viewer; a no-op unless Follow is on.
// It is what the viewer's ticker goroutine invokes on every logTick.
func (v *logViewer) tick() {
	if v.follow.Checked {
		v.update()
	}
}

// showLogs opens a small window with the named model's captured log lines.
// Each model has at most one log window: opening the logs again (from the
// "View Logs" button or the tray) brings the existing window to the front
// instead of opening a second one. While its Follow checkbox is on, the window
// keeps updating (and stays pinned to the newest lines) as long as the model
// keeps running; the Refresh button reloads the lines on demand. The window
// closes when its Back button is tapped. It returns the window (or nil if the
// app is not ready), which is handy for tests.
func (this *QGUI) showLogs(m runnerDefinitions.RunningModel) fyne.Window {
	name := m.ModelName()

	this.mu.Lock()
	app := this.app
	if win := this.logWindows[name]; win != nil {
		this.mu.Unlock()
		// this model's logs are already open: raise the window instead of
		// opening a duplicate
		win.Show()
		win.RequestFocus()
		return win
	}
	this.mu.Unlock()

	if app == nil {
		return nil
	}

	win := app.NewWindow(fmt.Sprintf("Logs - %s", name))
	content, _ := newLogViewer(m, win, func() { this.forgetLogWindow(name, win) })
	win.SetContent(content)
	win.Resize(fyne.NewSize(800, 500))

	this.mu.Lock()
	if this.logWindows == nil {
		this.logWindows = map[string]fyne.Window{}
	}
	this.logWindows[name] = win
	this.mu.Unlock()

	win.Show()
	return win
}

// forgetLogWindow drops the reference to a model's log window once that window
// has closed, so opening the logs again creates a fresh window. It only clears
// the entry when it still points at win, so a window that closed after being
// replaced by a newer one does not evict the newer window.
func (this *QGUI) forgetLogWindow(name string, win fyne.Window) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.logWindows[name] == win {
		delete(this.logWindows, name)
	}
}
