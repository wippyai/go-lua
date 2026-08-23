package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestHeapIngressEngineIssuesGeneratedSourceRows(t *testing.T) {
	record := mountedRecord(t, "heap-ingress-engine-table", "return {}\n")
	bound := materializerBinding(t, record)
	committed, _ := queryCanonicalProgram(t, record, bound)
	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal heap-ingress engine fixture: %v", sealFailure)
	}
	state, solveStatus, solveReport := sealed.SolveWithReport(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve heap-ingress engine fixture: status=%v state=%v reason=%v failure=%v point=%v group=%v member=%v rule=%v", solveStatus, state, solveReport.Reason(), solveReport.Failure(), solveReport.Point(), solveReport.Group(), solveReport.Member(), solveReport.Rule())
	}
	count := 0
	for _, mount := range record.Artifacts {
		rows, rowsOK := mount.Program.RuleOccurrenceCount()
		if !rowsOK {
			t.Fatal("heap-ingress rule occurrences")
		}
		for index := 0; index < rows; index++ {
			row, rowOK := mount.Program.RuleOccurrenceAt(index)
			if !rowOK || string(row.Key()) != "heap-ingress" {
				continue
			}
			ordinal, ordinalOK := row.Occurrence()
			occurrence, occurrenceOK := mount.Program.OccurrenceAt(int(ordinal))
			if !ordinalOK || !occurrenceOK || occurrence.Kind() != programschema.OccurrenceAllocation {
				continue
			}
			count++
		}
	}
	if count == 0 {
		t.Fatal("table fixture issued no heap-ingress rows")
	}
}
