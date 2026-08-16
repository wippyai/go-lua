package link

import (
	"testing"

	linkproject "github.com/wippyai/go-lua/program/link/project"
)

func TestOwnerCapabilityIsExactAndDetached(t *testing.T) {
	contract := contract(t)
	program := source(t, ``)
	spec := func() *Spec {
		return &Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}}
	}
	left, err := Seal(spec())
	if err != nil {
		t.Fatal(err)
	}
	right, err := Seal(spec())
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() != right.ContentID() || left == right {
		t.Fatal("fixture did not produce equal-content independent Links")
	}
	first := left.OwnerCapability()
	copyOfFirst := first
	second := right.OwnerCapability()
	if !first.Available() || !first.Matches(copyOfFirst) {
		t.Fatal("capability copy lost exact owner identity")
	}
	if first.Matches(second) || first.ContentID() != left.ContentID() {
		t.Fatal("equal-content foreign capability crossed owner fence")
	}
	if (OwnerCapability{}).Available() {
		t.Fatal("zero capability is available")
	}
}
