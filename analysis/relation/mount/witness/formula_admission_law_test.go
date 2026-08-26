package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

func TestMountedAdmitsFormulaOnlyRuntimeScopes(t *testing.T) {
	value := newSemanticAdmissionFixture(t)
	mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner))
	if !ok || !mounted.Available() {
		t.Fatal("semantic mount refused")
	}

	formula := content(t, "physical-formula")
	first, firstOK := mounted.AdmitRuntimeFormula(formula)
	repeated, repeatedOK := mounted.AdmitRuntimeFormula(formula)
	if !firstOK || !repeatedOK || !first.Available() || !repeated.Available() || !first.Same(repeated) {
		t.Fatal("same physical formula did not recover one runtime scope")
	}

	other, otherOK := mounted.AdmitRuntimeFormula(content(t, "other-physical-formula"))
	if !otherOK || !other.Available() || first.Same(other) {
		t.Fatal("different physical formulas shared one runtime scope")
	}
	if scope, scopeOK := mounted.AdmitRuntimeFormula(identity.ContentID{}); scopeOK || scope.Available() {
		t.Fatal("zero physical formula identity was admitted")
	}
	if _, regionOK := mounted.RegionForScope(first); regionOK {
		t.Fatal("formula-only scope resolved to a neutral Region")
	}

	denominator, denominatorOK := mounted.Denominator(value.denominator)
	if !denominatorOK {
		t.Fatal("denominator witness missing")
	}
	cell, cellOK := mounted.IssueCell(denominator, first, value.column, value.row)
	if !cellOK || !cell.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("authenticated formula-only scope could not issue a cell")
	}
	if _, tokenOK := mounted.ScopeToken(first); !tokenOK {
		t.Fatal("formula-only scope could not project its authenticated token")
	}
	if !cell.Scope().ValidFor(mounted.RuntimeFence()) {
		t.Fatal("formula-only cell carried an invalid scope token")
	}
}

func TestMountedFormulaOnlyAdmissionCannotBeUpgradedToRegion(t *testing.T) {
	value := newSemanticAdmissionFixture(t)
	mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner))
	if !ok || !mounted.Available() {
		t.Fatal("semantic mount refused")
	}

	physicalRegion := finite(t, "formula-only-upgrade")
	formula := physicalRegion.Identity()
	if !formula.Available() {
		t.Fatal("physical region identity unavailable")
	}
	scope, scopeOK := mounted.AdmitRuntimeFormula(formula)
	if !scopeOK || !scope.Available() {
		t.Fatal("formula-only scope admission refused")
	}
	if upgraded, upgradedOK := mounted.AdmitRuntimeRegion(physicalRegion); upgradedOK || upgraded.Available() {
		t.Fatal("formula-only scope was upgraded to a neutral Region")
	}
	if _, regionOK := mounted.RegionForScope(scope); regionOK {
		t.Fatal("formula-only scope acquired a neutral Region")
	}
}
