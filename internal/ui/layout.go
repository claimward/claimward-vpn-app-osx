// SPDX-License-Identifier: BSD-3-Clause
//
// The window's boxes. Every text is laid out to WRAP: a label's or a button's
// height is what its text needs at the width its box has (HeightFor), never a
// fixed number of rows that a long URL, identity or refusal would run out of.
// WireGuard's own rows truncate (KeyValueRow.swift); this app deliberately does
// not.

package ui

import (
	"strconv"
	"strings"

	"github.com/go-widgets/toolkit"
)

// stack is a VBox that adds up the height its fixed rows take.
type stack struct {
	*toolkit.VBox
	sum, n, spacing int
}

func newStack(spacing int) *stack {
	s := &stack{VBox: toolkit.NewVBox(), spacing: spacing}
	s.Spacing = spacing
	return s
}

// add puts x in a row h tall.
func (s *stack) add(x toolkit.Widget, h int) {
	s.AddFixed(x, h)
	s.sum += h
	s.n++
}

// height is what the rows added so far take, spacing included.
func (s *stack) height() int {
	return s.sum + max(s.n-1, 0)*s.spacing
}

// rowFor is the height of a label's row at width: a row, or the lines its text wraps to.
func rowFor(l *toolkit.Label, width int) int {
	return max(toolkit.Scaled(rowH), l.HeightFor(max(width, 1)))
}

// buttonRow lays buttons side by side over width, each as wide as the others, in
// a row as tall as the tallest wrapped caption needs.
func buttonRow(width int, bs ...*toolkit.Button) (*toolkit.HBox, int) {
	px := toolkit.Scaled
	h := toolkit.NewHBox()
	h.Spacing = px(gap)
	each := max((width-(len(bs)-1)*px(gap))/max(len(bs), 1), 1)
	tall := px(btnH)
	for _, b := range bs {
		h.AddFlex(b, 1)
		tall = max(tall, b.HeightFor(each))
	}
	return h, tall
}

// detailWidth is the width of the detail panel's content inside a window total
// wide: what the sidebar and the gap leave, less the scroll bar's gutter.
func detailWidth(total int) int {
	return max(total-toolkit.Scaled(sidebarW)-2*toolkit.Scaled(gap)-toolkit.ScrollGutter(), 1)
}

// logWidth is the room for a log line's text in the log below the detail panel.
func logWidth(total int) int {
	return max(detailWidth(total)-toolkit.Scaled(28), 1)
}

// textBlock is the height of n lines of the toolkit's text.
func textBlock(n int) int {
	if n <= 0 {
		return 0
	}
	f := toolkit.CurrentFont()
	return n*f.Height() + (n-1)*toolkit.Scaled(toolkit.CardLineSpacing)
}

// cardHeight is the room a card takes: none while another page than shown is
// active, h while it shows.
func cardHeight(active, shown, h int) int {
	if active != shown {
		return 0
	}
	return h
}

// anyShown says whether any fact from first to last is in the shown mask.
func anyShown(mask, first, last int) bool {
	for i := first; i <= last; i++ {
		if visible(mask, i) {
			return true
		}
	}
	return false
}

// maxRowLines is the most lines a server row grows to hold.
const maxRowLines = 4

// column is the detail panel: a scroll view whose content is as tall as the
// layout measured, so a panel taller than the window scrolls and no row is cut.
type column struct {
	*toolkit.ScrollView
	height int
}

// SetBounds places the viewport, and the content in it at the measured height.
func (c *column) SetBounds(r toolkit.Rect) {
	c.ScrollView.SetBounds(r)
	w, h := max(r.W-toolkit.ScrollGutter(), 1), max(c.height, r.H)
	c.Child.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: w, H: h})
	c.SetContentSize(w, h)
}

// serverRowHeight is the height of a server row that holds its wrapped name and
// state beside a logo as tall as the row (what toolkit.LogoRows draws).
func serverRowHeight(items []string, listW int) int {
	f := toolkit.CurrentFont()
	pad := toolkit.Scaled(4)
	h := toolkit.Scaled(serverRowH)
	for range 4 {
		side := h - 2*pad
		textW := max(listW-toolkit.Scaled(14)-3*pad-side, 1)
		lines := 2
		for _, it := range items {
			name, state := splitRow(it)
			lines = max(lines, len(toolkit.WrapLines(f, name, textW))+len(toolkit.WrapLines(f, state, textW)))
		}
		// Every row of the list is this tall: past four lines the name ends in "…".
		lines = min(lines, maxRowLines)
		need := max(h, textBlock(lines)+2*toolkit.Scaled(2))
		if need == h {
			break
		}
		h = need
	}
	return h
}

