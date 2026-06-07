package paramevidence

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// PathAccess describes whether parameter-path evidence proves a read
// requirement or a write requirement. Read evidence creates readonly fields;
// write evidence keeps only the final written slot mutable.
type PathAccess uint8

const (
	PathAccessRead PathAccess = iota
	PathAccessWrite
)

// MergeCallExpectation merges a call-boundary expected parameter type into the
// current inferred evidence. Parameter evidence uses the convergence law, with
// hard expected contracts allowed to dominate compatible passive body evidence.
func MergeCallExpectation(old, expected typ.Type, isParam bool) typ.Type {
	if typ.IsAny(expected) {
		return typ.Any
	}
	if typ.IsAny(old) {
		return typ.Any
	}
	if isParam {
		if expectedParamTypeDominates(old, expected) {
			return expected
		}
		return value.MergeForConvergence(old, expected)
	}
	if callExpectationCanRefineLocal(old) {
		return expected
	}
	return value.MergeForConvergence(old, expected)
}

func callExpectationCanRefineLocal(old typ.Type) bool {
	return old == nil ||
		typ.IsUnknown(old) ||
		typ.IsSoft(old, typ.SoftAnnotationPolicy)
}

func expectedParamTypeDominates(old, expected typ.Type) bool {
	if typ.IsAbsentOrUnknown(old) || typ.IsAbsentOrUnknown(expected) {
		return false
	}
	if typ.IsAny(old) || typ.IsAny(expected) || expected.Kind().IsPlaceholder() {
		return false
	}
	if subtype.IsSubtype(old, expected) {
		if typ.MorePrecise(old, expected) {
			return false
		}
		if typ.ContainsAny(expected) && !typ.TypeEquals(old, expected) {
			return false
		}
		return true
	}
	oldRec := recordForPathMerge(old)
	expectedRec := recordForPathMerge(expected)
	if oldRec == nil || expectedRec == nil {
		return false
	}
	return recordEvidenceCompatibleWithExpected(oldRec, expectedRec)
}

func recordEvidenceCompatibleWithExpected(old, expected *typ.Record) bool {
	if old == nil || expected == nil {
		return false
	}
	for _, field := range old.Fields {
		expectedField := expected.GetField(field.Name)
		if expectedField == nil {
			if expected.Open {
				continue
			}
			return false
		}
		if fieldEvidenceIsUnresolved(field.Type) {
			continue
		}
		expectedType := expectedField.Type
		if expectedField.Optional {
			expectedType = typ.NewOptional(expectedType)
		}
		if !evidenceTypeCompatibleWithExpected(field.Type, expectedType) {
			return false
		}
	}
	if old.HasMapComponent() {
		if !expected.HasMapComponent() {
			return false
		}
		if !fieldEvidenceIsUnresolved(old.MapKey) && !evidenceTypeCompatibleWithExpected(old.MapKey, expected.MapKey) {
			return false
		}
		if !fieldEvidenceIsUnresolved(old.MapValue) && !evidenceTypeCompatibleWithExpected(old.MapValue, expected.MapValue) {
			return false
		}
	}
	return true
}

func evidenceTypeCompatibleWithExpected(evidence, expected typ.Type) bool {
	if fieldEvidenceIsUnresolved(evidence) {
		return true
	}
	if evidence == nil || expected == nil {
		return false
	}
	if subtype.IsSubtype(evidence, expected) {
		return true
	}
	switch e := typ.UnwrapAnnotated(evidence).(type) {
	case *typ.Alias:
		return evidenceTypeCompatibleWithExpected(e.Target, expected)
	case *typ.Union:
		for _, member := range e.Members {
			if !evidenceTypeCompatibleWithExpected(member, expected) {
				return false
			}
		}
		return true
	case *typ.Record:
		if expectedMap := mapForEvidenceExpected(expected); expectedMap != nil {
			return recordEvidenceCompatibleWithExpectedMap(e, expectedMap)
		}
	}
	if opt, ok := typ.UnwrapAnnotated(expected).(*typ.Optional); ok {
		return evidenceTypeCompatibleWithExpected(evidence, opt.Inner)
	}
	return false
}

func mapForEvidenceExpected(t typ.Type) *typ.Map {
	for {
		switch v := typ.UnwrapAnnotated(t).(type) {
		case *typ.Alias:
			t = v.Target
		case *typ.Optional:
			t = v.Inner
		case *typ.Map:
			return v
		default:
			return nil
		}
	}
}

func recordEvidenceCompatibleWithExpectedMap(evidence *typ.Record, expected *typ.Map) bool {
	if evidence == nil || expected == nil {
		return false
	}
	for _, field := range evidence.Fields {
		keyType := typ.LiteralString(field.Name)
		if !evidenceTypeCompatibleWithExpected(keyType, expected.Key) {
			return false
		}
		if !evidenceTypeCompatibleWithExpected(field.Type, expected.Value) {
			return false
		}
	}
	if evidence.HasMapComponent() {
		if !evidenceTypeCompatibleWithExpected(evidence.MapKey, expected.Key) {
			return false
		}
		if !evidenceTypeCompatibleWithExpected(evidence.MapValue, expected.Value) {
			return false
		}
	}
	return true
}

