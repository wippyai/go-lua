package transformer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalRootInputTemplateAttachesOnlyToLexicalRoot(t *testing.T) {
	program := formalRootInputTestProgram(t, standard.Registry())
	arena := program.bodies[0].relation.arena
	valueCount, pathCount, guardCount := len(arena.values), len(arena.paths), len(arena.guards)
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	if len(arena.values) != valueCount || len(arena.paths) != pathCount || len(arena.guards) != guardCount {
		t.Fatalf("root freeze grew sealed syntax: values %d/%d paths %d/%d guards %d/%d",
			len(arena.values), valueCount, len(arena.paths), pathCount, len(arena.guards), guardCount)
	}
	rootCell := program.formalRegion.roots[0]
	root, ok := template.equation(rootCell)
	if !ok || root.Operator.rootInput == nil || root.Operator.rootInput != &template.rootInputs[0] || !root.Operator.rootInput.valid() {
		t.Fatalf("lexical root input = %#v/%t", root.Operator.rootInput, ok)
	}
	other, ok := template.equation(formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode})
	if !ok || other.Operator.rootInput != nil {
		t.Fatalf("non-root operator acquired input template: %#v/%t", other.Operator.rootInput, ok)
	}

	input := root.Operator.rootInput
	if !input.care || input.program != program || input.variable != 1 {
		t.Fatalf("root authority = %#v", input)
	}
	if len(input.bindings) != program.bodies[0].relation.shape.InputCount() {
		t.Fatalf("binding count = %d", len(input.bindings))
	}
	for index, binding := range input.bindings {
		if !binding.valid(program.bodies[0].relation.arena, program.bodies[0].relation.shape) ||
			binding.middleValue == 0 || binding.inputValue == 0 || binding.middlePath == 0 || binding.inputPath == 0 {
			t.Fatalf("binding %d is not one atomic value/path identity: %#v", index, binding)
		}
	}
	if got, want := len(input.groups), formalRootInputExpectedGroupCount(program.bodies[0].productDomain); got != want {
		t.Fatalf("complete product input groups = %d, want %d", got, want)
	}
}

func TestFormalRootInputTemplateRejectsDuplicateAndForeignProductGroups(t *testing.T) {
	first := formalRootInputTestProgram(t, standard.Registry())
	second := formalRootInputTestProgram(t, standard.Registry())
	templates, err := freezeFormalRootInputTemplates(first)
	if err != nil || len(templates) != 1 || len(templates[0].groups) == 0 {
		t.Fatalf("freeze root inputs = %d/%v", len(templates), err)
	}

	duplicate := templates[0]
	duplicate.groups = append(append([]formalInputGroupRef(nil), duplicate.groups...), duplicate.groups[0])
	if duplicate.valid() {
		t.Fatal("root input accepted duplicate complete product group")
	}

	foreignTemplates, err := freezeFormalRootInputTemplates(second)
	if err != nil {
		t.Fatal(err)
	}
	foreign := templates[0]
	foreign.groups = append([]formalInputGroupRef(nil), foreign.groups...)
	foreign.groups[0] = foreignTemplates[0].groups[0]
	if foreign.valid() {
		t.Fatal("root input accepted another forest's same-shaped group")
	}

	body := &first.bodies[0]
	span, _ := first.formalFibers.span(1)
	if span.groupCount == 0 {
		t.Fatal("fixture has no dependent product group")
	}
	removed := first.formalFibers.groups[span.groupFirst]
	first.formalFibers.groups[span.groupFirst] = formalFiberGroupDescriptor{}
	if _, err := freezeFormalInputGroupRefs(first.formalFibers, body); err == nil {
		t.Fatal("group freeze accepted incomplete formal product inventory")
	}
	first.formalFibers.groups[span.groupFirst] = removed
	if span.groupCount > 1 {
		left := first.formalFibers.groups[span.groupFirst]
		right := first.formalFibers.groups[span.groupFirst+1]
		first.formalFibers.groups[span.groupFirst], first.formalFibers.groups[span.groupFirst+1] = right, left
		if _, err := freezeFormalInputGroupRefs(first.formalFibers, body); err == nil {
			t.Fatal("group freeze accepted ProductDomain lane-order drift")
		}
		first.formalFibers.groups[span.groupFirst], first.formalFibers.groups[span.groupFirst+1] = left, right
	}
}

