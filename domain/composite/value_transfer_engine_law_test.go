package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/snapshot"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The assignment is intentionally a two-target swap.  Program emits one
// storage-write occurrence for each target, and Value seals one directed
// transfer for each of those occurrences.  Keeping the fixture here (rather
// than manufacturing transfer rows) makes the law exercise the mounted
// occurrence-to-transfer directory used by the generated Rule.
const valueTransferEngineSwapSource = "local a = 1\nlocal b = 0\nb = a\nreturn b\n"

type valueTransferEngineWrite struct {
	point      identity.ContentID
	inputPoint identity.ContentID
	transport  identity.ContentID
	from       valuedomain.Coordinate
	to         valuedomain.Coordinate
	transfer   valuedomain.StorageTransfer
}

// TestValueTransferEnginePublishesCanonicalOwnerFacts walks a real mounted
// Value-transfer Rule through the composite construction and solve boundary.
// The assertions consume Value's typed summary observation and owner fence;
// they do not compare a legacy hot Rule or replay a retired callback path.
func TestValueTransferEnginePublishesCanonicalOwnerFacts(t *testing.T) {
	record := mountedRecord(t, "value-transfer-engine-swap", valueTransferEngineSwapSource)
	bound := materializerBinding(t, record)
	committed, table := queryCanonicalProgram(t, record, bound)
	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal value-transfer engine fixture: %v", sealFailure)
	}
	state, solveStatus, solveReport := sealed.SolveWithReport(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		pointDigest := solveReport.Point().Digest()
		for _, mount := range record.Artifacts {
			count, _ := mount.Program.RuleOccurrenceCount()
			for index := 0; index < count; index++ {
				row, ok := mount.Program.RuleOccurrenceAt(index)
				if ok && row.PointID() == identity.ContentID(pointDigest) {
					t.Logf("failed point rule row=%d key=%s stage=%s inputs=%d", index, row.Key(), row.Stage(), row.InputPointCount())
				}
			}
		}
		t.Fatalf("solve value-transfer engine fixture: status=%v state=%v reason=%v failure=%v point=%v group=%v member=%v rule=%v", solveStatus, state, solveReport.Reason(), solveReport.Failure(), solveReport.Point(), solveReport.Group(), solveReport.Member(), solveReport.Rule())
	}
	published, publishedOK := sealed.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("value-transfer engine fixture published no snapshot")
	}
	view := published.Snapshot()
	queryPlan, queryPlanOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	if !queryPlanOK {
		t.Fatal("open published Value query column")
	}
	publications, publicationsOK := bound.QueryPublications(committed, table)
	if !publicationsOK {
		t.Fatal("value-transfer query publications")
	}

	writes := valueTransferEngineWrites(t, record)
	if len(writes) != 1 {
		t.Fatalf("assignment fixture has %d Value transfer writes, want one target", len(writes))
	}
	valueByPoint := make(map[identity.ContentID]QueryPublication)
	for _, publication := range publications {
		if publication.Site.Family == QueryFamilyValueSummary {
			valueByPoint[publication.Site.Point] = publication
		}
	}
	if len(valueByPoint) == 0 {
		t.Fatal("query publication table has no Value summary sites")
	}

	seenTargets := make(map[uint32]struct{}, len(writes))
	var firstObservation valuedomain.ValueSummaryObservation
	var firstObservationOK bool
	for _, write := range writes {
		publication, publicationOK := valueByPoint[write.point]
		inputPublication, inputPublicationOK := valueByPoint[write.inputPoint]
		if !publicationOK || !inputPublicationOK {
			if !write.inputPoint.Available() {
				t.Fatalf("Value transfer point %s has no canonical input point", write.point)
			}
			t.Fatalf("Value transfer point %s has no canonical summary publication", write.point)
		}
		answer, readStatus := snapshot.Query(&view, queryPlan, publication.Key)
		if readStatus != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("Value transfer point %s query status=%s available=%t", write.point, readStatus, answer.Available())
		}
		observation, observationOK := engine.AnswerValue[valuedomain.ValueSummaryObservation](answer)
		if !observationOK || !record.ValueSchema.OwnsSummaryObservation(observation) {
			t.Fatalf("Value transfer point %s did not publish an owner-authenticated Value summary", write.point)
		}
		inputAnswer, inputReadStatus := snapshot.Query(&view, queryPlan, inputPublication.Key)
		if inputReadStatus != snapshot.ReadHit || !inputAnswer.Available() {
			t.Fatalf("Value transfer point %s input query status=%s available=%t", write.point, inputReadStatus, inputAnswer.Available())
		}
		inputObservation, inputObservationOK := engine.AnswerValue[valuedomain.ValueSummaryObservation](inputAnswer)
		if !inputObservationOK || !record.ValueSchema.OwnsSummaryObservation(inputObservation) {
			t.Fatalf("Value transfer point %s input did not publish an owner-authenticated Value summary", write.point)
		}
		fromIndex, fromOK := record.ValueSchema.CoordinateIndex(write.from)
		toIndex, toOK := record.ValueSchema.CoordinateIndex(write.to)
		if !fromOK || !toOK {
			t.Fatalf("Value transfer point %s has non-canonical endpoint coordinates", write.point)
		}
		if fromIndex == toIndex {
			t.Fatalf("Value transfer point %s aliases its source and target coordinate", write.point)
		}
		if !inputObservation.Present[fromIndex] || !observation.Present[toIndex] {
			t.Logf("write point=%s input=%s transport=%s", write.point, write.inputPoint, write.transport)
			var fromID identity.ContentID
			for coordinateIndex := 0; coordinateIndex < record.ValueSchema.CoordinateCount(); coordinateIndex++ {
				id, dense, coordinateOK := record.ValueSchema.CanonicalCoordinateAt(coordinateIndex)
				if coordinateOK && dense == fromIndex {
					fromID = id
				}
			}
			t.Logf("source coordinate=%d id=%s", fromIndex, fromID)
			for _, mount := range record.Artifacts {
				transferCount, _ := mount.Program.LocalTransferCount()
				for transferIndex := 0; transferIndex < transferCount; transferIndex++ {
					edge, edgeOK := mount.Program.LocalTransferAt(transferIndex)
					fromPublication, fromPublished := valueByPoint[edge.From()]
					toPublication, toPublished := valueByPoint[edge.To()]
					if !edgeOK || !fromPublished || !toPublished {
						continue
					}
					fromAnswer, _ := snapshot.Query(&view, queryPlan, fromPublication.Key)
					toAnswer, _ := snapshot.Query(&view, queryPlan, toPublication.Key)
					fromObservation, fromObserved := engine.AnswerValue[valuedomain.ValueSummaryObservation](fromAnswer)
					toObservation, toObserved := engine.AnswerValue[valuedomain.ValueSummaryObservation](toAnswer)
					if fromObserved && toObserved && (fromObservation.Present[fromIndex] || toObservation.Present[fromIndex]) {
						t.Logf("source transport %s -> %s full=%t from=%t to=%t", edge.From(), edge.To(), edge.Full(), fromObservation.Present[fromIndex], toObservation.Present[fromIndex])
					}
				}
				occurrenceCount, _ := mount.Program.OccurrenceCount()
				ruleCount, _ := mount.Program.RuleOccurrenceCount()
				for ruleIndex := 0; ruleIndex < ruleCount; ruleIndex++ {
					ruleRow, ruleOK := mount.Program.RuleOccurrenceAt(ruleIndex)
					if ruleOK && (ruleRow.PointID() == write.transport || ruleRow.PointID() == write.inputPoint) {
						t.Logf("adjacent rule key=%s stage=%s point=%s", ruleRow.Key(), ruleRow.Stage(), ruleRow.PointID())
					}
				}
				for occurrenceIndex := 0; occurrenceIndex < occurrenceCount; occurrenceIndex++ {
					occurrence, occurrenceOK := mount.Program.OccurrenceAt(occurrenceIndex)
					if !occurrenceOK {
						continue
					}
					matches := occurrence.ID() == fromID
					_, inputCount, _ := occurrence.InputSpan()
					for inputIndex := 0; inputIndex < int(inputCount); inputIndex++ {
						inputID, inputOK := mount.Program.OccurrenceInputID(occurrenceIndex, inputIndex)
						matches = matches || inputOK && inputID == fromID
					}
					if matches {
						_, pointCount, _ := occurrence.PointSpan()
						for pointIndex := 0; pointIndex < int(pointCount); pointIndex++ {
							pointID, _ := mount.Program.OccurrencePointID(occurrenceIndex, pointIndex)
							t.Logf("source occurrence kind=%d id=%s point=%s", occurrence.Kind(), occurrence.ID(), pointID)
						}
					}
				}
			}
			for candidatePoint, candidatePublication := range valueByPoint {
				candidateAnswer, candidateRead := snapshot.Query(&view, queryPlan, candidatePublication.Key)
				candidateObservation, candidateOK := engine.AnswerValue[valuedomain.ValueSummaryObservation](candidateAnswer)
				if candidateRead == snapshot.ReadHit && candidateOK && record.ValueSchema.OwnsSummaryObservation(candidateObservation) {
					t.Logf("candidate point=%s from=%t to=%t rows=%d", candidatePoint, candidateObservation.Present[fromIndex], candidateObservation.Present[toIndex], candidateObservation.Rows)
				}
			}
			t.Fatalf("Value transfer point %s input=%s omitted source/target facts: from=%t to=%t", write.point, write.inputPoint, inputObservation.Present[fromIndex], observation.Present[toIndex])
		}
		if !record.ValueSchema.AdmitsCoordinate(write.from, inputObservation.Values[fromIndex]) || !record.ValueSchema.AdmitsCoordinate(write.to, observation.Values[toIndex]) {
			t.Fatalf("Value transfer point %s published a foreign endpoint fact", write.point)
		}
		if observation.Values[toIndex].IsBottom() || observation.Values[toIndex].IsTop() {
			t.Fatalf("Value transfer point %s published lattice fallback at target", write.point)
		}
		if !record.ValueSchema.Equal(inputObservation.Values[fromIndex], observation.Values[toIndex]) {
			t.Fatalf("Value transfer point %s did not carry the source Value to its target", write.point)
		}
		if _, duplicate := seenTargets[toIndex]; duplicate {
			t.Fatalf("two Value transfer writes published the same target coordinate at point %s", write.point)
		}
		seenTargets[toIndex] = struct{}{}

		cell, cellOK := publication.CanonicalCell(answer)
		if !cellOK || !cell.Available() || cell.ContractID() != publication.Contract().ContentID() {
			t.Fatalf("Value transfer point %s did not close its canonical result cell", write.point)
		}
		if !firstObservationOK {
			firstObservation, firstObservationOK = inputObservation, true
		}
	}
	if len(seenTargets) != len(writes) || !firstObservationOK {
		t.Fatal("Value transfer swap did not publish one canonical fact vector per target")
	}

	// The nearest negative is an equal-content Value schema from another Link:
	// its owner directory must refuse the borrowed observation, even though the
	// observation's shape and facts are otherwise valid.
	foreignRecord := mountedRecord(t, "value-transfer-engine-foreign", valueTransferEngineSwapSource)
	if foreignRecord.ValueSchema == record.ValueSchema {
		t.Fatal("foreign Value fixture reused the local schema owner")
	}
	if foreignRecord.ValueSchema.OwnsSummaryObservation(firstObservation) {
		t.Fatal("foreign Value schema accepted a local canonical summary observation")
	}
}

