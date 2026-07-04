package semantics

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func stringLit(value string) *ast.StringExpr {
	return &ast.StringExpr{Value: value}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func stringIndex(obj ast.Expr, key string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(key),
		KeySyntax: ast.AttrKeyIndex,
	}
}

func intIndex(obj ast.Expr, index string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       number(index),
		KeySyntax: ast.AttrKeyIndex,
	}
}

func typeCall(arg ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{arg}}
}

func call(name string) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident(name)}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func assign(lhs []ast.Expr, rhs ...ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs}
}

func function(names []string, stmts ...ast.Stmt) *ast.FunctionExpr {
	return &ast.FunctionExpr{
		ParList: &ast.ParList{Names: names},
		Stmts:   stmts,
	}
}

func mustLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	id, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok {
		t.Fatalf("missing local symbol %d for %v", index, stmt.Names)
	}
	return id
}

func mustGenericForAt(t *testing.T, bindings *bind.Result, stmt *ast.GenericForStmt, index int) symbol.ID {
	t.Helper()
	ids := bindings.GenericForSymbols(stmt)
	if index < 0 || index >= len(ids) {
		t.Fatalf("missing generic for symbol at %d", index)
	}
	return ids[index]
}

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok {
		t.Fatalf("missing symbol for %q", ident.Value)
	}
	return id
}

func requireStmtPoints(t *testing.T, built *cfgbuild.Result, stmt ast.Stmt, want int) []cfg.Point {
	t.Helper()
	points := built.StmtPoints.PointsFor(stmt)
	if len(points) != want {
		t.Fatalf("points for %T = %v, want %d", stmt, points, want)
	}
	return points
}

func assertEntry(t *testing.T, got ObjectEntryFact, wantIndex int, wantSuffix path.Path, wantValue ast.Expr) {
	t.Helper()
	if got.Index != wantIndex || got.Value != wantValue || got.Source.Expr != wantValue {
		t.Fatalf("entry = %#v, want index %d value %p", got, wantIndex, wantValue)
	}
	if got.Source.Kind != sourceprovenance.SourceExpression || got.Source.ExprIndex != sourceprovenance.NoSourceIndex || got.Source.TargetIndex != sourceprovenance.NoSourceIndex {
		t.Fatalf("entry source = %#v, want expression source without value-list indexes", got.Source)
	}
	if !got.Suffix.Equal(wantSuffix) {
		t.Fatalf("entry suffix = %#v, want %#v", got.Suffix, wantSuffix)
	}
}

func stringLitSuffix(value string, syntax ast.AttrKeySyntax) path.Path {
	switch syntax {
	case ast.AttrKeyDot:
		return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: value}}}
	case ast.AttrKeyIndex:
		return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: value}}}
	default:
		return path.Path{}
	}
}

func intSuffix(index int) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: index}}}
}

func fieldChainSuffix(names ...string) path.Path {
	segments := make([]segment.Segment, len(names))
	for i, name := range names {
		segments[i] = segment.Segment{Kind: segment.SegmentField, Name: name}
	}
	return path.Path{Segments: segments}
}

func TestExtractChunkAssignmentsUseStmtPointsAndPreserveIdentity(t *testing.T) {
	nameType := &ast.PrimitiveTypeExpr{Name: "string"}
	local := &ast.LocalAssignStmt{
		Names: []string{"a", "b"},
		Types: []ast.TypeExpr{nameType, nil},
		Exprs: []ast.Expr{number("1"), number("2")},
	}
	aWrite := ident("a")
	bWrite := ident("b")
	write := assign([]ast.Expr{aWrite, bWrite}, number("3"), number("4"))
	stmts := []ast.Stmt{local, write}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	localPoints := requireStmtPoints(t, built, local, 2)
	first, ok := result.LocalAssignment(localPoints[0])
	if !ok {
		t.Fatalf("missing first local assignment")
	}
	if first.Stmt != local || first.Index != 0 || first.Name != "a" || first.Type != nameType || first.Expr != local.Exprs[0] {
		t.Fatalf("first local fact = %#v", first)
	}
	if first.Symbol != mustLocalAt(t, bindings, local, 0) || !first.HasSymbol {
		t.Fatalf("first local symbol = %d/%v", first.Symbol, first.HasSymbol)
	}
	second, ok := result.LocalAssignment(localPoints[1])
	if !ok || second.Stmt != local || second.Index != 1 || second.Name != "b" || second.Expr != local.Exprs[1] {
		t.Fatalf("second local fact = %#v, ok=%v", second, ok)
	}
	second.Exprs[0] = ident("mutated")
	again, _ := result.LocalAssignment(localPoints[1])
	if again.Exprs[0] != local.Exprs[0] {
		t.Fatalf("LocalAssignment exposed mutable expr slice")
	}
	localView, ok := result.LocalAssignmentView(localPoints[1])
	if !ok {
		t.Fatalf("missing local assignment view")
	}
	borrowedLocal, ok := localView.Borrowed()
	if !ok || borrowedLocal.Expr != local.Exprs[1] || borrowedLocal.Exprs[0] != local.Exprs[0] {
		t.Fatalf("borrowed local assignment = %#v, ok=%v", borrowedLocal, ok)
	}
	localAllocs := testing.AllocsPerRun(1000, func() {
		view, ok := result.LocalAssignmentView(localPoints[1])
		if !ok {
			t.Fatalf("missing local assignment view")
		}
		borrowed, ok := view.Borrowed()
		if !ok || borrowed.Expr == nil {
			t.Fatalf("borrowed local assignment = %#v, ok=%v", borrowed, ok)
		}
	})
	if localAllocs != 0 {
		t.Fatalf("LocalAssignmentView allocations/run = %.1f, want zero", localAllocs)
	}

	writePoints := requireStmtPoints(t, built, write, 2)
	firstWrite, ok := result.OrdinaryAssignment(writePoints[0])
	if !ok {
		t.Fatalf("missing first ordinary assignment")
	}
	if firstWrite.Stmt != write || firstWrite.Index != 0 || firstWrite.Target != aWrite || firstWrite.Value != write.Rhs[0] {
		t.Fatalf("first ordinary fact = %#v", firstWrite)
	}
	if firstWrite.Symbol != mustIdentSymbol(t, bindings, aWrite) || !firstWrite.HasSymbol {
		t.Fatalf("first ordinary symbol = %d/%v", firstWrite.Symbol, firstWrite.HasSymbol)
	}
	if !firstWrite.HasPath || !firstWrite.Path.Equal(path.NewPath(firstWrite.Symbol, "a")) {
		t.Fatalf("first ordinary path = %v/%v, want root a", firstWrite.Path, firstWrite.HasPath)
	}
	secondWrite, ok := result.OrdinaryAssignment(writePoints[1])
	if !ok || secondWrite.Target != bWrite {
		t.Fatalf("second ordinary assignment = %#v, ok=%v", secondWrite, ok)
	}
	ordinaryView, ok := result.OrdinaryAssignmentView(writePoints[1])
	if !ok {
		t.Fatalf("missing ordinary assignment view")
	}
	borrowedOrdinary, ok := ordinaryView.Borrowed()
	if !ok || borrowedOrdinary.Target != bWrite || borrowedOrdinary.Rhs[0] != write.Rhs[0] {
		t.Fatalf("borrowed ordinary assignment = %#v, ok=%v", borrowedOrdinary, ok)
	}
	ordinaryAllocs := testing.AllocsPerRun(1000, func() {
		view, ok := result.OrdinaryAssignmentView(writePoints[1])
		if !ok {
			t.Fatalf("missing ordinary assignment view")
		}
		borrowed, ok := view.Borrowed()
		if !ok || borrowed.Value == nil {
			t.Fatalf("borrowed ordinary assignment = %#v, ok=%v", borrowed, ok)
		}
	})
	if ordinaryAllocs != 0 {
		t.Fatalf("OrdinaryAssignmentView allocations/run = %.1f, want zero", ordinaryAllocs)
	}
}

func TestExtractChunkOrdinaryAssignmentsResolveStaticMemberPaths(t *testing.T) {
	local := localAssign([]string{"t", "k"}, number("0"), stringLit("key"))
	dotWrite := assign([]ast.Expr{dot(ident("t"), "x")}, number("1"))
	indexWrite := assign([]ast.Expr{stringIndex(ident("t"), "x")}, number("2"))
	dynamicWrite := assign([]ast.Expr{&ast.AttrGetExpr{
		Object:    ident("t"),
		Key:       ident("k"),
		KeySyntax: ast.AttrKeyIndex,
	}}, number("3"))
	nestedDynamicWrite := assign([]ast.Expr{dot(&ast.AttrGetExpr{
		Object:    ident("t"),
		Key:       ident("k"),
		KeySyntax: ast.AttrKeyIndex,
	}, "value")}, number("4"))
	stmts := []ast.Stmt{local, dotWrite, indexWrite, dynamicWrite, nestedDynamicWrite}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	tSym := mustLocalAt(t, bindings, local, 0)
	dotFact, ok := result.OrdinaryAssignment(requireStmtPoints(t, built, dotWrite, 1)[0])
	if !ok {
		t.Fatalf("missing dot assignment")
	}
	if !dotFact.HasPath || !dotFact.Path.Equal(path.NewPath(tSym, "t").Field("x")) {
		t.Fatalf("dot path = %v/%v, want t.x", dotFact.Path, dotFact.HasPath)
	}
	indexFact, ok := result.OrdinaryAssignment(requireStmtPoints(t, built, indexWrite, 1)[0])
	if !ok {
		t.Fatalf("missing static index assignment")
	}
	if !indexFact.HasPath || !indexFact.Path.Equal(path.NewPath(tSym, "t").IndexStr("x")) {
		t.Fatalf("static index path = %v/%v, want t[\"x\"]", indexFact.Path, indexFact.HasPath)
	}
	indexFact.Path.Segments[0].Name = "mutated"
	again, _ := result.OrdinaryAssignment(requireStmtPoints(t, built, indexWrite, 1)[0])
	if !again.Path.Equal(path.NewPath(tSym, "t").IndexStr("x")) {
		t.Fatalf("ordinary assignment exposed mutable path: %v", again.Path)
	}
	dynamicFact, ok := result.OrdinaryAssignment(requireStmtPoints(t, built, dynamicWrite, 1)[0])
	if !ok {
		t.Fatalf("missing dynamic index assignment")
	}
	if dynamicFact.HasPath {
		t.Fatalf("dynamic index path resolved unexpectedly: %v", dynamicFact.Path)
	}
	if !dynamicFact.HasContainerPath || !dynamicFact.ContainerPath.Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("dynamic index container path = %v/%v, want t", dynamicFact.ContainerPath, dynamicFact.HasContainerPath)
	}
	dynamicFact.ContainerPath.Symbol = 999
	dynamicAgain, _ := result.OrdinaryAssignment(requireStmtPoints(t, built, dynamicWrite, 1)[0])
	if !dynamicAgain.ContainerPath.Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("ordinary assignment exposed mutable container path: %v", dynamicAgain.ContainerPath)
	}
	nestedDynamicFact, ok := result.OrdinaryAssignment(requireStmtPoints(t, built, nestedDynamicWrite, 1)[0])
	if !ok {
		t.Fatalf("missing nested dynamic index assignment")
	}
	if nestedDynamicFact.HasPath {
		t.Fatalf("nested dynamic index path resolved unexpectedly: %v", nestedDynamicFact.Path)
	}
	if !nestedDynamicFact.HasContainerPath || !nestedDynamicFact.ContainerPath.Equal(path.NewPath(tSym, "t")) {
		t.Fatalf("nested dynamic index container path = %v/%v, want t", nestedDynamicFact.ContainerPath, nestedDynamicFact.HasContainerPath)
	}
}