func TestFormalRootInputTemplateTracksRegisteredCarrierInventoryAcrossAxisGrowth(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	extraAxis := axis.Spec[int]{
		Key:       axis.NewKey[int]("test.formal-root-input.extra"),
		Bottom:    func() int { return 0 },
		Top:       func() int { return 2 },
		Equal:     func(left, right int) bool { return left == right },
		LessOrEq:  func(left, right int) bool { return left <= right },
		Join:      func(left, right int) int { return max(left, right) },
		Meet:      func(left, right int) int { return min(left, right) },
		Hash:      func(value int) uint64 { return uint64(value) },
		Retention: axis.ImmutableRetention[int](),
		Canonical: axis.PendingCanonical[int]("test-only axis"),
		Boundary:  axis.PortableIdentity,
	}
	extendedRegistry, err := standard.RegistryWithAxes(extraAxis.Erase())
	if err != nil {
		t.Fatal(err)
	}
	extended := formalRootInputTestProgram(t, extendedRegistry)
	if extended.bodies[0].productDomain.Registry() != extendedRegistry {
		t.Fatal("extended fixture did not retain the added-axis registry")
	}
	baseRoots, err := freezeFormalRootInputTemplates(base)
	if err != nil {
		t.Fatal(err)
	}
	extendedRoots, err := freezeFormalRootInputTemplates(extended)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		name    string
		program *RelationProgram
		root    formalRootInputTemplate
	}{{"base", base, baseRoots[0]}, {"extended", extended, extendedRoots[0]}} {
		want := formalRootInputExpectedGroupCount(sample.program.bodies[0].productDomain)
		if len(sample.root.groups) != want {
			t.Fatalf("%s groups = %d, want registered inventory %d", sample.name, len(sample.root.groups), want)
		}
		seen := make(map[state.LaneOrdinal]struct{}, len(sample.root.groups))
		lanes := sample.program.bodies[0].productDomain.LaneInventory()
		for index, group := range sample.root.groups {
			if !group.valid() {
				t.Fatalf("%s has invalid group %#v", sample.name, group)
			}
			if index >= len(lanes) || group.lane != lanes[index].Ordinal() {
				t.Fatalf("%s group %d lane ordinal = %d, want ProductDomain order", sample.name, index, group.lane)
			}
			if _, duplicate := seen[group.lane]; duplicate {
				t.Fatalf("%s duplicates lane ordinal %d", sample.name, group.lane)
			}
			seen[group.lane] = struct{}{}
		}
	}
	// Sparse value axes are coordinates of the one complete Values carrier,
	// not solver lanes. Axis growth must therefore require no root-template
	// vocabulary change while the exact ProductDomain carrier census remains
	// authoritative above.
	if len(extendedRoots[0].groups) != len(baseRoots[0].groups) {
		t.Fatalf("one added value axis changed root carrier groups by %d", len(extendedRoots[0].groups)-len(baseRoots[0].groups))
	}
	baseValues, extendedValues := 0, 0
	for _, group := range baseRoots[0].groups {
		if group.kind == formalInputGroupValues {
			baseValues++
		}
	}
	for _, group := range extendedRoots[0].groups {
		if group.kind == formalInputGroupValues {
			extendedValues++
		}
	}
	if baseValues != 1 || extendedValues != 1 {
		t.Fatalf("Values carrier count across axis growth = %d/%d, want 1/1", baseValues, extendedValues)
	}
}

func TestFormalRootInputTemplateRejectsIncompleteBoundaryBindings(t *testing.T) {
	program := formalRootInputTestProgram(t, standard.Registry())
	arena := program.bodies[0].relation.arena
	arena.middle.entries = arena.middle.entries[:len(arena.middle.entries)-1]
	if _, err := freezeFormalRootInputTemplates(program); err == nil || !strings.Contains(err.Error(), "bindings cover") {
		t.Fatalf("incomplete binding freeze error = %v", err)
	}
}

func formalRootInputExpectedGroupCount(domain state.ProductDomain) int {
	return len(domain.LaneInventory())
}

