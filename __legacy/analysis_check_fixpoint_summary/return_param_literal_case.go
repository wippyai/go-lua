package summary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// ReturnParamLiteralCase records a portable case split: when a parameter path
// has a concrete literal type, a return slot has Value.
type ReturnParamLiteralCase struct {
	ParamIndex  int
	ParamSuffix []segment.Segment
	When        typ.Type
	ReturnIndex int
	Value       product.Value
}

type returnParamLiteralCaseKey struct {
	paramIndex  int
	paramSuffix pathdom.PathKey
	when        string
	returnIndex int
}

type returnParamLiteralCaseMap = factmap.Map[returnParamLiteralCaseKey, ReturnParamLiteralCase, product.Value]

var returnParamLiteralCaseMaps registrycache.Cache[returnParamLiteralCaseMap]

func returnParamLiteralCaseLane(reg *axis.Registry) returnParamLiteralCaseMap {
	return returnParamLiteralCaseMaps.GetFor(reg, newReturnParamLiteralCaseMap)
}

func newReturnParamLiteralCaseMap(reg *axis.Registry) returnParamLiteralCaseMap {
	return returnParamLiteralCaseMap{
		Key:   returnParamLiteralCaseKeyOf,
		Value: func(c ReturnParamLiteralCase) product.Value { return c.Value },
		WithValue: func(c ReturnParamLiteralCase, value product.Value) ReturnParamLiteralCase {
			c.Value = value
			return c
		},
		Less: returnParamLiteralCaseLess,
		Valid: func(c ReturnParamLiteralCase) bool {
			return c.ParamIndex >= 0 &&
				c.ReturnIndex >= 0 &&
				c.When != nil &&
				usefulReturnConditionValue(reg, c.Value)
		},
		CloneFact: func(c ReturnParamLiteralCase) ReturnParamLiteralCase {
			c.ParamSuffix = append([]segment.Segment(nil), c.ParamSuffix...)
			return c
		},
		Domain:  product.Domain(reg),
		Collide: func(a, b product.Value) product.Value { return joinReturnValue(reg, a, b) },
	}
}

func normalizeReturnParamLiteralCases(reg *axis.Registry, in []ReturnParamLiteralCase) []ReturnParamLiteralCase {
	return returnParamLiteralCaseLane(reg).Normalize(in)
}

func cloneReturnParamLiteralCases(in []ReturnParamLiteralCase) []ReturnParamLiteralCase {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnParamLiteralCase, len(in))
	for i, c := range in {
		c.ParamSuffix = append([]segment.Segment(nil), c.ParamSuffix...)
		out[i] = c
	}
	return out
}

func returnParamLiteralCasesEqual(reg *axis.Registry, a, b []ReturnParamLiteralCase) bool {
	return returnParamLiteralCaseLane(reg).Equal(a, b)
}

func returnParamLiteralCasesLessOrEq(reg *axis.Registry, a, b []ReturnParamLiteralCase) bool {
	return returnParamLiteralCaseLane(reg).LessOrEq(a, b)
}

func joinReturnParamLiteralCases(reg *axis.Registry, a, b []ReturnParamLiteralCase) []ReturnParamLiteralCase {
	return returnParamLiteralCaseLane(reg).Join(a, b)
}

func widenReturnParamLiteralCases(reg *axis.Registry, prev, next []ReturnParamLiteralCase) []ReturnParamLiteralCase {
	return returnParamLiteralCaseLane(reg).Widen(prev, next)
}

func returnParamLiteralCaseKeyOf(c ReturnParamLiteralCase) returnParamLiteralCaseKey {
	return returnParamLiteralCaseKey{
		paramIndex:  c.ParamIndex,
		paramSuffix: pathdom.Path{Segments: c.ParamSuffix}.Key(),
		when:        c.When.String(),
		returnIndex: c.ReturnIndex,
	}
}

func returnParamLiteralCaseLess(a, b ReturnParamLiteralCase) bool {
	left := returnParamLiteralCaseKeyOf(a)
	right := returnParamLiteralCaseKeyOf(b)
	if left.paramIndex != right.paramIndex {
		return left.paramIndex < right.paramIndex
	}
	if left.paramSuffix != right.paramSuffix {
		return left.paramSuffix < right.paramSuffix
	}
	if left.when != right.when {
		return left.when < right.when
	}
	return left.returnIndex < right.returnIndex
}
