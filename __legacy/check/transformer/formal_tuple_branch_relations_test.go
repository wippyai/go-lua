package transformer

import (
	"context"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFormalBranchRelationsExecutesCanonicalScalarFactorOnExactFamily(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	x := symbol.ID(101)
	path := pathdom.NewPath(x, "param")
	resolver := visibility.NewResolver(nil)
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	falseValue := typevalue.LiteralBool(reg, false)
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(101), Value: falseValue},
		{Slot: statekey.SymbolValue(102), Value: product.Top()},
		{Slot: statekey.SymbolValue(103), Value: product.Top()},
		{Slot: statekey.SymbolValue(104), Value: product.Top()},
	})

	transaction := func(floor int64) factapply.BranchRelationTransaction {
		rows := factflow.NewBranchRefinementSet().WithNumFloorRefinements(
			factflow.NewBranchNumFloorRefinementOnEdge(path, floor, true),
		)
		facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: rows}})
		return factapply.PlanBranchRelationTransaction(facts, point, true)
	}
	seed := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(x), falseValue)
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(point), seed))

	truthyFacts := factflow.NewFacts(factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceOnEdge(path, true)),
	}})
	truthy := factapply.PlanBranchRelationTransaction(truthyFacts, point, true)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{
			{kind: boundaryStepBranchRelations, branch: transaction(4)},
			{kind: boundaryStepBranchRelations, branch: truthy},
		}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.stepCapability != formalRelationStepCapabilityBranchRelations || equation.Operator.branchRelations == nil {
		t.Fatal("formal BranchRelations capability was not admitted")
	}
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	truthyCell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 2}
	truthyEquation, truthyOK := program.formalTemplate.equation(truthyCell)
	if !truthyOK || truthyEquation.Operator.stepCapability != formalRelationStepCapabilityBranchRelations ||
		truthyEquation.Operator.branchRelations == nil || !execution.values[truthyCell].bottom() {
		t.Fatal("factor-native lexical truthiness did not reject the false-only tuple leaf")
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	before := execution.values[operands.Flow.Source.cell]
	after := execution.values[cell]
	beforeRegions, err := execution.algebra.tupleLeafRegions(before)
	if err != nil || len(beforeRegions) != 1 {
		t.Fatalf("before regions=%d err=%v", len(beforeRegions), err)
	}
	afterRegions, err := execution.algebra.tupleLeafRegions(after)
	if err != nil || len(afterRegions) != 1 {
		t.Fatalf("after regions=%d err=%v", len(afterRegions), err)
	}
	plan := equation.Operator.branchRelations.plans[0].current.coordinates[0]
	prior, err := execution.algebra.materializeFormalBranchCoordinateOperands(beforeRegions[0].evaluator, plan)
	if err != nil {
		t.Fatal(err)
	}
	next, err := execution.algebra.materializeFormalBranchCoordinateOperands(afterRegions[0].evaluator, plan)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := body.productDomain.CoordinateScalarEqual(prior.Scalars[0], next.Scalars[0])
	if err != nil || equal {
		t.Fatalf("formal scalar relation did not change its coordinate: equal=%t err=%v", equal, err)
	}
	sameSkeleton, err := body.productDomain.CoordinateSkeletonRepresentationEqual(prior.Skeleton, next.Skeleton)
	if err != nil {
		t.Fatal(err)
	}
	if !sameSkeleton {
		t.Fatal("scalar-only branch refinement unexpectedly changed its family skeleton")
	}

	span, directory, _, _ := execution.algebra.span(1)
	factorPlan := equation.Operator.branchRelations.plans[0]
	if len(factorPlan.currentProjectionOrdinals) >= span.count ||
		len(factorPlan.originalReadOrdinals) >= span.count {
		t.Fatalf("factor-local BranchRelations retained full-product input: current/original/span=%d/%d/%d",
			len(factorPlan.currentProjectionOrdinals),
			len(factorPlan.originalReadOrdinals), span.count)
	}
	allowed := map[formalFiberOrdinal]struct{}{plan.family.skeleton: {}}
	allowed[plan.family.scalars[plan.positions[0]]] = struct{}{}
	for ordinal := 0; ordinal < span.count; ordinal++ {
		left, leftErr := directory.valueAt(before.root, formalFiberOrdinal(ordinal))
		right, rightErr := directory.valueAt(after.root, formalFiberOrdinal(ordinal))
		if leftErr != nil || rightErr != nil {
			t.Fatalf("fiber %d: %v/%v", ordinal, leftErr, rightErr)
		}
		if left != right {
			if _, owned := allowed[formalFiberOrdinal(ordinal)]; !owned {
				t.Fatalf("BranchRelations changed unrelated formal fiber %d", ordinal)
			}
		}
	}

	containsWrite := func(want formalFiberOrdinal) bool {
		for _, ordinal := range factorPlan.writeOrdinals {
			if ordinal == want {
				return true
			}
		}
		return false
	}
	t.Run("rejection shrinks Care", func(t *testing.T) {
		rejected, applyErr := execution.algebra.applyFormalBranchRelations(truthyEquation.Operator, after)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if !rejected.bottom() {
			t.Fatal("rejected BranchRelations leaf retained Care")
		}
	})
	t.Run("semantic no-op returns exact predecessor root", func(t *testing.T) {
		again, applyErr := execution.algebra.applyFormalBranchRelations(equation.Operator, after)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if again != after {
			t.Fatal("semantic no-op manufactured a new tuple root")
		}
	})
	t.Run("skeleton-writable coordinate retains complete factor spelling", func(t *testing.T) {
		layout, ok := equation.Operator.branchRelations.factors.FactorLayout(0)
		skeletonWrites := layout.CurrentCoordinateSkeletonWrites()
		if !ok || len(skeletonWrites) != 1 || !skeletonWrites[0] {
			t.Fatal("coordinate factor did not declare its skeleton write")
		}
		if !containsWrite(plan.family.skeleton) {
			t.Fatal("skeleton-writable factor omitted its skeleton fiber")
		}
		for _, position := range plan.positions {
			if position < 0 || position >= len(plan.family.scalars) || !containsWrite(plan.family.scalars[position]) {
				t.Fatal("skeleton-writable factor omitted a scalar spelling fiber")
			}
		}
		if len(factorPlan.writeOrdinals) >= span.count {
			t.Fatalf("sparse BranchRelations publication retained full span width %d", span.count)
		}
	})
}

