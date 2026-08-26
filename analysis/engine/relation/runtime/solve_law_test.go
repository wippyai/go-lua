package runtime_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// TestSolveRetainsTheExactApplyObservation proves the terminal catalog is
// populated by a real non-Publish Apply schedule entry. The fixture's worker
// returns NoSelection over the empty initial root, so the result is a valid
// zero-row semantic extent rather than a fabricated terminal value.
func TestSolveRetainsTheExactApplyObservation(t *testing.T) {
	fixture := testfixture.New(t, 0xEB)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.Geometry())
	if !ok || !result.Available() {
		t.Fatal("authentic Apply solve was refused")
	}
	if result.Publications() != 0 || !result.Root().Same(fixture.Base()) {
		t.Fatalf("non-Publish Apply mutated the database: publications=%d rootChanged=%t", result.Publications(), !result.Root().Same(fixture.Base()))
	}
	catalog := result.Applications()
	if !catalog.Available() {
		t.Fatal("terminal application catalog unavailable")
	}
	contract, ok := fixture.Mounted().Observation(fixture.TwoScalarApplyObservation())
	if !ok || !contract.Available() || contract.Dependency() != fixture.DependencyTwoScalarApply() {
		t.Fatal("fixture Apply observation contract unavailable")
	}
	entry, ok := fixture.Mounted().Arrangement().Execution().Dependency(contract.Dependency())
	if !ok || !entry.Available() || entry.Node().Kind() != algebra.KindApply {
		t.Fatal("fixture Apply schedule entry unavailable")
	}
	application, ok := catalog.Lookup(contract.Dependency(), contract.Operation())
	if !ok || !application.Available() {
		t.Fatal("exact Apply observation was not retained")
	}
	if application.Dependency() != entry.Dependency() || application.Operation() != contract.Operation() || !application.Root().Same(result.Root()) {
		t.Fatal("terminal observation identity does not match sealed schedule/root")
	}
	extent := application.Result()
	if !extent.Available() || extent.Operation() != contract.Operation() || extent.Len() != 0 {
		t.Fatalf("NoSelection extent was not preserved: available=%t count=%d", extent.Available(), extent.Len())
	}
	if !catalog.CompleteDependency(contract.Dependency()) {
		t.Fatal("declared Apply key was not completed by the authentic evaluation")
	}
	if _, ok := catalog.Lookup(fixture.DependencyLeft(), contract.Operation()); ok {
		t.Fatal("relation-only Input acquired a fabricated terminal application")
	}
}

// Multiple mounted views may redeem one unique Apply occurrence. The
// semantic catalog therefore canonicalizes repeated (dependency, operation)
// declarations instead of storing duplicate result extents.
func TestTerminalCatalogCanonicalizesRepeatedMountedObservation(t *testing.T) {
	fixture := testfixture.New(t, 0xEC)
	contracts := fixture.Mounted().Observations()
	if len(contracts) == 0 {
		t.Fatal("fixture has no mounted observation contract")
	}
	repeated := append(append([]algebra.ObservationContract(nil), contracts...), contracts[0])
	catalog, ok := terminal.NewCatalog(repeated)
	if !ok || !catalog.Available() || catalog.Len() != 0 {
		t.Fatal("repeated mounted observation did not canonicalize")
	}
	contract := contracts[0]
	if !catalog.Declared(contract.Dependency(), contract.Operation()) {
		t.Fatal("canonical catalog lost the mounted composite key")
	}
}

func TestSolveResultRootCountersAndCatalogAreSealed(t *testing.T) {
	var zero terminal.Result
	if zero.Available() || zero.Root().Available() || zero.Evaluations() != 0 || zero.Publications() != 0 {
		t.Fatal("zero runtime result exposed state")
	}
	if zero.Applications().Available() {
		t.Fatal("zero runtime result exposed an application catalog")
	}
	if _, ok := runtime.Solve(witness.Mounted{}, database.Version{}, geometry.Geometry{}); ok {
		t.Fatal("unsealed runtime inputs were accepted")
	}
}

// A root from another mount is foreign even when the caller supplies a valid
// Geometry from the local mount.  Runtime must refuse at admission instead of
// allowing Queue or an evaluator to substitute the requested base later.
func TestSolveRefusesForeignInitialRoot(t *testing.T) {
	fixture := testfixture.New(t, 0xE9)
	foreign := testfixture.New(t, 0xEA)
	if _, ok := runtime.Solve(fixture.Mounted(), foreign.Base(), fixture.Geometry()); ok {
		t.Fatal("foreign initial root was accepted")
	}
}
