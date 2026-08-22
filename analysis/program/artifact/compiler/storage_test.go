package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/domain/composite"
)

// TestProgramArtifactStorageTransfersUseExactLocalPlacement keeps storage
// transfer placement in the reusable Program artifact.  The assertion does
// not depend on Link/domain construction: Entry and predecessor witnesses are
// retained while Program ownership is live, then only immutable point IDs
// cross into the artifact.
func TestProgramArtifactStorageTransfersUseExactLocalPlacement(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-storage-stage.lua", Text: []byte(`
local function move(flag: boolean)
    local source: number | string
    if flag then
        source = 1
    else
        source = "x"
    end
    local destination = source
    return destination
end
return move
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile storage fixture: %s", failure.Error())
	}
	program := summaryProgram(t, artifact)
	transferCount, transfersPublished := program.LocalTransferCount()
	if !transfersPublished {
		t.Fatal("local-transfer family is unpublished")
	}

	required := map[programschema.OccurrenceKind]bool{
		programschema.OccurrenceStorageRead:         false,
		programschema.OccurrenceStorageBindTransfer: false,
		programschema.OccurrenceStorageWrite:        false,
	}
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for occurrenceIndex := 0; occurrenceIndex < occurrenceCount; occurrenceIndex++ {
		occurrence, occurrenceOK := program.OccurrenceAt(occurrenceIndex)
		if !occurrenceOK {
			t.Fatalf("artifact occurrence %d unavailable", occurrenceIndex)
		}
		kind := occurrence.Kind()
		if _, wanted := required[kind]; !wanted {
			continue
		}
		required[kind] = true
		placements := 0
		ruleCount, rulesPublished := program.RuleOccurrenceCountForKey("value-transfer")
		if !rulesPublished {
			t.Fatal("rule-occurrence family is unpublished")
		}
		for ruleIndex := 0; ruleIndex < ruleCount; ruleIndex++ {
			rule, ruleOK := program.RuleOccurrenceForKeyAt("value-transfer", ruleIndex)
			ordinal, ordinalOK := rule.Occurrence()
			parent, parentOK := program.OccurrenceAt(int(ordinal))
			if !ruleOK || !ordinalOK || !parentOK || parent.ID() != occurrence.ID() {
				continue
			}
			placements++
			point := rule.PointID()
			input, inputOK := rule.InputPoint()
			expectedStage := programissuance.StageLocal
			if kind == programschema.OccurrenceStorageWrite {
				expectedStage = programissuance.StagePredecessor
			}
			if !point.Available() || !inputOK || point == input || rule.Stage() != expectedStage {
				t.Fatalf("kind=%d rule=%d is not a distinct Local placement", kind, ruleIndex)
			}
			switch kind {
			case programschema.OccurrenceStorageRead:
				if rule.InputSpec() != programissuance.InputEntryGeometry {
					t.Fatalf("kind=%d rule=%d did not retain Program Entry", kind, ruleIndex)
				}
			case programschema.OccurrenceStorageBindTransfer:
				if rule.InputSpec() != programissuance.InputPreviousStage {
					t.Fatalf("bind rule=%d did not retain the ordinary Local cut as its Finish input", ruleIndex)
				}
			case programschema.OccurrenceStorageWrite:
				route, routeOK := rule.PredecessorRouteID()
				if rule.InputSpec() != programissuance.InputPredecessorGeometry || !routeOK || !route.Available() {
					t.Fatalf("write rule=%d did not retain exact predecessor route", ruleIndex)
				}
			}
			localParent := false
			for edgeIndex := 0; edgeIndex < transferCount; edgeIndex++ {
				edge, edgeOK := program.LocalTransferAt(edgeIndex)
				if !edgeOK || edge.To() != point {
					continue
				}
				if kind == programschema.OccurrenceStorageBindTransfer && edge.Full() && edge.From() == input {
					localParent = true
					continue
				}
				_, pointCount, spanOK := occurrence.PointSpan()
				for pointIndex := uint32(0); pointIndex < pointCount; pointIndex++ {
					parent, parentOK := program.OccurrencePointID(occurrenceIndex, int(pointIndex))
					if spanOK && parentOK && parent == edge.From() {
						localParent = true
					}
				}
			}
			if !localParent {
				t.Fatalf("kind=%d rule=%d lacks its declared incoming local transport", kind, ruleIndex)
			}
		}
		if placements == 0 {
			t.Fatalf("kind=%d has no ValueStorageTransfer placement", kind)
		}
	}
	for kind, found := range required {
		if !found {
			t.Fatalf("fixture did not issue storage occurrence kind=%d", kind)
		}
	}
}
