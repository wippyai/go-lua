package transformer

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalComponentTerminalForestAllocatesUniqueOwnedLeaves(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-owner"))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{
		lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1),
	})
	forest := formalComponentTestArena(t, program)
	first, firstOK := forest.authority(1)
	second, secondOK := forest.authority(2)
	if !firstOK || !secondOK || first == second {
		t.Fatal("formal terminal forest did not retain two body authorities")
	}
	firstTerm := first.terms.Root(Root{Kind: RootParam, Index: 0})
	secondTerm := second.terms.Root(Root{Kind: RootParam, Index: 0})
	firstLeaf, err := first.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: first.variable, arena: first.terms, term: firstTerm}})
	if err != nil {
		t.Fatal(err)
	}
	secondLeaf, err := second.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: second.variable, arena: second.terms, term: secondTerm}})
	if err != nil {
		t.Fatal(err)
	}
	if firstLeaf == secondLeaf || firstLeaf < 2 || secondLeaf < 2 {
		t.Fatalf("forest terminal leaves = %d/%d, want distinct global typed indexes", firstLeaf, secondLeaf)
	}
	if _, err := first.terminal(secondLeaf); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("first authority accepted second leaf: %v", err)
	}
	if _, err := first.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: second.variable, arena: second.terms, term: secondTerm}}); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("first authority accepted second arena term: %v", err)
	}
}

func TestFormalComponentSymbolicTermsFormCanonicalFiniteLattice(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-symbolic")))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{body})
	forest := formalComponentTestArena(t, program)
	authority, _ := forest.authority(1)
	left, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: authority.terms.Root(Root{Kind: RootParam, Index: 0})}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: authority.terms.Root(Root{Kind: RootParam, Index: 1})}})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := authority.combine(context.Background(), formalComponentJoin, left, right)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := authority.terminal(joined)
	if err != nil || terminal.kind != formalComponentBindings || len(terminal.bindings) != 2 {
		t.Fatalf("symbolic value union = %#v, %v", terminal, err)
	}
	if ordered, orderErr := authority.lessOrEq(left, joined); orderErr != nil || !ordered {
		t.Fatalf("singleton <= union = %t, %v", ordered, orderErr)
	}
	if ordered, orderErr := authority.lessOrEq(joined, left); orderErr != nil || ordered {
		t.Fatalf("union <= singleton = %t, %v", ordered, orderErr)
	}
	for _, op := range []formalComponentBinaryOp{formalComponentJoin, formalComponentWiden} {
		got, combineErr := authority.combine(context.Background(), op, right, left)
		if combineErr != nil || got != joined {
			t.Fatalf("symbolic union op %d = %d, %v; want canonical %d", op, got, combineErr, joined)
		}
	}
	for _, op := range []formalComponentBinaryOp{formalComponentMeet, formalComponentNarrow} {
		got, combineErr := authority.combine(context.Background(), op, joined, left)
		if got != 0 || !errors.Is(combineErr, errFormalSymbolicMeetUnproven) {
			t.Fatalf("unproven symbolic meet op %d = %d, %v", op, got, combineErr)
		}
	}
	if got, err := authority.combine(context.Background(), formalComponentWiden, left, left); err != nil || got != left {
		t.Fatalf("identical symbolic widen = %d, %v; want %d", got, err, left)
	}
	joinTerm := authority.terms.JoinValue(
		authority.terms.Root(Root{Kind: RootParam, Index: 1}),
		authority.terms.Root(Root{Kind: RootParam, Index: 0}),
	)
	flattened, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: joinTerm}})
	if err != nil || flattened != joined {
		t.Fatalf("sealed valueJoin set = %d, %v; want canonical union %d", flattened, err, joined)
	}
	pathLeft, err := authority.internPathTerm(formalQualifiedPathTerm{arena: authority.terms, term: authority.terms.Path(Root{Kind: RootParam, Index: 0})})
	if err != nil {
		t.Fatal(err)
	}
	pathRight, err := authority.internPathTerm(formalQualifiedPathTerm{arena: authority.terms, term: authority.terms.Path(Root{Kind: RootParam, Index: 1})})
	if err != nil {
		t.Fatal(err)
	}
	pathUnion, err := authority.combine(context.Background(), formalComponentJoin, pathLeft, pathRight)
	if err != nil {
		t.Fatal(err)
	}
	pathTerminal, err := authority.terminal(pathUnion)
	if err != nil || pathTerminal.kind != formalComponentPathTerms || len(pathTerminal.pathTerms) != 2 {
		t.Fatalf("symbolic path union = %#v, %v", pathTerminal, err)
	}
	if got, combineErr := authority.combine(context.Background(), formalComponentNarrow, pathUnion, pathLeft); got != 0 || !errors.Is(combineErr, errFormalSymbolicMeetUnproven) {
		t.Fatalf("unproven symbolic path narrow = %d, %v", got, combineErr)
	}
	outcome, err := authority.internOutcomeOccurrence(formalQualifiedOutcomeOccurrence{code: authority.code, ref: 1, root: 1})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err = authority.terminal(outcome)
	if err != nil || terminal.kind != formalComponentOutcomeOccurrence || terminal.outcome.ref != 1 {
		t.Fatalf("outcome terminal = %#v, %v", terminal, err)
	}
}

