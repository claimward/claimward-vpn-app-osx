// SPDX-License-Identifier: BSD-3-Clause
//
// The View. It builds the widgets from go-widgets/toolkit and binds every
// one of them to the ViewModel (viewmodel.go) through go-widgets/mvvmtk and
// go-widgets/mvvm. It NEVER writes a widget's state: the binders own that,
// so the file passes go-widgets/mvvmlint. What it does write is layout --
// boxes and their sizes -- which is the View's own business.
//
// The main window is a list of servers and, beside it, the selected server's
// detail, laid out like WireGuard's tunnel panel: sections with a title, each
// a table of key and value rows, every empty row hidden except the status.
//
// On macOS the controls a person types into or chooses from are the system's
// own: an NSTextField for the server's address, an NSPopUpButton for the
// identity provider and the tenant (host.go lists them for the window, which
// embeds them). The painted entries and drop-downs below them are only the
// fallback a host that does not claim the region shows, and what the tests
// exercise.
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
	// factH is a fact's row, rowH a label's, btnH a button's.
	factH    = 22
	rowH     = 26
	btnH     = 30
	titleH   = 36
	sidebarW = 210
	// editStripH is the strip of the Edit button, at the panel's bottom right.
	editStripH = 32
	editW      = 90
	// keyW is the width of the fact keys' column, iconW the status icon's.
	keyW  = 130
	iconW = 18
	// tenantLabelW and refreshW are the fixed ends of the tenant row.
	tenantLabelW = 60
	refreshW     = 90
	// The cards' heights, in logical pixels: the sign-in prompt is three rows
	// and a button row, the removal question a row and a button row.
	signInCardH = 3*rowH + btnH + 3*4
	removeCardH = rowH + btnH + 4
	// formW is the server window's content, whose cards are four rows tall.
	formCardH = 3*rowH + btnH + 3*4
)

// factKeys name the facts' rows, in the order of the fact constants.
var factKeys = [nFacts]string{
	factURL: "URL", factProvider: "Provider", factAccount: "Account",
	factExclusive: "Exclusive", factApproval: "Helper approval", factHelper: "Helper",
	factTenant: "Tenant", factStatus: "Status", factAddress: "Address", factLease: "Lease expires",
}

// widgets are the View's controls, built once and bound once.
type widgets struct {
	busy, message *toolkit.Label

	deviceHint, deviceCode, deviceURI *toolkit.Label
	openPage, cancelSignIn            *toolkit.Button

	// The sidebar: the configured servers, one row each, and the flat + and −
	// buttons under them; the detail panel shows the selected one.
	serversTitle *toolkit.Label
	servers      *toolkit.ListBox
	// serverPane frames the list over its bar of joined + and − buttons.
	serverPane *toolkit.ListPane

	// The detail panel: the selected server's logo and name, a section title
	// over its facts (a key and a value each), the status icon, the tenant choice.
	serverLogo               *logoView
	serverName               *toolkit.Label
	serverTitle, ifaceTitle  *toolkit.Label
	keys, values             [nFacts]*toolkit.Label
	statusIcon               *statusIcon
	tenantLabel              *toolkit.Label
	tenantPick               *toolkit.DropDown
	nTenant                  *toolkit.Native
	signInWith               [maxProviders]*toolkit.Button
	signOut, connect         *toolkit.Button
	disconnect, chooseTenant *toolkit.Button
	connectNote              *toolkit.Label
	edit                     *toolkit.Button

	// The removal question.
	removeText                                 *toolkit.Label
	confirmRemove, confirmRevoke, cancelRemove *toolkit.Button

	logTitle *toolkit.Label
	log      *toolkit.ListBox

	// The server window: the URL to type and Next; the preview (logo, name, what
	// it says), Add and Back; the edit (URL, provider); and the line that says
	// what happened.
	urlLabel, formMsg                    *toolkit.Label
	server                               *toolkit.Entry
	nServer                              *toolkit.Native
	next, cancel                         *toolkit.Button
	previewLogo                          *logoView
	previewName, previewDetail           *toolkit.Label
	add, back, cancelPreview             *toolkit.Button
	editURLLabel, editURL, providerLabel *toolkit.Label
	provider                             *toolkit.DropDown
	nProvider                            *toolkit.Native
	done                                 *toolkit.Button

	blank [4]*toolkit.Label

	// shown, logoTick and statusKind are what the ViewModel last said: which
	// rows the panel lays out, and what to repaint the pictures with.
	shown, logoTick, statusKind int
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
	page, signIn, remove, form *toolkit.CardLayout
}

