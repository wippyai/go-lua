package factapply

import (
	"context"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestBranchFactorSchedulerHonorsTransitiveAndFamilyBundleDependencies(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	seal := new(branchProgramSeal)
	x, y := statekey.SymbolValue(9101), statekey.SymbolValue(9102)
	atoms := []branchAtom{
		{seal: seal, access: branchAtomAccess{valueWrites: []statekey.Value{x}}},
		{seal: seal, access: branchAtomAccess{valueReads: []statekey.Value{x}, valueWrites: []statekey.Value{y}}},
		{seal: seal, access: branchAtomAccess{valueReads: []statekey.Value{y}}},
	}
	stages, err := scheduleBranchAtoms(domain, atoms, state.CoordinateDependencyPlan{}, seal)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 3 {
		t.Fatalf("transitive stages = %d, want 3", len(stages))
	}

	keys := keyspace.New()
	slot, err := domain.PathBranchProofCoordinateSlot(keys, pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathPresence,
		Path: keys.FromPath(pathdom.NewPath(symbol.ID(9103), "p")), Presence: presence.Present(),
	})
	if err != nil {
		t.Fatal(err)
	}
	family, ok := domain.PathValueFamily()
	if !ok {
		t.Fatal("path family missing")
	}
	if !branchAtomAccessConflict(domain,
		branchAtomAccess{laneWrites: []state.ProductLane{family.Lane()}},
		branchAtomAccess{coordinateWrites: []state.CoordinateSlot{slot}},
	) {
		t.Fatal("whole-family write did not conflict with family scalar write")
	}
}

func TestBranchFactorSchedulerTreatsCoordinateFamilyOwnershipAsAConflict(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	slot := func(id symbol.ID, field string) state.CoordinateSlot {
		t.Helper()
		coordinate, err := domain.PathBranchProofCoordinateSlot(keys, pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     keys.FromPath(pathdom.NewPath(id, field)),
			Presence: presence.Present(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return coordinate
	}
	leftSlot := slot(symbol.ID(9111), "left")
	rightSlot := slot(symbol.ID(9112), "right")
	equal, err := domain.CoordinateSlotEqual(leftSlot, rightSlot)
	if err != nil {
		t.Fatal(err)
	}
	if equal || leftSlot.Family() != rightSlot.Family() {
		t.Fatal("test coordinates do not form two distinct slots in one family")
	}
	family := leftSlot.Family()

	tests := []struct {
		name        string
		left, right branchAtomAccess
		conflict    bool
	}{
		{name: "family-write-slot-read", left: branchAtomAccess{coordinateFamilyWrites: []state.CoordinateFamily{family}}, right: branchAtomAccess{coordinateReads: []state.CoordinateSlot{leftSlot}}, conflict: true},
		{name: "family-write-slot-write", left: branchAtomAccess{coordinateFamilyWrites: []state.CoordinateFamily{family}}, right: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{leftSlot}}, conflict: true},
		{name: "family-write-family-read", left: branchAtomAccess{coordinateFamilyWrites: []state.CoordinateFamily{family}}, right: branchAtomAccess{coordinateFamilyReads: []state.CoordinateFamily{family}}, conflict: true},
		{name: "family-write-family-write", left: branchAtomAccess{coordinateFamilyWrites: []state.CoordinateFamily{family}}, right: branchAtomAccess{coordinateFamilyWrites: []state.CoordinateFamily{family}}, conflict: true},
		{name: "family-read-slot-write", left: branchAtomAccess{coordinateFamilyReads: []state.CoordinateFamily{family}}, right: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{leftSlot}}, conflict: true},
		{name: "slot-write-family-read", left: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{leftSlot}}, right: branchAtomAccess{coordinateFamilyReads: []state.CoordinateFamily{family}}, conflict: true},
		{name: "family-read-slot-read", left: branchAtomAccess{coordinateFamilyReads: []state.CoordinateFamily{family}}, right: branchAtomAccess{coordinateReads: []state.CoordinateSlot{leftSlot}}},
		{name: "disjoint-patch-writes", left: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{leftSlot}}, right: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{rightSlot}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := branchAtomAccessConflict(domain, test.left, test.right); got != test.conflict {
				t.Fatalf("conflict = %t, want %t", got, test.conflict)
			}
			if got := branchAtomAccessConflict(domain, test.right, test.left); got != test.conflict {
				t.Fatalf("reverse conflict = %t, want %t", got, test.conflict)
			}
		})
	}

	seal := new(branchProgramSeal)
	stages, err := scheduleBranchAtoms(domain, []branchAtom{
		{seal: seal, access: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{leftSlot}}},
		{seal: seal, access: branchAtomAccess{coordinateWrites: []state.CoordinateSlot{rightSlot}}},
	}, state.CoordinateDependencyPlan{}, seal)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 || len(stages[0].atoms) != 2 {
		t.Fatalf("disjoint Patch writes scheduled as %#v, want one parallel stage", stages)
	}

	stages, err = scheduleBranchAtoms(domain, []branchAtom{
		{seal: seal, access: branchAtomAccess{coordinateFamilyWrites: []state.CoordinateFamily{family}}},
		{seal: seal, access: branchAtomAccess{coordinateReads: []state.CoordinateSlot{rightSlot}}},
	}, state.CoordinateDependencyPlan{}, seal)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 2 {
		t.Fatalf("family owner and scalar reader stages = %d, want 2", len(stages))
	}
}

