// SPDX-License-Identifier: BSD-3-Clause
//
// The host: the View as an application.Handler. It owns the framebuffer,
// turns the window's input into toolkit events for the widget tree, and is
// the UI thread's clock -- work posted from other goroutines (the status
// poll, a finished Connect, a tray click) is run at the top of Frame, on the
// thread that draws, which is the only one that touches the ViewModel.

package ui

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-widgets/application"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Queue carries functions to the UI thread. Post is safe from any
// goroutine; Drain runs what was posted, in order, on the caller's.
type Queue struct {
	mu      sync.Mutex
	fns     []func()
	pending atomic.Bool
}

// Post queues f for the UI thread.
func (q *Queue) Post(f func()) {
	q.mu.Lock()
	q.fns = append(q.fns, f)
	q.mu.Unlock()
	q.pending.Store(true)
}

// Pending reports whether anything is queued.
func (q *Queue) Pending() bool { return q.pending.Load() }

// Drain runs everything queued, including what those functions post.
func (q *Queue) Drain() {
	for {
		q.mu.Lock()
		fns := q.fns
		q.fns = nil
		q.pending.Store(false)
		q.mu.Unlock()
		if len(fns) == 0 {
			return
		}
		for _, f := range fns {
			f()
		}
	}
}

// UseOpenTypeText switches the toolkit to its anti-aliased OpenType face,
// for the whole process; on failure the bitmap face stays.
func UseOpenTypeText() error { return toolkit.UseOpenTypeText() }

// View is the window: the bound widgets, laid out, as an
// application.Handler.
type View struct {
	vm    *ViewModel
	queue *Queue
	w     *widgets
	cards *cards
	root  toolkit.Widget
	// build composes the widgets: layout for the main window, settingsLayout
	// for the settings window.
	build func(*widgets, *cards) toolkit.Widget

	theme       *toolkit.Theme
	buf         []byte
	width, high int
	// spare is where the next frame is painted. damageAll says the whole of it
	// is to be reported; damage is the rectangles of the widgets whose bound
	// state changed since DamageRects was last called. It comes from those
	// changes, never from comparing two frames.
	spare     []byte
	damageAll bool
	damage    []toolkit.Rect
	// builtAt is the metric scale the boxes were sized for.
	builtAt float64

	dirty, relayout atomic.Bool
	pressed         bool
	// px, py is where the pointer last was: a wheel scrolls what is under it.
	px, py int
	unbind []func()
}

// NewView binds a View to vm. Its Frame drains queue: give the ViewModel
// WithPost(queue.Post).
func NewView(vm *ViewModel, queue *Queue) *View {
	v := newView(vm, queue, layout)
	// While the settings form has a window of its own, this one keeps showing
	// the status page.
	page := func() {
		if vm.SettingsInWindow.Get() {
			v.cards.page.Active = PageStatus
		} else {
			v.cards.page.Active = vm.Page.Get()
		}
		v.relayout.Store(true)
		v.wholeFrame()
	}
	v.unbind = append(v.unbind,
		vm.Page.SubscribeChanged(page),
		vm.SettingsInWindow.SubscribeChanged(page),
	)
	page()
	return v
}

// NewSettingsView is the settings form alone, for a window of its own (see
// WithSettingsWindow). Its Frame drains queue too: both windows run on the UI
// thread, so whichever draws first runs what was posted.
func NewSettingsView(vm *ViewModel, queue *Queue) *View {
	return newView(vm, queue, settingsLayout)
}

func newView(vm *ViewModel, queue *Queue, build func(*widgets, *cards) toolkit.Widget) *View {
	v := &View{vm: vm, queue: queue, theme: brandTheme(), build: build}
	relayout := func() { v.relayout.Store(true); v.wholeFrame() }
	v.w, v.cards, v.unbind = bindView(vm, v.mark, relayout)
	v.dirty.Store(true)
	return v
}

// Close releases the bindings.
func (v *View) Close() {
	for _, u := range v.unbind {
		u()
	}
	v.unbind = nil
}

