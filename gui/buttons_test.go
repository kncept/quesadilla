package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/backend/running"
)

// findButtons walks an object tree (containers and widget renderers) and
// collects every button in it.
func findButtons(o fyne.CanvasObject, out []*widget.Button) []*widget.Button {
	switch c := o.(type) {
	case *widget.Button:
		return append(out, c)
	case *fyne.Container:
		for _, child := range c.Objects {
			out = findButtons(child, out)
		}
	case fyne.Widget:
		if r := test.WidgetRenderer(c); r != nil {
			for _, child := range r.Objects() {
				out = findButtons(child, out)
			}
		}
	}
	return out
}

// tapButtonCenter simulates a real pointer tap at the centre of the given
// button, in canvas coordinates, the way a user would click it. This goes
// through Fyne's hit-testing, so a button that is not laid out (0x0) is
// never found and the tap misses.
func tapButtonCenter(t *testing.T, button *widget.Button) {
	t.Helper()
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(button)
	test.TapCanvas(
		fyne.CurrentApp().Driver().CanvasForObject(button),
		fyne.NewPos(abs.X+button.Size().Width/2, abs.Y+button.Size().Height/2),
	)
}

// TestModelsTableButtonTap verifies that a real pointer tap at the rendered
// start/stop button position triggers its handler. This catches the case
// where the button is never laid out (left at 0x0), in which case Fyne's
// hit-testing can never find it and OnTapped is silently never called.
func TestModelsTableButtonTap(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	win := test.NewWindow(table)
	win.Resize(fyne.NewSize(900, 600))

	buttons := findButtons(table, nil)
	if len(buttons) != 1 {
		t.Fatalf("found %d buttons in the rendered table, want 1", len(buttons))
	}
	button := buttons[0]
	if button.Size().Width <= 0 || button.Size().Height <= 0 {
		t.Fatalf("button has zero size: %v (never laid out)", button.Size())
	}
	if got := button.Text; got != "Start" {
		t.Fatalf("button text = %q, want %q", got, "Start")
	}

	called := false
	button.OnTapped = func() { called = true }
	tapButtonCenter(t, button)

	if !called {
		t.Fatal("real tap at the button did NOT trigger OnTapped")
	}
}

// TestRunningModelsTableButtonTap verifies the same for the Overview's
// running-models table: a real tap on the rendered stop button asks the
// running model to quit.
func TestRunningModelsTableButtonTap(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)

	fake := &fakeRunningModel{name: "tiny-llama"}
	stop := running.Default().Track(fake)
	defer stop()

	page := g.overviewPage()
	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(900, 600))

	// The row now has two buttons: View Logs and Stop. Find the Stop button
	// specifically by its text.
	buttons := findButtons(page, nil)
	if len(buttons) != 2 {
		t.Fatalf("found %d buttons in the rendered page, want 2 (View Logs and Stop)", len(buttons))
	}
	var button *widget.Button
	for _, b := range buttons {
		if b.Text == "Stop" {
			button = b
		}
	}
	if button == nil {
		t.Fatal("expected a Stop button in the rendered page")
	}
	if button.Size().Width <= 0 || button.Size().Height <= 0 {
		t.Fatalf("button has zero size: %v (never laid out)", button.Size())
	}

	tapButtonCenter(t, button)

	if fake.quits != 1 {
		t.Fatalf("stop taps = %d, want 1", fake.quits)
	}
}
