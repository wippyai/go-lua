package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

func TestCallsiteMountedSelectedCallEffectStageOwnerFenceLaw(t *testing.T) {
	project, err := testfixture.FrozenCorpusProject("advice/always-true-guard")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatal(err)
	}
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.binding == nil || plan.state.artifacts == nil {
		t.Fatalf("callsite native-stage compile=%v diagnostics=%+v", status, diagnostics)
	}
	defer plan.Close()
	if runtimeDiagnostic, instantiated := plan.state.instantiateRuntimeTopology(); !instantiated || plan.state.graph == nil {
		t.Fatalf("callsite native-stage runtime topology=%+v", runtimeDiagnostic)
	}

	var mount, occurrence, point keyspace.ContentID
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

	receipt, receiptOK := plan.state.binding.effectSelected.MountedSelectedCallEffectStage(plan.state.graph, mount, occurrence)
	_, memberOK := receipt.RuleMember()
	if !receiptOK || !receipt.Available() || receipt.Stage() != engine.ArtifactRuleStageCallEffect || receipt.MountID() != mount || receipt.OccurrenceID() != occurrence || receipt.ReusablePointID() != point || !memberOK {
		t.Fatal("selected callsite did not issue its exact cold CallEffect-stage receipt")
	}
	if opaque, ok := plan.state.binding.effectOpaque.MountedSelectedCallEffectStage(plan.state.graph, mount, occurrence); ok || opaque.Available() {
		t.Fatal("opaque callsite issued a selected CallEffect-stage receipt")
	}
	foreign := occurrence
	foreign[0] ^= 0xFF
	if foreign == occurrence {
		foreign[1] ^= 0xFF
	}
	if candidate, ok := plan.state.binding.effectSelected.MountedSelectedCallEffectStage(plan.state.graph, mount, foreign); ok || candidate.Available() {
		t.Fatal("foreign Call occurrence entered selected callsite stage inverse")
	}
}
