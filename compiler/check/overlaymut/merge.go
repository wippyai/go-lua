package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// MergeFieldAssignments merges src into dst.
func MergeFieldAssignments(
	dst map[cfg.SymbolID]map[string]typ.Type,
	src map[cfg.SymbolID]map[string]typ.Type,
) {
	for _, sym := range cfg.SortedSymbolIDs(src) {
		fields := src[sym]
		if dst[sym] == nil {
			dst[sym] = make(map[string]typ.Type)
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			fieldType := fields[name]
			if existing := dst[sym][name]; existing != nil {
				dst[sym][name] = value.JoinPrecise(existing, fieldType)
			} else {
				dst[sym][name] = fieldType
			}
		}
	}
}

// ApplyFieldMergeToOverlay merges collected field assignments into symbol types in the overlay.
func ApplyFieldMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	fieldAssignments map[cfg.SymbolID]map[string]typ.Type,
) {
	for _, sym := range cfg.SortedSymbolIDs(fieldAssignments) {
		fields := fieldAssignments[sym]
		if len(fields) == 0 {
			continue
		}
		baseType := overlay[sym]
		merged := MergeFieldsIntoType(baseType, fields)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}

// MergeFieldsIntoType merges a set of field types into a base type.
func MergeFieldsIntoType(baseType typ.Type, fields map[string]typ.Type) typ.Type {
	return mergeFieldsIntoType(baseType, fields, false)
}

// MergeRequiredFieldsIntoType merges declared field-surface types into a base
// type and marks those fields present on the resulting record.
func MergeRequiredFieldsIntoType(baseType typ.Type, fields map[string]typ.Type) typ.Type {
	return mergeFieldsIntoType(baseType, fields, true)
}

func mergeFieldsIntoType(baseType typ.Type, fields map[string]typ.Type, required bool) typ.Type {
	if len(fields) == 0 {
		return baseType
	}

	fieldNames := cfg.SortedFieldNames(fields)

	if baseType == nil {
		builder := typ.NewRecord().SetOpen(true)
		for _, name := range fieldNames {
			builder.Field(name, fields[name])
		}
		return builder.Build()
	}

	switch v := baseType.(type) {
	case *typ.Map:
		builder := typ.NewRecord().SetOpen(true)
		builder.MapComponent(v.Key, v.Value)
		for _, name := range fieldNames {
			builder.Field(name, fields[name])
		}
		return builder.Build()
	case *typ.Record:
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		remaining := make(map[string]typ.Type, len(fields))
		for name, fieldType := range fields {
			remaining[name] = fieldType
		}
		for _, f := range v.Fields {
			fieldType := f.Type
			if next, ok := remaining[f.Name]; ok {
				fieldType = value.JoinPrecise(fieldType, next)
				delete(remaining, f.Name)
			}
			addMergedRecordField(builder, f, fieldType, required)
		}
		for _, name := range cfg.SortedFieldNames(remaining) {
			if remaining[name] != nil {
				builder.Field(name, remaining[name])
			}
		}
		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if v.HasMapComponent() {
			builder.MapComponent(v.MapKey, v.MapValue)
		}
		return builder.Build()
	default:
		builder := typ.NewRecord().SetOpen(true)
		for _, name := range fieldNames {
			builder.Field(name, fields[name])
		}
		return builder.Build()
	}
}

func addMergedRecordField(builder *typ.RecordBuilder, field typ.Field, fieldType typ.Type, required bool) {
	switch {
	case required && field.Readonly:
		builder.ReadonlyField(field.Name, fieldType)
	case required:
		builder.Field(field.Name, fieldType)
	case field.Optional && field.Readonly:
		builder.OptReadonlyField(field.Name, fieldType)
	case field.Optional:
		builder.OptField(field.Name, fieldType)
	case field.Readonly:
		builder.ReadonlyField(field.Name, fieldType)
	default:
		builder.Field(field.Name, fieldType)
	}
}

// ApplyMapMutatorMergeToOverlay adds map components to symbol types based on map-write evidence.
func ApplyMapMutatorMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	mapMutatorAssignments map[cfg.SymbolID][]MapMutatorInfo,
) {
	for _, sym := range cfg.SortedSymbolIDs(mapMutatorAssignments) {
		infos := mapMutatorAssignments[sym]
		if len(infos) == 0 {
			continue
		}

		var keyType, valType typ.Type
		for _, info := range infos {
			if mapMutatorAssignmentDeletesSlot(info.ValueType) {
				continue
			}
			keyType = typ.JoinPreferNonSoft(keyType, info.KeyType)
			valType = JoinValueTypes(valType, info.ValueType)
		}
		if valType == nil {
			continue
		}
		if keyType == nil {
			keyType = typ.String
		}

		baseType := overlay[sym]
		merged := MergeMapComponentIntoType(baseType, keyType, valType)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}

// JoinValueTypes joins two value types, preferring arrays over empty records.
func JoinValueTypes(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	aIsEmptyRecord := unwrap.IsEmptyRecord(a)
	bIsEmptyRecord := unwrap.IsEmptyRecord(b)
	_, aIsArray := a.(*typ.Array)
	_, bIsArray := b.(*typ.Array)
	aIsPlaceholder := a.Kind().IsPlaceholder()
	bIsPlaceholder := b.Kind().IsPlaceholder()

	if aIsEmptyRecord && bIsArray {
		return b
	}
	if bIsEmptyRecord && aIsArray {
		return a
	}
	if aIsPlaceholder && bIsArray {
		return b
	}
	if bIsPlaceholder && aIsArray {
		return a
	}

	return value.JoinPrecise(a, b)
}

func mapMutatorAssignmentDeletesSlot(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return mapMutatorAssignmentDeletesSlot(v.Target)
	case *typ.Optional:
		return mapMutatorAssignmentDeletesSlot(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !mapMutatorAssignmentDeletesSlot(member) {
				return false
			}
		}
		return true
	default:
		return unwrap.IsNilType(v)
	}
}

// MergeMapComponentIntoType adds a map component to a base type.
func MergeMapComponentIntoType(baseType, keyType, valType typ.Type) typ.Type {
	if baseType == nil {
		return typ.NewMap(keyType, valType)
	}

	switch v := baseType.(type) {
	case *typ.Map:
		newKey := typ.JoinPreferNonSoft(v.Key, keyType)
		newVal := value.JoinPrecise(v.Value, valType)
		return typ.NewMap(newKey, newVal)
	case *typ.Record:
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		for _, f := range v.Fields {
			builder.Field(f.Name, f.Type)
		}
		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if v.HasMapComponent() {
			newKey := typ.JoinPreferNonSoft(v.MapKey, keyType)
			newVal := value.JoinPrecise(v.MapValue, valType)
			builder.MapComponent(newKey, newVal)
		} else {
			existingKey := querycore.KeyType(v)
			if existingKey == nil {
				existingKey = typ.String
			}
			builder.MapComponent(typ.JoinPreferNonSoft(existingKey, keyType), valType)
		}
		return builder.Build()
	default:
		return typ.NewMap(keyType, valType)
	}
}

// ApplyDirectMutationsToOverlay widens array element types based on table.insert mutations.
func ApplyDirectMutationsToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	mutations map[cfg.SymbolID]typ.Type,
) {
	for _, sym := range cfg.SortedSymbolIDs(mutations) {
		elemType := mutations[sym]
		if elemType == nil {
			continue
		}
		baseType := overlay[sym]
		merged := value.AdmitArrayElementMutation(baseType, elemType, typ.JoinPreferNonSoft)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}