func TestExtractChunkObjectLiteralStaticEntriesAndDynamicSkip(t *testing.T) {
	namedValue := number("1")
	stringValue := number("2")
	intValue := number("3")
	firstArrayValue := number("4")
	dynamicValue := number("5")
	secondArrayValue := number("6")
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("name"), KeySyntax: ast.AttrKeyDot, Value: namedValue},
		{Key: stringLit("key"), KeySyntax: ast.AttrKeyIndex, Value: stringValue},
		{Key: number("7"), KeySyntax: ast.AttrKeyIndex, Value: intValue},
		{Value: firstArrayValue},
		{Key: ident("dynamic"), KeySyntax: ast.AttrKeyIndex, Value: dynamicValue},
		{Value: secondArrayValue},
	}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	fact, ok := result.ObjectLiteral(table)
	if !ok {
		t.Fatalf("missing object literal sidecar")
	}
	if fact.Expr != table || fact.Table != table {
		t.Fatalf("object literal identity = %#v", fact)
	}
	entries := fact.Entries
	if len(entries) != 5 {
		t.Fatalf("entries = %#v, want 5 static entries", entries)
	}
	assertEntry(t, entries[0], 0, stringLitSuffix("name", ast.AttrKeyDot), namedValue)
	assertEntry(t, entries[1], 1, stringLitSuffix("key", ast.AttrKeyIndex), stringValue)
	assertEntry(t, entries[2], 2, intSuffix(7), intValue)
	assertEntry(t, entries[3], 3, intSuffix(1), firstArrayValue)
	assertEntry(t, entries[4], 5, intSuffix(2), secondArrayValue)

	entries[0].Suffix.Segments[0].Name = "mutated"
	again, _ := result.ObjectLiteral(table)
	if !again.Entries[0].Suffix.Equal(stringLitSuffix("name", ast.AttrKeyDot)) {
		t.Fatalf("ObjectLiteral exposed mutable suffix: %#v", again.Entries[0].Suffix)
	}

	if _, ok := result.ObjectLiteral(dynamicValue); ok {
		t.Fatalf("dynamic field value unexpectedly has object sidecar")
	}
}

func TestExtractChunkEmptyObjectLiteralPublishesSidecar(t *testing.T) {
	table := &ast.TableExpr{}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	fact, ok := result.ObjectLiteral(table)
	if !ok {
		t.Fatalf("missing empty object literal sidecar")
	}
	if fact.Expr != table || fact.Table != table {
		t.Fatalf("object literal identity = %#v", fact)
	}
	if len(fact.Entries) != 0 {
		t.Fatalf("empty literal entries = %#v, want none", fact.Entries)
	}
}

func TestExtractChunkObjectLiteralThroughLogicalOperand(t *testing.T) {
	table := &ast.TableExpr{}
	local := localAssign([]string{"t"}, &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      &ast.NilExpr{},
		Rhs:      table,
	})
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	fact, ok := result.ObjectLiteral(table)
	if !ok {
		t.Fatalf("missing object literal sidecar for logical operand")
	}
	if fact.Expr != table || fact.Table != table {
		t.Fatalf("object literal identity = %#v", fact)
	}
}

func TestExtractChunkObjectLiteralThroughAssertionWrappers(t *testing.T) {
	asTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("a"), KeySyntax: ast.AttrKeyDot, Value: number("1")},
	}}
	asCast := &ast.CastExpr{
		Expr:   asTable,
		Type:   &ast.PrimitiveTypeExpr{Name: "number"},
		Syntax: ast.CastSyntaxAs,
	}
	colonTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("b"), KeySyntax: ast.AttrKeyDot, Value: number("2")},
	}}
	colonCast := &ast.CastExpr{
		Expr:   colonTable,
		Type:   &ast.PrimitiveTypeExpr{Name: "number"},
		Syntax: ast.CastSyntaxColonColon,
	}
	anyTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("raw"), KeySyntax: ast.AttrKeyDot, Value: number("5")},
	}}
	anyCast := &ast.CastExpr{
		Expr:   anyTable,
		Type:   &ast.PrimitiveTypeExpr{Name: "any"},
		Syntax: ast.CastSyntaxColonColon,
	}
	nonNilTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("c"), KeySyntax: ast.AttrKeyDot, Value: number("3")},
	}}
	nonNil := &ast.NonNilAssertExpr{Expr: nonNilTable}
	nestedTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("leaf"), KeySyntax: ast.AttrKeyDot, Value: number("4")},
	}}
	nestedCast := &ast.CastExpr{
		Expr:   nestedTable,
		Type:   &ast.PrimitiveTypeExpr{Name: "number"},
		Syntax: ast.CastSyntaxAs,
	}
	rootTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("nested"), KeySyntax: ast.AttrKeyDot, Value: nestedCast},
	}}
	local := localAssign([]string{"a", "b", "c", "d", "e"}, asCast, colonCast, nonNil, rootTable, anyCast)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	for _, tc := range []struct {
		name  string
		expr  ast.Expr
		table *ast.TableExpr
		want  path.Path
	}{
		{name: "as", expr: asCast, table: asTable, want: fieldChainSuffix("a")},
		{name: "colon", expr: colonCast, table: colonTable, want: fieldChainSuffix("b")},
		{name: "non-nil", expr: nonNil, table: nonNilTable, want: fieldChainSuffix("c")},
	} {
		fact, ok := result.ObjectLiteral(tc.expr)
		if !ok {
			t.Fatalf("%s wrapper missing object literal sidecar", tc.name)
		}
		if fact.Expr != tc.expr || fact.Table != tc.table {
			t.Fatalf("%s object literal identity = %#v", tc.name, fact)
		}
		if len(fact.Entries) != 1 || !fact.Entries[0].Suffix.Equal(tc.want) {
			t.Fatalf("%s entries = %#v, want suffix %v", tc.name, fact.Entries, tc.want)
		}
	}
	if _, ok := result.ObjectLiteral(asTable); ok {
		t.Fatalf("wrapped table also keyed by inner table")
	}
	if _, ok := result.ObjectLiteral(anyCast); ok {
		t.Fatalf("any cast produced object literal proof sidecar")
	}
	if _, ok := result.ObjectLiteral(anyTable); ok {
		t.Fatalf("any-cast wrapped table also keyed by inner table")
	}

	rootFact, ok := result.ObjectLiteral(rootTable)
	if !ok {
		t.Fatalf("missing root object literal sidecar")
	}
	if len(rootFact.Entries) != 2 {
		t.Fatalf("root entries = %#v, want wrapped nested entry and leaf", rootFact.Entries)
	}
	assertEntry(t, rootFact.Entries[0], 0, fieldChainSuffix("nested"), nestedCast)
	if !rootFact.Entries[1].Suffix.Equal(fieldChainSuffix("nested", "leaf")) {
		t.Fatalf("nested wrapped entry suffix = %v", rootFact.Entries[1].Suffix)
	}

	nestedFact, ok := result.ObjectLiteral(nestedCast)
	if !ok {
		t.Fatalf("missing nested wrapped object literal sidecar")
	}
	if nestedFact.Expr != nestedCast || nestedFact.Table != nestedTable {
		t.Fatalf("nested wrapped identity = %#v", nestedFact)
	}
}

func TestExtractChunkObjectLiteralNestedStaticEntriesFlatten(t *testing.T) {
	nestedLeaf := number("1")
	dynamicValue := number("2")
	deepLeaf := number("3")
	deeper := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("d"), KeySyntax: ast.AttrKeyDot, Value: deepLeaf},
	}}
	nested := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("b"), KeySyntax: ast.AttrKeyDot, Value: nestedLeaf},
		{Key: ident("dynamic"), KeySyntax: ast.AttrKeyIndex, Value: dynamicValue},
		{Key: stringLit("c"), KeySyntax: ast.AttrKeyDot, Value: deeper},
	}}
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("a"), KeySyntax: ast.AttrKeyDot, Value: nested},
	}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	fact, ok := result.ObjectLiteral(table)
	if !ok {
		t.Fatalf("missing object literal sidecar")
	}
	entries := fact.Entries
	if len(entries) != 4 {
		t.Fatalf("entries = %#v, want root and nested static entries", entries)
	}
	assertEntry(t, entries[0], 0, fieldChainSuffix("a"), nested)
	assertEntry(t, entries[1], 0, fieldChainSuffix("a", "b"), nestedLeaf)
	assertEntry(t, entries[2], 2, fieldChainSuffix("a", "c"), deeper)
	assertEntry(t, entries[3], 0, fieldChainSuffix("a", "c", "d"), deepLeaf)
	for _, entry := range entries {
		if entry.Value == dynamicValue {
			t.Fatalf("dynamic nested field was included: %#v", entry)
		}
	}
}

func TestExtractChunkObjectLiteralCallArgumentEntriesAndSources(t *testing.T) {
	userID := stringLit("u1")
	profile := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       stringLit("user_id"),
		KeySyntax: ast.AttrKeyDot,
		Value:     userID,
	}}}
	event := stringLit("created")
	makeCall := &ast.FuncCallExpr{Func: ident("make_event")}
	arg := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("profile"), KeySyntax: ast.AttrKeyDot, Value: profile},
		{Key: stringLit("event"), KeySyntax: ast.AttrKeyDot, Value: event},
		{Key: stringLit("generated"), KeySyntax: ast.AttrKeyDot, Value: makeCall},
	}}
	okCall := &ast.FuncCallExpr{Func: dot(ident("result"), "ok"), Args: []ast.Expr{arg}}
	local := localAssign([]string{"wrapped"}, okCall)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make_event", "result"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, local, 3)
	okFact, ok := result.Call(points[1])
	if !ok || okFact.Call != okCall {
		t.Fatalf("ok call fact = %#v, ok=%v", okFact, ok)
	}
	if len(okFact.ArgumentSources) != 1 || okFact.ArgumentSources[0].Kind != sourceprovenance.SourceExpression || okFact.ArgumentSources[0].Expr != arg {
		t.Fatalf("ok call argument sources = %#v, want table argument expression", okFact.ArgumentSources)
	}
	if len(okFact.ArgumentSpans) != 1 || len(okFact.ArgumentLabels) != 1 {
		t.Fatalf("ok call argument metadata spans=%#v labels=%#v, want one per argument", okFact.ArgumentSpans, okFact.ArgumentLabels)
	}

	fact, ok := result.ObjectLiteral(arg)
	if !ok {
		t.Fatalf("missing call argument object literal sidecar")
	}
	if fact.Expr != arg || fact.Table != arg {
		t.Fatalf("call argument object literal identity = %#v", fact)
	}
	if len(fact.Entries) != 4 {
		t.Fatalf("call argument entries = %#v, want root, nested, and call-valued entries", fact.Entries)
	}
	assertEntry(t, fact.Entries[0], 0, fieldChainSuffix("profile"), profile)
	assertEntry(t, fact.Entries[1], 0, fieldChainSuffix("profile", "user_id"), userID)
	assertEntry(t, fact.Entries[2], 1, fieldChainSuffix("event"), event)
	generated := fact.Entries[3]
	if generated.Index != 2 || generated.Value != makeCall || !generated.Suffix.Equal(fieldChainSuffix("generated")) {
		t.Fatalf("generated entry = %#v", generated)
	}
	if generated.Source.Kind != sourceprovenance.SourceCall || generated.Source.Expr != makeCall || generated.Source.CallPoint != points[0] || !generated.Source.HasCallPoint {
		t.Fatalf("generated source = %#v, want make_event call point %d", generated.Source, points[0])
	}
	if generated.ValueLabel != "" {
		t.Fatalf("generated value label = %q, want empty for call expression", generated.ValueLabel)
	}

	nestedFact, ok := result.ObjectLiteral(profile)
	if !ok || len(nestedFact.Entries) != 1 {
		t.Fatalf("nested object literal = %#v, ok=%v", nestedFact, ok)
	}
	assertEntry(t, nestedFact.Entries[0], 0, fieldChainSuffix("user_id"), userID)
}

func TestCallArgumentMetadataLabelsUnpackExpansion(t *testing.T) {
	values := ident("values")
	unpackCall := &ast.FuncCallExpr{Func: ident("unpack"), Args: []ast.Expr{values}}
	acceptCall := &ast.FuncCallExpr{Func: ident("accept"), Args: []ast.Expr{unpackCall}}
	stmt := &ast.FuncCallStmt{Expr: acceptCall}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"accept", "unpack"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, stmt, 2)
	fact, ok := result.Call(points[1])
	if !ok || fact.Call != acceptCall {
		t.Fatalf("accept call fact = %#v, ok=%v", fact, ok)
	}
	if len(fact.ArgumentLabels) != 1 || fact.ArgumentLabels[0] != "unpack(...)" {
		t.Fatalf("argument labels = %#v, want unpack(...)", fact.ArgumentLabels)
	}
}