func fieldEvidenceIsUnresolved(t typ.Type) bool {
	if typ.IsAbsentOrUnknown(t) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return fieldEvidenceIsUnresolved(v.Target)
	case *typ.Record:
		return len(v.Fields) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

// MergePathCallExpectation wraps a parameter path read into caller evidence.
func MergePathCallExpectation(old typ.Type, segments []constraint.Segment, expected typ.Type, isParam bool) typ.Type {
	if len(segments) == 0 {
		return MergeCallExpectation(old, expected, isParam)
	}
	if expected == nil || expected.Kind().IsPlaceholder() || typ.IsAbsentOrUnknown(expected) {
		return old
	}
	return MergeExpectedAtPath(old, segments, expected, isParam, PathAccessRead)
}

// MergeExpectedAtPath merges leaf evidence into a structural parameter path.
func MergeExpectedAtPath(base typ.Type, segments []constraint.Segment, expected typ.Type, isParam bool, access PathAccess) typ.Type {
	if len(segments) == 0 {
		return MergeCallExpectation(base, expected, isParam)
	}
	seg := segments[0]
	field, ok := constraint.SegmentFieldName(seg)
	if !ok {
		return base
	}

	rec := recordForPathMerge(base)
	child := typ.Type(nil)
	wasOptional := isParam
	if rec != nil {
		if existing := rec.GetField(field); existing != nil {
			child = existing.Type
			wasOptional = wasOptional || existing.Optional
		} else if rec.HasMapComponent() && rec.MapValue != nil {
			child = rec.MapValue
			wasOptional = true
		}
	}
	if child == nil {
		child = typ.Unknown
	}
	mergedChild := MergeExpectedAtPath(child, segments[1:], expected, isParam, access)
	if mergedChild == nil {
		return base
	}
	return setRecordFieldAccess(base, field, mergedChild, wasOptional, pathFieldReadonly(access, len(segments)))
}

// WidenArrayElementAtPath admits an array-element mutation under a parameter
// path while preserving readonly ownership for intermediate containers.
func WidenArrayElementAtPath(base typ.Type, segments []constraint.Segment, element typ.Type) typ.Type {
	if len(segments) == 0 {
		return value.AdmitArrayElementMutation(base, element, value.MergeForConvergence)
	}
	seg := segments[0]
	field, ok := constraint.SegmentFieldName(seg)
	if !ok {
		return base
	}

	rec := recordForPathMerge(base)
	child := typ.Type(nil)
	optional := false
	if rec != nil {
		if existing := rec.GetField(field); existing != nil {
			child = existing.Type
			optional = existing.Optional
		} else if rec.HasMapComponent() && rec.MapValue != nil {
			child = rec.MapValue
			optional = true
		}
	}
	updated := WidenArrayElementAtPath(child, segments[1:], element)
	if updated == nil {
		return base
	}
	return setRecordFieldAccess(base, field, updated, optional, true)
}

func pathFieldReadonly(access PathAccess, remainingSegments int) bool {
	return access == PathAccessRead || remainingSegments > 1
}

func recordForPathMerge(t typ.Type) *typ.Record {
	for {
		switch v := typ.UnwrapAnnotated(t).(type) {
		case *typ.Alias:
			t = v.Target
		case *typ.Optional:
			t = v.Inner
		case *typ.Record:
			return v
		default:
			return nil
		}
	}
}

func setRecordFieldAccess(base typ.Type, field string, fieldType typ.Type, optional bool, readonly bool) typ.Type {
	if field == "" || fieldType == nil {
		return base
	}
	switch v := typ.UnwrapAnnotated(base).(type) {
	case *typ.Alias:
		updated := setRecordFieldAccess(v.Target, field, fieldType, optional, readonly)
		if updated == nil || typ.TypeEquals(updated, v.Target) {
			return base
		}
		return typ.NewAlias(v.Name, updated)
	case *typ.Union:
		updated := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if member == nil || typ.IsAny(member) || typ.TypeEquals(member, typ.Nil) {
				updated = append(updated, member)
				continue
			}
			next := setRecordFieldAccess(member, field, fieldType, optional, readonly)
			if next == nil {
				next = member
			}
			if !typ.TypeEquals(member, next) {
				changed = true
			}
			updated = append(updated, next)
		}
		if !changed {
			return base
		}
		return typ.NewUnion(updated...)
	case *typ.Optional:
		updated := setRecordFieldAccess(v.Inner, field, fieldType, optional, readonly)
		if updated == nil || typ.TypeEquals(updated, v.Inner) {
			return base
		}
		return typ.NewOptional(updated)
	case *typ.Record:
		return rebuildRecordWithField(v, field, fieldType, optional, readonly)
	default:
		builder := typ.NewRecord().SetOpen(true)
		addRecordField(builder, field, fieldType, optional, readonly)
		return builder.Build()
	}
}

func rebuildRecordWithField(rec *typ.Record, field string, fieldType typ.Type, optional bool, readonly bool) typ.Type {
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
			addRecordField(builder, f.Name, f.Type, f.Optional, f.Readonly)
			continue
		}
		addRecordField(builder, f.Name, fieldType, optional || f.Optional, f.Readonly && readonly)
		added = true
	}
	if !added {
		addRecordField(builder, field, fieldType, optional, readonly)
	}
	return builder.Build()
}

func addRecordField(builder *typ.RecordBuilder, name string, fieldType typ.Type, optional, readonly bool) {
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
