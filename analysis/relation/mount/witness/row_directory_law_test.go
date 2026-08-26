package witness

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestMountedRowsUnionsMembershipsAndUsesCanonicalIdentityOrder(t *testing.T) {
	owner, ok := model.IssueOwnerID(rowDirectoryContent(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, rowDirectoryContent(t, "schema"))
	if !ok {
		t.Fatal("schema")
	}
	relation, ok := model.IssueRelationID(owner, rowDirectoryContent(t, "relation"))
	if !ok {
		t.Fatal("relation")
	}
	firstKey, ok := model.IssueKeyID(relation, rowDirectoryContent(t, "key/first"))
	if !ok {
		t.Fatal("first key")
	}
	secondKey, ok := model.IssueKeyID(relation, rowDirectoryContent(t, "key/second"))
	if !ok {
		t.Fatal("second key")
	}
	firstRef, ok := model.NewDenominatorRef(relation, firstKey)
	if !ok {
		t.Fatal("first denominator")
	}
	secondRef, ok := model.NewDenominatorRef(relation, secondKey)
	if !ok {
		t.Fatal("second denominator")
	}
	rowA, ok := model.IssueRowID(relation, rowDirectoryContent(t, "row/a"))
	if !ok {
		t.Fatal("row a")
	}
	rowB, ok := model.IssueRowID(relation, rowDirectoryContent(t, "row/b"))
	if !ok {
		t.Fatal("row b")
	}
	rowC, ok := model.IssueRowID(relation, rowDirectoryContent(t, "row/c"))
	if !ok {
		t.Fatal("row c")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("runtime fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	firstMembership, ok := binding.NewMembershipView(relation, []model.RowID{rowB, rowA})
	if !ok {
		t.Fatal("first membership")
	}
	secondMembership, ok := binding.NewMembershipView(relation, []model.RowID{rowA, rowC})
	if !ok {
		t.Fatal("second membership")
	}
	firstWitness, ok := issuer.IssueDenominator(firstRef, firstMembership, rowDirectoryContent(t, "evidence/first"))
	if !ok {
		t.Fatal("first witness")
	}
	secondWitness, ok := issuer.IssueDenominator(secondRef, secondMembership, rowDirectoryContent(t, "evidence/second"))
	if !ok {
		t.Fatal("second witness")
	}
	refs := []model.DenominatorRef{secondRef, firstRef}
	witnesses := map[model.DenominatorRef]binding.DenominatorWitness{firstRef: firstWitness, secondRef: secondWitness}
	directory, ok := mountedRows(refs, witnesses, fence)
	if !ok {
		t.Fatal("overlapping denominator memberships refused")
	}
	rows := directory[relation]
	if len(rows) != 3 {
		t.Fatalf("union cardinality = %d, want 3", len(rows))
	}
	for index, row := range rows {
		if index > 0 && !rowLess(rows[index-1], row) {
			t.Fatalf("row directory is not strictly canonical at %d", index)
		}
		if rowIndex := rowIndex(rows, row); rowIndex != index {
			t.Fatalf("row inverse index = %d, want %d", rowIndex, index)
		}
	}
	if rows[0] == rows[1] || rows[1] == rows[2] || rows[0] == rows[2] {
		t.Fatal("overlapping membership was not deduplicated")
	}
	reversed, ok := mountedRows([]model.DenominatorRef{firstRef, secondRef}, witnesses, fence)
	if !ok || len(reversed[relation]) != len(rows) {
		t.Fatal("denominator declaration order changed the relation directory")
	}
	for index, row := range rows {
		if reversed[relation][index] != row {
			t.Fatalf("row directory permutation changed position %d", index)
		}
	}
}

func TestMountedRowsRejectsForeignAndStaleWitnesses(t *testing.T) {
	owner, ok := model.IssueOwnerID(rowDirectoryContent(t, "reject-owner"))
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, rowDirectoryContent(t, "reject-schema"))
	if !ok {
		t.Fatal("schema")
	}
	relation, ok := model.IssueRelationID(owner, rowDirectoryContent(t, "reject-relation"))
	if !ok {
		t.Fatal("relation")
	}
	key, ok := model.IssueKeyID(relation, rowDirectoryContent(t, "reject-key"))
	if !ok {
		t.Fatal("key")
	}
	ref, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	row, ok := model.IssueRowID(relation, rowDirectoryContent(t, "reject-row"))
	if !ok {
		t.Fatal("row")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{2}, identity.Generation(1))
	if !ok {
		t.Fatal("runtime fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	membership, ok := binding.NewMembershipView(relation, []model.RowID{row})
	if !ok {
		t.Fatal("membership")
	}
	witnessValue, ok := issuer.IssueDenominator(ref, membership, rowDirectoryContent(t, "reject-evidence"))
	if !ok {
		t.Fatal("witness")
	}
	witnesses := map[model.DenominatorRef]binding.DenominatorWitness{ref: witnessValue}
	staleFence, ok := binding.NewFence(schema, fence.Mount(), identity.Generation(2))
	if !ok {
		t.Fatal("stale fence")
	}
	if _, ok := mountedRows([]model.DenominatorRef{ref}, witnesses, staleFence); ok {
		t.Fatal("stale witness accepted")
	}
	foreignRelation, ok := model.IssueRelationID(owner, rowDirectoryContent(t, "reject-foreign-relation"))
	if !ok {
		t.Fatal("foreign relation")
	}
	foreignKey, ok := model.IssueKeyID(foreignRelation, rowDirectoryContent(t, "reject-foreign-key"))
	if !ok {
		t.Fatal("foreign key")
	}
	foreignRef, ok := model.NewDenominatorRef(foreignRelation, foreignKey)
	if !ok {
		t.Fatal("foreign denominator")
	}
	if _, ok := mountedRows([]model.DenominatorRef{foreignRef}, witnesses, fence); ok {
		t.Fatal("foreign denominator without admitted witness accepted")
	}
	if _, ok := mountedRows([]model.DenominatorRef{ref}, nil, fence); ok {
		t.Fatal("missing witness map accepted")
	}
}

func rowDirectoryContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("mount/witness/row-directory-law/v1", []byte(label))
	if !ok {
		t.Fatal("derive content")
	}
	return value
}

func rowIndex(rows []model.RowID, row model.RowID) int {
	for index, candidate := range rows {
		if candidate == row {
			return index
		}
	}
	return -1
}
