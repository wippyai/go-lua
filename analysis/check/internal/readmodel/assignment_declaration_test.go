package readmodel

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// A declaration without an initializer defers the annotated contract to later
// assignments; the implicit nil state belongs to flow-sensitive use analysis,
// not to assignment obligations.
func TestForEachAssignmentSkipsDeclarationWithoutInitializer(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type Query = {
	one: fun(self: Query): number?,
}

local reader: Query
reader = {
	one = function(_self: Query): number?
		return nil
	end,
}
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var inadmissible []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if !assignment.Check.Admissible {
			inadmissible = append(inadmissible, assignment)
		}
		return true
	})
	for _, assignment := range inadmissible {
		t.Errorf("unexpected inadmissible assignment: target=%q actual=%v expected=%v",
			assignment.TargetLabel, assignment.TypeWithPresence, assignment.Expected)
	}
}

// A nil-filled target of an initialized multi-assignment still reports: the
// value list supplied no value for the target, so nil flows into a
// non-optional annotation.
func TestForEachAssignmentReportsUnderSuppliedInitializedTarget(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local a, b: string, string = "x"
`)
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var got []Assignment
	New(result).ForEachAssignment(func(assignment Assignment) bool {
		if assignment.TargetLabel == "b" && !assignment.Check.Admissible {
			got = append(got, assignment)
		}
		return true
	})
	if len(got) != 1 {
		t.Fatalf("under-supplied target assignments = %#v, want one inadmissible b assignment", got)
	}
	if got[0].TypeWithPresence == nil || !typ.Nil.Equals(got[0].TypeWithPresence) {
		t.Fatalf("under-supplied target actual = %v, want nil", got[0].TypeWithPresence)
	}
}