func valueTransferEngineWrites(t testing.TB, record LinkInputs) []valueTransferEngineWrite {
	t.Helper()
	if record.ValueSchema == nil {
		t.Fatal("Value schema is unavailable")
	}
	writes := make([]valueTransferEngineWrite, 0, 2)
	seen := make(map[identity.ContentID]struct{})
	for _, mount := range record.Artifacts {
		count, countOK := mount.Program.RuleOccurrenceCount()
		if !countOK {
			t.Fatal("Value transfer rule occurrences")
		}
		for index := 0; index < count; index++ {
			row, rowOK := mount.Program.RuleOccurrenceAt(index)
			if !rowOK || string(row.Key()) != "value-transfer" {
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
				t.Fatalf("Value transfer Rule row %d has no canonical Value owner row", index)
			}
			point := row.PointID()
			if !point.Available() {
				t.Fatalf("Value transfer Rule row %d has no point", index)
			}
			// The compiler proves the predecessor point is one exact occurrence
			// finish point, so the generated read and identity carry share the
			// single owner-issued input at ordinal zero.
			inputPoint, inputPointOK := row.InputPointAt(0)
			if !inputPointOK {
				t.Fatalf("Value transfer Rule row %d has no input point", index)
			}
			transferID, transferIDOK := transfer.ID()
			if !transferIDOK {
				t.Fatalf("Value transfer Rule row %d has no canonical owner row identity", index)
			}
			if _, duplicate := seen[transferID]; duplicate {
				continue
			}
			seen[transferID] = struct{}{}
			var transport identity.ContentID
			transferCount, transferRowsOK := mount.Program.LocalTransferCount()
			if !transferRowsOK {
				t.Fatalf("Value transfer Rule row %d has no local transfer family", index)
			}
			for transferIndex := 0; transferIndex < transferCount; transferIndex++ {
				edge, edgeOK := mount.Program.LocalTransferAt(transferIndex)
				if edgeOK && edge.To() == point {
					if transport.Available() {
						t.Fatalf("Value transfer Rule row %d has multiple local transport sources", index)
					}
					transport = edge.From()
				}
			}
			writes = append(writes, valueTransferEngineWrite{point: point, inputPoint: inputPoint, transport: transport, from: from, to: to, transfer: transfer})
		}
	}
	return writes
}
