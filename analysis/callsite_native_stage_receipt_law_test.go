package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"

	"github.com/wippyai/go-lua/domain/composite"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func TestCallsiteMountedSelectedCallEffectStageOwnerFenceLaw(t *testing.T) {
	plan, _, compileDiagnostics := fixtureCompile(t, "advice/always-true-guard")
	if plan.state == nil || plan.state.binding == nil || plan.state.artifacts == nil {
		t.Fatalf("callsite native-stage compile diagnostics=%+v", compileDiagnostics)
	}
	if runtimeDiagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated || plan.state.graph == nil {
		t.Fatalf("callsite native-stage runtime topology=%+v", runtimeDiagnostic)
	}

	var mount, occurrence, point identity.ContentID
	for _, mounted := range plan.state.artifacts.mounts {
		if mounted.artifact == nil || mounted.artifact.RuleOccurrenceCount(programartifact.RuleRoleEffectSelected) == 0 {
			continue
		}
		row, rowOK := mounted.artifact.RuleOccurrenceAt(programartifact.RuleRoleEffectSelected, 0)
		if !rowOK || row.PointCount() != 1 {
			t.Fatal("selected CallEffect artifact occurrence")
		}
		var pointOK bool
		mount, occurrence = mounted.moduleKey, row.ID()
		point, pointOK = row.PointAt(0)
		if !pointOK {
			t.Fatal("selected CallEffect artifact point")
		}
		break
	}
	if !mount.Available() || !occurrence.Available() || !point.Available() {
		t.Fatal("fixture has no selected CallEffect occurrence")
	}

	selected, selectedOK := composite.RuleHandle[*callsite.HotRule](plan.state.binding.Rules(), programartifact.RuleRoleEffectSelected)
	opaqueRule, opaqueRuleOK := composite.RuleHandle[*callsite.HotRule](plan.state.binding.Rules(), programartifact.RuleRoleEffectOpaque)
	if !selectedOK || !opaqueRuleOK {
		t.Fatal("rule table did not publish the callsite effect rules")
	}
	receipt, receiptOK := selected.MountedSelectedCallEffectStage(plan.state.graph, mount, occurrence)
	_, memberOK := receipt.RuleMember()
	if !receiptOK || !receipt.Available() || receipt.Stage() != rows.ArtifactRuleStageCallEffect || receipt.MountID() != mount || receipt.OccurrenceID() != occurrence || receipt.ReusablePointID() != point || !memberOK {
		t.Fatal("selected callsite did not issue its exact cold CallEffect-stage receipt")
	}
	if opaque, ok := opaqueRule.MountedSelectedCallEffectStage(plan.state.graph, mount, occurrence); ok || opaque.Available() {
		t.Fatal("opaque callsite issued a selected CallEffect-stage receipt")
	}
	foreign := occurrence
	foreign[0] ^= 0xFF
	if foreign == occurrence {
		foreign[1] ^= 0xFF
	}
	if candidate, ok := selected.MountedSelectedCallEffectStage(plan.state.graph, mount, foreign); ok || candidate.Available() {
		t.Fatal("foreign Call occurrence entered selected callsite stage inverse")
	}
}
