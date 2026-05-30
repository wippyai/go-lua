package check

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// TestZZExportFactsProbe inspects the canonical FunctionFacts return projection
// for an inferred-return exported function. Diagnostic only.
func TestZZExportFactsProbe(t *testing.T) {
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
	checker := newSessionTestChecker(nil)
	checker.flowMode = FlowCanonical
	sess := checker.Check(src, "page_registry.lua")

	facts := sess.rootFunctionFactsForExport()
	t.Logf("rootFunctionFactsForExport len=%d", len(facts))
	g := sess.RootGraph()
	for _, sym := range cfg.SortedSymbolIDs(facts) {
		name := ""
		if g != nil {
			name = g.NameOf(sym)
		}
		rt := functionfact.ReturnProjection(facts, sym, api.PhaseNarrowing)
		exp := functionfact.ExportTypeProjection(facts, sym, api.PhaseNarrowing)
		t.Logf("  sym=%d name=%q returns=%v export=%s", sym, name, rt, exp)
	}
}
