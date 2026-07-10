package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// ParamEquality records a normal-return equality relation between two
// parameter roots.
type ParamEquality struct {
	Left  int
	Right int
}

// paramEqualityLane is the canonical must (intersection) lattice for parameter
// equalities: each relation is canonicalized to Left<Right and kept only when
// guaranteed on every joined path.
var paramEqualityLane = factset.Set[ParamEquality, ParamEquality]{
	Key:       func(eq ParamEquality) ParamEquality { return eq },
	EqualFact: func(a, b ParamEquality) bool { return a == b },
	Less: func(a, b ParamEquality) bool {
		if a.Left != b.Left {
			return a.Left < b.Left
		}
		return a.Right < b.Right
	},
	Admit: func(eq ParamEquality) (ParamEquality, bool) {
		if eq.Left == eq.Right || eq.Left < 0 || eq.Right < 0 {
			return eq, false
		}
		if eq.Right < eq.Left {
			eq.Left, eq.Right = eq.Right, eq.Left
		}
		return eq, true
	},
	Intersect: true,
}

func normalizeParamEqualities(in []ParamEquality) []ParamEquality {
	return paramEqualityLane.Normalize(in)
}

func paramEqualitiesSummaryEqual(reg *axis.Registry, a, b Summary) bool {
	if paramEqualitiesBottom(reg, a) || paramEqualitiesBottom(reg, b) {
		return paramEqualitiesBottom(reg, a) && paramEqualitiesBottom(reg, b)
	}
	return paramEqualityLane.Equal(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func paramEqualitiesSummaryLessOrEq(reg *axis.Registry, a, b Summary) bool {
	if paramEqualitiesBottom(reg, a) {
		return true
	}
	if paramEqualitiesBottom(reg, b) {
		return false
	}
	return paramEqualityLane.LessOrEq(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func joinParamEqualities(reg *axis.Registry, a, b Summary) []ParamEquality {
	switch {
	case paramEqualitiesBottom(reg, a):
		return paramEqualityLane.Normalize(b.NormalReturnParamEqualities)
	case paramEqualitiesBottom(reg, b):
		return paramEqualityLane.Normalize(a.NormalReturnParamEqualities)
	}
	return paramEqualityLane.Join(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func paramEqualitiesBottom(reg *axis.Registry, s Summary) bool {
	return normalReturnParamCount(reg, s) == 0 && len(s.NormalReturnParamEqualities) == 0
}