func TestBranchConsequenceBarrierNormalizationHoistsOnlyOriginalReadAtoms(t *testing.T) {
	write := branchAtomDraft{apply: branchIdentityKernel, access: branchAtomAccess{
		valueWrites: []statekey.Value{statekey.SymbolValue(1)},
	}}
	original := branchAtomDraft{apply: branchIdentityKernel, original: true, careActivation: true}
	barrier := branchAtomDraft{consequence: true}

	normalized := normalizeBranchConsequenceBarriers([]branchAtomDraft{write, barrier, original, barrier})
	if len(normalized) != 3 || normalized[0].consequence || !normalized[1].original || !normalized[2].consequence {
		t.Fatalf("original-only normalization = %#v, want write/original/consequence", normalized)
	}

	current := branchAtomDraft{apply: branchIdentityKernel, access: branchAtomAccess{
		valueWrites: []statekey.Value{statekey.SymbolValue(2)},
	}}
	normalized = normalizeBranchConsequenceBarriers([]branchAtomDraft{write, barrier, current, barrier})
	if len(normalized) != 4 || !normalized[1].consequence || normalized[2].consequence || !normalized[3].consequence {
		t.Fatalf("current-dependent normalization = %#v, want write/consequence/write/consequence", normalized)
	}

	originalWithWrite := original
	originalWithWrite.seed.WritePaths = []keyspace.Key{{}}
	normalized = normalizeBranchConsequenceBarriers([]branchAtomDraft{write, barrier, originalWithWrite, barrier})
	if len(normalized) != 4 || !normalized[1].consequence || !normalized[3].consequence {
		t.Fatalf("writing original atom crossed consequence fence: %#v", normalized)
	}
}

func TestBranchRelationFactorsExposeValuesTopReachabilityAndOriginalRoles(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9201)
	target := symbol.ID(9201)
	path := pathdom.NewPath(target, "target")
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, typevalue.NewCache())
	constraint := factflow.NewValueConstraint(product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String)))
	facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
		point: factflow.NewBranchRefinementSet(factflow.NewBranchRefinement(path, constraint, true, factflow.ValueRefinement{}, false)),
	}})
	factors, err := authority.PrepareBranchRelationFactors(domain, PlanBranchRelationTransaction(facts, point, true), mustCoordinateFactorInventory(t, authority, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	factor, ok := factors.Factor(0)
	if !ok || len(factor.ValueWrites()) != 1 {
		t.Fatalf("guard factor = %#v/%t", factor, ok)
	}
	layout, layoutOK := factors.FactorLayout(0)
	writes := layout.CurrentValueWriteOrdinals()
	if !layoutOK || len(writes) != 1 || layout.WritesValuesTop() {
		t.Fatalf("guard factor write layout = %v/top=%t/%t", writes, layout.WritesValuesTop(), layoutOK)
	}
	writes[0] = -1
	if got := layout.CurrentValueWriteOrdinals(); len(got) != 1 || got[0] < 0 {
		t.Fatal("branch factor layout exposed mutable Values-write metadata")
	}
	if !factor.CurrentValuesTopRead() || !factor.ValuesTopPreserve() || factor.ValuesTopWrite() {
		t.Fatalf("finite Values roles: read=%t preserve=%t write=%t", factor.CurrentValuesTopRead(), factor.ValuesTopPreserve(), factor.ValuesTopWrite())
	}
	if !factor.CurrentReachabilityRead() {
		t.Fatal("guard factor omitted reachability feasibility input")
	}

	truthyFacts := factflow.NewFacts(factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceOnEdge(path, true)),
	}})
	truthy, err := authority.PrepareBranchRelationFactors(domain, PlanBranchRelationTransaction(truthyFacts, point, true), mustCoordinateFactorInventory(t, authority, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	truthyFactor, ok := truthy.Factor(0)
	truthyLayout, factorNative := truthy.FactorLayout(0)
	if !ok || !factorNative || truthyLayout.OriginalValueCount() != 1 || truthyLayout.CurrentValueCount() != 0 ||
		len(truthyFactor.OriginalValueReads()) != 0 || len(truthyFactor.ValueReads()) != 0 || !truthyFactor.OriginalReachabilityRead() {
		t.Fatalf("truthy original/current roles are not separated: %#v/%t", truthyFactor, ok)
	}
}

func TestBranchRelationFactorRejectsToSparseProductBottom(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9203)
	target := symbol.ID(9203)
	targetPath := pathdom.NewPath(target, "target")
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	stringOnly := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
		point: factflow.NewBranchRefinementSet(factflow.NewBranchRefinement(
			targetPath, factflow.NewValueConstraint(stringOnly), true, factflow.ValueRefinement{}, false,
		)),
	}})
	targetKey, ok := factKeyspaceKeyAt(resolver, point, targetPath)
	if !ok {
		t.Fatal("target path is unresolved")
	}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: targetKey, Presence: presence.Present()}
	proofSlot, err := domain.PathBranchProofCoordinateSlot(resolver.KeySpace(), proof)
	if err != nil {
		t.Fatal(err)
	}
	transaction := PlanBranchRelationTransaction(facts, point, true)
	factors, err := authority.PrepareBranchRelationFactors(
		domain, transaction, mustCoordinateFactorInventory(t, authority, domain, []state.CoordinateSlot{proofSlot}),
	)
	if err != nil {
		t.Fatal(err)
	}
	numberOnly := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(target), numberOnly).
		AddBranchProof(proof)
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	result := factors.ApplyFactor(0, edge, input, input)
	if result.Err != nil || result.Canceled {
		t.Fatalf("rejecting factor = canceled %t err %v", result.Canceled, result.Err)
	}
	if !stateIsBottom(reg, result.Output) {
		t.Fatal("rejecting factor did not project to canonical product bottom")
	}
}

