package taskfile

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// The build tasks must not carry a go-task fingerprint (sources/generates): it
// did not list go.mod, go.sum or the embedded assets, so `task bundle` answered
// "up to date" after a dependency bump and shipped the previous binary. go
// build's own cache is the only judge of freshness.
func TestBuildTasksHaveNoFingerprint(t *testing.T) {
	f, err := os.Open("../../Taskfile.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line == "sources:" || line == "generates:" {
			t.Errorf("Taskfile.yml:%d: %q makes `task` skip a build after go.mod changes", n, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n < 50 {
		t.Fatalf("read %d lines of Taskfile.yml: not the Taskfile", n)
	}
}