// Resize is told the framebuffer's size in device pixels.
func (v *View) Resize(w, h int, _ float64) {
	if w <= 0 || h <= 0 {
		return
	}
	v.width, v.high = w, h
	v.buf = make([]byte, 4*w*h)
	v.wholeFrame()
	v.relayout.Store(true)
	v.dirty.Store(true)
}

// arrange (re)builds the boxes when the scale changed, and gives the tree
// its bounds.
func (v *View) arrange() {
	if s := toolkit.MetricScale(); v.root == nil || s != v.builtAt {
		v.root, v.builtAt = v.build(v.w, v.cards), s
	}
	m := toolkit.Scaled(margin)
	v.root.SetBounds(toolkit.Rect{X: m, Y: m, W: v.width - 2*m, H: v.high - 2*m})
}

// Frame runs what was posted to the UI thread, then draws if anything
// changed.
func (v *View) Frame() ([]byte, int, int, bool) {
	v.queue.Drain()
	if v.buf == nil {
		return nil, 0, 0, false
	}
	if v.relayout.Swap(false) || v.root == nil {
		v.arrange()
	}
	if !v.dirty.Swap(false) {
		return v.buf, v.width, v.high, false
	}
	// Paint into the spare buffer and swap. What is reported as damaged is
	// what the bindings marked (mark), never a comparison of two frames;
	// anything outside those rectangles paints identically, which
	// damage_test.go asserts.
	if len(v.spare) != len(v.buf) {
		v.spare = make([]byte, len(v.buf))
		v.damageAll = true
	}
	bg := v.theme.Background
	for i := 0; i+3 < len(v.spare); i += 4 {
		v.spare[i], v.spare[i+1], v.spare[i+2], v.spare[i+3] = bg.R, bg.G, bg.B, bg.A
	}
	p := painter.NewPixelPainter(v.spare, v.width, v.high)
	v.root.Draw(p, v.theme)
	v.buf, v.spare = v.spare, v.buf
	return v.buf, v.width, v.high, true
}

// maxDamage is how many rectangles are kept before they are merged into their
// union.
const (
	maxDamage = 8
	damagePad = 2
)

// wholeFrame says the next presented frame is to be reported in full.
func (v *View) wholeFrame() { v.damageAll = true; v.dirty.Store(true) }

// mark is told that w's bound content changed: the frame is repainted and w's
// rectangle, padded for anti-aliasing and clipped to the frame, is damaged.
func (v *View) mark(w toolkit.Widget) {
	v.dirty.Store(true)
	if v.damageAll {
		return
	}
	b := w.Bounds()
	r := toolkit.Rect{X: b.X - damagePad, Y: b.Y - damagePad, W: b.W + 2*damagePad, H: b.H + 2*damagePad}
	if r.X < 0 {
		r.W += r.X
		r.X = 0
	}
	if r.Y < 0 {
		r.H += r.Y
		r.Y = 0
	}
	if r.X+r.W > v.width {
		r.W = v.width - r.X
	}
	if r.Y+r.H > v.high {
		r.H = v.high - r.Y
	}
	if r.W <= 0 || r.H <= 0 {
		return
	}
	v.damage = append(v.damage, r)
	if len(v.damage) > maxDamage {
		var u toolkit.Rect
		for _, d := range v.damage {
			u = u.Union(d)
		}
		v.damage = []toolkit.Rect{u}
	}
}

// DamageRects reports the rectangles, in framebuffer pixels, that changed
// since the last call: what the window has to re-present instead of the whole
// frame. It reports nothing (the whole frame) after a resize, and whenever no
// painted frame is pending, so a host that presents for any other reason, an
// expose for one, repaints everything and is never left with stale pixels.
func (v *View) DamageRects() []toolkit.Rect {
	out := v.damage
	if v.damageAll {
		out = nil
	}
	v.damage, v.damageAll = nil, false
	return out
}

// NeedsPresent, PresentImmediate and PresentThrottle let the window skip
// the blit when nothing changed and nothing is queued.
func (v *View) NeedsPresent() bool     { return v.dirty.Load() || v.relayout.Load() || v.queue.Pending() }
func (v *View) PresentImmediate() bool { return v.NeedsPresent() }
func (v *View) PresentThrottle() bool  { return false }

