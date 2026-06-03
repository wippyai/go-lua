package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalChannelDiscriminantNarrowsNestedValueAtReadPoint(t *testing.T) {
	src := `
type ChanInt = {__tag: "int"}
type ChanStr = {__tag: "str"}
type SelResult =
    {channel: ChanInt, value: {error: string}, ok: boolean} |
    {channel: ChanStr, value: {data: number}, ok: boolean}

local function get_result(a: ChanInt, b: ChanStr): SelResult
    return {channel = a, value = {error = "oops"}, ok = true}
end

local function f(ch1: ChanInt, ch2: ChanStr)
    local result = get_result(ch1, ch2)
    if result.channel == ch1 then
        local e: string = result.value.error
    end
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithParamNames(t, res.Session.Results, "ch1", "ch2")
	resultSym := singleSymbolNamed(t, fn.Graph, "result")
	ch1Sym := singleSymbolNamed(t, fn.Graph, "ch1")
	readPoint := assignTargetPoint(t, fn.Graph, "e")
	path := constraint.NewPath(resultSym, "result").Field("value").Field("error")

	got := fn.NarrowedTypeAt(readPoint, path)
	if !typ.TypeEquals(got, typ.String) {
		root := fn.NarrowedTypeAt(readPoint, constraint.NewPath(resultSym, "result"))
		value := fn.NarrowedTypeAt(readPoint, constraint.NewPath(resultSym, "result").Field("value"))
		ch1 := fn.NarrowedTypeAt(readPoint, constraint.NewPath(ch1Sym, "ch1"))
		t.Fatalf("NarrowedTypeAt(result.value.error at guarded read) = %v, want string; root=%v value=%v ch1=%v diagnostics=%v", got, root, value, ch1, testutil.ErrorMessages(res.Diagnostics))
	}
}

func findFunctionWithParamNames(t *testing.T, results map[*ast.FunctionExpr]*api.FuncResult, names ...string) *api.FuncResult {
	t.Helper()
	for _, result := range results {
		if result == nil || result.Graph == nil {
			continue
		}
		params := result.Graph.ParamSymbols()
		if len(params) != len(names) {
			continue
		}
		matches := true
		for i, sym := range params {
			if result.Graph.NameOf(sym) != names[i] {
				matches = false
				break
			}
		}
		if matches {
			return result
		}
	}
	t.Fatalf("no function with params %v", names)
	return nil
}

func singleSymbolNamed(t *testing.T, g *cfg.Graph, name string) cfg.SymbolID {
	t.Helper()
	symbols := g.Bindings().SymbolsByName(name)
	if len(symbols) != 1 {
		t.Fatalf("symbols named %q = %v, want exactly one", name, symbols)
	}
	return symbols[0]
}

func assignTargetPoint(t *testing.T, g *cfg.Graph, name string) cfg.Point {
	t.Helper()
	var point cfg.Point
	g.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if point != 0 || info == nil {
			return
		}
		for _, target := range info.Targets {
			if target.Name == name {
				point = p
				return
			}
		}
	})
	if point == 0 {
		t.Fatalf("no assignment target named %q", name)
	}
	return point
}
