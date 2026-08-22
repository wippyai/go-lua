package linkexecutionplan

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func planLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("link-execution-plan-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

type planLawDirectory struct {
	directory executioncontext.Directory
	modules   []identity.ContentID
}

func newPlanLawDirectory(t *testing.T, labels ...string) planLawDirectory {
	t.Helper()
	link := planLawID(t, "link")
	contexts := make([]executioncontext.Context, 0, len(labels))
	roots := make([]executioncontext.RootContext, 0, len(labels))
	modules := make([]identity.ContentID, 0, len(labels))
	for index, label := range labels {
		module := planLawID(t, "module/"+label)
		suffix := string(rune('a' + index))
		context, contextOK := executioncontext.NewContext(link, module, planLawID(t, "actor/"+label+"/"+suffix), planLawID(t, "representative/"+label+"/"+suffix))
		root, rootOK := executioncontext.NewRootContext(link, planLawID(t, "root/"+label+"/"+suffix), context.ID())
		if !contextOK || !rootOK {
			t.Fatalf("context %s", label)
		}
		contexts = append(contexts, context)
		roots = append(roots, root)
		modules = append(modules, module)
	}
	directory, directoryOK := executioncontext.Seal(link, contexts, roots, nil)
	if !directoryOK {
		t.Fatal("seal directory")
	}
	return planLawDirectory{directory: directory, modules: modules}
}

func newPlanLawLayout(t *testing.T, graph *equation.Graph, directory executioncontext.Directory, owners []contextfiber.PointOwner) contextfiber.Layout {
	t.Helper()
	index, indexOK := contextfiber.New(directory, len(owners), identity.Generation(1))
	if !indexOK {
		t.Fatal("index")
	}
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, directory, owners, identity.Generation(1), graph)
	if !layoutOK {
		t.Fatal("layout")
	}
	return layout
}

func TestLiftPreservesMountedContextRowsAndSingularGlobalRows(t *testing.T) {
	graph, _ := realPlanLawGraph(t)
	directory := newPlanLawDirectory(t, "same", "same")
	mounted, mountedOK := contextfiber.Mounted(directory.modules[0])
	global, globalOK := contextfiber.LinkGlobal(directory.directory.LinkID())
	if !mountedOK || !globalOK {
		t.Fatal("owners")
	}
	layout := newPlanLawLayout(t, graph, directory.directory, []contextfiber.PointOwner{mounted, global})
	edges, lifted := liftEdgesWithBoundEdges(graph, directory.directory, []pointPair{
		{from: 0, to: 0}, // same-Point graph self-dependency
		{from: 1, to: 0}, // global -> mounted fan-out
	}, []contextfiber.PointOwner{mounted, global}, []identity.ContentID{directory.modules[0], directory.modules[0]}, layout, nil)
	if !lifted || len(edges) != 4 {
		t.Fatalf("lifted edges=%d/%v, want two self rows and two global fan-out rows", len(edges), lifted)
	}
	left, leftOK := layout.Lookup(0, 0)
	right, rightOK := layout.Lookup(1, 0)
	sharedLeft, sharedLeftOK := layout.Lookup(0, 1)
	sharedRight, sharedRightOK := layout.Lookup(1, 1)
	if !leftOK || !rightOK || left == right || !sharedLeftOK || !sharedRightOK || sharedLeft != sharedRight {
		t.Fatalf("state mapping mounted=%d/%d global=%d/%d", left, right, sharedLeft, sharedRight)
	}
	seenSelf := 0
	seenGlobalFan := 0
	for _, edge := range edges {
		switch {
		case edge.from == edge.to:
			seenSelf++
		case edge.sourcePoint == 1 && edge.targetPoint == 0:
			seenGlobalFan++
		}
	}
	if seenSelf != 2 || seenGlobalFan != 2 {
		t.Fatalf("self=%d global-fan=%d", seenSelf, seenGlobalFan)
	}
	preparedEdges := make([]schedule.Edge, len(edges))
	for index, edge := range edges {
		preparedEdges[index] = schedule.Edge{From: schedule.Node(edge.from), To: schedule.Node(edge.to)}
	}
	wto, err := schedule.Prepare(int(layout.StateCount()), preparedEdges)
	if err != nil || wto == nil || wto.RegionCount() != 2 {
		t.Fatalf("lifted self WTO regions=%d err=%v, want one self region per context", func() int {
			if wto == nil {
				return 0
			}
			return wto.RegionCount()
		}(), err)
	}
}

