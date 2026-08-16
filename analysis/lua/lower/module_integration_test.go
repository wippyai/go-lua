package lower_test

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

var boundarySourceCases = []sourceCase{
	{ID: "boundaries.case.import.direct-global.literal", Form: "FuncCallExpr", Source: "local Module = require(\"pkg.core\")\n", Line: 1},
	{ID: "boundaries.case.import.direct-global.nonliteral", Form: "FuncCallExpr", Source: "local request = \"runtime\"\nlocal Dynamic = require(request)\n", Line: 2},
}

// These are the two atomic boundary sources.  Each test follows the exact
// authored call from parser evidence through the binder and into the sealed
// Program; it does not infer an Import from Program-wide shape.
func TestSourceBoundaryAtomicImportWitnesses(t *testing.T) {
	for _, sourceCase := range boundarySourceCases {
		sourceCase := sourceCase
		t.Run(sourceCase.ID, func(t *testing.T) {
			call, binding := boundaryRequireAnchor(t, sourceCase)
			assertDirectGlobalRequire(t, binding, call)
			p := lowerBoundarySource(t, sourceCase.Source)

			switch sourceCase.ID {
			case "boundaries.case.import.direct-global.literal":
				assertLiteralBoundaryImport(t, p)
			case "boundaries.case.import.direct-global.nonliteral":
				assertNonliteralBoundaryCall(t, p)
			default:
				t.Fatalf("unhandled boundary source case %s", sourceCase.ID)
			}
		})
	}
}

func assertLiteralBoundaryImport(t *testing.T, p *program.Program) {
	t.Helper()
	if p.Module().Count() != 1 {
		t.Fatalf("ImportCount = %d, want one exact literal boundary", p.Module().Count())
	}
	imported, ok := p.Module().ImportAt(0)
	if !ok {
		t.Fatal("missing literal Import")
	}
	span, ok := p.Source().Identity().Span(imported.Term)
	if !ok || span.File != "boundaries.atomic.lua" || span.StartLine != 1 || span.EndLine != 1 {
		t.Fatalf("literal Import span = %#v/%v, want line 1 in boundaries.atomic.lua", span, ok)
	}
	if imported.Call == 0 || imported.Request == 0 || imported.Key == 0 || imported.Alias == 0 {
		t.Fatalf("Import = %#v, want direct Call/Request/Key/Alias", imported)
	}
	request, _, text, stringOK := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(imported.Request) - 1))
	if !stringOK || request != imported.Request || text != "pkg.core" {
		t.Fatalf("literal Import request = %q/%v, want pkg.core", text, stringOK)
	}
	key, keyOK := p.Source().Keys().Exact(imported.Key)
	if !keyOK || key.Kind != keyspace.LiteralString || key.String != "pkg.core" {
		t.Fatalf("literal Import module key = %#v/%v", key, keyOK)
	}
}

func assertNonliteralBoundaryCall(t *testing.T, p *program.Program) {
	t.Helper()
	if p.Module().Count() != 0 {
		t.Fatalf("nonliteral ImportCount = %d, want no static Import", p.Module().Count())
	}
	call, ok := p.Flow().Authored().Calls().At(0)
	if !ok || call == 0 {
		t.Fatal("nonliteral require lost its ordinary Call")
	}
	span, ok := p.Source().Identity().Span(call)
	if !ok || span.File != "boundaries.atomic.lua" || span.StartLine != 2 || span.EndLine != 2 {
		t.Fatalf("ordinary nonliteral Call span = %#v/%v, want line 2 in boundaries.atomic.lua", span, ok)
	}
}

// A direct literal local binding owns its Import alias. A nonliteral request
// remains an ordinary Call and does not enter the static Import census.
func TestSourceBoundaryLawDirectRequireAliasRequiresStaticLiteral(t *testing.T) {
	p := lowerBoundarySource(t, "local Module = require(\"pkg.core\")\nlocal request = \"runtime\"\nlocal Dynamic = require(request)\n")
	if p.Module().Count() != 1 {
		t.Fatalf("ImportCount = %d, want literal direct require only", p.Module().Count())
	}
	imported, ok := p.Module().ImportAt(0)
	if !ok {
		t.Fatal("missing literal Import")
	}
	if imported.Request == 0 || imported.Key == 0 || imported.Alias == 0 {
		t.Fatalf("literal Import = %#v, want Request/Key/Alias", imported)
	}
	request, _, text, stringOK := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(imported.Request) - 1))
	if !stringOK || request != imported.Request || text != "pkg.core" {
		t.Fatalf("literal Import request = %q/%v", text, stringOK)
	}
	if p.Flow().Authored().Calls().Count() < 2 {
		t.Fatalf("literal/nonliteral Calls = %d, want both ordinary call occurrences", p.Flow().Authored().Calls().Count())
	}
}

