package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

type contextQualifiedActivationLawFixture struct {
	runtime       *solverRuntime
	graph         *equation.Graph
	contexts      executioncontext.Directory
	index         contextfiber.Index
	plan          *linkexecutionplan.LinkExecutionPlan
	from, to      executioncontext.Context
	siblingFrom   executioncontext.Context
	siblingTarget executioncontext.Context
	transition    executioncontext.Transition
	otherRoute    executioncontext.Transition
	factor        composition.Key
}

func contextQualifiedActivationLawID(t testing.TB, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/engine/context-qualified-activation-law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive context-qualified activation law identity %q", label)
	}
	return id
}

func contextQualifiedActivationLawKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func newContextQualifiedActivationLawFixture(t testing.TB) contextQualifiedActivationLawFixture {
	t.Helper()
	factor := contextQualifiedActivationLawKey(0x31)
	source, sourceOK := composition.Seal(composition.Candidate{
		Factors: []composition.Factor{{Key: factor}},
	})
	if !sourceOK || source == nil {
		t.Fatal("context-qualified activation source composition")
	}
	batch := equation.NewBatch()
	left, leftOK := batch.AdmitSite(contextQualifiedActivationLawKey(0x32), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	right, rightOK := batch.AdmitSite(contextQualifiedActivationLawKey(0x33), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	if !leftOK || !rightOK || !batch.Seal() {
		t.Fatalf("context-qualified activation point batch left=%t right=%t", leftOK, rightOK)
	}
	topology, topologyOK := equation.SealTopology(source, equation.TopologySpec{
		Batch:  batch,
		Points: []equation.PointSpec{{Site: left}, {Site: right}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("context-qualified activation topology")
	}
	relation, relationOK := topology.InitialRelation()
	graph, graphOK := topology.Graph(relation)
	if !relationOK || !graphOK || graph == nil || graph.PointCount() != 2 || graph.RegionCount() != 0 {
		t.Fatalf("context-qualified activation graph relation=%t graph=%t points=%d regions=%d", relationOK, graphOK, graph.PointCount(), graph.RegionCount())
	}

	linkID := contextQualifiedActivationLawID(t, "link")
	moduleID := contextQualifiedActivationLawID(t, "shared-module")
	rows := make([]executioncontext.Context, 0, 4)
	roots := make([]executioncontext.RootContext, 0, 4)
	// The four contexts are sibling cache instances of one module inside one
	// actor. Activation is actor-local, so a route between two actors is not
	// an edge any directory carries and could not stand in for a sibling here.
	actor := contextQualifiedActivationLawID(t, "actor")
	for index := 0; index < 4; index++ {
		representative := contextQualifiedActivationLawID(t, "representative-"+string(rune('a'+index)))
		row, rowOK := executioncontext.NewContext(linkID, moduleID, actor, representative)
		root, rootOK := executioncontext.NewRootContext(linkID, contextQualifiedActivationLawID(t, "root-"+string(rune('a'+index))), row.ID())
		if !rowOK || !rootOK {
			t.Fatalf("context-qualified activation context %d row=%t root=%t", index, rowOK, rootOK)
		}
		rows = append(rows, row)
		roots = append(roots, root)
	}
	transition, transitionOK := executioncontext.NewTransition(linkID, rows[0].ID(), rows[1].ID())
	otherRoute, otherRouteOK := executioncontext.NewTransition(linkID, rows[0].ID(), rows[2].ID())
	if !transitionOK || !otherRouteOK {
		t.Fatal("context-qualified activation transitions")
	}
	directory, directoryOK := executioncontext.Seal(linkID, rows, roots, []executioncontext.Transition{transition, otherRoute})
	if !directoryOK || !directory.Available() {
		t.Fatal("context-qualified activation directory")
	}
	owner, ownerOK := contextfiber.Mounted(moduleID)
	if !ownerOK {
		t.Fatal("context-qualified activation point owner")
	}
	generation := identity.Generation(1)
	index, indexOK := contextfiber.New(directory, graph.PointCount(), generation)
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, directory, []contextfiber.PointOwner{owner, owner}, generation, graph)
	plan, planOK := linkexecutionplan.New(graph, layout, directory, nil)
	if !indexOK || !layoutOK || !planOK || plan == nil || !plan.Available() {
		t.Fatalf("context-qualified activation state plane index=%t layout=%t plan=%t", indexOK, layoutOK, planOK)
	}
	from, fromOK := directory.Context(rows[0].ID())
	to, toOK := directory.Context(rows[1].ID())
	siblingFrom, siblingFromOK := directory.Context(rows[2].ID())
	siblingTarget, siblingTargetOK := directory.Context(rows[3].ID())
	if !fromOK || !toOK || !siblingFromOK || !siblingTargetOK {
		t.Fatal("context-qualified activation context rows")
	}
	statePointRows, statePointRowsOK := buildStatePointRows(graph, plan, true)
	if !statePointRowsOK {
		t.Fatal("context-qualified activation state-point rows")
	}
	runtime := &solverRuntime{
		graph:          graph,
		contexts:       directory,
		contextIndex:   index,
		contextLayout:  layout,
		artifactBacked: true,
		executionPlan:  plan,
		statePointRows: statePointRows,
		activeStates:   make([]bool, int(plan.StateCount())),
	}
	fromOrdinal, fromOrdinalOK := index.ContextOrdinal(from.ID())
	if !fromOrdinalOK {
		t.Fatal("context-qualified activation source ordinal")
	}
	sourceState, sourceStateOK := plan.Lookup(fromOrdinal, contextfiber.PointOrdinal(0))
	if !sourceStateOK {
		t.Fatal("context-qualified activation source state")
	}
	runtime.activeStates[int(sourceState)] = true
	return contextQualifiedActivationLawFixture{
		runtime: runtime, graph: graph, contexts: directory, index: index, plan: plan,
		from: from, to: to, siblingFrom: siblingFrom, siblingTarget: siblingTarget,
		transition: transition, otherRoute: otherRoute, factor: factor,
	}
}

func (fixture contextQualifiedActivationLawFixture) completeContext() equation.ActivationContext {
	return equation.ActivationContext{
		TransitionID:  fixture.transition.ID(),
		FromContextID: fixture.from.ID(),
		ToContextID:   fixture.to.ID(),
	}
}

func (fixture contextQualifiedActivationLawFixture) selectedEdge() runtimeFactorEdge {
	fromOrdinal, _ := fixture.index.ContextOrdinal(fixture.from.ID())
	toOrdinal, _ := fixture.index.ContextOrdinal(fixture.to.ID())
	return runtimeFactorEdge{
		index:       0,
		key:         contextQualifiedActivationLawKey(0x34),
		factor:      fixture.factor,
		source:      0,
		target:      1,
		fromContext: fromOrdinal,
		toContext:   toOrdinal,
		context:     fixture.completeContext(),
	}
}

func stateForContextPoint(t testing.TB, fixture contextQualifiedActivationLawFixture, context executioncontext.Context, point int) int {
	t.Helper()
	contextID := context.ID()
	contextOrdinal, contextOK := fixture.index.ContextOrdinal(contextID)
	if !contextOK {
		t.Fatalf("context %x has no state-plane ordinal", contextID[:4])
	}
	state, stateOK := fixture.plan.Lookup(contextOrdinal, contextfiber.PointOrdinal(point))
	if !stateOK {
		t.Fatalf("context %x point %d has no StateOrdinal", contextID[:4], point)
	}
	cell, cellOK := fixture.plan.StateAt(state)
	cellContext, cellContextOK := cell.ContextOrdinal()
	cellPoint, cellPointOK := cell.PointOrdinal()
	if !cellOK || !cellContextOK || !cellPointOK || cellContext != contextOrdinal || int(cellPoint) != point {
		t.Fatalf("StateOrdinal %d inverse context=%v/%t point=%v/%t", state, cellContext, cellContextOK, cellPoint, cellPointOK)
	}
	return int(state)
}

func selectedStateEventNodes(t testing.TB, events []schedule.Event) map[int]struct{} {
	t.Helper()
	seen := make(map[int]struct{})
	for _, event := range events {
		if event.Kind != schedule.EventNode {
			t.Fatalf("selected state event retained non-node event kind=%v", event.Kind)
		}
		seen[int(event.Node)] = struct{}{}
	}
	return seen
}

// TestContextQualifiedActivationLiftsExactlyOneStatePair is the runtime law
// for one selected activation edge. The complete transition tuple is resolved
// to one source and one target StateOrdinal, then only those rows are active
// and present in the filtered execution events. Equal graph points in sibling
// contexts never become implicit activation or schedule targets.
func TestContextQualifiedActivationLiftsExactlyOneStatePair(t *testing.T) {
	fixture := newContextQualifiedActivationLawFixture(t)
	runtime, context := fixture.runtime, fixture.completeContext()
	source := stateForContextPoint(t, fixture, fixture.from, 0)
	target := stateForContextPoint(t, fixture, fixture.to, 1)
	siblingSource := stateForContextPoint(t, fixture, fixture.siblingFrom, 0)
	siblingTarget := stateForContextPoint(t, fixture, fixture.siblingTarget, 1)
	if source == siblingSource || target == siblingTarget {
		t.Fatal("sibling contexts collapsed onto selected StateOrdinals")
	}
	if !runtime.validSelectedActivationContext(context, 0, 1) {
		t.Fatal("complete activation tuple was not authenticated")
	}
	pairs, pairsOK := runtime.liftGraphPairStates(0, 1, context)
	if !pairsOK || len(pairs) != 1 || pairs[0] != (schedule.Edge{From: schedule.Node(source), To: schedule.Node(target)}) {
		t.Fatalf("selected activation lifted pairs=%v want one exact %d->%d", pairs, source, target)
	}

	selected := fixture.selectedEdge()
	prepared := &preparedSelectedFactorOverlay{
		runtime:   runtime,
		additions: []preparedFactorAddition{{edge: selected}},
	}
	if !prepared.bindArtifactStateOverlay(runtime) {
		t.Fatal("complete selected activation edge was not admitted to the state overlay")
	}
	if len(prepared.stateFactorRows) != 1 || prepared.stateFactorRows[0].source != source || prepared.stateFactorRows[0].target != target {
		t.Fatalf("state factor rows=%v want exact source=%d target=%d", prepared.stateFactorRows, source, target)
	}
	if len(prepared.stateTargets) != 1 || prepared.stateTargets[0] != target {
		t.Fatalf("state targets=%v want [%d]", prepared.stateTargets, target)
	}
	if !prepared.stateActive[source] || !prepared.stateActive[target] || prepared.stateActive[siblingSource] || prepared.stateActive[siblingTarget] {
		t.Fatal("selected activation changed sibling active-state rows")
	}
	events := selectedStateEventNodes(t, prepared.stateExecutionEvents)
	if len(events) != 2 {
		t.Fatalf("filtered selected state events=%v want source and target only", events)
	}
	if _, found := events[source]; !found {
		t.Fatalf("source StateOrdinal %d missing from selected state events", source)
	}
	if _, found := events[target]; !found {
		t.Fatalf("target StateOrdinal %d missing from selected state events", target)
	}
	for _, sibling := range []int{siblingSource, siblingTarget} {
		if _, found := events[sibling]; found {
			t.Fatalf("sibling StateOrdinal %d was scheduled by selected activation", sibling)
		}
	}
	if len(prepared.stateExecution.Edges()) != 1 || prepared.stateExecution.Edges()[0] != pairs[0] {
		t.Fatalf("state execution edges=%v want exact selected pair %v", prepared.stateExecution.Edges(), pairs[0])
	}
}

// TestContextQualifiedActivationRefusesIncompleteForeignAndAmbiguousTuples
// keeps the runtime refusal boundary closed. A partial/missing tuple, a
// foreign context, or a complete tuple whose transition disagrees with its
// endpoint pair cannot activate or schedule any state row.
func TestContextQualifiedActivationRefusesIncompleteForeignAndAmbiguousTuples(t *testing.T) {
	fixture := newContextQualifiedActivationLawFixture(t)
	runtime := fixture.runtime
	valid := fixture.completeContext()
	foreign := contextQualifiedActivationLawID(t, "foreign-context")
	cases := map[string]equation.ActivationContext{
		"missing": {},
		"partial": {TransitionID: valid.TransitionID},
		"foreign-context": {
			TransitionID:  valid.TransitionID,
			FromContextID: foreign,
			ToContextID:   valid.ToContextID,
		},
		// The IDs are all present, but the transition names A -> sibling-C
		// while the tuple claims A -> B. This is a complete but ambiguous
		// graph-point pair and must not be resolved by module equality.
		"ambiguous-endpoints": {
			TransitionID:  fixture.otherRoute.ID(),
			FromContextID: valid.FromContextID,
			ToContextID:   valid.ToContextID,
		},
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			if runtime.validSelectedActivationContext(candidate, 0, 1) {
				t.Fatal("invalid activation tuple authenticated")
			}
			if pairs, paired := runtime.liftGraphPairStates(0, 1, candidate); paired || len(pairs) != 0 {
				t.Fatalf("invalid activation tuple lifted pairs=%v paired=%t", pairs, paired)
			}
			selected := fixture.selectedEdge()
			selected.context = candidate
			prepared := &preparedSelectedFactorOverlay{
				runtime:   runtime,
				additions: []preparedFactorAddition{{edge: selected}},
			}
			if prepared.bindArtifactStateOverlay(runtime) {
				t.Fatal("invalid activation tuple installed a state overlay")
			}
			if prepared.stateActive != nil || prepared.stateExecution != nil || prepared.stateTargets != nil {
				t.Fatal("invalid activation tuple left prepared activation state")
			}
		})
	}
}
