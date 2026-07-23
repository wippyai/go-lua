package exporter_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/exporter"
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
