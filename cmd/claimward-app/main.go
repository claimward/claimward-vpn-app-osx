// SPDX-License-Identifier: BSD-3-Clause

// Command claimward-app is the Claimward VPN desktop app for macOS: a window
// and a menu-bar status item, drawn by go-widgets with the system's own text
// fields and pop-up buttons embedded in it, that signs the person in, lets
// them choose a tenant, and asks the privileged helper (cmd/claimward-helper)
// to bring the tunnel up and down.
//
// It runs unprivileged, from the Claimward.app bundle: notifications need the
// bundle (UNUserNotificationCenter refuses a bare binary, and the app logs
// that and goes on).
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/claimward/claimward-vpn-app-osx/internal/backend"
	"github.com/claimward/claimward-vpn-app-osx/internal/brand"
	"github.com/claimward/claimward-vpn-app-osx/internal/ui"
	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/discovery"
	"github.com/claimward/claimward-vpn-client/pkg/helperclient"
	"github.com/claimward/claimward-vpn-client/pkg/macauth"
	"github.com/go-widgets/application"
	"github.com/go-widgets/tray"
)

// windowTitle names the window.
const windowTitle = "Claimward VPN"

// The main window's content size in points: WireGuard's macOS manager opens at
// 800 by 480 (AppDelegate.swift, setContentSize). application honours Config.Height
// from go-widgets/application v0.17.0; before, it opened screen-tall.
const (
	contentWidth  = 800
	contentHeight = 480
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "claimward-app:", err)
		os.Exit(1)
	}
}

// openSettingsWindow opens the settings form in a window of its own through
// open (application.OpenWindow) and returns how to close it. ok is false when
// the platform has no second window: the form then stays in the main window.
func openSettingsWindow(vm *ui.ViewModel, queue *ui.Queue, open func(application.WindowConfig, application.Handler) (*application.Window, error)) (func(), bool) {
	sv := ui.NewSettingsView(vm, queue)
	w, err := open(application.WindowConfig{Title: "Server", Width: 480, Height: 300}, sv)
	if err != nil {
		log.Printf("claimward-app: settings window: %v (showing the form in the main window)", err)
		sv.Close()
		return nil, false
	}
	w.OnClose(func() { sv.Close(); vm.SettingsWindowClosed() })
	return func() {
		if err := w.Close(); err != nil {
			log.Printf("claimward-app: close the settings window: %v", err)
		}
	}, true
}

func run() error {
	cfg, err := appcore.LoadConfig()
	if err != nil {
		// An unreadable config starts the app unconfigured, on the Settings
		// page, rather than not at all: it is how the person repairs it.
		log.Printf("claimward-app: load %s: %v (starting unconfigured)", appcore.ConfigPath(), err)
		cfg = &appcore.Config{}
	}
	// The core, plus what the app adds: the helper's approval of a server (the
	// system's own authorization dialog), the preview of a server before it is
	// added, and the helper's leases.
	core := backend.New(appcore.New(cfg), helperclient.New(cfg.SocketPath), discovery.New(nil), macauth.Obtain)

	queue := &ui.Queue{}
	var vm *ui.ViewModel
	// The settings form (Settings, and + under the servers) opens in a window
	// of its own. Where the platform has one window only, it stays a page of
	// the main window.
	settingsWindow := func() (func(), bool) {
		return openSettingsWindow(vm, queue, application.OpenWindow)
	}
	vm = ui.NewViewModel(core,
		ui.WithSettingsWindow(settingsWindow),
		ui.WithPost(queue.Post),
		ui.WithOpener(application.OpenURL),
		ui.WithInterval(time.Second, 500*time.Millisecond),
		ui.WithConfigPath("Saved to "+appcore.ConfigPath()),
	)
	if err := ui.UseOpenTypeText(); err != nil {
		log.Printf("claimward-app: falling back to the bitmap font: %v", err)
	}
	view := ui.NewView(vm, queue)
	defer view.Close()

	vm.Start()
	defer vm.Stop()
	// What changes with the clock (a lease's relative time) is refreshed every
	// second, as WireGuard's panel does.
	go func() {
		for range time.Tick(time.Second) {
			queue.Post(vm.Tick)
		}
	}()

	t := tray.New(brand.TrayTemplate).SetTooltip("Claimward")
	// Closing the window hides it (Config.HideOnClose): the app keeps running in
	// the status item and quits only through its Quit item. "Open" raises the
	// window again; Raise shows it if hidden, and activate brings the app, with
	// its window, in front of the other applications.
	openWindow := func() {
		if err := application.Raise(); err != nil {
			log.Printf("claimward-app: raise the window: %v", err)
		}
		activate()
	}
	unbind := ui.BindTray(t, vm, queue.Post, ui.TrayActions{
		Open: openWindow,
		Quit: func() { os.Exit(0) },
	})
	defer unbind()
	// The tunnel dropping or the session expiring is told to the person who is
	// not looking at the window. A notification that cannot be shown is logged.
	defer ui.BindNotifications(t, vm, openWindow, log.Printf)()
	defer t.Quit()

	spec := application.Spec{Name: "Claimward", Identifier: "com.claimward.app", Version: version}
	// Menu gives the menu bar the standard Edit menu: without it Cut, Copy and
	// Paste have no menu route in the embedded text fields.
	cfgApp := application.Config{Title: windowTitle, Width: contentWidth, Height: contentHeight, HideOnClose: true, Menu: tray.NewMenu().Add(tray.EditMenu())}
	return application.Run(spec, cfgApp, view, func() {
		// The status item joins once the window's loop runs. It is not a
		// Spec.Tray because that builds its menu once, and the binding above
		// changes the menu as the status changes.
		go func() {
			if err := t.Attach(); err != nil {
				log.Printf("claimward-app: no status item: %v", err)
			}
		}()
	})
}
