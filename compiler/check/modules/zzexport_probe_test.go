package modules_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
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
	for _, tc := range []struct {
		name string
		opts []testutil.Option
	}{
		{"canonical", []testutil.Option{testutil.WithCheckOption(check.WithCanonicalFlow())}},
		{"legacy", nil},
	} {
		mod := testutil.CheckAndExport(src, "page_registry", tc.opts...)
		sess := mod.Session
		raw := modules.ExportType(sess.RootResultValue(), nil)
		t.Logf("[%s] modules.ExportType (synth-only) = %s", tc.name, raw)
	}
}

// zzExportProbe inspects what an exported function field carries (return vector +
// ErrorReturn label) under the canonical flow. Diagnostic only.
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
	for _, tc := range []struct {
		name string
		opts []testutil.Option
	}{
		{"canonical", []testutil.Option{testutil.WithCheckOption(check.WithCanonicalFlow())}},
		{"legacy", nil},
	} {
		mod := testutil.CheckAndExport(src, "client", tc.opts...)
		// Inspect raw Export (pre-summary-enrichment) ErrorReturn too.
		if rawRec, ok := unwrap.Alias(mod.Manifest.Export).(*typ.Record); ok {
			if rf := rawRec.GetField("request"); rf != nil {
				if rfn := unwrap.Function(rf.Type); rfn != nil {
					t.Logf("[%s] RAW request = %s errRet=%v", tc.name, rfn, erreffect.HasErrorReturnLabel(rfn))
				}
			}
		}
		export := unwrap.Alias(mod.Manifest.EnrichedExport())
		rec, ok := export.(*typ.Record)
		if !ok {
			t.Fatalf("[%s] export not a record: %T -> %s", tc.name, export, export)
		}
		f := rec.GetField("request")
		if f == nil {
			t.Fatalf("[%s] no request field; export=%s", tc.name, export)
		}
		fn := unwrap.Function(f.Type)
		if fn == nil {
			t.Fatalf("[%s] request not a function: %s", tc.name, f.Type)
		}
		t.Logf("[%s] request fn = %s", tc.name, fn)
		t.Logf("[%s] HasErrorReturnLabel = %v", tc.name, erreffect.HasErrorReturnLabel(fn))
	}
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
	for _, tc := range []struct {
		name string
		opts []testutil.Option
	}{
		{"canonical", []testutil.Option{testutil.WithCheckOption(check.WithCanonicalFlow())}},
		{"legacy", nil},
	} {
		mod := testutil.CheckAndExport(src, "page_registry", tc.opts...)
		export := unwrap.Alias(mod.Manifest.EnrichedExport())
		rec, ok := export.(*typ.Record)
		if !ok {
			t.Fatalf("[%s] export not a record: %T -> %s", tc.name, export, export)
		}
		f := rec.GetField("build_page")
		if f == nil {
			t.Fatalf("[%s] no build_page field; export=%s", tc.name, export)
		}
		fn := unwrap.Function(f.Type)
		if fn == nil {
			t.Fatalf("[%s] build_page not a function: %s", tc.name, f.Type)
		}
		t.Logf("[%s] build_page fn = %s", tc.name, fn)
		for i, r := range fn.Returns {
			t.Logf("[%s]   return[%d] = %s", tc.name, i, r)
		}
	}
}
