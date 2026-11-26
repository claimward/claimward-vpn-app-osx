// SPDX-License-Identifier: BSD-3-Clause
//
// The ViewModel. It holds ALL of the window's and the tray's state as
// go-widgets/mvvm primitives and NEVER references a widget: the View
// (view.go) and the tray (tray_binding.go) bind to these, so state only
// ever changes here.
//
// The logic itself -- what the status means, which button is on offer, the
// sign-in, the tenant choice, the settings form, the polling -- is
// claimward-vpn-client's pkg/appvm, shared with the other apps. This file is
// the adapter: every appvm.State is copied into the observables on the UI
// thread (through the post function), and every command is a call on the
// appvm.Model. No decision is taken here.

package ui

import (
	"net/url"
	"slices"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/appvm"
	"github.com/go-widgets/mvvm"
)

// Core is what the ViewModel needs of appcore.Core.
type Core = appvm.Service

var _ Core = (*appcore.Core)(nil)

// Pages of the window, and the cards of its optional panels: each is the
// index of a card in a CardLayout the View binds to the observable.
const (
	PageStatus   = 0
	PageSettings = 1

	PanelHidden = 0
	PanelShown  = 1

	ProviderGitHub = 0
	ProviderOIDC   = 1 // also go-authn: both take an issuer and a client id
)

// Providers are the identity providers offered, in DropDown order.
var Providers = appvm.Providers

// ViewModel is the application state.
type ViewModel struct {
	m *appvm.Model
	// post brings a result back to the UI thread, the only one that touches an
	// observable.
	post func(func())

	// syncing is set while a State is copied, so that what the model says is
	// not taken for the person choosing.
	syncing bool

	// Page is the window's page (PageStatus, PageSettings).
	Page *mvvm.Observable[int]

	// State is the headline: Connected, Disconnected, Signed out, Not
	// configured. Detail is the line under it: the address and interface
	// when connected, the server otherwise, or what the configuration lacks.
	State, Detail *mvvm.Observable[string]
	// Account, Tenant and Helper are the status lines: who is signed in,
	// into which tenant, and whether the privileged helper answers.
	Account, Tenant, Helper *mvvm.Observable[string]
	// Busy says what is under way ("" when nothing is); Message is the last
	// error or notice.
	Busy, Message *mvvm.Observable[string]

	// SignInPanel shows the device-flow prompt while a sign-in waits for
	// the person: DeviceCode to type at DeviceURI.
	SignInPanel           *mvvm.Observable[int]
	DeviceURI, DeviceCode *mvvm.Observable[string]

	// TenantPanel shows the tenant choice when the server offered several,
	// or refused a connection until one is chosen. TenantNames are the
	// DropDown's rows (the "no choice" row first) and TenantIndex the selected
	// one.
	TenantPanel *mvvm.Observable[int]
	TenantNames *mvvm.ObservableList[string]
	TenantIndex *mvvm.Observable[int]
	// TenantName is the selected row's title, which is what a native pop-up
	// button reports; it follows TenantIndex and moves it when the person picks.
	TenantName *mvvm.Observable[string]

	// Servers are the sidebar's rows, one per configured server, named by host;
	// ServerIndex is the selected one, and ServerName heads the detail panel.
	Servers     *mvvm.ObservableList[string]
	ServerIndex *mvvm.Observable[int]
	ServerName  *mvvm.Observable[string]

	// Log is the connection log, oldest first.
	Log *mvvm.ObservableList[string]

	// The settings form. ProviderPanel shows the GitHub or the OIDC fields.
	ServerURL, GitHubClientID, OIDCIssuer, OIDCClientID *mvvm.Observable[string]
	ProviderIndex, ProviderPanel                        *mvvm.Observable[int]
	// ProviderName is the chosen provider's label: what a native pop-up button
	// reports. It follows ProviderIndex and moves it when the person picks.
	ProviderName *mvvm.Observable[string]
	ConfigPath   *mvvm.Observable[string]

	// SettingsInWindow is true while the settings form has a window of its own
	// (WithSettingsWindow): the main window then keeps its status page.
	SettingsInWindow *mvvm.Observable[bool]
	// PageShown is the page the main window shows: Page, except that it stays
	// on the status page while the settings form has a window of its own.
	PageShown *mvvm.Observable[int]

	// TrayStatus is the tray's status line and tooltip.
	TrayStatus *mvvm.Observable[string]

	SignIn, CancelSignIn, OpenSignInPage      *mvvm.Command
	Connect, Disconnect, SignOut              *mvvm.Command
	ChooseTenant                              *mvvm.Command
	OpenSettings, SaveSettings, CloseSettings *mvvm.Command
	// AddServer and RemoveServer are the sidebar's + and − buttons.
	AddServer, RemoveServer *mvvm.Command
	// EditServer is the sidebar's gear: it opens the server window prefilled
	// with the SELECTED entry, and cannot run with none selected.
	// EditServerLabel is the gear's accessible name: "Edit server", plus the
	// reason while it is disabled.
	EditServer      *mvvm.Command
	EditServerLabel *mvvm.Observable[string]
	canEdit         *mvvm.Observable[bool]

	// can is what the model allows now, so commands re-evaluate with it.
	can *mvvm.Observable[appvm.Can]
	// options are the tenant rows last copied, to rebuild the list only when
	// they change.
	options []appvm.TenantOption

	// openWindow and closeWindow are the settings window's: see
	// WithSettingsWindow.
	openWindow  func() (closeWindow func(), ok bool)
	closeWindow func()
}

