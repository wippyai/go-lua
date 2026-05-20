package value

import (
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// JoinMapRecordShape joins a map shape with a record carrying a map component.
// The caller supplies the slot join law; this package owns only the structural
// table reconstruction.
func JoinMapRecordShape(a, b typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	if joined, ok := joinMapRecordShapeDirected(a, b, join); ok {
		return joined, true
	}
	return joinMapRecordShapeDirected(b, a, join)
}

func joinMapRecordShapeDirected(mapType, recordType typ.Type, join func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	m, ok := unwrap.Alias(mapType).(*typ.Map)
	if !ok || m == nil {
		return nil, false
	}
	r, ok := unwrap.Alias(recordType).(*typ.Record)
	if !ok || r == nil || !r.HasMapComponent() {
		return nil, false
	}

	key := join(m.Key, r.MapKey)
	value := join(m.Value, r.MapValue)
	if len(r.Fields) == 0 && r.Metatable == nil {
		return typ.NewMap(key, value), true
	}
	builder := typ.NewRecord()
	if r.Open {
		builder.SetOpen(true)
	}
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	builder.MapComponent(key, value)
	for _, field := range r.Fields {
		fieldType := field.Type
		optional := true
		if subtype.IsSubtype(typ.LiteralString(field.Name), key) {
			fieldType = join(field.Type, value)
		} else {
			optional = field.Optional
		}
		switch {
		case optional && field.Readonly:
			builder.OptReadonlyField(field.Name, fieldType)
		case optional:
			builder.OptField(field.Name, fieldType)
		case field.Readonly:
			builder.ReadonlyField(field.Name, fieldType)
		default:
			builder.Field(field.Name, fieldType)
		}
	}
	return builder.Build(), true
}

// CollapseTableTopEvidence collapses unions where builtin table top already
// covers concrete table-shaped evidence.
func CollapseTableTopEvidence(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Alias:
		target := CollapseTableTopEvidence(v.Target)
		if target != nil && !typ.TypeEquals(target, v.Target) {
			return typ.NewAlias(v.Name, target)
		}
		return t
	case *typ.Optional:
		inner := CollapseTableTopEvidence(v.Inner)
		if inner != nil && !typ.TypeEquals(inner, v.Inner) {
			return typ.NewOptional(inner)
		}
		return t
	case *typ.Union:
		return collapseTableTopUnion(v)
	default:
		return t
	}
}

func collapseTableTopUnion(u *typ.Union) typ.Type {
	if u == nil {
		return nil
	}
	tableTop := firstTableTopMember(u.Members)
	members := make([]typ.Type, 0, len(u.Members))
	changed := false

	if tableTop == nil {
		for _, member := range u.Members {
			collapsed := CollapseTableTopEvidence(member)
			if !typ.TypeEquals(collapsed, member) {
				changed = true
			}
			members = append(members, collapsed)
		}
		if !changed {
			return u
		}
		return typ.NewUnion(members...)
	}

	tableAdded := false
	for _, member := range u.Members {
		if member == nil {
			continue
		}
		if unwrap.IsNilType(typ.UnwrapAnnotated(member)) {
			members = append(members, member)
			continue
		}
		collapsed := CollapseTableTopEvidence(member)
		if TableTopCovers(collapsed) {
			if !tableAdded {
				members = append(members, tableTop)
				tableAdded = true
			}
			if !typ.TypeEquals(member, tableTop) {
				changed = true
			}
			continue
		}
		if !typ.TypeEquals(collapsed, member) {
			changed = true
		}
		members = append(members, collapsed)
	}
	if !changed {
		return u
	}
	return typ.NewUnion(members...)
}

func firstTableTopMember(members []typ.Type) typ.Type {
	for _, member := range members {
		if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(member)) {
			return member
		}
	}
	return nil
}

// SelectTableUpperBound returns the table-top upper bound when one candidate
// already covers the other table-shaped candidate.
func SelectTableUpperBound(a, b typ.Type) (typ.Type, bool) {
	if isOnlyTableTop(a) && typ.IsAny(b) {
		return a, true
	}
	if isOnlyTableTop(b) && typ.IsAny(a) {
		return b, true
	}
	if containsTableTop(a) && TableTopCovers(b) && subtype.IsSubtype(b, a) {
		return a, true
	}
	if containsTableTop(b) && TableTopCovers(a) && subtype.IsSubtype(a, b) {
		return b, true
	}
	return nil, false
}

func containsTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return containsTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return containsTableTop(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if containsTableTop(member) {
				return true
			}
		}
	}
	return false
}

func isOnlyTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return isOnlyTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return isOnlyTableTop(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		hasTableTop := false
		for _, member := range v.Members {
			if unwrap.IsNilType(member) {
				continue
			}
			if !isOnlyTableTop(member) {
				return false
			}
			hasTableTop = true
		}
		return hasTableTop
	default:
		return false
	}
}

// TableTopCovers reports whether builtin table top can cover t.
func TableTopCovers(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) {
		return true
	}
	if unwrap.IsNilType(t) || unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return TableTopCovers(v.UnaliasedTarget())
	case *typ.Optional:
		return TableTopCovers(v.Inner)
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && TableTopCovers(v.Body)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !TableTopCovers(member) {
				return false
			}
		}
		return true
	case *typ.Record, *typ.Map, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return false
	}
}

// RefinesTableKeyByTruthiness reports whether candidate preserves baseline's
// table shape while replacing a stale falsy table-key component with its truthy
// refinement.
func RefinesTableKeyByTruthiness(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	candidateInner, _ := SplitNilable(candidate)
	baselineInner, _ := SplitNilable(baseline)
	if candidateInner == nil || baselineInner == nil {
		return false
	}
	return nonNilRefinesTableKeyByTruthiness(candidateInner, baselineInner)
}

func nonNilRefinesTableKeyByTruthiness(candidate, baseline typ.Type) bool {
	candidate = unwrap.Alias(candidate)
	baseline = unwrap.Alias(baseline)
	switch b := baseline.(type) {
	case *typ.Record:
		c, ok := candidate.(*typ.Record)
		return ok && recordRefinesTableKeyByTruthiness(c, b)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		return ok && mapKeyRefinesByTruthiness(c.Key, c.Value, b.Key, b.Value)
	default:
		return false
	}
}

func recordRefinesTableKeyByTruthiness(candidate, baseline *typ.Record) bool {
	return sameRecordFrameEquivalent(candidate, baseline) &&
		candidate.HasMapComponent() && baseline.HasMapComponent() &&
		mapKeyRefinesByTruthiness(candidate.MapKey, candidate.MapValue, baseline.MapKey, baseline.MapValue)
}

func mapKeyRefinesByTruthiness(candidateKey, candidateValue, baselineKey, baselineValue typ.Type) bool {
	return IsTruthyRefinement(candidateKey, baselineKey) && Equivalent(candidateValue, baselineValue)
}

func sameRecordFrameEquivalent(candidate, baseline *typ.Record) bool {
	if candidate == nil || baseline == nil || candidate.Open != baseline.Open || len(candidate.Fields) != len(baseline.Fields) {
		return false
	}
	if (candidate.Metatable == nil) != (baseline.Metatable == nil) {
		return false
	}
	if candidate.Metatable != nil && !typ.TypeEquals(candidate.Metatable, baseline.Metatable) {
		return false
	}
	for i, field := range candidate.Fields {
		other := baseline.Fields[i]
		if field.Name != other.Name || field.Optional != other.Optional || field.Readonly != other.Readonly {
			return false
		}
		if !Equivalent(field.Type, other.Type) {
			return false
		}
	}
	return true
}
