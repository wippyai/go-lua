package exportmanifest

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	if result == nil || result.Graph() == nil || root.IsEmpty() {
		return nil, false
	}
	dom := dominance.ComputeImmediateDominatorInfo(result.Graph())
	members := newObjectMemberMaps()
	addLocalObjectLiteralMembers(result, point, root, members)
	addOrdinaryAssignmentMembers(result, point, dom, root, members)
	addFunctionDefinitionMembers(result, point, dom, root, members)
	addStateStaticMembers(result, point, root, members)
	recovered, recoveredOK := recordFromMemberMaps(members)
	contract, contractOK := pathRecordContractType(result, point, root)
	if recoveredOK && contractOK {
		return mergeRecordMembers(recovered, contract)
	}
	if recoveredOK {
		return recovered, true
	}
	if contractOK {
		return contract, true
	}
	return nil, false
}

func pathRecordContractType(result *body.Result, point cfg.Point, root pathdom.Path) (typ.Type, bool) {
	if t, ok := pathSymbolRecordContractType(result, root); ok {
		return t, true
	}
	value, ok := result.PathValueAtBoundary(point, root)
	if !ok {
		return nil, false
	}
	t, ok := exportValueType(result, value)
	if !ok || typ.IsAny(t) || typ.IsUnknown(t) {
		return nil, false
	}
	return tableShapeRecordContract(t)
}

func pathSymbolRecordContractType(result *body.Result, root pathdom.Path) (typ.Type, bool) {
	if result == nil || root.Symbol == 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(root.Symbol)
	if !ok || annotation == nil {
		return nil, false
	}
	t, ok := typeresolve.NewWithExternal(result, result.ModuleTypes()).Type(annotation)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return nil, false
	}
	return tableShapeRecordContract(t)
}

func tableShapeRecordContract(t typ.Type) (typ.Type, bool) {
	switch resolved := unwrap.Alias(t).(type) {
	case *typ.Record:
		return resolved, true
	case *typ.Map:
		return typetable.NewRecord().MapComponent(resolved.Key, resolved.Value).Build(), true
	default:
		return nil, false
	}
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
	dom *dominance.ImmediateDominators,
	root pathdom.Path,
	members *objectMemberMaps,
) {
	if root.Symbol == 0 || result.Graph() == nil {
		return
	}
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasPath || fact.Path.Symbol != root.Symbol {
			continue
		}
		optional := false
		if !dom.Dominates(candidate, point) {
			if !reachesPoint(result.Graph(), candidate, point) {
				continue
			}
			optional = true
		}
		member, ok := directMemberSegment(root.Segments, fact.Path.Segments)
		if !ok {
			continue
		}
		t, ok := ordinaryAssignmentMemberType(result, candidate, root, fact)
		if !ok {
			continue
		}
		members.add(member, t, optional)
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
		if sig, ok := result.ExpressionSignatureAt(point, expr); ok && sig.Type != nil {
			return sig.Type, true, true
		}
		if fn, ok := expr.(*ast.FunctionExpr); ok {
			if t, ok := functionExpressionType(result, fn); ok {
				return t, true, true
			}
		}
	}
	resolved := false
	if value, ok := ordinaryAssignmentRHSValue(result, point, fact); ok {
		resolved = true
		if t, ok := exportValueType(result, value); ok {
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
	valueSource, ok := sourcebridge.ValueSourceFromASTSource(fact.Source)
	if !ok {
		return product.Value{}, false
	}
	return result.SourceValueForExplanationAtBoundary(point, valueSource)
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
	t, ok := exportValueType(result, value)
	if !ok || typ.IsAny(t) || typ.IsUnknown(t) {
		return nil, false
	}
	return t, true
}

func addLocalObjectLiteralMembers(
	result *body.Result,
	point cfg.Point,
	root pathdom.Path,
	members *objectMemberMaps,
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
		addObjectLiteralEntries(result, point, root, literal.Entries, members)
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
	members *objectMemberMaps,
) {
	for _, entry := range entries {
		member, ok := directMemberSegment(root.Segments, entry.Suffix.Segments)
		if !ok {
			continue
		}
		members.add(member, objectEntryFactType(result, point, entry), false)
	}
}

func addStateStaticMembers(
	result *body.Result,
	point cfg.Point,
	root pathdom.Path,
	members *objectMemberMaps,
) {
	if root.Symbol == 0 {
		return
	}
	st, ok := result.StateAt(point)
	if !ok {
		return
	}
	snapshot := st.PathStaticMembersSnapshot(result.KeySpace())
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
		if members.has(member) {
			continue
		}
		t, ok := exportValueType(result, value)
		if !ok {
			t = typ.Unknown
		}
		members.add(member, t, false)
	}
}

