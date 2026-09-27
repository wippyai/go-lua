package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
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

// MergeFieldsIntoType merges a set of written field types into a base type.
// A written field already present on a record base joins with its existing type.
func MergeFieldsIntoType(baseType typ.Type, fields map[string]typ.Type) typ.Type {
	if len(fields) == 0 {
		return baseType
	}
	return typ.WriteInto(baseType, func(t typ.Type) typ.Type {
		return mergeFields(t, fields)
	})
}

func mergeFields(baseType typ.Type, fields map[string]typ.Type) typ.Type {
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
		builder := typ.NewRecord().SetDeclared(v.Declared).SetComplete(v.Complete)
		if v.Open {
			builder.SetOpen(true)
		}
		existing := make(map[string]bool)
		for _, f := range v.Fields {
			fieldType := f.Type
			if written, ok := fields[f.Name]; ok {
				// A field holds every value stored into it, so its domain is
				// the join of the initializer type and the written types.
				fieldType = typ.JoinPreferNonSoft(fieldType, written)
			}
			builder.Field(f.Name, fieldType)
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
	return typ.WriteInto(baseType, func(t typ.Type) typ.Type {
		return mergeMapComponent(t, keyType, valType)
	})
}

func mergeMapComponent(baseType, keyType, valType typ.Type) typ.Type {
	if baseType == nil {
		return typ.NewMap(keyType, valType)
	}

	switch v := baseType.(type) {
	case *typ.Map:
		newKey := typ.JoinPreferNonSoft(v.Key, keyType)
		newVal := typ.JoinPreferNonSoft(v.Value, valType)
		return typ.NewMap(newKey, newVal)
	case *typ.Record:
		builder := typ.NewRecord().SetDeclared(v.Declared).SetComplete(v.Complete)
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
			t = EditAtPath(t, api.FieldWriteKey{Path: path}.Segments(), func(table typ.Type) typ.Type {
				return mergeWrittenFields(table, fields)
			}, MergePathEdit)
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

type PathEditMode uint8

const (
	MergePathEdit PathEditMode = iota
	OverwritePathEdit
)

// EditAtPath applies one leaf edit while rebuilding the types along a static
// path. The mode selects the path's field-merge or structured-overwrite rules.
func EditAtPath(
	t typ.Type,
	segments []constraint.Segment,
	apply func(typ.Type) typ.Type,
	mode PathEditMode,
) typ.Type {
	if len(segments) == 0 {
		return apply(t)
	}
	child, rest, rebuild := pathEditStep(t, segments, mode)
	if rebuild == nil {
		return t
	}
	return rebuild(EditAtPath(child, rest, apply, mode))
}

func pathEditStep(
	t typ.Type,
	segments []constraint.Segment,
	mode PathEditMode,
) (typ.Type, []constraint.Segment, func(typ.Type) typ.Type) {
	seg := segments[0]
	switch mode {
	case MergePathEdit:
		switch v := t.(type) {
		case *typ.Optional:
			return v.Inner, segments, func(inner typ.Type) typ.Type {
				if inner == v.Inner {
					return t
				}
				return typ.NewOptional(inner)
			}
		case *typ.Record:
			if seg.Kind == constraint.SegmentIndexInt {
				return nil, nil, nil
			}
			field := v.GetField(seg.Name)
			if field == nil {
				return nil, nil, nil
			}
			return field.Type, segments[1:], func(merged typ.Type) typ.Type {
				if merged == nil || typ.TypeEquals(merged, field.Type) {
					return t
				}
				updated := *field
				updated.Type = merged
				return v.WithField(updated)
			}
		}
	case OverwritePathEdit:
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString, constraint.SegmentIndexInt:
			return structuredChildType(t, seg), segments[1:], func(child typ.Type) typ.Type {
				return rebuildStructuredChild(t, seg, child)
			}
		}
	}
	return nil, nil, nil
}

func structuredChildType(baseType typ.Type, seg constraint.Segment) typ.Type {
	for alias, ok := baseType.(*typ.Alias); ok; alias, ok = baseType.(*typ.Alias) {
		baseType = alias.Target
	}

	switch t := baseType.(type) {
	case *typ.Record:
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if field := t.GetField(seg.Name); field != nil {
				return field.Type
			}
			if t.HasMapComponent() && (typ.IsAny(t.MapKey) || t.MapKey.Kind() == kind.String) {
				return t.MapValue
			}
		case constraint.SegmentIndexInt:
			if t.HasMapComponent() && (typ.IsAny(t.MapKey) || t.MapKey.Kind() == kind.Integer || t.MapKey.Kind() == kind.Number) {
				return t.MapValue
			}
		}
	case *typ.Map:
		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if typ.IsAny(t.Key) || t.Key.Kind() == kind.String {
				return t.Value
			}
		case constraint.SegmentIndexInt:
			if typ.IsAny(t.Key) || t.Key.Kind() == kind.Integer || t.Key.Kind() == kind.Number {
				return t.Value
			}
		}
	case *typ.Array:
		if seg.Kind == constraint.SegmentIndexInt {
			return t.Element
		}
	}
	return nil
}

func rebuildStructuredChild(baseType typ.Type, seg constraint.Segment, childType typ.Type) typ.Type {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return overwriteStructuredField(baseType, seg.Name, childType)
	case constraint.SegmentIndexInt:
		return overwriteStructuredIndex(baseType, childType)
	default:
		return baseType
	}
}

func overwriteStructuredField(baseType typ.Type, field string, fieldType typ.Type) typ.Type {
	if field == "" || fieldType == nil {
		return baseType
	}
	switch t := baseType.(type) {
	case *typ.Alias:
		updated := overwriteStructuredField(t.Target, field, fieldType)
		if updated == nil || typ.TypeEquals(updated, t.Target) {
			return baseType
		}
		return typ.NewAlias(t.Name, updated)
	case *typ.Map:
		return typ.NewRecord().SetOpen(true).MapComponent(t.Key, t.Value).Field(field, fieldType).Build()
	default:
		return typ.ExtendRecordWithField(baseType, field, fieldType)
	}
}

func overwriteStructuredIndex(baseType typ.Type, elemType typ.Type) typ.Type {
	if elemType == nil {
		return baseType
	}
	return typ.WriteInto(baseType, func(t typ.Type) typ.Type {
		return overwriteStructuredIndexNonDynamic(t, elemType)
	})
}

func overwriteStructuredIndexNonDynamic(baseType typ.Type, elemType typ.Type) typ.Type {
	switch t := baseType.(type) {
	case *typ.Alias:
		updated := overwriteStructuredIndex(t.Target, elemType)
		if updated == nil || typ.TypeEquals(updated, t.Target) {
			return baseType
		}
		return typ.NewAlias(t.Name, updated)
	case *typ.Array:
		return typ.NewArray(elemType)
	case *typ.Map:
		return typ.NewMap(t.Key, elemType)
	case *typ.Record:
		builder := typ.NewRecord().SetComplete(t.Complete)
		if t.Open {
			builder.SetOpen(true)
		}
		for _, f := range t.Fields {
			builder.AddField(f)
		}
		if t.Metatable != nil {
			builder.Metatable(t.Metatable)
		}
		builder.MapComponent(typ.Integer, elemType)
		return builder.Build()
	default:
		return typ.NewMap(typ.Integer, elemType)
	}
}
