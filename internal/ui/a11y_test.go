// SPDX-License-Identifier: BSD-3-Clause

package ui

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

// Every painted control that a person can act on, and every picture, is
// described to the platform's accessibility layer with a name.
func TestEveryControlAndPictureIsNamed(t *testing.T) {
	v, _ := newTestView(t, twoServers())
	names := map[string]string{}
	for _, e := range v.A11yElements() {
		switch e.Role {
		case string(toolkit.RoleButton), string(toolkit.RoleImg):
			if e.Name == "" {
				t.Errorf("a %s at %d,%d has no name", e.Role, e.X, e.Y)
			}
		}
		names[e.Name] = e.Role
	}
	for name, role := range map[string]string{
		"Connect": string(toolkit.RoleButton), "Disconnect": string(toolkit.RoleButton),
		"Sign out": string(toolkit.RoleButton), "Edit": string(toolkit.RoleButton),
		editServerName: string(toolkit.RoleButton),
		"Server logo":  string(toolkit.RoleImg), "Not connected": string(toolkit.RoleImg),
	} {
		if got, ok := names[name]; !ok || got != role {
			t.Errorf("%q: role %q (present %v), want %q", name, got, ok, role)
		}
	}
}

func TestTheStatusIconSaysTheStatus(t *testing.T) {
	kind := StatusIdle
	s := newStatusIcon(func() int { return kind })
	for k, want := range map[int]string{StatusIdle: "Not connected", StatusUp: "Connected", StatusProblem: "Problem"} {
		kind = k
		if got := s.A11y(); got.Name != want || got.Role != toolkit.RoleImg {
			t.Errorf("kind %d: %+v, want %q", k, got, want)
		}
	}
}

func TestAccessibilityTreeBeforeTheFirstLayoutIsEmpty(t *testing.T) {
	v := NewView(NewViewModel(signedOut()), &Queue{})
	t.Cleanup(v.Close)
	if es := v.A11yElements(); es != nil {
		t.Fatalf("elements before any layout: %v", es)
	}
}
