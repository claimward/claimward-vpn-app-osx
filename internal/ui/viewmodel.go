// SPDX-License-Identifier: BSD-3-Clause
//
// The ViewModel. It holds ALL of the window's and the tray's state as
// go-widgets/mvvm primitives and NEVER references a widget: the View
// (view.go) and the tray (tray_binding.go) bind to these, so state only
// ever changes here.
//
// The logic itself -- what the status means, which button is on offer, the
// sign-in, the tenant choice, adding a server, the polling -- is
// claimward-vpn-client's pkg/appvm, shared with the other apps. This file is
// the adapter: every appvm.State is copied into the observables on the UI
// thread (through the post function), and every command is a call on the
// appvm.Model. What the app adds on top is what the model does not know: the
// helper's approval of a server, the preview of a server before it is added,
// the servers' logos, which servers exclude which, and the detail panel's
// facts.

package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/claimward/claimward-vpn-client/pkg/appcore"
	"github.com/claimward/claimward-vpn-client/pkg/appvm"
	"github.com/claimward/claimward-vpn-client/pkg/exclusive"
	"github.com/go-widgets/mvvm"

	"github.com/claimward/claimward-vpn-app-osx/internal/backend"
)

// Core is what the ViewModel needs: the client's service, and what the app
// adds to it (see internal/backend).
type Core interface {
	appvm.Service
	// Preview reads a server's own description, without adding it.
	Preview(ctx context.Context, rawURL string) (backend.Preview, error)
	// Approved says whether the helper trusts the server; LeaseExpires is when
	// its tunnel's lease ends (zero when there is none). Both are cached by the
	// last Status: they never wait on the helper.
	Approved(server string) bool
	LeaseExpires(server string) time.Time
	// Revoke makes the helper forget the server.
	Revoke(ctx context.Context, server string) error
}

var (
	_ Core = (*backend.Backend)(nil)
	_      = (*appcore.Core)(nil)
)

// Pages of the window, and the cards of its optional panels: each is the
// index of a card in a CardLayout the View binds to the observable.
const (
	PageStatus   = 0
	PageSettings = 1

	PanelHidden = 0
	PanelShown  = 1

	// The server window's cards: the URL to type, the preview of what that URL
	// answered, and the edit of an entry already added.
	FormURL     = 0
	FormPreview = 1
	FormEdit    = 2
)

// The detail panel's facts, in the order they are shown. Each has a value
// observable (Facts); the View names them (factKeys). A fact whose value is
// empty is hidden, except the status.
const (
	factURL = iota
	factProvider
	factAccount
	factExclusive
	factApproval
	factHelper
	factTenant
	factStatus
	factAddress
	factLease
	nFacts
)

// maxProviders is how many sign-in buttons there are: a server lists one to
// four providers in practice.
const maxProviders = 4

// Bits of Shown, what the detail panel's layout shows: a fact per bit, then a
// sign-in button per provider, then the tenant choice.
const (
	shownProvider = nFacts
	shownTenant   = nFacts + maxProviders
	// shownNote says the connect note has text.
	shownNote = shownTenant + 1
)

// Kinds of status, for the status row's icon.
const (
	StatusIdle = iota
	StatusUp
	StatusProblem
)

// Accessible names of the gear: its action, and what is missing when it is off.
const (
	editServerName     = "Edit server"
	editServerNoChoice = "Edit server (select a server first)"
)