func TestExtractFunctionObjectLiteralReturnCallArgument(t *testing.T) {
	event := stringLit("created")
	arg := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       stringLit("event"),
		KeySyntax: ast.AttrKeyDot,
		Value:     event,
	}}}
	okCall := &ast.FuncCallExpr{Func: dot(ident("result"), "ok"), Args: []ast.Expr{arg}}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{okCall}}
	fn := function(nil, ret)
	bindings := bind.BindFunction(fn, bind.Options{Globals: []string{"result"}})
	built := cfgbuild.BuildFunction(fn, bindings)

	result, err := ExtractFunction(fn, bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}

	points := requireStmtPoints(t, built, ret, 2)
	okFact, ok := result.Call(points[0])
	if !ok || okFact.Call != okCall || okFact.Context != CallContextReturnSource {
		t.Fatalf("return call fact = %#v, ok=%v", okFact, ok)
	}
	if len(okFact.ArgumentSources) != 1 || okFact.ArgumentSources[0].Kind != sourceprovenance.SourceExpression || okFact.ArgumentSources[0].Expr != arg {
		t.Fatalf("return call argument sources = %#v, want table argument expression", okFact.ArgumentSources)
	}
	fact, ok := result.ObjectLiteral(arg)
	if !ok {
		t.Fatalf("missing returned call argument object literal sidecar")
	}
	if len(fact.Entries) != 1 {
		t.Fatalf("returned call argument entries = %#v, want event entry", fact.Entries)
	}
	assertEntry(t, fact.Entries[0], 0, fieldChainSuffix("event"), event)
}

func TestExtractChunkObjectLiteralSkipsFinalExpandingArrayField(t *testing.T) {
	nonFinalVararg := &ast.Comma3Expr{}
	keyedVararg := &ast.Comma3Expr{}
	finalVararg := &ast.Comma3Expr{}
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Value: nonFinalVararg},
		{Key: stringLit("key"), KeySyntax: ast.AttrKeyDot, Value: keyedVararg},
		{Value: finalVararg},
	}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	fact, ok := result.ObjectLiteral(table)
	if !ok {
		t.Fatalf("missing object literal sidecar")
	}
	entries := fact.Entries
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want non-final array and keyed vararg only", entries)
	}
	if entries[0].Value != nonFinalVararg || !entries[0].Suffix.Equal(intSuffix(1)) {
		t.Fatalf("non-final vararg entry = %#v", entries[0])
	}
	if entries[0].Source.Kind != sourceprovenance.SourceVararg || entries[0].Source.Final || !entries[0].Source.Adjusted || entries[0].Source.Expanded {
		t.Fatalf("non-final vararg source = %#v, want adjusted single value", entries[0].Source)
	}
	if entries[1].Value != keyedVararg || !entries[1].Suffix.Equal(fieldChainSuffix("key")) {
		t.Fatalf("keyed vararg entry = %#v", entries[1])
	}
	if entries[1].Source.Kind != sourceprovenance.SourceVararg || entries[1].Source.Final || !entries[1].Source.Adjusted || entries[1].Source.Expanded {
		t.Fatalf("keyed vararg source = %#v, want adjusted single value", entries[1].Source)
	}
	for _, entry := range entries {
		if entry.Value == finalVararg {
			t.Fatalf("final expanding array field was included: %#v", entry)
		}
	}
}

func TestExtractChunkFunctionDefinitionFactPreservesIdentity(t *testing.T) {
	target := ident("f")
	fn := function(nil, localAssign([]string{"inside"}, number("1")))
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: fn,
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, stmt, 1)
	fact, ok := result.FunctionDefinition(points[0])
	if !ok {
		t.Fatalf("missing function definition fact")
	}
	if fact.Stmt != stmt || fact.Name != stmt.Name || fact.Func != fn {
		t.Fatalf("function definition fact = %#v", fact)
	}
	if fact.TargetSymbol != mustIdentSymbol(t, bindings, target) || !fact.HasTargetSymbol {
		t.Fatalf("function definition target = %d/%v", fact.TargetSymbol, fact.HasTargetSymbol)
	}
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, target), "f")
	if !fact.HasTargetPath || !fact.TargetPath.Equal(wantPath) {
		t.Fatalf("function definition target path = %v/%v, want %v", fact.TargetPath, fact.HasTargetPath, wantPath)
	}
	assign, ok := result.OrdinaryAssignment(points[0])
	if !ok {
		t.Fatalf("missing function definition ordinary assignment fact")
	}
	if assign.Value != fn || assign.Source.Kind != sourceprovenance.SourceExpression || assign.Source.Expr != fn {
		t.Fatalf("function definition assignment source = %#v value %p, want function expression %p", assign.Source, assign.Value, fn)
	}
	if !assign.HasSymbol || assign.Symbol != mustIdentSymbol(t, bindings, target) {
		t.Fatalf("function definition assignment symbol = %d/%v, want target symbol", assign.Symbol, assign.HasSymbol)
	}
	if !assign.HasPath || !assign.Path.Equal(wantPath) {
		t.Fatalf("function definition assignment path = %v/%v, want %v", assign.Path, assign.HasPath, wantPath)
	}
}

func TestExtractChunkFunctionDefinitionWithNilBindingsHasNoTargetSymbol(t *testing.T) {
	target := ident("f")
	fn := function(nil)
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: fn,
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, nil, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, stmt, 1)
	fact, ok := result.FunctionDefinition(points[0])
	if !ok {
		t.Fatalf("missing function definition fact")
	}
	if fact.Stmt != stmt || fact.Name != stmt.Name || fact.Func != fn {
		t.Fatalf("function definition fact = %#v", fact)
	}
	if fact.TargetSymbol != 0 || fact.HasTargetSymbol {
		t.Fatalf("function definition target = %d/%v, want 0/false", fact.TargetSymbol, fact.HasTargetSymbol)
	}
	if fact.HasTargetPath || !fact.TargetPath.IsEmpty() {
		t.Fatalf("function definition target path = %v/%v, want empty/false", fact.TargetPath, fact.HasTargetPath)
	}
}

func TestExtractChunkMemberFunctionDefinitionFactPublishesPathAssignment(t *testing.T) {
	tests := []struct {
		name string
		stmt *ast.FuncDefStmt
	}{
		{
			name: "dotted",
			stmt: &ast.FuncDefStmt{
				Name: &ast.FuncName{Func: dot(ident("module"), "f")},
				Func: function(nil),
			},
		},
		{
			name: "method",
			stmt: &ast.FuncDefStmt{
				Name: &ast.FuncName{Receiver: ident("module"), Method: "f"},
				Func: function(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moduleDecl := localAssign([]string{"module"}, &ast.TableExpr{})
			bodyStmt := localAssign([]string{"inside"}, number("1"))
			tt.stmt.Func.Stmts = []ast.Stmt{bodyStmt}
			stmts := []ast.Stmt{moduleDecl, tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{})
			built := cfgbuild.BuildChunk(stmts, bindings)
			if built == nil || built.Graph == nil {
				t.Fatalf("BuildChunk returned nil")
			}

			points := requireStmtPoints(t, built, tt.stmt, 1)
			node := built.Graph.Node(points[0])
			if node == nil || node.Kind != cfg.NodeAssign {
				t.Fatalf("member function point kind = %#v, want assign", node)
			}

			result, err := ExtractChunk(stmts, bindings, built)
			if err != nil {
				t.Fatalf("ExtractChunk: %v", err)
			}
			fact, ok := result.FunctionDefinition(points[0])
			if !ok {
				t.Fatalf("missing function definition fact")
			}
			if fact.Stmt != tt.stmt || fact.Name != tt.stmt.Name || fact.Func != tt.stmt.Func {
				t.Fatalf("function definition fact = %#v", fact)
			}
			if fact.TargetSymbol != 0 || fact.HasTargetSymbol {
				t.Fatalf("member function target = %d/%v, want 0/false", fact.TargetSymbol, fact.HasTargetSymbol)
			}
			wantPath := path.NewPath(mustLocalAt(t, bindings, moduleDecl, 0), "module").Field("f")
			if !fact.HasTargetPath || !fact.TargetPath.Equal(wantPath) {
				t.Fatalf("member function target path = %v/%v, want %v", fact.TargetPath, fact.HasTargetPath, wantPath)
			}
			assign, ok := result.OrdinaryAssignment(points[0])
			if !ok {
				t.Fatalf("missing member function ordinary assignment fact")
			}
			if assign.Value != tt.stmt.Func || assign.Source.Kind != sourceprovenance.SourceExpression || assign.Source.Expr != tt.stmt.Func {
				t.Fatalf("member function assignment source = %#v value %p, want function expression %p", assign.Source, assign.Value, tt.stmt.Func)
			}
			if !assign.HasPath || !assign.Path.Equal(wantPath) {
				t.Fatalf("member function assignment path = %v/%v, want %v", assign.Path, assign.HasPath, wantPath)
			}
			if assign.HasSymbol || assign.Symbol != 0 {
				t.Fatalf("member function assignment symbol = %d/%v, want 0/false", assign.Symbol, assign.HasSymbol)
			}
			if got := built.StmtPoints.PointsFor(bodyStmt); len(got) != 0 {
				t.Fatalf("nested member function body statement mapped to parent CFG points %v", got)
			}
		})
	}
}

func TestExtractChunkLabelFactPreservesIdentity(t *testing.T) {
	label := &ast.LabelStmt{Name: "again"}
	stmts := []ast.Stmt{label}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, label, 1)[0]
	fact, ok := result.Label(point)
	if !ok {
		t.Fatalf("missing label fact")
	}
	if fact.Stmt != label || fact.Name != "again" {
		t.Fatalf("label fact = %#v", fact)
	}
	if _, ok := result.LocalAssignment(point); ok {
		t.Fatalf("label point produced local assignment fact")
	}
	if _, ok := result.OrdinaryAssignment(point); ok {
		t.Fatalf("label point produced ordinary assignment fact")
	}
	if _, ok := result.FunctionDefinition(point); ok {
		t.Fatalf("label point produced function definition fact")
	}
}

func TestExtractChunkGotoFactPreservesIdentity(t *testing.T) {
	jump := &ast.GotoStmt{Label: "again"}
	stmts := []ast.Stmt{jump}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, jump, 1)[0]
	fact, ok := result.Goto(point)
	if !ok {
		t.Fatalf("missing goto fact")
	}
	if fact.Stmt != jump || fact.Label != "again" {
		t.Fatalf("goto fact = %#v", fact)
	}
	if _, ok := result.Label(point); ok {
		t.Fatalf("goto point produced label fact")
	}
	if _, ok := result.LocalAssignment(point); ok {
		t.Fatalf("goto point produced local assignment fact")
	}
	if _, ok := result.OrdinaryAssignment(point); ok {
		t.Fatalf("goto point produced ordinary assignment fact")
	}
}

