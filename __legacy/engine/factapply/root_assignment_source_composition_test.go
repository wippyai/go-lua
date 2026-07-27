package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestRootAssignmentSourceCompositionLaws(t *testing.T) {
	reg := standard.Registry()
	optionalString := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	declared := typevalue.FromType(reg, typ.String)

	t.Run("ordinary unresolved is unproductive", func(t *testing.T) {
		if _, productive := ComposeRootAssignmentSourceValue(reg, product.Value{}, false, RootAssignmentSourceComposition{}); productive {
			t.Fatal("ordinary unresolved source became productive")
		}
	})

	t.Run("executed source cell remains productive at scalar bottom", func(t *testing.T) {
		got, productive := ComposeRootAssignmentSourceValue(reg, product.Bottom(reg), false, RootAssignmentSourceComposition{SourceCellExecutes: true})
		if !productive || !product.Equal(reg, got, product.Bottom(reg)) {
			t.Fatal("executed source cell lost structural assignment authority at scalar bottom")
		}
	})

	t.Run("point proof removes nil before factor write", func(t *testing.T) {
		got, productive := ComposeRootAssignmentSourceValue(reg, optionalString, true, RootAssignmentSourceComposition{DefinitelyPresent: true})
		if !productive || !presence.Equal(product.PresenceOf(got), presence.Present()) {
			t.Fatalf("present composition = productive:%v presence:%v", productive, product.PresenceOf(got))
		}
		gotType, ok := typevalue.TypeOf(reg, got)
		if !ok || !gotType.Equals(typ.String) {
			t.Fatalf("present composition type = %v/%v, want string", gotType, ok)
		}
	})

	t.Run("declared contract remains lazy", func(t *testing.T) {
		got, productive := ComposeRootAssignmentSourceValue(reg, product.Value{}, false, RootAssignmentSourceComposition{
			Declared: declared, DeclaredMode: RootAssignmentDeclaredContract, HasDeclared: true,
		})
		if !productive || !product.Equal(reg, got, declared) {
			t.Fatal("declared contract consulted an absent runtime source")
		}
	})

	t.Run("declared overlay uses canonical merge", func(t *testing.T) {
		got, productive := ComposeRootAssignmentSourceValue(reg, optionalString, true, RootAssignmentSourceComposition{
			Declared: declared, DeclaredMode: RootAssignmentDeclaredOverlay, HasDeclared: true,
		})
		want, ok := ComposeRootAssignmentDeclaredValue(reg, optionalString, declared, RootAssignmentDeclaredOverlay, false)
		if !productive || !ok || !product.Equal(reg, got, want) {
			t.Fatal("source composition diverged from declared overlay law")
		}
	})
}

func TestResolvedRootAssignmentPlanComposesFromExactPointProof(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(760)
	sourceSymbol, targetSymbol := symbol.ID(760), symbol.ID(761)
	sourcePath, targetPath := pathdom.NewPath(sourceSymbol, "source"), pathdom.NewPath(targetSymbol, "target")
	builder := visibility.NewBuilder()
	builder.Define(point, sourceSymbol, "source")
	builder.Define(point, targetSymbol, "target")
	resolver := visibility.NewResolver(builder.Build())
	sourceState, ok := visibility.AddressAt(resolver, point, sourcePath).VisibleStateKey()
	if !ok {
		t.Fatal("source state key")
	}
	sourceKey, ok := visibility.KeyspaceKeyFromStateKey(resolver, sourceState)
	if !ok {
		t.Fatal("source key")
	}
	source := factflow.ValueSource{Kind: factflow.ValueSourcePath, PathKey: sourceState.PathKey()}
	optionalString := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	facts := factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, targetSymbol, targetPath, source),
	}})
	authority := NewRootAssignmentAuthority(
		NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts, nil, state.RegisteredProductDomain(reg),
	)
	transaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok {
		t.Fatal("root-assignment transaction missing")
	}
	plan, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil {
		t.Fatal(err)
	}
	proof, present := plan.SourcePresenceProof()
	if !present || proof.Path != sourceKey {
		t.Fatalf("source presence proof = %#v/%t, want exact current key %v", proof, present, sourceKey)
	}
	got, productive, err := plan.ComposeFactorPrimarySourceValue(reg, optionalString, true)
	want, wantProductive := ComposeRootAssignmentSourceValue(reg, optionalString, true, RootAssignmentSourceComposition{DefinitelyPresent: true})
	if err != nil || !productive || !wantProductive || !product.Equal(reg, got, want) {
		t.Fatalf("factor source composition diverged from shared law: productive=%t/%t err=%v", productive, wantProductive, err)
	}
}

