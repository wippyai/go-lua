package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

var benchmarkCells []Cell
var benchmarkRows []row
var benchmarkCellCount int

// BenchmarkCompileIndexSparseLarge models a large lowered body: most points
// carry one or two fact families, not one fact from every family.
func BenchmarkCompileIndexSparseLarge(b *testing.B) {
	const points = 10_000
	input := sparseBenchmarkFacts(points)
	b.Run("fact-occurrences", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRows, benchmarkCells = compileIndex(points, input)
		}
	})
	b.Run("point-family-probes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkRows, benchmarkCells = compileIndexByProbing(points, input)
		}
	})
}

func BenchmarkCursorCanonicalOrder(b *testing.B) {
	const points = 64
	plan := New(benchmarkGraph(points), sparseBenchmarkFacts(points))
	b.ReportAllocs()
	count := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor := plan.Cursor(cfg.Point(i % points))
		for _, ok := cursor.Next(); ok; _, ok = cursor.Next() {
			count++
		}
	}
	benchmarkCellCount = count
}

func BenchmarkObservationRequirementsSeal(b *testing.B) {
	const points = 2_048
	graph := cfg.New()
	previous := graph.Entry()
	input := factflow.FactsInput{RootAssignments: make(map[cfg.Point]factflow.RootAssignment)}
	for graph.Size() < points-1 {
		point := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(previous, point, false)
		previous = point
		if int(point)%7 == 0 {
			input.RootAssignments[point] = factflow.RootAssignment{}
		}
	}
	graph.AddEdge(previous, graph.Exit(), false)
	lowered := wir.NewBody("observation-requirements-benchmark")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	plan := New(graph, input)
	b.Run("identity-baseline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkObservationPlan = bindObservationIdentityBenchmarkBaseline(plan, owner, lowered, graph)
		}
	})
	b.Run("sealed-requirements", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkObservationPlan = plan.WithObservationIdentity(owner, lowered, graph)
		}
		b.ReportMetric(float64(benchmarkObservationPlan.observationRequirements.slotCount), "requirements/op")
	})
}

var benchmarkObservationPlan *Plan

// bindObservationIdentityBenchmarkBaseline is the pre-certificate traversal
// retained only as a benchmark oracle. Production uses WithObservationIdentity.
func bindObservationIdentityBenchmarkBaseline(p *Plan, body lexicalidentity.StableLexicalBodyID, lowered *wir.Body, graph cfg.Graph) *Plan {
	out := *p
	out.observationBody = lexicalidentity.StableLexicalBodyID{}
	out.observationPoints = nil
	if body == (lexicalidentity.StableLexicalBodyID{}) || lowered == nil || graph == nil || graph.Size() != p.PointCount() {
		return &out
	}
	reachable := cfg.RPOReadOnly(graph)
	debugPoints := lowered.DebugPoints()
	if len(reachable) == 0 || len(debugPoints) != len(reachable) {
		return &out
	}
	points := make([]observationPoint, p.PointCount())
	for index, point := range reachable {
		if uint64(point) >= uint64(len(points)) || points[point].after.Valid() {
			return &out
		}
		debugPoint := debugPoints[index]
		if debugPoint.Point != point || debugPoint.Ordinal != uint32(index+1) {
			return &out
		}
		after, ok := lowered.DebugPointID(point, wir.DebugPhaseAfter)
		if !ok {
			return &out
		}
		points[point].after = after
	}
	out.observationBody, out.observationPoints = body, points
	return &out
}

func benchmarkGraph(points int) cfg.Graph {
	graph := cfg.New()
	for graph.Size() < points {
		graph.AddNode(cfg.NodeNoop)
	}
	return graph
}