func TestScalarBranchRelationsHaveOneCanonicalFactorKernel(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9207)
	x, y, xs := symbol.ID(9207), symbol.ID(9208), symbol.ID(9209)
	xPath, yPath, xsPath := pathdom.NewPath(x, "x"), pathdom.NewPath(y, "y"), pathdom.NewPath(xs, "xs")
	builder := visibility.NewBuilder()
	builder.Define(point, x, "x")
	builder.Define(point, y, "y")
	builder.Define(point, xs, "xs")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	length := factflow.NewBranchLenRefinementOnEdge(xsPath, 4, true)
	floor := factflow.NewBranchNumFloorRefinementOnEdge(xPath, 2, true)
	ceiling := factflow.NewBranchNumCeilRefinementOnEdge(yPath, 9, true)
	difference := factflow.NewBranchScaledConstraintOnEdge(1, xPath, false, 0, pathdom.Path{}, false, yPath, false, 3, true)
	rows := factflow.NewBranchRefinementSet().
		WithLenRefinements(length).
		WithNumFloorRefinements(floor).
		WithNumCeilRefinements(ceiling).
		WithDiffConstraints(difference)
	facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: rows}})
	transaction := PlanBranchRelationTransaction(facts, point, true)
	factors, err := authority.PrepareBranchRelationFactors(domain, transaction, mustCoordinateFactorInventory(t, authority, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	if factors.Len() != 4 {
		t.Fatalf("scalar factor count = %d, want 4", factors.Len())
	}
	for index := 0; index < factors.Len(); index++ {
		layout, ok := factors.FactorLayout(index)
		if !ok {
			t.Fatalf("factor %d retained a State kernel", index)
		}
		if layout.CurrentValueCount() != 0 || layout.OriginalValueCount() != 0 ||
			len(layout.CurrentLanes()) != 0 || len(layout.OriginalLanes()) != 0 ||
			len(layout.CurrentCoordinates()) != 1 || len(layout.CurrentCoordinates()[0].Slots()) != 1 ||
			len(layout.OriginalCoordinates()) != 0 {
			t.Fatalf("factor %d layout is not one exact coordinate: %#v", index, layout)
		}
		coordinateWrites := layout.CurrentCoordinateWriteOrdinals()
		skeletonWrites := layout.CurrentCoordinateSkeletonWrites()
		if len(coordinateWrites) != 1 || len(coordinateWrites[0]) > 1 ||
			(len(coordinateWrites[0]) == 1 && coordinateWrites[0][0] != 0) ||
			len(skeletonWrites) != 1 || !skeletonWrites[0] {
			t.Fatalf("factor %d coordinate write layout = %v/%v", index, coordinateWrites, skeletonWrites)
		}
		if len(coordinateWrites[0]) != 0 {
			coordinateWrites[0][0] = -1
		}
		skeletonWrites[0] = false
		if got := layout.CurrentCoordinateWriteOrdinals()[0]; len(got) != len(coordinateWrites[0]) ||
			(len(got) != 0 && got[0] != 0) || !layout.CurrentCoordinateSkeletonWrites()[0] {
			t.Fatalf("factor %d exposed mutable coordinate-write metadata", index)
		}
		if factors.prepared.atoms[index].apply != nil || factors.prepared.atoms[index].factor == nil {
			t.Fatalf("factor %d has parallel or absent semantic kernels", index)
		}
		if err := validateBranchCoordinatePatch(domain, factors.prepared.factorPlans[index], nil, nil); err == nil {
			t.Fatalf("factor %d accepted a malformed partial patch", index)
		}
	}

	unchangedSlot := statekey.SymbolValue(symbol.ID(9210))
	unchangedValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	input := state.Reachable(state.State{}).WriteValue(reg, unchangedSlot, unchangedValue)
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	want := applyBranchLenRefinement(edge, resolver, input, length)
	want = applyBranchNumFloorRefinement(edge, resolver, want, floor)
	want = applyBranchNumCeilRefinement(edge, resolver, want, ceiling)
	want = applyBranchDiffConstraint(edge, resolver, want, difference)
	got := input
	for _, stage := range factors.Stages() {
		for _, index := range stage.Factors() {
			result := factors.ApplyFactor(index, edge, input, got)
			if result.Err != nil || result.Canceled {
				t.Fatalf("factor %d = canceled %t err %v", index, result.Canceled, result.Err)
			}
			got = result.Output
		}
	}
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native scalar relations differ from the former concrete semantics")
	}
	if value := got.ReadValue(reg, unchangedSlot); !product.Equal(reg, value, unchangedValue) {
		t.Fatal("scalar factor patch changed an unrelated Values coordinate")
	}
}

