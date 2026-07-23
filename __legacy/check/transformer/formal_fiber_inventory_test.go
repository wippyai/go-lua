package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type formalFiberTestSignature struct {
	role       formalFiberRole
	root       uint64
	vocabulary uint8
	lane       state.LaneID
	family     state.CoordinateFamilyID
	outcome    boundaryOutcomeRef
}

type formalFiberTestGroupSignature struct {
	kind     formalFiberGroupKind
	lane     state.LaneID
	members  []formalFiberOrdinal
	families []state.CoordinateFamilyID
}

func formalFiberTestGroupSignatures(span formalFiberDescriptorSpan) []formalFiberTestGroupSignature {
	groups := span.groupDescriptors()
	out := make([]formalFiberTestGroupSignature, len(groups))
	for index, group := range groups {
		out[index] = formalFiberTestGroupSignature{kind: group.kind, lane: group.lane.ID(), members: append([]formalFiberOrdinal(nil), group.members...)}
		for _, family := range group.coordinateFamilies {
			out[index].families = append(out[index].families, family.family.ID())
		}
	}
	return out
}

func formalFiberTestSignatures(span formalFiberDescriptorSpan) []formalFiberTestSignature {
	descriptors := span.descriptors()
	out := make([]formalFiberTestSignature, len(descriptors))
	for index, descriptor := range descriptors {
		out[index] = formalFiberTestSignature{
			role: descriptor.role, lane: descriptor.lane.ID(), family: descriptor.family.ID(), outcome: descriptor.outcome,
		}
		if root, ok := descriptor.slot.Root(); ok {
			out[index].root = root.Ordinal()
			out[index].vocabulary = uint8(root.Vocabulary())
		}
	}
	return out
}

func TestFormalFiberInventoryFreezesDeterministicTypedOrder(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-fiber-order")))
	first := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, false)
	second := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, false)
	left, ok := first.span(1)
	if !ok {
		t.Fatal("first inventory has no relation span")
	}
	right, ok := second.span(1)
	if !ok {
		t.Fatal("second inventory has no relation span")
	}
	if !reflect.DeepEqual(formalFiberTestSignatures(left), formalFiberTestSignatures(right)) {
		t.Fatal("equivalent forests froze different formal fiber descriptor order")
	}
	if !reflect.DeepEqual(formalFiberTestGroupSignatures(left), formalFiberTestGroupSignatures(right)) {
		t.Fatal("equivalent forests froze different formal fiber group order")
	}
	descriptors := left.descriptors()
	if len(descriptors) == 0 || descriptors[0].role != formalFiberCare {
		t.Fatal("formal care is not the first canonical fiber")
	}
	for index := 1; index < len(descriptors); index++ {
		if descriptors[index-1].role > descriptors[index].role {
			t.Fatalf("fiber role order decreases at %d: %d > %d", index, descriptors[index-1].role, descriptors[index].role)
		}
	}
	for index, descriptor := range descriptors {
		ordinal := formalFiberOrdinal(index)
		group, grouped := left.groupForOrdinal(ordinal)
		shouldBeGrouped := descriptor.role == formalFiberOrdinaryLane || descriptor.role == formalFiberCoordinate || descriptor.role == formalFiberGroundValueTop || descriptor.role == formalFiberGroundValue
		if grouped != shouldBeGrouped {
			t.Fatalf("descriptor %d grouped=%t, want %t", index, grouped, shouldBeGrouped)
		}
		if grouped {
			member, ok := group.member(ordinal)
			if !ok {
				t.Fatalf("descriptor %d has no group-owned member capability", index)
			}
			if address, ok := member.address(group); !ok || address != ordinal {
				t.Fatalf("descriptor %d group capability did not lower exactly", index)
			}
		}
	}
	groups := left.groupDescriptors()
	lanes := firstSpanProductDomain(t, first).LaneInventory()
	if len(groups) != len(lanes) {
		t.Fatalf("formal group count = %d, registered ProductLane count = %d", len(groups), len(lanes))
	}
	for index, lane := range lanes {
		if groups[index].lane != lane {
			t.Fatalf("formal group %d lane = %q/%d, want %q/%d", index, groups[index].lane.ID(), groups[index].lane.Ordinal(), lane.ID(), lane.Ordinal())
		}
	}
	arena, err := newFormalFiberDirectoryArena(left.count)
	if err != nil || arena.fiberCount() != len(descriptors) {
		t.Fatalf("run directory width = %d, descriptors = %d, err=%v", arena.fiberCount(), len(descriptors), err)
	}
}

func firstSpanProductDomain(t *testing.T, inventory *formalFiberInventory) state.ProductDomain {
	t.Helper()
	if inventory == nil || len(inventory.spans) == 0 {
		t.Fatal("formal inventory has no first span")
	}
	// The test fixture always uses the standard registered domain; equality of
	// opaque lane descriptors below additionally proves the retained inventory
	// was not reconstructed from lane names.
	return state.RegisteredProductDomain(standard.Registry())
}

