package program_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestErrorReturnSignatureRefinesValuePresenceAcrossErrorBranch(t *testing.T) {
	reg := standard.Registry()
	m := manifest.New("test")
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().
			Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.String)).
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
		if d.Code == diagnostics.CodeMissingMember && strings.Contains(d.Message, "release") {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want optional receiver diagnostic for release", diags)
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
		if d.Code == diagnostics.CodeMissingMember && strings.Contains(d.Message, "release") {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want optional receiver diagnostic for release", diags)
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