func TestBranchCoordinateSkeletonWriteOwnsCompleteSelectedFamilyImage(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	firstPath := keys.FromPath(pathdom.NewPath(symbol.ID(9207), "first"))
	secondPath := keys.FromPath(pathdom.NewPath(symbol.ID(9208), "second"))
	first, err := domain.PrepareCoordinateBranchBound(state.CoordinateBoundLength, state.CoordinateBoundLower, keys, firstPath, 3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.PrepareCoordinateBranchBound(state.CoordinateBoundLength, state.CoordinateBoundLower, keys, secondPath, 5)
	if err != nil {
		t.Fatal(err)
	}
	family := first.Slot().Family()
	if second.Slot().Family() != family {
		t.Fatal("length bounds did not share their registered coordinate family")
	}
	currentSkeleton, err := domain.CoordinateSkeletonBottom(family, keys)
	if err != nil {
		t.Fatal(err)
	}
	currentFirst, err := domain.CoordinateDefault(currentSkeleton, first.Slot())
	if err != nil {
		t.Fatal(err)
	}
	currentSecond, err := domain.CoordinateDefault(currentSkeleton, second.Slot())
	if err != nil {
		t.Fatal(err)
	}
	patchSkeleton, patchFirst, err := domain.ApplyCoordinateBranchMutation(first, currentSkeleton, currentFirst)
	if err != nil {
		t.Fatal(err)
	}
	patchSecondBase, err := domain.CoordinateDefault(patchSkeleton, second.Slot())
	if err != nil {
		t.Fatal(err)
	}
	var patchSecond state.CoordinateScalarFactor
	patchSkeleton, patchSecond, err = domain.ApplyCoordinateBranchMutation(second, patchSkeleton, patchSecondBase)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.CoordinateScalarRepresentationEqual(currentSecond, patchSecond)
	if err != nil || equal {
		t.Fatalf("fixture did not change the sibling scalar representation: equal=%t err=%v", equal, err)
	}

	plan := &branchAtomFactorPlan{layout: BranchRelationFactorLayout{
		currentCoordinates:      []BranchRelationCoordinateLayout{{family: family, slots: []state.CoordinateSlot{first.Slot(), second.Slot()}}},
		writeCoordinateOrdinals: [][]int{{0}},
		writeCoordinateSkeleton: []bool{true},
	}}
	current := []BranchRelationCoordinateOperands{{Skeleton: currentSkeleton, Scalars: []state.CoordinateScalarFactor{currentFirst, currentSecond}}}
	patch := []BranchRelationCoordinateOperands{{Skeleton: patchSkeleton, Scalars: []state.CoordinateScalarFactor{patchFirst, patchSecond}}}
	if err := validateBranchCoordinatePatch(domain, plan, current, patch); err != nil {
		t.Fatalf("skeleton-owned selected-family image was rejected: %v", err)
	}

	// Without skeleton ownership the scalar list remains exact: ordinal zero
	// cannot authorize changing its sibling.
	plan.layout.writeCoordinateSkeleton[0] = false
	patch[0].Skeleton = currentSkeleton
	if err := validateBranchCoordinatePatch(domain, plan, current, patch); err == nil {
		t.Fatal("scalar-only authority accepted an undeclared sibling write")
	}
}

func TestBranchValueRoleIdentityIsLexicalAndTransactionSealed(t *testing.T) {
	seal := new(branchProgramSeal)
	source, ok := branchLexicalValueRoleSource(symbol.ID(9217))
	if !ok {
		t.Fatal("lexical role source")
	}
	roles, err := sealBranchValueRoleSources([]branchValueRoleSource{source, source}, seal)
	if err != nil || len(roles) != 1 {
		t.Fatalf("sealed roles = %#v, err %v", roles, err)
	}
	if id, valid := roles[0].LexicalSymbol(); !valid || id != symbol.ID(9217) {
		t.Fatalf("lexical identity = %d/%v", id, valid)
	}
	if roles[0].validFor(new(branchProgramSeal)) {
		t.Fatal("role crossed branch transaction ownership")
	}
	atom := branchAtom{
		seal: seal,
		factor: func(branchAtomFactorRuntime, BranchRelationFactorFrame, BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
			return BranchRelationFactorPatch{}, true, nil
		},
		valueRoles: branchAtomValueRoles{currentReads: roles},
	}
	plan, err := sealBranchAtomFactorPlan(state.RegisteredProductDomain(standard.Registry()), nil, atom, seal)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || plan.layout.CurrentValueCount() != 1 || len(plan.layout.CurrentValueRoles()) != 1 {
		t.Fatalf("role layout = %#v", plan)
	}
	if _, ok := newBranchLexicalValueRole(seal, 0); ok {
		t.Fatal("zero lexical symbol minted a role")
	}
}

// Static path evidence has a producer-sealed coordinate identity.  Its exact
// dependency closure is therefore the only path-family authority the factor
// may expose; widening that access back to the whole family makes one proof
// appear to affect every independent implication row in the body.  Dynamic
// unbound keys are intentionally covered by the separate declaration test.
func TestStaticBranchPathEvidenceUsesExactCoordinateAccess(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9211)
	left, right := symbol.ID(9211), symbol.ID(9212)
	leftPath, rightPath := pathdom.NewPath(left, "left"), pathdom.NewPath(right, "right")
	builder := visibility.NewBuilder()
	builder.Define(point, left, "left")
	builder.Define(point, right, "right")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, typevalue.NewCache())
	pathFamily, ok := domain.PathValueFamily()
	if !ok {
		t.Fatal("path coordinate family missing")
	}

	proofs := map[string]factflow.BranchPathEvidence{
		"presence":       factflow.NewBranchPathPresenceEvidenceOnEdge(leftPath, presence.Present(), true),
		"not-equal":      factflow.NewBranchPathInequalityEvidenceOnEdge(leftPath, rightPath, true),
		"equal":          factflow.NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, true),
		"index-in-range": factflow.NewBranchIndexInRangeEvidenceOnEdge(leftPath, rightPath, true),
	}
	for name, proof := range proofs {
		t.Run(name, func(t *testing.T) {
			facts := factflow.NewFacts(factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
				point: factflow.NewBranchPathEvidenceSet(proof),
			}})
			factors, err := authority.PrepareBranchRelationFactors(
				domain,
				PlanBranchRelationTransaction(facts, point, true),
				mustCoordinateFactorInventory(t, authority, domain, nil),
			)
			if err != nil {
				t.Fatal(err)
			}
			exact := false
			for index := 0; index < factors.Len(); index++ {
				factor, present := factors.Factor(index)
				if !present {
					t.Fatalf("factor %d is absent", index)
				}
				for _, lane := range append(factor.LaneReads(), factor.LaneWrites()...) {
					if lane.Ordinal() == pathFamily.Lane().Ordinal() {
						t.Fatalf("factor %d widened static proof to whole path family %q", index, lane.ID())
					}
				}
				exact = exact || len(factor.CoordinateWrites()) != 0
			}
			if !exact {
				t.Fatal("static proof has no exact coordinate write")
			}
		})
	}
}

