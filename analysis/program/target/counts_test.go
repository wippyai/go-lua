package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestCountRowsPublishesExactlyTargetOwnerRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{builtin("counted", testString, RowSpec{Tail: RowClosed})}})
	rows := contract.CountRows()
	if !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerTarget) {
		t.Fatal("target CountRows did not cover its generated owner catalog")
	}
	ids := denominator.GeneratedTargetIDs()
	if got, ok := rows.Value(ids.TargetOperation); !ok || got == 0 {
		t.Fatalf("target operation count = %d/%v, want a nonzero row", got, ok)
	}
}
