package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Reproduces local-function-fact-authority. The indexed-base table.insert
// element-type seeding (lenseed.go applyTableInsert -> applyMapElementInsert) is
// fixed: suites is now {[string]: Entry[]} (matches legacy exactly), so the read
// is Entry[]? not {}?. The residual `?` is the legacy Flow.HasKeyOf key-presence
// correlation (a key from pairs(suites)/sorted_keys(suites) indexing suites is
// provably present): the canonical flow produces NO KeyOf constraint, so the read
// stays soundly optional. That fact production is out of the transfer lane
// (iteration extraction + interproc return propagation) -> handoff.

func TestZZInsertIndexedBaseMin(t *testing.T) {
	src := `
type Entry = {id: string, meta: {type: string, suite: string?, order: number?}?}

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local function group_by_suite(entries: {Entry})
    local suites = {}
    for _, entry in ipairs(entries) do
        local suite = entry.meta and entry.meta.suite
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        end
    end
    return suites
end

local entries: {Entry} = {}
local suites = group_by_suite(entries)
local suite_names = sorted_keys(suites)
for _, name in ipairs(suite_names) do
    local tests: {Entry} = suites[name]
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("MIN DIAG: %s", m)
	}
}

// TestZZInsertIndexedFixture runs the actual fixture through the canonical flow.
func TestZZInsertIndexedFixture(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	var target namedSuite
	found := false
	for _, s := range suites {
		if s.Name == "regression/local-function-fact-authority" {
			target = s
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fixture not found")
	}
	diags, entry := canonicalFixtureDiagnostics(target)
	t.Logf("entry=%s, %d diagnostics", entry, len(diags))
	for _, d := range diags {
		t.Logf("  %s", diagSummary(d))
	}
}
