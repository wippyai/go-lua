package indexproof

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/inspect"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// SequenceLengthKnownAtLeast proves that every reachable sequence arm has at
// least floor required elements. Recursive types are solved as a greatest
// fixed point after a least-fixed-point reachability check; no recursion cap
// or syntactic unfolding limit is used.
func SequenceLengthKnownAtLeast(t typ.Type, floor int64) bool {
	if floor <= 0 {
		return true
	}
	if !sequenceCanHaveLengthAtLeast(t, floor) {
		return false
	}
	return inspect.GreatestBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Tuple:
			return inspect.Constant(int64(len(value.Elements)) >= floor)
		case *typ.Record:
			for index := int64(1); index <= floor; index++ {
				member := value.GetStaticIntIndex(index)
				if member == nil || member.Optional {
					return inspect.Constant(false)
				}
			}
			return inspect.Constant(true)
		case *typ.Optional:
			return inspect.All(value.Inner)
		case *typ.Union:
			reachable := make([]typ.Type, 0, len(value.Members))
			for _, member := range value.Members {
				if sequenceCanHaveLengthAtLeast(member, floor) {
					reachable = append(reachable, member)
				}
			}
			if len(reachable) == 0 {
				return inspect.Constant(false)
			}
			return inspect.All(reachable...)
		case *typ.Recursive:
			return inspect.All(value.Body)
		default:
			return inspect.Constant(false)
		}
	})
}

func sequenceCanHaveLengthAtLeast(t typ.Type, floor int64) bool {
	return inspect.LeastBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Tuple:
			return inspect.Constant(int64(len(value.Elements)) >= floor)
		case *typ.Record:
			for index := int64(1); index <= floor; index++ {
				member := value.GetStaticIntIndex(index)
				if member == nil || member.Optional {
					return inspect.Constant(false)
				}
			}
			return inspect.Constant(true)
		case *typ.Optional:
			return inspect.Any(value.Inner)
		case *typ.Union:
			return inspect.Any(value.Members...)
		case *typ.Recursive:
			return inspect.Any(value.Body)
		default:
			return inspect.Constant(false)
		}
	})
}

// CanHaveLengthAtLeast reports whether some runtime value represented by t
// may have length at least floor. Unlike SequenceLengthKnownAtLeast, this is a
// reachability predicate: maps, strings and gradual types can satisfy a
// point-local length floor even though their type alone does not prove it.
//
// It is used to discard type-union arms made unreachable by an independently
// proved length floor before asking a universal in-range question.
func CanHaveLengthAtLeast(t typ.Type, floor int64) bool {
	if floor <= 0 {
		return true
	}
	return inspect.LeastBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Array, *typ.Map, *typ.ReadonlyMap:
			return inspect.Constant(true)
		case *typ.Tuple:
			return inspect.Constant(int64(len(value.Elements)) >= floor)
		case *typ.Record:
			if value.Open || value.Metatable != nil || value.HasMapComponent() {
				return inspect.Constant(true)
			}
			for _, member := range value.StaticMembers {
				if member.Kind == typ.StaticMemberIntIndex && member.Index >= floor && !member.Optional {
					return inspect.Constant(true)
				}
			}
			return inspect.Constant(false)
		case *typ.Optional:
			return inspect.Any(value.Inner)
		case *typ.Union:
			return inspect.Any(value.Members...)
		case *typ.Recursive:
			return inspect.Any(value.Body)
		case *typ.Literal:
			literal, ok := value.Value.(string)
			return inspect.Constant(value.Base == kind.String && ok && int64(len(literal)) >= floor)
		default:
			if current == nil {
				return inspect.Constant(false)
			}
			switch unwrap.Alias(current).Kind() {
			case kind.String, kind.Any, kind.Unknown:
				return inspect.Constant(true)
			default:
				return inspect.Constant(false)
			}
		}
	})
}

// StaticIndexPresentUnderLengthFloor reports whether every runtime value in t
// that can satisfy len(value) >= floor has a required member at index. The
// length premise is external flow evidence; this function only performs the
// canonical type-side conditional proof.
//
// Recursive types are solved as a finite monotone equation system. There is no
// recursion-depth cap or alternate syntactic implementation.
func StaticIndexPresentUnderLengthFloor(t typ.Type, index, floor int64) bool {
	if index < 1 || floor < index || !CanHaveLengthAtLeast(t, floor) {
		return false
	}
	return inspect.GreatestBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Optional:
			return inspect.All(value.Inner)
		case *typ.Union:
			reachable := make([]typ.Type, 0, len(value.Members))
			for _, member := range value.Members {
				if CanHaveLengthAtLeast(member, floor) {
					reachable = append(reachable, member)
				}
			}
			if len(reachable) == 0 {
				return inspect.Constant(false)
			}
			return inspect.All(reachable...)
		case *typ.Recursive:
			return inspect.All(value.Body)
		case *typ.Array:
			return inspect.Constant(true)
		case *typ.Tuple:
			return inspect.Constant(index <= int64(len(value.Elements)))
		case *typ.Record:
			member := value.GetStaticIntIndex(index)
			return inspect.Constant(member != nil && !member.Optional)
		default:
			return inspect.Constant(false)
		}
	})
}