// ViewModel is the application state.
type ViewModel struct {
	m    *appvm.Model
	core Core
	// post brings a result back to the UI thread, the only one that touches an
	// observable.
	post func(func())
	now  func() time.Time
	wg   sync.WaitGroup

	// syncing is set while a State is copied, so that what the model says is
	// not taken for the person choosing.
	syncing bool
	// last is the State last copied (UI thread).
	last appvm.State

	// Page is the window's page (PageStatus, PageSettings).
	Page *mvvm.Observable[int]

	// Facts are the detail panel's values, one per fact; Shown says which of
	// them (and of the buttons) the panel lays out; StatusKind is the status
	// row's icon (StatusIdle, StatusUp, StatusProblem).
	Facts      [nFacts]*mvvm.Observable[string]
	Shown      *mvvm.Observable[int]
	StatusKind *mvvm.Observable[int]

	// Busy says what is under way ("" when nothing is); Message is the last
	// error or notice.
	Busy, Message *mvvm.Observable[string]
	notice        string

	// SignInPanel shows the device-flow prompt while a sign-in waits for
	// the person: DeviceCode to type at DeviceURI.
	SignInPanel           *mvvm.Observable[int]
	DeviceURI, DeviceCode *mvvm.Observable[string]

	// TenantNames are the tenant choice's rows (the "no choice" row first),
	// TenantIndex the selected one, TenantName its title (what a native pop-up
	// button reports).
	TenantNames *mvvm.ObservableList[string]
	TenantIndex *mvvm.Observable[int]
	TenantName  *mvvm.Observable[string]

	// Servers are the sidebar's rows, one per server: "name, status". ServerIndex
	// is the selected one, ServerName heads the detail panel.
	Servers     *mvvm.ObservableList[string]
	ServerIndex *mvvm.Observable[int]
	ServerName  *mvvm.Observable[string]
	// LogoTick changes whenever a logo arrives, to repaint the pictures.
	LogoTick *mvvm.Observable[int]
	rows     []appvm.ServerRow
	logos    map[string]string // server URL -> SVG ("" when it has none)
	fetched  map[string]string // server URL -> the url+name a logo was asked for

	// SignInWith are the sign-in buttons, one per provider; ProviderLabels
	// their titles.
	SignInWith     [maxProviders]*mvvm.Command
	ProviderLabels [maxProviders]*mvvm.Observable[string]
	// pending is a sign-in waiting for the provider just chosen to be in force.
	pending int

	// ConnectNote says why Connect is off when the servers exclude each other.
	ConnectNote *mvvm.Observable[string]
	connectOK   *mvvm.Observable[bool]

	// Log is the connection log, oldest first.
	Log *mvvm.ObservableList[string]

	// The server window. FormCard is its card (FormURL, FormPreview, FormEdit).
	// ServerURL is the URL typed (or, editing, the entry's), FormMsg what
	// happened to it. The preview's name and what it says of the server:
	// PreviewName, PreviewDetail, and its logo (PreviewLogoDoc).
	FormCard                   *mvvm.Observable[int]
	ServerURL, FormMsg         *mvvm.Observable[string]
	PreviewName, PreviewDetail *mvvm.Observable[string]
	previewLogo                string
	previewURL                 string
	// Providers are the identity providers of the entry being edited; the
	// chosen one is ProviderIndex, whose title is ProviderName (what a native
	// pop-up button reports).
	Providers              *mvvm.ObservableList[string]
	ProviderIndex          *mvvm.Observable[int]
	ProviderName           *mvvm.Observable[string]
	Next, ConfirmAdd, Back *mvvm.Command

	// SettingsInWindow is true while the server window has a window of its own
	// (WithSettingsWindow): the main window then keeps its status page.
	SettingsInWindow *mvvm.Observable[bool]
	// PageShown is the page the main window shows: Page, except that it stays
	// on the status page while the server window has a window of its own.
	PageShown *mvvm.Observable[int]

	// RemovePanel shows the question of removing the selected server;
	// RemoveText is the question. ConfirmRevoke (remove and forget the helper's
	// approval) can run only for a server the helper approved.
	RemovePanel                                *mvvm.Observable[int]
	RemoveText                                 *mvvm.Observable[string]
	ConfirmRemove, ConfirmRevoke, CancelRemove *mvvm.Command
	canRevoke                                  *mvvm.Observable[bool]

	// TrayStatus is the tray's status line and tooltip.
	TrayStatus *mvvm.Observable[string]

	CancelSignIn, OpenSignInPage *mvvm.Command
	Connect, Disconnect, SignOut *mvvm.Command
	ChooseTenant                 *mvvm.Command
	CloseSettings                *mvvm.Command
	// AddServer and RemoveServer are the sidebar's + and − buttons.
	AddServer, RemoveServer *mvvm.Command
	// EditServer is the sidebar's gear and the panel's Edit button: it opens
	// the server window with the SELECTED entry, and cannot run with none
	// selected. EditServerLabel is the gear's accessible name: "Edit server",
	// plus the reason while it is disabled.
	EditServer      *mvvm.Command
	EditServerLabel *mvvm.Observable[string]
	canEdit         *mvvm.Observable[bool]
	canRemove       *mvvm.Observable[bool]

	// can is what the model allows now, so commands re-evaluate with it.
	can *mvvm.Observable[appvm.Can]
	// options are the tenant rows last copied, to rebuild the list only when
	// they change.
	options []appvm.TenantOption

	// openWindow and closeWindow are the server window's: see
	// WithSettingsWindow.
	openWindow  func() (closeWindow func(), ok bool)
	closeWindow func()
}

