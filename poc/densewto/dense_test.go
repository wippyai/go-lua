package densewto

import (
	"testing"

	fixsummary "github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const (
	inputRoot symbol.ID = 8101
	flagRoot  symbol.ID = 8102
	tableRoot symbol.ID = 8103
	callRoot  symbol.ID = 8104
)

type fixture struct {
	graph       *cfg.CFG
	facts       factflow.Facts
	input       factflow.FactsInput
	resolver    *visibility.Resolver
	loop        NaturalLoop
	callPoint   cfg.Point
	resultPoint cfg.Point
}

type fixtureSources struct {
	reg      *axis.Registry
	facts    factflow.Facts
	resolver *visibility.Resolver
}

func (s fixtureSources) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	if source.Kind == factflow.ValueSourceCall && source.HasCallPoint {
		value := read(source.CallPoint).ReadReturnSlot(s.reg, source.ResultIndex)
		return value, !product.Equal(s.reg, value, product.Bottom(s.reg))
	}
	p, ok := s.facts.ExpressionPath(source.ExprRef)
	if !ok {
		return product.Value{}, false
	}
	value := in.ReadPathKey(s.reg, s.resolver.KeySpace(), s.resolver.KeyAt(point, p))
	return value, !product.Equal(s.reg, value, product.Bottom(s.reg))
}

func newFixture(reg *axis.Registry) fixture {
	g := cfg.New()
	seed := g.AddNode(cfg.NodeAssign)
	head := g.AddNode(cfg.NodeBranch)
	call := g.AddNode(cfg.NodeCall)
	result := g.AddNode(cfg.NodeAssign)
	implication := g.AddNode(cfg.NodeAssign)
	pathWrite := g.AddNode(cfg.NodeAssign)
	passes := make([]cfg.Point, 48)
	for i := range passes {
		passes[i] = g.AddNode(cfg.NodeAssign)
	}
	latch := g.AddNode(cfg.NodeAssign)
	after := g.AddNode(cfg.NodeAssign)
	g.AddEdge(g.Entry(), seed, false)
	g.AddEdge(seed, head, false)
	g.AddEdge(head, call, true)
	g.AddEdge(head, after, false)
	g.AddEdge(call, result, false)
	g.AddEdge(result, implication, false)
	g.AddEdge(implication, pathWrite, false)
	previous := pathWrite
	for _, pass := range passes {
		g.AddEdge(previous, pass, false)
		previous = pass
	}
	g.AddEdge(previous, latch, false)
	g.AddEdge(latch, head, false)
	g.AddEdge(after, g.Exit(), false)

	inputPath := pathdom.NewPath(inputRoot, "input")
	flagPath := pathdom.NewPath(flagRoot, "flag")
	tablePath := pathdom.NewPath(tableRoot, "box")
	callPath := pathdom.NewPath(callRoot, "answer")
	defs := []visibility.Definition{
		{Point: g.Entry(), Symbol: inputRoot, Root: inputPath.Root},
		{Point: g.Entry(), Symbol: flagRoot, Root: flagPath.Root},
		{Point: g.Entry(), Symbol: tableRoot, Root: tablePath.Root},
		{Point: g.Entry(), Symbol: callRoot, Root: callPath.Root},
	}
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{Graph: g, Definitions: defs}))
	exprInput := factflow.ExprRef(8101)
	exprCall := factflow.ExprRef(8102)
	expressionSource := func(ref factflow.ExprRef) factflow.ValueSource {
		return factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: ref, HasExpr: true}
	}
	callSource := factflow.ValueSource{Kind: factflow.ValueSourceCall, CallPoint: call, HasCallPoint: true, ResultIndex: 0}
	present := factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	input := factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			seed:   factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, flagRoot, flagPath, expressionSource(exprInput)),
			result: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, callRoot, callPath, callSource),
		},
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			pathWrite: factflow.NewPathAssignment(tablePath.Field("value"), expressionSource(exprCall)),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{exprInput: inputPath, exprCall: callPath},
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			head: factflow.NewBranchRefinementSet(factflow.NewBranchRefinement(flagPath, present, true, factflow.ValueRefinement{}, false)),
		},
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
			implication: factflow.NewPathValuePresenceImplicationSet(factflow.NewPathValuePresenceImplication(flagPath, typevalue.LiteralBool(reg, true), callPath, presence.Present())),
		},
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextAssignmentSource, Point: call, HasPoint: true,
				ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, 0, 0, callRoot, callPath)}}),
		},
	}
	body := []cfg.Point{call, result, implication, pathWrite}
	body = append(body, passes...)
	body = append(body, latch)
	return fixture{graph: g, input: input, facts: factflow.NewFacts(input), resolver: resolver, callPoint: call, resultPoint: result,
		loop: NaturalLoop{Prefix: []cfg.Point{g.Entry(), seed}, Head: head, Body: body, Suffix: []cfg.Point{after, g.Exit()}},
	}
}

func entryState(reg *axis.Registry) state.State {
	return state.State{}.
		WriteValue(reg, statekey.SymbolValue(inputRoot), typevalue.LiteralBool(reg, true)).
		WriteValue(reg, statekey.SymbolValue(tableRoot), product.Top())
}

