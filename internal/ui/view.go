// SPDX-License-Identifier: BSD-3-Clause
//
// The View. It builds the widgets from go-widgets/toolkit and binds every
// one of them to the ViewModel (viewmodel.go) through go-widgets/mvvmtk and
// go-widgets/mvvm. It NEVER writes a widget's state: the binders own that,
// so the file passes go-widgets/mvvmlint. What it does write is layout --
// boxes and their sizes -- which is the View's own business.
//
// On macOS the controls a person types into or chooses from are the system's
// own: an NSTextField for every text input, an NSPopUpButton for the identity
// provider and the tenant (host.go lists them for the window, which embeds
// them). The painted entries and drop-downs below them are only the fallback a
// host that does not claim the region shows, and what the tests exercise.
//
// The View is also the application.Handler the window drives (host.go):
// the framebuffer, the input, and the UI thread's clock.

package ui

import (
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/mvvmtk"
	"github.com/go-widgets/toolkit"
)

// Layout, in logical pixels (toolkit.Scaled turns them into device ones).
const (
	margin = 14
	gap    = 8
	rowH   = 26
	btnH   = 30
	titleH = 30
)

// widgets are the View's controls, built once and bound once.
type widgets struct {
	title    *toolkit.Label
	settings *toolkit.Button

	state, detail, account, tenant, helper *toolkit.Label
	busy, message                          *toolkit.Label

	deviceHint, deviceCode, deviceURI *toolkit.Label
	openPage, cancelSignIn            *toolkit.Button

	tenantLabel *toolkit.Label
	tenantPick  *toolkit.DropDown
	// nTenant is the native pop-up button over tenantPick.
	nTenant *toolkit.Native

	signIn, signOut, connect, disconnect, chooseTenant *toolkit.Button

	logTitle *toolkit.Label
	log      *toolkit.ListBox

	serverLabel, providerLabel, githubLabel, issuerLabel, clientLabel, pathLabel *toolkit.Label
	server, githubID, issuer, clientID                                           *toolkit.Entry
	// The text inputs are native controls (the OS's own text field, with its
	// input method); the entries above are their painted fallbacks.
	nServer, nGitHub, nIssuer, nClient *toolkit.Native
	provider                           *toolkit.DropDown
	// nProvider is the native pop-up button over provider.
	nProvider    *toolkit.Native
	save, cancel *toolkit.Button

	blankSignIn, blankTenant *toolkit.Label
}

// cards are the CardLayouts that show one of several things: the page, and
// the panels that appear only when they apply. Each one's Active is bound to
// a ViewModel observable.
//
// A CardLayout rather than a toolkit.Stack because the ViewModel's pages are
// ints that bind straight to Active, while Stack.Visible is a string
// observable and mvvm has no converting binding. Both keep the inactive pages
// out of the focus walk.
type cards struct {
	page, signIn, tenant, provider *toolkit.CardLayout
}

