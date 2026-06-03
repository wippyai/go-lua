package modules_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TestZZExportProbeSynthOnly inspects modules.ExportType (synth-based) output
// before the FunctionFacts export projection, to isolate where returns drop.
func TestZZExportProbeSynthOnly(t *testing.T) {
	src := `
type Entry = { id: string, data: {[string]: any} }
local pages = {}
local function qualify_id(entry_id, relative_id)
    return entry_id .. ":" .. relative_id
end
function pages.build_page(entry: Entry)
    local data_func = entry.data.data_func
    if data_func and data_func ~= "" then
        data_func = qualify_id(entry.id, data_func)
    end
    local page = {}
    page.data_func = data_func
    return page
end
return pages
`
	mod := testutil.CheckAndExport(src, "page_registry")
	sess := mod.Session
	raw := modules.ExportType(sess.RootResultValue(), nil)
	t.Logf("modules.ExportType (synth-only) = %s", raw)
}

// zzExportProbe inspects what an exported function field carries (return vector +
// ErrorReturn label). Diagnostic only.
func TestZZExportProbeDeclaredReturnErrorReturn(t *testing.T) {
	src := `
local M = {}

type Response = {
    metadata: {
        response_id: string,
    },
}

function M.request(ok: boolean): (Response?, string?)
    if ok then
        return { metadata = { response_id = "resp-123" } }, nil
    end
    return nil, "failed"
end

return M
`
	mod := testutil.CheckAndExport(src, "client")
	// Inspect raw Export (pre-summary-enrichment) ErrorReturn too.
	if rawRec, ok := unwrap.Alias(mod.Manifest.Export).(*typ.Record); ok {
		if rf := rawRec.GetField("request"); rf != nil {
			if rfn := unwrap.Function(rf.Type); rfn != nil {
				t.Logf("RAW request = %s errRet=%v", rfn, erreffect.HasErrorReturnLabel(rfn))
			}
		}
	}
	export := unwrap.Alias(mod.Manifest.EnrichedExport())
	rec, ok := export.(*typ.Record)
	if !ok {
		t.Fatalf("export not a record: %T -> %s", export, export)
	}
	f := rec.GetField("request")
	if f == nil {
		t.Fatalf("no request field; export=%s", export)
	}
	fn := unwrap.Function(f.Type)
	if fn == nil {
		t.Fatalf("request not a function: %s", f.Type)
	}
	t.Logf("request fn = %s", fn)
	t.Logf("HasErrorReturnLabel = %v", erreffect.HasErrorReturnLabel(fn))
}

func TestZZExportProbeInferredReturn(t *testing.T) {
	src := `
type Entry = {
    id: string,
    data: {[string]: any},
}

local pages = {}

local function qualify_id(entry_id, relative_id)
    return entry_id .. ":" .. relative_id
end

function pages.build_page(entry: Entry)
    local data_func = entry.data.data_func
    if data_func and data_func ~= "" then
        data_func = qualify_id(entry.id, data_func)
    end

    local page = {}
    page.data_func = data_func
    return page
end

return pages
`
	mod := testutil.CheckAndExport(src, "page_registry")
	export := unwrap.Alias(mod.Manifest.EnrichedExport())
	rec, ok := export.(*typ.Record)
	if !ok {
		t.Fatalf("export not a record: %T -> %s", export, export)
	}
	f := rec.GetField("build_page")
	if f == nil {
		t.Fatalf("no build_page field; export=%s", export)
	}
	fn := unwrap.Function(f.Type)
	if fn == nil {
		t.Fatalf("build_page not a function: %s", f.Type)
	}
	t.Logf("build_page fn = %s", fn)
	for i, r := range fn.Returns {
		t.Logf("  return[%d] = %s", i, r)
	}
}
