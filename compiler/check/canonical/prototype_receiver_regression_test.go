package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalPrototypeReceiverMethodSurfaceAtCallPoint(t *testing.T) {
	src := `
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

function module.build()
	local graph = module.new()
	if graph:is_empty() then
		return graph, nil
	end
	local has_cycles, cycle_desc = graph:has_cycles()
	if has_cycles then
		return nil, cycle_desc
	end
	return graph, nil
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn, point, receiver := methodCallPoint(t, res.Session.Results, "is_empty")
	sym, ok := fn.Graph.Bindings().SymbolOf(receiver)
	if !ok || sym == 0 {
		t.Fatalf("receiver %q has no symbol", receiver.Value)
	}
	got := fn.NarrowedTypeAt(point, constraint.NewPath(sym, receiver.Value))
	if _, ok := querycore.Method(got, "is_empty"); !ok {
		t.Fatalf("receiver at %s call point has no is_empty method: %v; diagnostics=%v", receiver.Value, got, testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalPrototypeReceiverMethodSurfaceSurvivesReturnedArgument(t *testing.T) {
	src := `
local compiler = {}
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
	return setmetatable({
		references = {},
		input_data = nil,
	}, flow_graph_mt)
end

function FlowGraph:resolve_reference(name)
	if not name then
		return nil, "missing"
	end
	return self.references[name], nil
end

function compiler.build_graph(operations, session_context)
	local graph = FlowGraph.new()
	graph.input_data = operations[1].config.data
	if session_context and session_context.node_id then
		graph.session_parent_id = session_context.node_id
	end
	graph.references.root = "root-id"
	return graph, nil
end

function compiler.validate_graph(graph)
	return graph:resolve_reference("root")
end

function compiler.compile_to_commands(graph, session_context)
	local node_id, err = graph:resolve_reference("root")
	if err then
		return nil, err
	end
	return node_id, nil
end

function compiler.compile(operations, session_context)
	local graph, err = compiler.build_graph(operations, session_context)
	if err then
		return nil, err
	end
	local ok, validation_err = compiler.validate_graph(graph)
	if not ok then
		return nil, validation_err
	end
	return compiler.compile_to_commands(graph, session_context)
end

compiler.compile({
	{ config = { data = "payload" } },
})
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected returned constructor value to keep prototype method surface through argument entry, got: %v", msgs)
	}
}

func TestCanonicalPrototypeReceiverMutatorEffectSurvivesReturnedBuilder(t *testing.T) {
	src := `
local compiler = {}
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
	return setmetatable({
		node_order = table.create(4, 0),
	}, flow_graph_mt)
end

function FlowGraph:add_node(node_id)
	table.insert(self.node_order, node_id)
	return node_id, nil
end

function FlowGraph:first_node()
	if #self.node_order == 0 then
		return nil, "empty"
	end
	return self.node_order[1], nil
end

function compiler.build_graph()
	local graph = FlowGraph.new()
	local _, err = graph:add_node("root-node")
	if err then
		return nil, err
	end
	return graph, nil
end

function compiler.compile_to_commands(graph)
	local node_id, err = graph:first_node()
	if err then
		return nil, err
	end
	return node_id:sub(1, 4), nil
end

function compiler.compile()
	local graph, err = compiler.build_graph()
	if err then
		return nil, err
	end
	return compiler.compile_to_commands(graph)
end

compiler.compile()
`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected method mutator receiver effect to survive returned builder, got: %v", msgs)
	}
}