func TestFormalComponentSymbolicSetLawsAndLoopRecurrenceDoNotGrow(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-set-laws")))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{body})
	forest := formalComponentTestArena(t, program)
	authority, _ := forest.authority(1)
	valueLeaf := func(term ValueTerm) decisionLeaf {
		t.Helper()
		leaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: term}})
		if err != nil {
			t.Fatal(err)
		}
		return leaf
	}
	a := valueLeaf(authority.terms.Root(Root{Kind: RootParam, Index: 0}))
	b := valueLeaf(authority.terms.Root(Root{Kind: RootParam, Index: 1}))
	c := valueLeaf(authority.terms.Constant(product.Top()))
	ab, err := authority.combine(context.Background(), formalComponentJoin, a, b)
	if err != nil {
		t.Fatal(err)
	}
	bc, err := authority.combine(context.Background(), formalComponentJoin, b, c)
	if err != nil {
		t.Fatal(err)
	}
	left, err := authority.combine(context.Background(), formalComponentJoin, ab, c)
	if err != nil {
		t.Fatal(err)
	}
	right, err := authority.combine(context.Background(), formalComponentJoin, a, bc)
	if err != nil || left != right {
		t.Fatalf("symbolic union associativity = %d/%d, %v", left, right, err)
	}
	if got, meetErr := authority.combine(context.Background(), formalComponentMeet, a, b); got != 0 || !errors.Is(meetErr, errFormalSymbolicMeetUnproven) {
		t.Fatalf("unproven disjoint symbolic meet = %d, %v", got, meetErr)
	}
	if got, narrowErr := authority.combine(context.Background(), formalComponentNarrow, left, ab); got != 0 || !errors.Is(narrowErr, errFormalSymbolicMeetUnproven) {
		t.Fatalf("unproven symbolic narrow = %d, %v", got, narrowErr)
	}

	terminalCount := len(forest.terminals)
	valueCount, pathCount, guardCount := len(authority.terms.values), len(authority.terms.paths), len(authority.terms.guards)
	frameCount, loopCount, allocationCount := len(authority.terms.callFrames), len(authority.terms.loopMus), len(authority.terms.allocations)
	effectCount := len(authority.code.effects.nodes)
	current := a
	for iteration := 0; iteration < 1000; iteration++ {
		incoming := b
		if iteration&1 != 0 {
			incoming = a
		}
		current, err = authority.combine(context.Background(), formalComponentWiden, current, incoming)
		if err != nil {
			t.Fatal(err)
		}
	}
	if current != ab || len(forest.terminals) != terminalCount {
		t.Fatalf("stable loop recurrence = leaf %d terminals %d; want %d/%d", current, len(forest.terminals), ab, terminalCount)
	}
	if len(authority.terms.values) != valueCount || len(authority.terms.paths) != pathCount || len(authority.terms.guards) != guardCount ||
		len(authority.terms.callFrames) != frameCount || len(authority.terms.loopMus) != loopCount || len(authority.terms.allocations) != allocationCount ||
		len(authority.code.effects.nodes) != effectCount {
		t.Fatal("symbolic set recurrence grew sealed syntax arenas")
	}
}

