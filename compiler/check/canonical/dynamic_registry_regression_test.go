package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestDynamicRegistryRendererGuardRegression(t *testing.T) {
	pageRegistry := `
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
	main := `
local page_registry = require("page_registry")
local function takes_string(name: string)
    return name
end
local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end
    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end
local page = page_registry.build_page({ id = "demo", data = { data_func = "load_data" } })
return get_page_data(page)
`
	mod := testutil.CheckAndExport(pageRegistry, "page_registry", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	res := testutil.Check(main, testutil.WithStdlib(), testutil.WithModule("page_registry", mod), testutil.WithCheckOption(check.WithCanonicalFlow()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected dynamic registry renderer guard to be clean, got diagnostics: %v", msgs)
	}
}

func TestDynamicRegistryExportProjectionRegression(t *testing.T) {
	pageRegistry := `
type Entry = { id: string, data: {[string]: any} }
local pages = {}
function pages.build_page(entry: Entry)
    local page = {}
    page.data_func = entry.data.data_func
    return page
end
return pages
`
	mod := testutil.CheckAndExport(pageRegistry, "page_registry", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	rec := unwrap.Record(mod.Manifest.EnrichedExport())
	if rec == nil {
		t.Fatalf("expected record export, got %v", mod.Manifest.EnrichedExport())
	}
	field := rec.GetField("build_page")
	if field == nil {
		t.Fatalf("expected build_page export in %v", rec)
	}
	fn := unwrap.Function(field.Type)
	if fn == nil || len(fn.Returns) == 0 {
		t.Fatalf("expected build_page function with return, got %v", field.Type)
	}
	ret := unwrap.Record(fn.Returns[0])
	if ret == nil {
		t.Fatalf("expected build_page record return, got %v", fn.Returns[0])
	}
	dataFunc := ret.GetField("data_func")
	if dataFunc == nil || !typ.IsAny(dataFunc.Type) {
		t.Fatalf("expected build_page return data_func:any, got %v", fn.Returns[0])
	}
}
