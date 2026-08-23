package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestPackSourceEngineIssuesGeneratedSourceRows(t *testing.T) {
	record := mountedRecord(t, "pack-source-engine-literal", "return 1\n")
	bound := materializerBinding(t, record)
	committed, _ := queryCanonicalProgram(t, record, bound)
	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal pack-source engine fixture: %v", sealFailure)
	}
	state, solveStatus, solveReport := sealed.SolveWithReport(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve pack-source engine fixture: status=%v state=%v reason=%v failure=%v point=%v group=%v member=%v rule=%v", solveStatus, state, solveReport.Reason(), solveReport.Failure(), solveReport.Point(), solveReport.Group(), solveReport.Member(), solveReport.Rule())
	}
	count := 0
	for _, mount := range record.Artifacts {
		rows, rowsOK := mount.Program.RuleOccurrenceCount()
		if !rowsOK {
			t.Fatal("pack-source rule occurrences")
		}
		for index := 0; index < rows; index++ {
			row, rowOK := mount.Program.RuleOccurrenceAt(index)
			if !rowOK || string(row.Key()) != "pack-source" {
				continue
			}
			ordinal, ordinalOK := row.Occurrence()
			occurrence, occurrenceOK := mount.Program.OccurrenceAt(int(ordinal))
			if !ordinalOK || !occurrenceOK || occurrence.Kind() != programschema.OccurrenceValues && occurrence.Kind() != programschema.OccurrenceCall {
				continue
			}
			count++
		}
	}
	if count == 0 {
		t.Fatal("literal fixture issued no pack-source rows")
	}
}
