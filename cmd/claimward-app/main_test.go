package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/claimward/claimward-vpn-app-osx/internal/uiserver"
)

// When the handoff file cannot be made, the dashboard is not started at all:
// the URL (with its token) must not fall back to a command line.
func TestOpenDashboardStopsWhenTheHandoffFileCannotBeMade(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	oldExe := osExecutable
	t.Cleanup(func() { osExecutable = oldExe })
	// A path that cannot be started, so a regression fails to launch instead of
	// re-running this test binary.
	osExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "no-such-claimward"), nil }

	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	a := &trayApp{ui: &uiserver.Server{}}
	a.openDashboard("")

	if !strings.Contains(logs.String(), "missing: no such file") {
		t.Errorf("log = %q, want the handoff directory error", logs.String())
	}
	if strings.Contains(logs.String(), "start failed") || a.uiCmd != nil {
		t.Errorf("the dashboard was started anyway: %q", logs.String())
	}
}