func TestFormalFiberInventoryOwnsOneArenaPerBodyAndRejectsCrossBody(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-fiber-ownership"))
	inventory := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{
		lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1),
	}, false)
	first, firstOK := inventory.span(1)
	second, secondOK := inventory.span(2)
	if !firstOK || !secondOK {
		t.Fatal("relation bodies did not receive distinct descriptor spans")
	}
	firstDescriptors := first.descriptors()
	if len(firstDescriptors) == 0 {
		t.Fatal("first relation has no formal descriptors")
	}
	if _, accepted := second.ordinal(firstDescriptors[0]); accepted {
		t.Fatal("second relation accepted first relation's descriptor")
	}
	firstArena, err := newFormalFiberDirectoryArena(first.count)
	if err != nil {
		t.Fatal(err)
	}
	secondArena, err := newFormalFiberDirectoryArena(second.count)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondArena.validateRoot(firstArena.defaultRoot()); err == nil {
		t.Fatal("second relation accepted first relation's directory root")
	}
}

func TestFormalFiberInventoryDoesNotGrowForDynamicReadSyntax(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-fiber-dynamic-read")))
	plain := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, false)
	dynamic := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, true)
	plainSpan, _ := plain.span(1)
	dynamicSpan, _ := dynamic.span(1)
	if !reflect.DeepEqual(formalFiberTestSignatures(plainSpan), formalFiberTestSignatures(dynamicSpan)) {
		t.Fatal("sealed DynamicRead syntax grew or changed the static formal fiber universe")
	}
}

func TestFormalFiberInventoryDoesNotStoreEffectOccurrences(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-fiber-effect-syntax")))
	one := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, false, 1)
	two := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, false, 2)
	oneSpan, _ := one.span(1)
	twoSpan, _ := two.span(1)
	if !reflect.DeepEqual(formalFiberTestSignatures(oneSpan), formalFiberTestSignatures(twoSpan)) {
		t.Fatal("repeated sealed effect occurrences grew or changed the formal tuple storage universe")
	}
}

func TestFormalFiberInventoryValuesGroupOwnsExactValueFactor(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-values-group"))
	inventory := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{
		lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1),
	}, false)
	span, _ := inventory.span(1)
	group, ok := span.valuesGroup()
	if !ok || !group.valid() {
		t.Fatal("Values has no typed whole-factor group")
	}
	domain, ok := group.lattice()
	if !ok || !domain.Equal(domain.Bottom(), state.ValueFactor[FormalSlot]{}) || !domain.Equal(domain.Top(), state.ValueFactor[FormalSlot]{Top: true}) {
		t.Fatal("Values group does not retain the exact state.ValueFactor[FormalSlot] lattice")
	}
	top, ok := group.top()
	if !ok {
		t.Fatal("Values group has no group-owned Top member")
	}
	topOrdinal, _ := top.address(group.descriptor)
	descriptors := span.descriptors()
	if descriptors[topOrdinal].role != formalFiberGroundValueTop {
		t.Fatal("Values Top capability addresses a non-Top descriptor")
	}
	wantMembers := 1
	factor := state.ValueFactor[FormalSlot]{Values: make(map[FormalSlot]product.Value)}
	for ordinal, descriptor := range descriptors {
		if descriptor.role != formalFiberGroundValue {
			continue
		}
		wantMembers++
		member, present := group.slot(descriptor.slot)
		if !present {
			t.Fatalf("Values group omitted FormalSlot at descriptor %d", ordinal)
		}
		if address, accepted := member.address(group.descriptor); !accepted || address != formalFiberOrdinal(ordinal) {
			t.Fatalf("Values FormalSlot at descriptor %d has wrong group address", ordinal)
		}
		factor.Values[descriptor.slot] = product.Bottom(standard.Registry())
	}
	if len(group.descriptor.members) != wantMembers || !group.owns(factor) {
		t.Fatalf("Values group owns %d members, want %d, factor-owned=%t", len(group.descriptor.members), wantMembers, group.owns(factor))
	}
	if group.owns(state.ValueFactor[FormalSlot]{Top: true, Values: factor.Values}) {
		t.Fatal("Values group accepted a noncanonical Top with finite members")
	}
	foreignSpan, _ := inventory.span(2)
	foreignDescriptors := foreignSpan.descriptors()
	for _, descriptor := range foreignDescriptors {
		if descriptor.role == formalFiberGroundValue {
			if _, accepted := group.slot(descriptor.slot); accepted {
				t.Fatal("Values group accepted another body's FormalSlot")
			}
			break
		}
	}
}