// Option configures a ViewModel.
type Option func(*config)

type config struct {
	post       func(func())
	opts       appvm.Options
	openWindow func() (func(), bool)
}

// WithSettingsWindow gives the settings form a window of its own. open opens
// it and returns how to close it; ok false (a platform with one window only)
// leaves the form on the main window's settings page. The window is closed
// when the form is saved or cancelled; when the person closes it, the app
// calls SettingsWindowClosed.
func WithSettingsWindow(open func() (closeWindow func(), ok bool)) Option {
	return func(c *config) { c.openWindow = open }
}

// WithOpener replaces how a URL is opened (application.OpenURL in the app).
func WithOpener(open func(string) error) Option { return func(c *config) { c.opts.Open = open } }

// WithPost replaces how a result comes back to the UI thread: post runs a
// function there. The default runs it at once, which is right for a test that
// calls Apply and nothing else.
func WithPost(post func(func())) Option { return func(c *config) { c.post = post } }

// WithInterval sets the polling intervals (see appvm.Options).
func WithInterval(every, fast time.Duration) Option {
	return func(c *config) { c.opts.Interval, c.opts.FastInterval = every, fast }
}

// WithConfigPath names where the settings are saved, for the form to say.
func WithConfigPath(p string) Option { return func(c *config) { c.opts.ConfigPath = p } }

