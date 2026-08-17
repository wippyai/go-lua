package executable

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealClosesFunctionFormalVarargAndCaptureOperands(t *testing.T) {
	counts := functionClosureCounts()
	term := keyspace.MakeTerm
	body2 := term(keyspace.FamilyBody, 2)
	function := term(keyspace.FamilyFunction, 1)
	bind := term(keyspace.FamilyBind, 1)
	rows := [][]keyspace.Term{{bind}, {term(keyspace.FamilyReturn, 1)}}
	fixture := openSealFixture(t, "function-closure.lua", counts, rows, functionClosureFlow(),
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{term(keyspace.FamilyCell, 1)}}},
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{term(keyspace.FamilyCell, 2)}}},
		[]keyspace.Term{body2})
	result, err := Seal(fixture.sourceView, fixture.flow, fixture.forest, fixture.control,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("function closure executable.Seal: %v", err)
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := term(family, ordinal)
			if !result.Executable(term) {
				t.Fatalf("function closure left (%d,%d) nonexecutable", family, ordinal)
			}
		}
	}
	if got, want := result.Count(), 13; got != want {
		t.Fatalf("function closure executable Count = %d, want %d", got, want)
	}
}
