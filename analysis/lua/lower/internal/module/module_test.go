package module

import "testing"

func TestEmptyModuleCensusHasNoUnresolvedEvidence(t *testing.T) {
	census := Census{}
	writer := New(nil, census)
	if census.Count() != 0 || !writer.Clean() {
		t.Fatalf("empty module writer = count %d clean %v, want 0/true", census.Count(), writer.Clean())
	}
}