// NewViewModel builds the ViewModel over core. Call Start to begin polling.
func NewViewModel(core Core, opts ...Option) *ViewModel {
	c := config{post: func(f func()) { f() }}
	for _, o := range opts {
		o(&c)
	}
	m := appvm.New(core, c.opts)
	vm := &ViewModel{
		m: m, post: c.post, openWindow: c.openWindow,
		SettingsInWindow: mvvm.NewObservable(false),
		PageShown:        mvvm.NewObservable(PageStatus),

		Page:    mvvm.NewObservable(PageStatus),
		State:   mvvm.NewObservable("Loading..."),
		Detail:  mvvm.NewObservable(""),
		Account: mvvm.NewObservable(""),
		Tenant:  mvvm.NewObservable(""),
		Helper:  mvvm.NewObservable(""),
		Busy:    mvvm.NewObservable(""),
		Message: mvvm.NewObservable(""),

		SignInPanel: mvvm.NewObservable(PanelHidden),
		DeviceURI:   mvvm.NewObservable(""),
		DeviceCode:  mvvm.NewObservable(""),

		TenantPanel: mvvm.NewObservable(PanelHidden),
		TenantNames: mvvm.NewObservableList[string](),
		TenantIndex: mvvm.NewObservable(0),
		TenantName:  mvvm.NewObservable(""),

		Servers:     mvvm.NewObservableList[string](),
		ServerIndex: mvvm.NewObservable(0),
		ServerName:  mvvm.NewObservable(""),

		Log: mvvm.NewObservableList[string](),

		ServerURL:      mvvm.NewObservable(""),
		GitHubClientID: mvvm.NewObservable(""),
		OIDCIssuer:     mvvm.NewObservable(""),
		OIDCClientID:   mvvm.NewObservable(""),
		ProviderIndex:  mvvm.NewObservable(0),
		ProviderPanel:  mvvm.NewObservable(ProviderGitHub),
		ProviderName:   mvvm.NewObservable(Providers[0].Label),
		ConfigPath:     mvvm.NewObservable(c.opts.ConfigPath),

		TrayStatus: mvvm.NewObservable("Claimward"),
		can:        mvvm.NewObservable(appvm.Can{}),
	}
	vm.commands()
	shown := func() {
		if vm.SettingsInWindow.Get() {
			vm.PageShown.Set(PageStatus)
		} else {
			vm.PageShown.Set(vm.Page.Get())
		}
	}
	vm.Page.SubscribeChanged(shown)
	vm.SettingsInWindow.SubscribeChanged(shown)
	vm.TenantIndex.SubscribeChanged(func() {
		if !vm.syncing {
			m.SelectTenant(vm.TenantIndex.Get())
		}
	})
	vm.ProviderIndex.Subscribe(func(i int) {
		if i >= 0 && i < len(Providers) && appvm.UsesIssuer(Providers[i].ID) {
			vm.ProviderPanel.Set(ProviderOIDC)
		} else {
			vm.ProviderPanel.Set(ProviderGitHub)
		}
		if i >= 0 && i < len(Providers) {
			vm.ProviderName.Set(Providers[i].Label)
		}
	})
	vm.ProviderName.Subscribe(func(name string) {
		for i, p := range Providers {
			if p.Label == name {
				vm.ProviderIndex.Set(i)
				return
			}
		}
	})
	vm.TenantName.Subscribe(func(name string) {
		// While a State is copied the index is already the row this name
		// belongs to, so the check below returns.
		rows := vm.TenantNames.Slice()
		if i := vm.TenantIndex.Get(); i >= 0 && i < len(rows) && rows[i] == name {
			return
		}
		for i, r := range rows {
			if r == name {
				vm.TenantIndex.Set(i)
				return
			}
		}
	})
	// The form saved or cancelled: its window has done its work.
	vm.Page.Subscribe(func(p int) {
		if p != PageSettings && vm.closeWindow != nil {
			close := vm.closeWindow
			vm.closeWindow = nil
			vm.SettingsInWindow.Set(false)
			close()
		}
	})
	vm.apply(m.State())
	// Subscriber callbacks run on any goroutine and must not touch the
	// observables: the state is carried to the UI thread.
	m.Subscribe(func(st appvm.State) { vm.post(func() { vm.apply(st) }) })
	return vm
}