func TestLiftRefusesMountedGlobalCollapseAndCrossModuleInference(t *testing.T) {
	graph, _ := realPlanLawGraph(t)
	leftDirectory := newPlanLawDirectory(t, "left")
	leftOwner, leftOK := contextfiber.Mounted(leftDirectory.modules[0])
	globalOwner, globalOK := contextfiber.LinkGlobal(leftDirectory.directory.LinkID())
	if !leftOK || !globalOK {
		t.Fatal("owners")
	}
	leftLayout := newPlanLawLayout(t, graph, leftDirectory.directory, []contextfiber.PointOwner{leftOwner, globalOwner})
	if _, lifted := liftEdgesWithBoundEdges(graph, leftDirectory.directory, []pointPair{{from: 0, to: 1}}, []contextfiber.PointOwner{leftOwner, globalOwner}, []identity.ContentID{leftDirectory.modules[0]}, leftLayout, nil); lifted {
		t.Fatal("mounted-to-global dependency collapsed without merge law")
	}
	crossDirectory := newPlanLawDirectory(t, "left", "right")
	crossLeft, crossLeftOK := contextfiber.Mounted(crossDirectory.modules[0])
	crossRight, crossRightOK := contextfiber.Mounted(crossDirectory.modules[1])
	if !crossLeftOK || !crossRightOK {
		t.Fatal("cross owners")
	}
	crossLayout := newPlanLawLayout(t, graph, crossDirectory.directory, []contextfiber.PointOwner{crossLeft, crossRight})
	if _, lifted := liftEdgesWithBoundEdges(graph, crossDirectory.directory, []pointPair{{from: 0, to: 1}}, []contextfiber.PointOwner{crossLeft, crossRight}, []identity.ContentID{crossDirectory.modules[0], crossDirectory.modules[1]}, crossLayout, nil); lifted {
		t.Fatal("cross-module dependency inferred without owner-issued binding")
	}
}

func TestContextTransportCensusRejectsOnlyExistingGraphTransportPairs(t *testing.T) {
	graph, _ := realPlanLawGraph(t)
	source, sourceOK := graph.PointAt(schedule.Node(0))
	target, targetOK := graph.PointAt(schedule.Node(0))
	if !sourceOK || !targetOK || !graphHasPointTransport(graph, source, target) {
		t.Fatal("graph transport census missed the sealed EnvironmentEdge pair")
	}
	other, otherOK := graph.PointAt(schedule.Node(1))
	if !otherOK || graphHasPointTransport(graph, other, target) {
		t.Fatal("graph transport census widened beyond the exact source/target pair")
	}
}

func TestContextTransportCensusIncludesGroupInputAndEnvironmentBoundary(t *testing.T) {
	graph := groupPlanLawGraph(t)
	group, groupOK := graph.HyperedgeAt(0)
	if !groupOK || group.InputCount() != 1 {
		t.Fatal("group transport fixture")
	}
	input, inputOK := group.InputAt(0)
	if !inputOK || !input.Point().Available() {
		t.Fatal("group input source point")
	}
	if !graphHasPointTransport(graph, input.Point(), group.Output()) {
		t.Fatal("Group input transport was absent from the exact census")
	}
	environment, environmentOK := group.EnvironmentInput()
	if !environmentOK || !environment.Point().Available() || !graphHasPointTransport(graph, environment.Point(), group.Output()) {
		t.Fatal("Group environment transport was absent from the exact census")
	}
	if graph.EnvironmentEdgeTotal() != 0 || graph.FactorEdgeTotal() != 0 {
		t.Fatal("Group census fixture unexpectedly gained another transport authority")
	}
	if graphHasPointTransport(graph, group.Output(), input.Point()) {
		t.Fatal("Group census widened beyond the exact source/target direction")
	}
}

func planLawCompositionKey(t *testing.T, label string) composition.Key {
	t.Helper()
	return composition.Key{ID: composition.ID(planLawID(t, "composition/"+label)), Version: 1}
}

