package exportmanifest

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
	addLocalObjectLiteralMembers(result, point, root, fields, staticStrings, staticInts)
	addStateStaticMembers(result, point, root, fields, staticStrings, staticInts)
	addOrdinaryAssignmentMembers(result, point, root, fields, staticStrings, staticInts)
	addFunctionDefinitionMembers(result, root, fields, staticStrings, staticInts)
	return recordFromMemberMaps(fields, staticStrings, staticInts)
}

// addOrdinaryAssignmentMembers publishes members written through direct
// `root.member = value` (or `root[key] = value`) assignment statements. A module
// that builds its export table by assigning fields onto a local after the table
// literal publishes each member from the assignment RHS at the assignment
// boundary. Reading the destination export path at the return boundary can see
// later degraded state and should only be a guarded last resort.
func addOrdinaryAssignmentMembers(
	result *body.Result,
	point cfg.Point,
	root pathdom.Path,
	fields map[string]typ.Type,
	staticStrings map[string]typ.Type,
	staticInts map[int]typ.Type,
) {
	if root.Symbol == 0 || result.Graph() == nil {
		return
	}
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasPath || fact.Path.Symbol != root.Symbol {
			continue
		}
		member, ok := directMemberSegment(root.Segments, fact.Path.Segments)
		if !ok {
			continue
		}
		t, ok := ordinaryAssignmentMemberType(result, candidate, root, fact)
		if !ok {
			continue
		}
		addObjectEntryType(fields, staticStrings, staticInts, member, t)
	}
}

func ordinaryAssignmentMemberType(result *body.Result, point cfg.Point, root pathdom.Path, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	if t, ok, resolved := ordinaryAssignmentRHSMemberType(result, point, root, fact); ok || resolved {
		return t, ok
	}
	return ordinaryAssignmentDestinationMemberType(result, point, fact.Path)
}

func ordinaryAssignmentRHSMemberType(result *body.Result, point cfg.Point, root pathdom.Path, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool, bool) {
	expr := ordinaryAssignmentRHSExpr(fact)
	if expr != nil {
		if t, ok := ordinaryAssignmentRHSPathType(result, point, root, fact, expr); ok {
			return t, true, true
		}
	}
	resolved := false
	if value, ok := ordinaryAssignmentRHSValue(result, point, fact); ok {
		resolved = true
		if t, ok := valueType(result.Registry(), value); ok {
			return t, true, true
		}
	}

	if expr == nil {
		return nil, false, resolved
	}
	if t, ok := exprType(result, point, expr); ok {
		return t, true, true
	}
	return nil, false, resolved
}

func ordinaryAssignmentRHSValue(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (product.Value, bool) {
	if fact.Source.Kind == sourceprovenance.SourceExpression {
		expr := ordinaryAssignmentRHSExpr(fact)
		if expr == nil {
			return product.Value{}, false
		}
		return result.ExpressionValueAtBoundary(point, expr)
	}
	valueSource, ok := valueSourceFromASTSource(fact.Source)
	if !ok {
		return product.Value{}, false
	}
	return result.SourceValueAtBoundary(point, valueSource)
}

func ordinaryAssignmentRHSExpr(fact semantics.OrdinaryAssignmentFact) ast.Expr {
	if fact.Source.Expr != nil {
		return fact.Source.Expr
	}
	return fact.Value
}

func ordinaryAssignmentRHSPathType(result *body.Result, point cfg.Point, root pathdom.Path, fact semantics.OrdinaryAssignmentFact, expr ast.Expr) (typ.Type, bool) {
	p, ok := result.ExpressionPath(expr)
	if !ok || p.IsEmpty() || p.Equal(root) {
		return nil, false
	}
	if fact.HasPath && p.Equal(fact.Path) {
		return nil, false
	}
	return pathExportRecordType(result, point, p)
}

func ordinaryAssignmentDestinationMemberType(result *body.Result, point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	value, ok := result.PathValueAtBoundary(point, p)
	if !ok {
		return nil, false
	}
	t, ok := valueType(result.Registry(), value)
	if !ok || typ.IsAny(t) || typ.IsUnknown(t) {
		return nil, false
	}
	return t, true
}

func addLocalObjectLiteralMembers(
	result *body.Result,
	point cfg.Point,
	root pathdom.Path,
	fields map[string]typ.Type,
	staticStrings map[string]typ.Type,
	staticInts map[int]typ.Type,
) {
	if root.Symbol == 0 || result.Graph() == nil || rootReassignedBefore(result, point, root) {
		return
	}
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != root.Symbol {
			continue
		}
		literal, ok := result.ObjectLiteral(fact.Expr)
		if !ok {
			return
		}
		addObjectLiteralEntries(result, point, root, literal.Entries, fields, staticStrings, staticInts)
		return
	}
}

func rootReassignedBefore(result *body.Result, point cfg.Point, root pathdom.Path) bool {
	for _, candidate := range result.Graph().RPO() {
		if candidate == point {
			return false
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if ok && fact.HasSymbol && fact.Symbol == root.Symbol {
			return true
		}
	}
	return false
}

func addObjectLiteralEntries(
	result *body.Result,
	point cfg.Point,
	root pathdom.Path,
	entries []semantics.ObjectEntryFact,
	fields map[string]typ.Type,
	staticStrings map[string]typ.Type,
	staticInts map[int]typ.Type,
) {
	for _, entry := range entries {
		member, ok := directMemberSegment(root.Segments, entry.Suffix.Segments)
		if !ok {
			continue
		}
		addObjectEntryType(fields, staticStrings, staticInts, member, objectEntryFactType(result, point, entry))
	}
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
		memberPath, ok := pathaddr.LocalPathFromKey(pathKey)
		if !ok || memberPath.Symbol != root.Symbol {
			continue
		}
		member, ok := directMemberSegment(root.Segments, memberPath.Segments)
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
		if fact.Name.Method != "" {
			receiver, ok := result.ExpressionPath(fact.Name.Receiver)
			if !ok || !receiver.Equal(root) {
				continue
			}
			if t, ok := functionDefinitionMemberType(result, fact.Func); ok {
				addObjectEntryType(fields, staticStrings, staticInts, segment.Segment{
					Kind: segment.SegmentField,
					Name: fact.Name.Method,
				}, t)
			}
			continue
		}
		target, ok := result.ExpressionPath(fact.Name.Func)
		if !ok {
			continue
		}
		member, ok := directMemberSegment(root.Segments, target.Segments)
		if !ok || target.Symbol != root.Symbol {
			continue
		}
		if t, ok := functionDefinitionMemberType(result, fact.Func); ok {
			addObjectEntryType(fields, staticStrings, staticInts, member, t)
		}
	}
}

func functionDefinitionMemberType(result *body.Result, fn *ast.FunctionExpr) (typ.Type, bool) {
	t, ok := functionExpressionType(result, fn)
	if !ok {
		return nil, false
	}
	return t, true
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
		projected = append(projected, objectEntry{suffix: entry.Suffix.Segments, t: objectEntryFactType(result, point, entry)})
	}
	return objectEntriesType(result, point, nil, projected)
}

func objectEntryFactType(result *body.Result, point cfg.Point, entry semantics.ObjectEntryFact) typ.Type {
	if t, ok := exprType(result, point, entry.Value); ok {
		return t
	}
	if t, ok := sourceType(result, point, entry.Source); ok {
		return t
	}
	return typ.Unknown
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
