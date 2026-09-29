package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestModuleAliasTypeInFunctionSignature covers a local function whose
// annotations name a module type through a module alias declared in the
// enclosing chunk (local automation_types = require("types")). The alias
// resolves inside the function's return inference, so the function's inferred
// signature carries the module type and its results index and return as it.
func TestModuleAliasTypeInFunctionSignature(t *testing.T) {
	mapType := typ.NewAlias("Map", typ.NewMap(typ.String, typ.Any))
	typesManifest := io.NewManifest("types")
	typesManifest.DefineType("Map", mapType)
	typesManifest.SetExport(typ.NewRecord().Build())

	source := `
		local automation_types = require("types")

		local function decode_map(raw: any): automation_types.Map
			if type(raw) == "table" then return raw :: automation_types.Map end
			return {}
		end

		local function stored_ref(raw: any): automation_types.Map
			local ref = decode_map(raw)
			local kind = ref.kind
			return ref
		end

		local function collect(rows: {any}): ({automation_types.Map}?, string?)
			local out: {automation_types.Map} = {}
			for _, row in ipairs(rows) do
				out[#out + 1] = decode_map(row)
			end
			return out, nil
		end

		return { stored_ref = stored_ref, collect = collect }
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("types", typesManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Errorf("line %d: %s", d.Position.Line, d.Message)
		}
	}

	export := result.Session.ExportType()
	if export == nil {
		t.Fatal("expected a module export type")
	}
	typ.Rewrite(export, func(node typ.Type) (typ.Type, bool) {
		if ref, ok := node.(*typ.Ref); ok {
			t.Errorf("export %s holds unresolved reference %s.%s", typ.FormatShort(export), ref.Module, ref.Name)
		}
		return nil, false
	})
}
