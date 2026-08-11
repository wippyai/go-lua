package lower_test

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
)

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