func TestFormalComponentRegisteredLaneAndGroundAlgebraMatchesDomain(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-algebra")))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{body})
	forest := formalComponentTestArena(t, program)
	authority, _ := forest.authority(1)
	var lane state.ProductLane
	for _, candidate := range authority.product.NonValuesLaneInventory() {
		families, familyErr := authority.product.CoordinateFamilies(candidate)
		if familyErr != nil {
			t.Fatal(familyErr)
		}
		if len(families) == 0 {
			lane = candidate
			break
		}
	}
	if lane.ID() == "" {
		t.Fatal("registered test product has no ordinary lane")
	}
	bottom, err := authority.product.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	top, err := authority.product.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomLeaf, err := authority.internLane(context.Background(), bottom)
	if err != nil {
		t.Fatal(err)
	}
	topLeaf, err := authority.internLane(context.Background(), top)
	if err != nil {
		t.Fatal(err)
	}
	if equal, equalErr := authority.equal(bottomLeaf, topLeaf); equalErr != nil || equal {
		t.Fatalf("lane Bottom Equal Top = %t, %v", equal, equalErr)
	}
	if ordered, orderErr := authority.lessOrEq(bottomLeaf, topLeaf); orderErr != nil || !ordered {
		t.Fatalf("lane Bottom <= Top = %t, %v", ordered, orderErr)
	}
	for _, op := range []formalComponentBinaryOp{formalComponentJoin, formalComponentMeet, formalComponentWiden, formalComponentNarrow} {
		leaf, combineErr := authority.combine(context.Background(), op, bottomLeaf, topLeaf)
		if combineErr != nil {
			t.Fatalf("lane operation %d: %v", op, combineErr)
		}
		terminal, terminalErr := authority.terminal(leaf)
		if terminalErr != nil {
			t.Fatal(terminalErr)
		}
		var want state.LaneFactor
		switch op {
		case formalComponentJoin:
			want, err = authority.product.LaneJoin(bottom, top)
		case formalComponentMeet:
			want, err = authority.product.LaneMeet(bottom, top)
		case formalComponentWiden:
			want, err = authority.product.LaneWiden(bottom, top)
		case formalComponentNarrow:
			want, err = authority.product.LaneNarrow(bottom, top)
		}
		if err != nil {
			t.Fatal(err)
		}
		equal, equalErr := authority.product.LaneEqual(terminal.lane, want)
		if equalErr != nil || !equal {
			t.Fatalf("lane operation %d differs from ProductDomain: equal=%t err=%v", op, equal, equalErr)
		}
	}

	groundBottom, err := authority.internGroundValue(product.Bottom(authority.product.Registry()))
	if err != nil {
		t.Fatal(err)
	}
	groundTop, err := authority.internGroundValue(product.Top())
	if err != nil {
		t.Fatal(err)
	}
	joined, err := authority.combine(context.Background(), formalComponentJoin, groundBottom, groundTop)
	if err != nil {
		t.Fatal(err)
	}
	joinedTerminal, err := authority.terminal(joined)
	if err != nil || !product.Equal(authority.product.Registry(), joinedTerminal.ground, product.Top()) {
		t.Fatalf("ground join terminal = %#v, %v; want product Top", joinedTerminal, err)
	}
	if ordered, orderErr := authority.lessOrEq(groundBottom, groundTop); orderErr != nil || !ordered {
		t.Fatalf("ground Bottom <= Top = %t, %v", ordered, orderErr)
	}

	laneDefault, err := authority.defaultFor(context.Background(), formalFiberDescriptor{
		forest: program.formalFibers, variable: 1, role: formalFiberOrdinaryLane, lane: lane,
	})
	if err != nil || laneDefault.kind != formalComponentDefaultTerminal {
		t.Fatalf("ordinary-lane default = %#v, %v", laneDefault, err)
	}
	laneTerminal, err := authority.terminal(laneDefault.leaf)
	if err != nil {
		t.Fatal(err)
	}
	if equal, equalErr := authority.product.LaneEqual(laneTerminal.lane, bottom); equalErr != nil || !equal {
		t.Fatalf("ordinary-lane default differs from registered Bottom: %t, %v", equal, equalErr)
	}
	groundDefault, err := authority.defaultFor(context.Background(), formalFiberDescriptor{
		forest: program.formalFibers, variable: 1, role: formalFiberGroundValue,
	})
	if err != nil || groundDefault.kind != formalComponentDefaultTerminal {
		t.Fatalf("ground default = %#v, %v", groundDefault, err)
	}
	careDefault, err := authority.defaultFor(context.Background(), formalFiberDescriptor{
		forest: program.formalFibers, variable: 1, role: formalFiberCare,
	})
	if err != nil || careDefault.kind != formalComponentDefaultBooleanFalse {
		t.Fatalf("care default = %#v, %v", careDefault, err)
	}
	if _, err := authority.defaultFor(context.Background(), formalFiberDescriptor{
		forest: &formalFiberInventory{}, variable: 1, role: formalFiberCare,
	}); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("foreign descriptor default error = %v", err)
	}
}

