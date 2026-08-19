package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The committed program publishes one native Call stage per selected CallEffect
// occurrence, addressed by the owning role capability. The law fences that
// issuance on three axes: the stage a selected occurrence resolves carries the
// exact cold coordinates and a member; the opaque role, which is a distinct
// capability over the same coordinates, resolves none; and a Call occurrence
// the artifact never sealed resolves none.
func TestCallsiteMountedSelectedCallEffectStageOwnerFenceLaw(t *testing.T) {
	plan, _, compileDiagnostics := fixtureCompile(t, "advice/always-true-guard")
	if plan.state == nil || plan.state.binding == nil || plan.state.artifacts == nil {
		t.Fatalf("callsite native-stage compile diagnostics=%+v", compileDiagnostics)
	}
	if runtimeDiagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated || plan.state.committed.program == nil {
		t.Fatalf("callsite native-stage runtime topology=%+v", runtimeDiagnostic)
	}

	var mount, occurrence, point identity.ContentID
	for _, mounted := range plan.state.artifacts.mounts {
		if !mounted.valid() {
			continue
		}
		for index := 0; index < mounted.snapshot.RulePlacementCount(); index++ {
			row, rowOK := mounted.snapshot.RulePlacementAt(index)
			if !rowOK || row.Key() != "effect-selected" || !row.OccurrenceID().Available() || !row.PointID().Available() {
				continue
			}
			mount, occurrence, point = mounted.moduleKey, row.OccurrenceID(), row.PointID()
			break
		}
		if mount.Available() {
			break
		}
	}
	if !mount.Available() || !occurrence.Available() || !point.Available() {
		t.Fatal("fixture has no selected CallEffect occurrence")
	}

	selected, selectedOK := mountedCapability(plan.state.binding, "effect-selected")
	opaque, opaqueOK := mountedCapability(plan.state.binding, "effect-opaque")
	if !selectedOK || !opaqueOK || selected == opaque {
		t.Fatal("rule table did not publish two distinct callsite effect capabilities")
	}
	committed := plan.state.committed.program
	stage, stageOK := committed.MountedNativeCallStage(selected, mount, occurrence)
	if !stageOK || !stage.Available() || stage.Kind() != rows.ArtifactRuleStageIssued5 || stage.MountID() != mount || stage.OccurrenceID() != occurrence || stage.PointID() != point || !stage.HasMember() {
		t.Fatal("selected callsite did not publish its exact cold CallEffect-stage row")
	}
	if opaqueStage, ok := committed.MountedNativeCallStage(opaque, mount, occurrence); ok || opaqueStage.Available() {
		t.Fatal("opaque callsite published a selected CallEffect-stage row")
	}
	foreign := occurrence
	foreign[0] ^= 0xFF
	if foreign == occurrence {
		foreign[1] ^= 0xFF
	}
	if candidate, ok := committed.MountedNativeCallStage(selected, mount, foreign); ok || candidate.Available() {
		t.Fatal("foreign Call occurrence entered the selected callsite stage directory")
	}
}