func TestCanonicalReturnedGraphKeepsNodeOrderKeysForNodeAndEdgeMaps(t *testing.T) {
	src := `
local uuid = require("uuid")
local compiler = {}
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
	return setmetatable({
		nodes = table.create(0, 4),
		node_order = table.create(4, 0),
		edges = table.create(0, 4),
	}, flow_graph_mt)
end

function FlowGraph:create_node(node_type)
	local node_id = uuid.v7()
	self.nodes[node_id] = {
		node_id = node_id,
		node_type = node_type,
		parent_node_id = nil,
		config = {},
		metadata = {},
		status = "pending",
	}
	table.insert(self.node_order, node_id)
	self.edges[node_id] = {
		targets = table.create(2, 0),
		error_targets = table.create(1, 0),
	}
	return node_id, nil
end

function FlowGraph:compute_auto_chain()
	for i = 1, #self.node_order - 1 do
		local current_node_id = self.node_order[i]
		local next_node_id = self.node_order[i + 1]
		local current_node = self.nodes[current_node_id]
		local next_node = self.nodes[next_node_id]
		if not current_node.parent_node_id and not next_node.parent_node_id then
			local current_edges = self.edges[current_node_id]
			table.insert(current_edges.targets, {
				target_node_id = next_node_id,
			})
		end
	end
end

function compiler.build_graph()
	local graph = FlowGraph.new()
	local _, err = graph:create_node("a")
	if err then
		return nil, err
	end
	local _, err2 = graph:create_node("b")
	if err2 then
		return nil, err2
	end
	return graph, nil
end

function compiler.compile()
	local graph, err = compiler.build_graph()
	if err then
		return nil, err
	end
	graph:compute_auto_chain()
	return graph, nil
end

	compiler.compile()
	`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected node_order key provenance to survive node/edge map writes, got: %v", msgs)
	}
}

func TestCanonicalPrototypeReceiverBaselineDoesNotOverwriteMutatedBuilderReceiver(t *testing.T) {
	src := `
local uuid = require("uuid")
local compiler = {}
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
	return setmetatable({
		nodes = table.create(0, 4),
		node_order = table.create(4, 0),
		edges = table.create(0, 4),
	}, flow_graph_mt)
end

function FlowGraph:create_node(node_type)
	local node_id = uuid.v7()
	self.nodes[node_id] = {
		node_id = node_id,
		node_type = node_type,
		parent_node_id = nil,
		config = {},
		metadata = {},
		status = "pending",
	}
	table.insert(self.node_order, node_id)
	self.edges[node_id] = {
		targets = table.create(2, 0),
		error_targets = table.create(1, 0),
	}
	return node_id, nil
end

function FlowGraph:compute_auto_chain()
	for i = 1, #self.node_order - 1 do
		local current_node_id = self.node_order[i]
		local next_node_id = self.node_order[i + 1]
		local current_node = self.nodes[current_node_id]
		local next_node = self.nodes[next_node_id]
		if not current_node.parent_node_id and not next_node.parent_node_id then
			local current_edges = self.edges[current_node_id]
			table.insert(current_edges.targets, {
				target_node_id = next_node_id,
			})
		end
	end
end

function compiler.build_graph(operations)
	local graph = FlowGraph.new()
	for _, op in ipairs(operations) do
		local _, err = graph:create_node(op.kind)
		if err then
			return nil, err
		end
	end
	graph:compute_auto_chain()
	return graph, nil
end

function compiler.compile()
	local graph, err = compiler.build_graph({ { kind = "a" }, { kind = "b" } })
	if err then
		return nil, err
	end
	return graph, nil
end

	compiler.compile()
	`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected prototype baseline to preserve mutated builder receiver facts, got: %v", msgs)
	}
}

func TestCanonicalReturnedGraphKeepsConstructorSequenceFieldsAcrossValidation(t *testing.T) {
	src := `
local compiler = {}
local FlowGraph = {}
local flow_graph_mt = { __index = FlowGraph }

function FlowGraph.new()
	return setmetatable({
		node_order = table.create(4, 0),
		input_routes = table.create(4, 0),
		static_data_sources = table.create(4, 0),
	}, flow_graph_mt)
end

function compiler.build_graph(operations)
	local graph = FlowGraph.new()
	for _, op in ipairs(operations) do
		table.insert(graph.node_order, op.id)
		if op.input then
			table.insert(graph.input_routes, { target_name = op.input })
		end
		if op.static then
			table.insert(graph.static_data_sources, { routes = table.create(0, 0) })
		end
	end
	return graph, nil
end

function compiler.validate_graph(graph)
	if #graph.input_routes > 0 then
		for _, route in ipairs(graph.input_routes) do
			if not route.target_name then
				return false, "missing"
			end
		end
	end
	for _, src in ipairs(graph.static_data_sources) do
		if #src.routes > 0 then
			return false, "unexpected"
		end
	end
	return true, nil
end

function compiler.compile_to_commands(graph)
	local commands = table.create(#graph.node_order * 2, 0)
	local static_data_ids = table.create(0, #graph.static_data_sources)
	return commands, static_data_ids
end

function compiler.compile(operations)
	local graph, graph_err = compiler.build_graph(operations)
	if graph_err then
		return nil, graph_err
	end
	local valid, validation_err = compiler.validate_graph(graph)
	if not valid then
		return nil, validation_err
	end
	return compiler.compile_to_commands(graph)
end

	compiler.compile({ { id = "root", input = "next", static = true } })
	`
	res := testutil.Check(src, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected returned graph sequence fields to survive validation/compile boundary, got: %v", msgs)
	}
}

