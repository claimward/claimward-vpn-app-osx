// SPDX-License-Identifier: BSD-3-Clause

package ui

import (
	"testing"

	"github.com/claimward/claimward-vpn-client/pkg/hproto"
)

// The tenant pop-up reports the title the person picked; the view model maps
// it back to the row the server model counts.
func TestPickingATenantByTitle(t *testing.T) {
	f := signedIn()
	f.offered = []hproto.Tenant{{ID: "chem", Name: "Chemistry"}, {ID: "hpc", Name: "HPC"}}
	vm, q := newVM(t, f)
	run(vm, q, vm.Connect.Execute)

	// A title that is no row is not a choice.
	vm.TenantName.Set("Nothing of the kind")
	q.Drain()
	if len(f.setTenant) != 0 {
		t.Fatalf("an unknown title chose %v", f.setTenant)
	}
	vm.TenantName.Set("HPC (hpc)")
	q.Drain()
	if len(f.setTenant) != 1 || f.setTenant[0] != "hpc" || vm.TenantIndex.Get() != 2 {
		t.Fatalf("SetTenant %v index %d", f.setTenant, vm.TenantIndex.Get())
	}
	// Saying the title of the row already chosen is not choosing again.
	vm.TenantName.Set("")
	vm.TenantName.Set("HPC (hpc)")
	q.Drain()
	if len(f.setTenant) != 1 {
		t.Fatalf("the same row chosen twice: %v", f.setTenant)
	}
}

// A row the model reports that the list does not hold shows as no title.
func TestATenantIndexOutOfRangeShowsNoTitle(t *testing.T) {
	vm, _ := newVM(t, signedIn())
	st := vm.m.State()
	st.TenantIndex = 9
	vm.apply(st)
	if got := vm.TenantName.Get(); got != "" {
		t.Fatalf("title %q for a row past the list", got)
	}
}