// splitRow undoes rowText.
func splitRow(item string) (name, state string) {
	if i := strings.LastIndex(item, ", "); i >= 0 {
		return item[:i], item[i+2:]
	}
	return item, ""
}

// wrapped are the labels whose text wraps to their box.
func (w *widgets) wrapped() []*toolkit.Label {
	ls := []*toolkit.Label{
		w.busy, w.message, w.deviceHint, w.deviceCode, w.deviceURI, w.serverName,
		w.connectNote, w.removeText, w.formMsg, w.previewName, w.previewDetail,
		w.editURL, w.urlLabel, w.editURLLabel, w.providerLabel, w.tenantLabel,
	}
	ls = append(ls, w.keys[:]...)
	return append(ls, w.values[:]...)
}

// buttons are every button of the window.
func (w *widgets) buttons() []*toolkit.Button {
	bs := []*toolkit.Button{
		w.openPage, w.cancelSignIn, w.signOut, w.connect, w.disconnect, w.chooseTenant,
		w.edit, w.confirmRemove, w.confirmRevoke, w.cancelRemove, w.next, w.cancel,
		w.add, w.back, w.cancelPreview, w.done,
	}
	return append(bs, w.signInWith[:]...)
}

// wrapKey is everything the heights depend on: when it changes, the boxes are
// laid out again.
func (w *widgets) wrapKey(total int) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(total))
	b.WriteString(strconv.FormatFloat(toolkit.MetricScale(), 'g', -1, 64))
	for _, l := range w.wrapped() {
		b.WriteByte(0)
		b.WriteString(l.Text().Get())
	}
	for _, x := range w.buttons() {
		b.WriteByte(0)
		b.WriteString(x.Label().Get())
	}
	for _, it := range w.servers.Items {
		b.WriteByte(0)
		b.WriteString(it)
	}
	return b.String()
}