func realPlanLawGraph(t *testing.T) (*equation.Graph, []composition.Key) {
	t.Helper()
	factor := planLawCompositionKey(t, "factor")
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
	})
	if !sourceOK || source == nil {
		t.Fatal("source composition")
	}
	batch := equation.NewBatch()
	firstKey := planLawCompositionKey(t, "mounted-site")
	secondKey := planLawCompositionKey(t, "global-site")
	first, firstOK := batch.AdmitSite(firstKey, equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	second, secondOK := batch.AdmitSite(secondKey, equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	if !firstOK || !secondOK || !batch.Seal() {
		t.Fatal("source batch")
	}
	input := equation.BoundaryInput(first, first, planLawCompositionKey(t, "self-input"), equation.TrueExpr(), equation.IdentityReindex(equation.EmptyScope()), equation.TrueExpr())
	if !input.Available() {
		t.Fatal("self input")
	}
	topology, topologyOK := equation.SealTopology(source, equation.TopologySpec{
		Batch:  batch,
		Points: []equation.PointSpec{{Site: first}, {Site: second}},
		EnvironmentEdges: []equation.EnvironmentEdge{{
			Target: equation.PointAt(0), Input: input,
		}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("topology")
	}
	relation, relationOK := topology.InitialRelation()
	if !relationOK {
		t.Fatal("initial relation")
	}
	graph, graphOK := topology.Graph(relation)
	if !graphOK || graph == nil || graph.EnvironmentEdgeTotal() != 1 || graph.PointCount() != 2 {
		t.Fatal("real graph")
	}
	orderedKeys := make([]composition.Key, graph.PointCount())
	for index := 0; index < graph.PointCount(); index++ {
		point, pointOK := graph.PointAt(schedule.Node(index))
		if !pointOK {
			t.Fatalf("point %d", index)
		}
		orderedKeys[index] = point.Site().Key()
	}
	return graph, orderedKeys
}

func groupPlanLawGraph(t *testing.T) *equation.Graph {
	t.Helper()
	factor := planLawCompositionKey(t, "group-factor")
	rule := planLawCompositionKey(t, "group-rule")
	family := planLawCompositionKey(t, "group-family")
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
		Rules: []composition.Rule{{
			Key: rule, OperandFamily: family, OutputKind: composition.FactorOutput, Output: factor, Inputs: 1,
			Reads:  []composition.Read{{Kind: composition.ReadExact, Input: 0, Factor: factor}},
			Writes: []composition.Write{{Kind: composition.WriteExact, Factor: factor}},
		}},
	})
	if !sourceOK || source == nil {
		t.Fatal("group source composition")
	}
	batch := equation.NewBatch()
	firstKey := planLawCompositionKey(t, "group-source-site")
	secondKey := planLawCompositionKey(t, "group-target-site")
	first, firstOK := batch.AdmitSite(firstKey, equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	second, secondOK := batch.AdmitSite(secondKey, equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.At(first)
	operand, operandOK := batch.AdmitOperand(occurrence, planLawCompositionKey(t, "group-operand"))
	if !firstOK || !secondOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("group source batch")
	}
	input := equation.BoundaryInput(first, second, planLawCompositionKey(t, "group-input"), equation.TrueExpr(), equation.IdentityReindex(equation.EmptyScope()), equation.TrueExpr())
	read := equation.Surface{Factor: factor, Form: equation.SurfaceReadExact, Local: 1}
	write := equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}
	topology, topologyOK := equation.SealTopology(source, equation.TopologySpec{
		Batch: batch,
		Rules: []equation.RuleInstance{{
			Schema: rule, OperandFamily: family, Occurrence: occurrence, Operand: operand,
			Reads:  []equation.ResolvedRead{{Index: 0, Surface: read}},
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: write}},
		}},
		Points: []equation.PointSpec{{Site: first}, {Site: second}},
		Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(1), Inputs: []equation.Input{input}, EnvironmentInput: input}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("group topology")
	}
	relation, relationOK := topology.InitialRelation()
	if !relationOK {
		t.Fatal("group initial relation")
	}
	graph, graphOK := topology.Graph(relation)
	if !graphOK || graph == nil || graph.GroupCount() != 1 {
		t.Fatal("group graph")
	}
	return graph
}

