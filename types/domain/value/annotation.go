package value

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// RefineStructuralAnnotation returns a refined structural annotation shape
// using evidence and the caller's slot join law.
func RefineStructuralAnnotation(
	annotation,
	evidence typ.Type,
	join func(typ.Type, typ.Type) typ.Type,
) (typ.Type, bool) {
	if join == nil {
		join = typ.JoinPreferNonSoft
	}
	return refineStructuralAnnotation(annotation, evidence, typ.NewGuard(), join)
}

func refineStructuralAnnotation(
	annotation,
	evidence typ.Type,
	guard internal.RecursionGuard,
	join func(typ.Type, typ.Type) typ.Type,
) (typ.Type, bool) {
	if annotation == nil || evidence == nil {
		return annotation, false
	}
	next, ok := guard.Enter(annotation)
	if !ok {
		return annotation, false
	}
	switch a := unwrap.Alias(annotation).(type) {
	case *typ.Optional:
		inner, changed := refineStructuralAnnotation(a.Inner, evidence, next, join)
		if !changed {
			return annotation, false
		}
		return typ.NewOptional(inner), true
	case *typ.Array:
		elem := arrayElementEvidence(evidence, next, join)
		if elem == nil {
			return annotation, false
		}
		refined := join(a.Element, elem)
		if typ.TypeEquals(refined, a.Element) {
			return annotation, false
		}
		return typ.NewArray(refined), true
	case *typ.Map:
		key, value, ok := mapEvidence(evidence, a.Key, next, join)
		if !ok {
			return annotation, false
		}
		refinedKey := a.Key
		if typ.IsAny(a.Key) || typ.IsUnknown(a.Key) {
			refinedKey = join(a.Key, key)
		}
		refinedValue := join(a.Value, value)
		if typ.TypeEquals(refinedKey, a.Key) && typ.TypeEquals(refinedValue, a.Value) {
			return annotation, false
		}
		return typ.NewMap(refinedKey, refinedValue), true
	case *typ.Record:
		return refineRecordAnnotation(a, evidence, next, join)
	case *typ.Union:
		members := make([]typ.Type, len(a.Members))
		changed := false
		for i, member := range a.Members {
			refined, memberChanged := refineStructuralAnnotation(member, evidence, next, join)
			members[i] = refined
			changed = changed || memberChanged
		}
		if !changed {
			return annotation, false
		}
		return typ.NewUnion(members...), true
	default:
		return annotation, false
	}
}