func TestFormalComponentCoordinateArithmeticRequiresJointGroup(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-coordinate")))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{body})
	forest := formalComponentTestArena(t, program)
	authority, _ := forest.authority(1)
	for _, lane := range authority.product.NonValuesLaneInventory() {
		families, familyErr := authority.product.CoordinateFamilies(lane)
		if familyErr != nil {
			t.Fatal(familyErr)
		}
		if len(families) == 0 {
			continue
		}
		bottom, bottomErr := authority.product.CoordinateSkeletonBottom(families[0], authority.coordinateKeys)
		if bottomErr != nil {
			t.Fatal(bottomErr)
		}
		top, topErr := authority.product.CoordinateSkeletonTop(families[0], authority.coordinateKeys)
		if topErr != nil {
			t.Fatal(topErr)
		}
		left, internErr := authority.internCoordinateSkeleton(bottom)
		if internErr != nil {
			t.Fatal(internErr)
		}
		right, internErr := authority.internCoordinateSkeleton(top)
		if internErr != nil {
			t.Fatal(internErr)
		}
		if _, combineErr := authority.combine(context.Background(), formalComponentJoin, left, right); !errors.Is(combineErr, errFormalCoordinateGroupRequired) {
			t.Fatalf("independent coordinate join error = %v", combineErr)
		}
		if _, equalErr := authority.equal(left, right); !errors.Is(equalErr, errFormalCoordinateGroupRequired) {
			t.Fatalf("independent coordinate equality error = %v", equalErr)
		}
		if _, orderErr := authority.lessOrEq(left, right); !errors.Is(orderErr, errFormalCoordinateGroupRequired) {
			t.Fatalf("independent coordinate order error = %v", orderErr)
		}
		return
	}
	t.Fatal("registered test product has no coordinate family")
}