// IndexInRange has two coupled consequences: its path proof and a ceiling
// supplied only when the resolved array type has an exact static length.  The
// concrete State adapter and the factor-native contract path must consume the
// same bound derivation and produce the same product image.
func TestIndexInRangeFactorMatchesConcreteStateKernel(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9221)
	index, array := symbol.ID(9221), symbol.ID(9222)
	indexPath, arrayPath := pathdom.NewPath(index, "index"), pathdom.NewPath(array, "array")
	builder := visibility.NewBuilder()
	builder.Define(point, index, "index")
	builder.Define(point, array, "array")
	resolver := visibility.NewResolver(builder.Build())
	types := typevalue.NewCache()
	authority := NewPathSemanticAuthority(resolver, nil, types)
	proof := factflow.NewBranchIndexInRangeEvidenceOnEdge(indexPath, arrayPath, true)
	facts := factflow.NewFacts(factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(proof),
	}})
	factors, err := authority.PrepareBranchRelationFactors(
		domain, PlanBranchRelationTransaction(facts, point, true),
		mustCoordinateFactorInventory(t, authority, domain, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	arrayValue := types.FromTypeWithWitness(reg, typ.NewTuple(typ.String, typ.Integer))
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(index), typevalue.FromType(reg, typ.Integer)).
		WriteValue(reg, statekey.SymbolValue(array), arrayValue)
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	want := applyBranchIndexStaticLengthCeil(types, edge, resolver, nil, input, proof)
	want = applyBranchPathEvidence(types, edge, resolver, nil, want, proof)
	got := input
	for _, stage := range factors.Stages() {
		for _, factor := range stage.Factors() {
			result := factors.ApplyFactor(factor, edge, input, got)
			if result.Err != nil || result.Canceled {
				t.Fatalf("factor %d = canceled %t err %v", factor, result.Canceled, result.Err)
			}
			got = result.Output
		}
	}
	if !domain.Lattice().Equal(got, want) {
		t.Fatalf("IndexInRange factor image differs from concrete State kernel\nwant: %#v\n got: %#v", want, got)
	}
	indexKey, ok := visibility.AddressAt(resolver, point, indexPath).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("index key is unresolved")
	}
	if ceiling, present := got.ReadNumCeil(resolver.KeySpace(), indexKey); !present || ceiling != 2 {
		t.Fatalf("IndexInRange ceiling = %d/%t, want 2/true", ceiling, present)
	}
}

func TestStaticPresenceFactorCarriesEqualitySupportForCanonicalClosure(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9215)
	left, right := symbol.ID(9215), symbol.ID(9216)
	leftPath, rightPath := pathdom.NewPath(left, "left"), pathdom.NewPath(right, "right")
	builder := visibility.NewBuilder()
	builder.Define(point, left, "left")
	builder.Define(point, right, "right")
	resolver := visibility.NewResolver(builder.Build())
	keys := resolver.KeySpace()
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	leftKey, leftOK := factKeyspaceKeyAt(resolver, point, leftPath)
	rightKey, rightOK := factKeyspaceKeyAt(resolver, point, rightPath)
	if !leftOK || !rightOK {
		t.Fatal("presence paths did not resolve")
	}
	equality := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: leftKey, Other: rightKey}
	mirrored := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: rightKey, Presence: presence.Present()}
	equalitySlot, err := domain.PathBranchProofCoordinateSlot(keys, equality)
	if err != nil {
		t.Fatal(err)
	}
	mirroredSlot, err := domain.PathBranchProofCoordinateSlot(keys, mirrored)
	if err != nil {
		t.Fatal(err)
	}
	proof := factflow.NewBranchPathPresenceEvidenceOnEdge(leftPath, presence.Present(), true)
	facts := factflow.NewFacts(factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(proof),
	}})
	factors, err := authority.PrepareBranchRelationFactors(
		domain, PlanBranchRelationTransaction(facts, point, true),
		mustCoordinateFactorInventory(t, authority, domain, []state.CoordinateSlot{equalitySlot, mirroredSlot}),
	)
	if err != nil {
		t.Fatal(err)
	}
	factorIndex, factor, ok := branchTestFactorWritingCoordinate(domain, factors, mirroredSlot)
	if !ok || !branchTestContainsCoordinate(domain, factor.CoordinateReads(), equalitySlot) {
		t.Fatal("static presence factor omitted its equality support or mirrored write")
	}
	input := state.Reachable(state.State{}).AddBranchProof(equality)
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	result := factors.ApplyFactor(factorIndex, edge, input, input)
	if result.Err != nil || result.Canceled || !result.Output.HasBranchProof(mirrored) {
		t.Fatalf("exact presence factor failed canonical equality closure: canceled=%t err=%v", result.Canceled, result.Err)
	}
}

