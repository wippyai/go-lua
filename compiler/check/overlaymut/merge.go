package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
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
				dst[sym][name] = typ.JoinPreferNonSoft(existing, fieldType)
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
		existing := make(map[string]bool)
		for _, f := range v.Fields {
			builder.Field(f.Name, f.Type)
			existing[f.Name] = true
		}
		for _, name := range fieldNames {
			if !existing[name] {
				builder.Field(name, fields[name])
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

// ApplyIndexerMergeToOverlay adds map components to symbol types based on dynamic index assignments.
func ApplyIndexerMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	indexerAssignments map[cfg.SymbolID][]mutator.IndexerInfo,
) {
	for _, sym := range cfg.SortedSymbolIDs(indexerAssignments) {
		infos := indexerAssignments[sym]
		if len(infos) == 0 {
			continue
		}

		var keyType, valType typ.Type
		for _, info := range infos {
			keyType = typ.JoinPreferNonSoft(keyType, info.KeyType)
			valType = JoinValueTypes(valType, info.ValType)
		}
		if keyType == nil {
			keyType = typ.String
		}
		if valType == nil {
			valType = typ.Unknown
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

	return typ.JoinPreferNonSoft(a, b)
}

// MergeMapComponentIntoType adds a map component to a base type.
func MergeMapComponentIntoType(baseType, keyType, valType typ.Type) typ.Type {
	if baseType == nil {
		return typ.NewMap(keyType, valType)
	}

	switch v := baseType.(type) {
	case *typ.Map:
		newKey := typ.JoinPreferNonSoft(v.Key, keyType)
		newVal := typ.JoinPreferNonSoft(v.Value, valType)
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
			newVal := typ.JoinPreferNonSoft(v.MapValue, valType)
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
		merged := flow.WidenArrayElementType(baseType, elemType, typ.JoinPreferNonSoft)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}

// FieldWriteSets returns field assignments collected per symbol as field-write
// sets of the symbols' own tables.
func FieldWriteSets(fieldAssignments map[cfg.SymbolID]map[string]typ.Type) map[cfg.SymbolID]api.FieldWriteSet {
	result := make(map[cfg.SymbolID]api.FieldWriteSet, len(fieldAssignments))
	for _, sym := range cfg.SortedSymbolIDs(fieldAssignments) {
		fields := fieldAssignments[sym]
		set := make(api.FieldWriteSet, len(fields))
		for _, name := range cfg.SortedFieldNames(fields) {
			set[api.FieldWriteKey{Field: name}] = fields[name]
		}
		result[sym] = set
	}
	return result
}

// MergeFieldWriteSets merges src into dst: written fields join preferring
// non-soft types, and the maps written by dynamic keys join component-wise.
func MergeFieldWriteSets(dst, src map[cfg.SymbolID]api.FieldWriteSet) {
	for _, sym := range cfg.SortedSymbolIDs(src) {
		set := dst[sym]
		if set == nil {
			set = make(api.FieldWriteSet, len(src[sym]))
			dst[sym] = set
		}
		for _, key := range api.SortedFieldWriteKeys(src[sym]) {
			written := src[sym][key]
			existing := set[key]
			switch {
			case existing == nil:
				set[key] = written
			case key.IsIndexer():
				set[key] = api.JoinFieldWrite(key, existing, written)
			default:
				set[key] = typ.JoinPreferNonSoft(existing, written)
			}
		}
	}
}

// ApplyFieldWritesToOverlay merges field writes into symbol types in the
// overlay. Each write lands on the table its key locates below the symbol:
// named fields merge as by MergeFieldsIntoType, and writes by dynamic keys add
// their map component as by MergeMapComponentIntoType. A write below a path
// the symbol's type does not describe as a record field leaves the type as is.
func ApplyFieldWritesToOverlay(overlay map[cfg.SymbolID]typ.Type, writes map[cfg.SymbolID]api.FieldWriteSet) {
	for _, sym := range cfg.SortedSymbolIDs(writes) {
		byPath := make(map[string]map[string]typ.Type)
		var paths []string
		for _, key := range api.SortedFieldWriteKeys(writes[sym]) {
			if byPath[key.Path] == nil {
				byPath[key.Path] = make(map[string]typ.Type)
				paths = append(paths, key.Path)
			}
			byPath[key.Path][key.Field] = writes[sym][key]
		}
		t := overlay[sym]
		for _, path := range paths {
			fields := byPath[path]
			t = mergeAtPath(t, api.FieldWriteKey{Path: path}.Segments(), func(table typ.Type) typ.Type {
				return mergeWrittenFields(table, fields)
			})
		}
		if t != nil {
			overlay[sym] = t
		}
	}
}

// mergeWrittenFields merges the fields written into one table into its type.
func mergeWrittenFields(table typ.Type, fields map[string]typ.Type) typ.Type {
	named := make(map[string]typ.Type, len(fields))
	for name, t := range fields {
		if name != flow.IndexerWriteField {
			named[name] = t
		}
	}
	table = MergeFieldsIntoType(table, named)
	if m, ok := fields[flow.IndexerWriteField].(*typ.Map); ok {
		table = MergeMapComponentIntoType(table, m.Key, m.Value)
	}
	return table
}

// mergeAtPath rebuilds t with apply run on the type of the record field at
// segments below it.
func mergeAtPath(t typ.Type, segments []constraint.Segment, apply func(typ.Type) typ.Type) typ.Type {
	if len(segments) == 0 {
		return apply(t)
	}
	switch v := t.(type) {
	case *typ.Optional:
		inner := mergeAtPath(v.Inner, segments, apply)
		if inner == v.Inner {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Record:
		seg := segments[0]
		if seg.Kind == constraint.SegmentIndexInt {
			return t
		}
		field := v.GetField(seg.Name)
		if field == nil {
			return t
		}
		merged := mergeAtPath(field.Type, segments[1:], apply)
		if merged == nil || typ.TypeEquals(merged, field.Type) {
			return t
		}
		updated := *field
		updated.Type = merged
		return v.WithField(updated)
	}
	return t
}
