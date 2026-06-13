package summary

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// ParamEquality records a normal-return equality relation between two
// parameter roots.
type ParamEquality struct {
	Left  int
	Right int
}

func normalizeParamEqualities(in []ParamEquality) []ParamEquality {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[ParamEquality]struct{}, len(in))
	out := make([]ParamEquality, 0, len(in))
	for _, eq := range in {
		if eq.Left == eq.Right || eq.Left < 0 || eq.Right < 0 {
			continue
		}
		if eq.Right < eq.Left {
			eq.Left, eq.Right = eq.Right, eq.Left
		}
		if _, ok := seen[eq]; ok {
			continue
		}
		seen[eq] = struct{}{}
		out = append(out, eq)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Left != out[j].Left {
			return out[i].Left < out[j].Left
		}
		return out[i].Right < out[j].Right
	})
	return out
}

func paramEqualitiesEqual(a, b []ParamEquality) bool {
	a = normalizeParamEqualities(a)
	b = normalizeParamEqualities(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func paramEqualitiesLessOrEq(a, b []ParamEquality) bool {
	a = normalizeParamEqualities(a)
	b = normalizeParamEqualities(b)
	if len(b) == 0 {
		return true
	}
	seen := make(map[ParamEquality]struct{}, len(a))
	for _, eq := range a {
		seen[eq] = struct{}{}
	}
	for _, eq := range b {
		if _, ok := seen[eq]; !ok {
			return false
		}
	}
	return true
}

func paramEqualitiesSummaryEqual(reg *axis.Registry, a, b Summary) bool {
	if paramEqualitiesBottom(reg, a) || paramEqualitiesBottom(reg, b) {
		return paramEqualitiesBottom(reg, a) && paramEqualitiesBottom(reg, b)
	}
	return paramEqualitiesEqual(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func paramEqualitiesSummaryLessOrEq(reg *axis.Registry, a, b Summary) bool {
	if paramEqualitiesBottom(reg, a) {
		return true
	}
	if paramEqualitiesBottom(reg, b) {
		return false
	}
	return paramEqualitiesLessOrEq(a.NormalReturnParamEqualities, b.NormalReturnParamEqualities)
}

func joinParamEqualities(reg *axis.Registry, a, b Summary) []ParamEquality {
	switch {
	case paramEqualitiesBottom(reg, a):
		return normalizeParamEqualities(b.NormalReturnParamEqualities)
	case paramEqualitiesBottom(reg, b):
		return normalizeParamEqualities(a.NormalReturnParamEqualities)
	}
	aEqualities := normalizeParamEqualities(a.NormalReturnParamEqualities)
	bEqualities := normalizeParamEqualities(b.NormalReturnParamEqualities)
	if len(aEqualities) == 0 || len(bEqualities) == 0 {
		return nil
	}
	seen := make(map[ParamEquality]struct{}, len(bEqualities))
	for _, eq := range bEqualities {
		seen[eq] = struct{}{}
	}
	out := make([]ParamEquality, 0, min(len(aEqualities), len(bEqualities)))
	for _, eq := range aEqualities {
		if _, ok := seen[eq]; ok {
			out = append(out, eq)
		}
	}
	return normalizeParamEqualities(out)
}

func paramEqualitiesBottom(reg *axis.Registry, s Summary) bool {
	return normalReturnParamCount(reg, s) == 0 && len(s.NormalReturnParamEqualities) == 0
}