func TestStaticEqualityFactorCarriesDescendantRefinementTransfer(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9217)
	left, right := symbol.ID(9217), symbol.ID(9218)
	leftPath, rightPath := pathdom.NewPath(left, "left"), pathdom.NewPath(right, "right")
	builder := visibility.NewBuilder()
	builder.Define(point, left, "left")
	builder.Define(point, right, "right")
	resolver := visibility.NewResolver(builder.Build())
	keys := resolver.KeySpace()
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	leftChild, leftOK := factKeyspaceKeyAt(resolver, point, leftPath.Field("child"))
	rightChild, rightOK := factKeyspaceKeyAt(resolver, point, rightPath.Field("child"))
	if !leftOK || !rightOK {
		t.Fatal("equality descendant paths did not resolve")
	}
	leftSlot, err := domain.PathRefinementCoordinateSlot(keys, leftChild)
	if err != nil {
		t.Fatal(err)
	}
	rightSlot, err := domain.PathRefinementCoordinateSlot(keys, rightChild)
	if err != nil {
		t.Fatal(err)
	}
	proof := factflow.NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, true)
	facts := factflow.NewFacts(factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(proof),
	}})
	factors, err := authority.PrepareBranchRelationFactors(
		domain, PlanBranchRelationTransaction(facts, point, true),
		mustCoordinateFactorInventory(t, authority, domain, []state.CoordinateSlot{leftSlot}),
	)
	if err != nil {
		t.Fatal(err)
	}
	factorIndex, factor, ok := branchTestFactorWritingCoordinate(domain, factors, rightSlot)
	if !ok || !branchTestContainsCoordinate(domain, factor.CoordinateReads(), leftSlot) {
		t.Fatal("static equality factor omitted its descendant refinement transfer")
	}
	tableType := typetable.NewRecord().Field("child", typ.String).Build()
	stringValue := typevalue.FromType(reg, typ.String)
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(left), typevalue.FromType(reg, tableType)).
		WriteValue(reg, statekey.SymbolValue(right), typevalue.FromType(reg, tableType)).
		WritePathKey(reg, keys, keys.Format(leftChild), stringValue)
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	result := factors.ApplyFactor(factorIndex, edge, input, input)
	if result.Err != nil || result.Canceled {
		t.Fatalf("exact equality factor canceled=%t err=%v", result.Canceled, result.Err)
	}
	if got := result.Output.ReadPathKey(reg, keys, keys.Format(rightChild)); !product.Equal(reg, got, stringValue) {
		t.Fatal("exact equality factor failed canonical descendant refinement transfer")
	}
}

func branchTestContainsCoordinate(domain state.ProductDomain, coordinates []state.CoordinateSlot, want state.CoordinateSlot) bool {
	for _, coordinate := range coordinates {
		equal, err := domain.CoordinateSlotEqual(coordinate, want)
		if err == nil && equal {
			return true
		}
	}
	return false
}

func branchTestFactorWritingCoordinate(
	domain state.ProductDomain,
	factors BranchRelationFactors,
	want state.CoordinateSlot,
) (int, BranchRelationFactor, bool) {
	for index := 0; index < factors.Len(); index++ {
		factor, ok := factors.Factor(index)
		if ok && branchTestContainsCoordinate(domain, factor.CoordinateWrites(), want) {
			return index, factor, true
		}
	}
	return 0, BranchRelationFactor{}, false
}

func TestBranchRelationFactorElidesUnactivatedPresenceConsequenceTopology(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9221)
	first, second, third := symbol.ID(9221), symbol.ID(9222), symbol.ID(9223)
	builder := visibility.NewBuilder()
	for sym, name := range map[symbol.ID]string{first: "first", second: "second", third: "third"} {
		builder.Define(point, sym, name)
	}
	resolver := visibility.NewResolver(builder.Build())
	keys := resolver.KeySpace()
	keyOf := func(sym symbol.ID, name string) keyspace.Key { return keys.FromPath(pathdom.NewPath(sym, name)) }
	rows := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(keyOf(first, "first"), presence.Present(), keyOf(second, "second"), presence.Present()),
		pathevidence.NewPathPresenceImplication(keyOf(second, "second"), presence.Present(), keyOf(third, "third"), presence.Present()),
	}
	slots := make([]state.CoordinateSlot, len(rows))
	for index, row := range rows {
		var err error
		slots[index], err = domain.PresenceImplicationCoordinateSlot(keys, row)
		if err != nil {
			t.Fatal(err)
		}
	}
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	transaction := PlanBranchRelationTransaction(factflow.NewFacts(factflow.FactsInput{}), point, true)
	factors, err := authority.PrepareBranchRelationFactors(domain, transaction, mustCoordinateFactorInventory(t, authority, domain, slots))
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < factors.Len(); index++ {
		if _, present := factors.PresenceImplicationDependencyPlan(index); present {
			t.Fatalf("unactivated transaction retained empty consequence factor %d", index)
		}
	}
	if factors.Len() != 0 || len(factors.Stages()) != 0 {
		t.Fatalf("empty transaction retained factors/stages = %d/%d", factors.Len(), len(factors.Stages()))
	}
}

