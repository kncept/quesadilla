package gui

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/backend/running"
)

// findLabels walks an object tree (containers and widget renderers) and
// collects every label in it.
func findLabels(o fyne.CanvasObject, out []*widget.Label) []*widget.Label {
	switch c := o.(type) {
	case *widget.Label:
		return append(out, c)
	case *fyne.Container:
		for _, child := range c.Objects {
			out = findLabels(child, out)
		}
	case fyne.Widget:
		if r := test.WidgetRenderer(c); r != nil {
			for _, child := range r.Objects() {
				out = findLabels(child, out)
			}
		}
	}
	return out
}

// findChecks walks an object tree (containers and widget renderers) and
// collects every checkbox in it.
func findChecks(o fyne.CanvasObject, out []*widget.Check) []*widget.Check {
	switch c := o.(type) {
	case *widget.Check:
		return append(out, c)
	case *fyne.Container:
		for _, child := range c.Objects {
			out = findChecks(child, out)
		}
	case fyne.Widget:
		if r := test.WidgetRenderer(c); r != nil {
			for _, child := range r.Objects() {
				out = findChecks(child, out)
			}
		}
	}
	return out
}

// withLogTick runs fn with the log viewer's refresh tick temporarily set to
// d. The viewer's ticker is created when newLogViewer is called, so this
// must be in effect before the viewer is built.
func withLogTick(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	old := logTick
	logTick = d
	defer func() { logTick = old }()
	fn()
}

// TestRunningModelsTableLogsButton verifies the Overview's running-models
// table shows a "View Logs" button in the logs column for each running model.
func TestRunningModelsTableLogsButton(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)

	fake := &fakeRunningModel{name: "tiny-llama"}
	stop := running.Default().Track(fake)
	defer stop()

	g.overviewPage()
	logsCol := len(runningModelColumns) - 2
	button := tableCellButton(g.runningModelsTable, 0, logsCol)
	if button == nil {
		t.Fatal("expected a View Logs button in the logs column")
	}
	if got := button.Text; got != "View Logs" {
		t.Fatalf("logs button = %q, want %q", got, "View Logs")
	}
}

// TestShowLogsWindow verifies showLogs opens a window titled after the model,
// whose content shows the model's captured log lines and a Back button.
func TestShowLogsWindow(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.app = test.NewApp()

	fake := &fakeRunningModel{name: "tiny-llama", logs: []string{"alpha", "beta", "gamma"}}

	win := g.showLogs(fake)
	if win == nil {
		t.Fatal("expected a log window to be created")
	}
	if got := win.Title(); got != "Logs - tiny-llama" {
		t.Fatalf("window title = %q, want %q", got, "Logs - tiny-llama")
	}

	content := win.Content()

	// the Back button is present
	var back *widget.Button
	for _, b := range findButtons(content, nil) {
		if b.Text == "Back" {
			back = b
		}
	}
	if back == nil {
		t.Fatal("expected a Back button in the log window")
	}

	// the captured log lines are shown
	labels := findLabels(content, nil)
	found := false
	for _, l := range labels {
		if l.Text == "alpha\nbeta\ngamma" {
			found = true
		}
	}
	if !found {
		texts := make([]string, 0, len(labels))
		for _, l := range labels {
			texts = append(texts, l.Text)
		}
		t.Fatalf("expected the log lines in a label, found: %v", texts)
	}
}

// TestShowLogsWindowBackButtonCloses verifies tapping the Back button closes
// the log window.
func TestShowLogsWindowBackButtonCloses(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.app = test.NewApp()

	fake := &fakeRunningModel{name: "tiny-llama", logs: []string{"line"}}
	win := g.showLogs(fake)
	if win == nil {
		t.Fatal("expected a log window to be created")
	}

	// observe the close (the window's Close triggers its onClosed handler)
	closed := false
	win.SetOnClosed(func() { closed = true })

	var back *widget.Button
	for _, b := range findButtons(win.Content(), nil) {
		if b.Text == "Back" {
			back = b
		}
	}
	if back == nil {
		t.Fatal("expected a Back button in the log window")
	}
	back.Tapped(nil)

	if !closed {
		t.Fatal("expected the Back button to close the window")
	}
}

