package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestStaticTransferEngineIssuesGeneratedIdentityRows(t *testing.T) {
	record := mountedRecord(t, "static-transfer-engine-swap", valueTransferEngineSwapSource)
	bound := materializerBinding(t, record)
	committed, _ := queryCanonicalProgram(t, record, bound)
	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal static-transfer engine fixture: %v", sealFailure)
	}
	state, solveStatus, solveReport := sealed.SolveWithReport(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve static-transfer engine fixture: status=%v state=%v reason=%v failure=%v point=%v group=%v member=%v rule=%v", solveStatus, state, solveReport.Reason(), solveReport.Failure(), solveReport.Point(), solveReport.Group(), solveReport.Member(), solveReport.Rule())
	}

	writes := staticTransferEngineWrites(t, record)
	if len(writes) == 0 {
		t.Fatal("assignment fixture issued no static-transfer rows")
	}
	if len(writes) != 1 {
		t.Fatalf("assignment fixture has %d static-transfer writes, want one target", len(writes))
	}
	if writes[0].from == writes[0].to {
		t.Fatal("static-transfer aliases its source and target coordinate")
	}
}

func staticTransferEngineWrites(t testing.TB, record LinkInputs) []valueTransferEngineWrite {
	t.Helper()
	if record.ValueSchema == nil {
		t.Fatal("Value schema is unavailable")
	}
	writes := make([]valueTransferEngineWrite, 0, 2)
	seen := make(map[identity.ContentID]struct{})
	for _, mount := range record.Artifacts {
		count, countOK := mount.Program.RuleOccurrenceCount()
		if !countOK {
			t.Fatal("static transfer rule occurrences")
		}
		for index := 0; index < count; index++ {
			row, rowOK := mount.Program.RuleOccurrenceAt(index)
			if !rowOK || string(row.Key()) != "static-transfer" {
				continue
			}
			ordinal, ordinalOK := row.Occurrence()
			occurrence, occurrenceOK := mount.Program.OccurrenceAt(int(ordinal))
			if !ordinalOK || !occurrenceOK || occurrence.Kind() != programschema.OccurrenceStorageWrite {
				continue
			}
			transfer, transferOK := record.ValueSchema.StorageTransferForArtifactOccurrence(mount.ModuleKey, occurrence.ID())
			from, to, endpointsOK := transfer.Endpoints()
			if !transferOK || !record.ValueSchema.OwnsStorageTransfer(transfer) || !endpointsOK {
				t.Fatalf("static transfer Rule row %d has no canonical Value owner row", index)
			}
			point := row.PointID()
			if !point.Available() {
				t.Fatalf("static transfer Rule row %d has no point", index)
			}
			inputPoint, inputPointOK := row.InputPointAt(0)
			if !inputPointOK {
				t.Fatalf("static transfer Rule row %d has no input point", index)
			}
			transferID, transferIDOK := transfer.ID()
			if !transferIDOK {
				t.Fatalf("static transfer Rule row %d has no canonical owner row identity", index)
			}
			if _, duplicate := seen[transferID]; duplicate {
				continue
			}
			seen[transferID] = struct{}{}
			writes = append(writes, valueTransferEngineWrite{point: point, inputPoint: inputPoint, from: from, to: to, transfer: transfer})
		}
	}
	return writes
}
