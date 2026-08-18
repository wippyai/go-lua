package module

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/parse"
)

func requireEvidence(binding *bind.Result) []bind.DirectGlobalCall {
	var evidence []bind.DirectGlobalCall
	for _, occurrence := range binding.DirectGlobalCalls() {
		if occurrence.Global.Matches("require") {
			evidence = append(evidence, occurrence)
		}
	}
	return evidence
}

func TestBuildCensusStaticImportShapeLaw(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		direct  int
		imports int
	}{
		{name: "literal", source: `require("runtime")`, direct: 1, imports: 1},
		{name: "typeof literal", source: `type ImportType = typeof(require("static"))`, direct: 1, imports: 1},
		{name: "dynamic/nonliteral", source: "local request = \"dynamic\"\nrequire(request)", direct: 1},
		{name: "missing", source: `require()`, direct: 1},
		{name: "extra", source: `require("first", "second")`, direct: 1},
		{name: "empty", source: `require("")`, direct: 1},
		{name: "member", source: `_G.require("member")`},
		{name: "alias", source: "local alias = require\nalias(\"aliased\")"},
		{name: "shadowed", source: "do\n  local require = function() end\n  require(\"shadowed\")\nend"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements, err := parse.ParseString(test.source, "module-census-shape.lua")
			if err != nil {
				t.Fatal(err)
			}
			binding := bind.BindChunk(statements)
			evidence := requireEvidence(binding)
			if got := len(evidence); got != test.direct {
				t.Fatalf("DirectGlobalCalls(require) = %d, want %d", got, test.direct)
			}
			census, err := BuildCensus(binding)
			if err != nil {
				t.Fatal(err)
			}
			if got := census.Count(); got != test.imports {
				t.Fatalf("Import census count = %d, want %d", got, test.imports)
			}
			for _, occurrence := range evidence {
				_, admitted := census.Ordinal(occurrence.Call)
				if admitted != (test.imports == 1) {
					t.Fatalf("require evidence admitted = %v, want %v", admitted, test.imports == 1)
				}
			}
		})
	}
}

func TestBuildCensusSelectsOnlyStaticBinderProvenGlobalRequire(t *testing.T) {
	statements, err := parse.ParseString(`
require("runtime")
type ImportType = typeof(require("static"))
local request = "dynamic"
require(request)
require()
require("first", "second")
require("")
local alias = require
alias("aliased")
_G.require("member")
do
  local require = function() end
  require("shadowed")
end
`, "module-census.lua")
	if err != nil {
		t.Fatal(err)
	}
	binding := bind.BindChunk(statements)
	census, err := BuildCensus(binding)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := census.Count(), 2; got != want {
		t.Fatalf("census count = %d, want %d", got, want)
	}

	var requires []bind.DirectGlobalCall
	for _, occurrence := range binding.DirectGlobalCalls() {
		if occurrence.Global.Matches("require") {
			requires = append(requires, occurrence)
		}
	}
	if got, want := len(requires), 6; got != want {
		t.Fatalf("binder require evidence = %d, want %d", got, want)
	}
	for index, occurrence := range requires[:2] {
		ordinal, ok := census.Ordinal(occurrence.Call)
		if !ok || ordinal != index {
			t.Fatalf("census ordinal[%d] = %d/%v, want %d/true", index, ordinal, ok, index)
		}
	}
	for index, occurrence := range requires[2:] {
		if ordinal, ok := census.Ordinal(occurrence.Call); ok {
			t.Fatalf("non-static require[%d] received ordinal %d", index+2, ordinal)
		}
	}
}