func TestFormalComponentAllRegisteredFactorRepresentationsDoNotGrow(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-all-factor-representations")))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{body})
	forest := formalComponentTestArena(t, program)
	authority, ok := forest.authority(1)
	if !ok {
		t.Fatal("formal component authority is missing")
	}
	lanes := authority.product.LaneInventory()
	if len(lanes) != 17 {
		t.Fatalf("registered factor count = %d, want all 17 State lanes", len(lanes))
	}
	for _, lane := range lanes {
		left, err := authority.product.LaneTop(lane)
		if err != nil {
			t.Fatal(err)
		}
		// Construct again through the registered carrier, rather than reusing the
		// first operand. Persistent map lanes may allocate a distinct spelling.
		right, err := authority.product.LaneTop(lane)
		if err != nil {
			t.Fatal(err)
		}
		before := len(forest.terminals)
		leftLeaf, err := authority.internLane(context.Background(), left)
		if err != nil {
			t.Fatalf("lane %q first spelling: %v", lane.ID(), err)
		}
		afterFirst := len(forest.terminals)
		rightLeaf, err := authority.internLane(context.Background(), right)
		if err != nil {
			t.Fatalf("lane %q second spelling: %v", lane.ID(), err)
		}
		if leftLeaf != rightLeaf || len(forest.terminals) != afterFirst || afterFirst < before {
			t.Fatalf("lane %q canonical collision = leaves %d/%d terminals %d/%d/%d", lane.ID(), leftLeaf, rightLeaf, before, afterFirst, len(forest.terminals))
		}
	}

	// Values is the representative persistent-map carrier: two independent
	// writes are semantic/canonical equals but deliberately not Same.
	valuesLane, present := authority.product.ProductLane(state.LaneValues)
	if !present {
		t.Fatal("Values lane is missing")
	}
	value := product.Top()
	leftState := state.State{}.WriteValue(authority.product.Registry(), statekey.SymbolValue(77), value)
	rightState := state.State{}.WriteValue(authority.product.Registry(), statekey.SymbolValue(77), value)
	leftFactors, err := authority.product.DecomposeLanes(leftState, []state.ProductLane{valuesLane})
	if err != nil {
		t.Fatal(err)
	}
	rightFactors, err := authority.product.DecomposeLanes(rightState, []state.ProductLane{valuesLane})
	if err != nil {
		t.Fatal(err)
	}
	if same, sameErr := authority.product.LaneSame(leftFactors[0], rightFactors[0]); sameErr != nil || same {
		t.Fatalf("independently built Values spelling Same = %t, %v; want false", same, sameErr)
	}
	leftLeaf, err := authority.internLane(context.Background(), leftFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	terminalCount := len(forest.terminals)
	rightLeaf, err := authority.internLane(context.Background(), rightFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	if leftLeaf != rightLeaf || len(forest.terminals) != terminalCount {
		t.Fatalf("independent Values spellings grew terminals: leaves %d/%d count %d/%d", leftLeaf, rightLeaf, terminalCount, len(forest.terminals))
	}

	// Coordinate skeleton and scalar terminal laws are registered separately
	// from ordinary lanes. Exercise every family and every frozen scalar slot.
	span, present := program.formalFibers.span(1)
	if !present {
		t.Fatal("formal factor span is missing")
	}
	for _, group := range authority.body.factors.nonValues {
		for _, family := range group.coordinateFamilies {
			leftSkeleton, err := authority.product.CoordinateSkeletonTop(family.family, authority.coordinateKeys)
			if err != nil {
				t.Fatal(err)
			}
			rightSkeleton, err := authority.product.CoordinateSkeletonTop(family.family, authority.coordinateKeys)
			if err != nil {
				t.Fatal(err)
			}
			leftLeaf, err := authority.internCoordinateSkeleton(leftSkeleton)
			if err != nil {
				t.Fatal(err)
			}
			terminalCount := len(forest.terminals)
			rightLeaf, err := authority.internCoordinateSkeleton(rightSkeleton)
			if err != nil {
				t.Fatal(err)
			}
			if leftLeaf != rightLeaf || len(forest.terminals) != terminalCount {
				t.Fatalf("coordinate family %q skeleton grew terminals", family.family.ID())
			}
			bottom, err := authority.product.CoordinateSkeletonBottom(family.family, authority.coordinateKeys)
			if err != nil {
				t.Fatal(err)
			}
			for _, ordinal := range family.scalars {
				if int(ordinal) < 0 || int(ordinal) >= span.count {
					t.Fatal("coordinate scalar ordinal is outside the body span")
				}
				slot := span.forest.descriptors[span.first+int(ordinal)].coordinate
				leftScalar, err := authority.product.CoordinateDefault(bottom, slot)
				if err != nil {
					t.Fatal(err)
				}
				rightScalar, err := authority.product.CoordinateDefault(bottom, slot)
				if err != nil {
					t.Fatal(err)
				}
				leftLeaf, err := authority.internCoordinateScalar(leftScalar)
				if err != nil {
					t.Fatal(err)
				}
				terminalCount := len(forest.terminals)
				rightLeaf, err := authority.internCoordinateScalar(rightScalar)
				if err != nil {
					t.Fatal(err)
				}
				if leftLeaf != rightLeaf || len(forest.terminals) != terminalCount {
					t.Fatalf("coordinate family %q scalar grew terminals", family.family.ID())
				}
			}
		}
	}
}

func TestFormalComponentCoordinatesUseBodyFormalKeySpace(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-component-coordinate-keyspace"))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{
		lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1),
	})
	forest := formalComponentTestArena(t, program)
	first, firstOK := forest.authority(1)
	second, secondOK := forest.authority(2)
	if !firstOK || !secondOK || first.keys != program.bodies[0].keys || first.coordinateKeys == program.bodies[0].keys || first.coordinateKeys == second.coordinateKeys {
		t.Fatal("formal terminal authorities did not retain distinct span keyspaces")
	}

	family := formalComponentTestCoordinateFamily(t, first)
	formalSkeleton, err := first.product.CoordinateSkeletonBottom(family, first.coordinateKeys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.internCoordinateSkeleton(formalSkeleton); err != nil {
		t.Fatalf("formal skeleton rejected: %v", err)
	}
	concreteSkeleton, err := first.product.CoordinateSkeletonBottom(family, program.bodies[0].keys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.internCoordinateSkeleton(concreteSkeleton); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("concrete skeleton error = %v, want foreign owner", err)
	}
	foreignSkeleton, err := second.product.CoordinateSkeletonBottom(family, second.coordinateKeys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.internCoordinateSkeleton(foreignSkeleton); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("cross-body skeleton error = %v, want foreign owner", err)
	}

	formalScalar := formalComponentTestCoordinateScalar(t, first, family, first.coordinateKeys)
	if _, err := first.internCoordinateScalar(formalScalar); err != nil {
		t.Fatalf("formal scalar rejected: %v", err)
	}
	concreteScalar := formalComponentTestCoordinateScalar(t, first, family, program.bodies[0].keys)
	if _, err := first.internCoordinateScalar(concreteScalar); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("concrete scalar error = %v, want foreign owner", err)
	}
	foreignScalar := formalComponentTestCoordinateScalar(t, second, family, second.coordinateKeys)
	if _, err := first.internCoordinateScalar(foreignScalar); !errors.Is(err, errFormalComponentForeignOwner) {
		t.Fatalf("cross-body scalar error = %v, want foreign owner", err)
	}
}