func TestExtractChunkCallReturnBranchAndTypeFacts(t *testing.T) {
	decl := localAssign([]string{"x"}, number("1"))
	printIdent := ident("print")
	xArg := ident("x")
	callExpr := &ast.FuncCallExpr{Func: printIdent, Args: []ast.Expr{xArg}}
	callStmt := &ast.FuncCallStmt{Expr: callExpr}
	xCond := ident("x")
	ifStmt := &ast.IfStmt{Condition: xCond}
	whileStmt := &ast.WhileStmt{Condition: ident("x")}
	repeatStmt := &ast.RepeatStmt{
		Stmts:     []ast.Stmt{localAssign([]string{"again"}, number("2"))},
		Condition: ident("x"),
	}
	typeDef := &ast.TypeDefStmt{Name: "Alias", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	interfaceDef := &ast.InterfaceDefStmt{Name: "Shape"}
	retExpr := ident("x")
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{retExpr}}
	stmts := []ast.Stmt{decl, callStmt, ifStmt, whileStmt, repeatStmt, typeDef, interfaceDef, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"print"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	callPoints := requireStmtPoints(t, built, callStmt, 1)
	call, ok := result.Call(callPoints[0])
	if !ok {
		t.Fatalf("missing call fact")
	}
	if call.Stmt != callStmt || call.Call != callExpr || call.Func != printIdent || len(call.Args) != 1 || call.Args[0] != xArg {
		t.Fatalf("call fact = %#v", call)
	}
	if call.CalleeSymbol != mustIdentSymbol(t, bindings, printIdent) || !call.HasCalleeSymbol {
		t.Fatalf("call symbol = %d/%v", call.CalleeSymbol, call.HasCalleeSymbol)
	}
	call.Args[0] = ident("mutated")
	callAgain, _ := result.Call(callPoints[0])
	if callAgain.Args[0] != xArg {
		t.Fatalf("Call exposed mutable args slice")
	}
	view, ok := result.CallView(callPoints[0])
	if !ok {
		t.Fatalf("missing call view")
	}
	borrowed, ok := view.Borrowed()
	if !ok || borrowed.Args[0] != xArg || borrowed.Call != callExpr {
		t.Fatalf("borrowed call fact = %#v, ok=%v", borrowed, ok)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		view, ok := result.CallView(callPoints[0])
		if !ok {
			t.Fatalf("missing call view")
		}
		borrowed, ok := view.Borrowed()
		if !ok || borrowed.Call == nil {
			t.Fatalf("borrowed call fact = %#v, ok=%v", borrowed, ok)
		}
	})
	if allocs != 0 {
		t.Fatalf("CallView allocations/run = %.1f, want zero", allocs)
	}

	for _, tt := range []struct {
		stmt ast.Stmt
		kind BranchKind
		cond ast.Expr
	}{
		{ifStmt, BranchIf, xCond},
		{whileStmt, BranchWhile, whileStmt.Condition},
		{repeatStmt, BranchRepeat, repeatStmt.Condition},
	} {
		points := requireStmtPoints(t, built, tt.stmt, 1)
		fact, ok := result.BranchCondition(points[0])
		if !ok || fact.Kind != tt.kind || fact.Stmt != tt.stmt || fact.Condition != tt.cond {
			t.Fatalf("branch fact for %T = %#v, ok=%v", tt.stmt, fact, ok)
		}
	}

	typePoint := requireStmtPoints(t, built, typeDef, 1)[0]
	if node := built.Graph.Node(typePoint); node == nil || node.Kind != cfg.NodeNoop {
		t.Fatalf("type def cfg node = %#v, want NodeNoop", node)
	}
	typeFact, ok := result.TypeDefinition(typePoint)
	if !ok || typeFact.Kind != cfgfacts.TypeDefinitionAlias || typeFact.Type != typeDef {
		t.Fatalf("type def fact = %#v, ok=%v", typeFact, ok)
	}
	interfacePoint := requireStmtPoints(t, built, interfaceDef, 1)[0]
	if node := built.Graph.Node(interfacePoint); node == nil || node.Kind != cfg.NodeNoop {
		t.Fatalf("interface def cfg node = %#v, want NodeNoop", node)
	}
	interfaceFact, ok := result.TypeDefinition(interfacePoint)
	if !ok || interfaceFact.Kind != cfgfacts.TypeDefinitionInterface || interfaceFact.Interface != interfaceDef {
		t.Fatalf("interface def fact = %#v, ok=%v", interfaceFact, ok)
	}

	returnPoint := requireStmtPoints(t, built, ret, 1)[0]
	returnFact, ok := result.Return(returnPoint)
	if !ok || returnFact.Stmt != ret || len(returnFact.Exprs) != 1 || returnFact.Exprs[0] != retExpr {
		t.Fatalf("return fact = %#v, ok=%v", returnFact, ok)
	}
	returnFact.Exprs[0] = ident("mutated")
	returnAgain, _ := result.Return(returnPoint)
	if returnAgain.Exprs[0] != retExpr {
		t.Fatalf("Return exposed mutable expr slice")
	}
	returnView, ok := result.ReturnView(returnPoint)
	if !ok {
		t.Fatalf("missing return view")
	}
	borrowedReturn, ok := returnView.Borrowed()
	if !ok || borrowedReturn.Exprs[0] != retExpr {
		t.Fatalf("borrowed return fact = %#v, ok=%v", borrowedReturn, ok)
	}
	returnAllocs := testing.AllocsPerRun(1000, func() {
		view, ok := result.ReturnView(returnPoint)
		if !ok {
			t.Fatalf("missing return view")
		}
		borrowed, ok := view.Borrowed()
		if !ok || len(borrowed.Exprs) == 0 {
			t.Fatalf("borrowed return fact = %#v, ok=%v", borrowed, ok)
		}
	})
	if returnAllocs != 0 {
		t.Fatalf("ReturnView allocations/run = %.1f, want zero", returnAllocs)
	}
}

func TestExtractParsedFunctionChannelReceiveCallFact(t *testing.T) {
	stmts, err := parse.ParseString(`
local function handle(ch)
	local value, ok = ch:receive()
	if ok then
		local id = value.id
	end
end
`, "test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	built := cfgbuild.BuildFunction(functions[0], bindings)
	result, err := ExtractFunction(functions[0], bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	var found bool
	for _, point := range built.Graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Method != "receive" {
			continue
		}
		found = true
		if fact.Context != CallContextAssignmentSource {
			t.Fatalf("receive context = %v, want assignment source", fact.Context)
		}
		if len(fact.ResultTargets) != 2 {
			t.Fatalf("receive result targets = %#v, want value and ok", fact.ResultTargets)
		}
		if fact.CallSpan.StartLine == 0 || fact.CalleeSpan.StartLine == 0 {
			t.Fatalf("receive spans call=%#v callee=%#v, want syntax-free call and callee spans", fact.CallSpan, fact.CalleeSpan)
		}
		if fact.CallSpan.StartLine > fact.CalleeSpan.StartLine ||
			(fact.CallSpan.StartLine == fact.CalleeSpan.StartLine && fact.CallSpan.StartCol > fact.CalleeSpan.StartCol) {
			t.Fatalf("receive spans call=%#v callee=%#v, want call span to cover callee", fact.CallSpan, fact.CalleeSpan)
		}
	}
	if !found {
		t.Fatal("missing receive call fact")
	}
}

func TestExtractChunkAssignmentAndReturnCallFactsUseLuaListRules(t *testing.T) {
	makeIdent := ident("make")
	makeCall := &ast.FuncCallExpr{Func: makeIdent}
	packIdent := ident("pack")
	packCall := &ast.FuncCallExpr{Func: packIdent}
	local := localAssign([]string{"a", "b", "c"}, makeCall, packCall)
	aRead := ident("a")
	tailIdent := ident("tail")
	tailCall := &ast.FuncCallExpr{Func: tailIdent}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{aRead, tailCall}}
	stmts := []ast.Stmt{local, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "pack", "tail"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	localPoints := requireStmtPoints(t, built, local, 5)
	makeFact, ok := result.Call(localPoints[0])
	if !ok {
		t.Fatalf("missing make call fact")
	}
	if makeFact.Context != CallContextAssignmentSource || makeFact.SourceStmt != local || makeFact.ExprIndex != 0 {
		t.Fatalf("make context = %#v", makeFact)
	}
	if makeFact.Final || makeFact.Expanded || !makeFact.Adjusted || makeFact.OpenTail {
		t.Fatalf("make flags = final:%v expanded:%v adjusted:%v open:%v", makeFact.Final, makeFact.Expanded, makeFact.Adjusted, makeFact.OpenTail)
	}
	if makeFact.CalleeSymbol != mustIdentSymbol(t, bindings, makeIdent) || !makeFact.HasCalleeSymbol {
		t.Fatalf("make callee symbol = %d/%v", makeFact.CalleeSymbol, makeFact.HasCalleeSymbol)
	}
	if len(makeFact.ResultTargets) != 1 || makeFact.ResultTargets[0].Kind != CallResultTargetLocalAssignment || makeFact.ResultTargets[0].Index != 0 || makeFact.ResultTargets[0].ResultIndex != 0 || makeFact.ResultTargets[0].Name != "a" {
		t.Fatalf("make result targets = %#v", makeFact.ResultTargets)
	}

	packFact, ok := result.Call(localPoints[1])
	if !ok {
		t.Fatalf("missing pack call fact")
	}
	if packFact.Context != CallContextAssignmentSource || packFact.ExprIndex != 1 || !packFact.Final || !packFact.Expanded || packFact.Adjusted {
		t.Fatalf("pack fact = %#v", packFact)
	}
	if len(packFact.ResultTargets) != 2 || packFact.ResultTargets[0].Index != 1 || packFact.ResultTargets[0].ResultIndex != 0 || packFact.ResultTargets[1].Index != 2 || packFact.ResultTargets[1].ResultIndex != 1 {
		t.Fatalf("pack result targets = %#v", packFact.ResultTargets)
	}

	aFact, ok := result.LocalAssignment(localPoints[2])
	if !ok {
		t.Fatalf("missing local a fact")
	}
	if aFact.Source.Kind != sourceprovenance.SourceCall || aFact.Source.Expr != makeCall || aFact.Source.ExprIndex != 0 || aFact.Source.ResultIndex != 0 || !aFact.Source.Adjusted || aFact.Source.CallPoint != localPoints[0] || !aFact.Source.HasCallPoint {
		t.Fatalf("a source = %#v", aFact.Source)
	}
	bFact, ok := result.LocalAssignment(localPoints[3])
	if !ok {
		t.Fatalf("missing local b fact")
	}
	cFact, ok := result.LocalAssignment(localPoints[4])
	if !ok {
		t.Fatalf("missing local c fact")
	}
	if bFact.Source.Kind != sourceprovenance.SourceCall || bFact.Source.Expr != packCall || !bFact.Source.Expanded || bFact.Source.ResultIndex != 0 || bFact.Source.CallPoint != localPoints[1] || !bFact.Source.HasCallPoint {
		t.Fatalf("b source = %#v", bFact.Source)
	}
	if cFact.Source.Kind != sourceprovenance.SourceCall || cFact.Source.Expr != packCall || !cFact.Source.Expanded || cFact.Source.ResultIndex != 1 || cFact.Source.CallPoint != localPoints[1] || !cFact.Source.HasCallPoint {
		t.Fatalf("c source = %#v", cFact.Source)
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	tailFact, ok := result.Call(returnPoints[0])
	if !ok {
		t.Fatalf("missing return tail call fact")
	}
	if tailFact.Context != CallContextReturnSource || tailFact.SourceStmt != ret || tailFact.ExprIndex != 1 || !tailFact.Final || !tailFact.Expanded || !tailFact.OpenTail {
		t.Fatalf("tail fact = %#v", tailFact)
	}
	if len(tailFact.ResultTargets) != 1 || tailFact.ResultTargets[0].Kind != CallResultTargetReturn || tailFact.ResultTargets[0].Index != 1 || tailFact.ResultTargets[0].ResultIndex != 0 || !tailFact.ResultTargets[0].OpenTail {
		t.Fatalf("tail result targets = %#v", tailFact.ResultTargets)
	}
	returnFact, ok := result.Return(returnPoints[1])
	if !ok {
		t.Fatalf("missing return fact")
	}
	if len(returnFact.Sources) != 2 || returnFact.Sources[0].Kind != sourceprovenance.SourceExpression || returnFact.Sources[0].Expr != aRead {
		t.Fatalf("return first source = %#v", returnFact.Sources)
	}
	if returnFact.Sources[1].Kind != sourceprovenance.SourceCall || returnFact.Sources[1].Expr != tailCall || !returnFact.Sources[1].Expanded || !returnFact.Sources[1].OpenTail || returnFact.Sources[1].CallPoint != returnPoints[0] || !returnFact.Sources[1].HasCallPoint {
		t.Fatalf("return tail source = %#v", returnFact.Sources[1])
	}
	returnFact.Sources[1].Kind = sourceprovenance.SourceNil
	returnAgain, _ := result.Return(returnPoints[1])
	if returnAgain.Sources[1].Kind != sourceprovenance.SourceCall {
		t.Fatalf("Return exposed mutable sources slice")
	}
}

func TestExtractChunkValueShortCircuitAssignmentCallFacts(t *testing.T) {
	orMakeCall := &ast.FuncCallExpr{Func: ident("make")}
	orExpr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      ident("cached"),
		Rhs:      orMakeCall,
	}
	orLocal := localAssign([]string{"x"}, orExpr)
	andMakeCall := &ast.FuncCallExpr{Func: ident("make")}
	andExpr := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      ident("guard"),
		Rhs:      andMakeCall,
	}
	andLocal := localAssign([]string{"y"}, andExpr)
	stmts := []ast.Stmt{orLocal, andLocal}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"cached", "guard", "make"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	orPoints := requireStmtPoints(t, built, orLocal, 2)
	orCallFact, ok := result.Call(orPoints[0])
	if !ok || orCallFact.Call != orMakeCall || orCallFact.Context != CallContextExpressionProducer || orCallFact.ExprIndex != 0 {
		t.Fatalf("or call fact = %#v, ok=%v", orCallFact, ok)
	}
	if len(orCallFact.ResultTargets) != 1 || orCallFact.ResultTargets[0].Kind != CallResultTargetExpression || orCallFact.ResultTargets[0].ResultIndex != 0 {
		t.Fatalf("or call result targets = %#v", orCallFact.ResultTargets)
	}
	orAssign, ok := result.LocalAssignment(orPoints[1])
	if !ok || orAssign.Source.Kind != sourceprovenance.SourceExpression || orAssign.Source.Expr != orExpr {
		t.Fatalf("or assignment source = %#v, ok=%v", orAssign.Source, ok)
	}

	andPoints := requireStmtPoints(t, built, andLocal, 2)
	andCallFact, ok := result.Call(andPoints[0])
	if !ok || andCallFact.Call != andMakeCall || andCallFact.Context != CallContextExpressionProducer || andCallFact.ExprIndex != 0 {
		t.Fatalf("and call fact = %#v, ok=%v", andCallFact, ok)
	}
	if len(andCallFact.ResultTargets) != 1 || andCallFact.ResultTargets[0].Kind != CallResultTargetExpression || andCallFact.ResultTargets[0].ResultIndex != 0 {
		t.Fatalf("and call result targets = %#v", andCallFact.ResultTargets)
	}
	andAssign, ok := result.LocalAssignment(andPoints[1])
	if !ok || andAssign.Source.Kind != sourceprovenance.SourceExpression || andAssign.Source.Expr != andExpr {
		t.Fatalf("and assignment source = %#v, ok=%v", andAssign.Source, ok)
	}
}

