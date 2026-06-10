package relation

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizeUnionForJoin applies the union normalization policy explicitly
// requested by relation and join code.
func NormalizeUnionForJoin(members ...Type) Type {
	return normalizeUnionForRelationEvidence(members)
}

// NormalizeUnionForProjection applies the relation-owned union policy for
// projected values, such as Lua field and callable return projections.
func NormalizeUnionForProjection(members ...Type) Type {
	return normalizeUnionForRelationEvidence(members)
}

func normalizeUnionForRelationEvidence(members []Type) Type {
	flat, hasNil, hasUnknown, hasAny := flattenUnionForRelationEvidence(members)
	if hasAny {
		return Any
	}
	if len(flat) > 0 {
		flat = canonicalJoinPolicyMembers(flat)
		flat = applyJoinUnionSubsumption(flat)
	}
	if len(flat) == 0 && hasUnknown {
		if hasNil {
			return NewOptional(Unknown)
		}
		return Unknown
	}
	if hasNil {
		flat = append(flat, Nil)
	}
	return NewUnion(flat...)
}

func flattenUnionForRelationEvidence(members []Type) (flat []Type, hasNil, hasUnknown, hasAny bool) {
	flat = make([]Type, 0, len(members))
	var addMember func(Type)
	addMember = func(member Type) {
		if member == nil {
			return
		}
		unwrapped := UnwrapAnnotated(member)
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
			for _, nested := range unwrapped.(*Union).Members {
				addMember(nested)
			}
			return
		case kind.Optional:
			hasNil = true
			addMember(unwrapped.(*Optional).Inner)
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

func canonicalJoinPolicyMembers(members []Type) []Type {
	normalized := NewUnion(members...)
	if u, ok := UnwrapAnnotated(normalized).(*Union); ok {
		out := make([]Type, len(u.Members))
		copy(out, u.Members)
		return out
	}
	if normalized == Never {
		return nil
	}
	return []Type{normalized}
}

func applyJoinUnionSubsumption(members []Type) []Type {
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
		if lit, ok := member.(*Literal); ok {
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
