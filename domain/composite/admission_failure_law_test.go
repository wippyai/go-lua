package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
)

func admissionLawModule(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("admission-failure-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

// TestRefusedActivationAdmissionNamesItsOperands states the observability law
// for a refused activation admission: the walk names the pass and the rule,
// and the rule's own evidence recovers at activation's type with both module
// identities intact. A refused occurrence is therefore never reported as a
// bare rule name.
func TestRefusedActivationAdmissionNamesItsOperands(t *testing.T) {
	trigger := admissionLawModule(t, "module/trigger")
	body := admissionLawModule(t, "module/body")
	evidence := callactivation.Refusal{Reason: callactivation.RefusalBodyNotResident, Trigger: trigger, Body: body}

	failure := RefusedAdmission(AdmissionStageActivation, DiagnosticRule(3), axis.NewCell(evidence))
	if !failure.Available() || failure.Stage != AdmissionStageActivation || failure.Rule != DiagnosticRule(3) {
		t.Fatalf("verdict is %s, want the activation pass at rule slot 3", failure)
	}
	recovered, ok := AdmissionRejection[callactivation.Refusal](failure)
	if !ok {
		t.Fatal("the activation lane's own evidence does not recover at its own type")
	}
	if recovered.Reason != callactivation.RefusalBodyNotResident || recovered.Trigger != trigger || recovered.Body != body {
		t.Fatalf("recovered evidence is %s, want the refused route's own operands", recovered)
	}
}

// TestAdmissionVerdictsWithoutRuleEvidenceRecoverNothing states the other half
// of the law: a pass that raised no rule evidence, and a caller that names the
// wrong evidence type, both receive the absent value rather than a guess.
func TestAdmissionVerdictsWithoutRuleEvidenceRecoverNothing(t *testing.T) {
	if failure := RefusedAdmission(AdmissionStageNone, DiagnosticRule(1), axis.NewCell(callactivation.Refusal{Reason: callactivation.RefusalInput})); failure.Available() {
		t.Fatalf("an unreached pass renders as %s, want an absent verdict", failure)
	}
	placement := RefusedAdmissionRule(AdmissionStagePlacement, DiagnosticRuleUnknown)
	if !placement.Available() {
		t.Fatal("a refused placement walk renders as an absent verdict")
	}
	if _, ok := AdmissionRejection[callactivation.Refusal](placement); ok {
		t.Fatal("a walk that raised no rule evidence recovered activation evidence")
	}
	activation := RefusedAdmission(AdmissionStageActivation, DiagnosticRule(3), axis.NewCell(callactivation.Refusal{Reason: callactivation.RefusalInput}))
	if _, ok := AdmissionRejection[MountFailure](activation); ok {
		t.Fatal("activation evidence recovered at a foreign evidence type")
	}
}
