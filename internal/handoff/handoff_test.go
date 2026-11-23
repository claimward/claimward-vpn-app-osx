package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripDeletesTheFile(t *testing.T) {
	p, err := Write("http://127.0.0.1:1/?t=secret")
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", fi.Mode().Perm())
	}
	di, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v, want 0700", di.Mode().Perm())
	}
	got, err := Read(p)
	if err != nil || got != "http://127.0.0.1:1/?t=secret" {
		t.Fatalf("Read = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Dir(p)); !os.IsNotExist(err) {
		t.Errorf("the directory is still there: %v", err)
	}
	if _, err := Read(p); err == nil {
		t.Error("a second Read succeeded")
	}
}

func TestReadRefusesAWorldReadableFile(t *testing.T) {
	p, err := Write("u")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Dir(p)) })
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(p); err == nil || !strings.Contains(err.Error(), "private") {
		t.Errorf("Read = %v, want a refusal", err)
	}
}

func TestReadRefusesAStrangerFileAndASymlink(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "url")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(f); err == nil || !strings.Contains(err.Error(), "not a file made by Write") {
		t.Errorf("a path outside a claimward-ui dir: %v", err)
	}
	if _, err := Read(filepath.Join(d, "other")); err == nil {
		t.Error("a wrong file name was read")
	}
	dir := filepath.Join(d, dirPrefix+"x")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f, filepath.Join(dir, "url")); err != nil {
		t.Skip("no symlinks here")
	}
	if _, err := Read(filepath.Join(dir, "url")); err == nil || !strings.Contains(err.Error(), "private regular") {
		t.Errorf("a symlink: %v", err)
	}
}

func TestWriteFailsWhenTheTempDirCannotBeMade(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := Write("u"); err == nil {
		t.Error("no error")
	}
}