func formalRootInputTestProgram(t *testing.T, registry *axis.Registry) *RelationProgram {
	t.Helper()
	if registry == nil {
		t.Fatal("nil registry")
	}
	const (
		param   symbol.ID = 101
		capture symbol.ID = 102
		global  symbol.ID = 103
		ambient symbol.ID = 104
	)
	bodyID := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	arena := NewArena(registry)
	if !arena.bindLexicalOwner(bodyID) {
		t.Fatal("bind lexical owner")
	}
	for _, id := range []symbol.ID{param, capture, global, ambient} {
		if arena.bindEnvironmentSymbol(id) == 0 {
			t.Fatalf("bind symbol %d", id)
		}
	}
	paramValue := arena.Root(Root{Kind: RootParam, Index: 0})
	captureValue := arena.Root(Root{Kind: RootCapture, Index: 0})
	globalValue := arena.Root(Root{Kind: RootGlobal, Index: 0})
	selectGuard := arena.Truthy(paramValue)
	if paramValue == 0 || captureValue == 0 || globalValue == 0 || selectGuard == 0 ||
		arena.Not(selectGuard) == 0 || arena.SelectValue(selectGuard, captureValue, globalValue) == 0 {
		t.Fatal("bind formal test correlated value")
	}
	if err := arena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	shape := Shape{Params: 1, Captures: 1, Globals: 1, Ambients: 1}
	entries := make([]relationMiddleEntry, 0, shape.InputCount())
	for _, item := range []struct {
		symbol symbol.ID
		root   Root
	}{{param, Root{Kind: RootParam}}, {capture, Root{Kind: RootCapture}}, {global, Root{Kind: RootGlobal}}, {ambient, Root{Kind: RootAmbient}}} {
		middle, ok := arena.middleRoot(key.SymbolValue(item.symbol))
		if !ok || arena.Root(middle) == 0 || arena.Path(middle) == 0 || arena.Root(item.root) == 0 || arena.Path(item.root) == 0 {
			t.Fatalf("bind input root %#v", item.root)
		}
		// Keep the exact complementary predicate vocabulary available to formal
		// executor tests after this shared arena is sealed.
		if truthy := arena.Truthy(arena.Root(item.root)); truthy == 0 || arena.Not(truthy) == 0 {
			t.Fatalf("bind input guard %#v", item.root)
		}
		entries = append(entries, relationMiddleEntry{middle: middle, input: item.root})
	}
	if err := arena.middle.bindInputs(shape, entries); err != nil {
		t.Fatal(err)
	}
	// Provide a small nested/sibling lexical loop vocabulary for formal
	// executor tests before the shared owner is permanently sealed.
	outer := arena.loopMu(10, 0, []cfg.Point{10, 11, 12}, []loopMuBackedge{{from: 12, to: 10}})
	inner := arena.loopMu(20, outer, []cfg.Point{20, 21, 22}, []loopMuBackedge{{from: 22, to: 20}})
	sibling := arena.loopMu(30, outer, []cfg.Point{30, 31, 32}, []loopMuBackedge{{from: 32, to: 30}})
	if outer == 0 || inner == 0 || sibling == 0 {
		t.Fatal("bind formal test loop vocabulary")
	}
	effects := NewEffectArena(arena)
	returnPlan, ok := factapply.PlanReturnTransactionSources(factflow.Facts{}, 1, nil)
	if !ok {
		t.Fatal("freeze zero-result N5 transaction")
	}
	code := &relationCode{
		terms: arena, effects: effects, descriptors: DefaultDescriptorRegistry(), shape: shape,
		nodes:    []relationNode{{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {returnTransaction: returnTransactionTerm{transaction: returnPlan}}}, contributions: []semanticContribution{{}}, root: 1, sealed: true,
		publication: relationPublicationPlan{points: []relationPointPublication{{point: 1, ref: 1}}},
	}
	arena.Seal()
	effects.Seal()
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	plan := operationplan.New(graph, factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{param}).
		WithBoundaryParamContracts([]product.Value{product.Top()}).
		WithBoundaryCaptures([]symbol.ID{capture}).
		WithBoundaryGlobals([]operationplan.BoundaryGlobal{{Symbol: global, Contract: product.Top()}})
	visibilityBuilder := visibility.NewBuilder()
	for _, id := range []symbol.ID{param, capture, global, ambient} {
		visibilityBuilder.Define(1, id, fmt.Sprintf("formal-root-%d", id))
	}
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	paths := factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	body := relationProgramBody{
		body: bodyID, variable: 1, keys: resolver.KeySpace(), plan: plan, graph: graph,
		relation:      Relation{shape: shape, arena: arena, effects: effects, descriptors: code.descriptors, code: code, root: 1},
		domain:        state.Domain(registry),
		productDomain: state.RegisteredProductDomain(registry),
		entrySeedPlan: state.NewEntrySeedPlan([]state.ValueSeed{
			{Slot: key.SymbolValue(param), Value: product.Top()},
			{Slot: key.SymbolValue(capture), Value: product.Top()},
			{Slot: key.SymbolValue(global), Value: product.Top()},
			{Slot: key.SymbolValue(ambient), Value: product.Top()},
		}),
		initialStatePlan: testInitialStatePlan(t, bodyID, graph),
		pathSemantics:    paths,
		returns:          factapply.NewReturnAuthority(paths, factflow.Facts{}),
	}
	program := &RelationProgram{registry: registry, bodies: []relationProgramBody{body}, byBody: map[lexicalidentity.StableLexicalBodyID]relationVar{bodyID: 1}}
	refreezeFormalTestStaticTopology(t, program)
	return program
}
