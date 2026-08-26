package witness

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestCertificateDenominatorsAdmitsCompleteOnlyReferences(t *testing.T) {
	owner, ok := model.IssueOwnerID(completeDenominatorToken("owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, completeDenominatorToken("relation"))
	if !ok {
		t.Fatal("relation")
	}
	key, ok := model.IssueKeyID(relation, completeDenominatorToken("key"))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	refs, ok := certificateDenominators(nil, nil, nil, nil, []model.DenominatorRef{denominator})
	if !ok || len(refs) != 1 || refs[0] != denominator {
		t.Fatalf("Complete-only denominator was not admitted: %v %#v", ok, refs)
	}
}

func completeDenominatorToken(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("analysis/relation/mount/witness/complete-denominator-law/v1", []byte(label))
	return value
}