// bindView builds the widgets and binds them. mark(w) is told that w's
// content changed (the View then repaints, and reports w's rectangle as
// damaged); relayout asks for the cards to be laid out again, which damages
// the whole window.
func bindView(vm *ViewModel, mark func(toolkit.Widget), relayout func()) (*widgets, *cards, []func()) {
	w := &widgets{
		title:   toolkit.NewLabel("Claimward VPN"),
		state:   toolkit.NewLabel(""),
		detail:  toolkit.NewLabel(""),
		account: toolkit.NewLabel(""),
		tenant:  toolkit.NewLabel(""),
		helper:  toolkit.NewLabel(""),
		busy:    toolkit.NewLabel(""),
		message: toolkit.NewLabel(""),

		deviceHint:   toolkit.NewLabel("Open the sign-in page, then enter this code:"),
		deviceCode:   toolkit.NewLabel(""),
		deviceURI:    toolkit.NewLabel(""),
		openPage:     toolkit.NewButton("Open sign-in page", nil),
		cancelSignIn: toolkit.NewButton("Cancel sign-in", nil),

		tenantLabel: toolkit.NewLabel("Tenant for this session:"),
		tenantPick:  toolkit.NewDropDown(nil, 0),

		signIn:       toolkit.NewButton("Sign in", nil),
		signOut:      toolkit.NewButton("Sign out", nil),
		connect:      toolkit.NewButton("Connect", nil),
		disconnect:   toolkit.NewButton("Disconnect", nil),
		chooseTenant: toolkit.NewButton("Choose tenant", nil),

		logTitle: toolkit.NewLabel("Connection log"),
		log:      toolkit.NewListBox(nil),

		serverLabel:   toolkit.NewLabel("Server URL"),
		providerLabel: toolkit.NewLabel("Identity provider"),
		githubLabel:   toolkit.NewLabel("GitHub OAuth app client ID"),
		issuerLabel:   toolkit.NewLabel("OIDC issuer"),
		clientLabel:   toolkit.NewLabel("OIDC client ID"),
		pathLabel:     toolkit.NewLabel(""),
		server:        toolkit.NewEntry(""),
		githubID:      toolkit.NewEntry(""),
		issuer:        toolkit.NewEntry(""),
		clientID:      toolkit.NewEntry(""),
		provider:      toolkit.NewDropDown(providerLabels(), 0),
		save:          toolkit.NewButton("Save", nil),
		cancel:        toolkit.NewButton("Cancel", nil),

		blankSignIn: toolkit.NewLabel(""),
		blankTenant: toolkit.NewLabel(""),
	}
	w.settings = toolkit.NewButton("Settings", nil)
	w.server.Placeholder = "https://vpn.example.org"
	w.githubID.Placeholder = "Iv1.0123456789abcdef"
	w.issuer.Placeholder = "https://login.example.org"
	w.nServer = nativeEntry("server_url", w.server)
	w.nGitHub = nativeEntry("github_client_id", w.githubID)
	w.nIssuer = nativeEntry("oidc_issuer", w.issuer)
	w.nClient = nativeEntry("oidc_client_id", w.clientID)
	w.nProvider = nativePopUp("provider", providerLabels(), w.provider)
	w.nTenant = nativePopUp("tenant", nil, w.tenantPick)

	c := &cards{
		page: &toolkit.CardLayout{}, signIn: &toolkit.CardLayout{},
		tenant: &toolkit.CardLayout{}, provider: &toolkit.CardLayout{},
	}
	of := func(x toolkit.Widget) func() { return func() { mark(x) } }
	onCard := func() { relayout() }
	row := func(s string) string { return s }
	unbind := []func(){
		mvvmtk.BindLabel(w.state, vm.State, of(w.state)),
		mvvmtk.BindLabel(w.detail, vm.Detail, of(w.detail)),
		mvvmtk.BindLabel(w.account, vm.Account, of(w.account)),
		mvvmtk.BindLabel(w.tenant, vm.Tenant, of(w.tenant)),
		mvvmtk.BindLabel(w.helper, vm.Helper, of(w.helper)),
		mvvmtk.BindLabel(w.busy, vm.Busy, of(w.busy)),
		mvvmtk.BindLabel(w.message, vm.Message, of(w.message)),
		mvvmtk.BindLabel(w.deviceCode, vm.DeviceCode, of(w.deviceCode)),
		mvvmtk.BindLabel(w.deviceURI, vm.DeviceURI, of(w.deviceURI)),
		mvvmtk.BindLabel(w.pathLabel, vm.ConfigPath, of(w.pathLabel)),

		mvvmtk.BindCommand(w.settings, vm.OpenSettings, of(w.settings)),
		mvvmtk.BindCommand(w.openPage, vm.OpenSignInPage, of(w.openPage)),
		mvvmtk.BindCommand(w.cancelSignIn, vm.CancelSignIn, of(w.cancelSignIn)),
		mvvmtk.BindCommand(w.signIn, vm.SignIn, of(w.signIn)),
		mvvmtk.BindCommand(w.signOut, vm.SignOut, of(w.signOut)),
		mvvmtk.BindCommand(w.connect, vm.Connect, of(w.connect)),
		mvvmtk.BindCommand(w.disconnect, vm.Disconnect, of(w.disconnect)),
		mvvmtk.BindCommand(w.chooseTenant, vm.ChooseTenant, of(w.chooseTenant)),
		mvvmtk.BindCommand(w.save, vm.SaveSettings, of(w.save)),
		mvvmtk.BindCommand(w.cancel, vm.CloseSettings, of(w.cancel)),

		mvvmtk.BindDropDownOptions(w.tenantPick, vm.TenantNames, row, of(w.nTenant)),
		mvvmtk.BindSelectedIndex(w.tenantPick, vm.TenantIndex, of(w.nTenant)),
		mvvmtk.BindSelectedIndex(w.provider, vm.ProviderIndex, of(w.nProvider)),
		mvvmtk.BindListItems(w.log, vm.Log, row, of(w.log)),

		mvvmtk.BindEntryText(w.server, vm.ServerURL, of(w.nServer)),
		mvvmtk.BindEntryText(w.githubID, vm.GitHubClientID, of(w.nGitHub)),
		mvvmtk.BindEntryText(w.issuer, vm.OIDCIssuer, of(w.nIssuer)),
		mvvmtk.BindEntryText(w.clientID, vm.OIDCClientID, of(w.nClient)),
		// The native controls write the same observables the fallbacks read.
		mvvm.BindTwoWay(vm.ServerURL, w.nServer.Text(), of(w.nServer)),
		mvvm.BindTwoWay(vm.GitHubClientID, w.nGitHub.Text(), of(w.nGitHub)),
		mvvm.BindTwoWay(vm.OIDCIssuer, w.nIssuer.Text(), of(w.nIssuer)),
		mvvm.BindTwoWay(vm.OIDCClientID, w.nClient.Text(), of(w.nClient)),
		mvvm.BindTwoWay(vm.ProviderName, w.nProvider.Text(), of(w.nProvider)),
		mvvm.BindTwoWay(vm.TenantName, w.nTenant.Text(), of(w.nTenant)),
		mvvm.BindList(vm.TenantNames, &w.nTenant.Items, row, of(w.nTenant)),

		mvvm.OneWay(vm.Page, &c.page.Active, onCard),
		mvvm.OneWay(vm.SignInPanel, &c.signIn.Active, onCard),
		mvvm.OneWay(vm.TenantPanel, &c.tenant.Active, onCard),
		mvvm.OneWay(vm.ProviderPanel, &c.provider.Active, onCard),
	}
	return w, c, unbind
}

