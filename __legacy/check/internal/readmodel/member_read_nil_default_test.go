package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
)

func TestForEachMissingMemberReadSkipsDuplicateNilDefaultOccurrence(t *testing.T) {
	stmts := parseChunk(t, `
type Entry = {id: string}
type Suite = {name: string, tests: {Entry}}

local function takes_suite(suite: Suite): ()
end

local entry: Entry = {id = "a"}
local suite = {name = "alpha"}
suite.tests = suite.tests or {}
takes_suite(suite)
`)
	checked, err := program.RunChunk(stmts, program.Config{Check: body.Config{Registry: standard.Registry()}})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	var reads []MissingMemberRead
	New(checked.RootResult()).ForEachMissingMemberRead(func(read MissingMemberRead) bool {
		reads = append(reads, read)
		return true
	})
	if len(reads) != 0 {
		t.Fatalf("missing member reads = %#v, want nil-default duplicate suppressed", reads)
	}
}
