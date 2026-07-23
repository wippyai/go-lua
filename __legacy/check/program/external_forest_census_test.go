package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// runExternalCensusChunkWithGlobals mirrors the compiler/check adapter seam:
// ambient wippy globals arrive as declared global types alongside the stdlib
// signature source.
func runExternalCensusChunkWithGlobals(t *testing.T, src string, globalTypes map[string]typ.Type) {
	t.Helper()
	names := make([]string, 0, len(globalTypes))
	for name := range globalTypes {
		names = append(names, name)
	}
	stmts := parseChunk(t, src)
	if _, err := RunChunk(stmts, Config{Check: body.Config{
		Registry:    standard.Registry(),
		Globals:     names,
		GlobalTypes: globalTypes,
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
		},
	}}); err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
}

// Distilled from app.test.compress:zstd_dict (wippy-golua-seam/tests/app).
// Two declared ambient globals, one an interface type; a helper and its
// caller both read the record-typed global while the caller also reads the
// interface-typed one after it, so the caller receives the shared global
// carrier through the call edge next to its own.
func TestExternalCensusGlobalCarrierConflict(t *testing.T) {
	luaError := typ.NewInterface("Error", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	runExternalCensusChunkWithGlobals(t, `
local function helper()
	local m = mystr
end
local function main()
	helper()
	local m = mystr
	local e = errors
end
main()
`, map[string]typ.Type{
		"mystr":  typ.String,
		"errors": luaError,
	})
}

// Distilled from app.test.errors:internal (wippy-golua-seam/tests/app).
// A closure passed to pcall reads an ambient stdlib global while its
// enclosing function frames the call.
func TestExternalCensusAmbientSymbolCallerEnvironment(t *testing.T) {
	runExternalCensusChunk(t, `
local function main()
	pcall(function()
		error("boom")
	end)
end
`)
}

// Distilled from app.test.errors:stack (wippy-golua-seam/tests/app).
// A truthiness guard nested inside another guard combines a member read
// with a length comparison on the same unknown-typed value.
func TestExternalCensusBoundaryAtomExact(t *testing.T) {
	runExternalCensusChunk(t, `
local cs = errors.call_stack()
if cs then
	if cs.frames and #cs.frames > 0 then
	end
end
`)
}

// Distilled from app.test.types:import_base64 (wippy-golua-seam/tests/app).
// A function destructuring an unknown call into two locals is called twice
// inside one and-chain at the chunk root.
func TestExternalCensusWorldFreezerMalformed(t *testing.T) {
	runExternalCensusChunk(t, `
local function test_decode()
	local decoded, err = base64.decode("y")
	return err == nil
end
return test_decode() and test_decode()
`)
}

// Every structural and/or result is a control-flow phi: the bypass edge
// selects the left operand and every certified RHS exit selects the right.
// Nesting creates independent cells, and repeated calls keep their frame
// results path-owned rather than eagerly reading untaken frames at the join.
func TestStructuralLogicalResultCellsCoverNestedAndOrCallFrames(t *testing.T) {
	runExternalCensusChunk(t, `
local function yes()
	return true
end
local function mark(value)
	return value
end

return (yes() and mark("a")) or mark("b")
`)
}

func TestExternalCallInputTransportsLoopPathEvidenceToConcreteKeyspace(t *testing.T) {
	runExternalCensusChunk(t, `
type Expr = { eval: (source: string, env: unknown) -> (unknown, string?) }
type Target = { condition: string? }
type Node = { error_targets: {Target} }
local expr: Expr = {
	eval = function(_source: string, _env: unknown): (unknown, string?)
		return nil, nil
	end,
}
local function route_errors(self: Node): ({unknown}?, string?)
	local env = {}
	for _, target in ipairs(self.error_targets) do
		if target.condition then
			local _, err = expr.eval(target.condition :: string, env)
			if err then
				return nil, err
			end
		end
	end
 	return {}, nil
end
local node: Node = { error_targets = {} }
route_errors(node)
`)
}

// Distilled from app.test.fs:readdir (wippy-golua-seam/tests/app).
// A generic-for consumes an iterator/state pair produced by an unknown call.
func TestExternalCensusGenericForCustomIterator(t *testing.T) {
	runExternalCensusChunk(t, `
local iter, state = readdir("/x")
for entry in iter, state do
end
`)
}
