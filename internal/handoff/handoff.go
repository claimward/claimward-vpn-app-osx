// Package handoff passes the dashboard's loopback URL, which carries the
// access token, from the tray process to the webview process without putting
// it on a command line: another local user can read any process's argv with
// `ps`, but not a 0600 file in a 0700 directory.
package handoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const dirPrefix = "claimward-ui-"

// Write stores url in a new 0600 file inside a new 0700 directory and returns
// the file's path.
func Write(url string) (string, error) {
	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "url")
	if err := os.WriteFile(p, []byte(url), 0o600); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return p, nil
}

// Read returns the URL held in the file Write made, and deletes the file and
// its directory: the URL is good for one reader. It refuses a file another
// user could read or that is not one Write made.
func Read(p string) (string, error) {
	dir := filepath.Dir(p)
	if filepath.Base(p) != "url" || !strings.HasPrefix(filepath.Base(dir), dirPrefix) {
		return "", errors.New("handoff: not a file made by Write")
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("handoff: %s is not a private regular file (mode %v)", p, fi.Mode())
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	os.RemoveAll(dir)
	return string(b), nil
}