// Option configures a ViewModel.
type Option func(*config)

type config struct {
	post       func(func())
	now        func() time.Time
	opts       appvm.Options
	openWindow func() (func(), bool)
}

// WithSettingsWindow gives the server window a window of its own. open opens
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

// WithClock replaces the clock the lease's relative time is read against.
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// NewViewModel builds the ViewModel over core. Call Start to begin polling.
func NewViewModel(core Core, opts ...Option) *ViewModel {
	c := config{post: func(f func()) { f() }, now: time.Now}
	for _, o := range opts {
		o(&c)
	}
	m := appvm.New(core, c.opts)
	vm := &ViewModel{
		m: m, core: core, post: c.post, now: c.now, openWindow: c.openWindow,
		SettingsInWindow: mvvm.NewObservable(false),
		PageShown:        mvvm.NewObservable(PageStatus),

		Page:       mvvm.NewObservable(PageStatus),
		Shown:      mvvm.NewObservable(0),
		StatusKind: mvvm.NewObservable(StatusIdle),
		Busy:       mvvm.NewObservable(""),
		Message:    mvvm.NewObservable(""),

		SignInPanel: mvvm.NewObservable(PanelHidden),
		DeviceURI:   mvvm.NewObservable(""),
		DeviceCode:  mvvm.NewObservable(""),

		TenantNames: mvvm.NewObservableList[string](),
		TenantIndex: mvvm.NewObservable(0),
		TenantName:  mvvm.NewObservable(""),

		Servers:     mvvm.NewObservableList[string](),
		ServerIndex: mvvm.NewObservable(0),
		ServerName:  mvvm.NewObservable("No server"),
		LogoTick:    mvvm.NewObservable(0),
		logos:       map[string]string{},
		fetched:     map[string]string{},
		pending:     -1,

		ConnectNote: mvvm.NewObservable(""),
		connectOK:   mvvm.NewObservable(false),

		Log: mvvm.NewObservableList[string](),

		FormCard:      mvvm.NewObservable(FormURL),
		ServerURL:     mvvm.NewObservable(""),
		FormMsg:       mvvm.NewObservable(""),
		PreviewName:   mvvm.NewObservable(""),
		PreviewDetail: mvvm.NewObservable(""),
		Providers:     mvvm.NewObservableList[string](),
		ProviderIndex: mvvm.NewObservable(0),
		ProviderName:  mvvm.NewObservable(""),

		RemovePanel: mvvm.NewObservable(PanelHidden),
		RemoveText:  mvvm.NewObservable(""),
		canRevoke:   mvvm.NewObservable(false),

		TrayStatus: mvvm.NewObservable("Claimward"),
		can:        mvvm.NewObservable(appvm.Can{}),
	}
	for i := range vm.Facts {
		vm.Facts[i] = mvvm.NewObservable("")
	}
	for i := range vm.ProviderLabels {
		vm.ProviderLabels[i] = mvvm.NewObservable("")
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
	vm.ServerIndex.SubscribeChanged(func() {
		if !vm.syncing {
			m.SelectServer(vm.ServerIndex.Get())
		}
	})
	vm.ProviderIndex.SubscribeChanged(func() {
		if !vm.syncing {
			m.SelectProvider(vm.ProviderIndex.Get())
		}
	})
	vm.TenantName.Subscribe(func(name string) { vm.pick(vm.TenantNames, vm.TenantIndex, name) })
	vm.ProviderName.Subscribe(func(name string) { vm.pick(vm.Providers, vm.ProviderIndex, name) })
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

// pick moves index to the row of rows titled name, which is what a native
// pop-up button reports; while a State is copied the index is already the row
// the name belongs to, so nothing moves.
func (vm *ViewModel) pick(rows *mvvm.ObservableList[string], index *mvvm.Observable[int], name string) {
	list := rows.Slice()
	if i := index.Get(); i >= 0 && i < len(list) && list[i] == name {
		return
	}
	if i := slices.Index(list, name); i >= 0 {
		index.Set(i)
	}
}

func (vm *ViewModel) commands() {
	cmd := func(run func(), ok func(appvm.Can) bool) *mvvm.Command {
		c := mvvm.NewCommand(func() { vm.clearNotice(); run() }, func() bool { return ok(vm.can.Get()) })
		mvvm.BindCanExecute(c, vm.can)
		return c
	}
	vm.CancelSignIn = cmd(vm.m.CancelSignIn, func(c appvm.Can) bool { return c.CancelSignIn })
	vm.OpenSignInPage = cmd(vm.m.OpenSignInPage, func(c appvm.Can) bool { return c.OpenSignInPage })
	vm.ChooseTenant = cmd(vm.m.ChooseTenant, func(c appvm.Can) bool { return c.ChooseTenant })
	vm.Disconnect = cmd(vm.m.Disconnect, func(c appvm.Can) bool { return c.Disconnect })
	vm.SignOut = cmd(vm.m.SignOut, func(c appvm.Can) bool { return c.SignOut })
	vm.CloseSettings = cmd(vm.m.CloseSettings, func(c appvm.Can) bool { return c.CloseSettings })
	vm.Next = cmd(vm.next, func(c appvm.Can) bool { return c.SaveSettings })
	vm.ConfirmAdd = cmd(vm.confirmAdd, func(c appvm.Can) bool { return c.SaveSettings })
	vm.Back = cmd(func() { vm.FormCard.Set(FormURL); vm.FormMsg.Set("") }, func(appvm.Can) bool { return true })
	vm.AddServer = cmd(vm.openAdd, func(c appvm.Can) bool { return c.OpenSettings })

	// Connect waits on the servers that exclude each other, as well as the model.
	vm.Connect = mvvm.NewCommand(func() { vm.clearNotice(); vm.m.Connect() }, vm.connectOK.Get)
	mvvm.BindCanExecute(vm.Connect, vm.connectOK)

	// The sign-in buttons, one per provider.
	for i := range vm.SignInWith {
		i := i
		vm.SignInWith[i] = mvvm.NewCommand(func() { vm.clearNotice(); vm.signInWith(i) },
			func() bool { return vm.can.Get().SignIn || vm.can.Get().ChooseProvider })
		mvvm.BindCanExecute(vm.SignInWith[i], vm.can)
	}

	// The gear acts on the selected row; − asks before it removes.
	vm.canEdit = mvvm.NewObservable(false)
	vm.canRemove = mvvm.NewObservable(false)
	vm.EditServerLabel = mvvm.NewObservable(editServerName)
	vm.EditServer = mvvm.NewCommand(func() { vm.clearNotice(); vm.openEdit() }, vm.canEdit.Get)
	mvvm.BindCanExecute(vm.EditServer, vm.canEdit)
	vm.RemoveServer = mvvm.NewCommand(vm.askRemove, vm.canRemove.Get)
	mvvm.BindCanExecute(vm.RemoveServer, vm.canRemove)
	always := func() bool { return true }
	vm.CancelRemove = mvvm.NewCommand(func() { vm.RemovePanel.Set(PanelHidden) }, always)
	vm.ConfirmRemove = mvvm.NewCommand(func() { vm.RemovePanel.Set(PanelHidden); vm.m.RemoveServer() }, always)
	vm.ConfirmRevoke = mvvm.NewCommand(vm.revokeAndRemove, vm.canRevoke.Get)
	mvvm.BindCanExecute(vm.ConfirmRevoke, vm.canRevoke)
	vm.can.Subscribe(func(appvm.Can) { vm.refreshEdit() })
	vm.ServerIndex.Subscribe(func(int) { vm.refreshEdit() })
}

// clearNotice forgets the last notice of the app's own (not the model's): the
// person acts again.
func (vm *ViewModel) clearNotice() { vm.notice = "" }

// notify shows a notice of the app's own until the next action.
func (vm *ViewModel) notify(s string) {
	vm.notice = s
	vm.Message.Set(s)
}

// refreshEdit re-decides whether the gear and − may run: a server is selected
// and the model allows it.
func (vm *ViewModel) refreshEdit() {
	i := vm.ServerIndex.Get()
	selected := i >= 0 && i < len(vm.rows)
	vm.canEdit.Set(selected && vm.can.Get().OpenSettings)
	vm.canRemove.Set(selected && vm.can.Get().RemoveServer)
	if selected {
		vm.EditServerLabel.Set(editServerName)
	} else {
		vm.EditServerLabel.Set(editServerNoChoice)
	}
}

// signInWith signs in with provider i of the selected server's list. When it
// is not the provider in force yet, it is chosen first, and the sign-in runs
// once the choice shows in the State.
func (vm *ViewModel) signInWith(i int) {
	st := vm.last
	if len(st.ProviderOptions) == 0 || (i == st.ProviderIndex && !st.NeedsProvider) {
		vm.m.SignIn()
		return
	}
	vm.pending = i
	vm.m.SelectProvider(i)
}

// resolvePending signs in with the provider chosen for it, once it is in force.
func (vm *ViewModel) resolvePending(st appvm.State) {
	if vm.pending < 0 {
		return
	}
	switch {
	case st.Error != "" || vm.pending >= len(st.ProviderOptions):
		vm.pending = -1
	case st.Busy == "" && st.ProviderIndex == vm.pending && !st.NeedsProvider:
		vm.pending = -1
		vm.m.SignIn()
	}
}

// openForm shows the server window on card, opening its own window when the
// platform has one.
func (vm *ViewModel) openForm(card int) {
	if _, ok := vm.m.OpenSettings(); !ok {
		return
	}
	vm.FormCard.Set(card)
	vm.FormMsg.Set("")
	if vm.openWindow != nil && vm.closeWindow == nil {
		if close, ok := vm.openWindow(); ok {
			vm.closeWindow = close
			vm.SettingsInWindow.Set(true)
		}
	}
}

// openAdd is +: the window asks for a URL and nothing else.
func (vm *ViewModel) openAdd() {
	vm.ServerURL.Set("")
	vm.PreviewName.Set("")
	vm.PreviewDetail.Set("")
	vm.previewLogo, vm.previewURL = "", ""
	vm.openForm(FormURL)
}

// openEdit is the gear: the window shows the selected entry.
func (vm *ViewModel) openEdit() {
	vm.ServerURL.Set(vm.last.Server)
	vm.openForm(FormEdit)
}

// next reads the server's own description of itself, for the person to see
// before the server is added.
func (vm *ViewModel) next() {
	raw := strings.TrimSpace(vm.ServerURL.Get())
	if raw == "" {
		vm.FormMsg.Set("Type the server's address, such as https://vpn.example.org.")
		return
	}
	vm.FormMsg.Set("Reading the server's description…")
	vm.spawn(func() {
		p, err := vm.core.Preview(context.Background(), raw)
		vm.post(func() { vm.previewed(raw, p, err) })
	})
}

// previewed shows what the server answered.
func (vm *ViewModel) previewed(raw string, p backend.Preview, err error) {
	if err != nil {
		vm.FormMsg.Set(fmt.Sprintf("%s did not answer as a Claimward server: %v", backend.Host(raw), err))
		return
	}
	vm.previewURL, vm.previewLogo = raw, string(p.Logo)
	vm.PreviewName.Set(p.Name)
	vm.PreviewDetail.Set(previewDetail(p))
	vm.LogoTick.Set(vm.LogoTick.Get() + 1)
	vm.FormMsg.Set("")
	vm.FormCard.Set(FormPreview)
}

func previewDetail(p backend.Preview) string {
	s := "Sign-in with " + strings.Join(p.Providers, ", ")
	if len(p.Providers) == 0 {
		s = "No sign-in provider listed"
	}
	if p.Exclusive {
		s += ". Exclusive: it requires being the only connection."
	}
	return s
}

// confirmAdd adds the previewed server.
func (vm *ViewModel) confirmAdd() { vm.m.SaveSettings(appvm.Form{ServerURL: vm.previewURL}) }

// askRemove shows the question of removing the selected server.
func (vm *ViewModel) askRemove() {
	vm.RemoveText.Set("Remove " + vm.ServerName.Get() + "?")
	vm.RemovePanel.Set(PanelShown)
}

// revokeAndRemove forgets the helper's approval of the selected server, then
// removes it. A refusal of the helper keeps the server, and says why.
func (vm *ViewModel) revokeAndRemove() {
	vm.RemovePanel.Set(PanelHidden)
	server := vm.last.Server
	vm.spawn(func() {
		err := vm.core.Revoke(context.Background(), server)
		vm.post(func() {
			if err != nil {
				vm.notify("Revoking the approval of " + backend.Host(server) + " failed: " + err.Error())
				return
			}
			vm.m.RemoveServer()
		})
	})
}

// spawn runs work on a goroutine that Wait waits for.
func (vm *ViewModel) spawn(work func()) {
	vm.wg.Add(1)
	go func() { defer vm.wg.Done(); work() }()
}

// SettingsWindowClosed is told that the person closed the server window: the
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

// Model is the shared view model, for what the app does with its events.
func (vm *ViewModel) Model() *appvm.Model { return vm.m }

// Start polls the status until Stop. The first poll is immediate.
func (vm *ViewModel) Start() { vm.m.Start() }

// Stop ends the polling and cancels the action in progress.
func (vm *ViewModel) Stop() { vm.m.Stop() }

// Wait blocks until the poller and every action have finished and published
// their results. Results still have to be run on the UI thread.
func (vm *ViewModel) Wait() { vm.m.Wait(); vm.wg.Wait() }

// Refresh reads the status once, as a poll does.
func (vm *ViewModel) Refresh() { vm.m.Refresh() }

// Apply shows a status, as a poll does.
func (vm *ViewModel) Apply(st appcore.Status) { vm.m.Apply(st) }

// Tick refreshes what changes with the clock (the lease's relative time). The
// app calls it every second on the UI thread.
func (vm *ViewModel) Tick() { vm.facts(vm.last) }

// LogoDoc is the SVG of the selected server's logo, "" when it has none.
func (vm *ViewModel) LogoDoc() string { return vm.logos[vm.last.Server] }

// RowLogo is the SVG of the logo of the server on row i, "" when it has none.
func (vm *ViewModel) RowLogo(i int) string {
	if i < 0 || i >= len(vm.rows) {
		return ""
	}
	return vm.logos[vm.rows[i].URL]
}

// PreviewLogoDoc is the SVG of the logo of the server being added.
func (vm *ViewModel) PreviewLogoDoc() string { return vm.previewLogo }

// rowText is the sidebar's row: the server's name, then its state.
func rowText(r appvm.ServerRow) string {
	state := "Signed out"
	switch {
	case r.Connected:
		state = "Connected"
	case r.LoggedIn:
		state = "Signed in"
	}
	return r.Name + ", " + state
}

// apply copies a State into the observables. It runs on the UI thread.
func (vm *ViewModel) apply(st appvm.State) {
	vm.syncing = true
	defer func() { vm.syncing = false }()

	vm.last = st
	vm.rows = st.Servers
	vm.can.Set(st.Can)
	if st.Screen == appvm.ScreenSettings {
		vm.Page.Set(PageSettings)
	} else {
		vm.Page.Set(PageStatus)
	}
	vm.Busy.Set(st.Busy)
	switch {
	case st.SettingsError != "":
		vm.Message.Set(st.SettingsError)
		vm.FormMsg.Set(st.SettingsError)
	case st.Error != "":
		vm.Message.Set(st.Error)
	default:
		vm.Message.Set(vm.notice)
	}
	vm.TrayStatus.Set(st.TrayText)

	if st.Slot == appvm.SlotSignIn {
		vm.SignInPanel.Set(PanelShown)
	} else {
		vm.SignInPanel.Set(PanelHidden)
	}
	vm.DeviceURI.Set(st.SignInURL)
	vm.DeviceCode.Set(st.SignInCode)

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
	vm.TenantName.Set(at(vm.TenantNames.Slice(), st.TenantIndex))

	labels := make([]string, len(st.ProviderOptions))
	for i, o := range st.ProviderOptions {
		labels[i] = o.Label
	}
	if !slices.Equal(vm.Providers.Slice(), labels) {
		vm.Providers.Clear()
		vm.Providers.Append(labels...)
	}
	vm.ProviderIndex.Set(st.ProviderIndex)
	vm.ProviderName.Set(at(labels, st.ProviderIndex))
	vm.providerButtons(labels)

	lines := make([]string, len(st.Servers))
	for i, r := range st.Servers {
		lines[i] = rowText(r)
	}
	if !slices.Equal(vm.Servers.Slice(), lines) {
		vm.Servers.Clear()
		vm.Servers.Append(lines...)
	}
	vm.ServerIndex.Set(st.ServerIndex)
	if len(st.Servers) > 0 {
		vm.ServerName.Set(st.ServerName)
	} else {
		vm.ServerName.Set("No server")
	}
	vm.fetchLogos(st.Servers)
	vm.refreshEdit()
	vm.canRevoke.Set(vm.core.Approved(st.Server))

	note := exclusiveNote(st.Servers)
	vm.ConnectNote.Set(note)
	vm.connectOK.Set(st.Can.Connect && note == "")

	vm.facts(st)
	vm.resolvePending(st)

	loglines, appended := appvm.LogDelta(vm.Log.Slice(), st.Log)
	if !appended {
		vm.Log.Clear()
	}
	if len(loglines) > 0 || !appended {
		vm.Log.Append(loglines...)
	}
}

// at is rows[i], "" when there is no such row.
func at(rows []string, i int) string {
	if i < 0 || i >= len(rows) {
		return ""
	}
	return rows[i]
}

// providerButtons titles the sign-in buttons: one per provider the server
// lists ("Sign in" when it lists one or none).
func (vm *ViewModel) providerButtons(labels []string) {
	for i := range vm.ProviderLabels {
		title := ""
		switch {
		case i == 0 && len(labels) <= 1:
			title = "Sign in"
		case i < len(labels):
			title = "Sign in with " + labels[i]
		}
		vm.ProviderLabels[i].Set(title)
	}
}

// fetchLogos asks, once per server and name, for the logos not yet held.
func (vm *ViewModel) fetchLogos(rows []appvm.ServerRow) {
	live := map[string]bool{}
	for _, r := range rows {
		live[r.URL] = true
		key := r.URL + "\x00" + r.Name
		if vm.fetched[r.URL] == key {
			continue
		}
		vm.fetched[r.URL] = key
		vm.spawn(func() {
			svg := string(vm.core.Logo(context.Background(), r.URL))
			vm.post(func() {
				if vm.fetched[r.URL] != key {
					return
				}
				vm.logos[r.URL] = svg
				vm.LogoTick.Set(vm.LogoTick.Get() + 1)
			})
		})
	}
	for u := range vm.fetched {
		if !live[u] {
			delete(vm.fetched, u)
			delete(vm.logos, u)
		}
	}
}

// exclusiveNote is why Connect is off on the selected row: the connected
// servers and it refuse each other, in the helper's own words ("X is
// connected and exclusive", "Y requires being the only connection").
func exclusiveNote(rows []appvm.ServerRow) string {
	member := func(r appvm.ServerRow) exclusive.Member {
		return exclusive.Member{URL: r.URL, Name: r.Name, Exclusive: r.Exclusive}
	}
	var target *appvm.ServerRow
	var connected []exclusive.Member
	for i := range rows {
		switch {
		case rows[i].Selected:
			target = &rows[i]
		case rows[i].Connected:
			connected = append(connected, member(rows[i]))
		}
	}
	if target == nil || target.Connected {
		return ""
	}
	if err := exclusive.Check(member(*target), connected); err != nil {
		return err.Error()
	}
	return ""
}

// facts fills the detail panel from a State, with the clock's time.
func (vm *ViewModel) facts(st appvm.State) {
	has := len(st.Servers) > 0
	val := func(ok bool, s string) string {
		if ok {
			return s
		}
		return ""
	}
	set := func(i int, v string) { vm.Facts[i].Set(v) }
	set(factURL, val(has, st.Server))
	set(factProvider, val(has && len(st.ProviderOptions) > 0, providerValue(st)))
	set(factAccount, val(has, st.Account))
	set(factExclusive, val(has && st.Exclusive, "Yes: it requires being the only connection"))
	set(factApproval, val(has, vm.approval(st.Server)))
	set(factHelper, val(has, st.Helper))
	// The tenant is a fact while it cannot be chosen; the choice has its own row.
	choosing := st.Slot == appvm.SlotTenants && !st.Connected
	set(factTenant, val(has && !choosing, st.Tenant))
	set(factStatus, st.Headline)
	set(factAddress, val(has && st.Connected, st.Address))
	set(factLease, val(has && st.Connected, vm.lease(st.Server)))

	kind := StatusIdle
	switch {
	case st.Error != "":
		kind = StatusProblem
	case st.Connected:
		kind = StatusUp
	}
	vm.StatusKind.Set(kind)

	mask := 1 << factStatus
	for i := 0; i < nFacts; i++ {
		if vm.Facts[i].Get() != "" {
			mask |= 1 << i
		}
	}
	for i := range vm.ProviderLabels {
		if vm.ProviderLabels[i].Get() != "" && st.Actions == appvm.ActionsSignedOut {
			mask |= 1 << (shownProvider + i)
		}
	}
	if choosing {
		mask |= 1 << shownTenant
	}
	if vm.ConnectNote.Get() != "" {
		mask |= 1 << shownNote
	}
	vm.Shown.Set(mask)
}

func providerValue(st appvm.State) string {
	if st.NeedsProvider || st.ProviderIndex < 0 || st.ProviderIndex >= len(st.ProviderOptions) {
		return "Not chosen yet"
	}
	return st.ProviderOptions[st.ProviderIndex].Label
}

// approval says whether the helper trusts the server.
func (vm *ViewModel) approval(server string) string {
	if vm.core.Approved(server) {
		return "Approved on this Mac"
	}
	return "Not approved yet: you will be asked when you connect"
}

// lease is when the server's lease on the tunnel ends, relative to now.
func (vm *ViewModel) lease(server string) string {
	exp := vm.core.LeaseExpires(server)
	if exp.IsZero() {
		return ""
	}
	return relative(exp.Sub(vm.now()))
}

// relative says a duration the way a person reads it: "in 4 min", "in 2 h 5 min".
func relative(d time.Duration) string {
	switch {
	case d <= 0:
		return "expired"
	case d < time.Minute:
		return fmt.Sprintf("in %d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("in %d min", int(d.Minutes()))
	}
	h := int(d.Hours())
	return fmt.Sprintf("in %d h %d min", h, int(d.Minutes())%60)
}

func sameOptions(a, b []appvm.TenantOption) bool { return slices.Equal(a, b) }
