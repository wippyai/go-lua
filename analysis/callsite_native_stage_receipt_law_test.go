package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"

	"github.com/wippyai/go-lua/domain/composite"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"

	"github.com/wippyai/go-lua/analysis/identity"
)

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

	selected, selectedOK := composite.RuleHandleByKey[*callsite.HotRule](plan.state.binding.Rules(), "effect-selected")
	opaqueRule, opaqueRuleOK := composite.RuleHandleByKey[*callsite.HotRule](plan.state.binding.Rules(), "effect-opaque")
	if !selectedOK || !opaqueRuleOK {
		t.Fatal("rule table did not publish the callsite effect rules")
	}
	compilation, compiled := plan.state.beginRuntimeConstruction()
	if !compiled || compilation == nil {
		t.Fatal("callsite native-stage construction")
	}
	receipt, receiptOK := selected.MountedSelectedCallEffectStage(compilation, mount, occurrence)
	if !receiptOK || !receipt.Available() || receipt.Kind() != rows.ArtifactRuleStageIssued5 || receipt.MountID() != mount || receipt.OccurrenceID() != occurrence || receipt.PointID() != point || !receipt.HasMember() {
		t.Fatal("selected callsite did not issue its exact cold CallEffect-stage receipt")
	}
	if opaque, ok := opaqueRule.MountedSelectedCallEffectStage(compilation, mount, occurrence); ok || opaque.Available() {
		t.Fatal("opaque callsite issued a selected CallEffect-stage receipt")
	}
	foreign := occurrence
	foreign[0] ^= 0xFF
	if foreign == occurrence {
		foreign[1] ^= 0xFF
	}
	if candidate, ok := selected.MountedSelectedCallEffectStage(compilation, mount, foreign); ok || candidate.Available() {
		t.Fatal("foreign Call occurrence entered selected callsite stage inverse")
	}
}