func formalComponentTestCoordinateFamily(t *testing.T, authority *formalComponentTerminalAuthority) state.CoordinateFamily {
	t.Helper()
	for _, lane := range authority.product.NonValuesLaneInventory() {
		families, err := authority.product.CoordinateFamilies(lane)
		if err != nil {
			t.Fatal(err)
		}
		for _, family := range families {
			_, handled, slotErr := authority.product.CoordinateReturnIdentitySlot(family, authority.coordinateKeys, identity.ID{Kind: "table", Site: t.Name(), Index: 1})
			if slotErr != nil {
				t.Fatal(slotErr)
			}
			if handled {
				return family
			}
		}
	}
	t.Fatal("registered test product has no return-identity coordinate family")
	return state.CoordinateFamily{}
}

func formalComponentTestCoordinateScalar(t *testing.T, authority *formalComponentTerminalAuthority, family state.CoordinateFamily, keys *keyspace.KeySpace) state.CoordinateScalarFactor {
	t.Helper()
	slot, handled, err := authority.product.CoordinateReturnIdentitySlot(family, keys, identity.ID{Kind: "table", Site: t.Name(), Index: 1})
	if err != nil || !handled {
		t.Fatalf("return-identity slot = handled %t, err %v", handled, err)
	}
	skeleton, err := authority.product.CoordinateSkeletonBottom(family, keys)
	if err != nil {
		t.Fatal(err)
	}
	scalar, err := authority.product.CoordinateDefault(skeleton, slot)
	if err != nil {
		t.Fatal(err)
	}
	return scalar
}