// StaticIndexExcludesNilUnderLengthFloor reports whether every runtime value
// in t that can satisfy len(value) >= floor has a required, non-nil member at
// index. It is the exact type-side complement to a flow proof that a constant
// Lua array index is in range.
func StaticIndexExcludesNilUnderLengthFloor(t typ.Type, index, floor int64) bool {
	if index < 1 || floor < index || !CanHaveLengthAtLeast(t, floor) {
		return false
	}
	return inspect.GreatestBoolFixedPoint(t, func(current typ.Type) inspect.BoolEquation {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Optional:
			return inspect.All(value.Inner)
		case *typ.Union:
			reachable := make([]typ.Type, 0, len(value.Members))
			for _, member := range value.Members {
				if CanHaveLengthAtLeast(member, floor) {
					reachable = append(reachable, member)
				}
			}
			if len(reachable) == 0 {
				return inspect.Constant(false)
			}
			return inspect.All(reachable...)
		case *typ.Recursive:
			return inspect.All(value.Body)
		case *typ.Array:
			element := value.Element
			if element == nil {
				element = typ.Unknown
			}
			return inspect.Constant(!typevalue.TypeIncludesNil(element))
		case *typ.Tuple:
			if index > int64(len(value.Elements)) {
				return inspect.Constant(false)
			}
			element := value.Elements[index-1]
			if element == nil {
				element = typ.Unknown
			}
			return inspect.Constant(!typevalue.TypeIncludesNil(element))
		case *typ.Record:
			member := value.GetStaticIntIndex(index)
			return inspect.Constant(member != nil && !member.Optional && !typevalue.TypeIncludesNil(member.Type))
		default:
			return inspect.Constant(false)
		}
	})
}

// StaticIndexTypeUnderLengthFloor returns the exact selected member type under
// the premise len(value) >= floor. It succeeds only when every type arm still
// reachable under that premise has a required member at index. This is the
// canonical type-side projection for a range-certified constant Lua read.
func StaticIndexTypeUnderLengthFloor(t typ.Type, index, floor int64) (typ.Type, bool) {
	if !StaticIndexPresentUnderLengthFloor(t, index, floor) {
		return nil, false
	}
	memo := make(map[*typ.Recursive]*typ.Recursive)
	var project func(typ.Type) (typ.Type, bool)
	project = func(current typ.Type) (typ.Type, bool) {
		switch value := unwrap.Alias(current).(type) {
		case *typ.Optional:
			return project(value.Inner)
		case *typ.Union:
			members := make([]typ.Type, 0, len(value.Members))
			for _, member := range value.Members {
				if !CanHaveLengthAtLeast(member, floor) {
					continue
				}
				selected, ok := project(member)
				if !ok {
					return nil, false
				}
				members = append(members, selected)
			}
			if len(members) == 0 {
				return nil, false
			}
			return typeexpr.Union(members...), true
		case *typ.Recursive:
			if projected, ok := memo[value]; ok {
				return projected, true
			}
			projected := typ.NewRecursivePlaceholder(value.Name + "[index]")
			memo[value] = projected
			body, ok := project(value.Body)
			if !ok {
				delete(memo, value)
				return nil, false
			}
			projected.SetBody(body)
			return projected, true
		case *typ.Array:
			if value.Element == nil {
				return typ.Unknown, true
			}
			return value.Element, true
		case *typ.Tuple:
			if index < 1 || index > int64(len(value.Elements)) {
				return nil, false
			}
			selected := value.Elements[index-1]
			if selected == nil {
				selected = typ.Unknown
			}
			return selected, true
		case *typ.Record:
			member := value.GetStaticIntIndex(index)
			if member == nil || member.Optional {
				return nil, false
			}
			if member.Type == nil {
				return typ.Unknown, true
			}
			return member.Type, true
		default:
			return nil, false
		}
	}
	return project(t)
}
