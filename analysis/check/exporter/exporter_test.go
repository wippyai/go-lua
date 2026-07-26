package exporter_test

import (
	"go/build"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/exporter"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPackageDoesNotImportCompilerSyntax(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not locate exporter test")
	}
	pkg, err := build.Default.ImportDir(filepath.Dir(filename), 0)
	if err != nil {
		t.Fatalf("inspect exporter imports: %v", err)
	}
	forbidden := map[string]bool{
		"github.com/wippyai/go-lua/compiler/ast":   true,
		"github.com/wippyai/go-lua/compiler/parse": true,
	}
	for _, imported := range pkg.Imports {
		if forbidden[imported] {
			t.Fatalf("exporter production package imports forbidden syntax package %q", imported)
		}
	}
}

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

func TestDeriveSummaryPublishesEvaluatedReturnTemplates(t *testing.T) {
	source := `local M = {}
local function identity(value: string) return value end
M.identity = identity
function M.make() return { id = "p1", nested = { theme = "dark" } } end
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	summary := exporter.DeriveSummary(result)
	identity, ok := summary.Function("identity", 1)
	if !ok || identity.Return.Parameter == nil || *identity.Return.Parameter != 0 {
		t.Fatalf("export = %v; identity relation = %#v, want fact-derived parameter return", summary.Type, identity)
	}
	make, found := summary.Function("make", 0)
	if !found || len(make.Return.Table) != 2 || !make.Return.Closed() {
		t.Fatalf("literal relation = %#v, want evaluated closed table return", make)
	}
}

func TestDeriveSummaryPublishesFactDerivedParameterReturn(t *testing.T) {
	source := `local M = {}
function M.id(value: table): table
  return value
end
return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	summary := exporter.DeriveSummary(result)
	identity, found := summary.Function("id", 1)
	if !found || identity.Return.Parameter == nil || *identity.Return.Parameter != 0 {
		t.Fatalf("id relation = %#v, want engine-proven return of parameter 0", identity)
	}
}

func TestDeriveSummaryPublishesFactDerivedFreshTableTemplate(t *testing.T) {
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
	summary := exporter.DeriveSummary(result)
	make, found := summary.Function("make", 1)
	if !found || len(make.Return.Table) != 2 || make.Return.Table[0].Value.Parameter == nil || *make.Return.Table[0].Value.Parameter != 0 {
		t.Fatalf("make relation = %#v, want evaluated table template with parameter origin", make)
	}
	for _, function := range summary.Functions {
		if function.Path == "forward" && len(function.Return.Table) != 0 {
			t.Fatalf("forward relation = %#v, want no imported forwarding template without a published forwarding fact", function)
		}
	}
}

func TestDeriveSummaryWithholdsImportedCompositionWithoutFact(t *testing.T) {
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
	summary := exporter.DeriveSummary(result)
	for _, function := range summary.Functions {
		if function.Path == "new_source" && function.Return.Valid(function.Arity) {
			t.Fatalf("new_source relation = %#v, want no imported composition without a published forwarding fact", function)
		}
	}
}

// TestDeriveSummaryPublishesFactDerivedStoreOwner proves the exported store
// relation reads its formal positions from the engine's per-parameter escape
// summary. The positional wrapper's owner comes from the placement facts that
// name the container of the store; a wrapper that writes into module-captured
// state has no owner formal and publishes an escaping root instead.
func TestDeriveSummaryPublishesFactDerivedStoreOwner(t *testing.T) {
	source := `type Item = { id: string }
type Box = { label: string }
local M = {}
local saved: {[string]: Item} = {}

function M.store_item(item: Item, box: Box)
    ownership.store(item, box)
end

function M.keep_item(item: Item)
    saved.last = item
end

return M`
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	summary := exporter.DeriveSummary(result)

	store, ok := summary.Function("store_item", 2)
	if !ok || !store.Store.Valid(2) {
		t.Fatalf("store_item relation = %#v, want a valid store", store)
	}
	if store.Store.EscapingRoot || store.Store.Value != 0 || store.Store.Owner != 1 {
		t.Fatalf("store_item store = %#v, want the fact-derived owner formal at 1", store.Store)
	}

	keep, ok := summary.Function("keep_item", 1)
	if !ok || !keep.Store.Valid(1) {
		t.Fatalf("keep_item relation = %#v, want a valid store", keep)
	}
	if !keep.Store.EscapingRoot || keep.Store.Value != 0 {
		t.Fatalf("keep_item store = %#v, want an escaping root at formal 0", keep.Store)
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
	summary := exporter.DeriveSummary(result)
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
	if _, found := exporter.DeriveSummary(result).Function("store_item", 2); found {
		t.Fatal("stale ownership alias relation escaped its replaced module root")
	}
}
