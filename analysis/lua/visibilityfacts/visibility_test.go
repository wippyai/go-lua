package visibilityfacts

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestDefinitionsFromWIRDefinesRootAssignmentsChannelSelectsAndEntrySeeds(t *testing.T) {
	src := `
local payload = { checkpoint = 1 }
local result = nil
local selected = nil
local chan = payload.channel
result, selected = channel.select({ chan })
payload = { checkpoint = result }
sink(payload.checkpoint, selected)
`
	stmts, bindings := parseBoundChunk(t, src, "channel", "sink")
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	body := wirlower.Lower("visibility-parity", stmts, bindings, built)
	wirResolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{
		Graph:       built.Graph,
		Definitions: DefinitionsFromWIR(bindings, built.Graph, body),
	}))
	payload := mustLocalSymbolAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	result := mustLocalSymbolAt(t, bindings, stmts[1].(*ast.LocalAssignStmt), 0)
	selected := mustLocalSymbolAt(t, bindings, stmts[2].(*ast.LocalAssignStmt), 0)
	chanSym := mustLocalSymbolAt(t, bindings, stmts[3].(*ast.LocalAssignStmt), 0)

	paths := []pathdom.Path{
		pathdom.NewPath(payload, "payload").Field("checkpoint"),
		pathdom.NewPath(result, "result"),
		pathdom.NewPath(selected, "selected"),
		pathdom.NewPath(chanSym, "chan"),
	}

	for _, p := range paths {
		if point, got := firstVisibleKey(built.Graph, wirResolver, p); got == "" {
			t.Fatalf("DefinitionsFromWIR missing visibility for %s (first point %d)", p.String(), point)
		}
	}
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op != wir.OpSelect {
				continue
			}
			if got := wirResolver.KeyAt(point, pathdom.NewPath(result, "result")); got == "" {
				t.Fatalf("DefinitionsFromWIR missing select result visibility at point %d", point)
			}
			if got := wirResolver.KeyAt(point, pathdom.NewPath(chanSym, "chan")); got == "" {
				t.Fatalf("DefinitionsFromWIR missing select case visibility at point %d", point)
			}
			return
		}
	}
	t.Fatalf("lowered WIR did not contain select instruction")
}

func TestDefinitionsFromWIRSeedsEntryAndReassignmentVersions(t *testing.T) {
	src := `
local function f(bindings)
    local before = bindings.checkpoint
    bindings = {}
    local after = bindings.checkpoint
end
`
	stmts, bindings := parseBoundChunk(t, src)
	fn := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildFunction returned nil")
	}
	body := wirlower.LowerFunction("f", fn, bindings, built)
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{
		Graph:       built.Graph,
		Definitions: DefinitionsFromWIR(bindings, built.Graph, body),
	}))
	params := bindings.ParamSymbols(fn)
	if len(params) != 1 || params[0] == 0 {
		t.Fatalf("ParamSymbols = %#v, want bindings param", params)
	}
	checkpointPath := pathdom.NewPath(params[0], "bindings").Field("checkpoint")

	var assignPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Assign != wir.AssignOrdinaryRootWrite {
				continue
			}
			if inst.Dst.Kind != wir.OperandPath {
				continue
			}
			p := body.Path(wir.PathRef(inst.Dst.Ref))
			if p.Symbol == params[0] && len(p.Segments) == 0 {
				assignPoint = point
				break
			}
		}
		if assignPoint != 0 {
			break
		}
	}
	if assignPoint == 0 {
		t.Fatalf("missing ordinary root write to bindings")
	}
	entryKey := resolver.KeyAt(built.Graph.Entry(), checkpointPath)
	if entryKey == "" {
		t.Fatalf("entry key for checkpoint path is empty")
	}
	assignKey := resolver.KeyAt(assignPoint, checkpointPath)
	if assignKey == "" {
		t.Fatalf("assignment key for checkpoint path is empty")
	}
	if assignKey == entryKey {
		t.Fatalf("assignment key = entry key %q, want reassignment version", assignKey)
	}
	beforeKey := resolver.Before().KeyAt(assignPoint, checkpointPath)
	if beforeKey != entryKey {
		t.Fatalf("assignment input key = %q, want entry key %q", beforeKey, entryKey)
	}
}