// layout composes the widgets into boxes sized for the current metric scale,
// for what the panel shows and for the text each box has to hold, in a window
// total device pixels wide. It is rebuilt when any of those change, never per
// frame; the widgets and their bindings are untouched by it.
func layout(w *widgets, c *cards, total int) toolkit.Widget {
	px := toolkit.Scaled
	detailW := detailWidth(total)
	card := func(l *toolkit.CardLayout, pages ...toolkit.Widget) *toolkit.Container {
		ct := toolkit.NewContainer(l)
		for _, p := range pages {
			ct.AddWidget(p)
		}
		return ct
	}

	device := newStack(px(4))
	device.add(w.deviceHint, rowFor(w.deviceHint, detailW))
	device.add(w.deviceCode, rowFor(w.deviceCode, detailW))
	device.add(w.deviceURI, rowFor(w.deviceURI, detailW))
	deviceButtons, h := buttonRow(detailW, w.openPage, w.cancelSignIn)
	device.add(deviceButtons, h)

	// The sidebar: the servers, framed over the bar of their + and − buttons.
	w.servers.RowHeight = serverRowHeight(w.servers.Items, px(sidebarW))
	sidebar := toolkit.NewVBox()
	sidebar.Spacing = px(4)
	sidebar.AddFixed(w.serversTitle, px(rowH))
	sidebar.AddFlex(w.serverPane, 1)

	detail := newStack(px(2))
	// A fact: its key at the left, its value (after the icon, for the status).
	fact := func(i int) {
		h := toolkit.NewHBox()
		h.Spacing = px(gap)
		valueW := detailW - px(keyW) - px(gap)
		h.AddFixed(w.keys[i], px(keyW))
		if i == factStatus {
			h.AddFixed(w.statusIcon, px(iconW))
			valueW -= px(iconW) + px(gap)
		}
		h.AddFlex(w.values[i], 1)
		detail.add(h, max(px(factH), w.keys[i].HeightFor(px(keyW)), w.values[i].HeightFor(max(valueW, 1))))
	}
	section := func(title *toolkit.Label, first, last int) {
		if !anyShown(w.shown, first, last) {
			return // a section with nothing to show has no title either
		}
		detail.add(title, px(rowH))
		for i := first; i <= last; i++ {
			if visible(w.shown, i) {
				fact(i)
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

	removal := newStack(px(4))
	removal.add(w.removeText, rowFor(w.removeText, detailW))
	removalButtons, h := buttonRow(detailW, w.confirmRemove, w.confirmRevoke, w.cancelRemove)
	removal.add(removalButtons, h)

	// The detail panel: the selected server, then the session, like WireGuard's.
	detail.add(head, max(px(titleH), w.serverName.HeightFor(max(detailW-px(titleH)-px(gap), 1))))
	section(w.serverTitle, factURL, factTenant)
	if visible(w.shown, shownTenant) {
		detail.add(tenantRow, max(px(btnH), w.chooseTenant.HeightFor(px(refreshW))))
	}
	section(w.ifaceTitle, factStatus, factLease)
	section(w.peerTitle, factEndpoint, factTransfer)
	var providers []*toolkit.Button
	for i, b := range w.signInWith {
		if visible(w.shown, shownProvider+i) {
			providers = append(providers, b)
		}
	}
	if len(providers) > 0 {
		signIn, h := buttonRow(detailW, providers...)
		detail.add(signIn, h)
	}
	detail.add(card(c.signIn, w.blank[0], device.VBox), cardHeight(c.signIn.Active, PanelShown, max(px(signInCardH), device.height())))
	session, h := buttonRow(detailW, w.connect, w.disconnect, w.signOut)
	detail.add(session, h)
	if visible(w.shown, shownNote) {
		detail.add(w.connectNote, rowFor(w.connectNote, detailW))
	}
	detail.add(card(c.remove, w.blank[1], removal.VBox), cardHeight(c.remove.Active, PanelShown, max(px(removeCardH), removal.height())))
	detail.add(w.log, px(logH))
	// The panel scrolls when its rows are taller than the window; the Edit
	// button stays under it.
	scroll := &column{ScrollView: toolkit.NewScrollView(detail.VBox), height: detail.height()}
	strip := toolkit.NewHBox()
	strip.Spacing = px(gap)
	strip.AddFlex(w.blank[2], 1)
	strip.AddFixed(w.edit, px(editW))
	right := newStack(px(gap))
	right.AddFlex(scroll, 1)
	right.add(strip, max(px(editStripH), w.edit.HeightFor(px(editW))))

	status := toolkit.NewHBox()
	status.Spacing = px(2 * gap)
	status.AddFixed(sidebar, px(sidebarW))
	status.AddFlex(right.VBox, 1)

	root := newStack(px(gap))
	root.AddFlex(card(c.page, status, serverForm(w, c, total)), 1)
	root.add(w.busy, rowFor(w.busy, total))
	root.add(w.message, rowFor(w.message, total))
	return toolkit.NewPopoverHost(root.VBox)
}

// serverForm is the server window's content: the address to type, then what
// the address said, or the entry being edited.
func serverForm(w *widgets, c *cards, total int) *toolkit.VBox {
	px := toolkit.Scaled
	nameW := max(total-px(titleH)-px(gap), 1)

	urlPage := newStack(px(4))
	urlPage.add(w.urlLabel, rowFor(w.urlLabel, total))
	urlPage.add(w.nServer, px(rowH))
	urlButtons, h := buttonRow(total, w.next, w.cancel)
	urlPage.add(urlButtons, h)

	name := toolkit.NewHBox()
	name.Spacing = px(gap)
	name.AddFixed(w.previewLogo, px(titleH))
	name.AddFlex(w.previewName, 1)
	previewPage := newStack(px(4))
	previewPage.add(name, max(px(titleH), w.previewName.HeightFor(nameW)))
	previewPage.add(w.previewDetail, rowFor(w.previewDetail, total))
	previewButtons, h := buttonRow(total, w.add, w.back, w.cancelPreview)
	previewPage.add(previewButtons, h)

	editPage := newStack(px(4))
	editPage.add(w.editURLLabel, rowFor(w.editURLLabel, total))
	editPage.add(w.editURL, rowFor(w.editURL, total))
	editPage.add(w.providerLabel, rowFor(w.providerLabel, total))
	editPage.add(w.nProvider, px(rowH))
	editButtons, h := buttonRow(total, w.done)
	editPage.add(editButtons, h)

	pages := toolkit.NewContainer(c.form)
	pages.AddWidget(urlPage.VBox)
	pages.AddWidget(previewPage.VBox)
	pages.AddWidget(editPage.VBox)

	form := newStack(px(4))
	form.add(pages, max(px(formCardH+rowH), urlPage.height(), previewPage.height(), editPage.height()))
	form.add(w.formMsg, max(px(2*rowH), w.formMsg.HeightFor(max(total, 1))))
	form.AddFlex(w.blank[3], 1)
	return form.VBox
}

// settingsLayout is the server window: the form, then the line that says
// what happened to the address.
func settingsLayout(w *widgets, c *cards, total int) toolkit.Widget {
	px := toolkit.Scaled
	root := newStack(px(gap))
	root.AddFlex(serverForm(w, c, total), 1)
	root.add(w.message, rowFor(w.message, total))
	return toolkit.NewPopoverHost(root.VBox)
}
