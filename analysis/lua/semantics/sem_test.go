package semantics

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
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

func TestExtractChunkCallArgumentSourcesForTableArgument(t *testing.T) {
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

func TestExtractFunctionReturnCallArgumentSourceForTableArgument(t *testing.T) {
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
	fact, ok := built.Declarations.FunctionDefinition(points[0])
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
			fact, ok := built.Declarations.FunctionDefinition(points[0])
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

	typePoint := requireStmtPoints(t, built, typeDef, 1)[0]
	if node := built.Graph.Node(typePoint); node == nil || node.Kind != cfg.NodeNoop {
		t.Fatalf("type def cfg node = %#v, want NodeNoop", node)
	}
	typeFact, ok := built.Declarations.TypeDefinition(typePoint)
	if !ok || typeFact.Kind != cfgbuild.TypeDefinitionAlias || typeFact.Type != typeDef {
		t.Fatalf("type def fact = %#v, ok=%v", typeFact, ok)
	}
	interfacePoint := requireStmtPoints(t, built, interfaceDef, 1)[0]
	if node := built.Graph.Node(interfacePoint); node == nil || node.Kind != cfg.NodeNoop {
		t.Fatalf("interface def cfg node = %#v, want NodeNoop", node)
	}
	interfaceFact, ok := built.Declarations.TypeDefinition(interfacePoint)
	if !ok || interfaceFact.Kind != cfgbuild.TypeDefinitionInterface || interfaceFact.Interface != interfaceDef {
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

func TestExtractChunkNestedTableCallUsesExpressionProducerContext(t *testing.T) {
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

	genericFact, ok := built.Meta.GenericFor(loopPoints[2])
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
	genericAgain, _ := built.Meta.GenericFor(loopPoints[2])
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
	loopPoints := requireStmtPoints(t, built, loop, 3)
	iterCallFact, ok := result.Call(loopPoints[0])
	if !ok || iterCallFact.Call != iterCall || iterCallFact.Context != CallContextIteratorSource {
		t.Fatalf("iterator call = %#v, ok=%v", iterCallFact, ok)
	}
	genericFact, ok := built.Meta.GenericFor(loopPoints[1])
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

func TestCallFactIsDirectGlobalUsesBindings(t *testing.T) {
	stmts, err := parse.ParseString(`
assert(x)
local assert = function(v) return v end
assert(x)
obj:assert(x)
`, "call_global_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert", "x", "obj"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	globalPoint := requireStmtPoints(t, built, stmts[0], 1)[0]
	globalFact, ok := result.Call(globalPoint)
	if !ok {
		t.Fatalf("missing global call fact")
	}
	if !globalFact.IsDirectGlobal(bindings, "assert") {
		t.Fatalf("global assert call was not recognized")
	}
	if globalFact.IsDirectGlobal(bindings, "pcall") {
		t.Fatalf("global assert call recognized as pcall")
	}

	shadowPoint := requireStmtPoints(t, built, stmts[2], 1)[0]
	shadowFact, ok := result.Call(shadowPoint)
	if !ok {
		t.Fatalf("missing shadowed call fact")
	}
	if shadowFact.IsDirectGlobal(bindings, "assert") {
		t.Fatalf("shadowed local assert recognized as global")
	}

	methodPoint := requireStmtPoints(t, built, stmts[3], 1)[0]
	methodFact, ok := result.Call(methodPoint)
	if !ok {
		t.Fatalf("missing method call fact")
	}
	if methodFact.IsDirectGlobal(bindings, "assert") {
		t.Fatalf("method call recognized as direct global")
	}
}

func TestCallFactGlobalCallPredicates(t *testing.T) {
	stmts, err := parse.ParseString(`
assert(x)
local value = assert(x)
local ok, result = pcall(run)
local xok, xresult = xpcall(run, handler)
local pcall = function(fn) return true, fn() end
local shadow_ok, shadow_result = pcall(run)
`, "call_predicates_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert", "x", "pcall", "xpcall", "run", "handler"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	callFactForStmt := func(stmt ast.Stmt) CallFact {
		t.Helper()
		for _, point := range built.StmtPoints.PointsFor(stmt) {
			if fact, ok := result.Call(point); ok {
				return fact
			}
		}
		t.Fatalf("missing call fact for %T", stmt)
		return CallFact{}
	}

	assertFact := callFactForStmt(stmts[0])
	if !assertFact.IsDirectGlobalStatement(bindings, "assert") {
		t.Fatalf("statement assert was not recognized")
	}

	assignAssertFact := callFactForStmt(stmts[1])
	if assignAssertFact.IsDirectGlobalStatement(bindings, "assert") {
		t.Fatalf("assignment assert recognized as statement assert")
	}

	pcallFact := callFactForStmt(stmts[2])
	if !pcallFact.IsProtectedCall(bindings) {
		t.Fatalf("global pcall was not recognized as protected call")
	}

	xpcallFact := callFactForStmt(stmts[3])
	if !xpcallFact.IsProtectedCall(bindings) {
		t.Fatalf("global xpcall was not recognized as protected call")
	}

	shadowFact := callFactForStmt(stmts[5])
	if shadowFact.IsProtectedCall(bindings) {
		t.Fatalf("shadowed local pcall recognized as protected call")
	}
}

func TestCallFactResultTargetPathReturnsDefensiveCopy(t *testing.T) {
	stmts, err := parse.ParseString(`
local a, b = make()
`, "call_target_path_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	var fact CallFact
	for _, point := range built.StmtPoints.PointsFor(stmts[0]) {
		if got, ok := result.Call(point); ok {
			fact = got
			break
		}
	}
	if fact.Call == nil {
		t.Fatal("missing call fact")
	}
	want := path.NewPath(mustLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 1), "b")
	got, ok := fact.ResultTargetPath(1)
	if !ok || !got.Equal(want) {
		t.Fatalf("ResultTargetPath(1) = %#v/%v, want %#v/true", got, ok, want)
	}
	got.Segments = append(got.Segments, segment.Segment{Kind: segment.SegmentField, Name: "mutated"})
	again, ok := fact.ResultTargetPath(1)
	if !ok || !again.Equal(want) {
		t.Fatalf("ResultTargetPath exposed mutable path: %#v/%v, want %#v/true", again, ok, want)
	}
	if missing, ok := fact.ResultTargetPath(3); ok || !missing.IsEmpty() {
		t.Fatalf("ResultTargetPath(3) = %#v/%v, want empty/false", missing, ok)
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
	expectedRoles := map[cfg.Point]cfgbuild.NumericForRole{
		points[0]: cfgbuild.NumericForRoleInit,
		points[1]: cfgbuild.NumericForRoleCheck,
	}
	for _, point := range points {
		fact, ok := built.NumericFors.Get(point)
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
		fact, ok := built.Meta.GenericFor(point)
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

	firstFact, _ := built.Meta.GenericFor(points[0])
	firstFact.Names[0] = "mutated"
	firstFact.Exprs[0] = ident("mutated")
	firstFact.Symbols[0] = 0
	again, _ := built.Meta.GenericFor(points[0])
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

	_, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	deadPoint := cfg.Point(9999)
	if _, ok := built.Declarations.FunctionDefinition(deadPoint); ok {
		t.Fatalf("unmapped function definition produced function fact at dead point")
	}
	if _, ok := built.Declarations.TypeDefinition(deadPoint); ok {
		t.Fatalf("unmapped type definition produced type fact at dead point")
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

func TestExtractFunctionDefinitionAssignmentReportsExtraPointMismatch(t *testing.T) {
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
				return newResult(nil).extractFunctionDefinitionAssignment(stmt, nil, []cfg.Point{1, 2})
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
