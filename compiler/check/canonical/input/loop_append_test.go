package input

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildLoopAppendLengthsNumericForTableInsert(t *testing.T) {
	in := loopAppendInputs(t, nil, `
local out = {}
for i = 1, 3 do
	table.insert(out, "x")
end
return out
`, "table")

	if len(in.LoopAppendLengths) != 1 {
		t.Fatalf("LoopAppendLengths = %#v, want one numeric count fact", in.LoopAppendLengths)
	}
	fact := in.LoopAppendLengths[0]
	if fact.Count != 3 || fact.ParamIndex != -1 || fact.TargetRoot == 0 || fact.TargetKey == "" {
		t.Fatalf("numeric loop fact = %#v, want count=3 with versioned target", fact)
	}
}

func TestBuildLoopAppendLengthsPairsMapParam(t *testing.T) {
	in := loopAppendInputs(t, []string{"xs"}, `
local out = {}
for k, v in pairs(xs) do
	table.insert(out, v)
end
return out
`, "table", "pairs")
	markParamMap(t, &in, 0)

	facts := BuildLoopAppendLengths(in)
	if len(facts) != 1 {
		t.Fatalf("LoopAppendLengths = %#v, want one param relation fact", facts)
	}
	fact := facts[0]
	if fact.Count != 0 || fact.ParamIndex != 0 || fact.TargetRoot == 0 || fact.TargetKey == "" {
		t.Fatalf("pairs loop fact = %#v, want ParamIndex=0 with versioned target", fact)
	}
}

func TestBuildLoopAppendLengthsRejectsConditionalAppend(t *testing.T) {
	in := loopAppendInputs(t, []string{"xs"}, `
local out = {}
for k, v in pairs(xs) do
	if v then
		table.insert(out, v)
	end
end
return out
`, "table", "pairs")
	markParamMap(t, &in, 0)

	if facts := BuildLoopAppendLengths(in); len(facts) != 0 {
		t.Fatalf("conditional append produced loop length facts: %#v", facts)
	}
}

func TestBuildLoopAppendLengthsRejectsReassignedSourceParam(t *testing.T) {
	in := loopAppendInputs(t, []string{"xs", "other"}, `
local out = {}
xs = other
for k, v in pairs(xs) do
	table.insert(out, v)
end
return out
`, "table", "pairs")
	markParamMap(t, &in, 0)

	if facts := BuildLoopAppendLengths(in); len(facts) != 0 {
		t.Fatalf("reassigned source parameter produced loop length facts: %#v", facts)
	}
}

func TestBuildConstValuesPropagatesLocalLiteralKey(t *testing.T) {
	in := loopAppendInputs(t, nil, `
local key = "p-q"
Point(obj[key])
`, "Point", "obj")
	if in.Graph == nil {
		t.Fatal("missing graph")
	}
	symbols := in.Graph.Bindings().SymbolsByName("key")
	if len(symbols) != 1 {
		t.Fatalf("symbols named key = %v, want one", symbols)
	}
	var callPoint cfg.Point
	in.Graph.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if callPoint == 0 {
			callPoint = p
		}
	})
	if callPoint == 0 {
		t.Fatal("missing Point(obj[key]) call point")
	}
	val := in.ConstValues[symbols[0]][callPoint]
	if val == nil || val.Kind != flow.ConstString || val.Str != "p-q" {
		t.Fatalf("const key at call point = %#v, want p-q", val)
	}
}

func loopAppendInputs(t *testing.T, params []string, body string, globals ...string) Inputs {
	t.Helper()
	stmts, err := parse.ParseString(body, "loop_append.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	if len(params) > 0 {
		fn.ParList = &ast.ParList{Names: params}
	}
	return BuildFromFunction(fn, nil, nil, globals...)
}

func markParamMap(t *testing.T, in *Inputs, paramIndex int) {
	t.Helper()
	if in == nil || paramIndex < 0 || paramIndex >= len(in.Scope.ParamSymbols) {
		t.Fatalf("missing param symbol %d in %#v", paramIndex, in)
	}
	sym := in.Scope.ParamSymbols[paramIndex]
	if in.Scope.DeclaredTypes == nil {
		in.Scope.DeclaredTypes = make(map[cfg.SymbolID]typ.Type)
	}
	in.Scope.DeclaredTypes[sym] = typ.NewMap(typ.String, typ.String)
	if in.Scope.DeclaredParamTypes == nil {
		in.Scope.DeclaredParamTypes = make(map[int]typ.Type)
	}
	in.Scope.DeclaredParamTypes[paramIndex] = typ.NewMap(typ.String, typ.String)
}
