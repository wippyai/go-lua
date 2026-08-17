package grammarproof

import "testing"

// TestGeneratedGrammarproofEvidenceHasSemanticRowsAndDigests binds the
// generated artifact to its actual proof relation. A non-empty file alone is
// not evidence that reductions and ingress are committed together.
func TestGeneratedGrammarproofEvidenceHasSemanticRowsAndDigests(t *testing.T) {
	if Generated.Digest == "" || Generated.TraceDigest == "" || Generated.IngressDigest == "" {
		t.Fatalf("generated grammarproof digests = %#v, want all populated", Generated)
	}
	if len(Generated.Productions) == 0 || len(Generated.Ingress) == 0 {
		t.Fatalf("generated grammarproof rows = productions %d ingress %d, want both non-empty", len(Generated.Productions), len(Generated.Ingress))
	}
	if Generated.Productions[0].Key == "" || Generated.Productions[0].Witness == "" {
		t.Fatalf("generated production row = %#v, want keyed witness", Generated.Productions[0])
	}
}
