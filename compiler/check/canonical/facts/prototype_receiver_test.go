package facts

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
)

func TestBuildPreTransferStoresPrototypeReceiverFactsForClassIndexPattern(t *testing.T) {
	root := parseFactsTestChunk(t, `
local module = {}
local Class = {}
local class_mt = { __index = Class }

function module.new()
	return setmetatable({
		nodes = {},
	}, class_mt)
end

function Class:is_empty()
	return next(self.nodes) == nil
end

function Class:has_cycles()
	return false, nil
end
`)
	m, rootGraph := buildPrototypeFactsForSource(t, root)
	moduleSym := symbolNamed(t, rootGraph, "module")
	classSym := symbolNamed(t, rootGraph, "Class")
	mtSym := symbolNamed(t, rootGraph, "class_mt")

	assertMetatableIndex(t, m, mtSym, classSym)
	newRef := assertFieldFunc(t, m, moduleSym, "new")
	assertSetMetatableSite(t, m.SetMetatableSites(newRef), mtSym, classSym)

	isEmptyRef := assertFieldFunc(t, m, classSym, "is_empty")
	assertMethodReceiver(t, m.MethodReceivers(isEmptyRef), classSym)
	hasCyclesRef := assertFieldFunc(t, m, classSym, "has_cycles")
	assertMethodReceiver(t, m.MethodReceivers(hasCyclesRef), classSym)
	assertPrototypeMethod(t, m.PrototypeMethods(), classSym, "is_empty")
	assertPrototypeMethod(t, m.PrototypeMethods(), classSym, "has_cycles")
}

func TestBuildPreTransferStoresPrototypeReceiverFactsForSplitMethodTable(t *testing.T) {
	root := parseFactsTestChunk(t, `
local workflow_state = {}
local methods = {}
local workflow_state_mt = { __index = methods }

function workflow_state.new(dataflow_id)
	local instance = {
		dataflow_id = dataflow_id,
		nodes = {},
		active_yields = {},
		queued_commands = {},
	}
	return setmetatable(instance, workflow_state_mt), nil
end

function methods:load_state()
	self.nodes["root"] = {
		status = "failed",
		parent_node_id = "parent",
	}
	return self, nil
end

function methods:is_node_active(node_id)
	return self.nodes[node_id] ~= nil
end
`)
	m, rootGraph := buildPrototypeFactsForSource(t, root)
	workflowSym := symbolNamed(t, rootGraph, "workflow_state")
	methodsSym := symbolNamed(t, rootGraph, "methods")
	mtSym := symbolNamed(t, rootGraph, "workflow_state_mt")

	assertMetatableIndex(t, m, mtSym, methodsSym)
	newRef := assertFieldFunc(t, m, workflowSym, "new")
	assertSetMetatableSite(t, m.SetMetatableSites(newRef), mtSym, methodsSym)

	loadRef := assertFieldFunc(t, m, methodsSym, "load_state")
	assertMethodReceiver(t, m.MethodReceivers(loadRef), methodsSym)
	activeRef := assertFieldFunc(t, m, methodsSym, "is_node_active")
	assertMethodReceiver(t, m.MethodReceivers(activeRef), methodsSym)
	assertPrototypeMethod(t, m.PrototypeMethods(), methodsSym, "load_state")
	assertPrototypeMethod(t, m.PrototypeMethods(), methodsSym, "is_node_active")
}

func buildPrototypeFactsForSource(t *testing.T, root *ast.FunctionExpr) (Module, *cfg.Graph) {
	t.Helper()
	graphs := collectFactsTestGraphs(root, "next", "setmetatable")
	funcRefsBySymbol := factsTestFuncSymbolRefs(graphs)
	m := BuildPreTransfer(Program{
		Refs: factsTestRefs(graphs),
		Graph: func(r ref.FuncRef) *cfg.Graph {
			return graphByRef(graphs, r)
		},
		Evidence: func(g *cfg.Graph) api.FlowEvidence {
			return trace.GraphEvidence(g, g.Bindings())
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (ref.FuncRef, bool) {
			r, ok := funcRefsBySymbol[sym]
			return r, ok
		},
	})
	return m, graphs[root]
}

func assertMetatableIndex(t *testing.T, m Module, mt, proto cfg.SymbolID) {
	t.Helper()
	got, ok := m.PrototypeForMetatable(mt)
	if !ok || got != proto {
		t.Fatalf("PrototypeForMetatable(%d) = %d/%v, want %d/true; indexes=%+v", mt, got, ok, proto, m.MetatableIndexes())
	}
}

func assertFieldFunc(t *testing.T, m Module, owner cfg.SymbolID, field string) ref.FuncRef {
	t.Helper()
	r, ok := m.FieldFuncRef(owner, mustFieldKey(field))
	if !ok {
		t.Fatalf("FieldFuncRef(%d,%q) missing; methods=%+v", owner, field, m.PrototypeMethods())
	}
	return r
}

func assertSetMetatableSite(t *testing.T, sites []metatable.SetMetatableSite, mt, proto cfg.SymbolID) {
	t.Helper()
	for _, site := range sites {
		if site.MetatableSym == mt && site.PrototypeSym == proto {
			return
		}
	}
	t.Fatalf("missing setmetatable site mt=%d proto=%d in %+v", mt, proto, sites)
}

func assertMethodReceiver(t *testing.T, receivers []metatable.MethodReceiver, proto cfg.SymbolID) {
	t.Helper()
	for _, receiver := range receivers {
		if receiver.PrototypeSym == proto && receiver.SelfSlot == 0 {
			return
		}
	}
	t.Fatalf("missing method receiver proto=%d slot 0 in %+v", proto, receivers)
}

func assertPrototypeMethod(t *testing.T, methods []metatable.PrototypeMethod, proto cfg.SymbolID, field string) {
	t.Helper()
	key := mustFieldKey(field)
	for _, method := range methods {
		if method.PrototypeSym == proto && method.Field == key {
			return
		}
	}
	t.Fatalf("missing prototype method proto=%d field=%q in %+v", proto, field, methods)
}