func (vm *ViewModel) commands() {
	cmd := func(run func(), ok func(appvm.Can) bool) *mvvm.Command {
		c := mvvm.NewCommand(run, func() bool { return ok(vm.can.Get()) })
		mvvm.BindCanExecute(c, vm.can)
		return c
	}
	vm.SignIn = cmd(vm.m.SignIn, func(c appvm.Can) bool { return c.SignIn })
	vm.CancelSignIn = cmd(vm.m.CancelSignIn, func(c appvm.Can) bool { return c.CancelSignIn })
	vm.OpenSignInPage = cmd(vm.m.OpenSignInPage, func(c appvm.Can) bool { return c.OpenSignInPage })
	vm.ChooseTenant = cmd(vm.m.ChooseTenant, func(c appvm.Can) bool { return c.ChooseTenant })
	vm.Connect = cmd(vm.m.Connect, func(c appvm.Can) bool { return c.Connect })
	vm.Disconnect = cmd(vm.m.Disconnect, func(c appvm.Can) bool { return c.Disconnect })
	vm.SignOut = cmd(vm.m.SignOut, func(c appvm.Can) bool { return c.SignOut })
	vm.OpenSettings = cmd(vm.openSettings, func(c appvm.Can) bool { return c.OpenSettings })
	vm.SaveSettings = cmd(vm.saveSettings, func(c appvm.Can) bool { return c.SaveSettings })
	vm.CloseSettings = cmd(vm.m.CloseSettings, func(c appvm.Can) bool { return c.CloseSettings })
	// The configuration holds one server for now: + opens its form, and − has
	// nothing it may remove.
	vm.AddServer = cmd(vm.openSettings, func(c appvm.Can) bool { return c.OpenSettings })
	vm.RemoveServer = mvvm.NewCommand(func() {}, func() bool { return false })
	// The gear acts on the selected row. The configuration holds one server for
	// now, so editing it is opening its form.
	vm.canEdit = mvvm.NewObservable(false)
	vm.EditServerLabel = mvvm.NewObservable(editServerName)
	vm.EditServer = mvvm.NewCommand(vm.openSettings, vm.canEdit.Get)
	mvvm.BindCanExecute(vm.EditServer, vm.canEdit)
	vm.can.Subscribe(func(appvm.Can) { vm.refreshEdit() })
	vm.ServerIndex.Subscribe(func(int) { vm.refreshEdit() })
}

// Accessible names of the gear: its action, and what is missing when it is off.
const (
	editServerName     = "Edit server"
	editServerNoChoice = "Edit server (select a server first)"
)

// refreshEdit re-decides whether the gear may run: a server is selected and
// the form may open.
func (vm *ViewModel) refreshEdit() {
	i := vm.ServerIndex.Get()
	selected := i >= 0 && i < vm.Servers.Len()
	vm.canEdit.Set(selected && vm.can.Get().OpenSettings)
	if selected {
		vm.EditServerLabel.Set(editServerName)
	} else {
		vm.EditServerLabel.Set(editServerNoChoice)
	}
}

