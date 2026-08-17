package targetingress

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestTargetIngressBuildClosesEveryTargetRowToDeclaredParents(t *testing.T) {
	evidence, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.ValidateRows(targetVocabulary(denominator.GeneratedRelationEntries())); err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) == 0 || evidence.Digest == "" {
		t.Fatalf("target ingress evidence = %#v, want rows and digest", evidence)
	}
}
