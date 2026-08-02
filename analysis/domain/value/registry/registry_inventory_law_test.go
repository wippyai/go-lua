package registry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/semantic/inventory"
)

func TestRegistryInventoryLaw(t *testing.T) {
	in, err := inventory.Base()
	if err != nil {
		t.Fatal(err)
	}
	want := in.ValueAxes()
	reg := Registry()
	if reg == nil {
		t.Fatal("Registry must return a registry")
	}
	view := reg.SpecsView()
	if view.Len() != len(want) {
		t.Fatalf("canonical registry has %d sparse axes, inventory has %d", view.Len(), len(want))
	}
	for index, entry := range want {
		spec := view.At(index)
		if spec.ID() != entry.ID {
			t.Fatalf("canonical registry axis %d = %q, inventory axis = %q", index, spec.ID(), entry.ID)
		}
		if got := spec.BoundaryPolicy(); got != inventoryBoundaryPolicy(t, entry.Boundary) {
			t.Fatalf("canonical registry axis %q boundary = %d, inventory boundary %q maps to %d", spec.ID(), got, entry.Boundary, inventoryBoundaryPolicy(t, entry.Boundary))
		}
	}

	plan, err := reg.CanonicalPlan()
	if err != nil {
		t.Fatalf("Registry CanonicalPlan() error = %v", err)
	}
	if !plan.InventorySealed() {
		t.Fatal("Registry must publish its sealed inventory authority")
	}
	authority, ok := plan.AuthorityIdentity()
	if !ok || authority != plan.SchemaIdentity() {
		t.Fatal("Registry must publish authority for its complete canonical inventory")
	}
}

func inventoryBoundaryPolicy(t testing.TB, boundary string) axis.BoundaryPolicy {
	t.Helper()
	switch boundary {
	case "local-only":
		return axis.LocalOnly
	case "portable-identity":
		return axis.PortableIdentity
	case "projected":
		return axis.Projected
	default:
		t.Fatalf("unknown inventory boundary %q", boundary)
		return axis.BoundaryUnspecified
	}
}
