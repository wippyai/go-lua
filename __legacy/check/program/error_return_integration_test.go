package program_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestErrorReturnSignatureRefinesValuePresenceAcrossErrorBranch(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("test")
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().
			Returns(typeexpr.Optional(typ.Number), typeexpr.Optional(typ.String)).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})

	stmts := parseChunk(t, `
		local value, err = f()
		if err == nil then
			local ok = value
		else
			local failed = value
		end
	`)
	result := runChunk(t, stmts, body.Config{
		Registry: reg,
		Globals:  []string{"f"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})

	value := localSymbolAt(t, result, stmts[0].(*ast.LocalAssignStmt), 0)
	branch := firstBranchPoint(t, result)
	thenPoint := branchSuccessor(t, result.Graph(), branch, true)
	elsePoint := branchSuccessor(t, result.Graph(), branch, false)

	thenState := stateAt(t, result, thenPoint)
	assertSymbolPresence(t, reg, thenState, value, presence.Present())
	elseState := stateAt(t, result, elsePoint)
	assertSymbolPresence(t, reg, elseState, value, presence.Absent())

	if diags := diagnostics.Produce(result); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestErrorReturnSignatureRefinesErrorPresenceAcrossValueAbsentBranch(t *testing.T) {
	reg := standard.Registry()
	errType := typ.NewInterface("Error", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	userType := typ.NewInterface("User", []typ.Method{
		{Name: "id", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	m := manifest.New("test")
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().
			Returns(typeexpr.Optional(userType), typeexpr.Optional(errType)).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})

	stmts := parseChunk(t, `
		local user, err = f()
		if user == nil then
			local kind: string = err:kind()
		end
	`)
	result := runChunk(t, stmts, body.Config{
		Registry: reg,
		Globals:  []string{"f"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})

	if diags := diagnostics.Produce(result); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want value-absent branch to prove error present", diags)
	}
}

func TestErrorReturnLocalFunctionSummaryRefinesValuePresenceAcrossGuard(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		local function process(x: number): (number?, string?)
			if x < 0 then
				return nil, "negative"
			end
			return x * 2, nil
		end
		local result, err = process(5)
		if err ~= nil then
			return
		end
		local n: number = result
	`)
	result := runChunk(t, stmts, body.Config{Registry: reg})
	assign := stmts[1].(*ast.LocalAssignStmt)
	value := localSymbolAt(t, result, assign, 0)
	branch := firstBranchPoint(t, result)
	normalPoint := branchSuccessor(t, result.Graph(), branch, false)
	normalState := stateAt(t, result, normalPoint)
	assertSymbolPresence(t, reg, normalState, value, presence.Present())
	nPoint := localAssignmentPointByName(t, result, "n")
	nValue, ok := result.SymbolValueAtBoundary(nPoint, value)
	if !ok {
		t.Fatalf("missing result boundary value at n assignment")
	}
	if got := product.PresenceOf(nValue); !presence.Equal(got, presence.Present()) {
		t.Fatalf("result boundary presence at n assignment = %s, want present", got)
	}
	if diags := diagnostics.Produce(result); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestErrorReturnLocalFunctionWithoutGuardKeepsReceiverOptional(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		type DB = {release: fun(self)}
		local real_db: DB = {release = function(self) end}
		local function fetch(ok: boolean): (DB?, string?)
			if not ok then
				return nil, "failed"
			end
			return real_db
		end
		local function use(ok: boolean)
			local db, err = fetch(ok)
			db:release()
		end
	`)
	result := runChunk(t, stmts, body.Config{Registry: reg})

	diags := diagnostics.Produce(result)
	for _, d := range diags {
		if d.Code == diagnostics.CodeOptionalMethodCall {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want optional receiver diagnostic for release", diags)
}

func TestErrorReturnLocalFunctionWithoutGuardKeepsFieldReadOptional(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		type Config = {host: string, port: number}
		local function parse_config(ok: boolean): (Config?, string?)
			if not ok then
				return nil, "failed"
			end
			return {host = "localhost", port = 8080}, nil
		end
		local function use(ok: boolean)
			local cfg, err = parse_config(ok)
			local host: string = cfg.host
		end
	`)
	result := runChunk(t, stmts, body.Config{Registry: reg})

	diags := diagnostics.Produce(result)
	for _, d := range diags {
		if d.Code == diagnostics.CodeAssignmentType && strings.Contains(d.Message, "cfg.host") && strings.Contains(d.Message, "may be nil") {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want cfg.host optional assignment diagnostic without err guard", diags)
}

func TestErrorReturnDelegatedTailCallRefinesReceiverAcrossGuard(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		type DB = {release: fun(self)}
		local real_db: DB = {release = function(self) end}
		local function open_db(ok: boolean): (DB?, string?)
			if not ok then
				return nil, "failed"
			end
			return real_db
		end
		local function get_db(ok: boolean): (DB?, string?)
			return open_db(ok)
		end
		local function use(ok: boolean)
			local db, err = get_db(ok)
			if err then
				return
			end
			db:release()
		end
	`)
	result := runChunk(t, stmts, body.Config{Registry: reg})

	if diags := diagnostics.Produce(result); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestErrorReturnDelegatedTailCallDoesNotInventRelationFromOptionalDeclaration(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		type DB = {release: fun(self)}
		local real_db: DB = {release = function(self) end}
		local function uncertain(ok: boolean): (DB?, string?)
			if ok then
				return nil, nil
			end
			return real_db, nil
		end
		local function wrap(ok: boolean): (DB?, string?)
			return uncertain(ok)
		end
		local function use(ok: boolean)
			local db, err = wrap(ok)
			if err then
				return
			end
			db:release()
		end
	`)
	result := runChunk(t, stmts, body.Config{Registry: reg})

	diags := diagnostics.Produce(result)
	for _, d := range diags {
		if d.Code == diagnostics.CodeOptionalMethodCall {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want optional receiver diagnostic for release", diags)
}

func TestTableFreezeAndIsFrozenBranchCarryFrozenProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		table.freeze(t)
		if table.isfrozen(t) then
			local ok = t
		end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"table", "t"}})
	tValue, ok := bindings.GlobalSymbol("t")
	if !ok {
		t.Fatal("missing global symbol for t")
	}
	entryID := testTableIdentity(31, 31)
	entryState := state.State{}.WriteValue(reg, key.SymbolValue(tValue), product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(entryID)))
	checked, err := program.RunBoundChunk(stmts, bindings, program.Config{Check: body.Config{
		Registry:   reg,
		Globals:    []string{"table", "t"},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
		EntryState: entryState,
	}})
	if err != nil {
		t.Fatalf("RunBoundChunk: %v", err)
	}
	result := checked.RootResult()

	branch := firstBranchPoint(t, result)
	branchState := stateAt(t, result, branch)
	idValue := branchState.ReadValue(reg, key.SymbolValue(tValue))
	tableID, ok := product.Get(reg, idValue, identity.Key).ID()
	if !ok {
		t.Fatalf("table value identity missing at branch point")
	}
	if !branchState.IsTableFrozen(tableID) {
		t.Fatalf("branch state is not frozen for %s", tableID)
	}

	if diags := diagnostics.Produce(result); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestErrorReturnGuardRefinesCallbackArgument(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
		type Err = {kind: string, message: string}
		type User = {id: string, name: string, roles: {string}}
		type Session = {id: string, user: User}
		local admin_roles: {string} = {"admin"}
		local users: {[string]: User} = {
			u1 = {id = "u1", name = "Ada", roles = admin_roles},
		}
		local M = {}
		function M.find_user(id: string): (User?, Err?)
			local user = users[id]
			if not user then
				return nil, {kind = "not_found", message = id}
			end
			return user, nil
		end
		function M.create_session(user: User, now: number): (Session?, Err?)
			return {id = user.id, user = user}, nil
		end
		function M.with_user(id: string, now: number, fn: (User, number) -> (Session?, Err?)): (Session?, Err?)
			local user, err = M.find_user(id)
			if err then
				return nil, err
			end
			return fn(user, now)
		end
	local session, err = M.with_user("u1", 1, M.create_session)
	`)
	result := runChunk(t, stmts, body.Config{Registry: reg})

	if diags := diagnostics.Produce(result); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want callback user argument refined after error guard", diags)
	}
}

func runChunk(t *testing.T, stmts []ast.Stmt, config body.Config) *body.Result {
	t.Helper()
	result, err := program.RunChunk(stmts, program.Config{Check: config})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	return result.RootResult()
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "error_return_integration_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func localSymbolAt(t *testing.T, result *body.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := result.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) || locals[index] == 0 {
		t.Fatalf("local symbol %d missing in %#v", index, locals)
	}
	return locals[index]
}

func firstBranchPoint(t *testing.T, result *body.Result) cfg.Point {
	t.Helper()
	graph := result.Graph()
	for _, point := range graph.RPO() {
		if _, ok := result.BranchCondition(point); ok {
			return point
		}
	}
	t.Fatalf("missing branch condition")
	return 0
}

func localAssignmentPointByName(t *testing.T, result *body.Result, name string) cfg.Point {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Name == name {
			return point
		}
	}
	t.Fatalf("missing local assignment %q", name)
	return 0
}

func branchSuccessor(t *testing.T, graph cfg.Graph, branch cfg.Point, cond bool) cfg.Point {
	t.Helper()
	for _, succ := range graph.Successors(branch) {
		gotCond, ok := graph.EdgeCond(branch, succ)
		if ok && gotCond == cond {
			return succ
		}
	}
	t.Fatalf("missing branch successor for %v edge from %d", cond, branch)
	return 0
}

func stateAt(t *testing.T, result *body.Result, point cfg.Point) state.State {
	t.Helper()
	got, ok := result.StateAt(point)
	if !ok {
		t.Fatalf("missing state at %d", point)
	}
	return got
}

func assertSymbolPresence(t *testing.T, reg *axis.Registry, st state.State, sym symbol.ID, want presence.Value) {
	t.Helper()
	got := product.PresenceOf(st.ReadValue(reg, key.SymbolValue(sym)))
	if !presence.Equal(got, want) {
		t.Fatalf("symbol %d presence = %s, want %s", sym, got, want)
	}
}
