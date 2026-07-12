package rootguardeffects

import (
	"testing"

	fixsummary "github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const (
	sourceRoot   symbol.ID = 701
	fallbackRoot symbol.ID = 702
	tempRoot     symbol.ID = 703
	outputRoot   symbol.ID = 704
	resultRoot   symbol.ID = 705
)

type fixture struct {
	graph    *cfg.CFG
	input    factflow.FactsInput
	facts    factflow.Facts
	resolver *visibility.Resolver
	points   []cfg.Point
}

func newFixture(reg *axis.Registry) fixture {
	g := cfg.New()
	assign := g.AddNode(cfg.NodeAssign)
	branch := g.AddNode(cfg.NodeBranch)
	yes := g.AddNode(cfg.NodeAssign)
	no := g.AddNode(cfg.NodeAssign)
	join := g.AddNode(cfg.NodeJoin)
	// Ordinary local statements with no boundary-visible fact are retained in
	// the concrete CFG but disappear from the compiled boundary payload.
	pass1 := g.AddNode(cfg.NodeAssign)
	pass2 := g.AddNode(cfg.NodeAssign)
	pass3 := g.AddNode(cfg.NodeAssign)
	pass4 := g.AddNode(cfg.NodeAssign)
	pass5 := g.AddNode(cfg.NodeAssign)
	pass6 := g.AddNode(cfg.NodeAssign)
	final := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), assign, false)
	g.AddEdge(assign, branch, false)
	g.AddEdge(branch, yes, true)
	g.AddEdge(branch, no, false)
	g.AddEdge(yes, join, false)
	g.AddEdge(no, join, false)
	g.AddEdge(join, pass1, false)
	g.AddEdge(pass1, pass2, false)
	g.AddEdge(pass2, pass3, false)
	g.AddEdge(pass3, pass4, false)
	g.AddEdge(pass4, pass5, false)
	g.AddEdge(pass5, pass6, false)
	g.AddEdge(pass6, final, false)
	g.AddEdge(final, g.Exit(), false)

	paths := map[symbol.ID]pathdom.Path{
		sourceRoot: pathdom.NewPath(sourceRoot, "source"), fallbackRoot: pathdom.NewPath(fallbackRoot, "fallback"),
		tempRoot: pathdom.NewPath(tempRoot, "temp"), outputRoot: pathdom.NewPath(outputRoot, "output"), resultRoot: pathdom.NewPath(resultRoot, "result"),
	}
	defs := make([]visibility.Definition, 0, len(paths))
	for id, path := range paths {
		defs = append(defs, visibility.Definition{Point: g.Entry(), Symbol: id, Root: path.Root})
	}
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{Graph: g, Definitions: defs}))
	source := func(ref factflow.ExprRef) factflow.ValueSource {
		return factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: ref, HasExpr: true}
	}
	present := factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	absent := factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Absent()))
	input := factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, tempRoot, paths[tempRoot], source(1)),
			yes:    factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, outputRoot, paths[outputRoot], source(2)),
			no:     factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, outputRoot, paths[outputRoot], source(3)),
			final:  factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, resultRoot, paths[resultRoot], source(4)),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{1: paths[sourceRoot], 2: paths[tempRoot], 3: paths[fallbackRoot], 4: paths[outputRoot]},
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{branch: factflow.NewBranchRefinementSet(
			factflow.NewBranchRefinement(paths[tempRoot], present, true, absent, true),
		)},
	}
	return fixture{graph: g, input: input, facts: factflow.NewFacts(input), resolver: resolver, points: g.RPO()}
}

type fixtureSources struct {
	reg      *axis.Registry
	resolver *visibility.Resolver
	facts    factflow.Facts
}

func (s fixtureSources) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	path, ok := s.facts.ExpressionPath(source.ExprRef)
	if !ok {
		return product.Value{}, false
	}
	current := in
	if read != nil {
		current = read(point)
	}
	value := current.ReadPathKey(s.reg, s.resolver.KeySpace(), s.resolver.KeyAt(point, path))
	return value, !product.Equal(s.reg, value, product.Bottom(s.reg))
}

