package product

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/semantic/inventory"
)

func TestCanonicalRegistryInventoryLaw(t *testing.T) {
	in, err := inventory.Base()
	if err != nil {
		t.Fatal(err)
	}
	want := in.ValueAxes()
	reg := CanonicalRegistry()
	if reg == nil || !reg.Frozen() {
		t.Fatal("CanonicalRegistry must return a frozen registry")
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

	fresh, err := RegistryWithCanonicalAxes()
	if err != nil {
		t.Fatalf("RegistryWithCanonicalAxes() error = %v", err)
	}
	if fresh == reg || !fresh.Frozen() {
		t.Fatal("RegistryWithCanonicalAxes must return a distinct frozen registry")
	}
	if got := productRegistryIDs(fresh); !slices.Equal(got, productRegistryIDs(reg)) {
		t.Fatalf("fresh canonical registry axes = %v, singleton axes = %v", got, productRegistryIDs(reg))
	}
	left, err := reg.CanonicalPlan()
	if err != nil {
		t.Fatalf("singleton CanonicalPlan() error = %v", err)
	}
	right, err := fresh.CanonicalPlan()
	if err != nil {
		t.Fatalf("fresh CanonicalPlan() error = %v", err)
	}
	leftAuthority, leftOK := left.AuthorityIdentity()
	rightAuthority, rightOK := right.AuthorityIdentity()
	if left.SchemaIdentity() != right.SchemaIdentity() || !leftOK || !rightOK || leftAuthority != rightAuthority {
		t.Fatal("canonical registry identity changed between equivalent inventory realizations")
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

func productRegistryIDs(reg *axis.Registry) []string {
	view := reg.SpecsView()
	ids := make([]string, view.Len())
	for index := range ids {
		ids[index] = view.At(index).ID()
	}
	return ids
}