// serverLabel names a server by its host, the URL itself when it has none.
func serverLabel(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// openSettings fills the form from the configuration in use.
func (vm *ViewModel) openSettings() {
	f, ok := vm.m.OpenSettings()
	if !ok {
		return
	}
	vm.ServerURL.Set(f.ServerURL)
	vm.ProviderIndex.Set(appvm.ProviderIndex(f.Provider))
	vm.GitHubClientID.Set(f.GitHubClientID)
	vm.OIDCIssuer.Set(f.OIDCIssuer)
	vm.OIDCClientID.Set(f.OIDCClientID)
	if vm.openWindow != nil && vm.closeWindow == nil {
		if close, ok := vm.openWindow(); ok {
			vm.closeWindow = close
			vm.SettingsInWindow.Set(true)
		}
	}
}

// SettingsWindowClosed is told that the person closed the settings window: the
// form is cancelled, as its Cancel button would.
func (vm *ViewModel) SettingsWindowClosed() {
	if vm.closeWindow == nil {
		return
	}
	vm.closeWindow = nil
	vm.SettingsInWindow.Set(false)
	if vm.Page.Get() == PageSettings {
		vm.m.CloseSettings()
	}
}

// saveSettings hands the form to the model, which saves it (an incomplete
// configuration too) and says what is wrong with it.
func (vm *ViewModel) saveSettings() {
	i := vm.ProviderIndex.Get()
	if i < 0 || i >= len(Providers) {
		i = 0
	}
	vm.m.SaveSettings(appvm.Form{
		ServerURL: vm.ServerURL.Get(), Provider: Providers[i].ID,
		GitHubClientID: vm.GitHubClientID.Get(),
		OIDCIssuer:     vm.OIDCIssuer.Get(), OIDCClientID: vm.OIDCClientID.Get(),
	})
}

// Model is the shared view model, for what the app does with its events.
func (vm *ViewModel) Model() *appvm.Model { return vm.m }

// Start polls the status until Stop. The first poll is immediate.
func (vm *ViewModel) Start() { vm.m.Start() }

// Stop ends the polling and cancels the action in progress.
func (vm *ViewModel) Stop() { vm.m.Stop() }

// Wait blocks until the poller and every action have finished and published
// their results. Results still have to be run on the UI thread.
func (vm *ViewModel) Wait() { vm.m.Wait() }

// Refresh reads the status once, as a poll does.
func (vm *ViewModel) Refresh() { vm.m.Refresh() }

// Apply shows a status, as a poll does.
func (vm *ViewModel) Apply(st appcore.Status) { vm.m.Apply(st) }

// apply copies a State into the observables. It runs on the UI thread.
func (vm *ViewModel) apply(st appvm.State) {
	vm.syncing = true
	defer func() { vm.syncing = false }()

	vm.can.Set(st.Can)
	if st.Screen == appvm.ScreenSettings {
		vm.Page.Set(PageSettings)
	} else {
		vm.Page.Set(PageStatus)
	}
	vm.State.Set(st.Headline)
	switch {
	case st.Screen == appvm.ScreenUnconfigured:
		vm.Detail.Set(st.Problem)
	case st.Connected:
		vm.Detail.Set(st.Address)
	default:
		vm.Detail.Set(st.Server)
	}
	vm.Account.Set(st.Account)
	vm.Helper.Set(st.Helper)
	vm.Tenant.Set(st.Tenant)
	vm.Busy.Set(st.Busy)
	if st.SettingsError != "" {
		vm.Message.Set(st.SettingsError)
	} else {
		vm.Message.Set(st.Error)
	}
	vm.ConfigPath.Set(st.ConfigPath)
	vm.TrayStatus.Set(st.TrayText)

	if st.Slot == appvm.SlotSignIn {
		vm.SignInPanel.Set(PanelShown)
	} else {
		vm.SignInPanel.Set(PanelHidden)
	}
	vm.DeviceURI.Set(st.SignInURL)
	vm.DeviceCode.Set(st.SignInCode)

	// The choice is made before connecting: once connected the tenant is the
	// session's, and the status line says which.
	if st.Slot == appvm.SlotTenants && !st.Connected {
		vm.TenantPanel.Set(PanelShown)
	} else {
		vm.TenantPanel.Set(PanelHidden)
	}
	if !sameOptions(vm.options, st.TenantOptions) {
		vm.options = append([]appvm.TenantOption(nil), st.TenantOptions...)
		rows := make([]string, len(vm.options))
		for i, o := range vm.options {
			rows[i] = o.Label
		}
		vm.TenantNames.Clear()
		vm.TenantNames.Append(rows...)
	}
	vm.TenantIndex.Set(st.TenantIndex)
	if rows := vm.TenantNames.Slice(); st.TenantIndex >= 0 && st.TenantIndex < len(rows) {
		vm.TenantName.Set(rows[st.TenantIndex])
	} else {
		vm.TenantName.Set("")
	}

	var servers []string
	if st.Server != "" {
		servers = []string{serverLabel(st.Server)}
	}
	if !slices.Equal(vm.Servers.Slice(), servers) {
		vm.Servers.Clear()
		vm.Servers.Append(servers...)
	}
	vm.ServerIndex.Set(0)
	if len(servers) > 0 {
		vm.ServerName.Set(servers[0])
	} else {
		vm.ServerName.Set("No server")
	}
	vm.refreshEdit()

	lines, appended := appvm.LogDelta(vm.Log.Slice(), st.Log)
	if !appended {
		vm.Log.Clear()
	}
	if len(lines) > 0 || !appended {
		vm.Log.Append(lines...)
	}
}

func sameOptions(a, b []appvm.TenantOption) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