func formalComponentTestProgram(t *testing.T, bodies []lexicalidentity.StableLexicalBodyID) *RelationProgram {
	t.Helper()
	reg := standard.Registry()
	productDomain := state.RegisteredProductDomain(reg)
	program := &RelationProgram{registry: reg, bodies: make([]relationProgramBody, len(bodies)), byBody: make(map[lexicalidentity.StableLexicalBodyID]relationVar, len(bodies))}
	for index, bodyID := range bodies {
		variable := relationVar(index + 1)
		arena := NewArena(reg)
		if !arena.bindLexicalOwner(bodyID) {
			t.Fatal("bind lexical owner")
		}
		shape := Shape{Params: 2}
		firstRoot := arena.Root(Root{Kind: RootParam, Index: 0})
		secondRoot := arena.Root(Root{Kind: RootParam, Index: 1})
		arena.JoinValue(firstRoot, secondRoot)
		arena.Constant(product.Top())
		arena.Path(Root{Kind: RootParam, Index: 0})
		arena.Path(Root{Kind: RootParam, Index: 1})
		if arena.bindEnvironmentSymbol(1) == 0 {
			t.Fatal("bind symbolic Middle register")
		}
		if err := arena.sealMiddleRegisterSchema(); err != nil {
			t.Fatal(err)
		}
		if arena.middleSymbolPath(1) == 0 {
			t.Fatal("freeze symbolic Middle path")
		}
		effects := NewEffectArena(arena)
		code := &relationCode{
			terms: arena, effects: effects, descriptors: DefaultDescriptorRegistry(), shape: shape,
			nodes:         []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}},
			outcomes:      []boundaryOutcomeTuple{{}, {}},
			contributions: []semanticContribution{{}},
			root:          1,
		}
		// This fixture exercises terminal ownership rather than return lowering;
		// seal the closed term/code tables directly so an empty synthetic outcome
		// need not manufacture a fake N5 transaction.
		arena.Seal()
		effects.Seal()
		code.sealed = true
		keys := keyspace.New()
		program.bodies[index] = relationProgramBody{
			body: bodyID, variable: variable, keys: keys,
			relation:      Relation{shape: shape, arena: arena, effects: effects, descriptors: code.descriptors, code: code, root: 1},
			productDomain: productDomain,
		}
		program.byBody[bodyID] = variable
	}
	formalFibers, err := freezeFormalFiberInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalFibers = formalFibers
	return program
}

func formalComponentTestArena(t *testing.T, program *RelationProgram) *formalComponentTerminalArena {
	t.Helper()
	schema, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	arena, err := newFormalComponentTerminalArena(schema)
	if err != nil {
		t.Fatal(err)
	}
	return arena
}
