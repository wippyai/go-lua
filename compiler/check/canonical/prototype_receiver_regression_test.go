package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check"
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
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
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
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
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
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
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
