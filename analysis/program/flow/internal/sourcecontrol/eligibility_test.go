package sourcecontrol

import "testing"

func TestTableFieldThrowEligibilityRequiresASealedGraph(t *testing.T) {
	if (TableFieldThrowEligibility{}).Available() {
		t.Fatal("zero TableField eligibility proof reported availability")
	}
	if proof, err := (&Result{}).TableFieldThrowEligibility(zeroSourceControlSourceView(), zeroSourceControlFlowView(), nil, 0, 0); err == nil || proof.Available() {
		t.Fatalf("unavailable graph produced eligibility proof: proof=%v err=%v", proof, err)
	}
}
