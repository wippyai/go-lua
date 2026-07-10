package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestResultShapeExhaustivenessProofsRequireDiscriminantCase(t *testing.T) {
	result, err := CheckFunction(parseFunction(t, `
function f(r: { ok: true, value: string } | { ok: false, error: string }): string
	return r.value
end
`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	var proofs []ResultShapeExhaustivenessProof
	result.ForEachResultShapeExhaustivenessProof(func(proof ResultShapeExhaustivenessProof) bool {
		proofs = append(proofs, proof)
		return true
	})
	if len(proofs) != 1 {
		t.Fatalf("proofs len = %d, want 1 (%#v)", len(proofs), proofs)
	}
	if proofs[0].ReadLabel != "r.value" || proofs[0].RequiredCase != "r.ok == true" {
		t.Fatalf("proof = %#v, want r.value requiring r.ok == true", proofs[0])
	}
}
