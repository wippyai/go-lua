package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// zzMMrepo is an untyped repo whose query returns `any`; cross-module export
// demotes the inferred any to unknown.
const zzMMrepo = `
local executor = {}
function executor:query(): any
	return nil
end
local repo = {}
function repo.list_by_type(_a, _b)
	local contexts, err = executor:query()
	if err then return nil, err end
	return contexts
end
return repo
`

func zzMMcheck(t *testing.T, label, readerProbe string) {
	t.Helper()
	repoMod := testutil.CheckAndExport(zzMMrepo, "repo", testutil.WithStdlib())
	if repoMod.HasError() {
		t.Fatalf("[%s] repo export error: %v", label, testutil.ErrorMessages(repoMod.Errors))
	}
	r := testutil.Check(readerProbe, testutil.WithStdlib(), testutil.WithModule("repo", repoMod))
	if !r.HasError() {
		t.Logf("[%s] NO ERROR", label)
		return
	}
	for _, e := range r.Errors {
		t.Logf("[%s] err: %s @ %d:%d", label, e.Message, e.Position.Line, e.Position.Column)
	}
}

// M1: what does the imported untyped list_by_type return (first slot)?
func TestZZMM_M1_ListReturn(t *testing.T) {
	zzMMcheck(t, "M1", `
local repo = require("repo")
local c, e = repo.list_by_type("a", "b")
local probe: number = c
`)
}

// M2: what is `c or {}` where c is the imported (demoted) value?
func TestZZMM_M2_OrFallback(t *testing.T) {
	zzMMcheck(t, "M2", `
local repo = require("repo")
local c, e = repo.list_by_type("a", "b")
local fallback = c or {}
local probe: number = fallback
`)
}

// M3: index `c or {}` under a length guard (the failing pattern).
func TestZZMM_M3_GuardedIndex(t *testing.T) {
	zzMMcheck(t, "M3", `
local repo = require("repo")
local c, e = repo.list_by_type("a", "b")
local xs = c or {}
if xs and #xs > 0 then
	local probe: number = xs[1]
end
`)
}