func TestResolvedRootAssignmentPlanDoesNotPublishReflexivePathEquality(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(761)
	target := symbol.ID(761)
	targetPath := pathdom.NewPath(target, "target")
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	stateKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		t.Fatal("target state key")
	}
	source := factflow.ValueSource{Kind: factflow.ValueSourcePath, PathKey: stateKey.PathKey()}
	facts := factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, source),
	}})
	authority := NewRootAssignmentAuthority(NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts, nil, state.RegisteredProductDomain(reg))
	transaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok {
		t.Fatal("root-assignment transaction missing")
	}
	plan, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil {
		t.Fatal(err)
	}
	if proof, published := plan.PublishedEqualityProof(); published || proof != (pathevidence.BranchProof{}) {
		t.Fatalf("reflexive equality = %#v/%t, want no quotient publication", proof, published)
	}
}

func TestResolvedRootAssignmentPlanPresenceProofUsesExactVisibleVersion(t *testing.T) {
	sourceSymbol := symbol.ID(762)
	sourcePath := pathdom.NewPath(sourceSymbol, "source")
	oldPoint, currentPoint := cfg.Point(762), cfg.Point(763)
	builder := visibility.NewBuilder()
	builder.Define(oldPoint, sourceSymbol, "source")
	builder.Define(currentPoint, sourceSymbol, "source")
	resolver := visibility.NewResolver(builder.Build())
	oldKey, oldOK := visibility.AddressAt(resolver, oldPoint, sourcePath).VisibleKeyspaceKey()
	currentKey, currentOK := visibility.AddressAt(resolver, currentPoint, sourcePath).VisibleKeyspaceKey()
	currentState, stateOK := visibility.AddressAt(resolver, currentPoint, sourcePath).VisibleStateKey()
	if !oldOK || !currentOK || !stateOK || oldKey == currentKey {
		t.Fatal("expected two distinct visible SSA versions")
	}
	source := factflow.ValueSource{Kind: factflow.ValueSourcePath, PathKey: currentState.PathKey()}
	target := symbol.ID(764)
	targetPath := pathdom.NewPath(target, "target")
	builder.Define(currentPoint, target, "target")
	resolver = visibility.NewResolver(builder.Build())
	oldKey, oldOK = visibility.AddressAt(resolver, oldPoint, sourcePath).VisibleKeyspaceKey()
	currentKey, currentOK = visibility.AddressAt(resolver, currentPoint, sourcePath).VisibleKeyspaceKey()
	if !oldOK || !currentOK || oldKey == currentKey {
		t.Fatal("rebuilt resolver lost distinct source versions")
	}
	facts := factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		currentPoint: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, source),
	}})
	authority := NewRootAssignmentAuthority(
		NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts, nil, state.RegisteredProductDomain(standard.Registry()),
	)
	transaction, ok := PlanRootAssignmentTransaction(facts, currentPoint)
	if !ok {
		t.Fatal("root-assignment transaction missing")
	}
	plan, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil {
		t.Fatal(err)
	}
	proof, present := plan.SourcePresenceProof()
	if !present || proof.Path != currentKey || proof.Path == oldKey {
		t.Fatalf("source presence proof path = %v/%t, want current %v and not old %v", proof.Path, present, currentKey, oldKey)
	}
}