func configs(t testing.TB, f fixture, reg *axis.Registry, lanes []state.LaneID, revision int) (transfer.Config, *Executor) {
	t.Helper()
	sources := fixtureSources{reg: reg, facts: f.facts, resolver: f.resolver}
	provided := typevalue.LiteralString(reg, "revision-a")
	if revision != 0 {
		provided = typevalue.LiteralString(reg, "revision-b")
	}
	provider := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		// This is the current summary-provider seam. It returns a payload and
		// never enters or solves the callee body.
		return callpayload.CallOutcome{PostReturnAuthority: true, Results: []callpayload.CallResult{{Index: 0, Value: provided}}}
	}
	node := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{Facts: f.facts, Sources: sources, CallOutcome: provider, Visibility: f.resolver})
	edge := factapply.NewFactsEdgeTransfer(factapply.FactsEdgeTransferConfig{Facts: f.facts, Sources: sources, CallOutcome: provider, Visibility: f.resolver})
	wto := solve.NewWTOPlan(f.graph.RPO(), func(p cfg.Point) []cfg.Point { return cfg.SuccessorsReadOnly(f.graph, p) })
	ordinary := transfer.Config{Graph: f.graph, Registry: reg, EntryState: entryState(reg), StateLanes: lanes, NodeTransfer: node, EdgeTransfer: edge,
		Schedule: transfer.ScheduleWTO, WTOPlan: wto, WidenAt: func(p cfg.Point) bool { return p == f.loop.Head }, WidenDelay: func(cfg.Point) int { return 1 }}
	exec, err := Compile(Config{Graph: f.graph, Registry: reg, Operations: operationplan.New(f.graph, f.input), Node: node, Edge: edge,
		EntryState: entryState(reg), StateLanes: lanes, WidenDelay: 1, Loop: f.loop})
	if err != nil {
		t.Fatal(err)
	}
	return ordinary, exec
}

func TestDenseWTOExactAtEveryPointExitSummaryAndLane(t *testing.T) {
	reg := standard.Registry()
	f := newFixture(reg)
	selections := [][]state.LaneID{nil}
	for _, lane := range state.DefaultLanes() {
		selections = append(selections, []state.LaneID{lane})
	}
	if len(state.DefaultLanes()) != 17 {
		t.Fatalf("default lanes=%d", len(state.DefaultLanes()))
	}
	for _, revision := range []int{0, 1} {
		for _, lanes := range selections {
			ordinary, direct := configs(t, f, reg, lanes, revision)
			want := transfer.Run(ordinary)
			got := direct.Run()
			domain := state.DomainWithOptionalLanes(reg, lanes)
			for p := cfg.Point(0); int(p) < f.graph.Size(); p++ {
				if !domain.Equal(want[p], got.Points[p]) {
					t.Fatalf("revision=%d lanes=%v point=%d differs", revision, lanes, p)
				}
			}
			wantSummary := fixsummary.Normalize(reg, fixsummary.Summary{Returns: []product.Value{want[f.graph.Exit()].ReadValue(reg, statekey.SymbolValue(callRoot))}})
			gotSummary := fixsummary.Normalize(reg, fixsummary.Summary{Returns: []product.Value{got.Points[f.graph.Exit()].ReadValue(reg, statekey.SymbolValue(callRoot))}})
			if !fixsummary.NormalizedDomain(reg).Equal(wantSummary, gotSummary) {
				t.Fatalf("revision=%d lanes=%v normalized summary differs", revision, lanes)
			}
		}
	}
}

func TestCallHoleDependencyRevisionChangesPublishedResult(t *testing.T) {
	reg := standard.Registry()
	f := newFixture(reg)
	_, first := configs(t, f, reg, nil, 0)
	_, second := configs(t, f, reg, nil, 1)
	firstValue := first.Run().Points[f.graph.Exit()].ReadValue(reg, statekey.SymbolValue(callRoot))
	secondValue := second.Run().Points[f.graph.Exit()].ReadValue(reg, statekey.SymbolValue(callRoot))
	if !product.Equal(reg, firstValue, typevalue.LiteralString(reg, "revision-a")) {
		t.Fatal("first summary-provider revision was not published")
	}
	if !product.Equal(reg, secondValue, typevalue.LiteralString(reg, "revision-b")) {
		t.Fatal("replacement summary-provider revision was not published")
	}
	if product.Equal(reg, firstValue, secondValue) {
		t.Fatal("dependency revision left a stale call result")
	}
}

func TestDenseWTORejectsIncompleteShapeAndHeapProvenanceExtension(t *testing.T) {
	reg := standard.Registry()
	f := newFixture(reg)
	bad := f.loop
	bad.Suffix = bad.Suffix[:1]
	if _, err := Compile(Config{Graph: f.graph, Registry: reg, Operations: operationplan.New(f.graph, f.input), Loop: bad}); err == nil {
		t.Fatal("incomplete WTO admitted")
	}
	// Heap provenance does not have a separate speculative handler here: all
	// admitted heap work remains inside the production point transaction.
	meta, ok := operationplan.Describe(operationplan.CallSite)
	if !ok || !meta.Stages.Has(operationplan.E5CallEffects) {
		t.Fatal("call heap/effect barrier is not owned by canonical operation plan")
	}
}
