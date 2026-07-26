package engine_test

import (
	"strings"
	"testing"
)

// An imported return catalog states a correlation between two result slots: a
// guard on one slot proves the other occupied. The guard decides a set of
// values, so the catalog has to be scanned over that whole set. A truthiness
// guard admits every value that is neither nil nor false, so a return that puts
// a truthy value in the trigger slot without the companion refutes the
// correlation even when the producer never returns `true` itself.

const truthyCatalogProvider = `local M = {}

function M.probe(flag: boolean): (any, string?)
    if flag then
        return true, "payload"
    end
    return 1, nil
end

return M
`

// TestTruthyGuardScansEveryTruthyReturn pins that a truthy-but-not-true return
// with an absent companion refutes the correlation. Discharging the assignment
// here would rest on the `true` return alone while the call may have taken the
// `1` return, where the companion is nil.
func TestTruthyGuardScansEveryTruthyReturn(t *testing.T) {
	diagnostics := checkModules(t, []string{"provider", "main"}, map[string]string{
		"provider": truthyCatalogProvider,
		"main": `local provider = require("provider")
local ok, v = provider.probe(true)
if ok then
    local s: string = v
    return s
end
return ""
`,
	})
	if !strings.Contains(diagnosticLines(diagnostics), "type.assignment") {
		t.Fatalf("a truthy guard discharged a companion slot the catalog leaves nil on another truthy return:\n%s",
			diagnosticLines(diagnostics))
	}
}

// TestNilGuardKeepsItsProvenCorrelation pins the other side: where every return
// that puts nil in the trigger slot does carry the companion, the correlation
// still holds and the guarded read is discharged.
func TestNilGuardKeepsItsProvenCorrelation(t *testing.T) {
	diagnostics := checkModules(t, []string{"provider", "main"}, map[string]string{
		"provider": `local M = {}

function M.fetch(id: string): (string?, string?)
    if id == "" then
        return nil, "empty id"
    end
    return "value:" .. id, nil
end

return M
`,
		"main": `local provider = require("provider")
local value, err = provider.fetch("u1")
if err == nil then
    local s: string = value
    return s
end
return ""
`,
	})
	if strings.Contains(diagnosticLines(diagnostics), "type.assignment") {
		t.Fatalf("a proven nil-guard correlation was withdrawn:\n%s", diagnosticLines(diagnostics))
	}
}
