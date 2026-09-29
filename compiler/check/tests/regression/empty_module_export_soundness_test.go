package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEmptyModuleExportAbsentCall(t *testing.T) {
	module := testutil.CheckAndExport(`return {}`, "empty", testutil.WithStdlib())
	if module.HasError() {
		t.Fatal(testutil.ErrorMessages(module.Errors))
	}
	record, ok := module.Manifest.Export.(*typ.Record)
	if !ok || record.Open || !record.Complete || len(record.Fields) != 0 {
		t.Fatalf("empty module export = %v, want closed complete empty record", module.Manifest.Export)
	}
	encoded, err := io.EncodeManifest(module.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatal(err)
	}
	for _, strict := range []bool{false, true} {
		result := testutil.Check(`local empty = require("empty"); empty.normalize()`, testutil.WithStdlib(), testutil.WithManifest("empty", decoded), testutil.WithCheckOptions(check.Options{Strict: strict}))
		if messages := strings.Join(testutil.ErrorMessages(result.Errors), "; "); !strings.Contains(messages, "expected function, got nil") {
			t.Fatalf("strict=%v: %s", strict, messages)
		}
	}
}

func TestSchemaOpenEmptyRecordKeepsUnknownExtraField(t *testing.T) {
	record := typ.NewRecord().SetOpen(true).Build()
	got, ok := core.Index(record, typ.LiteralString("normalize"))
	if !ok || got != typ.Unknown {
		t.Fatalf("schema-open extra field = %v, %v; want unknown", got, ok)
	}
}

func TestModuleExportRetainsPostConstructorFields(t *testing.T) {
	for _, source := range []string{
		`local M = {}; M.run = function() return 1 end; return M`,
		`local M = {}
local function extend()
 M.run = function() return 1 end
end
extend()
return M`,
	} {
		module := testutil.CheckAndExport(source, "extended", testutil.WithStdlib())
		if module.HasError() {
			t.Fatalf("export %s: %v", source, testutil.ErrorMessages(module.Errors))
		}
		for _, strict := range []bool{false, true} {
			result := testutil.Check(`local m = require("extended"); local n: number = m.run()`, testutil.WithStdlib(), testutil.WithModule("extended", module), testutil.WithCheckOptions(check.Options{Strict: strict}))
			if result.HasError() {
				t.Fatalf("strict=%v source=%s: %v", strict, source, testutil.ErrorMessages(result.Errors))
			}
		}
	}
}