func TestNewLiftsRealGraphDeterministicallyAcrossContexts(t *testing.T) {
	graph, pointKeys := realPlanLawGraph(t)
	directory := newPlanLawDirectory(t, "same", "same")
	mounted, mountedOK := contextfiber.Mounted(directory.modules[0])
	global, globalOK := contextfiber.LinkGlobal(directory.directory.LinkID())
	if !mountedOK || !globalOK {
		t.Fatal("owners")
	}
	loop, loopOK := graph.EnvironmentEdgeAtIndex(0)
	if !loopOK || !loop.Target().Available() {
		t.Fatal("self-loop row")
	}
	loopPoint, loopPointOK := graph.PointIndex(loop.Target())
	if !loopPointOK {
		t.Fatal("self-loop point index")
	}
	mountedKey := loop.Target().Site().Key()
	owners := make([]contextfiber.PointOwner, len(pointKeys))
	mountedOwnerCount := 0
	for index, key := range pointKeys {
		if key == mountedKey {
			owners[index] = mounted
			mountedOwnerCount++
		} else {
			owners[index] = global
		}
	}
	if mountedOwnerCount != 1 {
		t.Fatalf("self-loop mounted owner count=%d", mountedOwnerCount)
	}
	layout := newPlanLawLayout(t, graph, directory.directory, owners)
	first, firstOK := New(graph, layout, directory.directory, nil)
	second, secondOK := New(graph, layout, directory.directory, nil)
	if !firstOK || !secondOK || first == nil || second == nil || !first.Available() || !second.Available() {
		t.Fatal("real plan construction")
	}
	foreignGraph, foreignKeys := realPlanLawGraph(t)
	if foreignGraph == nil || len(foreignKeys) != len(pointKeys) || foreignGraph == graph {
		t.Fatal("foreign equal-shaped graph")
	}
	if _, foreignOK := New(foreignGraph, layout, directory.directory, nil); foreignOK {
		t.Fatal("equal-shaped foreign graph borrowed the exact layout")
	}
	if first.StateCount() != 3 || second.StateCount() != 3 || first.StateCount() != layout.StateCount() || first.EdgeCount() != 2 {
		t.Fatalf("state/edge counts=%d/%d/%d, want compact 3 states and two lifted self edges", first.StateCount(), second.StateCount(), first.EdgeCount())
	}
	if first.Schedule() == nil || second.Schedule() == nil || first.Schedule().NodeCount() != int(first.StateCount()) || first.Schedule().RegionCount() != 2 {
		t.Fatalf("schedule nodes/regions=%d/%d", first.Schedule().NodeCount(), first.Schedule().RegionCount())
	}
	if first.Schedule().EventCount() != second.Schedule().EventCount() {
		t.Fatal("schedule event count changed across construction")
	}
	for state := 0; state < int(first.StateCount()); state++ {
		firstCell, firstCellOK := first.StateAt(contextfiber.StateOrdinal(state))
		secondCell, secondCellOK := second.StateAt(contextfiber.StateOrdinal(state))
		point, pointOK := firstCell.PointOrdinal()
		if !firstCellOK || !secondCellOK || !firstCell.Available() || !secondCell.Available() || firstCell != secondCell || !pointOK {
			t.Fatalf("state %d mapping=%+v/%+v", state, firstCell, secondCell)
		}
		if point == contextfiber.PointOrdinal(loopPoint) {
			if _, contextOK := firstCell.ContextOrdinal(); !contextOK {
				t.Fatalf("mounted state %d lost context", state)
			}
		} else if _, contextOK := firstCell.ContextOrdinal(); contextOK {
			t.Fatalf("global state %d acquired fabricated context", state)
		}
	}
	if !reflect.DeepEqual(first.Edges(), second.Edges()) || !reflect.DeepEqual(first.StateEdges(), second.StateEdges()) {
		t.Fatal("lifted edge order or metadata changed across construction")
	}
	for event := 0; event < first.Schedule().EventCount(); event++ {
		left, leftOK := first.Schedule().EventAt(event)
		right, rightOK := second.Schedule().EventAt(event)
		if !leftOK || !rightOK || left != right {
			t.Fatalf("schedule event %d=%+v/%+v", event, left, right)
		}
	}
}