func TestCanonicalPrototypeReceiverSeedsMethodEntrySelfFromConstructorPublication(t *testing.T) {
	src := `
local session_writer = {}
session_writer.__index = session_writer

function session_writer.new(session_id: string)
	local self = setmetatable({}, session_writer)
	self.session_id = session_id
	return self
end

function session_writer:get_user_id(): string
	return self.user_id
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithParamNames(t, res.Session.Results, "self")
	params := fn.Graph.ParamSymbols()
	if len(params) == 0 || params[0] == 0 {
		t.Fatalf("method graph has no self parameter")
	}
	selfSym := params[0]
	selfType := fn.NarrowedTypeAt(fn.Graph.Entry(), constraint.NewPath(selfSym, "self"))
	if typ.IsAny(selfType) || typ.IsUnknown(selfType) {
		t.Fatalf("method-entry self = %v, want constructor-published instance shape; diagnostics=%v", selfType, testutil.ErrorMessages(res.Diagnostics))
	}
	sessionID, ok := querycore.Field(selfType, "session_id")
	if !ok || !typ.TypeEquals(sessionID, typ.String) {
		t.Fatalf("method-entry self.session_id = %v/%v, want string; self=%v diagnostics=%v", sessionID, ok, selfType, testutil.ErrorMessages(res.Diagnostics))
	}
	if userID, ok := querycore.Field(selfType, "user_id"); ok {
		t.Fatalf("method-entry self.user_id = %v/true, want absent field; self=%v diagnostics=%v", userID, selfType, testutil.ErrorMessages(res.Diagnostics))
	}
	retPoint, retExpr := singleReturnExpr(t, fn)
	actual := observation.FromFuncResult(fn, nil).WithProofValues().TypeOfWithExpected(retExpr, retPoint, typ.String)
	if !typ.TypeEquals(actual, typ.Nil) {
		t.Fatalf("return self.user_id observed as %v, want nil from absent field before return check; self=%v diagnostics=%v", actual, selfType, testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalSplitMetatableReceiverCarriesLoadStateWrites(t *testing.T) {
	src := `
local workflow_state = {}
local methods = {}
local workflow_state_mt = { __index = methods }

function workflow_state.new(dataflow_id)
	if not dataflow_id or dataflow_id == "" then
		return nil, "Dataflow ID is required"
	end

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
	self.active_yields["parent"] = {
		pending_children = {},
		results = {},
	}
	return self, nil
end

function methods:get_failed_node_errors()
	local failed_nodes = {}
	for node_id, node_data in pairs(self.nodes) do
		if node_data.status == "failed" then
			table.insert(failed_nodes, node_id)
		end
	end
	return table.concat(failed_nodes, "; ")
end

local state, err = workflow_state.new("df")
if err then return nil, err end
state:load_state()
state:get_failed_node_errors()
`
	res := testutil.Check(src, testutil.WithStdlib())
	loadFn, _ := functionAssigningSelfStaticIndex(t, res.Session.Results, "nodes", "root")
	selfSym := loadFn.Graph.ParamSymbols()[0]
	selfPath := constraint.NewPath(selfSym, "self")
	entrySelf := loadFn.NarrowedTypeAt(loadFn.Graph.Entry(), selfPath)
	if nodes, ok := querycore.Field(entrySelf, "nodes"); !ok || nodes == nil || typ.TypeEquals(nodes, typ.Nil) {
		t.Fatalf("load_state entry self.nodes = %v/%v, want constructor-published table; self=%v diagnostics=%v", nodes, ok, entrySelf, testutil.ErrorMessages(res.Diagnostics))
	}
	retPoint := firstReturnPoint(t, loadFn)
	statusPath := selfPath.Field("nodes").IndexStr("root").Field("status")
	status := loadFn.NarrowedTypeAt(retPoint, statusPath)
	if !typ.TypeEquals(status, typ.LiteralString("failed")) {
		t.Fatalf("load_state return self.nodes[\"root\"].status = %v, want \"failed\"; self=%v diagnostics=%v", status, loadFn.NarrowedTypeAt(retPoint, selfPath), testutil.ErrorMessages(res.Diagnostics))
	}

	rootFn, nextPoint, receiver := methodCallPoint(t, res.Session.Results, "get_failed_node_errors")
	stateSym, ok := rootFn.Graph.Bindings().SymbolOf(receiver)
	if !ok || stateSym == 0 {
		t.Fatalf("receiver %q has no symbol", receiver.Value)
	}
	callerStatusPath := constraint.NewPath(stateSym, receiver.Value).Field("nodes").IndexStr("root").Field("status")
	callerStatus := rootFn.NarrowedTypeAt(nextPoint, callerStatusPath)
	if !typ.TypeEquals(callerStatus, typ.LiteralString("failed")) {
		t.Fatalf("caller state.nodes[\"root\"].status after load_state = %v, want \"failed\"; state=%v diagnostics=%v", callerStatus, rootFn.NarrowedTypeAt(nextPoint, constraint.NewPath(stateSym, receiver.Value)), testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalSplitMetatableReceiverPreservesConstructorAliasWithUnknownFields(t *testing.T) {
	src := `
local node = {}
local methods = {}
local mt = { __index = methods }

type RouteTarget = {
	data_type: string?,
	metadata: unknown?,
}

type NodeConfig = {
	data_targets: {RouteTarget}?,
	input_transform: unknown?,
}

type NodeDefinition = {
	config: NodeConfig?,
	metadata: {[string]: unknown}?,
}

type NodeArgs = {
	node_id: string,
	node: NodeDefinition?,
}

function node.new(args: NodeArgs)
	local instance = {
		node_id = args.node_id,
		data_targets = args.node and args.node.config and args.node.config.data_targets or {},
		_queued_commands = {},
		_deps = args.node,
	}

	return setmetatable(instance, mt), nil
end

function methods:submit()
	if #self._queued_commands == 0 then
		return "Node [" .. self.node_id .. "]", nil
	end
	return "queued", nil
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	if res.HasError() {
		t.Fatalf("expected aliased constructor contract with unknown fields to seed method self, got: %v", testutil.ErrorMessages(res.Diagnostics))
	}
	fn := findFunctionWithParamNames(t, res.Session.Results, "self")
	selfSym := fn.Graph.ParamSymbols()[0]
	selfPath := constraint.NewPath(selfSym, "self")
	entrySelf := fn.NarrowedTypeAt(fn.Graph.Entry(), selfPath)
	nodeID, ok := querycore.Field(entrySelf, "node_id")
	if !ok || !typ.TypeEquals(nodeID, typ.String) {
		t.Fatalf("method-entry self.node_id = %v/%v, want string; self=%v", nodeID, ok, entrySelf)
	}
}