func solveFixture(f fixture, reg *axis.Registry, entry state.State, lanes []state.LaneID) transfer.Result {
	sources := fixtureSources{reg, f.resolver, f.facts}
	return transfer.Run(transfer.Config{
		Graph: f.graph, Registry: reg, EntryState: entry, StateLanes: lanes,
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{Facts: f.facts, Sources: sources, Visibility: f.resolver}),
		EdgeTransfer: factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{Facts: f.facts, Sources: sources, Visibility: f.resolver}),
	})
}

func fixtureEntry(reg *axis.Registry) state.State {
	maybe := product.Join(reg, typevalue.LiteralString(reg, "payload"), typevalue.Nil(reg))
	return state.State{}.WriteValue(reg, statekey.SymbolValue(sourceRoot), maybe).WriteValue(reg, statekey.SymbolValue(fallbackRoot), typevalue.LiteralString(reg, "fallback"))
}

func TestPlanMatchesConcreteAtEveryPointExitAndSummaryAcrossAllLanes(t *testing.T) {
	reg := standard.Registry()
	f := newFixture(reg)
	plan, err := Compile(f.graph, f.input, resultRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Admission(); got.RootAssignments != 4 || got.GuardRefinements != 2 || got.Points != len(f.points) {
		t.Fatalf("admission=%+v", got)
	}
	if len(state.DefaultLanes()) != 17 {
		t.Fatalf("default lane count=%d", len(state.DefaultLanes()))
	}
	observe := ObservationSet{}
	for _, point := range f.points {
		observe[point] = struct{}{}
	}
	for _, selection := range append([][]state.LaneID{nil}, singletonLanes()...) {
		domain := state.Domain(reg)
		if selection != nil {
			domain = state.DomainWithLanes(reg, selection)
		}
		want := solveFixture(f, reg, fixtureEntry(reg), selection)
		got, err := plan.Execute(Config{Registry: reg, Resolver: f.resolver, Entry: fixtureEntry(reg), StateLanes: selection}, observe)
		if err != nil {
			t.Fatalf("lanes=%v: %v", selection, err)
		}
		for _, point := range f.points {
			if !domain.Equal(want[point], got.Points[point]) {
				t.Fatalf("lanes=%v point=%d differs", selection, point)
			}
		}
		if !domain.Equal(want[f.graph.Exit()], got.Exit) {
			t.Fatalf("lanes=%v exit differs", selection)
		}
		wantReturn := want[f.graph.Exit()].ReadValue(reg, statekey.SymbolValue(resultRoot))
		wantSummary := fixsummary.Normalize(reg, fixsummary.Summary{Returns: []product.Value{wantReturn}})
		if !fixsummary.NormalizedDomain(reg).Equal(wantSummary, got.Summary) {
			t.Fatalf("lanes=%v summary differs", selection)
		}
	}
}

func singletonLanes() [][]state.LaneID {
	out := make([][]state.LaneID, 0, len(state.DefaultLanes()))
	for _, lane := range state.DefaultLanes() {
		out = append(out, []state.LaneID{lane})
	}
	return out
}

func TestCompileFailsClosedOnObjectAndCallSidecars(t *testing.T) {
	reg := standard.Registry()
	f := newFixture(reg)
	mutated := f.input
	mutated.ObjectLiterals = map[factflow.ExprRef]factflow.ObjectLiteral{1: {}}
	if _, err := Compile(f.graph, mutated, resultRoot); err == nil {
		t.Fatal("object sidecar admitted")
	}
	mutated = f.input
	mutated.CallSites = map[cfg.Point]factflow.CallSite{f.points[1]: {}}
	if _, err := Compile(f.graph, mutated, resultRoot); err == nil {
		t.Fatal("call sidecar admitted")
	}
}

func TestCompileFailsClosedOnCycle(t *testing.T) {
	reg := standard.Registry()
	f := newFixture(reg)
	f.graph.AddEdge(f.points[len(f.points)-2], f.points[1], false)
	if _, err := Compile(f.graph, f.input, resultRoot); err == nil {
		t.Fatal("cycle admitted")
	}
}
