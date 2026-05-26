package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression guard: nested calls used as arguments must still contribute
// parameter evidence to local helper functions.
func TestNestedCall_ParameterEvidenceFlowIntoLocalHelper(t *testing.T) {
	source := `
		type Entry = { id: string, kind: string }

		local function extract(entry)
			return {
				id = entry.id,
				kind = entry.kind,
			}
		end

		local entries: {Entry} = {
			{ id = "a", kind = "k" },
		}

		local out = {}
		for _, entry in ipairs(entries) do
			table.insert(out, extract(entry))
		end

		for _, item in ipairs(out) do
			local id: string = item.id
			local kind: string = item.kind
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNestedCallback_CapturesRefinedParameterEvidence(t *testing.T) {
	funcsManifest := typeio.NewManifest("funcs")
	funcsManifest.SetExport(typ.NewRecord().
		Field("call", typ.Func().Param("name", typ.String).Returns(typ.Any).Build()).
		Build())
	source := `
		type Entry = { id: string, kind: string }

		local funcs = require("funcs")

		local function run(entry)
			return pcall(function()
				return funcs.call(entry.id)
			end)
		end

		local entries: {Entry} = {
			{ id = "a", kind = "k" },
		}

		for _, entry in ipairs(entries) do
			run(entry)
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("funcs", funcsManifest))
	if result.HasError() {
		t.Fatalf("expected callback capture to preserve refined entry type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