func TestExtractChunkNestedStatementArgumentCallSourcesPointAtInnerCall(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	outer := &ast.FuncCallExpr{Func: ident("f"), Args: []ast.Expr{inner}}
	stmt := &ast.FuncCallStmt{Expr: outer}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f", "g"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, stmt, 2)
	innerFact, ok := result.Call(points[0])
	if !ok {
		t.Fatalf("missing inner call fact")
	}
	if innerFact.Call != inner || innerFact.Context != CallContextExpressionProducer || !innerFact.Final || !innerFact.Adjusted {
		t.Fatalf("inner call fact = %#v", innerFact)
	}
	if len(innerFact.ResultTargets) != 1 || innerFact.ResultTargets[0].Kind != CallResultTargetExpression || innerFact.ResultTargets[0].ResultIndex != 0 {
		t.Fatalf("inner result targets = %#v", innerFact.ResultTargets)
	}
	outerFact, ok := result.Call(points[1])
	if !ok {
		t.Fatalf("missing outer call fact")
	}
	if outerFact.Call != outer || outerFact.Context != CallContextStatement || outerFact.Stmt != stmt {
		t.Fatalf("outer call fact = %#v", outerFact)
	}
	if len(outerFact.ArgumentSources) != 1 {
		t.Fatalf("outer argument sources = %#v, want one", outerFact.ArgumentSources)
	}
	arg := outerFact.ArgumentSources[0]
	if arg.Kind != sourceprovenance.SourceCall || arg.Expr != inner || arg.CallPoint != points[0] || !arg.HasCallPoint || arg.ResultIndex != 0 {
		t.Fatalf("outer argument source = %#v, want inner call point %d", arg, points[0])
	}
}

func TestExtractChunkMemberReadCallReceiverIsExpressionProducer(t *testing.T) {
	lookupCall := &ast.FuncCallExpr{Func: dot(ident("store"), "lookup")}
	memberRead := dot(lookupCall, "status")
	local := localAssign([]string{"status"}, memberRead)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"store"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, local, 2)
	callFact, ok := result.Call(points[0])
	if !ok {
		t.Fatalf("missing member receiver call fact at point %d", points[0])
	}
	if callFact.Call != lookupCall || callFact.Context != CallContextExpressionProducer {
		t.Fatalf("call fact = %#v, want lookup expression producer", callFact)
	}
	if len(callFact.ResultTargets) != 1 || callFact.ResultTargets[0].Kind != CallResultTargetExpression || callFact.ResultTargets[0].ResultIndex != 0 {
		t.Fatalf("lookup result targets = %#v, want expression slot 0", callFact.ResultTargets)
	}
	assign, ok := result.LocalAssignment(points[1])
	if !ok || assign.Expr != memberRead {
		t.Fatalf("assignment = %#v, ok=%v", assign, ok)
	}
}

func TestExtractChunkObjectLiteralEntryCallSourcePointsAtNestedCall(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	table := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       stringLit("x"),
		KeySyntax: ast.AttrKeyDot,
		Value:     makeCall,
	}}}
	local := localAssign([]string{"t"}, table)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, local, 2)
	callFact, ok := result.Call(points[0])
	if !ok || callFact.Call != makeCall || callFact.Context != CallContextExpressionProducer {
		t.Fatalf("make call fact = %#v, ok=%v", callFact, ok)
	}
	literal, ok := result.ObjectLiteral(table)
	if !ok || len(literal.Entries) != 1 {
		t.Fatalf("object literal = %#v, ok=%v", literal, ok)
	}
	entrySource := literal.Entries[0].Source
	if entrySource.Kind != sourceprovenance.SourceCall || entrySource.Expr != makeCall || entrySource.CallPoint != points[0] || !entrySource.HasCallPoint {
		t.Fatalf("object entry source = %#v, want make call point %d", entrySource, points[0])
	}
}

func TestExtractChunkConditionAndIteratorCallFactsUseDeferredContexts(t *testing.T) {
	readyIdent := ident("ready")
	readyCall := &ast.FuncCallExpr{Func: readyIdent}
	ifStmt := &ast.IfStmt{Condition: readyCall}
	iterIdent := ident("iter")
	iterCall := &ast.FuncCallExpr{Func: iterIdent}
	stateIdent := ident("state")
	stateCall := &ast.FuncCallExpr{Func: stateIdent}
	loop := &ast.GenericForStmt{
		Names: []string{"k"},
		Exprs: []ast.Expr{iterCall, stateCall},
	}
	stmts := []ast.Stmt{ifStmt, loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ready", "iter", "state"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	ifPoints := requireStmtPoints(t, built, ifStmt, 2)
	conditionCall, ok := result.Call(ifPoints[0])
	if !ok {
		t.Fatalf("missing condition call fact")
	}
	if conditionCall.Context != CallContextCondition || conditionCall.SourceStmt != ifStmt || conditionCall.ExprIndex != 0 {
		t.Fatalf("condition call context = %#v", conditionCall)
	}
	if conditionCall.ConditionNegated {
		t.Fatalf("condition call unexpectedly negated: %#v", conditionCall)
	}
	if !conditionCall.Final || conditionCall.Expanded || !conditionCall.Adjusted || conditionCall.OpenTail {
		t.Fatalf("condition call flags = %#v", conditionCall)
	}
	if conditionCall.CalleeSymbol != mustIdentSymbol(t, bindings, readyIdent) || !conditionCall.HasCalleeSymbol {
		t.Fatalf("condition callee symbol = %d/%v", conditionCall.CalleeSymbol, conditionCall.HasCalleeSymbol)
	}
	if len(conditionCall.ResultTargets) != 0 {
		t.Fatalf("condition result targets = %#v, want none", conditionCall.ResultTargets)
	}
	branchFact, ok := result.BranchCondition(ifPoints[1])
	if !ok {
		t.Fatalf("missing condition branch fact")
	}
	if branchFact.Source.Kind != sourceprovenance.SourceCall || branchFact.Source.Expr != readyCall || branchFact.Source.CallPoint != ifPoints[0] || !branchFact.Source.HasCallPoint {
		t.Fatalf("condition source = %#v", branchFact.Source)
	}
	if branchFact.Source.TargetIndex != sourceprovenance.NoSourceIndex || !branchFact.Source.Adjusted || branchFact.Source.Expanded {
		t.Fatalf("condition source flags = %#v", branchFact.Source)
	}

	canAccessCall := &ast.FuncCallExpr{Func: ident("can_access"), Args: []ast.Expr{ident("page")}}
	guardCondition := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      ident("mr"),
		Rhs: &ast.LogicalOpExpr{
			Operator: "or",
			Lhs:      &ast.UnaryNotOpExpr{Expr: dot(ident("page"), "secure")},
			Rhs:      canAccessCall,
		},
	}
	guardStmt := &ast.IfStmt{Condition: guardCondition}
	guardStmts := []ast.Stmt{guardStmt}
	guardBindings := bind.BindChunk(guardStmts, bind.Options{Globals: []string{"can_access", "mr", "page"}})
	guardBuilt := cfgbuild.BuildChunk(guardStmts, guardBindings)
	guardResult, err := ExtractChunk(guardStmts, guardBindings, guardBuilt)
	if err != nil {
		t.Fatalf("ExtractChunk nested guard: %v", err)
	}
	guardPoints := requireStmtPoints(t, guardBuilt, guardStmt, 2)
	guardCall, ok := guardResult.Call(guardPoints[0])
	if !ok {
		t.Fatalf("missing nested condition call fact")
	}
	if guardCall.Context != CallContextExpressionProducer || guardCall.Call != canAccessCall {
		t.Fatalf("nested condition call fact = %#v", guardCall)
	}
	if _, ok := guardResult.BranchCondition(guardPoints[1]); !ok {
		t.Fatalf("missing nested condition branch fact")
	}

	loopPoints := requireStmtPoints(t, built, loop, 4)
	iterFact, ok := result.Call(loopPoints[0])
	if !ok {
		t.Fatalf("missing iterator call fact")
	}
	if iterFact.Context != CallContextIteratorSource || iterFact.SourceStmt != loop || iterFact.ExprIndex != 0 || iterFact.Final || iterFact.Expanded || !iterFact.Adjusted {
		t.Fatalf("iterator call fact = %#v", iterFact)
	}
	if iterFact.CalleeSymbol != mustIdentSymbol(t, bindings, iterIdent) || !iterFact.HasCalleeSymbol {
		t.Fatalf("iterator callee symbol = %d/%v", iterFact.CalleeSymbol, iterFact.HasCalleeSymbol)
	}
	stateFact, ok := result.Call(loopPoints[1])
	if !ok {
		t.Fatalf("missing final iterator source call fact")
	}
	if stateFact.Context != CallContextIteratorSource || stateFact.ExprIndex != 1 || !stateFact.Final || !stateFact.Expanded || stateFact.Adjusted || stateFact.OpenTail {
		t.Fatalf("final iterator source fact = %#v", stateFact)
	}
	if len(stateFact.ResultTargets) != 0 {
		t.Fatalf("iterator source result targets = %#v, want none", stateFact.ResultTargets)
	}

	genericFact, ok := result.GenericFor(loopPoints[2])
	if !ok {
		t.Fatalf("missing generic for check fact")
	}
	if len(genericFact.Sources) != 2 {
		t.Fatalf("generic for sources = %#v", genericFact.Sources)
	}
	if genericFact.Sources[0].Kind != sourceprovenance.SourceCall || genericFact.Sources[0].Expr != iterCall || genericFact.Sources[0].CallPoint != loopPoints[0] || !genericFact.Sources[0].HasCallPoint || !genericFact.Sources[0].Adjusted {
		t.Fatalf("first generic source = %#v", genericFact.Sources[0])
	}
	if genericFact.Sources[1].Kind != sourceprovenance.SourceCall || genericFact.Sources[1].Expr != stateCall || genericFact.Sources[1].CallPoint != loopPoints[1] || !genericFact.Sources[1].HasCallPoint || !genericFact.Sources[1].Expanded || genericFact.Sources[1].OpenTail {
		t.Fatalf("final generic source = %#v", genericFact.Sources[1])
	}
	genericFact.Sources[0].Kind = sourceprovenance.SourceNil
	genericAgain, _ := result.GenericFor(loopPoints[2])
	if genericAgain.Sources[0].Kind != sourceprovenance.SourceCall {
		t.Fatalf("GenericFor exposed mutable sources slice")
	}
}

