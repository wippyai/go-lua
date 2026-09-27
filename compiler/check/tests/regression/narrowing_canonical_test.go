package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAssertReturnNarrowsFalsyUnionThroughChecker(t *testing.T) {
	result := testutil.Check(`
local function maybe(): string | false | nil
	return "ready"
end
local value: string = assert(maybe())
local invalid: number = value
`, testutil.WithStdlib())
	if len(result.Errors) != 1 {
		t.Fatalf("want only the invalid number assignment diagnosed, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNarrowedExpansionPadsOpenCallWithAnyThroughChecker(t *testing.T) {
	result := testutil.Check(`
local function capture(callee: any)
	local first, second, third = callee()
	return second
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected diagnostics: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	var found bool
	for fn, fnResult := range result.Session.Results {
		if fn == nil || len(fn.ParList.Names) != 1 || fn.ParList.Names[0] != "callee" {
			continue
		}
		fnResult.Graph.EachAssign(func(p cfg.Point, assign *cfg.AssignInfo) {
			if len(assign.Targets) != 3 || len(assign.Sources) != 1 {
				return
			}
			values := fnResult.NarrowSynth.ExpandValues(assign.Sources, 3, p)
			if len(values) != 3 || !typ.IsAny(values[2]) {
				t.Errorf("open call should pad third value with any, got %v", values)
			}
			found = true
		})
	}
	if !found {
		t.Fatal("checker did not analyze capture assignment")
	}
}

func TestModuleAliasResolutionPrefersExportOverPlaceholderThroughChecker(t *testing.T) {
	manifest := io.NewManifest("narrow_alias")
	manifest.SetExport(typ.NewRecord().Field("value", typ.Number).Build())
	result := testutil.Check(`
local imported = require("narrow_alias")
local good: number = imported.value
local bad: string = imported.value
`, testutil.WithStdlib(), testutil.WithManifest("narrow_alias", manifest))
	if len(result.Errors) != 1 {
		t.Fatalf("want only the invalid string assignment diagnosed, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
