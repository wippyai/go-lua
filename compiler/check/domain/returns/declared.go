package returns

import (
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// RefineDeclaredReturnVector merges compatible body evidence into a declared
// return tuple without letting the body replace an incompatible public contract.
func RefineDeclaredReturnVector(declared, evidence []typ.Type) ([]typ.Type, bool) {
	if len(declared) == 0 || len(evidence) == 0 || len(declared) != len(evidence) {
		return nil, false
	}
	out := make([]typ.Type, len(declared))
	copy(out, declared)
	changed := false
	for i := range declared {
		refined, ok := RefineDeclaredReturnType(declared[i], evidence[i])
		if !ok || typ.TypeEquals(refined, declared[i]) {
			continue
		}
		out[i] = refined
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
}

// RefineDeclaredReturnType refines a single declared return slot with compatible
// same-expression evidence. Top-level any/unknown remain declared carriers; soft
// dynamic structure nested inside records, maps, arrays, and optionals can be
// tightened by concrete body evidence.
func RefineDeclaredReturnType(annotation, evidence typ.Type) (typ.Type, bool) {
	return refineDeclaredReturnType(annotation, evidence, true)
}

func refineDeclaredReturnType(annotation, evidence typ.Type, topLevel bool) (typ.Type, bool) {
	annotation = subst.ExpandInstantiated(annotation)
	evidence = subst.ExpandInstantiated(evidence)
	if annotation == nil || evidence == nil || typ.IsAbsentOrUnknown(evidence) {
		return annotation, false
	}
	if topLevel && (typ.IsAny(annotation) || typ.IsAbsentOrUnknown(annotation)) {
		return annotation, false
	}
	if !topLevel && (typ.IsAny(annotation) || typ.IsAbsentOrUnknown(annotation)) {
		return evidence, !typ.TypeEquals(annotation, evidence)
	}
	if typ.MorePrecise(evidence, annotation) {
		return evidence, true
	}
	if refined, ok := refineDeclaredStructuralReturn(annotation, evidence); ok {
		return refined, true
	}
	if refined, ok := value.RefineStructuralAnnotation(annotation, evidence, typ.JoinPreferNonSoft); ok && typ.MorePrecise(refined, annotation) {
		return refined, true
	}
	return annotation, false
}

func refineDeclaredStructuralReturn(annotation, evidence typ.Type) (typ.Type, bool) {
	switch a := unwrap.Alias(annotation).(type) {
	case *typ.Optional:
		innerEvidence := nonNilEvidence(evidence)
		if innerEvidence == nil {
			return annotation, false
		}
		inner, changed := refineDeclaredReturnType(a.Inner, innerEvidence, false)
		if !changed {
			return annotation, false
		}
		return typ.NewOptional(inner), true
	case *typ.Array:
		elemEvidence := declaredArrayElementEvidence(evidence)
		if elemEvidence == nil {
			return annotation, false
		}
		elem, changed := refineDeclaredReturnType(a.Element, elemEvidence, false)
		if !changed {
			return annotation, false
		}
		return typ.NewArray(elem), true
	case *typ.Map:
		rec := declaredRecordEvidence(evidence)
		if rec == nil {
			return annotation, false
		}
		return refineDeclaredMapWithRecord(a.Key, a.Value, rec, annotation)
	case *typ.Record:
		rec := declaredRecordEvidence(evidence)
		if rec == nil {
			return annotation, false
		}
		return refineDeclaredRecordWithRecord(a, rec)
	default:
		return annotation, false
	}
}

func nonNilEvidence(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if opt, ok := t.(*typ.Optional); ok {
		return opt.Inner
	}
	if t != nil && t.Kind() == kind.Nil {
		return nil
	}
	return t
}

func declaredArrayElementEvidence(t typ.Type) typ.Type {
	switch e := unwrap.Alias(t).(type) {
	case *typ.Array:
		return e.Element
	case *typ.Tuple:
		var elem typ.Type
		for _, slot := range e.Elements {
			elem = typ.JoinReturnSlot(elem, slot)
		}
		return elem
	case *typ.Optional:
		return declaredArrayElementEvidence(e.Inner)
	case *typ.Union:
		var elem typ.Type
		for _, member := range e.Members {
			if member == nil || member.Kind() == kind.Nil {
				continue
			}
			memberElem := declaredArrayElementEvidence(member)
			if memberElem == nil {
				continue
			}
			elem = typ.JoinReturnSlot(elem, memberElem)
		}
		return elem
	default:
		return nil
	}
}

func declaredRecordEvidence(t typ.Type) *typ.Record {
	switch e := unwrap.Alias(t).(type) {
	case *typ.Record:
		return e
	case *typ.Optional:
		return declaredRecordEvidence(e.Inner)
	default:
		return nil
	}
}

func refineDeclaredMapWithRecord(key, valueType typ.Type, evidence *typ.Record, original typ.Type) (typ.Type, bool) {
	if evidence == nil {
		return original, false
	}
	refinedMapValue, mapChanged := refineDeclaredMapValue(key, valueType, evidence)
	builder := typ.NewRecord().MapComponent(key, refinedMapValue)
	addedField := false
	for _, field := range evidence.Fields {
		if !declaredStringKeyCompatible(field.Name, key) {
			continue
		}
		fieldType := declaredMapFieldType(valueType, field.Type)
		addRecordField(builder, typ.Field{
			Name:     field.Name,
			Type:     fieldType,
			Optional: field.Optional,
			Readonly: field.Readonly,
		})
		addedField = true
	}
	if !mapChanged && !addedField {
		return original, false
	}
	return builder.Build(), true
}

func refineDeclaredRecordWithRecord(annotation *typ.Record, evidence *typ.Record) (typ.Type, bool) {
	if annotation == nil || evidence == nil {
		return annotation, false
	}
	allowNewFields := annotation.Open || annotation.HasMapComponent()
	refinedMapValue := annotation.MapValue
	mapChanged := false
	if annotation.HasMapComponent() {
		refinedMapValue, mapChanged = refineDeclaredMapValue(annotation.MapKey, annotation.MapValue, evidence)
	}

	fields := make(map[string]typ.Field, len(annotation.Fields)+len(evidence.Fields))
	order := make([]string, 0, len(annotation.Fields)+len(evidence.Fields))
	for _, field := range annotation.Fields {
		fields[field.Name] = field
		order = append(order, field.Name)
	}

	fieldChanged := false
	for _, evidenceField := range evidence.Fields {
		if existing, ok := fields[evidenceField.Name]; ok {
			refined, changed := refineDeclaredReturnType(existing.Type, evidenceField.Type, false)
			if changed {
				existing.Type = refined
				fields[evidenceField.Name] = existing
				fieldChanged = true
			}
			continue
		}
		if !allowNewFields {
			continue
		}
		if annotation.HasMapComponent() && !declaredStringKeyCompatible(evidenceField.Name, annotation.MapKey) {
			continue
		}
		fieldType := evidenceField.Type
		if annotation.HasMapComponent() {
			fieldType = declaredMapFieldType(annotation.MapValue, evidenceField.Type)
		}
		fields[evidenceField.Name] = typ.Field{
			Name:     evidenceField.Name,
			Type:     fieldType,
			Optional: evidenceField.Optional,
			Readonly: evidenceField.Readonly,
		}
		order = append(order, evidenceField.Name)
		fieldChanged = true
	}

	if !mapChanged && !fieldChanged {
		return annotation, false
	}
	builder := typ.NewRecord().SetOpen(annotation.Open)
	if annotation.Metatable != nil {
		builder.Metatable(annotation.Metatable)
	}
	if annotation.HasMapComponent() {
		builder.MapComponent(annotation.MapKey, refinedMapValue)
	}
	for _, member := range annotation.StaticMembers {
		builder.AddStaticMember(member)
	}
	for _, name := range order {
		addRecordField(builder, fields[name])
	}
	return builder.Build(), true
}

func refineDeclaredMapValue(key, valueType typ.Type, evidence *typ.Record) (typ.Type, bool) {
	var valueEvidence typ.Type
	for _, field := range evidence.Fields {
		if !declaredStringKeyCompatible(field.Name, key) {
			continue
		}
		valueEvidence = typ.JoinReturnSlot(valueEvidence, field.Type)
	}
	if valueEvidence == nil {
		return valueType, false
	}
	return refineDeclaredReturnType(valueType, valueEvidence, false)
}

func declaredMapFieldType(annotation, evidence typ.Type) typ.Type {
	refined, changed := refineDeclaredReturnType(annotation, evidence, false)
	if changed {
		return refined
	}
	return annotation
}

func declaredStringKeyCompatible(name string, key typ.Type) bool {
	if key == nil {
		return false
	}
	return typ.IsAny(key) || typ.IsUnknown(key) || subtype.IsSubtype(typ.LiteralString(name), key)
}

func addRecordField(builder *typ.RecordBuilder, field typ.Field) {
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