func TestExtractChunkNegatedConditionPredicateCallCarriesPolarity(t *testing.T) {
	readyCall := call("ready")
	ifStmt := &ast.IfStmt{Condition: &ast.UnaryNotOpExpr{Expr: readyCall}}
	stmts := []ast.Stmt{ifStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ready"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, ifStmt, 2)
	conditionFact, ok := result.Call(points[0])
	if !ok {
		t.Fatalf("missing condition call fact")
	}
	if conditionFact.Context != CallContextCondition || conditionFact.Call != readyCall {
		t.Fatalf("condition call fact = %#v", conditionFact)
	}
	if !conditionFact.ConditionNegated {
		t.Fatalf("condition call missing unary-not polarity: %#v", conditionFact)
	}
}

func TestExtractChunkConditionPredicateCallPreservesNestedCallEvidence(t *testing.T) {
	tokenCall := call("token")
	authorizeCall := &ast.FuncCallExpr{
		Func: ident("authorize"),
		Args: []ast.Expr{tokenCall},
	}
	ifStmt := &ast.IfStmt{Condition: authorizeCall}
	stmts := []ast.Stmt{ifStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"authorize", "token"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, ifStmt, 3)
	tokenFact, ok := result.Call(points[0])
	if !ok {
		t.Fatalf("missing nested argument call fact")
	}
	if tokenFact.Call != tokenCall || tokenFact.Context != CallContextExpressionProducer || tokenFact.ExprIndex != callorder.NoExprIndex {
		t.Fatalf("nested argument call fact = %#v", tokenFact)
	}
	if len(tokenFact.ResultTargets) != 1 ||
		tokenFact.ResultTargets[0].Kind != CallResultTargetExpression ||
		tokenFact.ResultTargets[0].Index != callorder.NoExprIndex {
		t.Fatalf("nested argument targets = %#v", tokenFact.ResultTargets)
	}

	conditionFact, ok := result.Call(points[1])
	if !ok {
		t.Fatalf("missing condition predicate call fact")
	}
	if conditionFact.Call != authorizeCall || conditionFact.Context != CallContextCondition || conditionFact.ExprIndex != 0 {
		t.Fatalf("condition predicate call fact = %#v", conditionFact)
	}
	if len(conditionFact.ArgumentSources) != 1 {
		t.Fatalf("condition argument sources = %#v, want nested call source", conditionFact.ArgumentSources)
	}
	argSource := conditionFact.ArgumentSources[0]
	if argSource.Kind != sourceprovenance.SourceCall ||
		argSource.Expr != tokenCall ||
		argSource.CallPoint != points[0] ||
		!argSource.HasCallPoint ||
		argSource.ExprIndex != 0 ||
		argSource.TargetIndex != 0 ||
		argSource.ResultIndex != 0 {
		t.Fatalf("condition nested argument source = %#v", argSource)
	}

	branchFact, ok := result.BranchCondition(points[2])
	if !ok {
		t.Fatalf("missing branch condition fact")
	}
	if branchFact.Source.Kind != sourceprovenance.SourceCall ||
		branchFact.Source.Expr != authorizeCall ||
		branchFact.Source.CallPoint != points[1] ||
		!branchFact.Source.HasCallPoint ||
		branchFact.Source.TargetIndex != sourceprovenance.NoSourceIndex ||
		!branchFact.Source.Adjusted {
		t.Fatalf("branch predicate source = %#v", branchFact.Source)
	}
}

func TestExtractChunkAssertionWrappedCallProducersKeepOuterSources(t *testing.T) {
	fooCall := call("foo")
	fooCast := &ast.CastExpr{Expr: fooCall, Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	localCast := localAssign([]string{"x"}, fooCast)

	mustCall := call("must")
	mustAssert := &ast.NonNilAssertExpr{Expr: mustCall}
	localNonNil := localAssign([]string{"y"}, mustAssert)

	barCall := call("bar")
	barCast := &ast.CastExpr{Expr: barCall, Type: &ast.PrimitiveTypeExpr{Name: "string"}}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{barCast}}

	readyCall := call("ready")
	readyCast := &ast.CastExpr{Expr: readyCall, Type: &ast.PrimitiveTypeExpr{Name: "boolean"}}
	ifStmt := &ast.IfStmt{Condition: readyCast}

	iterCall := call("iter")
	iterCast := &ast.CastExpr{Expr: iterCall, Type: &ast.PrimitiveTypeExpr{Name: "any"}}
	loop := &ast.GenericForStmt{Names: []string{"item"}, Exprs: []ast.Expr{iterCast}}

	stmts := []ast.Stmt{localCast, localNonNil, ifStmt, loop, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"foo", "must", "bar", "ready", "iter"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatal("BuildChunk returned nil")
	}
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	localCastPoints := requireStmtPoints(t, built, localCast, 2)
	assertWrappedCallSource(t, result, localCastPoints[0], localCastPoints[1], localCast, fooCall, fooCast)

	localNonNilPoints := requireStmtPoints(t, built, localNonNil, 2)
	assertWrappedCallSource(t, result, localNonNilPoints[0], localNonNilPoints[1], localNonNil, mustCall, mustAssert)

	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnCall, ok := result.Call(returnPoints[0])
	if !ok || returnCall.Call != barCall || returnCall.ExprIndex != 0 || returnCall.Context != CallContextReturnSource {
		t.Fatalf("return call = %#v, ok=%v", returnCall, ok)
	}
	returnFact, ok := result.Return(returnPoints[1])
	if !ok || len(returnFact.Sources) != 1 || returnFact.Sources[0].Kind != sourceprovenance.SourceCall || returnFact.Sources[0].Expr != barCast || returnFact.Sources[0].CallPoint != returnPoints[0] || !returnFact.Sources[0].HasCallPoint {
		t.Fatalf("return sources = %#v, ok=%v", returnFact.Sources, ok)
	}

	ifPoints := requireStmtPoints(t, built, ifStmt, 2)
	conditionCall, ok := result.Call(ifPoints[0])
	if !ok || conditionCall.Call != readyCall || conditionCall.Context != CallContextCondition {
		t.Fatalf("condition call = %#v, ok=%v", conditionCall, ok)
	}
	branchFact, ok := result.BranchCondition(ifPoints[1])
	if !ok || branchFact.Source.Kind != sourceprovenance.SourceCall || branchFact.Source.Expr != readyCast || branchFact.Source.CallPoint != ifPoints[0] || !branchFact.Source.HasCallPoint {
		t.Fatalf("branch source = %#v, ok=%v", branchFact.Source, ok)
	}

	loopPoints := requireStmtPoints(t, built, loop, 3)
	iterCallFact, ok := result.Call(loopPoints[0])
	if !ok || iterCallFact.Call != iterCall || iterCallFact.Context != CallContextIteratorSource {
		t.Fatalf("iterator call = %#v, ok=%v", iterCallFact, ok)
	}
	genericFact, ok := result.GenericFor(loopPoints[1])
	if !ok || len(genericFact.Sources) != 1 || genericFact.Sources[0].Kind != sourceprovenance.SourceCall || genericFact.Sources[0].Expr != iterCast || genericFact.Sources[0].CallPoint != loopPoints[0] || !genericFact.Sources[0].HasCallPoint {
		t.Fatalf("generic sources = %#v, ok=%v", genericFact.Sources, ok)
	}
}

func assertWrappedCallSource(t *testing.T, result *Result, callPoint, assignPoint cfg.Point, stmt *ast.LocalAssignStmt, innerCall *ast.FuncCallExpr, outerExpr ast.Expr) {
	t.Helper()
	callFact, ok := result.Call(callPoint)
	if !ok || callFact.Call != innerCall || callFact.Context != CallContextAssignmentSource || callFact.SourceStmt != stmt {
		t.Fatalf("call fact = %#v, ok=%v", callFact, ok)
	}
	assignFact, ok := result.LocalAssignment(assignPoint)
	if !ok || assignFact.Source.Kind != sourceprovenance.SourceCall || assignFact.Source.Expr != outerExpr || assignFact.Source.CallPoint != callPoint || !assignFact.Source.HasCallPoint {
		t.Fatalf("assignment source = %#v, ok=%v", assignFact.Source, ok)
	}
}

func TestExtractChunkAssignmentValueSourcesHandleAdjustRetNilFillAndVararg(t *testing.T) {
	singleCall := &ast.FuncCallExpr{Func: ident("single"), AdjustRet: true}
	adjusted := assign([]ast.Expr{ident("x"), ident("y")}, singleCall)
	vararg := &ast.Comma3Expr{}
	varargAssign := assign([]ast.Expr{ident("p"), ident("q"), ident("r")}, number("1"), vararg)
	varargReturn := &ast.ReturnStmt{Exprs: []ast.Expr{number("2"), vararg}}
	stmts := []ast.Stmt{adjusted, varargAssign, varargReturn}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"single"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	adjustedPoints := requireStmtPoints(t, built, adjusted, 3)
	callFact, ok := result.Call(adjustedPoints[0])
	if !ok {
		t.Fatalf("missing adjusted call fact")
	}
	if !callFact.Final || callFact.Expanded || !callFact.Adjusted {
		t.Fatalf("adjusted call flags = %#v", callFact)
	}
	if len(callFact.ResultTargets) != 1 || callFact.ResultTargets[0].Kind != CallResultTargetOrdinaryAssignment || callFact.ResultTargets[0].Index != 0 || callFact.ResultTargets[0].ResultIndex != 0 {
		t.Fatalf("adjusted call targets = %#v", callFact.ResultTargets)
	}
	first, ok := result.OrdinaryAssignment(adjustedPoints[1])
	if !ok {
		t.Fatalf("missing first adjusted assignment")
	}
	second, ok := result.OrdinaryAssignment(adjustedPoints[2])
	if !ok {
		t.Fatalf("missing second adjusted assignment")
	}
	if first.Source.Kind != sourceprovenance.SourceCall || first.Source.Expr != singleCall || !first.Source.Final || !first.Source.Adjusted || first.Source.Expanded || first.Source.CallPoint != adjustedPoints[0] || !first.Source.HasCallPoint {
		t.Fatalf("first adjusted source = %#v", first.Source)
	}
	if second.Source.Kind != sourceprovenance.SourceNil || second.Source.ExprIndex != sourceprovenance.NoSourceIndex {
		t.Fatalf("second adjusted source = %#v", second.Source)
	}

	varargPoints := requireStmtPoints(t, built, varargAssign, 3)
	qFact, ok := result.OrdinaryAssignment(varargPoints[1])
	if !ok {
		t.Fatalf("missing q assignment")
	}
	rFact, ok := result.OrdinaryAssignment(varargPoints[2])
	if !ok {
		t.Fatalf("missing r assignment")
	}
	if qFact.Source.Kind != sourceprovenance.SourceVararg || qFact.Source.Expr != vararg || !qFact.Source.Expanded || qFact.Source.ResultIndex != 0 {
		t.Fatalf("q source = %#v", qFact.Source)
	}
	if rFact.Source.Kind != sourceprovenance.SourceVararg || rFact.Source.Expr != vararg || !rFact.Source.Expanded || rFact.Source.ResultIndex != 1 {
		t.Fatalf("r source = %#v", rFact.Source)
	}

	returnPoint := requireStmtPoints(t, built, varargReturn, 1)[0]
	returnFact, ok := result.Return(returnPoint)
	if !ok {
		t.Fatalf("missing vararg return fact")
	}
	if len(returnFact.Sources) != 2 || returnFact.Sources[1].Kind != sourceprovenance.SourceVararg || !returnFact.Sources[1].Expanded || !returnFact.Sources[1].OpenTail {
		t.Fatalf("vararg return sources = %#v", returnFact.Sources)
	}
}

func TestExtractChunkCallFactResolvesMethodPaths(t *testing.T) {
	obj := ident("obj")
	arg := ident("arg")
	callExpr := &ast.FuncCallExpr{Receiver: obj, Method: "run", Args: []ast.Expr{arg}}
	stmt := &ast.FuncCallStmt{Expr: callExpr}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"obj", "arg"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, stmt, 1)[0]
	fact, ok := result.Call(point)
	if !ok {
		t.Fatalf("missing method call fact")
	}
	if fact.Context != CallContextStatement || fact.SourceStmt != stmt || !fact.Final || !fact.Adjusted || fact.Expanded {
		t.Fatalf("method call flags = %#v", fact)
	}
	receiverPath := path.NewPath(mustIdentSymbol(t, bindings, obj), "obj")
	methodPath := receiverPath.Field("run")
	if !fact.HasReceiverPath || !fact.ReceiverPath.Equal(receiverPath) {
		t.Fatalf("receiver path = %#v, want %#v", fact.ReceiverPath, receiverPath)
	}
	if !fact.HasMethodPath || !fact.MethodPath.Equal(methodPath) {
		t.Fatalf("method path = %#v, want %#v", fact.MethodPath, methodPath)
	}
	if !fact.HasCalleePath || !fact.CalleePath.Equal(methodPath) {
		t.Fatalf("callee path = %#v, want %#v", fact.CalleePath, methodPath)
	}
	if !fact.CalleeMemberAccess {
		t.Fatalf("method call did not carry member-access evidence")
	}
	if !fact.HasReceiverSource || fact.ReceiverSource.Kind != sourceprovenance.SourceExpression || fact.ReceiverSource.Expr != obj {
		t.Fatalf("receiver source = %#v, want expression source for receiver", fact.ReceiverSource)
	}
	if len(fact.ArgumentSources) != 1 || fact.ArgumentSources[0].Kind != sourceprovenance.SourceExpression || fact.ArgumentSources[0].Expr != arg || fact.ArgumentSources[0].ExprIndex != 0 || fact.ArgumentSources[0].TargetIndex != 0 || fact.ArgumentSources[0].ResultIndex != 0 || !fact.ArgumentSources[0].Final {
		t.Fatalf("method argument sources = %#v", fact.ArgumentSources)
	}
	fact.ArgumentSources[0].Kind = sourceprovenance.SourceNil
	fact.ReceiverSource.Kind = sourceprovenance.SourceNil
	fact.MethodPath.Segments[0].Name = "mutated"
	again, _ := result.Call(point)
	if !again.MethodPath.Equal(methodPath) {
		t.Fatalf("Call exposed mutable method path: %#v", again.MethodPath)
	}
	if !again.HasReceiverSource || again.ReceiverSource.Kind != sourceprovenance.SourceExpression {
		t.Fatalf("Call exposed mutable receiver source: %#v", again.ReceiverSource)
	}
	if again.ArgumentSources[0].Kind != sourceprovenance.SourceExpression {
		t.Fatalf("Call exposed mutable argument sources: %#v", again.ArgumentSources)
	}
}

