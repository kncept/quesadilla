package gui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

func menuLabels(g *QGUI) []string {
	labels := make([]string, 0, len(g.tray.Items))
	for _, item := range g.tray.Items {
		if item.IsSeparator {
			labels = append(labels, "-")
		} else {
			labels = append(labels, item.Label)
		}
	}
	return labels
}

func TestSingleInstanceLock(t *testing.T) {
	lockDir := t.TempDir()

	release, ok := acquireInstanceLock(lockDir)
	if !ok {
		t.Fatal("expected to acquire the single instance lock")
	}

	release()

	// the lock must be available again after release
	if _, ok := acquireInstanceLock(lockDir); !ok {
		t.Fatal("expected to acquire the lock again after release")
	}
}

// TestSingleInstanceLockCrossProcess verifies that a second process cannot
// acquire the lock while the first process holds it.
func TestSingleInstanceLockCrossProcess(t *testing.T) {
	const holderEnv = "QUESADILLA_TEST_LOCK_HOLDER"
	const dirEnv = "QUESADILLA_TEST_LOCK_DIR"

	if os.Getenv(holderEnv) == "1" {
		// child process: hold the lock for a while
		release, ok := acquireInstanceLock(os.Getenv(dirEnv))
		if !ok {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "locked")
		time.Sleep(10 * time.Second)
		release()
		return
	}

	lockDir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestSingleInstanceLockCrossProcess")
	cmd.Env = append(os.Environ(), holderEnv+"=1", dirEnv+"="+lockDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// wait for the child to report holding the lock
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
		close(lineCh)
	}()

	var ready bool
	timeout := time.After(30 * time.Second)
	for !ready {
		select {
		case line, ok := <-lineCh:
			if !ok {
				t.Fatal("lock holder exited before locking")
			}
			ready = strings.Contains(line, "locked")
		case err := <-errCh:
			t.Fatalf("scanner error: %v", err)
		case <-timeout:
			t.Fatal("timed out waiting for lock holder")
		}
	}

	if _, ok := acquireInstanceLock(lockDir); ok {
		t.Fatal("expected the lock to fail while another process holds it")
	}
}

func newTrayGui(t *testing.T) *QGUI {
	t.Helper()
	test.NewApp()
	g := CreateGui(nil)
	g.tray = fyne.NewMenu("Quesadilla")
	return g
}

func TestTrayMenuNoModelsRunning(t *testing.T) {
	g := newTrayGui(t)
	g.refreshTrayMenu()

	want := []string{"Open Control Suite", "-", "No models running", "-", "Quit"}
	if got := menuLabels(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected menu: %v", got)
	}
	last := g.tray.Items[len(g.tray.Items)-1]
	if !last.IsQuit {
		t.Fatal("quit item should have IsQuit set")
	}
}

func TestTrayMenuModelsRunning(t *testing.T) {
	g := newTrayGui(t)

	stop := running.Default().Track(modelDefinitions.Model{ModelName: "tiny-llama", ModelType: "gguf"})
	defer stop()
	g.refreshTrayMenu()

	want := []string{"Open Control Suite", "-", "Models running:", "  tiny-llama", "-", "Quit"}
	if got := menuLabels(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected menu: %v", got)
	}

	// a model added later is picked up on the next refresh
	stop2 := running.Default().Track(modelDefinitions.Model{ModelName: "big-llama", ModelType: "gguf"})
	defer stop2()
	g.refreshTrayMenu()

	want = []string{"Open Control Suite", "-", "Models running:", "  big-llama", "  tiny-llama", "-", "Quit"}
	if got := menuLabels(g); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected menu after refresh: %v", got)
	}
}
