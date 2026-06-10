// Package normalize owns pure type normalization policies shared by relation,
// coalescing, and Lua access projection code.
package normalize

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// UnionForJoin applies the union normalization policy used by join-like
// evidence merging.
func UnionForJoin(members ...typ.Type) typ.Type {
	return unionForRelationEvidence(members)
}

// UnionForProjection applies the union normalization policy used by value
// projections such as Lua field and callable-return access.
func UnionForProjection(members ...typ.Type) typ.Type {
	return unionForRelationEvidence(members)
}

func unionForRelationEvidence(members []typ.Type) typ.Type {
	flat, hasNil, hasUnknown, hasAny := flattenUnionForRelationEvidence(members)
	if hasAny {
		return typ.Any
	}
	if len(flat) > 0 {
		flat = canonicalPolicyMembers(flat)
		flat = applyUnionSubsumption(flat)
	}
	if len(flat) == 0 && hasUnknown {
		if hasNil {
			return typ.NewOptional(typ.Unknown)
		}
		return typ.Unknown
	}
	if hasNil {
		flat = append(flat, typ.Nil)
	}
	return typ.NewUnion(flat...)
}

func flattenUnionForRelationEvidence(members []typ.Type) (flat []typ.Type, hasNil, hasUnknown, hasAny bool) {
	flat = make([]typ.Type, 0, len(members))
	var addMember func(typ.Type)
	addMember = func(member typ.Type) {
		if member == nil {
			return
		}
		unwrapped := typ.UnwrapAnnotated(member)
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
			hasAny = true
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
	return flat, hasNil, hasUnknown, hasAny
}

func canonicalPolicyMembers(members []typ.Type) []typ.Type {
	normalized := typ.NewUnion(members...)
	if u, ok := typ.UnwrapAnnotated(normalized).(*typ.Union); ok {
		out := make([]typ.Type, len(u.Members))
		copy(out, u.Members)
		return out
	}
	if normalized == typ.Never {
		return nil
	}
	return []typ.Type{normalized}
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
			switch lit.Base {
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
