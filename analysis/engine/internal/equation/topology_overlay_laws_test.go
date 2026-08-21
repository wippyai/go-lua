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
	origin := MaterializationOrigin{Family: boundaryKey(92), Application: boundaryKey(94), Target: boundaryKey(95), Endpoint: boundaryKey(96), TriggerOrdinal: 0}
	materialized, materializedOK = materialized.WithOrigin(origin)
	if !materializedOK {
		t.Fatal("binding origin")
	}
	trigger := boundaryKey(93)
	topology.materializations = []TemplateMaterialization{materialized}
	topology.instanceKeys = []composition.Key{trigger}
	topology.triggers[trigger] = activationTriggerBinding{family: origin.Family, application: origin.Application}
	member, memberOK := topology.SelectActivationMember(trigger, PairLocator{Application: origin.Application, Target: origin.Target, Endpoint: origin.Endpoint})
	if !memberOK {
		t.Fatal("binding member")
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

func TestGraphSummaryKeyRangeRetainsSealedOwnerAndOrder(t *testing.T) {
	fixture := newTemplateMaterializationFixtureWithGrammar(t, true, false)
	surface := Surface{Factor: boundaryKey(201), Form: SurfaceReadSummary, Local: 1,
		Semantic: boundaryKey(218), Normalizer: boundaryKey(218)}
	keys := []uint64{1, 3}
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:     fixture.actuals,
		Points:    []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
		Queries:   []QueryInstance{{Family: fixture.query, Point: PointAt(0), Surfaces: []Surface{surface}}},
		Summaries: []SummaryMapping{{Surface: surface, Keys: keys}},
	})
	if !sealed || topology == nil {
		t.Fatal("summary topology seal")
	}
	keys[0] = 99
	graph, graphOK := initialGraph(topology)
	keyRange, ranged := graph.SummaryKeyRange(surface)
	if !graphOK || !ranged || keyRange.Count() != 2 {
		t.Fatal("sealed summary key range")
	}
	for index, expected := range []uint64{1, 3} {
		actual, present := keyRange.At(index)
		if !present || actual != expected {
			t.Fatalf("summary key %d = %d, want %d", index, actual, expected)
		}
	}
	if _, present := keyRange.At(-1); present {
		t.Fatal("negative summary key index was accepted")
	}
	if _, present := keyRange.At(keyRange.Count()); present {
		t.Fatal("summary key range overrun was accepted")
	}
	unknown := surface
	unknown.Local = 2
	if _, present := graph.SummaryKeyRange(unknown); present {
		t.Fatal("unknown summary surface was accepted")
	}
	var empty SummaryKeyRange
	if empty.Count() != 0 {
		t.Fatal("empty summary key range had a count")
	}
	if _, present := empty.At(0); present {
		t.Fatal("empty summary key range exposed a key")
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
