package typevalue

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// InRangeIndexExcludesNil reports whether every element selected by an
// in-range integer index excludes nil. This is the type-axis half of the
// canonical dynamic-read evidence contract: path/numeric evidence proves that
// a member is selected, while this predicate proves that nil cannot be the
// selected member's value.
//
// Recursive types are solved as finite monotone equation systems. There is no
// traversal-depth cap and no alternate concrete/symbolic interpretation.
func InRangeIndexExcludesNil(t typ.Type) bool {
	if !indexCanSelectElement(t) {
		return false
	}
	return inspect.GreatestBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Array:
			element := value.Element
			if element == nil {
				element = typ.Unknown
			}
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: !TypeIncludesNil(element)}
		case *typ.Tuple:
			if len(value.Elements) == 0 {
				return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
			}
			for _, element := range value.Elements {
				if element == nil {
					element = typ.Unknown
				}
				if TypeIncludesNil(element) {
					return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
				}
			}
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: true}
		case *typ.Record:
			found := false
			for index := int64(1); ; index++ {
				member := value.GetStaticIntIndex(index)
				if member == nil || member.Optional {
					return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: found}
				}
				found = true
				if TypeIncludesNil(member.Type) {
					return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
				}
			}
		case *typ.Optional:
			return inspect.BoolEquation{Join: inspect.BoolAll, Inputs: []typ.Type{value.Inner}}
		case *typ.Union:
			reachable := make([]typ.Type, 0, len(value.Members))
			for _, member := range value.Members {
				if indexCanSelectElement(member) {
					reachable = append(reachable, member)
				}
			}
			if len(reachable) == 0 {
				return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
			}
			return inspect.BoolEquation{Join: inspect.BoolAll, Inputs: reachable}
		case *typ.Recursive:
			return inspect.BoolEquation{Join: inspect.BoolAll, Inputs: []typ.Type{value.Body}}
		default:
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
		}
	})
}

func indexCanSelectElement(t typ.Type) bool {
	return inspect.LeastBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Array:
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: true}
		case *typ.Tuple:
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: len(value.Elements) != 0}
		case *typ.Record:
			member := value.GetStaticIntIndex(1)
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: member != nil && !member.Optional}
		case *typ.Optional:
			return inspect.BoolEquation{Join: inspect.BoolAll, Inputs: []typ.Type{value.Inner}}
		case *typ.Union:
			if len(value.Members) == 0 {
				return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
			}
			return inspect.BoolEquation{Join: inspect.BoolAny, Inputs: value.Members}
		case *typ.Recursive:
			return inspect.BoolEquation{Join: inspect.BoolAny, Inputs: []typ.Type{value.Body}}
		default:
			return inspect.BoolEquation{Join: inspect.BoolConstant, Constant: false}
		}
	})
}
