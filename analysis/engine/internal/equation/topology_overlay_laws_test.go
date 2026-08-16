package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// The sealed authority must publish the graph compiled during sealing. A
// second initial-publication call is an identity lookup, not another spec copy or
// compilation.
func TestTopologyGraphReturnsSealedInitialPayload(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !materializedOK {
		t.Fatal("materialization")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:            fixture.actuals,
		Materializations: []TemplateMaterialization{materialized},
		Points:           []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
	})
	if !sealed || topology == nil {
		t.Fatal("topology seal")
	}
	first, firstOK := initialGraph(topology)
	second, secondOK := initialGraph(topology)
	if !firstOK || !secondOK || first == nil || first != second || first.payload != nil || !topology.OwnsGraph(first) {
		t.Fatal("the initial publication did not return the sealed initial graph")
	}
	relation, relationOK := topology.InitialRelation()
	if !relationOK || !relation.Available() || relation.Generation() != 1 {
		t.Fatal("sealed topology lacked its first publication stamp")
	}
	if _, published := topology.Publish(relation, []AcceptedMember{{}}); published {
		t.Fatal("invalid accepted activation was admitted")
	}
	if _, issued := topology.Graph(Relation{}); issued {
		t.Fatal("unpublished relation was admitted as a graph anchor")
	}
}

func TestTopologyGraphAcceptedRevisionIsACompactSharedView(t *testing.T) {
	fixture := newTemplateMaterializationFixture(t)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !materializedOK {
		t.Fatal("materialization")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:            fixture.actuals,
		Materializations: []TemplateMaterialization{materialized},
		Points:           []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
	})
	if !sealed || topology == nil {
		t.Fatal("topology seal")
	}
	key := boundaryKey(91)
	receipt := activationReceipt{key: key, family: boundaryKey(92), trigger: boundaryKey(93), application: boundaryKey(94), target: boundaryKey(95), endpoint: boundaryKey(96)}
	topology.receipts = []activationReceipt{receipt}
	topology.receiptAt = map[composition.Key]int{key: 0}
	topology.receiptByTrigger = map[composition.Key][]int{receipt.trigger: {0}}
	member, memberOK := topology.SelectReceiptMember(receipt.trigger, PairLocator{Application: receipt.application, Target: receipt.target, Endpoint: receipt.endpoint})
	if !memberOK {
		t.Fatal("receipt member")
	}
	accepted, acceptedOK := topology.Accept(member, TrueExpr())
	if !acceptedOK {
		t.Fatal("accepted activation")
	}
	if _, rejected := topology.Accept(member, Expr{}); rejected {
		t.Fatal("unavailable premise accepted")
	}
	base, baseRelationOK := topology.InitialRelation()
	published, publishedOK := topology.Publish(base, []AcceptedMember{accepted})
	initial, initialOK := initialGraph(topology)
	view, viewOK := topology.Graph(published)
	if !baseRelationOK || !publishedOK || !initialOK || !viewOK || initial == nil || view == nil || view == initial || view.payload != initial || !topology.OwnsGraph(view) {
		t.Fatal("accepted graph was not a shared immutable view")
	}
	if !base.Precedes(published) || published.Generation() != base.Generation().Next() {
		t.Fatal("accepted publication did not advance exactly one generation")
	}
	if view.relation.Digest() == initial.relation.Digest() || view.PointCount() != initial.PointCount() || view.GroupCount() != initial.GroupCount() || view.FactorEdgeTotal() != initial.FactorEdgeTotal() {
		t.Fatal("accepted graph lost revision identity or structural equivalence")
	}
}

func TestActivationGraphOverlayBuildsFeedbackCertificate(t *testing.T) {
	fixture := newTemplateMaterializationFixtureWithGrammar(t, true, true)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !materializedOK {
		t.Fatal("materialization")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:            fixture.actuals,
		Materializations: []TemplateMaterialization{materialized},
		Queries: []QueryInstance{{Family: fixture.query, Point: PointAt(0), Surfaces: []Surface{{
			Factor: boundaryKey(201), Form: SurfaceReadSummary, Local: 1,
			Semantic: boundaryKey(218), Normalizer: boundaryKey(218),
		}}}},
	})
	if !sealed || topology == nil {
		t.Fatal("topology seal")
	}
	base, baseOK := initialGraph(topology)
	if !baseOK || base == nil || base.FactorEdgeTotal() == 0 {
		t.Fatal("base factor edge")
	}
	static, staticOK := base.FactorEdgeAtIndex(0)
	if !staticOK || static.Input().Point().Key() == static.Target().Key() {
		t.Fatal("direct factor edge endpoints")
	}
	source, sourceOK := base.PointIndex(static.Input().Point())
	target, targetOK := base.PointIndex(static.Target())
	if !sourceOK || !targetOK {
		t.Fatal("factor edge indices")
	}
	execution, executionErr := schedule.Prepare(base.PointCount(), []schedule.Edge{{From: schedule.Node(source), To: schedule.Node(target)}, {From: schedule.Node(target), To: schedule.Node(source)}})
	if executionErr != nil || execution == nil || execution.RegionCount() == 0 {
		t.Fatal("feedback schedule")
	}
	selected := SelectedStructuralFactorEdge{key: boundaryKey(250), source: static.Input().Point(), target: static.Target(), input: static.Input(), factor: static.Factor()}
	overlay, overlayOK := base.ActivationGraphOverlay(execution, []SelectedStructuralFactorEdge{selected})
	if !overlayOK || overlay == nil || overlay.FactorEdgeTotal() != base.FactorEdgeTotal()+1 || overlay.RegionCount() == 0 {
		t.Fatal("direct activation feedback overlay")
	}
	added, addedOK := overlay.FactorEdgeAtIndex(overlay.FactorEdgeTotal() - 1)
	if !addedOK || added.Key() != selected.Key() {
		t.Fatal("selected factor addition certificate")
	}
	if _, demanded := overlay.Demand(); !demanded {
		t.Fatal("feedback overlay demand")
	}
}

