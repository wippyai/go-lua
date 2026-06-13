package semantics

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
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
	if got.Source.Kind != factflow.ValueSourceExpression || got.Source.ExprIndex != factflow.NoValueSourceIndex || got.Source.TargetIndex != factflow.NoValueSourceIndex {
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
	stmts := []ast.Stmt{local, dotWrite, indexWrite, dynamicWrite}
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

func TestExtractChunkObjectLiteralThroughAssertionWrappers(t *testing.T) {
	asTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("a"), KeySyntax: ast.AttrKeyDot, Value: number("1")},
	}}
	asCast := &ast.CastExpr{
		Expr:   asTable,
		Type:   &ast.PrimitiveTypeExpr{Name: "any"},
		Syntax: ast.CastSyntaxAs,
	}
	colonTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("b"), KeySyntax: ast.AttrKeyDot, Value: number("2")},
	}}
	colonCast := &ast.CastExpr{
		Expr:   colonTable,
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
		Type:   &ast.PrimitiveTypeExpr{Name: "any"},
		Syntax: ast.CastSyntaxAs,
	}
	rootTable := &ast.TableExpr{Fields: []*ast.Field{
		{Key: stringLit("nested"), KeySyntax: ast.AttrKeyDot, Value: nestedCast},
	}}
	local := localAssign([]string{"a", "b", "c", "d"}, asCast, colonCast, nonNil, rootTable)
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
	if entries[0].Source.Kind != factflow.ValueSourceVararg || entries[0].Source.Final || !entries[0].Source.Adjusted || entries[0].Source.Expanded {
		t.Fatalf("non-final vararg source = %#v, want adjusted single value", entries[0].Source)
	}
	if entries[1].Value != keyedVararg || !entries[1].Suffix.Equal(fieldChainSuffix("key")) {
		t.Fatalf("keyed vararg entry = %#v", entries[1])
	}
	if entries[1].Source.Kind != factflow.ValueSourceVararg || entries[1].Source.Final || !entries[1].Source.Adjusted || entries[1].Source.Expanded {
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
	if _, ok := result.OrdinaryAssignment(points[0]); ok {
		t.Fatalf("function definition point produced ordinary assignment fact")
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
	if aFact.Source.Kind != factflow.ValueSourceCall || aFact.Source.Expr != makeCall || aFact.Source.ExprIndex != 0 || aFact.Source.ResultIndex != 0 || !aFact.Source.Adjusted || aFact.Source.CallPoint != localPoints[0] || !aFact.Source.HasCallPoint {
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
	if bFact.Source.Kind != factflow.ValueSourceCall || bFact.Source.Expr != packCall || !bFact.Source.Expanded || bFact.Source.ResultIndex != 0 || bFact.Source.CallPoint != localPoints[1] || !bFact.Source.HasCallPoint {
		t.Fatalf("b source = %#v", bFact.Source)
	}
	if cFact.Source.Kind != factflow.ValueSourceCall || cFact.Source.Expr != packCall || !cFact.Source.Expanded || cFact.Source.ResultIndex != 1 || cFact.Source.CallPoint != localPoints[1] || !cFact.Source.HasCallPoint {
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
	if len(returnFact.Sources) != 2 || returnFact.Sources[0].Kind != factflow.ValueSourceExpression || returnFact.Sources[0].Expr != aRead {
		t.Fatalf("return first source = %#v", returnFact.Sources)
	}
	if returnFact.Sources[1].Kind != factflow.ValueSourceCall || returnFact.Sources[1].Expr != tailCall || !returnFact.Sources[1].Expanded || !returnFact.Sources[1].OpenTail || returnFact.Sources[1].CallPoint != returnPoints[0] || !returnFact.Sources[1].HasCallPoint {
		t.Fatalf("return tail source = %#v", returnFact.Sources[1])
	}
	returnFact.Sources[1].Kind = factflow.ValueSourceNil
	returnAgain, _ := result.Return(returnPoints[1])
	if returnAgain.Sources[1].Kind != factflow.ValueSourceCall {
		t.Fatalf("Return exposed mutable sources slice")
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
	if branchFact.Source.Kind != factflow.ValueSourceCall || branchFact.Source.Expr != readyCall || branchFact.Source.CallPoint != ifPoints[0] || !branchFact.Source.HasCallPoint {
		t.Fatalf("condition source = %#v", branchFact.Source)
	}
	if branchFact.Source.TargetIndex != factflow.NoValueSourceIndex || !branchFact.Source.Adjusted || branchFact.Source.Expanded {
		t.Fatalf("condition source flags = %#v", branchFact.Source)
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
	if genericFact.Sources[0].Kind != factflow.ValueSourceCall || genericFact.Sources[0].Expr != iterCall || genericFact.Sources[0].CallPoint != loopPoints[0] || !genericFact.Sources[0].HasCallPoint || !genericFact.Sources[0].Adjusted {
		t.Fatalf("first generic source = %#v", genericFact.Sources[0])
	}
	if genericFact.Sources[1].Kind != factflow.ValueSourceCall || genericFact.Sources[1].Expr != stateCall || genericFact.Sources[1].CallPoint != loopPoints[1] || !genericFact.Sources[1].HasCallPoint || !genericFact.Sources[1].Expanded || genericFact.Sources[1].OpenTail {
		t.Fatalf("final generic source = %#v", genericFact.Sources[1])
	}
	genericFact.Sources[0].Kind = factflow.ValueSourceNil
	genericAgain, _ := result.GenericFor(loopPoints[2])
	if genericAgain.Sources[0].Kind != factflow.ValueSourceCall {
		t.Fatalf("GenericFor exposed mutable sources slice")
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
	if !ok || len(returnFact.Sources) != 1 || returnFact.Sources[0].Kind != factflow.ValueSourceCall || returnFact.Sources[0].Expr != barCast || returnFact.Sources[0].CallPoint != returnPoints[0] || !returnFact.Sources[0].HasCallPoint {
		t.Fatalf("return sources = %#v, ok=%v", returnFact.Sources, ok)
	}

	ifPoints := requireStmtPoints(t, built, ifStmt, 2)
	conditionCall, ok := result.Call(ifPoints[0])
	if !ok || conditionCall.Call != readyCall || conditionCall.Context != CallContextCondition {
		t.Fatalf("condition call = %#v, ok=%v", conditionCall, ok)
	}
	branchFact, ok := result.BranchCondition(ifPoints[1])
	if !ok || branchFact.Source.Kind != factflow.ValueSourceCall || branchFact.Source.Expr != readyCast || branchFact.Source.CallPoint != ifPoints[0] || !branchFact.Source.HasCallPoint {
		t.Fatalf("branch source = %#v, ok=%v", branchFact.Source, ok)
	}

	loopPoints := requireStmtPoints(t, built, loop, 3)
	iterCallFact, ok := result.Call(loopPoints[0])
	if !ok || iterCallFact.Call != iterCall || iterCallFact.Context != CallContextIteratorSource {
		t.Fatalf("iterator call = %#v, ok=%v", iterCallFact, ok)
	}
	genericFact, ok := result.GenericFor(loopPoints[1])
	if !ok || len(genericFact.Sources) != 1 || genericFact.Sources[0].Kind != factflow.ValueSourceCall || genericFact.Sources[0].Expr != iterCast || genericFact.Sources[0].CallPoint != loopPoints[0] || !genericFact.Sources[0].HasCallPoint {
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
	if !ok || assignFact.Source.Kind != factflow.ValueSourceCall || assignFact.Source.Expr != outerExpr || assignFact.Source.CallPoint != callPoint || !assignFact.Source.HasCallPoint {
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
	if first.Source.Kind != factflow.ValueSourceCall || first.Source.Expr != singleCall || !first.Source.Final || !first.Source.Adjusted || first.Source.Expanded || first.Source.CallPoint != adjustedPoints[0] || !first.Source.HasCallPoint {
		t.Fatalf("first adjusted source = %#v", first.Source)
	}
	if second.Source.Kind != factflow.ValueSourceNil || second.Source.ExprIndex != factflow.NoValueSourceIndex {
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
	if qFact.Source.Kind != factflow.ValueSourceVararg || qFact.Source.Expr != vararg || !qFact.Source.Expanded || qFact.Source.ResultIndex != 0 {
		t.Fatalf("q source = %#v", qFact.Source)
	}
	if rFact.Source.Kind != factflow.ValueSourceVararg || rFact.Source.Expr != vararg || !rFact.Source.Expanded || rFact.Source.ResultIndex != 1 {
		t.Fatalf("r source = %#v", rFact.Source)
	}

	returnPoint := requireStmtPoints(t, built, varargReturn, 1)[0]
	returnFact, ok := result.Return(returnPoint)
	if !ok {
		t.Fatalf("missing vararg return fact")
	}
	if len(returnFact.Sources) != 2 || returnFact.Sources[1].Kind != factflow.ValueSourceVararg || !returnFact.Sources[1].Expanded || !returnFact.Sources[1].OpenTail {
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
	if len(fact.ArgumentSources) != 1 || fact.ArgumentSources[0].Kind != factflow.ValueSourceExpression || fact.ArgumentSources[0].Expr != arg || fact.ArgumentSources[0].ExprIndex != 0 || fact.ArgumentSources[0].TargetIndex != 0 || fact.ArgumentSources[0].ResultIndex != 0 || !fact.ArgumentSources[0].Final {
		t.Fatalf("method argument sources = %#v", fact.ArgumentSources)
	}
	fact.ArgumentSources[0].Kind = factflow.ValueSourceNil
	fact.MethodPath.Segments[0].Name = "mutated"
	again, _ := result.Call(point)
	if !again.MethodPath.Equal(methodPath) {
		t.Fatalf("Call exposed mutable method path: %#v", again.MethodPath)
	}
	if again.ArgumentSources[0].Kind != factflow.ValueSourceExpression {
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
	callFact := buildCallFact(local, nil, CallContextAssignmentSource, local.Exprs, 0, selectCall, bindings, targets)
	secondFact := callFact
	secondFact.ChannelSelect.ResultTarget.Path = path.NewPath(symbol.ID(9999), "second")
	result.calls[2] = secondFact
	result.calls[1] = callFact
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

	originalCasePath := copyPath(selectFact.Cases[0].ChannelPath)
	selects[0].Cases[0].ChannelPath.Segments = append(selects[0].Cases[0].ChannelPath.Segments, segment.Segment{Kind: segment.SegmentField, Name: "mutated"})
	again := result.ChannelSelects()
	if !again[0].Cases[0].ChannelPath.Equal(originalCasePath) {
		t.Fatalf("ChannelSelects exposed mutable channel path")
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
			callFact := buildCallFact(local, nil, CallContextAssignmentSource, local.Exprs, 0, selectCall, bindings, targets)
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
	original := copyPath(fact.Check.OtherPath)
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