func addFunctionDefinitionMembers(
	result *body.Result,
	point cfg.Point,
	dom *dominance.ImmediateDominators,
	root pathdom.Path,
	members *objectMemberMaps,
) {
	if result.Graph() == nil {
		return
	}
	for _, candidate := range result.Graph().RPO() {
		fact, ok := result.FunctionDefinition(candidate)
		if !ok || fact.Name == nil || fact.Func == nil {
			continue
		}
		optional := false
		if !dom.Dominates(candidate, point) {
			if !reachesPoint(result.Graph(), candidate, point) {
				continue
			}
			optional = true
		}
		if fact.Name.Method != "" {
			receiver, ok := result.ExpressionPath(fact.Name.Receiver)
			if !ok || !receiver.Equal(root) {
				continue
			}
			if t, ok := functionDefinitionMemberType(result, fact.Func); ok {
				members.add(segment.Segment{
					Kind: segment.SegmentField,
					Name: fact.Name.Method,
				}, t, optional)
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
			members.add(member, t, optional)
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
	members := newObjectMemberMaps()
	for _, entry := range astEntries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		t, ok := exprType(result, point, entry.Value)
		if !ok {
			t = typ.Unknown
		}
		members.add(entry.Suffix.Segments[0], t, false)
	}
	for _, entry := range typedEntries {
		if len(entry.suffix) != 1 {
			continue
		}
		members.add(entry.suffix[0], entry.t, false)
	}
	if members.empty() {
		return nil, false
	}
	return recordFromMemberMaps(members)
}

func recordFromMemberMaps(members *objectMemberMaps) (typ.Type, bool) {
	if members == nil || members.empty() {
		return nil, false
	}
	parts := typ.RecordParts{
		Fields:        sortedFields(members.fields),
		StaticMembers: sortedStaticMembers(members.staticStrings, members.staticInts),
	}
	return typetable.RebuildRecord(parts), true
}

type objectMember struct {
	t        typ.Type
	optional bool
}

type objectMemberMaps struct {
	fields        map[string]objectMember
	staticStrings map[string]objectMember
	staticInts    map[int]objectMember
}

func newObjectMemberMaps() *objectMemberMaps {
	return &objectMemberMaps{
		fields:        make(map[string]objectMember),
		staticStrings: make(map[string]objectMember),
		staticInts:    make(map[int]objectMember),
	}
}

func (m *objectMemberMaps) empty() bool {
	return m == nil || (len(m.fields) == 0 && len(m.staticStrings) == 0 && len(m.staticInts) == 0)
}

func (m *objectMemberMaps) has(seg segment.Segment) bool {
	if m == nil {
		return false
	}
	switch seg.Kind {
	case segment.SegmentField:
		_, ok := m.fields[seg.Name]
		return ok
	case segment.SegmentIndexString:
		_, ok := m.staticStrings[seg.Name]
		return ok
	case segment.SegmentIndexInt:
		_, ok := m.staticInts[seg.Index]
		return ok
	default:
		return false
	}
}

func (m *objectMemberMaps) add(seg segment.Segment, t typ.Type, optional bool) {
	if m == nil {
		return
	}
	switch seg.Kind {
	case segment.SegmentField:
		if seg.Name != "" {
			m.fields[seg.Name] = mergeObjectMember(m.fields[seg.Name], t, optional)
		}
	case segment.SegmentIndexString:
		m.staticStrings[seg.Name] = mergeObjectMember(m.staticStrings[seg.Name], t, optional)
	case segment.SegmentIndexInt:
		m.staticInts[seg.Index] = mergeObjectMember(m.staticInts[seg.Index], t, optional)
	}
}

func mergeObjectMember(existing objectMember, t typ.Type, optional bool) objectMember {
	if t == nil {
		t = typ.Unknown
	}
	if existing.t == nil {
		return objectMember{t: t, optional: optional}
	}
	if !typ.TypeEquals(existing.t, t) {
		if union, ok := normalize.UnionType([]typ.Type{existing.t, t}); ok {
			t = union
		}
	}
	return objectMember{t: t, optional: existing.optional && optional}
}

func reachesPoint(graph cfg.Graph, from, to cfg.Point) bool {
	if graph == nil {
		return false
	}
	if from == to {
		return true
	}
	seen := make(map[cfg.Point]struct{})
	stack := []cfg.Point{from}
	for len(stack) > 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[point]; ok {
			continue
		}
		seen[point] = struct{}{}
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if succ == to {
				return true
			}
			stack = append(stack, succ)
		}
	}
	return false
}

func sortedFields(fields map[string]objectMember) []typ.Field {
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
		member := fields[name]
		out = append(out, typ.Field{Name: name, Type: member.t, Optional: member.optional})
	}
	return out
}

func sortedStaticMembers(strings map[string]objectMember, ints map[int]objectMember) []typ.StaticMember {
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
		member := strings[name]
		out = append(out, typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     name,
			Type:     member.t,
			Optional: member.optional,
		})
	}
	indexes := make([]int, 0, len(ints))
	for index := range ints {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		member := ints[index]
		out = append(out, typ.StaticMember{
			Kind:     typ.StaticMemberIntIndex,
			Index:    int64(index),
			Type:     member.t,
			Optional: member.optional,
		})
	}
	return out
}
