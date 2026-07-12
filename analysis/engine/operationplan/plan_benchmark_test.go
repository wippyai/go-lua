package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

var benchmarkCells []Cell
var benchmarkRows []row

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

func sparseBenchmarkFacts(points int) factflow.FactsInput {
	in := factflow.FactsInput{
		RootAssignments:        make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:        make(map[cfg.Point]factflow.PathAssignment),
		NoNormalReturns:        make(map[cfg.Point]struct{}),
		BranchConditionSources: make(map[cfg.Point]factflow.ValueSource),
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
			in.BranchConditionSources[point] = factflow.ValueSource{}
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
		for _, d := range descriptors {
			if pointFactPresent(input, cfg.Point(point), d.kind) {
				cells = append(cells, Cell{kind: d.kind, class: d.class})
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
