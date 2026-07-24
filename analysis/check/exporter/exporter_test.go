package exporter_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/exporter"
	"github.com/wippyai/go-lua/analysis/module/exportrelation"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDeriveProjectsClosedReturnShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		check  func(*testing.T, typ.Type)
	}{
		{name: "scalar", source: `return 42`, check: func(t *testing.T, got typ.Type) {
			if got.String() != "42" {
				t.Fatalf("export = %s, want literal 42", got)
			}
		}},
		{name: "record and nested record", source: `return {name = "service", config = {retries = 3}}`, check: func(t *testing.T, got typ.Type) {
			record, ok := got.(*typ.Record)
			if !ok || record.GetField("name") == nil || record.GetField("config") == nil || !record.Open {
				t.Fatalf("export = %T %[1]v, want open record with proven fields", got)
			}
			config, ok := record.GetField("config").Type.(*typ.Record)
			if !ok || config.GetField("retries") == nil || config.GetField("retries").Type.String() != "3" {
				t.Fatalf("nested config = %#v, want retries: 3", record.GetField("config").Type)
			}
		}},
		{name: "callable", source: `local function format(value: string): number return 1 end
return format`, check: func(t *testing.T, got typ.Type) {
			if _, ok := got.(*typ.Function); !ok {
				t.Fatalf("export = %T %[1]v, want callable", got)
			}
		}},
		{name: "opaque result stays unknown", source: `return provider()`, check: func(t *testing.T, got typ.Type) {
			if got != typ.Unknown {
				t.Fatalf("export = %s, want unknown", got)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := engine.Check(test.source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			test.check(t, exporter.Derive(result))
		})
	}
}

func TestDeriveRetainsProvenStaticWritesOnReturnedModuleTable(t *testing.T) {
	result, err := engine.Check(`
local M = {}
function M.format(value: string): number return 1 end
M.version = "v1"
return M`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	record, ok := exporter.Derive(result).(*typ.Record)
	if !ok {
		t.Fatalf("export = %T, want record", exporter.Derive(result))
	}
	if record.GetField("format") == nil || record.GetField("version") == nil || record.GetField("version").Type.String() != `"v1"` {
		t.Fatalf("export = %v, want static module fields", record)
	}
}

func TestDeriveRetainsRecursiveCallableSignatureOnReturnedModuleTable(t *testing.T) {
	result, err := engine.Check(`
type Node = { next: Node? }
local M = {}
function M.new(): Node return { next = nil } end
return M`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	record, ok := exporter.Derive(result).(*typ.Record)
	if !ok || record.GetField("new") == nil {
		t.Fatalf("export = %T %[1]v, want module new field", exporter.Derive(result))
	}
	function, ok := record.GetField("new").Type.(*typ.Function)
	if !ok || len(function.Returns) != 1 || function.Returns[0] == typ.Unknown {
		t.Fatalf("new export = %T %[1]v, want recursive callable signature", record.GetField("new").Type)
	}
}

func TestDeriveUnionsReachableReturnCandidates(t *testing.T) {
	result, err := engine.Check("for i = 1, 1 do return \"loop\" end\nreturn 2")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	got := exporter.Derive(result).String()
	if got != `2 | "loop"` && got != `"loop" | 2` {
		t.Fatalf("export = %s, want union of return candidates", got)
	}
}

func TestDeriveRejectsARecordShapeInvalidatedByUnknownIndexMutation(t *testing.T) {
	result, err := engine.Check("local M = {version = \"v1\"}\nM[provider()] = nil\nreturn M")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got := exporter.Derive(result); got != typ.Unknown {
		t.Fatalf("export = %T %[1]v, want unknown after unknown mutation", got)
	}
}

func TestDeriveSummaryPublishesSealedMemberReturnTemplates(t *testing.T) {
	source := `local M = {}
local function identity(value: string) return value end
M.identity = identity
function M.make() return { id = "p1", nested = { theme = "dark" } } end
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	summary := exporter.DeriveSummary(result, source)
	identity, ok := summary.Function("identity", 1)
	if !ok || identity.Return.Parameter == nil || *identity.Return.Parameter != 0 || !identity.Valid() {
		t.Fatalf("export = %v; identity relation = %#v", summary.Type, identity)
	}
	make, ok := summary.Function("make", 0)
	if !ok || len(make.Return.Table) != 2 || !make.Valid() {
		t.Fatalf("export = %v; literal relation = %#v", summary.Type, make)
	}
}

func TestDeriveSummaryPublishesDirectParameterReturnTemplate(t *testing.T) {
	source := `local M = {}
function M.id(value: table): table
  return value
end
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	summary := exporter.DeriveSummary(result, source)
	identity, ok := summary.Function("id", 1)
	if !ok || identity.Return.Parameter == nil || *identity.Return.Parameter != 0 {
		t.Fatalf("id relation = %#v, want parameter 0", identity)
	}
}

func TestDeriveSummaryWithImportsPreservesPublishedFreshTableRelation(t *testing.T) {
	source := `local upstream = require("upstream")
local M = {}
function M.make(id: string)
  local packet = { id = id, meta = { route = "worker" } }
  return packet
end
function M.forward(id: string) return upstream.make(id) end
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	parameter := 0
	upstream := exportrelation.Summary{Type: typ.Unknown, Functions: []exportrelation.Function{{
		Path:  "make",
		Arity: 1,
		Return: exportrelation.Value{Table: []exportrelation.Member{
			{Suffix: ".id", Value: exportrelation.Value{Parameter: &parameter}},
			{Suffix: ".meta", Value: exportrelation.Value{Table: []exportrelation.Member{{Suffix: ".route", Value: exportrelation.Value{Scalar: `scalar/string/"worker"`}}}}},
		}},
	}}}
	summary := exporter.DeriveSummaryWithImports(result, source, map[string]exportrelation.Summary{"upstream": upstream}, map[string]string{"upstream": "upstream"})
	for _, name := range []string{"make", "forward"} {
		function, ok := summary.Function(name, 1)
		if !ok || len(function.Return.Table) != 2 || len(function.Return.Table[1].Value.Table) != 1 {
			t.Fatalf("%s relation = %#v, want closed nested table witness", name, function)
		}
	}
}

func TestDeriveSummaryWithImportsComposesTableArgumentRelation(t *testing.T) {
	source := `local protocol = require("protocol")
type Source = { messages: string, ticks: number }
type SourceBox = { value: Source }
local M = {}
function M.new_source(messages: string, ticks: number): SourceBox
  return protocol.box_source({ messages = messages, ticks = ticks })
end
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	parameter := 0
	upstream := exportrelation.Summary{Type: typ.Unknown, Functions: []exportrelation.Function{{
		Path: "box_source", Arity: 1,
		Return: exportrelation.Value{Table: []exportrelation.Member{{Suffix: ".value", Value: exportrelation.Value{Parameter: &parameter}}}},
	}}}
	summary := exporter.DeriveSummaryWithImports(result, source, map[string]exportrelation.Summary{"protocol": upstream}, map[string]string{"protocol": "protocol"})
	function, ok := summary.Function("new_source", 2)
	if !ok || len(function.Return.Table) != 1 || len(function.Return.Table[0].Value.Table) != 2 {
		t.Fatalf("new_source relation = %#v, want composed boxed parameter table", function)
	}
	messages := function.Return.Table[0].Value.Table[0]
	ticks := function.Return.Table[0].Value.Table[1]
	if messages.Suffix != ".messages" || messages.Value.Parameter == nil || *messages.Value.Parameter != 0 ||
		ticks.Suffix != ".ticks" || ticks.Value.Parameter == nil || *ticks.Value.Parameter != 1 {
		t.Fatalf("composed members = %#v, want messages/ticks parameter witnesses", function.Return.Table[0].Value.Table)
	}
}

func TestDeriveSummaryPublishesOwnershipStoreAlias(t *testing.T) {
	source := `local M = {}
local store_item = ownership.store
M.store_item = store_item
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	summary := exporter.DeriveSummary(result, source)
	store, ok := summary.Function("store_item", 2)
	if !ok || !store.Store.Valid(2) || store.Store.Value != 0 || store.Store.Owner != 1 {
		t.Fatalf("store relation = %#v, want published ownership alias", store)
	}
}

func TestDeriveSummaryRejectsStaleOwnershipStoreAlias(t *testing.T) {
	source := `local M = {}
local store_item = ownership.store
M.store_item = store_item
M = {}
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, found := exporter.DeriveSummary(result, source).Function("store_item", 2); found {
		t.Fatal("stale ownership alias relation escaped its replaced module root")
	}
}