// TestShowLogsWindowControls verifies the log window's bottom bar carries a
// Refresh button (next to Back) and a Follow checkbox, starting unchecked.
func TestShowLogsWindowControls(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.app = test.NewApp()

	fake := &fakeRunningModel{name: "tiny-llama", logs: []string{"line"}}
	win := g.showLogs(fake)
	if win == nil {
		t.Fatal("expected a log window to be created")
	}

	buttons := findButtons(win.Content(), nil)
	var back, refresh *widget.Button
	for _, b := range buttons {
		switch b.Text {
		case "Back":
			back = b
		case "Refresh":
			refresh = b
		}
	}
	if back == nil {
		t.Fatal("expected a Back button in the log window")
	}
	if refresh == nil {
		t.Fatal("expected a Refresh button in the log window")
	}

	checks := findChecks(win.Content(), nil)
	if len(checks) != 1 || checks[0].Text != "Follow" {
		t.Fatalf("expected a single Follow checkbox in the log window, found %d checks", len(checks))
	}
	if checks[0].Checked {
		t.Fatal("Follow should start unchecked")
	}
}

// TestShowLogsRefreshButton verifies tapping Refresh reloads the model's
// captured log lines into the viewer.
func TestShowLogsRefreshButton(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.app = test.NewApp()

	fake := &fakeRunningModel{name: "tiny-llama", logs: []string{"alpha", "beta"}}
	win := g.showLogs(fake)
	if win == nil {
		t.Fatal("expected a log window to be created")
	}

	fake.addLog("gamma")

	var refresh *widget.Button
	for _, b := range findButtons(win.Content(), nil) {
		if b.Text == "Refresh" {
			refresh = b
		}
	}
	if refresh == nil {
		t.Fatal("expected a Refresh button in the log window")
	}
	refresh.Tapped(nil)

	found := false
	for _, l := range findLabels(win.Content(), nil) {
		if l.Text == "alpha\nbeta\ngamma" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the Refresh button to reload the log lines into the label")
	}
}

// TestShowLogsFollowAutoUpdates verifies that with the Follow checkbox on, a
// viewer tick reloads the model's newest output into the viewer and keeps the
// view pinned to the newest lines.
func TestShowLogsFollowAutoUpdates(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.app = test.NewApp()

	// enough lines for the log area to overflow the window, so "pinned to
	// the bottom" is observable as a non-zero scroll offset
	var logs []string
	for i := 0; i < 80; i++ {
		logs = append(logs, "log line")
	}
	fake := &fakeRunningModel{name: "tiny-llama", logs: logs}

	win := g.app.NewWindow("Logs - tiny-llama")

	// Park the auto-update tick far in the future so the viewer's ticker
	// goroutine stays out of the way: the test drives v.tick directly, the
	// exact code path the ticker invokes on every logTick.
	withLogTick(t, time.Hour, func() {
		content, v := newLogViewer(fake, win)
		win.SetContent(content)
		win.Resize(fyne.NewSize(800, 500))
		win.Show()

		// a real tap inside the checkbox's active area (Check.Tapped
		// dereferences the event, so it cannot be nil like a Button tap)
		v.follow.Tapped(&fyne.PointEvent{Position: fyne.NewPos(10, 5)})
		if !v.follow.Checked {
			t.Fatal("tapping the Follow checkbox should check it")
		}

		fake.addLog("fresh output")
		v.tick()

		want := strings.Repeat("log line\n", 80) + "fresh output"
		if v.text.Text != want {
			t.Fatalf("the tick did not reload the newest log lines into the label")
		}
		if v.scroll.Offset.Y <= 0 {
			t.Fatalf("expected the view to be pinned to the newest lines, offset = %v", v.scroll.Offset)
		}
	})
}

// TestShowLogsNoAutoUpdateWithoutFollow verifies that with the Follow
// checkbox off, the viewer does not reload the log lines on its own (the
// Refresh button is the manual path).
func TestShowLogsNoAutoUpdateWithoutFollow(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.app = test.NewApp()

	fake := &fakeRunningModel{name: "tiny-llama", logs: []string{"alpha"}}

	withLogTick(t, 25*time.Millisecond, func() {
		win := g.showLogs(fake)
		if win == nil {
			t.Fatal("expected a log window to be created")
		}

		fake.addLog("beta")

		// wait well past several ticks without touching the window
		time.Sleep(300 * time.Millisecond)

		for _, l := range findLabels(win.Content(), nil) {
			if l.Text == "alpha\nbeta" {
				t.Fatal("the log label was auto-updated although Follow is off")
			}
		}
	})
}