// A local declaration shadows the global name in lexical binding, so this is
// deliberately rejected as an Import even though its spelling is require.
func TestSourceBoundaryShadowedRequireIsRejected(t *testing.T) {
	const source = "local require = function(value) return value end\nlocal shadow = require(\"shadowed\")\n"
	call, binding := boundaryRequireAnchorAt(t, source, 2)
	if binding.IsImplicitGlobalUse(call.Func.(*ast.IdentExpr)) {
		t.Fatal("lexically shadowed require was accepted as an implicit global")
	}
	if global, ok := binding.GlobalIdentity(call.Func.(*ast.IdentExpr)); ok || global.Matches("require") {
		t.Fatal("lexically shadowed require fabricated the global identity")
	}
	if got := lowerBoundarySource(t, source).Module().Count(); got != 0 {
		t.Fatalf("shadowed require ImportCount = %d, want rejection", got)
	}
}

// Assignment to a global is ordinary Lua. It cannot retroactively alter the
// binder proof attached to this direct global use, but its nonliteral request
// is not static module evidence. The Program retains only its ordinary Call.
func TestSourceBoundaryGlobalRequireMutationRetainsOrdinaryCall(t *testing.T) {
	const source = "local request = \"runtime\"\nrequire = function(value) return value end\nlocal Dynamic = require(request)\n"
	call, binding := boundaryRequireAnchorAt(t, source, 3)
	ident := call.Func.(*ast.IdentExpr)
	if binding.IsImplicitGlobalUse(ident) {
		t.Fatal("assigned global require should resolve through its explicit global declaration")
	}
	global, ok := binding.GlobalIdentity(ident)
	if !ok || !global.Matches("require") {
		t.Fatal("post-assignment require did not retain its binder global identity")
	}

	p := lowerBoundarySource(t, source)
	if p.Module().Count() != 0 {
		t.Fatalf("ImportCount = %d, want no static Import for nonliteral request", p.Module().Count())
	}
	if p.Flow().Authored().Calls().Count() == 0 {
		t.Fatal("nonliteral require lost its ordinary Call")
	}
}

func lowerBoundarySource(t *testing.T, source string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "boundaries.atomic.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// boundaryRequireAnchor is intentionally specific to the boundary grammar
// form.  The unique (FuncCallExpr, line) anchor is checked before any Program
// is constructed, so a changed source shape cannot silently exercise another
// call on the same line.
func boundaryRequireAnchor(t *testing.T, sourceCase sourceCase) (*ast.FuncCallExpr, *bind.Result) {
	t.Helper()
	if sourceCase.Form != "FuncCallExpr" || sourceCase.Line <= 0 {
		t.Fatalf("boundary source case %s is not an anchored FuncCallExpr", sourceCase.ID)
	}
	return boundaryRequireAnchorAt(t, sourceCase.Source, sourceCase.Line)
}

func boundaryRequireAnchorAt(t *testing.T, source string, line int) (*ast.FuncCallExpr, *bind.Result) {
	t.Helper()
	statements, err := parse.ParseString(source, "boundaries.atomic.lua")
	if err != nil {
		t.Fatal(err)
	}
	var anchor *ast.FuncCallExpr
	for _, statement := range statements {
		local, ok := statement.(*ast.LocalAssignStmt)
		if !ok {
			continue
		}
		for _, expr := range local.Exprs {
			call, ok := expr.(*ast.FuncCallExpr)
			if !ok || call.Line() != line {
				continue
			}
			if anchor != nil {
				t.Fatalf("FuncCallExpr at line %d is not unique", line)
			}
			anchor = call
		}
	}
	if anchor == nil {
		t.Fatalf("missing FuncCallExpr anchor at line %d", line)
	}
	if _, ok := anchor.Func.(*ast.IdentExpr); !ok || len(anchor.Args) != 1 || anchor.Receiver != nil || anchor.Method != "" {
		t.Fatalf("anchor at line %d is not the direct one-argument require form", line)
	}
	return anchor, bind.BindChunk(statements)
}

func assertDirectGlobalRequire(t *testing.T, binding *bind.Result, call *ast.FuncCallExpr) {
	t.Helper()
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident.Value != "require" {
		t.Fatalf("call function = %#v, want identifier require", call.Func)
	}
	if !binding.IsImplicitGlobalUse(ident) {
		t.Fatal("direct require was not binder-proven as an implicit global use")
	}
	global, ok := binding.GlobalIdentity(ident)
	if !ok || !global.Matches("require") {
		t.Fatal("direct require did not retain the binder global identity")
	}
}

func TestSourceModuleVerticalAcceptsOnlyStaticBinderProvenGlobalRequire(t *testing.T) {
	p := parseBindLower(t, `local M = require("pkg.core")
local name = "pkg.dynamic"
local D = require(name)
local require = function(value) return value end
local shadow = require("shadowed")
return M, D, shadow`)
	if p.Module().Count() != 1 {
		t.Fatalf("ImportCount = %d, want literal global require only", p.Module().Count())
	}
	imported, ok := p.Module().ImportAt(0)
	if !ok {
		t.Fatal("missing literal Import")
	}
	if imported.Call == 0 || imported.Request == 0 || imported.Key == 0 || imported.Alias == 0 {
		t.Fatalf("literal Import = %#v, want direct Call/Request/Key/Alias", imported)
	}
	request, _, text, stringOK := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(imported.Request) - 1))
	if !stringOK || request != imported.Request || text != "pkg.core" {
		t.Fatalf("literal Import request = %q/%v", text, stringOK)
	}
	value, keyOK := p.Source().Keys().Exact(imported.Key)
	if !keyOK || value.String != "pkg.core" {
		t.Fatalf("literal Import key = %#v/%v", value, keyOK)
	}
	if p.Flow().Authored().Calls().Count() < 3 {
		t.Fatalf("require occurrences = %d, want ordinary dynamic/shadowed Calls retained", p.Flow().Authored().Calls().Count())
	}
}

