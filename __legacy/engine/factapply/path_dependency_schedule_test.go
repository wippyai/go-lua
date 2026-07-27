package factapply

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestRootAssignmentPathDependenciesSealCoordinatesAndNormalizedValuesAtomically(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(8101)
	target, sourceSymbol := symbol.ID(8101), symbol.ID(8102)
	targetPath, sourcePath := pathdom.NewPath(target, "target"), pathdom.NewPath(sourceSymbol, "source")
	ref := factflow.ExprRef(8101)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: ref, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, source),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: sourcePath},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	builder.Define(point, sourceSymbol, "source")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewRootAssignmentAuthority(NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts, nil, domain)
	transaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok {
		t.Fatal("root assignment transaction missing")
	}
	prepared, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil || !prepared.Valid() {
		t.Fatalf("PrepareResolvedRootAssignmentPlan = valid %t, err %v", prepared.Valid(), err)
	}
	if scalars, ok := prepared.ScalarFactorTransaction(); !ok || !domain.OwnsRootAssignmentScalarFactorTransaction(scalars) {
		t.Fatal("root assignment omitted its frozen scalar factor transaction")
	}
	schedule, err := prepared.PathDependencies(domain, nil)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Len() != 1 {
		t.Fatalf("root path stages = %d, want 1", schedule.Len())
	}
	stage, ok := schedule.Stage(0)
	if !ok || len(stage.IDs()) != 1 {
		t.Fatalf("root path stage = %#v/%t", stage.IDs(), ok)
	}
	plan, ok := schedule.CoordinatePlan()
	if !ok {
		t.Fatal("root path coordinate plan is unsealed")
	}
	dependency, ok := plan.Dependency(stage.IDs()[0])
	if !ok {
		t.Fatal("root path dependency missing")
	}
	if !dependencyHasValueLocation(dependency.LocationReads(), statekey.SymbolValue(sourceSymbol)) {
		t.Fatal("root path dependency omitted normalized source Values read")
	}
	if !dependencyHasValueLocation(dependency.LocationReads(), statekey.SymbolValue(target)) ||
		!dependencyHasValueLocation(dependency.LocationWrites(), statekey.SymbolValue(target)) {
		t.Fatal("root path dependency did not atomically certify normalized target Values read/write")
	}
	if len(dependency.CoordinateWrites()) != 1 {
		t.Fatalf("root coordinate writes = %d, want the equality publication (root Values use their normalized location)", len(dependency.CoordinateWrites()))
	}
}

func TestRootAssignmentPlanRejectsMissingCanonicalTargetIdentity(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(8103)
	target := symbol.ID(8103)
	ref := factflow.ExprRef(8103)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: ref, HasExpr: true}
	assignment := factflow.NewRootAssignmentWithDeclaredContractValue(
		factflow.RootAssignmentLocalDeclaration,
		target,
		pathdom.NewPath(target, "declared"),
		source,
		typevalue.LiteralString(reg, "contract"),
	)
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{point: assignment},
	})
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	authority := NewRootAssignmentAuthority(NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts, nil, domain)
	transaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok {
		t.Fatal("declared-contract root transaction missing")
	}
	plan, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil || !plan.Valid() {
		t.Fatalf("root assignment without visible path sidecar: valid=%v err=%v", plan.Valid(), err)
	}
	if _, ok := plan.TargetPathKey(); ok {
		t.Fatal("empty visible-path sidecar invented a target coordinate")
	}
	factors, ok := plan.FactorPlan()
	if !ok || !domain.OwnsRootAssignmentFactorPlan(factors) {
		t.Fatal("empty visible-path sidecar lost mandatory factor topology")
	}
}

func dependencyHasValueLocation(locations []state.CoordinateDependencyLocation, want statekey.Value) bool {
	for _, location := range locations {
		if concrete, root := location.Root.Concrete(); location.IsRoot() && root && concrete == want {
			return true
		}
	}
	return false
}
