// Command claimward-app is the Claimward macOS client.
//
// It runs as a menu-bar (tray) application. The tray process owns all state and
// exposes a loopback HTTP API + the embedded Svelte UI; the user-facing window
// is a thin webview pointed at that loopback URL.
//
// Two modes:
//
//	claimward-app          run the tray (default)
//	claimward-app ui <url> run a webview window for <url> (spawned by the tray)
package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/claimward/claimward-vpn-app-osx/internal/appcore"
	"github.com/claimward/claimward-vpn-app-osx/internal/uiserver"
	webview "github.com/webview/webview_go"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "ui" {
		runUI(os.Args[2])
		return
	}
	runTray()
}

// runUI is the webview subprocess: a chromeless window rendering the Svelte SPA.
func runUI(url string) {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("Claimward")
	// HintNone sets the actual initial window size. HintMin only constrains the
	// minimum and leaves the window at 0x0 (invisible), so use HintNone here and
	// add a minimum separately.
	w.SetSize(440, 660, webview.HintNone)
	w.SetSize(360, 480, webview.HintMin)
	w.Navigate(url)
	w.Run()
}

func runTray() {
	cfg, err := appcore.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	core := appcore.New(cfg)

	ui, err := uiserver.Start(core)
	if err != nil {
		log.Fatalf("ui server: %v", err)
	}

	app := &trayApp{core: core, ui: ui}
	systray.Run(app.onReady, ui.Close)
}

type trayApp struct {
	core *appcore.Core
	ui   *uiserver.Server

	mu    sync.Mutex
	uiCmd *exec.Cmd
}

func (a *trayApp) onReady() {
	systray.SetTitle("Claimward")
	systray.SetTooltip("Claimward VPN")

	mStatus := systray.AddMenuItem("Disconnected", "Current status")
	mStatus.Disable()
	systray.AddSeparator()
	mOpen := systray.AddMenuItem("Open Claimward…", "Open the dashboard window")
	mConnect := systray.AddMenuItem("Connect", "Authenticate and bring up the tunnel")
	mDisconnect := systray.AddMenuItem("Disconnect", "Tear down the tunnel")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit Claimward")

	refresh := func() {
		st := a.core.Status()
		switch {
		case !st.ConfigOK:
			mStatus.SetTitle("⚠︎ Not configured")
		case st.Connected:
			label := "● Connected"
			if st.AssignedIP != "" {
				label = "● Connected (" + st.AssignedIP + ")"
			}
			mStatus.SetTitle(label)
		case st.LoggedIn:
			mStatus.SetTitle("○ Signed in — disconnected")
		default:
			mStatus.SetTitle("○ Signed out")
		}
		setEnabled(mConnect, st.ConfigOK && !st.Connected)
		setEnabled(mDisconnect, st.Connected)
	}
	refresh()

	// Each menu item is handled in its own goroutine, and status refresh runs
	// separately, so a slow/blocked helper call can never starve menu clicks.
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			refresh()
		}
	}()

	go func() {
		for range mOpen.ClickedCh {
			a.openDashboard()
		}
	}()

	go func() {
		for range mConnect.ClickedCh {
			a.connect()
			refresh()
		}
	}()

	go func() {
		for range mDisconnect.ClickedCh {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := a.core.Disconnect(ctx); err != nil {
				log.Printf("disconnect: %v", err)
			}
			cancel()
			refresh()
		}
	}()

	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

// connect signs in if needed, then brings up the tunnel.
func (a *trayApp) connect() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if !a.core.Status().LoggedIn {
		if err := a.core.Login(ctx); err != nil {
			log.Printf("login: %v", err)
			return
		}
	}
	if err := a.core.Connect(ctx); err != nil {
		log.Printf("connect: %v", err)
	}
}

// openDashboard shows the webview window.
//
// When running inside a Claimward.app bundle (the supported way), it launches
// the nested Dashboard.app via `open`, so LaunchServices activates it in the
// foreground — a bare binary spawned from a menu-bar agent stays behind other
// windows on modern macOS. Outside a bundle (dev `task start`) it falls back to
// a direct exec, which may open in the background.
func (a *trayApp) openDashboard() {
	a.mu.Lock()
	defer a.mu.Unlock()

	exe, err := os.Executable()
	if err != nil {
		log.Printf("open dashboard: executable path: %v", err)
		return
	}
	url := a.ui.URL()

	if dash := dashboardBundle(exe); dash != "" {
		// `open` (without -n) launches the dashboard or, if already running,
		// just brings it to the front.
		cmd := exec.Command("open", dash, "--args", "ui", url)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("open dashboard: %v", err)
			return
		}
		log.Printf("opened dashboard (bundled): %s", url)
		return
	}

	// Dev fallback (no .app bundle): direct exec.
	if a.uiCmd != nil && a.uiCmd.ProcessState == nil {
		log.Printf("dashboard already open")
		return
	}
	cmd := exec.Command(exe, "ui", url)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("open dashboard: start failed: %v", err)
		return
	}
	a.uiCmd = cmd
	go func() { _ = cmd.Wait(); a.clearUICmd() }()
	log.Printf("opened dashboard (dev, may open in background — use `task bundle`): %s", url)
}

// dashboardBundle returns the path to the nested Dashboard.app when exe lives in
// a Claimward.app bundle, else "".
func dashboardBundle(exe string) string {
	macos := filepath.Dir(exe)      // …/Contents/MacOS
	contents := filepath.Dir(macos) // …/Contents
	if filepath.Base(contents) != "Contents" {
		return ""
	}
	dash := filepath.Join(contents, "Resources", "Dashboard.app")
	if fi, err := os.Stat(dash); err == nil && fi.IsDir() {
		return dash
	}
	return ""
}

func (a *trayApp) clearUICmd() {
	a.mu.Lock()
	a.uiCmd = nil
	a.mu.Unlock()
}

func setEnabled(m *systray.MenuItem, enabled bool) {
	if enabled {
		m.Enable()
	} else {
		m.Disable()
	}
}
