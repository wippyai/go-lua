package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBranchPresenceAffectedConeKeepsOnlyRowActivatedByTruthyGuard(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(9781)
	okSymbol, xSymbol := symbol.ID(9781), symbol.ID(9782)
	unrelatedTriggerSymbol, unrelatedTargetSymbol := symbol.ID(9783), symbol.ID(9784)
	okPath := pathdom.NewPath(okSymbol, "ok")
	xPath := pathdom.NewPath(xSymbol, "x")
	unrelatedTriggerPath := pathdom.NewPath(unrelatedTriggerSymbol, "other_ok")
	unrelatedTargetPath := pathdom.NewPath(unrelatedTargetSymbol, "other_x")

	visibilityBuilder := visibility.NewBuilder()
	for sym, name := range map[symbol.ID]string{
		okSymbol: "ok", xSymbol: "x", unrelatedTriggerSymbol: "other_ok", unrelatedTargetSymbol: "other_x",
	} {
		visibilityBuilder.Define(point, sym, name)
	}
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	keyAt := func(path pathdom.Path) keyspace.Key {
		key, ok := visibility.AddressAt(resolver, point, path).RootOrVisibleKeyspaceKey()
		if !ok {
			t.Fatalf("path %s has no point-local key", path.String())
		}
		return key
	}
	okKey, xKey := keyAt(okPath), keyAt(xPath)
	unrelatedTriggerKey, unrelatedTargetKey := keyAt(unrelatedTriggerPath), keyAt(unrelatedTargetPath)
	affected := pathevidence.NewPathValueRefinementImplication(
		okKey, typevalue.LiteralBool(reg, true), xKey, typevalue.LiteralString(reg, "v"),
	)
	unrelated := pathevidence.NewPathValueRefinementImplication(
		unrelatedTriggerKey, typevalue.LiteralBool(reg, true), unrelatedTargetKey, typevalue.LiteralString(reg, "other"),
	)
	rowSlots := make([]state.CoordinateSlot, 0, 2)
	for _, row := range []pathevidence.PathPresenceImplication{affected, unrelated} {
		slot, err := domain.PresenceImplicationCoordinateSlot(resolver.KeySpace(), row)
		if err != nil {
			t.Fatal(err)
		}
		rowSlots = append(rowSlots, slot)
	}
	inventory, err := domain.SealCoordinateFactorInventory(resolver.KeySpace(), rowSlots)
	if err != nil {
		t.Fatal(err)
	}

	// This is the true-edge fact emitted for a bare boolean `if ok`: the edge
	// narrows ok to the exact true literal; the false edge narrows it to false.
	truthyFact := factflow.NewBranchRefinement(
		okPath,
		factflow.NewValueConstraint(typevalue.LiteralBool(reg, true)), true,
		factflow.NewValueConstraint(typevalue.LiteralBool(reg, false)), true,
	)
	facts := factflow.NewFacts(factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
		point: factflow.NewBranchRefinementSet(truthyFact),
	}, BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
		point: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceOnEdge(okPath, true)),
	}})
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	factors, err := authority.PrepareBranchRelationFactors(
		domain, PlanBranchRelationTransaction(facts, point, true), inventory,
	)
	if err != nil {
		t.Fatal(err)
	}

	ordinary, ok := factors.Factor(0)
	if !ok {
		t.Fatal("truthy branch ordinary factor missing")
	}
	wantValueWrite := statekey.SymbolValue(okSymbol)
	if got := ordinary.ValueWrites(); len(got) != 1 || got[0] != wantValueWrite {
		t.Fatalf("truthy factor ValueWrites=%v, want [%d]; CoordinateWrites=%d LaneWrites=%v", got, wantValueWrite, len(ordinary.CoordinateWrites()), ordinary.LaneWrites())
	}
	if got := ordinary.CoordinateWrites(); len(got) != 0 {
		t.Fatalf("root truthy factor CoordinateWrites=%d, want 0; ValueWrites=%v LaneWrites=%v", len(got), ordinary.ValueWrites(), ordinary.LaneWrites())
	}
	if got := ordinary.LaneWrites(); len(got) != 1 || got[0].ID() != state.LaneDynamicIndex {
		t.Fatalf("root truthy factor LaneWrites=%v, want descendant-invalidating dynamic-index lane", got)
	}
	foundOriginalTruthy := false
	for index := 0; index < factors.Len(); index++ {
		factor, present := factors.Factor(index)
		if !present {
			continue
		}
		layout, factorNative := factors.FactorLayout(index)
		roles := layout.OriginalValueRoles()
		roleSymbol, roleOK := symbol.ID(0), false
		if len(roles) == 1 {
			roleSymbol, roleOK = roles[0].LexicalSymbol()
		}
		wantSymbol, wantOK := statekey.ParseSymbolValue(wantValueWrite)
		if !factorNative || !roleOK || !wantOK || roleSymbol != wantSymbol {
			continue
		}
		foundOriginalTruthy = true
		if len(factor.ValueWrites()) != 0 || len(factor.CoordinateWrites()) != 0 || len(factor.LaneWrites()) != 0 {
			t.Fatalf("original truthy evidence gained physical writes: values=%v coordinates=%d lanes=%v", factor.ValueWrites(), len(factor.CoordinateWrites()), factor.LaneWrites())
		}
	}
	if !foundOriginalTruthy {
		t.Fatal("original truthy evidence factor missing")
	}

	var selected PresenceImplicationDependencyPlan
	nonemptyCones := 0
	for index := 0; index < factors.Len(); index++ {
		plan, present := factors.PresenceImplicationDependencyPlan(index)
		if !present {
			continue
		}
		blocks := plan.Stages()
		if len(blocks) == 0 || len(blocks[0].Blocks()) == 0 {
			continue
		}
		nonemptyCones++
		if nonemptyCones == 1 {
			selected = plan
		}
		for _, stage := range blocks {
			for _, block := range stage.Blocks() {
				if len(block.rows) != 1 || block.rows[0].Trigger != affected.Trigger || block.rows[0].Target != affected.Target {
					t.Fatalf("affected cone %d selected rows=%v, want only ok->x (unrelated %v->%v excluded)", nonemptyCones, block.rows, unrelated.Trigger, unrelated.Target)
				}
				if len(block.predicateActivations) != 0 {
					if len(block.predicateActivations) != 1 || block.predicateActivations[0] != (pathPredicateActivation{
						path: okKey, kind: pathPredicateActivationTruthiness, truthy: true,
					}) {
						t.Fatalf("predicate activations=%#v, want exact truthy(ok)", block.predicateActivations)
					}
					selected = plan
				}
			}
		}
	}
	if nonemptyCones == 0 {
		t.Fatalf("truthy writes did not activate an implication block: ValueWrites=%v CoordinateWrites=%d LaneWrites=%v", ordinary.ValueWrites(), len(ordinary.CoordinateWrites()), ordinary.LaneWrites())
	}
	if nonemptyCones != 1 {
		t.Fatalf("nonempty affected cones=%d, want one normalized guard-write + original-care closure", nonemptyCones)
	}
	stages := selected.Stages()
	if len(stages) != 1 || len(stages[0].Blocks()) != 1 {
		t.Fatalf("affected cone stages/blocks=%d/%d, want 1/1", len(stages), len(stages[0].Blocks()))
	}
	block := stages[0].Blocks()[0]
	if got := block.ValueReads(); len(got) != 2 || got[0] != statekey.SymbolValue(okSymbol) || got[1] != statekey.SymbolValue(xSymbol) {
		t.Fatalf("selected block ValueReads=%v, want ok and x; truthy ValueWrites=%v", got, ordinary.ValueWrites())
	}
	if got := block.ValueWrites(); len(got) != 1 || got[0] != statekey.SymbolValue(xSymbol) {
		t.Fatalf("selected block ValueWrites=%v, want x only", got)
	}
	rowSlot, rowSlotErr := domain.PresenceImplicationCoordinateSlot(resolver.KeySpace(), affected)
	if rowSlotErr != nil {
		t.Fatal(rowSlotErr)
	}
	coordinateReads := block.CoordinateReads()
	if len(coordinateReads) != 1 || len(block.CoordinateWrites()) != 0 {
		t.Fatalf("root implication block coordinate access reads/writes=%d/%d, want exact row-membership read only", len(coordinateReads), len(block.CoordinateWrites()))
	}
	equalRow, equalRowErr := domain.CoordinateSlotEqual(coordinateReads[0], rowSlot)
	if equalRowErr != nil || !equalRow {
		t.Fatalf("root implication membership read drifted: equal=%t err=%v", equalRow, equalRowErr)
	}
	if !selected.valid {
		t.Fatal("original predicate care factor did not attach its typed activation to the affected block")
	}

	// The activation is a read lens: it must make the broad boolean trigger
	// exact for this consequence round, refine x, and leave ok physically broad.
	activatedBlock := selected.Stages()[0].Blocks()[0]
	broadBoolean := product.Join(reg, typevalue.LiteralBool(reg, true), typevalue.LiteralBool(reg, false))
	input := state.Reachable(domain.Lattice().Bottom()).
		WriteValue(reg, statekey.SymbolValue(okSymbol), broadBoolean).
		WriteValue(reg, statekey.SymbolValue(xSymbol), product.Top()).
		AddPathPresenceImplication(affected)
	family, familyOK := domain.PathEvidenceCoordinateFamily()
	if !familyOK {
		t.Fatal("registered product has no path-evidence coordinate family")
	}
	factorsIn, factorErr := domain.DecomposeLanes(input, []state.ProductLane{family.Lane()})
	if factorErr != nil || len(factorsIn) != 1 {
		t.Fatalf("path family decomposition=%d/%v", len(factorsIn), factorErr)
	}
	skeleton, scalars, decomposeErr := domain.DecomposeCoordinateFamily(factorsIn[0], family, resolver.KeySpace())
	if decomposeErr != nil {
		t.Fatal(decomposeErr)
	}
	declaredScalars := make([]state.CoordinateScalarFactor, 0, len(activatedBlock.CoordinateReads()))
	for _, scalar := range scalars {
		for _, declared := range activatedBlock.CoordinateReads() {
			equal, equalErr := domain.CoordinateSlotEqual(scalar.Slot(), declared)
			if equalErr != nil {
				t.Fatal(equalErr)
			}
			if equal {
				declaredScalars = append(declaredScalars, scalar)
				break
			}
		}
	}
	if len(declaredScalars) != len(activatedBlock.CoordinateReads()) {
		t.Fatalf("declared row scalars=%d, want %d", len(declaredScalars), len(activatedBlock.CoordinateReads()))
	}
	_, values := state.DecomposeValueLane(domain.Lattice(), input)
	binding, bindingErr := SealPresenceImplicationRootBinding(selected, func(dependency statekey.ValueDependency) (statekey.Value, bool) {
		return dependency.Concrete()
	}, func(root statekey.Value) bool { return root != 0 })
	if bindingErr != nil {
		t.Fatal(bindingErr)
	}
	blockAuthority, authorityOK := binding.BlockAuthority(activatedBlock)
	if !authorityOK {
		t.Fatal("activated block authority")
	}
	empty, emptyErr := domain.SealCoordinateFactorInventory(resolver.KeySpace(), nil)
	if emptyErr != nil {
		t.Fatal(emptyErr)
	}
	undeclaredAuthority, authorityErr := state.SealCoordinatePathEvidenceAuthority(
		domain, resolver.KeySpace(), activatedBlock.ValueReads(), activatedBlock.ValueWrites(), empty, empty,
		false, false, func(root statekey.Value) bool { return root != 0 },
	)
	if authorityErr != nil {
		t.Fatal(authorityErr)
	}
	undeclared, undeclaredErr := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, declaredScalars, values, true,
		undeclaredAuthority, state.PathDescendantMutationFactors{},
	)
	if undeclaredErr != nil {
		t.Fatal(undeclaredErr)
	}
	if _, valid := undeclared.HasImplication(affected); valid {
		t.Fatal("row membership was observable without its declared coordinate read")
	}
	mutation := state.PathDescendantMutationFactors{}
	if activatedBlock.PathMutation() {
		var mutationErr error
		mutation, mutationErr = domain.DecomposePathDescendantMutationFactors(input, resolver.KeySpace())
		if mutationErr != nil {
			t.Fatal(mutationErr)
		}
	}
	carrier, openErr := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, declaredScalars, values, true,
		blockAuthority, mutation,
	)
	if openErr != nil {
		t.Fatal(openErr)
	}
	next, feasible, changed, applyErr := ApplyPresenceImplicationCoordinateRound(selected, context.Background(), carrier, activatedBlock, binding)
	if applyErr != nil || !feasible || !changed {
		t.Fatalf("activated round feasible=%t changed=%t err=%v", feasible, changed, applyErr)
	}
	gotOK, valid := next.ReadValue(statekey.SymbolValue(okSymbol))
	if !valid || !product.Equal(reg, gotOK, broadBoolean) {
		t.Fatalf("predicate activation escaped into stored ok: valid=%t value=%v", valid, gotOK)
	}
	gotX, valid := next.ReadValue(statekey.SymbolValue(xSymbol))
	if !valid || !presence.Equal(product.PresenceOf(gotX), presence.Present()) {
		t.Fatalf("activated consequence x presence=%v valid=%t, want present", product.PresenceOf(gotX), valid)
	}

}