func TestCanonicalSplitMetatableReceiverSeedsMethodEntryWithoutLocalCall(t *testing.T) {
	src := `
local node = {}
local methods = {}
local mt = { __index = methods }

type NodeInstance = {
	node_id: string,
	queued: {unknown},
}

function node.new(node_id: string)
	local instance: NodeInstance = {
		node_id = node_id,
		queued = {},
	}
	return setmetatable(instance, mt), nil
end

function methods:submit()
	if #self.queued == 0 then
		return "Node [" .. self.node_id .. "]", nil
	end
	return "queued", nil
end

return node
`
	res := testutil.Check(src, testutil.WithStdlib())
	if res.HasError() {
		t.Fatalf("expected exported split-metatable constructor to seed method self without a local call, got: %v", testutil.ErrorMessages(res.Diagnostics))
	}
	fn := findFunctionWithParamNames(t, res.Session.Results, "self")
	selfSym := fn.Graph.ParamSymbols()[0]
	selfPath := constraint.NewPath(selfSym, "self")
	entrySelf := fn.NarrowedTypeAt(fn.Graph.Entry(), selfPath)
	nodeID, ok := querycore.Field(entrySelf, "node_id")
	if !ok || !typ.TypeEquals(nodeID, typ.String) {
		t.Fatalf("method-entry self.node_id = %v/%v, want string; self=%v", nodeID, ok, entrySelf)
	}
}

