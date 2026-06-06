package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// AssignmentEvidenceType reconciles a statically extracted assignment type with
// the solved source-owned value. It is the assignment transfer law used by
// post-fixpoint fact reducers.
func AssignmentEvidenceType(staticType, sourceType typ.Type) typ.Type {
	if sourceType == nil {
		return staticType
	}
	if staticType == nil || typ.TypeEquals(sourceType, staticType) {
		return sourceType
	}
	if typ.MorePrecise(staticType, sourceType) {
		return staticType
	}
	if typ.ContainsTypeParam(sourceType) && !typ.ContainsTypeParam(staticType) {
		return staticType
	}
	if typ.ContainsTypeParam(staticType) && !typ.ContainsTypeParam(sourceType) {
		return sourceType
	}
	if sourceType.Kind().IsPlaceholder() && !staticType.Kind().IsPlaceholder() {
		return staticType
	}
	if staticType.Kind().IsPlaceholder() {
		return sourceType
	}
	return sourceType
}

// ApplyValueTemplate overlays flow-resolved source slots into an extracted value
// template. It is shared by current transfer and producer-neutral mutation fact
// surfaces so postflow observes the same nested value law as transfer.
func ApplyValueTemplate(base typ.Type, template ValueTemplate, sourceType func(AssignmentSource) typ.Type) typ.Type {
	out := base
	if sourceType == nil {
		return out
	}
	for _, slot := range template.Slots {
		if len(slot.Segments) == 0 || slot.Source.Kind == AssignmentSourceStatic {
			continue
		}
		slotType := sourceType(slot.Source)
		if typ.IsAbsentOrUnknown(slotType) {
			continue
		}
		out = setValueTemplateSlot(out, slot.Segments, slotType)
	}
	return out
}

func setValueTemplateSlot(base typ.Type, segments []constraint.Segment, valueType typ.Type) typ.Type {
	if len(segments) == 0 || valueType == nil {
		return base
	}
	seg := segments[0]
	switch v := typ.UnwrapAnnotated(base).(type) {
	case *typ.Alias:
		updated := setValueTemplateSlot(v.Target, segments, valueType)
		if updated == nil || typ.TypeEquals(updated, v.Target) {
			return base
		}
		return typ.NewAlias(v.Name, updated)
	case *typ.Optional:
		updated := setValueTemplateSlot(v.Inner, segments, valueType)
		if updated == nil || typ.TypeEquals(updated, v.Inner) {
			return base
		}
		return typ.NewOptional(updated)
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			updated := setValueTemplateSlot(member, segments, valueType)
			if updated == nil {
				updated = member
			}
			if !value.FactTypeEqual(member, updated) {
				changed = true
			}
			members = append(members, updated)
		}
		if !changed {
			return base
		}
		return typ.NewUnion(members...)
	case *typ.Record:
		field, ok := valueTemplateFieldSegment(seg)
		if !ok {
			return base
		}
		child := typ.Type(nil)
		optional := false
		if existing := v.GetField(field); existing != nil {
			child = existing.Type
			optional = existing.Optional
		} else if v.HasMapComponent() {
			child = v.MapValue
			optional = true
		}
		updated := setValueTemplateSlot(child, segments[1:], valueType)
		if updated == nil {
			return base
		}
		return rebuildValueTemplateRecord(v, field, updated, optional)
	case *typ.Tuple:
		if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 || int(seg.Index) > len(v.Elements) {
			return base
		}
		idx := int(seg.Index) - 1
		updated := setValueTemplateSlot(v.Elements[idx], segments[1:], valueType)
		if updated == nil || typ.TypeEquals(updated, v.Elements[idx]) {
			return base
		}
		elements := make([]typ.Type, len(v.Elements))
		copy(elements, v.Elements)
		elements[idx] = updated
		return typ.NewTuple(elements...)
	case *typ.Array:
		if seg.Kind != constraint.SegmentIndexInt {
			return base
		}
		updated := setValueTemplateSlot(v.Element, segments[1:], valueType)
		if updated == nil || typ.TypeEquals(updated, v.Element) {
			return base
		}
		return typ.NewArray(updated)
	default:
		return base
	}
}

func valueTemplateFieldSegment(seg constraint.Segment) (string, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func rebuildValueTemplateRecord(rec *typ.Record, field string, fieldType typ.Type, optional bool) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	added := false
	for _, f := range rec.Fields {
		if f.Name != field {
			addValueTemplateRecordField(builder, f.Name, f.Type, f.Optional, f.Readonly)
			continue
		}
		addValueTemplateRecordField(builder, f.Name, fieldType, optional || f.Optional, f.Readonly)
		added = true
	}
	if !added {
		addValueTemplateRecordField(builder, field, fieldType, optional, false)
	}
	return builder.Build()
}

func addValueTemplateRecordField(builder *typ.RecordBuilder, name string, fieldType typ.Type, optional, readonly bool) {
	switch {
	case optional && readonly:
		builder.OptReadonlyField(name, fieldType)
	case optional:
		builder.OptField(name, fieldType)
	case readonly:
		builder.ReadonlyField(name, fieldType)
	default:
		builder.Field(name, fieldType)
	}
}

// NormalizeDynamicKeyType is the dynamic-key normalization law shared by current
// transfer and producer-neutral mutation fact surfaces.
func NormalizeDynamicKeyType(keyType typ.Type) typ.Type {
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		return typ.Unknown
	}
	if typ.UnwrapAnnotated(keyType).Kind() == kind.Literal {
		return keyType
	}
	return subtype.Widen(keyType)
}

// IndexWriteReadCanUseKeyValueOnly reports whether a dynamic indexed read may
// consume readback proof without a stable key path. Without path identity the
// proof is exact only for literal keys; non-literal keys need a key-path proof so
// different dynamic writes do not collapse into one readback bucket.
func IndexWriteReadCanUseKeyValueOnly(keyType typ.Type) bool {
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		return false
	}
	return typ.UnwrapAnnotated(keyType).Kind() == kind.Literal
}
