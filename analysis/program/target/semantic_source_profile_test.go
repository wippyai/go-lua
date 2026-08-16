package target_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
	"github.com/wippyai/go-lua/analysis/schema/relations"
)

// The profile owns the one closed string.gsub table-replacement branch. This
// law exercises TargetGsub's positive cardinality separately from the
// zero-row fixture in target's owner laws.
func TestSourcePublicationsMatchProfileGsubProjection(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := contract.SemanticSourceReceipt()
	if !ok {
		t.Fatal("profile Contract has no semantic-source publication")
	}
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	publications := receipt.Publications(schema)
	var published int
	found := false
	for _, publication := range publications {
		token := publication.Definition().Token()
		if token.Origin() == semanticsource.OriginTargetGsub && token.Facet() == 0 {
			published, found = publication.Count(), true
		}
	}
	if !found {
		t.Fatal("TargetGsub publication missing")
	}
	want := 0
	for index := 0; index < contract.OperationCount(); index++ {
		op, operationOK := contract.OperationAt(index)
		if !operationOK {
			t.Fatalf("OperationAt(%d)", index)
		}
		if _, _, _, _, _, replacementOK := contract.GsubTableReplacement(op); replacementOK {
			want++
		}
	}
	if published != want || want == 0 {
		t.Fatalf("TargetGsub publication = %d, typed projection = %d", published, want)
	}
}
