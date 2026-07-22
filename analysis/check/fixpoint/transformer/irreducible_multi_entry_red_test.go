package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// irreducibleEntryFixture is the smallest multi-entry cyclic region that can
// observe whether control entered at B rather than at H:
//
//	E -> H -> B -> H
//	|         |
//	+-------> B -> return
//
// H overwrites marker.  Therefore the E -> B -> return route must still be
// able to publish the entry marker, while routes through H may publish the
// overwritten marker.  Treating H as the region's sole entry silently loses
// the former behavior even if the frozen edge inventory still contains E->B.
func irreducibleEntryFixture() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	graph := cfg.New()
	entryChoice := graph.AddNode(cfg.NodeBranch)
	head := graph.AddNode(cfg.NodeAssign)
	bodyChoice := graph.AddNode(cfg.NodeBranch)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), entryChoice, false)
	graph.AddEdge(entryChoice, head, true)
	graph.AddEdge(entryChoice, bodyChoice, false)
	graph.AddEdge(head, bodyChoice, false)
	graph.AddEdge(bodyChoice, head, true)
	graph.AddEdge(bodyChoice, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	return graph, entryChoice, head, bodyChoice, ret
}

func TestIrreducibleRegionRetainsBothCompiledEntryTargets(t *testing.T) {
	graph, entryChoice, head, bodyChoice, _ := irreducibleEntryFixture()
	tape := mustCompileSymbolicWTOTape(t, graph)
	terms := NewArena(standard.Registry())
	freezer, err := newWorldProgramFreezer(terms, NewEffectArena(terms), Shape{}, len(tape.points)+len(tape.components))
	if err != nil {
		t.Fatal(err)
	}
	topology, err := newStructuralProgramTopology(tape, freezer)
	if err != nil {
		t.Fatal(err)
	}

	targets := make(map[cfg.Point]programRef, 2)
	entryDense := tape.denseIndex(entryChoice)
	for _, edge := range tape.edges[tape.points[entryDense].edgeBegin:tape.points[entryDense].edgeEnd] {
		point := tape.points[edge.to].point
		if point != head && point != bodyChoice {
			continue
		}
		target, targetErr := topology.edgeTarget(edge)
		if targetErr != nil {
			t.Fatal(targetErr)
		}
		targets[point] = target
	}
	if len(targets) != 2 {
		t.Fatalf("compiled irreducible entries = %v, want {%d,%d}", targets, head, bodyChoice)
	}
	if targets[head] == targets[bodyChoice] {
		t.Fatalf("compiled irreducible entries H=%d and B=%d collapse to one target %d; edge inventory alone does not preserve multi-entry semantics", head, bodyChoice, targets[head])
	}
}

func TestIrreducibleRegionDirectBodyEntryPreservesPreHeadOutcome(t *testing.T) {
	reg := standard.Registry()
	graph, entryChoice, head, bodyChoice, ret := irreducibleEntryFixture()
	entryConditionID := symbol.ID(1901)
	bodyConditionID := symbol.ID(1902)
	markerID := symbol.ID(1903)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar source shape rejected")
	}
	pathSource := func(id symbol.ID, name string) factflow.ValueSource {
		t.Helper()
		source, exact := factflow.NewPathValueSource(pathdom.NewPath(id, name).Key(), 0, 0, 0, shape)
		if !exact {
			t.Fatalf("path source %s rejected", name)
		}
		return source
	}
	entryCondition, ok := factflow.NewBranchCondition(pathSource(entryConditionID, "entry-condition"), true)
	if !ok {
		t.Fatal("entry condition rejected")
	}
	bodyCondition, ok := factflow.NewBranchCondition(pathSource(bodyConditionID, "body-condition"), true)
	if !ok {
		t.Fatal("body condition rejected")
	}
	afterHeadSource, ok := factflow.NewStringLiteralValueSource("after-H", 0, 0, 0, shape)
	if !ok {
		t.Fatal("head assignment source rejected")
	}
	markerSource := pathSource(markerID, "marker")
	facts := factflow.FactsInput{
		BranchConditionSources: map[cfg.Point]factflow.BranchCondition{
			entryChoice: entryCondition,
			bodyChoice:  bodyCondition,
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			head: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, markerID, pathdom.NewPath(markerID, "marker"), afterHeadSource),
		},
		Returns: map[cfg.Point]factflow.Return{
			ret: factflow.NewReturn([]factflow.ValueSource{markerSource}),
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	for _, point := range cfg.RPOReadOnly(graph) {
		visibilityBuilder.Define(point, entryConditionID, "entry-condition")
		visibilityBuilder.Define(point, bodyConditionID, "body-condition")
		visibilityBuilder.Define(point, markerID, "marker")
	}
	program, bodyID := freezeFormalRootAssignmentTestProgram(t, reg, graph, facts, visibility.NewResolver(visibilityBuilder.Build()), []symbol.ID{entryConditionID, bodyConditionID, markerID})
	entryMarker := typevalue.LiteralString(reg, "before-H")
	entry := state.Domain(reg).Bottom().
		WriteValue(reg, key.SymbolValue(entryConditionID), product.Top()).
		WriteValue(reg, key.SymbolValue(bodyConditionID), product.Top()).
		WriteValue(reg, key.SymbolValue(markerID), entryMarker)
	view, err := program.Solve(t.Context(), bodyID, entry)
	if err != nil {
		t.Fatal(err)
	}
	var publication FormalLexicalBodyCoordinates
	foundBody := false
	for _, candidate := range view.LexicalBodies() {
		if candidate.Body == bodyID {
			publication, foundBody = candidate, true
			break
		}
	}
	if !foundBody {
		t.Fatal("formal irreducible solve has no lexical publication")
	}
	published, ok := publication.PlannedNodeOutputs[ret]
	if !ok || !publication.NodeOutputReachable[ret] {
		t.Fatal("irreducible return has no reachable formal publication")
	}
	if !product.LessOrEq(reg, entryMarker, published.ReadValue(reg, key.ReturnSlot(0))) {
		t.Fatal("E -> B lost the pre-H return outcome; the irreducible region was entered only through H")
	}

	symbolic, err := executeFormalRelation(t.Context(), program)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := freezeFormalRootEntrySeed(program, bodyID, entry)
	if err != nil {
		t.Fatal(err)
	}
	substitution, err := newFormalRootEntrySubstitution(seed)
	if err != nil {
		t.Fatal(err)
	}
	specialized, err := substitution.specializeStabilized(t.Context(), symbolic)
	if err != nil {
		t.Fatal(err)
	}
	specializedPublication, err := specialized.Publication(bodyID)
	if err != nil {
		t.Fatal(err)
	}
	specializedReturn, present, err := specializedPublication.PlannedNodeOutput(t.Context(), ret, 0)
	if err != nil || !present || !product.Equal(reg, published.ReadValue(reg, key.ReturnSlot(0)), specializedReturn.ReadValue(reg, key.ReturnSlot(0))) {
		t.Fatalf("symbolic irreducible return = present:%t err:%v equal:%t", present, err, product.Equal(reg, published.ReadValue(reg, key.ReturnSlot(0)), specializedReturn.ReadValue(reg, key.ReturnSlot(0))))
	}
}