// bindView builds the widgets and binds them. mark(w) is told that w's
// content changed (the View then repaints, and reports w's rectangle as
// damaged); relayout asks for the cards to be laid out again, which damages
// the whole window.
func bindView(vm *ViewModel, mark func(toolkit.Widget), relayout func()) (*widgets, *cards, []func()) {
	w := &widgets{
		busy:    toolkit.NewLabel(""),
		message: toolkit.NewLabel(""),

		deviceHint:   toolkit.NewLabel("Open the sign-in page, then enter this code:"),
		deviceCode:   toolkit.NewLabel(""),
		deviceURI:    toolkit.NewLabel(""),
		openPage:     toolkit.NewButton("Open sign-in page", nil),
		cancelSignIn: toolkit.NewButton("Cancel sign-in", nil),

		serversTitle: toolkit.NewLabel("Servers"),
		servers:      toolkit.NewListBox(nil),
		serverName:   toolkit.NewLabel(""),
		serverTitle:  toolkit.NewLabel("Server"),
		ifaceTitle:   toolkit.NewLabel("Interface"),
		tenantLabel:  toolkit.NewLabel("Tenant"),
		tenantPick:   toolkit.NewDropDown(nil, 0),

		signOut:      toolkit.NewButton("Sign out", nil),
		connect:      toolkit.NewButton("Connect", nil),
		disconnect:   toolkit.NewButton("Disconnect", nil),
		chooseTenant: toolkit.NewButton("Refresh", nil),
		connectNote:  toolkit.NewLabel(""),
		edit:         toolkit.NewButton("Edit", nil),

		removeText:    toolkit.NewLabel(""),
		confirmRemove: toolkit.NewButton("Remove", nil),
		confirmRevoke: toolkit.NewButton("Remove and revoke approval", nil),
		cancelRemove:  toolkit.NewButton("Cancel", nil),

		logTitle: toolkit.NewLabel("Connection log"),
		log:      toolkit.NewListBox(nil),

		urlLabel:      toolkit.NewLabel("Server address"),
		formMsg:       toolkit.NewLabel(""),
		server:        toolkit.NewEntry(""),
		next:          toolkit.NewButton("Next", nil),
		cancel:        toolkit.NewButton("Cancel", nil),
		previewName:   toolkit.NewLabel(""),
		previewDetail: toolkit.NewLabel(""),
		add:           toolkit.NewButton("Add server", nil),
		back:          toolkit.NewButton("Back", nil),
		cancelPreview: toolkit.NewButton("Cancel", nil),
		editURLLabel:  toolkit.NewLabel("Server address"),
		editURL:       toolkit.NewLabel(""),
		providerLabel: toolkit.NewLabel("Identity provider"),
		provider:      toolkit.NewDropDown(nil, 0),
		done:          toolkit.NewButton("Done", nil),
	}
	for i := range w.blank {
		w.blank[i] = toolkit.NewLabel("")
	}
	for i := range w.keys {
		w.keys[i] = toolkit.NewLabel(factKeys[i])
		w.values[i] = toolkit.NewLabel("")
	}
	for i := range w.signInWith {
		w.signInWith[i] = toolkit.NewButton("", nil)
	}
	w.serverLogo = newLogoView("Server logo", vm.LogoDoc)
	w.previewLogo = newLogoView("Server logo", vm.PreviewLogoDoc)
	w.statusIcon = newStatusIcon(func() int { return w.statusKind })
	w.serverName.SetFontSize(18)
	w.previewName.SetFontSize(18)
	w.serverTitle.SetFontSize(15)
	w.ifaceTitle.SetFontSize(15)
	w.servers.RowHeight = toolkit.Scaled(44)
	w.servers.ItemRenderer = serverRow(vm)
	w.serverPane = toolkit.NewListPane(w.servers, toolkit.ListBarMac)
	w.serverPane.AccessibleName = "Servers"
	w.serverPane.Extra = []*toolkit.Button{gearButton()}
	w.nTenant = nativePopUp("tenant", nil, w.tenantPick)
	w.server.Placeholder = "https://vpn.example.org"
	w.nServer = nativeEntry("server_url", w.server)
	w.nProvider = nativePopUp("provider", nil, w.provider)

	c := &cards{
		page: &toolkit.CardLayout{}, signIn: &toolkit.CardLayout{},
		remove: &toolkit.CardLayout{}, form: &toolkit.CardLayout{},
	}
	of := func(x toolkit.Widget) func() { return func() { mark(x) } }
	onCard := func() { relayout() }
	row := func(s string) string { return s }
	repaint := func() { mark(w.servers); mark(w.serverLogo); mark(w.previewLogo) }
	unbind := []func(){
		mvvmtk.BindLabel(w.busy, vm.Busy, of(w.busy)),
		mvvmtk.BindLabel(w.message, vm.Message, of(w.message)),
		mvvmtk.BindLabel(w.deviceCode, vm.DeviceCode, of(w.deviceCode)),
		mvvmtk.BindLabel(w.deviceURI, vm.DeviceURI, of(w.deviceURI)),
		mvvmtk.BindLabel(w.serverName, vm.ServerName, of(w.serverName)),
		mvvmtk.BindLabel(w.connectNote, vm.ConnectNote, of(w.connectNote)),
		mvvmtk.BindLabel(w.removeText, vm.RemoveText, of(w.removeText)),
		mvvmtk.BindLabel(w.formMsg, vm.FormMsg, of(w.formMsg)),
		mvvmtk.BindLabel(w.previewName, vm.PreviewName, of(w.previewName)),
		mvvmtk.BindLabel(w.previewDetail, vm.PreviewDetail, of(w.previewDetail)),
		mvvmtk.BindLabel(w.editURL, vm.ServerURL, of(w.editURL)),

		mvvmtk.BindCommand(w.openPage, vm.OpenSignInPage, of(w.openPage)),
		mvvmtk.BindCommand(w.cancelSignIn, vm.CancelSignIn, of(w.cancelSignIn)),
		mvvmtk.BindCommand(w.signOut, vm.SignOut, of(w.signOut)),
		mvvmtk.BindCommand(w.connect, vm.Connect, of(w.connect)),
		mvvmtk.BindCommand(w.disconnect, vm.Disconnect, of(w.disconnect)),
		mvvmtk.BindCommand(w.chooseTenant, vm.ChooseTenant, of(w.chooseTenant)),
		mvvmtk.BindCommand(w.edit, vm.EditServer, of(w.edit)),
		mvvmtk.BindCommand(w.serverPane.Add, vm.AddServer, of(w.serverPane)),
		mvvmtk.BindCommand(w.serverPane.Remove, vm.RemoveServer, of(w.serverPane)),
		mvvmtk.BindCommand(w.serverPane.Extra[0], vm.EditServer, of(w.serverPane)),
		mvvm.BindTwoWay(vm.EditServerLabel, w.serverPane.Extra[0].Label(), of(w.serverPane)),
		mvvmtk.BindCommand(w.confirmRemove, vm.ConfirmRemove, of(w.confirmRemove)),
		mvvmtk.BindCommand(w.confirmRevoke, vm.ConfirmRevoke, of(w.confirmRevoke)),
		mvvmtk.BindCommand(w.cancelRemove, vm.CancelRemove, of(w.cancelRemove)),
		mvvmtk.BindCommand(w.next, vm.Next, of(w.next)),
		mvvmtk.BindCommand(w.cancel, vm.CloseSettings, of(w.cancel)),
		mvvmtk.BindCommand(w.add, vm.ConfirmAdd, of(w.add)),
		mvvmtk.BindCommand(w.back, vm.Back, of(w.back)),
		mvvmtk.BindCommand(w.cancelPreview, vm.CloseSettings, of(w.cancelPreview)),
		mvvmtk.BindCommand(w.done, vm.CloseSettings, of(w.done)),

		mvvmtk.BindListItems(w.servers, vm.Servers, row, of(w.servers)),
		mvvmtk.BindListSelection(w.servers, vm.ServerIndex, of(w.servers)),
		mvvmtk.BindDropDownOptions(w.tenantPick, vm.TenantNames, row, of(w.nTenant)),
		mvvmtk.BindSelectedIndex(w.tenantPick, vm.TenantIndex, of(w.nTenant)),
		mvvm.BindTwoWay(vm.TenantName, w.nTenant.Text(), of(w.nTenant)),
		mvvm.BindList(vm.TenantNames, &w.nTenant.Items, row, of(w.nTenant)),
		mvvmtk.BindDropDownOptions(w.provider, vm.Providers, row, of(w.nProvider)),
		mvvmtk.BindSelectedIndex(w.provider, vm.ProviderIndex, of(w.nProvider)),
		mvvm.BindTwoWay(vm.ProviderName, w.nProvider.Text(), of(w.nProvider)),
		mvvm.BindList(vm.Providers, &w.nProvider.Items, row, of(w.nProvider)),
		mvvmtk.BindListItems(w.log, vm.Log, row, of(w.log)),

		mvvmtk.BindEntryText(w.server, vm.ServerURL, of(w.nServer)),
		// The native control writes the same observable the fallback reads.
		mvvm.BindTwoWay(vm.ServerURL, w.nServer.Text(), of(w.nServer)),

		mvvm.OneWay(vm.PageShown, &c.page.Active, onCard),
		mvvm.OneWay(vm.SignInPanel, &c.signIn.Active, onCard),
		mvvm.OneWay(vm.RemovePanel, &c.remove.Active, onCard),
		mvvm.OneWay(vm.FormCard, &c.form.Active, onCard),
		// What the panel lays out, and what the pictures show.
		mvvm.OneWay(vm.Shown, &w.shown, onCard),
		mvvm.OneWay(vm.StatusKind, &w.statusKind, of(w.statusIcon)),
		mvvm.OneWay(vm.LogoTick, &w.logoTick, repaint),
	}
	for i := range w.values {
		unbind = append(unbind, mvvmtk.BindLabel(w.values[i], vm.Facts[i], of(w.values[i])))
	}
	for i := range w.signInWith {
		unbind = append(unbind,
			mvvmtk.BindCommand(w.signInWith[i], vm.SignInWith[i], of(w.signInWith[i])),
			mvvm.BindTwoWay(vm.ProviderLabels[i], w.signInWith[i].Label(), of(w.signInWith[i])))
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

// visible says whether bit of the shown mask is set.
func visible(shown, bit int) bool { return shown&(1<<bit) != 0 }

// layout composes the widgets into boxes sized for the current metric
// scale and for what the panel shows. It is rebuilt when the scale or what is
// shown changes, never per frame; the widgets and their bindings are untouched
// by it.
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

	// The sidebar: the servers, framed over the bar of their + and − buttons.
	sidebar := toolkit.NewVBox()
	sidebar.Spacing = px(4)
	sidebar.AddFixed(w.serversTitle, px(rowH))
	sidebar.AddFlex(w.serverPane, 1)

	// A fact: its key at the left, its value (after the icon, for the status).
	fact := func(i int) *toolkit.HBox {
		h := toolkit.NewHBox()
		h.Spacing = px(gap)
		h.AddFixed(w.keys[i], px(keyW))
		if i == factStatus {
			h.AddFixed(w.statusIcon, px(iconW))
		}
		h.AddFlex(w.values[i], 1)
		return h
	}
	section := func(d *toolkit.VBox, title *toolkit.Label, first, last int) {
		d.AddFixed(title, px(rowH))
		for i := first; i <= last; i++ {
			if visible(w.shown, i) {
				d.AddFixed(fact(i), px(factH))
			}
		}
	}

	// The tenant choice on the selected server, and the button that asks the
	// server for its tenants again.
	tenantRow := toolkit.NewHBox()
	tenantRow.Spacing = px(gap)
	tenantRow.AddFixed(w.tenantLabel, px(keyW))
	tenantRow.AddFlex(w.nTenant, 1)
	tenantRow.AddFixed(w.chooseTenant, px(refreshW))

	head := toolkit.NewHBox()
	head.Spacing = px(gap)
	head.AddFixed(w.serverLogo, px(titleH))
	head.AddFlex(w.serverName, 1)

	removal := toolkit.NewVBox()
	removal.Spacing = px(4)
	removal.AddFixed(w.removeText, px(rowH))
	removal.AddFixed(row(w.confirmRemove, w.confirmRevoke, w.cancelRemove), px(btnH))

	// The detail panel: the selected server, then the session, like WireGuard's.
	detail := toolkit.NewVBox()
	detail.Spacing = px(2)
	detail.AddFixed(head, px(titleH))
	section(detail, w.serverTitle, factURL, factTenant)
	if visible(w.shown, shownTenant) {
		detail.AddFixed(tenantRow, px(btnH))
	}
	section(detail, w.ifaceTitle, factStatus, factLease)
	signIn := toolkit.NewHBox()
	signIn.Spacing = px(gap)
	signedOut := false
	for i, b := range w.signInWith {
		if visible(w.shown, shownProvider+i) {
			signIn.AddFlex(b, 1)
			signedOut = true
		}
	}
	if signedOut {
		detail.AddFixed(signIn, px(btnH))
	}
	detail.AddFixed(card(c.signIn, w.blank[0], device), px(signInCardH))
	detail.AddFixed(row(w.connect, w.disconnect, w.signOut), px(btnH))
	if visible(w.shown, shownNote) {
		detail.AddFixed(w.connectNote, px(rowH))
	}
	detail.AddFixed(card(c.remove, w.blank[1], removal), px(removeCardH))
	detail.AddFlex(w.log, 1)
	strip := toolkit.NewHBox()
	strip.Spacing = px(gap)
	strip.AddFlex(w.blank[2], 1)
	strip.AddFixed(w.edit, px(editW))
	detail.AddFixed(strip, px(editStripH))

	status := toolkit.NewHBox()
	status.Spacing = px(2 * gap)
	status.AddFixed(sidebar, px(sidebarW))
	status.AddFlex(detail, 1)

	root := toolkit.NewVBox()
	root.Spacing = px(gap)
	root.AddFlex(card(c.page, status, serverForm(w, c)), 1)
	root.AddFixed(w.busy, px(rowH))
	root.AddFixed(w.message, px(rowH))
	return toolkit.NewPopoverHost(root)
}

// serverForm is the server window's content: the address to type, then what
// the address said, or the entry being edited.
func serverForm(w *widgets, c *cards) *toolkit.VBox {
	px := toolkit.Scaled
	buttons := func(ws ...toolkit.Widget) *toolkit.HBox {
		h := toolkit.NewHBox()
		h.Spacing = px(gap)
		for _, x := range ws {
			h.AddFlex(x, 1)
		}
		return h
	}
	urlPage := toolkit.NewVBox()
	urlPage.Spacing = px(4)
	urlPage.AddFixed(w.urlLabel, px(rowH))
	urlPage.AddFixed(w.nServer, px(rowH))
	urlPage.AddFixed(buttons(w.next, w.cancel), px(btnH))

	name := toolkit.NewHBox()
	name.Spacing = px(gap)
	name.AddFixed(w.previewLogo, px(titleH))
	name.AddFlex(w.previewName, 1)
	previewPage := toolkit.NewVBox()
	previewPage.Spacing = px(4)
	previewPage.AddFixed(name, px(titleH))
	previewPage.AddFixed(w.previewDetail, px(rowH))
	previewPage.AddFixed(buttons(w.add, w.back, w.cancelPreview), px(btnH))

	editPage := toolkit.NewVBox()
	editPage.Spacing = px(4)
	editPage.AddFixed(w.editURLLabel, px(rowH))
	editPage.AddFixed(w.editURL, px(rowH))
	editPage.AddFixed(w.providerLabel, px(rowH))
	editPage.AddFixed(w.nProvider, px(rowH))
	editPage.AddFixed(buttons(w.done), px(btnH))

	pages := toolkit.NewContainer(c.form)
	pages.AddWidget(urlPage)
	pages.AddWidget(previewPage)
	pages.AddWidget(editPage)

	form := toolkit.NewVBox()
	form.Spacing = px(4)
	form.AddFixed(pages, px(formCardH+rowH))
	form.AddFixed(w.formMsg, px(2*rowH))
	form.AddFlex(w.blank[3], 1)
	return form
}

// settingsLayout is the server window: the form, then the line that says
// what happened to the address.
func settingsLayout(w *widgets, c *cards) toolkit.Widget {
	px := toolkit.Scaled
	root := toolkit.NewVBox()
	root.Spacing = px(gap)
	root.AddFlex(serverForm(w, c), 1)
	root.AddFixed(w.message, px(rowH))
	return toolkit.NewPopoverHost(root)
}