// event hands an input event to the tree, in the root's coordinates.
func (v *View) event(ev toolkit.Event) {
	if v.root == nil {
		return
	}
	r := v.root.Bounds()
	ev.X -= r.X
	ev.Y -= r.Y
	v.root.OnEvent(ev)
	v.wholeFrame()
}

// MouseDown is a click: the toolkit acts on the press.
func (v *View) MouseDown(x, y int) {
	v.pressed, v.px, v.py = true, x, y
	v.event(toolkit.Event{Kind: toolkit.EventClick, X: x, Y: y})
}

// MouseMove is a drag while the button is held (a scrollbar thumb), a hover
// otherwise.
func (v *View) MouseMove(x, y int) {
	v.px, v.py = x, y
	kind := toolkit.EventMouseMove
	if v.pressed {
		kind = toolkit.EventMouseDrag
	}
	v.event(toolkit.Event{Kind: kind, X: x, Y: y})
}

func (v *View) MouseUp(x, y int) {
	v.pressed = false
	v.event(toolkit.Event{Kind: toolkit.EventMouseUp, X: x, Y: y})
}

// wheelPixelsPerRow is application's pixels per wheel notch.
const wheelPixelsPerRow = 40

// Scroll is a wheel delta in device pixels; the toolkit counts rows. The
// application moves the pointer to the wheel's position just before.
func (v *View) Scroll(dy int) {
	rows := dy / wheelPixelsPerRow
	if rows == 0 && dy != 0 {
		rows = 1
		if dy < 0 {
			rows = -1
		}
	}
	v.event(toolkit.Event{Kind: toolkit.EventScroll, X: v.px, Y: v.py, Delta: rows})
}

// keyNames maps the application's key names back to the toolkit's.
var keyNames = map[string]string{"Up": "ArrowUp", "Down": "ArrowDown", "Left": "ArrowLeft", "Right": "ArrowRight"}

// Key is a named key or a typed character.
func (v *View) Key(name string, r rune) {
	if name == "" {
		v.event(toolkit.Event{Kind: toolkit.EventChar, Code: string(r)})
		return
	}
	if k, ok := keyNames[name]; ok {
		name = k
	}
	v.event(toolkit.Event{Kind: toolkit.EventKeyDown, Code: name})
}

// Shortcut is a Ctrl chord: Ctrl+V pastes a client id into a field.
func (v *View) Shortcut(r rune, ctrl, meta bool) {
	if !ctrl && !meta {
		return
	}
	v.event(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+" + strings.ToUpper(string(r)), Ctrl: ctrl, Meta: meta})
}

// SystemAppearance is told the system's light or dark mode. The page is the
// brand's deep teal with white ink in both, so nothing changes.
func (v *View) SystemAppearance(a application.SystemAppearance) { _ = a }

// NativeControls describes the native controls the host embeds: one edit
// control per text input on the page being shown. It runs on the UI thread, as
// do the callbacks it returns.
func (v *View) NativeControls() []toolkit.NativeControl {
	cs := toolkit.WalkNative(v.root)
	for i := range cs {
		if f := cs[i].OnText; f != nil {
			cs[i].OnText = func(s string) { f(s); v.dirty.Store(true) }
		}
		if f := cs[i].OnClaim; f != nil {
			cs[i].OnClaim = func(b bool) { f(b); v.wholeFrame() }
		}
	}
	return cs
}

// A11yElements describes the painted widgets to the platform's accessibility
// layer: without it the window is one unlabelled rectangle and the painted
// buttons (Settings, Sign in, ...) are not in the macOS AX tree at all. The
// native text fields and pop-up are reached by the back-end as real controls.
func (v *View) A11yElements() []application.A11yElement {
	return application.A11yElementsOf(v.root)
}

var (
	_ application.Accessible            = (*View)(nil)
	_ application.NativeControlProvider = (*View)(nil)
	_ application.Handler               = (*View)(nil)
	_ application.ShortcutSink          = (*View)(nil)
	_ application.AppearanceSink        = (*View)(nil)
)