func sparseBenchmarkFacts(points int) factflow.FactsInput {
	in := factflow.FactsInput{
		RootAssignments:        make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:        make(map[cfg.Point]factflow.PathAssignment),
		NoNormalReturns:        make(map[cfg.Point]struct{}),
		BranchConditionSources: make(map[cfg.Point]factflow.BranchCondition),
		BranchRefinements:      make(map[cfg.Point]factflow.BranchRefinementSet),
		Returns:                make(map[cfg.Point]factflow.Return),
		CallSites:              make(map[cfg.Point]factflow.CallSite),
	}
	for i := 0; i < points; i++ {
		point := cfg.Point(i)
		if i%2 == 0 {
			in.RootAssignments[point] = factflow.RootAssignment{}
		}
		if i%3 == 0 {
			in.PathAssignments[point] = factflow.PathAssignment{}
		}
		if i%101 == 0 {
			in.NoNormalReturns[point] = struct{}{}
		}
		if i%17 == 0 {
			in.BranchConditionSources[point] = factflow.BranchCondition{}
			in.BranchRefinements[point] = factflow.BranchRefinementSet{}
		}
		if i%71 == 0 {
			in.Returns[point] = factflow.Return{}
		}
		if i%13 == 0 {
			in.CallSites[point] = factflow.CallSite{}
		}
	}
	return in
}

// compileIndexByProbing preserves the former O(points*kinds) implementation
// as a benchmark oracle. Production does not call it.
func compileIndexByProbing(size int, input factflow.FactsInput) ([]row, []Cell) {
	rows := make([]row, size)
	cells := make([]Cell, 0)
	for point := 0; point < size; point++ {
		rows[point].start = uint32(len(cells))
		for _, kind := range cursorOrder {
			if pointFactPresent(input, cfg.Point(point), kind) {
				cells = append(cells, Cell{kind: kind})
			}
		}
		rows[point].end = uint32(len(cells))
	}
	return rows, cells
}

func pointFactPresent(in factflow.FactsInput, point cfg.Point, kind Kind) bool {
	switch kind {
	case RootAssignment:
		_, ok := in.RootAssignments[point]
		return ok
	case PathAssignment:
		_, ok := in.PathAssignments[point]
		return ok
	case PathStaticMemberWrite:
		_, ok := in.PathStaticMemberWrites[point]
		return ok
	case DynamicIndexWrite:
		_, ok := in.DynamicIndexWrites[point]
		return ok
	case PathDescendantInvalidation:
		_, ok := in.PathDescendantInvalidations[point]
		return ok
	case CovariantExposure:
		return len(in.CovariantExposures[point]) != 0
	case NoNormalReturn:
		_, ok := in.NoNormalReturns[point]
		return ok
	case BranchEdgeReachability:
		_, ok := in.BranchEdgeReachability[point]
		return ok
	case BranchConditionSource:
		_, ok := in.BranchConditionSources[point]
		return ok
	case BranchRefinement:
		_, ok := in.BranchRefinements[point]
		return ok
	case BranchPresenceRelation:
		_, ok := in.BranchPresenceRelations[point]
		return ok
	case BranchPathRelation:
		_, ok := in.BranchPathRelations[point]
		return ok
	case BranchPathEvidence:
		_, ok := in.BranchPathEvidence[point]
		return ok
	case BranchSufficientLiteralCase:
		_, ok := in.BranchSufficientLiteralCases[point]
		return ok
	case PathValuePresenceImplication:
		_, ok := in.PathValuePresenceImplications[point]
		return ok
	case ChannelSelect:
		_, ok := in.ChannelSelects[point]
		return ok
	case PostconditionRefinement:
		_, ok := in.PostconditionRefinements[point]
		return ok
	case PostconditionPathRelation:
		return len(in.PostconditionPathRelations[point]) != 0
	case CallResultValue:
		_, ok := in.CallResultValues[point]
		return ok
	case ReturnPresenceRelation:
		_, ok := in.ReturnPresenceRelations[point]
		return ok
	case Return:
		_, ok := in.Returns[point]
		return ok
	case CallSite:
		_, ok := in.CallSites[point]
		return ok
	default:
		return false
	}
}