func TestCanonicalSplitMetatableReceiverPreservesNestedDependencyCallFields(t *testing.T) {
	src := `
local node = {}
local methods = {}
local mt = { __index = methods }

type Commit = {
	submit: (dataflow_id: string, op_id: string, commands: {unknown}) -> (boolean, string?),
}

type Deps = {
	commit: Commit,
}

type NodeInstance = {
	dataflow_id: string,
	queued: {unknown},
	_deps: Deps,
}

local commit: Commit = {
	submit = function(_dataflow_id: string, _op_id: string, _commands: {unknown}): (boolean, string?)
		return true, nil
	end,
}

local deps: Deps = {
	commit = commit,
}

function node.new(dataflow_id: string, supplied: Deps?)
	local effective_deps: Deps = supplied or deps
	local instance: NodeInstance = {
		dataflow_id = dataflow_id,
		queued = {},
		_deps = effective_deps,
	}
	return setmetatable(instance, mt), nil
end

function methods:submit()
	return self._deps.commit.submit(self.dataflow_id, "op", self.queued)
end

return node
`
	res := testutil.Check(src, testutil.WithStdlib())
	if res.HasError() {
		t.Fatalf("expected nested dependency function fields to remain callable through prototype self, got: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalSplitMetatableReceiverKeepsMethodsInsideCallback(t *testing.T) {
	src := `
local node = {}
local methods = {}
local mt = { __index = methods }

type NodeInstance = {
	node_id: string,
}

function node.new(node_id: string)
	local instance: NodeInstance = {
		node_id = node_id,
	}
	return setmetatable(instance, mt), nil
end

function methods:inputs()
	return { status = self.node_id }
end

function methods:_route_outputs()
	local ok, values = pcall(function()
		local values = self:inputs()
		return values
	end)
	return ok, values
end

return node
`
	res := testutil.Check(src, testutil.WithStdlib())
	if res.HasError() {
		t.Fatalf("expected callback-captured prototype receiver to keep method surface, got: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalSplitMetatableReceiverComposesExactMethodSelfWithConstructorBaseline(t *testing.T) {
	src := `
local node = {}
local methods = {}
local mt = { __index = methods }

type Commit = {
	submit: (dataflow_id: string, op_id: string, commands: {unknown}) -> (boolean, string?),
}

type Deps = {
	commit: Commit,
}

type NodeInstance = {
	dataflow_id: string,
	queued: {unknown},
	_deps: Deps,
}

local commit: Commit = {
	submit = function(_dataflow_id: string, _op_id: string, _commands: {unknown}): (boolean, string?)
		return true, nil
	end,
}

local deps: Deps = {
	commit = commit,
}

function node.new(dataflow_id: string)
	local instance: NodeInstance = {
		dataflow_id = dataflow_id,
		queued = {},
		_deps = deps,
	}
	return setmetatable(instance, mt), nil
end

function methods:_submit_final()
	return self._deps.commit.submit(self.dataflow_id, "op", self.queued)
end

function methods:complete()
	return self:_submit_final()
end

local instance, err = node.new("df")
if err then
	return nil, err
end
return instance:complete()
`
	res := testutil.Check(src, testutil.WithStdlib())
	if res.HasError() {
		t.Fatalf("expected method-to-method receiver self to retain constructor-required fields, got: %v", testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalSplitMetatableReceiverDoesNotRepairBadDirectSelfCall(t *testing.T) {
	src := `
local node = {}
local methods = {}
local mt = { __index = methods }

type Commit = {
	submit: (dataflow_id: string, op_id: string, commands: {unknown}) -> (boolean, string?),
}

type Deps = {
	commit: Commit,
}

type NodeInstance = {
	dataflow_id: string,
	queued: {unknown},
	_deps: Deps,
}

local commit: Commit = {
	submit = function(_dataflow_id: string, _op_id: string, _commands: {unknown}): (boolean, string?)
		return true, nil
	end,
}

local deps: Deps = {
	commit = commit,
}

function node.new(dataflow_id: string)
	local instance: NodeInstance = {
		dataflow_id = dataflow_id,
		queued = {},
		_deps = deps,
	}
	return setmetatable(instance, mt), nil
end

function methods:submit()
	return self._deps.commit.submit(self.dataflow_id, "op", self.queued)
end

local bad: {} = {}
return methods.submit(bad)
`
	res := testutil.Check(src, testutil.WithStdlib())
	if !res.HasError() {
		t.Fatalf("expected bad direct self call to remain invalid")
	}
}

func methodCallPoint(t *testing.T, results map[*ast.FunctionExpr]*api.FuncResult, method string) (*api.FuncResult, cfg.Point, *ast.IdentExpr) {
	t.Helper()
	for _, result := range results {
		if result == nil || result.Graph == nil {
			continue
		}
		for _, call := range result.Evidence.Calls {
			if call.Info == nil || call.Info.Call == nil || call.Info.Call.Method != method {
				continue
			}
			ident, ok := call.Info.Call.Receiver.(*ast.IdentExpr)
			if !ok || ident == nil {
				continue
			}
			return result, call.Point, ident
		}
	}
	t.Fatalf("no call to method %q", method)
	return nil, 0, nil
}

func singleReturnExpr(t *testing.T, result *api.FuncResult) (cfg.Point, ast.Expr) {
	t.Helper()
	if result == nil {
		t.Fatal("nil FuncResult")
	}
	var point cfg.Point
	var expr ast.Expr
	for _, ret := range result.Evidence.Returns {
		if ret.Info == nil || len(ret.Info.Exprs) != 1 {
			continue
		}
		if expr != nil {
			t.Fatalf("multiple single-expression returns")
		}
		point = ret.Point
		expr = ret.Info.Exprs[0]
	}
	if expr == nil {
		t.Fatalf("no single-expression return")
	}
	return point, expr
}

func firstReturnPoint(t *testing.T, result *api.FuncResult) cfg.Point {
	t.Helper()
	if result == nil || result.Graph == nil {
		t.Fatal("nil FuncResult")
	}
	var point cfg.Point
	result.Graph.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		if point == 0 {
			point = p
		}
	})
	if point == 0 {
		t.Fatalf("no return point")
	}
	return point
}

func functionAssigningSelfStaticIndex(t *testing.T, results map[*ast.FunctionExpr]*api.FuncResult, field, key string) (*api.FuncResult, cfg.Point) {
	t.Helper()
	for _, result := range results {
		if result == nil || result.Graph == nil {
			continue
		}
		params := result.Graph.ParamSymbols()
		if len(params) == 0 || result.Graph.NameOf(params[0]) != "self" {
			continue
		}
		var point cfg.Point
		result.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
			if point != 0 || info == nil {
				return
			}
			for _, target := range info.Targets {
				if targetExprMatchesSelfStaticIndex(target.Expr, field, key) {
					point = p
					return
				}
			}
		})
		if point != 0 {
			return result, point
		}
	}
	t.Fatalf("no self.%s[%q] assignment", field, key)
	return nil, 0
}

func targetExprMatchesSelfStaticIndex(expr ast.Expr, field, key string) bool {
	root, segments, ok := staticExprPath(expr)
	if !ok || root != "self" || len(segments) < 2 {
		return false
	}
	first := segments[0]
	second := segments[1]
	return first.Kind == constraint.SegmentField &&
		first.Name == field &&
		second.Kind == constraint.SegmentIndexString &&
		second.Name == key
}

func staticExprPath(expr ast.Expr) (string, []constraint.Segment, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e == nil || e.Value == "" {
			return "", nil, false
		}
		return e.Value, nil, true
	case *ast.AttrGetExpr:
		if e == nil {
			return "", nil, false
		}
		key, ok := e.Key.(*ast.StringExpr)
		if !ok || key.Value == "" {
			return "", nil, false
		}
		root, segments, ok := staticExprPath(e.Object)
		if !ok {
			return "", nil, false
		}
		switch e.KeySyntax {
		case ast.AttrKeyDot:
			segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: key.Value})
		case ast.AttrKeyIndex:
			segments = append(segments, constraint.Segment{Kind: constraint.SegmentIndexString, Name: key.Value})
		default:
			segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: key.Value})
		}
		return root, segments, true
	default:
		return "", nil, false
	}
}