func TestFormalBranchRelationsPathProofDeclaresFamilyReconciliation(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	left, right, siblingSymbol := symbol.ID(101), symbol.ID(102), symbol.ID(103)
	leftPath, rightPath := pathdom.NewPath(left, "param"), pathdom.NewPath(right, "capture")
	siblingPath := pathdom.NewPath(siblingSymbol, "local")
	builder := visibility.NewBuilder()
	builder.Define(point, left, "param")
	builder.Define(point, right, "capture")
	builder.Define(point, siblingSymbol, "local")
	resolver := visibility.NewResolver(builder.Build())
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(left), Value: product.Top()},
		{Slot: statekey.SymbolValue(right), Value: product.Top()},
		{Slot: statekey.SymbolValue(103), Value: product.Top()},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(point), state.Reachable(state.State{})))
	facts := factflow.NewFacts(factflow.FactsInput{BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
		point: factflow.NewBranchPathRelationSet(factflow.NewBranchPathEquality(leftPath, rightPath, true, false)),
	}, BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathPresenceEvidenceOnEdge(siblingPath, presence.Present(), true)),
	}})
	transaction := factapply.PlanBranchRelationTransaction(facts, point, true)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepBranchRelations, branch: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.branchRelations == nil {
		t.Fatal("formal path-equality BranchRelations capability was not admitted")
	}
	var proofCoordinate *formalBranchRelationCoordinatePlan
	var factorPlan *formalBranchRelationFactorPlan
	for index := range equation.Operator.branchRelations.plans {
		if equation.Operator.branchRelations.factors.FactorSource(index) != factapply.BranchRelationStepEvidence {
			continue
		}
		candidate := &equation.Operator.branchRelations.plans[index]
		for coordinateIndex := range candidate.current.coordinates {
			coordinate := &candidate.current.coordinates[coordinateIndex]
			if coordinate.family.family.ID() == "coupled-path-evidence" {
				proofCoordinate, factorPlan = coordinate, candidate
				break
			}
		}
	}
	if proofCoordinate == nil || proofCoordinate.publication != factapply.BranchRelationCoordinatePublicationReconcile {
		t.Fatalf("path-proof publication = %#v, want family reconciliation", proofCoordinate)
	}
	contains := func(values []formalFiberOrdinal, want formalFiberOrdinal) bool {
		for _, value := range values {
			if value == want {
				return true
			}
		}
		return false
	}
	if !contains(factorPlan.currentProjectionOrdinals, proofCoordinate.family.skeleton) ||
		!contains(factorPlan.writeOrdinals, proofCoordinate.family.skeleton) {
		t.Fatal("path-proof factor omitted its family skeleton authority")
	}
	selected := make(map[int]struct{}, len(proofCoordinate.positions))
	for _, position := range proofCoordinate.positions {
		selected[position] = struct{}{}
		ordinal := proofCoordinate.family.scalars[position]
		if !contains(factorPlan.currentProjectionOrdinals, ordinal) || !contains(factorPlan.semanticWriteOrdinals, ordinal) {
			t.Fatalf("path-proof reconciliation omitted selected semantic scalar %d", ordinal)
		}
	}
	for position, ordinal := range proofCoordinate.family.scalars {
		if !contains(factorPlan.writeOrdinals, ordinal) {
			t.Fatalf("path-proof reconciliation omitted physical family scalar %d", ordinal)
		}
		if _, semantic := selected[position]; !semantic && contains(factorPlan.currentProjectionOrdinals, ordinal) {
			t.Fatalf("path-proof preservation scalar %d entered semantic projection", ordinal)
		}
	}
	if len(proofCoordinate.carriers) != len(proofCoordinate.family.scalars)-len(selected) {
		t.Fatalf("path-proof carriers = %d, want %d", len(proofCoordinate.carriers), len(proofCoordinate.family.scalars)-len(selected))
	}

	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil || execution.values[cell].bottom() {
		t.Fatalf("formal path-proof reconciliation = bottom %t, err %v", execution.values[cell].bottom(), err)
	}
	after := execution.values[cell]
	again, err := execution.algebra.applyFormalBranchRelations(equation.Operator, after)
	if err != nil || again != after {
		t.Fatalf("path-proof reconciliation fixed point = same %t, err %v", again == after, err)
	}
}

func TestFormalBranchRelationsReconcileConditionsOnlyChangedPhysicalRoots(t *testing.T) {
	// A Reconcile kernel returns the complete family image. Only the changed
	// scalar may enter DD conditioning/publication; the skeleton and unchanged
	// siblings remain structural references to their predecessor roots.
	ordinals := []formalFiberOrdinal{0, 7, 11, 13, 17}
	positions, err := sealFormalOrdinalPositions(18, ordinals[1:])
	if err != nil {
		t.Fatal(err)
	}
	current := formalSparseLeafView{
		ordinals:  ordinals[1:],
		positions: positions,
		leaves:    []decisionLeaf{2, 3, 4, 5},
	}
	next := formalSparseLeafView{
		ordinals:  ordinals[1:],
		positions: positions,
		leaves:    []decisionLeaf{2, 3, 9, 5},
	}
	writes, err := sparseFormalBranchRelationLeafWrites(nil, current, next, ordinals)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 1 || writes[0] != (formalBranchRelationLeafWrite{ordinal: 13, leaf: 9}) {
		t.Fatalf("Reconcile physical writes = %#v, want only changed scalar", writes)
	}
}