func TestBranchRelationFactorsDeclareUnboundDynamicPresenceAsPathFamily(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9301)
	table := symbol.ID(9301)
	tablePath := pathdom.NewPath(table, "table")
	builder := visibility.NewBuilder()
	builder.Define(point, table, "table")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, typevalue.NewCache())
	transaction, ok := PlanBranchRelationTransaction(factflow.NewFacts(factflow.FactsInput{}), point, true).WithDynamicPresenceProof(tablePath)
	if !ok {
		t.Fatal("dynamic transaction planning failed")
	}
	factors, err := authority.PrepareBranchRelationFactors(domain, transaction, mustCoordinateFactorInventory(t, authority, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	factor, ok := factors.Factor(0)
	if !ok || len(factor.ValueReads()) != 0 || len(factor.ValueWrites()) != 0 {
		t.Fatalf("dynamic declaration invented Values access: %#v/%t", factor, ok)
	}
	family, _ := domain.PathValueFamily()
	if len(factor.LaneReads()) != 1 || factor.LaneReads()[0] != family.Lane() ||
		len(factor.LaneWrites()) != 1 || factor.LaneWrites()[0] != family.Lane() {
		t.Fatal("dynamic declaration omitted whole path-family ownership")
	}
}

func TestBranchRelationFactorsBindOnlyDynamicFactor(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9351)
	table := symbol.ID(9351)
	tablePath := pathdom.NewPath(table, "table")
	builder := visibility.NewBuilder()
	builder.Define(point, table, "table")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, typevalue.NewCache())
	transaction, ok := PlanBranchRelationTransaction(factflow.NewFacts(factflow.FactsInput{}), point, true).WithDynamicPresenceProof(tablePath)
	if !ok {
		t.Fatal("dynamic transaction planning failed")
	}
	declaration, err := authority.PrepareBranchRelationFactors(domain, transaction, mustCoordinateFactorInventory(t, authority, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	wantStages := declaration.Stages()
	bound, ok := declaration.BindDynamicPresenceKey(reg, typevalue.LiteralString(reg, "ready"))
	if !ok || bound.Len() != declaration.Len() {
		t.Fatalf("bound factors = %d/%t, declaration = %d", bound.Len(), ok, declaration.Len())
	}
	if !declaration.prepared.hasDynamic || declaration.prepared.dynamicAtom < 0 ||
		bound.prepared.dynamicAtom != declaration.prepared.dynamicAtom ||
		bound.prepared.dynamicStep != declaration.prepared.dynamicStep ||
		!bound.prepared.dynamicBound.dynamic.keyBound {
		t.Fatalf("dynamic template identity drifted: declaration=%d/%d bound=%d/%d",
			declaration.prepared.dynamicAtom, declaration.prepared.dynamicStep,
			bound.prepared.dynamicAtom, bound.prepared.dynamicStep)
	}
	for index := range declaration.prepared.atoms {
		if index == declaration.prepared.dynamicAtom {
			continue
		}
		before, after := declaration.prepared.atoms[index], bound.prepared.atoms[index]
		if before.dependencyID != after.dependencyID || before.fence != after.fence || before.dynamic != after.dynamic ||
			!reflect.DeepEqual(before.access, after.access) ||
			reflect.ValueOf(before.apply).Pointer() != reflect.ValueOf(after.apply).Pointer() {
			t.Fatalf("non-dynamic factor %d changed during key binding", index)
		}
	}
	gotStages := bound.Stages()
	if len(gotStages) != len(wantStages) {
		t.Fatalf("bound stages = %d, want %d", len(gotStages), len(wantStages))
	}
	for index := range wantStages {
		want, got := wantStages[index].Factors(), gotStages[index].Factors()
		if len(got) != len(want) {
			t.Fatalf("stage %d factors = %v, want %v", index, got, want)
		}
		for factor := range want {
			if got[factor] != want[factor] {
				t.Fatalf("stage %d factors = %v, want %v", index, got, want)
			}
		}
	}
	declared, _ := declaration.Factor(0)
	exact, _ := bound.Factor(0)
	if len(declared.LaneWrites()) == 0 || len(exact.LaneWrites()) != 0 || len(exact.CoordinateWrites()) != 1 {
		t.Fatalf("dynamic specialization declaration/exact access = lanes %d/%d coordinates %d",
			len(declared.LaneWrites()), len(exact.LaneWrites()), len(exact.CoordinateWrites()))
	}
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	out := state.Reachable(state.State{})
	for index := 0; index < bound.Len(); index++ {
		if _, present := bound.PresenceImplicationDependencyPlan(index); present {
			t.Fatalf("factor %d unexpectedly requires a consequence program", index)
		}
		result := bound.ApplyFactor(index, edge, out, out)
		if result.Err != nil || result.Canceled {
			t.Fatalf("factor %d = canceled %t err %v", index, result.Canceled, result.Err)
		}
		out = result.Output
	}
	if !out.HasBranchProofKind(pathevidence.BranchProofPathPresence) {
		t.Fatal("specialized dynamic factor did not publish its exact proof")
	}
}

func TestBranchNumericFactorWorksWithoutPathEvidenceLane(t *testing.T) {
	reg := standard.Registry()
	domain, err := state.TryRegisteredProductDomainWithLanes(reg, []state.LaneID{state.LaneValues, state.LaneNumFloors})
	if err != nil {
		t.Fatal(err)
	}
	point := cfg.Point(9401)
	target := symbol.ID(9401)
	path := pathdom.NewPath(target, "n")
	builder := visibility.NewBuilder()
	builder.Define(point, target, "n")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
		point: factflow.NewBranchRefinementSet().WithNumFloorRefinements(factflow.NewBranchNumFloorRefinementOnEdge(path, 7, true)),
	}})
	out, err := applyPreparedBranchFactorsForTest(context.Background(), authority, domain, PlanBranchRelationTransaction(facts, point, true), state.Reachable(state.State{}))
	if err != nil {
		t.Fatal(err)
	}
	key, ok := visibility.AddressAt(resolver, point, path).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("numeric path did not resolve")
	}
	if floor, ok := out.ReadNumFloor(resolver.KeySpace(), key); !ok || floor != 7 {
		t.Fatalf("numeric floor = %d/%t, want 7", floor, ok)
	}
}