// nativeEntry is a native single-line text field keyed by its configuration
// key, painting fallback where no host claims it.
func nativeEntry(key string, fallback *toolkit.Entry) *toolkit.Native {
	n := toolkit.NewNativeEntry("")
	n.Key = key
	n.Fallback = fallback
	return n
}

// nativePopUp is a native pop-up button over items, keyed by key, painting
// fallback where no host claims it.
func nativePopUp(key string, items []string, fallback *toolkit.DropDown) *toolkit.Native {
	n := toolkit.NewNativePopUp(items, "")
	n.Key = key
	n.Fallback = fallback
	return n
}

func providerLabels() []string {
	out := make([]string, len(Providers))
	for i, p := range Providers {
		out[i] = p.Label
	}
	return out
}

// layout composes the widgets into boxes sized for the current metric
// scale. It is rebuilt when the scale changes (a window dragged to a
// display of another density), never per frame; the widgets and their
// bindings are untouched by it.
func layout(w *widgets, c *cards) toolkit.Widget {
	px := toolkit.Scaled
	row := func(ws ...toolkit.Widget) *toolkit.HBox {
		h := toolkit.NewHBox()
		h.Spacing = px(gap)
		for _, x := range ws {
			h.AddFlex(x, 1)
		}
		return h
	}
	card := func(l *toolkit.CardLayout, pages ...toolkit.Widget) *toolkit.Container {
		ct := toolkit.NewContainer(l)
		for _, p := range pages {
			ct.AddWidget(p)
		}
		return ct
	}

	device := toolkit.NewVBox()
	device.Spacing = px(4)
	device.AddFixed(w.deviceHint, px(rowH))
	device.AddFixed(w.deviceCode, px(rowH))
	device.AddFixed(w.deviceURI, px(rowH))
	device.AddFixed(row(w.openPage, w.cancelSignIn), px(btnH))

	tenantRow := toolkit.NewHBox()
	tenantRow.Spacing = px(gap)
	tenantRow.AddFixed(w.tenantLabel, px(170))
	tenantRow.AddFlex(w.nTenant, 1)

	status := toolkit.NewVBox()
	status.Spacing = px(4)
	status.AddFixed(w.state, px(rowH))
	status.AddFixed(w.detail, px(rowH))
	status.AddFixed(w.account, px(rowH))
	status.AddFixed(w.tenant, px(rowH))
	status.AddFixed(w.helper, px(rowH))
	status.AddFixed(card(c.signIn, w.blankSignIn, device), 3*px(rowH)+px(btnH)+3*px(4))
	status.AddFixed(card(c.tenant, w.blankTenant, tenantRow), px(btnH))
	status.AddFixed(row(w.signIn, w.signOut), px(btnH))
	status.AddFixed(row(w.connect, w.disconnect, w.chooseTenant), px(btnH))
	status.AddFixed(w.logTitle, px(rowH))
	status.AddFlex(w.log, 1)

	github := toolkit.NewVBox()
	github.Spacing = px(4)
	github.AddFixed(w.githubLabel, px(rowH))
	github.AddFixed(w.nGitHub, px(rowH))
	oidc := toolkit.NewVBox()
	oidc.Spacing = px(4)
	oidc.AddFixed(w.issuerLabel, px(rowH))
	oidc.AddFixed(w.nIssuer, px(rowH))
	oidc.AddFixed(w.clientLabel, px(rowH))
	oidc.AddFixed(w.nClient, px(rowH))

	settings := toolkit.NewVBox()
	settings.Spacing = px(4)
	settings.AddFixed(w.serverLabel, px(rowH))
	settings.AddFixed(w.nServer, px(rowH))
	settings.AddFixed(w.providerLabel, px(rowH))
	settings.AddFixed(w.nProvider, px(rowH))
	settings.AddFixed(card(c.provider, github, oidc), 4*px(rowH)+3*px(4))
	settings.AddFixed(row(w.save, w.cancel), px(btnH))
	settings.AddFixed(w.pathLabel, px(rowH))
	settings.AddFlex(toolkit.NewLabel(""), 1)

	header := toolkit.NewHBox()
	header.Spacing = px(gap)
	header.AddFlex(w.title, 1)
	header.AddFixed(w.settings, px(110))

	root := toolkit.NewVBox()
	root.Spacing = px(gap)
	root.AddFixed(header, px(titleH))
	root.AddFlex(card(c.page, status, settings), 1)
	root.AddFixed(w.busy, px(rowH))
	root.AddFixed(w.message, px(rowH))
	return toolkit.NewPopoverHost(root)
}
