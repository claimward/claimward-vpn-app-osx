// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/claimward/claimward-vpn-app-osx/internal/backend"
	"github.com/claimward/claimward-vpn-app-osx/internal/ui"
	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/discovery"
	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/macauth"
	"github.com/go-widgets/application"
)

// testCore is the app's own core over an unconfigured client, as run builds it.
func testCore() *backend.Backend {
	cfg := &appcore.Config{}
	return backend.New(appcore.New(cfg), helperclient.New(cfg.SocketPath), discovery.New(nil), macauth.Obtain)
}

// A platform with one window (open fails) leaves the form on the main window.
func TestASecondWindowThatCannotOpenIsNotOffered(t *testing.T) {
	q := &ui.Queue{}
	vm := ui.NewViewModel(testCore(), ui.WithPost(q.Post))
	t.Cleanup(func() { vm.Stop(); vm.Wait() })
	var cfg application.WindowConfig
	// A panic (a broken error guard) is a failed assertion, so it does not end
	// the test binary before the tests after this one run.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("openSettingsWindow panicked: %v", r)
		}
	}()
	closeWin, ok := openSettingsWindow(vm, q, func(c application.WindowConfig, _ application.Handler) (*application.Window, error) {
		cfg = c
		return nil, application.ErrNotSupported
	})
	if ok || closeWin != nil {
		t.Errorf("ok %v close %v", ok, closeWin != nil)
	}
	if cfg.Title != "Server" || cfg.Width != 480 {
		t.Errorf("window %+v", cfg)
	}
}

// When the window cannot open, the whole path holds: no panic, nothing to
// close, and OpenSettings leaves the form on the main window's settings page.
// A panic is reported as an assertion, so a broken error guard fails by
// assertion and not only by crashing the test binary.
func TestTheFormStaysInTheMainWindowWhenTheWindowCannotOpen(t *testing.T) {
	q := &ui.Queue{}
	var vm *ui.ViewModel
	var opened int
	vm = ui.NewViewModel(testCore(),
		ui.WithPost(q.Post),
		ui.WithSettingsWindow(func() (func(), bool) {
			opened++
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("openSettingsWindow panicked: %v", r)
				}
			}()
			return openSettingsWindow(vm, q, func(application.WindowConfig, application.Handler) (*application.Window, error) {
				return nil, application.ErrNotSupported
			})
		}))
	t.Cleanup(func() { vm.Stop(); vm.Wait() })
	vm.Refresh()
	q.Drain()
	vm.AddServer.Execute()
	vm.Wait()
	q.Drain()
	if opened != 1 {
		t.Fatalf("open asked %d times", opened)
	}
	if vm.SettingsInWindow.Get() || vm.Page.Get() != ui.PageSettings {
		t.Errorf("inWindow %v page %d: the form must stay in the main window", vm.SettingsInWindow.Get(), vm.Page.Get())
	}
	if vm.PageShown.Get() != ui.PageSettings {
		t.Errorf("shown page %d, want the settings page", vm.PageShown.Get())
	}
}
