// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/claimward/claimward-vpn-app-osx/internal/ui"
	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/go-widgets/application"
)

// A platform with one window (open fails) leaves the form on the main window.
func TestASecondWindowThatCannotOpenIsNotOffered(t *testing.T) {
	q := &ui.Queue{}
	vm := ui.NewViewModel(appcore.New(&appcore.Config{Provider: "github"}), ui.WithPost(q.Post))
	t.Cleanup(func() { vm.Stop(); vm.Wait() })
	var cfg application.WindowConfig
	closeWin, ok := openSettingsWindow(vm, q, func(c application.WindowConfig, _ application.Handler) (*application.Window, error) {
		cfg = c
		return nil, application.ErrNotSupported
	})
	if ok || closeWin != nil {
		t.Errorf("ok %v close %v", ok, closeWin != nil)
	}
	if cfg.Title != "Server settings" || cfg.Width != 440 {
		t.Errorf("window %+v", cfg)
	}
}
