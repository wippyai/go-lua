// Package normalize owns pure type normalization policies shared by relation,
// coalescing, and Lua access projection code.
package normalize

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/literal"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// UnionForEvidence applies the shared union normalization policy used when
// aggregating evidence or projecting values.
func UnionForEvidence(members ...typ.Type) typ.Type {
	flat, hasNil, hasUnknown := flattenUnionForRelationEvidence(members)
	if len(flat) > 0 {
		flat = applyUnionSubsumption(flat)
	}
	if len(flat) == 0 && hasUnknown {
		if hasNil {
			return Optional(typ.Unknown)
		}
		return typ.Unknown
	}
	if hasNil {
		nonNil := typ.MaterializeUnion(flat)
		if nonNil == typ.Never {
			return typ.Nil
		}
		if nonNil.Kind() != kind.Union {
			return typ.MaterializeOptional(nonNil)
		}

		union := nonNil.(*typ.Union)
		withNil := make([]typ.Type, 0, len(union.Members)+1)
		withNil = append(withNil, typ.Nil)
		withNil = append(withNil, union.Members...)
		return typ.MaterializeUnion(withNil)
	}
	return typ.MaterializeUnion(flat)
}

// Optional applies the shared semantic optional construction policy
// used when evidence proves a value may also be nil.
func Optional(inner typ.Type) typ.Type {
	if inner == nil {
		return typ.Nil
	}
	unwrapped := unwrap.Annotated(inner)
	if unwrapped == nil || unwrapped.Kind() == kind.Nil {
		return typ.Nil
	}
	if unwrapped.Kind() == kind.Optional {
		return inner
	}
	if typ.IsAny(unwrapped) {
		return typ.Any
	}
	if unwrapped.Kind() == kind.Union {
		return UnionForEvidence(typ.Nil, inner)
	}
	return typ.MaterializeOptional(inner)
}

func flattenUnionForRelationEvidence(members []typ.Type) (flat []typ.Type, hasNil, hasUnknown bool) {
	flat = make([]typ.Type, 0, len(members))
	var addMember func(typ.Type)
	addMember = func(member typ.Type) {
		if member == nil {
			return
		}
		unwrapped := unwrap.Annotated(member)
		if unwrapped == nil {
			return
		}
		switch unwrapped.Kind() {
		case kind.Never:
			return
		case kind.Unknown:
			hasUnknown = true
			return
		case kind.Any:
			flat = append(flat, member)
			return
		case kind.Nil:
			hasNil = true
			return
		case kind.Union:
			for _, nested := range unwrapped.(*typ.Union).Members {
				addMember(nested)
			}
			return
		case kind.Optional:
			hasNil = true
			addMember(unwrapped.(*typ.Optional).Inner)
			return
		default:
			flat = append(flat, member)
		}
	}
	for _, member := range members {
		addMember(member)
	}
	return flat, hasNil, hasUnknown
}

func applyUnionSubsumption(members []typ.Type) []typ.Type {
	if len(members) < 2 {
		return members
	}
	hasNumberType := false
	for _, member := range members {
		if member.Kind() == kind.Number {
			hasNumberType = true
			break
		}
	}
	if hasNumberType {
		filtered := members[:0]
		for _, member := range members {
			if member.Kind() == kind.Integer {
				continue
			}
			filtered = append(filtered, member)
		}
		members = filtered
	}

	var baseMask uint8
	for _, member := range members {
		switch member.Kind() {
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
	}
	if baseMask == 0 {
		return members
	}

	filtered := members[:0]
	for _, member := range members {
		if lit, ok := member.(*typ.Literal); ok {
			drop := false
			base := literal.PrimitiveBase(lit)
			if base == nil {
				filtered = append(filtered, member)
				continue
			}
			switch base.Kind() {
			case kind.String:
				drop = baseMask&1 != 0
			case kind.Number:
				drop = baseMask&2 != 0
			case kind.Integer:
				drop = baseMask&4 != 0 || baseMask&2 != 0
			case kind.Boolean:
				drop = baseMask&8 != 0
			}
			if drop {
				continue
			}
		}
		filtered = append(filtered, member)
	}
	return filtered
}
