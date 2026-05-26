package abstract_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractCapturedFieldEvidence(t *testing.T) {
	graph := graphFromSource(t, `
local c = {}
c.name = "worker"
c["count"] = 1
`)
	symC := mustSymbol(t, graph, "c")
	evidence := trace.GraphEvidence(graph, graph.Bindings())
	inputs := abstract.BuildInputs(&core.FlowContext{
		Graph:    graph,
		Evidence: evidence,
	})
	got := abstract.ExtractCapturedFieldEvidence(inputs, map[cfg.SymbolID]bool{symC: true})

	fields := make(map[string]bool)
	for _, ev := range got {
		if ev.Target == symC {
			fields[ev.Field] = true
		}
	}
	for _, field := range []string{"name", "count"} {
		if !fields[field] {
			t.Fatalf("missing captured field evidence for %q; all=%v", field, fields)
		}
	}
}

func TestExtractCapturedFieldEvidence_UsesLoweredNestedFunctionDefinitionPath(t *testing.T) {
	graph := graphFromSource(t, `
local c = { repo = {} }
function c.repo.list()
	return {}
end
`)
	symC := mustSymbol(t, graph, "c")
	evidence := trace.GraphEvidence(graph, graph.Bindings())
	inputs := abstract.BuildInputs(&core.FlowContext{
		Graph:    graph,
		Evidence: evidence,
	})
	got := abstract.ExtractCapturedFieldEvidence(inputs, map[cfg.SymbolID]bool{symC: true})

	for _, ev := range got {
		if ev.Target != symC || ev.Field != "repo" || len(ev.TargetPath.Segments) != 2 {
			continue
		}
		if ev.TargetPath.Segments[0].Name == "repo" && ev.TargetPath.Segments[1].Name == "list" {
			return
		}
	}
	t.Fatalf("missing captured nested function-definition path; all=%#v", got)
}

func TestExtractCapturedContainerEvidence(t *testing.T) {
	graph := graphFromSource(t, `
local c = {}
`)
	symC := mustSymbol(t, graph, "c")
	inputs := &flow.Inputs{
		ContainerMutatorAssignments: []flow.ContainerMutatorAssignment{
			{
				Point:     graph.Exit(),
				Target:    constraint.NewPath(symC, "c"),
				ValueType: typ.Number,
			},
		},
		TableMutatorAssignments: []flow.TableMutatorAssignment{
			{
				Point:     graph.Exit(),
				Target:    constraint.NewPath(symC, "c").Field("items"),
				ValueType: typ.Number,
			},
		},
	}
	got := abstract.ExtractCapturedContainerEvidence(&core.FlowContext{
		Graph: graph,
	}, inputs, map[cfg.SymbolID]bool{symC: true})

	var sawContainer, sawTable bool
	for _, ev := range got {
		if ev.Target != symC {
			continue
		}
		switch ev.Kind {
		case api.ContainerMutationContainerElement:
			sawContainer = true
		case api.ContainerMutationTableElement:
			sawTable = len(ev.Segments) == 1 && ev.Segments[0].Name == "items"
		}
	}
	if !sawContainer {
		t.Fatal("missing captured generic container mutation evidence")
	}
	if !sawTable {
		t.Fatalf("missing captured table mutation evidence with .items path; all=%#v", got)
	}
}

func TestExtractCapturedContainerEvidence_UsesLoweredMapMutatorAssignments(t *testing.T) {
	graph := graphFromSource(t, `
local c = {}
`)
	symC := mustSymbol(t, graph, "c")
	valueType := typ.NewRecord().Field("last_activity", typ.String).Build()
	inputs := &flow.Inputs{
		MapMutatorAssignments: []flow.MapMutatorAssignment{
			{
				Point: graph.Exit(),
				Target: constraint.Path{
					Root:     "c",
					Symbol:   symC,
					Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "sessions"}},
				},
				KeyType:   typ.String,
				ValueType: valueType,
			},
		},
	}

	got := abstract.ExtractCapturedContainerEvidence(&core.FlowContext{
		Graph: graph,
	}, inputs, map[cfg.SymbolID]bool{symC: true})

	if len(got) != 1 {
		t.Fatalf("captured container evidence = %#v, want one", got)
	}
	if got[0].Kind != api.ContainerMutationMapElement ||
		got[0].Target != symC ||
		len(got[0].Segments) != 1 ||
		got[0].Segments[0].Name != "sessions" ||
		!typ.TypeEquals(got[0].KeyType, typ.String) ||
		!typ.TypeEquals(got[0].ValueType, valueType) {
		t.Fatalf("unexpected lowered map mutator evidence: %#v", got[0])
	}
}

func TestExtractFunctionEscapeEvidence(t *testing.T) {
	graph := graphFromSource(t, `
local api = {}
local function local_worker()
	return nil
end
api.worker = local_worker
function api.add()
	return nil
end
`)
	bindings := graph.Bindings()
	localSym := mustSymbol(t, graph, "local_worker")

	var addSym cfg.SymbolID
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info != nil && info.Name == "add" {
			addSym = info.Symbol
		}
	})
	if addSym == 0 {
		t.Fatal("expected api.add symbol")
	}

	got := trace.FunctionEscapes(graph, bindings)
	seen := make(map[cfg.SymbolID]bool)
	for _, ev := range got {
		seen[ev.Symbol] = true
	}
	if !seen[localSym] {
		t.Fatalf("missing field-assigned local function escape; all=%v", seen)
	}
	if !seen[addSym] {
		t.Fatalf("missing function-definition escape; all=%v", seen)
	}
}

func TestExtractFunctionDefinitionEvidence(t *testing.T) {
	graph := graphFromSource(t, `
local api = {}
local assigned = function()
	return 1
end
local function named()
	return assigned()
end
function api.add()
	return named()
end
`)
	assignedSym := mustSymbol(t, graph, "assigned")
	namedSym := mustSymbol(t, graph, "named")

	got := trace.FunctionDefinitions(graph)
	bySym := make(map[cfg.SymbolID]api.FunctionDefinitionEvidence)
	for _, ev := range got {
		if ev.Symbol != 0 {
			bySym[ev.Symbol] = ev
		}
	}

	assigned := bySym[assignedSym]
	if assigned.Symbol != assignedSym || assigned.Name != "assigned" || !assigned.IsLocal || assigned.FuncDef != nil {
		t.Fatalf("assigned function evidence = %#v", assigned)
	}
	named := bySym[namedSym]
	if named.Symbol != namedSym || named.Name != "named" || !named.IsLocal {
		t.Fatalf("named function evidence = %#v", named)
	}

	var sawFieldDefinition bool
	for _, ev := range got {
		if ev.FuncDef != nil && ev.FuncDef.Name == "add" && ev.Symbol != 0 {
			sawFieldDefinition = true
		}
	}
	if !sawFieldDefinition {
		t.Fatalf("missing api.add function-definition evidence; all=%#v", got)
	}
}

func graphFromSource(t *testing.T, source string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(source, "evidence.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}, "send", "table")
	if graph == nil {
		t.Fatal("expected graph")
	}
	return graph
}

func mustSymbol(t *testing.T, graph *cfg.Graph, name string) cfg.SymbolID {
	t.Helper()
	sym, ok := graph.SymbolAt(graph.Exit(), name)
	if !ok || sym == 0 {
		t.Fatalf("expected symbol for %s", name)
	}
	return sym
}