func TestDefinitionsFromWIRSeedsEntryAndGlobalReassignmentVersions(t *testing.T) {
	src := `
g = {}
local before = g.checkpoint
g = {}
local after = g.checkpoint
`
	stmts, bindings := parseBoundChunk(t, src, "g")
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	body := wirlower.Lower("global-visibility", stmts, bindings, built)
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{
		Graph:       built.Graph,
		Definitions: DefinitionsFromWIR(bindings, built.Graph, body),
	}))
	globalSym, ok := bindings.GlobalSymbol("g")
	if !ok || globalSym == 0 {
		t.Fatalf("missing global symbol g")
	}
	globalPath := pathdom.NewPath(globalSym, "g").Field("checkpoint")

	var assignPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Assign == wir.AssignOrdinaryRootWrite && inst.Dst.Kind == wir.OperandPath {
				p := body.Path(wir.PathRef(inst.Dst.Ref))
				if p.Symbol == globalSym && len(p.Segments) == 0 {
					assignPoint = point
				}
			}
			if global, ok := wirGlobalTableFieldRootSymbol(bindings, body, inst.Dst); ok && global == globalSym {
				assignPoint = point
			}
		}
	}
	if assignPoint == 0 {
		t.Fatalf("missing static-member root write for global g")
	}
	entryKey := resolver.KeyAt(built.Graph.Entry(), globalPath)
	if entryKey == "" {
		t.Fatalf("entry key for global path is empty")
	}
	assignKey := resolver.KeyAt(assignPoint, globalPath)
	if assignKey == "" {
		t.Fatalf("assignment key for global path is empty")
	}
	if assignKey == entryKey {
		t.Fatalf("assignment key = entry key %q, want reassignment version", assignKey)
	}
}

func TestDefinitionsFromWIRDefinesNumericForVariables(t *testing.T) {
	src := `
local function sum(xs: {number}): number
    local total: number = 0
    for i = 1, #xs do
        total = total + xs[i]
    end
    return total
end
`
	stmts, bindings := parseBoundChunk(t, src)
	fn := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	built := cfgbuild.BuildFunction(fn, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildFunction returned nil")
	}
	body := wirlower.LowerFunction("sum", fn, bindings, built)
	resolver := visibility.NewResolver(visibility.BuildForward(visibility.BuildConfig{
		Graph:       built.Graph,
		Definitions: DefinitionsFromWIR(bindings, built.Graph, body),
	}))
	params := bindings.ParamSymbols(fn)
	if len(params) != 1 || params[0] == 0 {
		t.Fatalf("ParamSymbols = %#v, want xs param", params)
	}
	arrayPath := pathdom.NewPath(params[0], "xs")

	var indexPath pathdom.Path
	var iteratePoint cfg.Point
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op != wir.OpIterate || inst.Iter != wir.IterNumeric {
				continue
			}
			for _, result := range body.Operands(inst.Results) {
				if result.Kind != wir.OperandPath {
					continue
				}
				p := body.Path(wir.PathRef(result.Ref))
				if p.Symbol != 0 && len(p.Segments) == 0 && p.Root == "i" {
					indexPath = p
					iteratePoint = point
					break
				}
			}
		}
	}
	if iteratePoint == 0 || indexPath.IsEmpty() {
		t.Fatalf("missing numeric-for index result")
	}
	if got := resolver.KeyAt(iteratePoint, indexPath); got == "" {
		t.Fatalf("numeric-for index has no visibility key at point %d", iteratePoint)
	}
	if got := resolver.KeyAt(built.Graph.Entry(), arrayPath); got == "" {
		t.Fatalf("numeric-for array parameter has no entry visibility key")
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

func mustLocalSymbolAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	sym, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok || sym == 0 {
		t.Fatalf("missing local symbol at index %d", index)
	}
	return sym
}

func firstVisibleKey(graph cfg.Graph, resolver *visibility.Resolver, p pathdom.Path) (cfg.Point, string) {
	for _, point := range graph.RPO() {
		if key := resolver.KeyAt(point, p); key != "" {
			return point, string(key)
		}
	}
	return 0, ""
}
