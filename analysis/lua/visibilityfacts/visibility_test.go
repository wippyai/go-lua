package visibilityfacts

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestResolverDefinesRootAssignmentsAndChannelSelectLocals(t *testing.T) {
	stmts, bindings := parseBoundChunk(t, `
local root
local result
local selectedCase
`)
	root := mustLocalSymbolAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	result := mustLocalSymbolAt(t, bindings, stmts[1].(*ast.LocalAssignStmt), 0)
	selectedCase := mustLocalSymbolAt(t, bindings, stmts[2].(*ast.LocalAssignStmt), 0)

	graph, points := linearGraph(cfg.NodeAssign, cfg.NodeAssign)
	assignPoint := points[0]
	selectPoint := points[1]
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignPoint: factflow.NewRootAssignment(
				factflow.RootAssignmentLocalDeclaration,
				root,
				pathdom.NewPath(root, "root"),
				factflow.NewUnknownValueSource(0),
			),
		},
		ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
			selectPoint: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				Kind:          factflow.ChannelSelectCase,
				ResultPath:    pathdom.NewPath(result, "result"),
				HasResultPath: true,
				CasePath:      pathdom.NewPath(selectedCase, "selectedCase"),
				HasCasePath:   true,
			})),
		},
	})

	resolver := Resolver(bindings, graph, facts)
	if got := resolver.KeyAt(assignPoint, pathdom.NewPath(root, "root")); got == "" {
		t.Fatalf("root assignment has no visible key at point %d", assignPoint)
	}
	if got := resolver.KeyAt(selectPoint, pathdom.NewPath(result, "result")); got == "" {
		t.Fatalf("channel select result has no visible key at point %d", selectPoint)
	}
	if got := resolver.KeyAt(selectPoint, pathdom.NewPath(selectedCase, "selectedCase")); got == "" {
		t.Fatalf("channel select case has no visible key at point %d", selectPoint)
	}
}

func TestResolverSeedsEntryForPathUsingLocalWithoutAssignment(t *testing.T) {
	stmts, bindings := parseBoundChunk(t, `local t`)
	tableSym := mustLocalSymbolAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	tablePath := pathdom.NewPath(tableSym, "t").Field("field")
	graph, _ := linearGraph(cfg.NodeAssign)
	facts := factflow.NewFacts(factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			1: tablePath,
		},
	})

	resolver := Resolver(bindings, graph, facts)
	if got := resolver.KeyAt(graph.Entry(), tablePath); got == "" {
		t.Fatalf("path-using local has no entry visibility")
	}
}

func TestResolverSeedsEntryForRootCallArgumentPath(t *testing.T) {
	stmts, bindings := parseBoundChunk(t, `local payload`)
	payloadSym := mustLocalSymbolAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	payloadPath := pathdom.NewPath(payloadSym, "payload")
	graph, points := linearGraph(cfg.NodeCall)
	callPoint := points[0]
	argRef := factflow.ExprRef(17)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			callPoint: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argRef, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			argRef: payloadPath,
		},
	})

	resolver := Resolver(bindings, graph, facts)
	if got := resolver.KeyAt(callPoint, payloadPath); got == "" {
		t.Fatalf("root call argument path has no visibility at call point")
	}
}

func TestResolverSeedsEntryForAssignedParameterPath(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"bindings"}}}
	stmts := []ast.Stmt{&ast.LocalAssignStmt{Names: []string{"f"}, Exprs: []ast.Expr{fn}}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	params := bindings.ParamSymbols(fn)
	if len(params) != 1 || params[0] == 0 {
		t.Fatalf("ParamSymbols = %#v, want one bindings parameter", params)
	}
	bindingsSym := params[0]
	bindingsPath := pathdom.NewPath(bindingsSym, "bindings")
	checkpointPath := bindingsPath.Field("checkpoint")
	graph, points := linearGraph(cfg.NodeAssign)
	assignPoint := points[0]
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignPoint: factflow.NewRootAssignment(
				factflow.RootAssignmentOrdinaryRootWrite,
				bindingsSym,
				bindingsPath,
				factflow.NewUnknownValueSource(0),
			),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			1: checkpointPath,
		},
	})

	resolver := Resolver(bindings, graph, facts)
	entryKey := resolver.KeyAt(graph.Entry(), checkpointPath)
	if entryKey == "" {
		t.Fatalf("assigned parameter member path has no entry visibility")
	}
	assignKey := resolver.KeyAt(assignPoint, checkpointPath)
	if assignKey == "" {
		t.Fatalf("assigned parameter member path has no assignment visibility")
	}
	if assignKey == entryKey {
		t.Fatalf("assignment key = entry key %q, want reassignment to create a new parameter version", assignKey)
	}
	assignInputKey := resolver.Before().KeyAt(assignPoint, checkpointPath)
	if assignInputKey != entryKey {
		t.Fatalf("assignment input key = %q, want original entry key %q", assignInputKey, entryKey)
	}
}

func TestResolverSeedsEntryForAssignedGlobalPath(t *testing.T) {
	_, bindings := parseBoundChunk(t, `g = 1`, "g")
	globalSym, ok := bindings.GlobalSymbol("g")
	if !ok {
		t.Fatalf("missing global symbol g")
	}
	globalPath := pathdom.NewPath(globalSym, "g").Field("field")
	graph, points := linearGraph(cfg.NodeAssign)
	assignPoint := points[0]
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignPoint: factflow.NewRootAssignment(
				factflow.RootAssignmentOrdinaryRootWrite,
				globalSym,
				pathdom.NewPath(globalSym, "g"),
				factflow.NewUnknownValueSource(0),
			),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			1: globalPath,
		},
	})

	resolver := Resolver(bindings, graph, facts)
	entryKey := resolver.KeyAt(graph.Entry(), globalPath)
	if entryKey == "" {
		t.Fatalf("assigned global path has no entry visibility")
	}
	assignKey := resolver.KeyAt(assignPoint, globalPath)
	if assignKey == "" {
		t.Fatalf("assigned global path has no assignment visibility")
	}
	if assignKey == entryKey {
		t.Fatalf("assignment key = entry key %q, want a new global version at assignment", assignKey)
	}
}

func parseBoundChunk(t *testing.T, src string, globals ...string) ([]ast.Stmt, *bind.Result) {
	t.Helper()
	stmts, err := parse.ParseString(src, "visibilityfacts_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return stmts, bind.BindChunk(stmts, bind.Options{Globals: globals})
}

func linearGraph(kinds ...cfg.NodeKind) (*cfg.CFG, []cfg.Point) {
	graph := cfg.New()
	points := make([]cfg.Point, len(kinds))
	prev := graph.Entry()
	for i, kind := range kinds {
		point := graph.AddNode(kind)
		points[i] = point
		graph.AddEdge(prev, point, false)
		prev = point
	}
	graph.AddEdge(prev, graph.Exit(), false)
	return graph, points
}

func mustLocalSymbolAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	sym, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok || sym == 0 {
		t.Fatalf("missing local symbol at index %d", index)
	}
	return sym
}