func TestActivationGraphOverlayCarriesInstalledDirectEdgesAcrossFrontiers(t *testing.T) {
	fixture := newTemplateMaterializationFixtureWithGrammar(t, true, true)
	materialized, materializedOK := MaterializeTemplateBoundary(fixture.source, fixture.binding,
		[]Site{fixture.input.Site(), fixture.local, fixture.output.Site()}, fixture.inputs)
	if !materializedOK {
		t.Fatal("materialization")
	}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:            fixture.actuals,
		Materializations: []TemplateMaterialization{materialized},
		Queries: []QueryInstance{{Family: fixture.query, Point: PointAt(0), Surfaces: []Surface{{
			Factor: boundaryKey(201), Form: SurfaceReadSummary, Local: 1,
			Semantic: boundaryKey(218), Normalizer: boundaryKey(218),
		}}}},
	})
	if !sealed || topology == nil {
		t.Fatal("topology seal")
	}
	base, baseOK := initialGraph(topology)
	if !baseOK || base == nil || base.FactorEdgeTotal() == 0 {
		t.Fatal("base factor edge")
	}
	static, staticOK := base.FactorEdgeAtIndex(0)
	if !staticOK || static.Input().Point().Key() == static.Target().Key() {
		t.Fatal("direct factor edge endpoints")
	}
	source, sourceOK := base.PointIndex(static.Input().Point())
	target, targetOK := base.PointIndex(static.Target())
	if !sourceOK || !targetOK {
		t.Fatal("factor edge indices")
	}
	first := SelectedStructuralFactorEdge{key: boundaryKey(250), source: static.Input().Point(), target: static.Target(), input: static.Input(), factor: static.Factor()}
	maps := make([]DecisionMap, len(static.Target().Scope().row.decisions))
	for index, decision := range static.Target().Scope().row.decisions {
		if static.Input().Point().Scope().contains(decision) {
			maps[index] = Identity(decision)
		} else {
			maps[index] = Forget(decision)
		}
	}
	reindex, reindexed := NewReindex(static.Target().Scope(), static.Input().Point().Scope(), maps)
	if !reindexed {
		t.Fatal("reverse reindex")
	}
	reverseInput := BoundaryInput(static.Target().Site(), static.Input().Point().Site(), boundaryKey(251), TrueExpr(), reindex, TrueExpr())
	if !reverseInput.Available() {
		t.Fatal("reverse input")
	}
	reverseInput.point = static.Target()
	second := SelectedStructuralFactorEdge{key: boundaryKey(252), source: static.Target(), target: static.Input().Point(), input: reverseInput, factor: static.Factor()}
	firstSchedule, firstErr := schedule.Prepare(base.PointCount(), []schedule.Edge{{From: schedule.Node(source), To: schedule.Node(target)}})
	if firstErr != nil || firstSchedule == nil || firstSchedule.RegionCount() != 0 {
		t.Fatal("acyclic first frontier")
	}
	firstOverlay, firstOverlayOK := base.ActivationGraphOverlay(firstSchedule, []SelectedStructuralFactorEdge{first})
	if !firstOverlayOK || firstOverlay == nil || firstOverlay.FactorEdgeTotal() != base.FactorEdgeTotal()+1 {
		t.Fatal("first direct installation")
	}
	feedbackSchedule, feedbackErr := schedule.Prepare(base.PointCount(), []schedule.Edge{{From: schedule.Node(source), To: schedule.Node(target)}, {From: schedule.Node(target), To: schedule.Node(source)}})
	if feedbackErr != nil || feedbackSchedule == nil || feedbackSchedule.RegionCount() == 0 {
		t.Fatal("feedback second frontier")
	}
	secondOverlay, secondOverlayOK := base.ActivationGraphOverlay(feedbackSchedule, []SelectedStructuralFactorEdge{first, second})
	if !secondOverlayOK || secondOverlay == nil || secondOverlay.FactorEdgeTotal() != base.FactorEdgeTotal()+2 || secondOverlay.RegionCount() == 0 {
		t.Fatal("installed direct catalog was not carried into feedback")
	}
	for index := base.FactorEdgeTotal(); index < secondOverlay.FactorEdgeTotal(); index++ {
		edge, edgeOK := secondOverlay.FactorEdgeAtIndex(index)
		if !edgeOK || edge.Key() != first.Key() && edge.Key() != second.Key() {
			t.Fatal("feedback direct edge certificate")
		}
	}
}

// initialGraph is the test-local spelling of "the sealed base publication":
// the first Relation of topology and the graph issued for it.
func initialGraph(topology *Topology) (*Graph, bool) {
	relation, ok := topology.InitialRelation()
	if !ok {
		return nil, false
	}
	return topology.Graph(relation)
}