func TestExtractChunkChannelSelectFacts(t *testing.T) {
	stmts, err := parse.ParseString(`
type Event = {kind: string}
type Stop = {reason: string}

function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>)
	local result = channel.select { events_ch:case_receive(), stop_ch:case_receive() }
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	var fn *ast.FunctionExpr
	for _, stmt := range stmts {
		if def, ok := stmt.(*ast.FuncDefStmt); ok {
			fn = def.Func
			break
		}
	}
	if fn == nil || len(fn.Stmts) != 1 {
		t.Fatalf("parsed function = %#v", fn)
	}
	local, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.LocalAssignStmt", fn.Stmts[0])
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	selectCall, ok := local.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("local expr = %T, want *ast.FuncCallExpr", local.Exprs[0])
	}

	result := newResult(nil)
	targets := localResultTargets(local, bindings)
	callFact := buildCallFact(local, nil, CallContextAssignmentSource, local.Exprs, 0, selectCall, bindings, targets, nil)
	secondFact := callFact
	secondFact.ChannelSelect.ResultTarget.Path = path.NewPath(symbol.ID(9999), "second")
	result.setCall(2, secondFact)
	result.setCall(1, callFact)
	if !callFact.HasChannelSelect {
		t.Fatalf("call fact missing channel select annotation: %#v", callFact)
	}
	pointFact, ok := result.ChannelSelect(1)
	if !ok {
		t.Fatalf("missing point-keyed channel select")
	}
	if pointFact.Call != callFact.Call {
		t.Fatalf("point select call = %p, want %p", pointFact.Call, callFact.Call)
	}
	selects := result.ChannelSelects()
	if len(selects) != 2 {
		t.Fatalf("channel selects = %#v, want two", selects)
	}
	selectFact := selects[0]
	if selectFact.Call != callFact.Call {
		t.Fatalf("select call = %p, want %p", selectFact.Call, callFact.Call)
	}
	wantResultPath := path.NewPath(mustLocalAt(t, bindings, local, 0), "result")
	if selectFact.ResultTarget.Kind != CallResultTargetLocalAssignment || !selectFact.ResultTarget.HasPath || !selectFact.ResultTarget.Path.Equal(wantResultPath) {
		t.Fatalf("result target = %#v, want path %#v", selectFact.ResultTarget, wantResultPath)
	}
	wantSecondPath := path.NewPath(symbol.ID(9999), "second")
	if !selects[1].ResultTarget.HasPath || !selects[1].ResultTarget.Path.Equal(wantSecondPath) {
		t.Fatalf("second result target = %#v, want path %#v", selects[1].ResultTarget, wantSecondPath)
	}
	if len(selectFact.Cases) != 2 {
		t.Fatalf("cases = %#v, want two", selectFact.Cases)
	}
	wantNames := []string{"events_ch", "stop_ch"}
	for i, wantName := range wantNames {
		receiver, ok := selectFact.Cases[i].CaseCall.Receiver.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("case %d receiver = %T, want *ast.IdentExpr", i, selectFact.Cases[i].CaseCall.Receiver)
		}
		wantPath := path.NewPath(mustIdentSymbol(t, bindings, receiver), wantName)
		if !selectFact.Cases[i].HasChannelPath || !selectFact.Cases[i].ChannelPath.Equal(wantPath) {
			t.Fatalf("case %d path = %#v, want %#v", i, selectFact.Cases[i].ChannelPath, wantPath)
		}
	}

	originalCasePath := selectFact.Cases[0].ChannelPath.Clone()
	selects[0].Cases[0].ChannelPath.Segments = append(selects[0].Cases[0].ChannelPath.Segments, segment.Segment{Kind: segment.SegmentField, Name: "mutated"})
	again := result.ChannelSelects()
	if !again[0].Cases[0].ChannelPath.Equal(originalCasePath) {
		t.Fatalf("ChannelSelects exposed mutable channel path")
	}
}

func TestExtractChunkChannelSelectFactsPreserveDuplicateCasePaths(t *testing.T) {
	stmts, err := parse.ParseString(`
type Event = {kind: string}
type Stop = {reason: string}

function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>)
	local result = channel.select { events_ch:case_receive(), events_ch:case_receive(), stop_ch:case_receive() }
end
`, "test")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	var fn *ast.FunctionExpr
	for _, stmt := range stmts {
		if def, ok := stmt.(*ast.FuncDefStmt); ok {
			fn = def.Func
			break
		}
	}
	if fn == nil || len(fn.Stmts) != 1 {
		t.Fatalf("parsed function = %#v", fn)
	}
	local, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want *ast.LocalAssignStmt", fn.Stmts[0])
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	selectCall, ok := local.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("local expr = %T, want *ast.FuncCallExpr", local.Exprs[0])
	}

	targets := localResultTargets(local, bindings)
	callFact := buildCallFact(local, nil, CallContextAssignmentSource, local.Exprs, 0, selectCall, bindings, targets, nil)
	if !callFact.HasChannelSelect {
		t.Fatalf("call fact missing channel select annotation: %#v", callFact)
	}
	selectFact := callFact.ChannelSelect
	if len(selectFact.Cases) != 3 {
		t.Fatalf("cases = %#v, want three", selectFact.Cases)
	}
	firstPath := selectFact.Cases[0].ChannelPath
	secondPath := selectFact.Cases[1].ChannelPath
	thirdPath := selectFact.Cases[2].ChannelPath
	if !selectFact.Cases[0].HasChannelPath || !selectFact.Cases[1].HasChannelPath || !firstPath.Equal(secondPath) {
		t.Fatalf("duplicate case paths = %#v / %#v, want equal channel paths", firstPath, secondPath)
	}
	if !selectFact.Cases[2].HasChannelPath || thirdPath.Equal(firstPath) {
		t.Fatalf("third case path = %#v, want distinct stop channel path", thirdPath)
	}
}

func TestExtractChunkChannelSelectFactsRejectsUnanchoredCalls(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "arbitrary select callee",
			src: `
local ch: Channel<string>
local result = foo.select { ch:case_receive() }
`,
		},
		{
			name: "arbitrary case receive receiver",
			src: `
local obj: {case_receive: () -> string}
local result = channel.select { obj:case_receive() }
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := parse.ParseString(tt.src, "test")
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel", "foo"}})
			local, ok := stmts[len(stmts)-1].(*ast.LocalAssignStmt)
			if !ok {
				t.Fatalf("last stmt = %T, want *ast.LocalAssignStmt", stmts[len(stmts)-1])
			}
			selectCall, ok := local.Exprs[0].(*ast.FuncCallExpr)
			if !ok {
				t.Fatalf("local expr = %T, want *ast.FuncCallExpr", local.Exprs[0])
			}
			targets := localResultTargets(local, bindings)
			callFact := buildCallFact(local, nil, CallContextAssignmentSource, local.Exprs, 0, selectCall, bindings, targets, nil)
			if callFact.HasChannelSelect {
				t.Fatalf("unanchored call produced channel select fact: %#v", callFact.ChannelSelect)
			}
		})
	}
}

func TestExtractChunkBranchConditionChecksResolvePaths(t *testing.T) {
	tests := []struct {
		name     string
		expr     func(*ast.IdentExpr) ast.Expr
		want     branchcond.CheckKind
		wantPath func(symbol.ID) path.Path
		typeName string
	}{
		{
			name: "truthy path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return dot(root, "ready")
			},
			want: branchcond.CheckTruthy,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("ready")
			},
		},
		{
			name: "falsy not path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.UnaryNotOpExpr{Expr: stringIndex(root, "missing")}
			},
			want: branchcond.CheckFalsy,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").IndexStr("missing")
			},
		},
		{
			name: "nil equal path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "child"), Rhs: &ast.NilExpr{}}
			},
			want: branchcond.CheckNil,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("child")
			},
		},
		{
			name: "nil not equal reversed path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: &ast.NilExpr{}, Rhs: intIndex(root, "1")}
			},
			want: branchcond.CheckNotNil,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").IndexInt(1)
			},
		},
		{
			name: "type equal path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(dot(root, "kind")), Rhs: stringLit("table")}
			},
			want: branchcond.CheckTypeEqual,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("kind")
			},
			typeName: "table",
		},
		{
			name: "type not equal reversed path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: stringLit("number"), Rhs: typeCall(stringIndex(root, "value"))}
			},
			want: branchcond.CheckTypeNot,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").IndexStr("value")
			},
			typeName: "number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := localAssign([]string{"obj"}, &ast.TableExpr{})
			root := ident("obj")
			cond := tt.expr(root)
			stmt := &ast.IfStmt{Condition: cond}
			stmts := []ast.Stmt{decl, stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
			built := cfgbuild.BuildChunk(stmts, bindings)
			if built == nil {
				t.Fatalf("BuildChunk returned nil")
			}

			result, err := ExtractChunk(stmts, bindings, built)
			if err != nil {
				t.Fatalf("ExtractChunk: %v", err)
			}

			point := requireStmtPoints(t, built, stmt, 1)[0]
			fact, ok := result.BranchCondition(point)
			if !ok {
				t.Fatalf("missing branch condition fact")
			}
			if fact.Kind != BranchIf || fact.Condition != cond {
				t.Fatalf("branch identity = %#v", fact)
			}
			check := fact.Check
			if check.Kind != tt.want {
				t.Fatalf("check kind = %v, want %v", check.Kind, tt.want)
			}
			if check.TypeName != tt.typeName {
				t.Fatalf("type name = %q, want %q", check.TypeName, tt.typeName)
			}
			wantPath := tt.wantPath(mustIdentSymbol(t, bindings, root))
			if !check.Path.Equal(wantPath) {
				t.Fatalf("check path = %#v, want %#v", check.Path, wantPath)
			}
		})
	}
}