func TestFormalFiberInventoryCoordinateGroupsCoverEveryRegisteredFamilyOnce(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-coordinate-group-census")))
	inventory := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{body}, false)
	span, _ := inventory.span(1)
	descriptors := span.descriptors()
	seen := make(map[formalFiberOrdinal]struct{})
	for _, groupDescriptor := range span.groupDescriptors() {
		if groupDescriptor.kind != formalFiberGroupCoordinateLane {
			continue
		}
		group := formalCoordinateLaneFiberGroup{descriptor: groupDescriptor}
		lane, ok := group.lane()
		if !ok {
			t.Fatal("coordinate group has no owned ProductLane")
		}
		families := group.families()
		if len(families) == 0 {
			t.Fatal("coordinate group contains no registered families")
		}
		for _, family := range families {
			if descriptors[family.skeleton].lane != lane || descriptors[family.skeleton].family != family.family || descriptors[family.skeleton].coordinateKind != formalFiberCoordinateFamilySkeleton {
				t.Fatalf("family %q skeleton metadata is not exact", family.family.ID())
			}
			if family.skeletonPosition < 0 || groupDescriptor.members[family.skeletonPosition] != family.skeleton {
				t.Fatalf("family %q has no frozen skeleton position", family.family.ID())
			}
			ordinals := append([]formalFiberOrdinal{family.skeleton}, family.scalars...)
			positions := append([]int{family.skeletonPosition}, family.scalarPositions...)
			if len(ordinals) != len(positions) {
				t.Fatalf("family %q scalar position metadata is incomplete", family.family.ID())
			}
			for index, ordinal := range ordinals {
				if _, duplicate := seen[ordinal]; duplicate {
					t.Fatalf("coordinate ordinal %d belongs to multiple lane groups", ordinal)
				}
				seen[ordinal] = struct{}{}
				if positions[index] < 0 || groupDescriptor.members[positions[index]] != ordinal {
					t.Fatalf("coordinate ordinal %d has wrong dense group position", ordinal)
				}
			}
		}
	}
	for ordinal, descriptor := range descriptors {
		_, covered := seen[formalFiberOrdinal(ordinal)]
		if (descriptor.role == formalFiberCoordinate) != covered {
			t.Fatalf("coordinate descriptor %d coverage=%t", ordinal, covered)
		}
	}
}

func TestFormalCoordinateLaneGroupSupportsMultipleFamiliesWithoutContiguity(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	var families []state.CoordinateFamily
	for _, candidateLane := range domain.NonValuesLaneInventory() {
		candidateFamilies, err := domain.CoordinateFamilies(candidateLane)
		if err != nil {
			t.Fatal(err)
		}
		for _, family := range candidateFamilies {
			families = append(families, family)
			if len(families) == 2 {
				break
			}
		}
		if len(families) == 2 {
			break
		}
	}
	if len(families) != 2 {
		t.Fatal("test registry has fewer than two coordinate families")
	}
	lane := families[0].Lane()
	descriptors := []formalFiberDescriptor{
		{role: formalFiberCoordinate, lane: lane, family: families[0], coordinateKind: formalFiberCoordinateFamilySkeleton},
		{role: formalFiberCoordinate, lane: lane, family: families[1], coordinateKind: formalFiberCoordinateFamilySkeleton},
		{role: formalFiberCoordinate, lane: lane, family: families[0], coordinateKind: formalFiberCoordinateFamilyScalar},
		{role: formalFiberCoordinate, lane: lane, family: families[1], coordinateKind: formalFiberCoordinateFamilyScalar},
	}
	group, err := freezeFormalCoordinateLaneFiberGroup(lane, families, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.coordinateFamilies) != 2 || !reflect.DeepEqual(group.members, []formalFiberOrdinal{0, 2, 1, 3}) {
		t.Fatalf("multi-family lane group = %#v, want explicit noncontiguous [0 2 1 3]", group.members)
	}
	for index, family := range group.coordinateFamilies {
		if family.skeletonPosition != index*2 || !reflect.DeepEqual(family.scalarPositions, []int{index*2 + 1}) {
			t.Fatalf("family %d dense positions = skeleton %d, scalars %v", index, family.skeletonPosition, family.scalarPositions)
		}
	}
}

func TestFormalFiberGroupCapabilitiesRejectForeignDescriptors(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-group-foreign"))
	inventory := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{
		lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1),
	}, false)
	first, _ := inventory.span(1)
	second, _ := inventory.span(2)
	firstValues, _ := first.valuesGroup()
	secondValues, _ := second.valuesGroup()
	member, _ := firstValues.top()
	if _, accepted := member.address(secondValues.descriptor); accepted {
		t.Fatal("another body's same-shaped Values group accepted a foreign member capability")
	}
	if _, accepted := second.groupForOrdinal(member.ordinal); !accepted {
		// A local same-numbered ordinal may exist, but it must resolve to the
		// second body's descriptor rather than authenticate the foreign member.
		return
	}
	if local, _ := second.groupForOrdinal(member.ordinal); local.same(firstValues.descriptor) {
		t.Fatal("foreign ordinal lookup returned the first body's group")
	}
}