func refineRecordAnnotation(
	annotation *typ.Record,
	evidence typ.Type,
	guard internal.RecursionGuard,
	join func(typ.Type, typ.Type) typ.Type,
) (typ.Type, bool) {
	if annotation == nil {
		return nil, false
	}
	if annotation.Open && len(annotation.Fields) == 0 && !annotation.HasMapComponent() && annotation.Metatable == nil {
		return evidence, !typ.TypeEquals(annotation, evidence)
	}
	if !annotation.HasMapComponent() {
		return annotation, false
	}
	key, value, ok := mapEvidence(evidence, annotation.MapKey, guard, join)
	if !ok {
		return annotation, false
	}
	refinedKey := annotation.MapKey
	if typ.IsAny(annotation.MapKey) || typ.IsUnknown(annotation.MapKey) {
		refinedKey = join(annotation.MapKey, key)
	}
	refinedValue := join(annotation.MapValue, value)
	if typ.TypeEquals(refinedKey, annotation.MapKey) && typ.TypeEquals(refinedValue, annotation.MapValue) {
		return annotation, false
	}
	builder := typ.NewRecord().SetOpen(annotation.Open)
	if annotation.Metatable != nil {
		builder.Metatable(annotation.Metatable)
	}
	builder.MapComponent(refinedKey, refinedValue)
	for _, field := range annotation.Fields {
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	return builder.Build(), true
}

func arrayElementEvidence(
	evidence typ.Type,
	guard internal.RecursionGuard,
	join func(typ.Type, typ.Type) typ.Type,
) typ.Type {
	if evidence == nil {
		return nil
	}
	next, ok := guard.Enter(evidence)
	if !ok {
		return nil
	}
	switch e := unwrap.Alias(evidence).(type) {
	case *typ.Array:
		return e.Element
	case *typ.Tuple:
		var elem typ.Type
		for _, slot := range e.Elements {
			elem = join(elem, slot)
		}
		return elem
	case *typ.Union:
		var elem typ.Type
		for _, member := range e.Members {
			memberElem := arrayElementEvidence(member, next, join)
			if memberElem == nil {
				continue
			}
			elem = join(elem, memberElem)
		}
		return elem
	case *typ.Optional:
		return arrayElementEvidence(e.Inner, next, join)
	case *typ.Intersection:
		var elem typ.Type
		seen := false
		for _, member := range e.Members {
			memberElem := arrayElementEvidence(member, next, join)
			if memberElem == nil {
				continue
			}
			if !seen {
				elem = memberElem
				seen = true
				continue
			}
			elem = subtype.NormalizeIntersection(elem, memberElem)
		}
		return elem
	case *typ.Recursive:
		return arrayElementEvidence(e.Body, next, join)
	default:
		return nil
	}
}

func mapEvidence(
	evidence,
	expectedKey typ.Type,
	guard internal.RecursionGuard,
	join func(typ.Type, typ.Type) typ.Type,
) (typ.Type, typ.Type, bool) {
	if evidence == nil {
		return nil, nil, false
	}
	next, ok := guard.Enter(evidence)
	if !ok {
		return nil, nil, false
	}
	switch e := unwrap.Alias(evidence).(type) {
	case *typ.Map:
		if expectedKey != nil && !keyEvidenceCompatible(e.Key, expectedKey) {
			return nil, nil, false
		}
		return e.Key, e.Value, true
	case *typ.Record:
		if e.HasMapComponent() {
			if expectedKey != nil && !keyEvidenceCompatible(e.MapKey, expectedKey) {
				return nil, nil, false
			}
			return e.MapKey, e.MapValue, true
		}
		if len(e.Fields) == 0 {
			return nil, nil, false
		}
		var value typ.Type
		for _, field := range e.Fields {
			if expectedKey != nil && !keyEvidenceCompatible(typ.LiteralString(field.Name), expectedKey) {
				return nil, nil, false
			}
			value = join(value, field.Type)
		}
		return typ.String, value, true
	case *typ.Union:
		var key typ.Type
		var value typ.Type
		seen := false
		for _, member := range e.Members {
			memberKey, memberValue, ok := mapEvidence(member, expectedKey, next, join)
			if !ok {
				continue
			}
			key = join(key, memberKey)
			value = join(value, memberValue)
			seen = true
		}
		return key, value, seen
	case *typ.Optional:
		return mapEvidence(e.Inner, expectedKey, next, join)
	case *typ.Intersection:
		var key typ.Type
		var value typ.Type
		seen := false
		for _, member := range e.Members {
			memberKey, memberValue, ok := mapEvidence(member, expectedKey, next, join)
			if !ok {
				continue
			}
			if !seen {
				key = memberKey
				value = memberValue
				seen = true
				continue
			}
			key = subtype.NormalizeIntersection(key, memberKey)
			value = subtype.NormalizeIntersection(value, memberValue)
		}
		return key, value, seen
	case *typ.Recursive:
		return mapEvidence(e.Body, expectedKey, next, join)
	default:
		return nil, nil, false
	}
}

func keyEvidenceCompatible(candidate, expected typ.Type) bool {
	if candidate == nil || expected == nil {
		return false
	}
	return subtype.IsSubtype(candidate, expected) || typ.IsAny(expected) || typ.IsUnknown(expected)
}
