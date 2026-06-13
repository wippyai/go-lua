package exportmanifest

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func objectLiteralExprType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	table, ok := pathexpr.ObjectLiteralTable(expr)
	if !ok {
		return nil, false
	}
	return objectEntriesType(result, point, pathexpr.ObjectEntries(table), nil)
}

func pathExportRecordType(result *body.Result, point cfg.Point, root pathdom.Path) (typ.Type, bool) {
	if result == nil || root.IsEmpty() {
		return nil, false
	}
	fields := make(map[string]typ.Type)
	staticStrings := make(map[string]typ.Type)
	staticInts := make(map[int]typ.Type)
	addStateStaticMembers(result, point, root, fields, staticStrings, staticInts)
	addFunctionDefinitionMembers(result, root, fields, staticStrings, staticInts)
	return recordFromMemberMaps(fields, staticStrings, staticInts)
}

func addStateStaticMembers(
	result *body.Result,
	point cfg.Point,
	root pathdom.Path,
	fields map[string]typ.Type,
	staticStrings map[string]typ.Type,
	staticInts map[int]typ.Type,
) {
	if root.Symbol == 0 {
		return
	}
	st, ok := result.StateAt(point)
	if !ok {
		return
	}
	snapshot := st.PathStaticMembersSnapshot()
	if snapshot.Bottom || snapshot.Top {
		return
	}
	for pathKey, value := range snapshot.Members {
		sym, _, suffix, ok := pathaddr.ParseResolverPath(pathKey)
		if !ok || sym != root.Symbol {
			continue
		}
		segments, ok := segment.ParseFormattedSegments(suffix)
		if !ok {
			continue
		}
		member, ok := directMemberSegment(root.Segments, segments)
		if !ok {
			continue
		}
		t, ok := valueType(result.Registry(), value)
		if !ok {
			t = typ.Unknown
		}
		addObjectEntryType(fields, staticStrings, staticInts, member, t)
	}
}

func addFunctionDefinitionMembers(
	result *body.Result,
	root pathdom.Path,
	fields map[string]typ.Type,
	staticStrings map[string]typ.Type,
	staticInts map[int]typ.Type,
) {
	if result.Graph() == nil {
		return
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.FunctionDefinition(point)
		if !ok || fact.Name == nil || fact.Func == nil {
			continue
		}
		target, ok := result.ExpressionPath(fact.Name.Func)
		if !ok {
			continue
		}
		if fact.Name.Method != "" {
			if !target.Equal(root) {
				continue
			}
			if t, ok := functionExpressionType(result, fact.Func); ok {
				addObjectEntryType(fields, staticStrings, staticInts, segment.Segment{
					Kind: segment.SegmentField,
					Name: fact.Name.Method,
				}, t)
			}
			continue
		}
		member, ok := directMemberSegment(root.Segments, target.Segments)
		if !ok || target.Symbol != root.Symbol {
			continue
		}
		if t, ok := functionExpressionType(result, fact.Func); ok {
			addObjectEntryType(fields, staticStrings, staticInts, member, t)
		}
	}
}

func directMemberSegment(prefix, target []segment.Segment) (segment.Segment, bool) {
	if len(target) != len(prefix)+1 {
		return segment.Segment{}, false
	}
	for i := range prefix {
		if prefix[i] != target[i] {
			return segment.Segment{}, false
		}
	}
	return target[len(prefix)], true
}

func objectLiteralType(result *body.Result, point cfg.Point, entries []semantics.ObjectEntryFact) (typ.Type, bool) {
	if len(entries) == 0 {
		return nil, false
	}
	projected := make([]objectEntry, 0, len(entries))
	for _, entry := range entries {
		t, ok := sourceType(result, point, entry.Source)
		if !ok {
			t = typ.Unknown
		}
		projected = append(projected, objectEntry{suffix: entry.Suffix.Segments, t: t})
	}
	return objectEntriesType(result, point, nil, projected)
}

type objectEntry struct {
	suffix []segment.Segment
	t      typ.Type
}

func objectEntriesType(result *body.Result, point cfg.Point, astEntries []pathexpr.ObjectEntry, typedEntries []objectEntry) (typ.Type, bool) {
	if len(astEntries) == 0 && len(typedEntries) == 0 {
		return nil, false
	}
	fields := make(map[string]typ.Type)
	staticStrings := make(map[string]typ.Type)
	staticInts := make(map[int]typ.Type)
	for _, entry := range astEntries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		t, ok := exprType(result, point, entry.Value)
		if !ok {
			t = typ.Unknown
		}
		addObjectEntryType(fields, staticStrings, staticInts, entry.Suffix.Segments[0], t)
	}
	for _, entry := range typedEntries {
		if len(entry.suffix) != 1 {
			continue
		}
		addObjectEntryType(fields, staticStrings, staticInts, entry.suffix[0], entry.t)
	}
	if len(fields) == 0 && len(staticStrings) == 0 && len(staticInts) == 0 {
		return nil, false
	}
	return recordFromMemberMaps(fields, staticStrings, staticInts)
}

func recordFromMemberMaps(fields map[string]typ.Type, staticStrings map[string]typ.Type, staticInts map[int]typ.Type) (typ.Type, bool) {
	if len(fields) == 0 && len(staticStrings) == 0 && len(staticInts) == 0 {
		return nil, false
	}
	parts := typ.RecordParts{
		Fields:        sortedFields(fields),
		StaticMembers: sortedStaticMembers(staticStrings, staticInts),
	}
	return typetable.RebuildRecord(parts), true
}

func addObjectEntryType(
	fields map[string]typ.Type,
	staticStrings map[string]typ.Type,
	staticInts map[int]typ.Type,
	seg segment.Segment,
	t typ.Type,
) {
	switch seg.Kind {
	case segment.SegmentField:
		if seg.Name != "" {
			fields[seg.Name] = t
		}
	case segment.SegmentIndexString:
		staticStrings[seg.Name] = t
	case segment.SegmentIndexInt:
		staticInts[seg.Index] = t
	}
}

func sortedFields(fields map[string]typ.Type) []typ.Field {
	if len(fields) == 0 {
		return nil
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]typ.Field, 0, len(names))
	for _, name := range names {
		out = append(out, typ.Field{Name: name, Type: fields[name]})
	}
	return out
}

func sortedStaticMembers(strings map[string]typ.Type, ints map[int]typ.Type) []typ.StaticMember {
	if len(strings) == 0 && len(ints) == 0 {
		return nil
	}
	out := make([]typ.StaticMember, 0, len(strings)+len(ints))
	names := make([]string, 0, len(strings))
	for name := range strings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, typ.StaticMember{
			Kind: typ.StaticMemberStringIndex,
			Name: name,
			Type: strings[name],
		})
	}
	indexes := make([]int, 0, len(ints))
	for index := range ints {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		out = append(out, typ.StaticMember{
			Kind:  typ.StaticMemberIntIndex,
			Index: int64(index),
			Type:  ints[index],
		})
	}
	return out
}
