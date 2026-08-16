package artifact_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
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
	receipt, receiptOK := grammar.Global()
	if !receiptOK {
		t.Fatal("Program artifact grammar capability unavailable")
	}
	artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile storage fixture: %s", failure.Error())
	}

	required := map[programartifact.OccurrenceKind]bool{
		programartifact.OccurrenceStorageRead:         false,
		programartifact.OccurrenceStorageBindTransfer: false,
		programartifact.OccurrenceStorageWrite:        false,
	}
	for occurrenceIndex := 0; occurrenceIndex < artifact.OccurrenceCount(); occurrenceIndex++ {
		occurrence, occurrenceOK := artifact.OccurrenceAt(occurrenceIndex)
		if !occurrenceOK {
			t.Fatalf("artifact occurrence %d unavailable", occurrenceIndex)
		}
		kind := occurrence.Kind()
		if _, wanted := required[kind]; !wanted {
			continue
		}
		required[kind] = true
		placements := 0
		for ruleIndex := 0; ruleIndex < artifact.RuleOccurrenceCount(programartifact.RuleRoleValueStorageTransfer); ruleIndex++ {
			rule, ruleOK := artifact.RuleOccurrenceAt(programartifact.RuleRoleValueStorageTransfer, ruleIndex)
			if !ruleOK || rule.ID() != occurrence.ID() {
				continue
			}
			placements++
			point, pointOK := rule.PointAt(0)
			input, inputOK := rule.InputPoint()
			if !pointOK || !inputOK || point == input || rule.Stage() != programartifact.RuleStageLocal {
				t.Fatalf("kind=%d rule=%d is not a distinct Local placement", kind, ruleIndex)
			}
			switch kind {
			case programartifact.OccurrenceStorageRead, programartifact.OccurrenceStorageBindTransfer:
				if rule.InputKind() != programartifact.RuleInputEntry {
					t.Fatalf("kind=%d rule=%d did not retain Program Entry", kind, ruleIndex)
				}
			case programartifact.OccurrenceStorageWrite:
				route, routeOK := rule.PredecessorRouteID()
				if rule.InputKind() != programartifact.RuleInputPredecessor || !routeOK || !route.Available() {
					t.Fatalf("write rule=%d did not retain exact predecessor route", ruleIndex)
				}
			}
			localParent := false
			for edgeIndex := 0; edgeIndex < artifact.LocalTransferCount(); edgeIndex++ {
				edge, edgeOK := artifact.LocalTransferAt(edgeIndex)
				if !edgeOK || edge.To() != point {
					continue
				}
				for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
					parent, parentOK := occurrence.PointAt(pointIndex)
					if parentOK && parent == edge.From() {
						localParent = true
					}
				}
			}
			if !localParent {
				t.Fatalf("kind=%d rule=%d lacks Finish -> Local transport", kind, ruleIndex)
			}
		}
		if placements == 0 {
			t.Fatalf("kind=%d has no ValueStorageTransfer placement", kind)
		}
	}
	for kind, found := range required {
		if !found {
			t.Fatal(fmt.Sprintf("fixture did not issue storage occurrence kind=%d", kind))
		}
	}
}
