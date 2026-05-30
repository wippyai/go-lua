package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZKeysProvBaseline confirms the interproc fixture residual before PART B.
func TestZZKeysProvBaseline(t *testing.T) {
	src := `
type Entry = {id: string}
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end
local m: {[string]: Entry} = {}
local names = sorted_keys(m)
for _, name in ipairs(names) do
    local e: Entry = m[name]
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	t.Logf("KEYS-PROV baseline: %d diags", len(msgs))
	for _, m := range msgs {
		t.Logf("  DIAG: %s", m)
	}
}

// TestZZKeysProvWrongContainer is a SOUNDNESS probe: keys collected from one
// container `a` indexing a DIFFERENT container `b` must stay optional and ERROR.
func TestZZKeysProvWrongContainer(t *testing.T) {
	src := `
type Entry = {id: string}
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end
local a: {[string]: Entry} = {}
local b: {[string]: Entry} = {}
local names = sorted_keys(a)
for _, name in ipairs(names) do
    local e: Entry = b[name]
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	t.Logf("KEYS-PROV wrong-container: %d diags (expect >=1)", len(msgs))
	for _, m := range msgs {
		t.Logf("  DIAG: %s", m)
	}
	if len(msgs) == 0 {
		t.Fatal("keys of a indexing b must stay optional and error")
	}
}

// TestZZKeysProvArbitraryFunction is a SOUNDNESS probe: a function that returns an
// ARBITRARY string array (NOT the keys of its parameter) is NOT a keys-collector, so
// its result indexing a map must stay optional and ERROR.
func TestZZKeysProvArbitraryFunction(t *testing.T) {
	src := `
type Entry = {id: string}
local function arbitrary(t): {string}
    local keys = {}
    table.insert(keys, "literal")
    return keys
end
local a: {[string]: Entry} = {}
local names = arbitrary(a)
for _, name in ipairs(names) do
    local e: Entry = a[name]
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	t.Logf("KEYS-PROV arbitrary-fn: %d diags (expect >=1)", len(msgs))
	for _, m := range msgs {
		t.Logf("  DIAG: %s", m)
	}
	if len(msgs) == 0 {
		t.Fatal("arbitrary string array indexing a map must stay optional and error")
	}
}

// TestZZKeysProvLiteralIndex is a SOUNDNESS probe: a literal index into the map
// (not a key drawn from the collected keys) must stay optional and ERROR even when
// a keys-collector result is in scope.
func TestZZKeysProvLiteralIndex(t *testing.T) {
	src := `
type Entry = {id: string}
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end
local a: {[string]: Entry} = {}
local names = sorted_keys(a)
local e: Entry = a["literal"]
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	t.Logf("KEYS-PROV literal-index: %d diags (expect >=1)", len(msgs))
	for _, m := range msgs {
		t.Logf("  DIAG: %s", m)
	}
	if len(msgs) == 0 {
		t.Fatal("literal index into a map must stay optional and error")
	}
}

// TestZZKeysProvReassigned is a SOUNDNESS probe: when the iterated array is
// reassigned from a non-collector source after the collector call, the provenance
// is ambiguous and must be declined, so the read stays optional and errors.
func TestZZKeysProvReassigned(t *testing.T) {
	src := `
type Entry = {id: string}
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end
local function other(): {string}
    return {"x"}
end
local a: {[string]: Entry} = {}
local names = sorted_keys(a)
names = other()
for _, name in ipairs(names) do
    local e: Entry = a[name]
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	msgs := testutil.ErrorMessages(res.Diagnostics)
	t.Logf("KEYS-PROV reassigned: %d diags (expect >=1)", len(msgs))
	for _, m := range msgs {
		t.Logf("  DIAG: %s", m)
	}
	if len(msgs) == 0 {
		t.Fatal("a reassigned collector result must lose provenance and error")
	}
}