func testFormalFiberInventory(t *testing.T, bodies []lexicalidentity.StableLexicalBodyID, dynamic bool, effectOccurrences ...int) *formalFiberInventory {
	t.Helper()
	reg := standard.Registry()
	program := &RelationProgram{registry: reg, bodies: make([]relationProgramBody, len(bodies)), byBody: make(map[lexicalidentity.StableLexicalBodyID]relationVar, len(bodies))}
	for index, bodyID := range bodies {
		variable := relationVar(index + 1)
		arena := NewArena(reg)
		if !arena.bindLexicalOwner(bodyID) {
			t.Fatal("bind lexical owner")
		}
		// Bind the first durable parameter spelling into Middle so the registry
		// must retain one lexical class across Input and resolver-Middle roots.
		if arena.bindEnvironmentSymbol(symbol.ID(200+index*10)) == 0 {
			t.Fatal("bind Middle register")
		}
		if err := arena.sealMiddleRegisterSchema(); err != nil {
			t.Fatal(err)
		}
		shape := Shape{Params: 2}
		left := arena.Root(Root{Kind: RootParam, Index: 0})
		right := arena.Root(Root{Kind: RootParam, Index: 1})
		condition := left
		if dynamic {
			condition = arena.DynamicReadValue(left, arena.Path(Root{Kind: RootParam, Index: 0}), right)
			if condition == 0 {
				t.Fatal("seal DynamicRead syntax")
			}
		}
		effects := NewEffectArena(arena)
		nodes := []relationNode{
			{}, {kind: relationNodeChoice, guard: arena.Truthy(condition), whenTrue: 2, whenFalse: 3},
			{kind: relationNodeBottom}, {kind: relationNodeNonreturning},
		}
		if len(effectOccurrences) != 0 && effectOccurrences[0] > 0 {
			target := arena.Path(Root{Kind: RootParam, Index: 0})
			effect, effectErr := effects.PathStore(PathStoreConfig{
				Target: target, Value: left, Site: EffectSite{Owner: 1, Ordinal: 1},
			})
			if effectErr != nil {
				t.Fatal(effectErr)
			}
			steps := make([]boundaryStep, effectOccurrences[0])
			for occurrence := range steps {
				steps[occurrence] = boundaryStep{kind: boundaryStepEffect, effect: effect}
			}
			nodes = []relationNode{
				{}, {kind: relationNodeSequence, steps: steps, next: 2},
				{kind: relationNodeChoice, guard: arena.Truthy(condition), whenTrue: 3, whenFalse: 4},
				{kind: relationNodeBottom}, {kind: relationNodeNonreturning},
			}
		}
		code, root, err := sealRelationCode(&relationCode{
			terms: arena, effects: effects, descriptors: DefaultDescriptorRegistry(), shape: shape,
			nodes:    nodes,
			outcomes: []boundaryOutcomeTuple{{}}, contributions: []semanticContribution{{}},
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		keys := keyspace.New()
		roots := relationRootCarrier{shape: shape}
		for ordinal := uint32(0); ordinal < shape.Params; ordinal++ {
			id := symbol.ID(200 + index*10 + int(ordinal))
			roots.roots = append(roots.roots, relationStateRoot{
				root: Root{Kind: RootParam, Index: ordinal},
				slot: statekey.SymbolValue(id),
				path: keys.FromPath(pathdom.NewPath(id, "")),
			})
		}
		program.bodies[index] = relationProgramBody{
			body: bodyID, variable: variable, keys: keys, roots: roots,
			relation:      Relation{shape: shape, arena: arena, effects: code.effects, descriptors: code.descriptors, code: code, root: root},
			productDomain: state.RegisteredProductDomain(reg),
		}
		program.byBody[bodyID] = variable
	}
	inventory, err := freezeFormalFiberInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func TestFormalCoordinateRegistryTagsInputAndMiddleOfOneBinding(t *testing.T) {
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	inventory := testFormalFiberInventory(t, []lexicalidentity.StableLexicalBodyID{owner}, false)
	if len(inventory.coordinateRegistries) != 1 {
		t.Fatalf("registry count = %d, want 1", len(inventory.coordinateRegistries))
	}
	registry := inventory.coordinateRegistries[0]
	var paired bool
	for _, members := range registry.members {
		var input, middle bool
		for _, root := range members {
			input = input || root.Vocabulary() == formal.Input
			middle = middle || root.Vocabulary() == formal.Middle
		}
		paired = paired || input && middle
	}
	if !paired {
		t.Fatal("Input and Middle spellings of one lexical binding did not share a class")
	}
}
