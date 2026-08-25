package algebra_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestKindVocabularyIsClosed(t *testing.T) {
	kinds := algebra.Kinds()
	if len(kinds) != algebra.KindCount || len(kinds) != 9 {
		t.Fatalf("Kinds length = %d, want nine", len(kinds))
	}
	seen := make(map[algebra.Kind]bool, len(kinds))
	for index, kind := range kinds {
		if seen[kind] {
			t.Fatalf("duplicate kind at %d: %d", index, kind)
		}
		seen[kind] = true
		if kind != algebra.Kind(index+1) {
			t.Errorf("kind %d = %d, want canonical ordinal %d", index, kind, index+1)
		}
	}
	if seen[algebra.KindInvalid] {
		t.Fatal("KindInvalid is admitted")
	}
}

func TestDigestIsDeterministicAndJoinOrientationSensitive(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{1})
	if !ok {
		t.Fatal("owner construction failed")
	}
	leftRelation, ok := model.IssueRelationID(owner, identity.ContentID{2})
	if !ok {
		t.Fatal("left relation construction failed")
	}
	rightRelation, ok := model.IssueRelationID(owner, identity.ContentID{3})
	if !ok {
		t.Fatal("right relation construction failed")
	}
	leftColumn, ok := model.IssueColumnID(leftRelation, identity.ContentID{4})
	if !ok {
		t.Fatal("left column construction failed")
	}
	rightColumn, ok := model.IssueColumnID(rightRelation, identity.ContentID{5})
	if !ok {
		t.Fatal("right column construction failed")
	}
	left := algebra.NewInput(leftRelation)
	right := algebra.NewInput(rightRelation)
	contract := algebra.NewJoinContract([]model.ColumnID{leftColumn}, []model.ColumnID{rightColumn})
	ab := algebra.NewJoin(left, right, contract)
	ba := algebra.NewJoin(right, left, contract)
	if ab.Digest() != algebra.NewJoin(left, right, contract).Digest() {
		t.Fatal("equal authored trees produced different digests")
	}
	if ab.Digest() == ba.Digest() {
		t.Fatal("oriented Join children have the same digest")
	}
	withDifferentColumns := algebra.NewJoinContract(nil, nil)
	if ab.Digest() == algebra.NewJoin(left, right, withDifferentColumns).Digest() {
		t.Fatal("declared join columns were omitted from structural digest")
	}
}

func TestJoinContractCopiesOrientedColumnVectors(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{31})
	if !ok {
		t.Fatal("owner construction failed")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{32})
	if !ok {
		t.Fatal("relation construction failed")
	}
	column, ok := model.IssueColumnID(relation, identity.ContentID{33})
	if !ok {
		t.Fatal("column construction failed")
	}
	left := []model.ColumnID{column}
	right := []model.ColumnID{column}
	contract := algebra.NewJoinContract(left, right)
	left[0] = model.ColumnID{}
	if len(contract.LeftColumns()) != 1 || contract.LeftColumns()[0] != column {
		t.Fatal("JoinContract did not defensively copy its logical vectors")
	}
	if len(contract.RightColumns()) != 1 {
		t.Fatal("JoinContract lost its right logical vector")
	}
}

func TestSelectReferencesCanonicalScopeID(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{21})
	if !ok {
		t.Fatal("owner construction failed")
	}
	scope, ok := model.IssueScopeID(owner, identity.ContentID{22})
	if !ok {
		t.Fatal("scope construction failed")
	}
	contract := algebra.NewSelectContract(algebra.SelectByScope, scope)
	if contract.Scope() != scope {
		t.Fatal("SelectContract did not retain the canonical ScopeID")
	}
}

func TestMergeCopiesChildrenAndPreservesOrderInDigest(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{11})
	if !ok {
		t.Fatal("owner construction failed")
	}
	firstRelation, ok := model.IssueRelationID(owner, identity.ContentID{12})
	if !ok {
		t.Fatal("first relation construction failed")
	}
	secondRelation, ok := model.IssueRelationID(owner, identity.ContentID{13})
	if !ok {
		t.Fatal("second relation construction failed")
	}
	first := algebra.NewInput(firstRelation)
	second := algebra.NewInput(secondRelation)
	children := []algebra.Expression{first, second}
	contract := algebra.NewMergeContract(model.KeyID{})
	merged := algebra.NewMerge(children, contract)
	before := merged.Digest()
	children[0] = second
	if merged.Digest() != before {
		t.Fatal("mutating constructor input changed immutable Merge")
	}
	returned := merged.Inputs()
	returned[0] = second
	if merged.Digest() != before {
		t.Fatal("mutating accessor result changed immutable Merge")
	}
	reversed := algebra.NewMerge([]algebra.Expression{second, first}, contract)
	if reversed.Digest() == before {
		t.Fatal("authored Merge order was lost from structural digest")
	}
}

func TestGroupDigestIncludesCanonicalCardinality(t *testing.T) {
	exactlyOne, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("exactly-one cardinality construction failed")
	}
	boundedMany, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("bounded-many cardinality construction failed")
	}
	child := algebra.NewInput(model.RelationID{})
	one := algebra.NewGroup(child, algebra.NewGroupContract(model.KeyID{}, exactlyOne))
	many := algebra.NewGroup(child, algebra.NewGroupContract(model.KeyID{}, boundedMany))
	if one.Digest() == many.Digest() {
		t.Fatal("Group cardinality was omitted from structural digest")
	}
}