func TestSourceModuleVerticalDoesNotAliasNonDirectBindings(t *testing.T) {
	p := parseBindLower(t, `local A, B = require("pair"), 1
local C
C = require("assigned")
return A, B, C`)
	if p.Module().Count() != 2 {
		t.Fatalf("ImportCount = %d, want both direct global calls", p.Module().Count())
	}
	for index := 0; index < p.Module().Count(); index++ {
		imported, ok := p.Module().ImportAt(index)
		if !ok {
			t.Fatalf("missing Import %d", index)
		}
		if imported.Alias != 0 {
			t.Fatalf("non-single direct local binding fabricated alias %v", imported.Alias)
		}
	}
}

func TestSourceModuleVerticalRetainsRejectedRequireShapesAsOrdinaryCalls(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "dynamic", source: "local request = \"dynamic\"\nrequire(request)"},
		{name: "zero arguments", source: "require()"},
		{name: "multiple arguments", source: "require(\"first\", \"second\")"},
		{name: "empty literal", source: "require(\"\")"},
		{name: "member", source: "_G.require(\"member\")"},
		{name: "call alias", source: "local alias = require\nalias(\"alias\")"},
		{name: "shadowed local", source: "local require = function() end\nrequire(\"shadowed\")"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			if p.Module().Count() != 0 {
				t.Fatalf("Import count = %d, want zero", p.Module().Count())
			}
			if got := p.Flow().Authored().Calls().Count(); got != 1 {
				t.Fatalf("ordinary Call count = %d, want one", got)
			}
		})
	}
}

func TestSourceModuleVerticalRetainsStaticTypeofRequireImport(t *testing.T) {
	p := parseBindLower(t, `type Snapshot = typeof(require("static"))`)
	if p.Module().Count() != 1 {
		t.Fatalf("Import count = %d, want one static Import", p.Module().Count())
	}
	row, ok := p.Module().ImportAt(0)
	if !ok || row.Request == 0 || row.Call == 0 {
		t.Fatalf("static Import = %#v/%v, want authored Request and Call", row, ok)
	}
	if !p.Flow().Containment().Static(row.Call) || p.Flow().Executable().Contains(row.Call) {
		t.Fatalf("static require Call = %v/%v, want static and non-executable", row.Call, p.Flow().Executable().Contains(row.Call))
	}
}

func TestSourceModuleImportsUseStableFinalSourceOrderSlots(t *testing.T) {
	original := parseBindLower(t, `return require("first"), require("second")`)
	replayed := parseBindLower(t, `return require("first"), require("second")`)
	permuted := parseBindLower(t, `return require("second"), require("first")`)

	termsAndRequests := func(p *program.Program) ([]keyspace.Term, []string) {
		module := p.Module()
		terms := make([]keyspace.Term, module.Count())
		requests := make([]string, module.Count())
		for index := range terms {
			imported, ok := module.ImportAt(index)
			if !ok {
				t.Fatalf("missing Import %d", index)
			}
			if imported.Request == 0 {
				t.Fatalf("Import %d has no literal request", index)
			}
			request, _, text, stringOK := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(imported.Request) - 1))
			if !stringOK || request != imported.Request {
				t.Fatalf("Import %d request %v is not String", index, imported.Request)
			}
			terms[index], requests[index] = imported.Term, text
		}
		return terms, requests
	}

	originalTerms, originalRequests := termsAndRequests(original)
	replayedTerms, replayedRequests := termsAndRequests(replayed)
	permutedTerms, permutedRequests := termsAndRequests(permuted)
	for index := range originalTerms {
		if originalTerms[index] != replayedTerms[index] || originalTerms[index] != permutedTerms[index] {
			t.Fatalf("Import slot %d = %v/%v/%v, want stable final dense identity", index, originalTerms[index], replayedTerms[index], permutedTerms[index])
		}
	}
	if got, want := originalRequests, []string{"first", "second"}; !slices.Equal(got, want) {
		t.Fatalf("original request order = %#v, want %#v", got, want)
	}
	if !slices.Equal(replayedRequests, originalRequests) {
		t.Fatalf("replayed request order = %#v, want %#v", replayedRequests, originalRequests)
	}
	if got, want := permutedRequests, []string{"second", "first"}; !slices.Equal(got, want) {
		t.Fatalf("permuted request order = %#v, want %#v", got, want)
	}
}