func TestBranchRelationFactorExecutionEqualsDirectTransaction(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9501)
	left, right := symbol.ID(9501), symbol.ID(9502)
	leftPath, rightPath := pathdom.NewPath(left, "left"), pathdom.NewPath(right, "right")
	builder := visibility.NewBuilder()
	builder.Define(point, left, "left")
	builder.Define(point, right, "right")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, typevalue.NewCache())
	facts := factflow.NewFacts(factflow.FactsInput{BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
		point: factflow.NewBranchPathRelationSet(factflow.NewBranchPathEquality(leftPath, rightPath, true, false)),
	}})
	transaction := PlanBranchRelationTransaction(facts, point, true)
	number := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	input := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(left), number).WriteValue(reg, statekey.SymbolValue(right), product.Top())
	direct, err := applyPreparedBranchFactorsForTest(context.Background(), authority, domain, transaction, input)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := authority.PrepareBranchRelationFactors(domain, transaction, mustCoordinateFactorInventory(t, authority, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	edge := transfer.EdgeContext{Context: context.Background(), Registry: reg, Edge: cfg.Edge{From: point, Cond: true}, HasCond: true}
	factored := input
	for index := 0; index < factors.Len(); index++ {
		if _, present := factors.PresenceImplicationDependencyPlan(index); present {
			t.Fatalf("factor %d unexpectedly requires a consequence program", index)
		}
		result := factors.ApplyFactor(index, edge, input, factored)
		if result.Err != nil || result.Canceled {
			t.Fatalf("factor %d = canceled %t err %v", index, result.Canceled, result.Err)
		}
		factored = result.Output
	}
	if !domain.Lattice().Equal(direct, factored) {
		t.Fatal("factor execution differs from direct canonical transaction")
	}
}

func TestBranchRelationFactorsPreserveLengthBeforeMemberInvalidation(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9601)
	result, channel := symbol.ID(9601), symbol.ID(9602)
	resultPath, channelPath := pathdom.NewPath(result, "result"), pathdom.NewPath(channel, "channel")
	chanInt := typ.NewAlias("__factor_order_ChanInt", typetable.NewRecord().Field("tag", typ.LiteralString("int")).Build())
	chanStr := typ.NewAlias("__factor_order_ChanStr", typetable.NewRecord().Field("tag", typ.LiteralString("str")).Build())
	intCase := typetable.NewRecord().Field("channel", chanInt).Field("value", typ.Number).Build()
	strCase := typetable.NewRecord().Field("channel", chanStr).Field("value", typ.String).Build()
	union := typeexpr.Union(intCase, strCase)
	builder := visibility.NewBuilder()
	builder.Define(point, result, "result")
	builder.Define(point, channel, "channel")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	floor := factflow.NewBranchLenRefinementOnEdge(resultPath.Field("value"), 7, true)
	relation := factflow.NewBranchPathEquality(resultPath.Field("channel"), channelPath, true, false)
	rows := factflow.NewBranchRefinementSet().WithLenRefinements(floor)
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: rows},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			point: factflow.NewBranchPathRelationSet(relation),
		},
	})
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(result), typevalue.FromType(reg, union)).
		WriteValue(reg, statekey.SymbolValue(channel), typevalue.FromType(reg, chanInt))
	canonical, err := applyPreparedBranchFactorsForTest(context.Background(), authority, domain, PlanBranchRelationTransaction(facts, point, true), input)
	if err != nil {
		t.Fatal(err)
	}
	memberKey, ok := visibility.AddressAt(resolver, point, resultPath.Field("value")).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("member path did not resolve")
	}
	if _, present := canonical.ReadLenFloor(resolver.KeySpace(), memberKey); present {
		t.Fatal("member-origin invalidation did not clear the earlier length fact")
	}
}