func TestBranchConditionCheckPathIsCopied(t *testing.T) {
	decl := localAssign([]string{"obj"}, &ast.TableExpr{})
	root := ident("obj")
	cond := dot(root, "ready")
	stmt := &ast.IfStmt{Condition: cond}
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, stmt, 1)[0]
	fact, ok := result.BranchCondition(point)
	if !ok {
		t.Fatalf("missing branch condition fact")
	}
	if len(fact.Check.Path.Segments) != 1 {
		t.Fatalf("path segments = %#v, want one segment", fact.Check.Path.Segments)
	}
	fact.Check.Path.Segments[0].Name = "mutated"

	again, _ := result.BranchCondition(point)
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, root), "obj").Field("ready")
	if !again.Check.Path.Equal(wantPath) {
		t.Fatalf("BranchCondition exposed mutable path segments: %#v", again.Check.Path)
	}
}

func TestBranchConditionCheckOtherPathIsCopied(t *testing.T) {
	decl := localAssign([]string{"obj", "other"}, &ast.TableExpr{}, &ast.TableExpr{})
	root := ident("obj")
	other := ident("other")
	cond := &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "ready"), Rhs: dot(other, "ready")}
	stmt := &ast.IfStmt{Condition: cond}
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, stmt, 1)[0]
	fact, ok := result.BranchCondition(point)
	if !ok {
		t.Fatalf("missing branch condition fact")
	}
	if len(fact.Check.OtherPath.Segments) != 1 {
		t.Fatalf("other path segments = %#v, want one segment", fact.Check.OtherPath.Segments)
	}
	original := fact.Check.OtherPath.Clone()
	fact.Check.OtherPath.Segments[0].Name = "mutated"

	again, _ := result.BranchCondition(point)
	if !again.Check.OtherPath.Equal(original) {
		t.Fatalf("BranchCondition exposed mutable other path segments: %#v", again.Check.OtherPath)
	}
}

func TestExtractChunkNumericForFactsUseStmtPointsAndPreserveIdentity(t *testing.T) {
	init := number("1")
	limit := number("10")
	step := number("2")
	bodyLocal := localAssign([]string{"bodyValue"}, number("3"))
	loop := &ast.NumberForStmt{
		Name:  "i",
		Init:  init,
		Limit: limit,
		Step:  step,
		Stmts: []ast.Stmt{bodyLocal},
	}
	stmts := []ast.Stmt{loop}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	loopID, ok := bindings.NumForSymbol(loop)
	if !ok {
		t.Fatalf("missing numeric for symbol")
	}
	points := requireStmtPoints(t, built, loop, 2)
	expectedRoles := map[cfg.Point]cfgfacts.NumericForRole{
		points[0]: cfgfacts.NumericForRoleInit,
		points[1]: cfgfacts.NumericForRoleCheck,
	}
	for _, point := range points {
		fact, ok := result.NumericFor(point)
		if !ok {
			t.Fatalf("missing numeric for fact at point %d", point)
		}
		if fact.Role != expectedRoles[point] {
			t.Fatalf("numeric for role at point %d = %v, want %v", point, fact.Role, expectedRoles[point])
		}
		if fact.Stmt != loop || fact.Name != "i" || fact.Init != init || fact.Limit != limit || fact.Step != step {
			t.Fatalf("numeric for fact = %#v", fact)
		}
		if fact.Symbol != loopID || !fact.HasSymbol {
			t.Fatalf("numeric for symbol = %d/%v, want %d/true", fact.Symbol, fact.HasSymbol, loopID)
		}
	}

	bodyPoint := requireStmtPoints(t, built, bodyLocal, 1)[0]
	if _, ok := result.LocalAssignment(bodyPoint); !ok {
		t.Fatalf("missing numeric for body local assignment fact")
	}
}

func TestExtractChunkGenericForFactsUseStmtPointsAndPreserveIdentity(t *testing.T) {
	iter := ident("iter")
	state := ident("state")
	bodyLocal := localAssign([]string{"bodyValue"}, number("3"))
	loop := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{iter, state},
		Stmts: []ast.Stmt{bodyLocal},
	}
	stmts := []ast.Stmt{loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"iter", "state"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	kID := mustGenericForAt(t, bindings, loop, 0)
	vID := mustGenericForAt(t, bindings, loop, 1)
	points := requireStmtPoints(t, built, loop, 3)
	expectedRoles := map[cfg.Point]cfgfacts.GenericForRole{
		points[0]: cfgfacts.GenericForRoleCheck,
		points[1]: cfgfacts.GenericForRoleVariable,
		points[2]: cfgfacts.GenericForRoleVariable,
	}
	expectedVariableIndexes := map[cfg.Point]int{
		points[0]: cfgfacts.NoGenericForVariableIndex,
		points[1]: 0,
		points[2]: 1,
	}
	for _, point := range points {
		fact, ok := result.GenericFor(point)
		if !ok {
			t.Fatalf("missing generic for fact at point %d", point)
		}
		if fact.Role != expectedRoles[point] {
			t.Fatalf("generic for role at point %d = %v, want %v", point, fact.Role, expectedRoles[point])
		}
		if fact.VariableIndex != expectedVariableIndexes[point] {
			t.Fatalf("generic for variable index at point %d = %d, want %d", point, fact.VariableIndex, expectedVariableIndexes[point])
		}
		if fact.Stmt != loop {
			t.Fatalf("generic for stmt = %p, want %p", fact.Stmt, loop)
		}
		if len(fact.Names) != 2 || fact.Names[0] != "k" || fact.Names[1] != "v" {
			t.Fatalf("generic for names = %v", fact.Names)
		}
		if len(fact.Exprs) != 2 || fact.Exprs[0] != iter || fact.Exprs[1] != state {
			t.Fatalf("generic for exprs = %#v", fact.Exprs)
		}
		if len(fact.Symbols) != 2 || fact.Symbols[0] != kID || fact.Symbols[1] != vID || !fact.HasSymbols {
			t.Fatalf("generic for symbols = %v/%v, want %d,%d/true", fact.Symbols, fact.HasSymbols, kID, vID)
		}
	}

	firstFact, _ := result.GenericFor(points[0])
	firstFact.Names[0] = "mutated"
	firstFact.Exprs[0] = ident("mutated")
	firstFact.Symbols[0] = 0
	again, _ := result.GenericFor(points[0])
	if again.Names[0] != "k" || again.Exprs[0] != iter || again.Symbols[0] != kID {
		t.Fatalf("GenericFor exposed mutable slices")
	}

	bodyPoint := requireStmtPoints(t, built, bodyLocal, 1)[0]
	if _, ok := result.LocalAssignment(bodyPoint); !ok {
		t.Fatalf("missing generic for body local assignment fact")
	}
}

func TestExtractFunctionRecordsFunctionIdentity(t *testing.T) {
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{ident("a")}}
	fn := function([]string{"a"}, ret)
	bindings := bind.BindFunction(fn, bind.Options{})
	built := cfgbuild.BuildFunction(fn, bindings)

	result, err := ExtractFunction(fn, bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	if result.Function() != fn {
		t.Fatalf("function identity = %p, want %p", result.Function(), fn)
	}
	retPoint := requireStmtPoints(t, built, ret, 1)[0]
	if _, ok := result.Return(retPoint); !ok {
		t.Fatalf("missing function return fact")
	}
}

func TestExtractChunkSkipsUnmappedDeclarationFacts(t *testing.T) {
	ret := &ast.ReturnStmt{}
	deadFn := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: ident("f")},
		Func: function(nil),
	}
	deadType := &ast.TypeDefStmt{Name: "Alias", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	deadIface := &ast.InterfaceDefStmt{Name: "Shape"}
	stmts := []ast.Stmt{ret, deadFn, deadType, deadIface}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	if got := built.StmtPoints.PointsFor(deadFn); len(got) != 0 {
		t.Fatalf("dead function definition mapped to points %v", got)
	}
	if got := built.StmtPoints.PointsFor(deadType); len(got) != 0 {
		t.Fatalf("dead type definition mapped to points %v", got)
	}
	if got := built.StmtPoints.PointsFor(deadIface); len(got) != 0 {
		t.Fatalf("dead interface definition mapped to points %v", got)
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	deadPoint := cfg.Point(9999)
	if _, ok := result.FunctionDefinition(deadPoint); ok {
		t.Fatalf("unmapped function definition produced function fact at dead point")
	}
	if _, ok := result.TypeDefinition(deadPoint); ok {
		t.Fatalf("unmapped type definition produced type fact at dead point")
	}
}

func TestExtractChunkSkipsUnmappedLabel(t *testing.T) {
	ret := &ast.ReturnStmt{}
	deadLabel := &ast.LabelStmt{Name: "dead"}
	stmts := []ast.Stmt{ret, deadLabel}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	if got := built.StmtPoints.PointsFor(deadLabel); len(got) != 0 {
		t.Fatalf("dead label mapped to points %v", got)
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	if _, ok := result.Label(cfg.Point(9999)); ok {
		t.Fatalf("unmapped label produced label fact at dead point")
	}
}

func TestExtractChunkSkipsUnmappedGoto(t *testing.T) {
	ret := &ast.ReturnStmt{}
	deadGoto := &ast.GotoStmt{Label: "dead"}
	stmts := []ast.Stmt{ret, deadGoto}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	if got := built.StmtPoints.PointsFor(deadGoto); len(got) != 0 {
		t.Fatalf("dead goto mapped to points %v", got)
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	if _, ok := result.Goto(cfg.Point(9999)); ok {
		t.Fatalf("unmapped goto produced goto fact at dead point")
	}
}

func TestExtractReportsMissingCFG(t *testing.T) {
	if _, err := ExtractChunk(nil, nil, nil); !errors.Is(err, ErrNoCFG) {
		t.Fatalf("ExtractChunk(nil) = %v, want ErrNoCFG", err)
	}
	if _, err := ExtractFunction(nil, nil, &cfgbuild.Result{}); !errors.Is(err, ErrNoCFG) {
		t.Fatalf("ExtractFunction(empty) = %v, want ErrNoCFG", err)
	}
}

func TestExtractReportsPointMismatch(t *testing.T) {
	stmt := localAssign([]string{"a", "b"}, number("1"), number("2"))
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	built.StmtPoints = cfgbuild.StmtPoints{}

	_, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("unmapped statement should be skipped, got %v", err)
	}

	built = cfgbuild.BuildChunk(stmts, bindings)
	// The public cfgbuild API does not expose a mutator for partial mappings;
	// exercise the mismatch guard directly through the package-local extractor.
	result := newResult(nil)
	if err := result.extractLocalAssign(stmt, bindings, built.StmtPoints.PointsFor(stmt)[:1]); !errors.Is(err, ErrPointMismatch) {
		t.Fatalf("extractLocalAssign mismatch = %v, want ErrPointMismatch", err)
	}
}

func TestExtractSinglePointMetadataReportsExtraPointMismatch(t *testing.T) {
	tests := []struct {
		name string
		err  func() error
	}{
		{
			name: "function definition",
			err: func() error {
				stmt := &ast.FuncDefStmt{
					Name: &ast.FuncName{Func: ident("f")},
					Func: function(nil),
				}
				return newResult(nil).extractFunctionDefinition(stmt, nil, []cfg.Point{1, 2})
			},
		},
		{
			name: "label",
			err: func() error {
				stmt := &ast.LabelStmt{Name: "again"}
				return newResult(nil).extractLabel(stmt, []cfg.Point{1, 2})
			},
		},
		{
			name: "goto",
			err: func() error {
				stmt := &ast.GotoStmt{Label: "again"}
				return newResult(nil).extractGoto(stmt, []cfg.Point{1, 2})
			},
		},
		{
			name: "interface definition",
			err: func() error {
				stmt := &ast.InterfaceDefStmt{Name: "Shape"}
				return newResult(nil).extractInterfaceDef(stmt, []cfg.Point{1, 2})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.err(); !errors.Is(err, ErrPointMismatch) {
				t.Fatalf("extra-point mismatch = %v, want ErrPointMismatch", err)
			}
		})
	}
}
