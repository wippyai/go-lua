package witness

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestCertificateDenominatorsIncludesJoinedInputSourceAuthority(t *testing.T) {
	owner := sourceAuthorityOwner(t)
	carrierRelation, carrierKey, carrierDenominator := sourceAuthorityDenominator(t, owner, "carrier")
	sourceRelation, sourceKey, sourceDenominator := sourceAuthorityDenominator(t, owner, "source")
	sourceColumn, ok := model.IssueColumnID(sourceRelation, sourceAuthorityToken(t, "column/source"))
	if !ok {
		t.Fatal("source column")
	}
	outputColumn, ok := model.IssueColumnID(carrierRelation, sourceAuthorityToken(t, "column/output"))
	if !ok {
		t.Fatal("output column")
	}
	typeID, ok := model.IssueTypeID(owner, sourceAuthorityToken(t, "type"))
	if !ok {
		t.Fatal("type")
	}
	delivery, ok := signature.NewCompleteSpanDelivery(carrierKey)
	if !ok {
		t.Fatal("delivery")
	}
	input, ok := signature.NewJoinedInput(sourceRelation, sourceColumn, typeID, signature.RequirePresent, delivery, sourceDenominator, carrierDenominator)
	if !ok {
		t.Fatal("joined input")
	}
	operationID, ok := model.IssueOperationID(owner, sourceAuthorityToken(t, "operation"))
	if !ok {
		t.Fatal("operation")
	}
	schemaID, ok := model.IssueSchemaID(owner, sourceAuthorityToken(t, "schema"))
	if !ok {
		t.Fatal("schema")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	codes, ok := outcome.NewSet(outcome.Produced)
	if !ok {
		t.Fatal("outcomes")
	}
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs:   []signature.Input{input},
		Outputs: []signature.Output{{
			Relation: carrierRelation, Column: outputColumn, Type: typeID,
			Presence: signature.ProducePresent, Denominator: carrierDenominator,
		}},
		Cardinality: cardinality,
		Outcomes:    codes,
	})
	if !ok {
		t.Fatal("signature")
	}
	refs, ok := certificateDenominators([]signature.Signature{operation}, nil, nil, nil, nil)
	if !ok || len(refs) != 2 {
		t.Fatalf("denominators = %#v, ok=%t", refs, ok)
	}
	seen := map[model.DenominatorRef]bool{}
	for _, ref := range refs {
		seen[ref] = true
	}
	if !seen[carrierDenominator] || !seen[sourceDenominator] || sourceKey != sourceDenominator.Key() {
		t.Fatalf("source/carrier authority missing: %#v", refs)
	}
}

func sourceAuthorityOwner(t *testing.T) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(sourceAuthorityToken(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	return owner
}

func sourceAuthorityDenominator(t *testing.T, owner model.OwnerID, label string) (model.RelationID, model.KeyID, model.DenominatorRef) {
	t.Helper()
	relation, ok := model.IssueRelationID(owner, sourceAuthorityToken(t, "relation/"+label))
	if !ok {
		t.Fatal("relation")
	}
	key, ok := model.IssueKeyID(relation, sourceAuthorityToken(t, "key/"+label))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return relation, key, denominator
}

func sourceAuthorityToken(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/relation/mount/witness/source-authority-law/v1", []byte(label))
	if !ok {
		t.Fatal("token")
	}
	return value
}
