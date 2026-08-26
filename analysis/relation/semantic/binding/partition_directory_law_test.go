package binding_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestPartitionDirectoryIsTotalExactAndPreservesAuthenticatedEmpty(t *testing.T) {
	value := newPartitionDirectoryFixture(t)
	populationMembership, ok := binding.NewMembershipView(value.relation, []model.RowID{value.populationRow})
	if !ok {
		t.Fatal("population membership")
	}
	population, ok := value.issuer.IssueDenominator(value.population, populationMembership, partitionContent(t, "population-evidence"))
	if !ok {
		t.Fatal("population witness")
	}
	emptyMembership, ok := binding.NewMembershipView(value.relation, []model.RowID{})
	if !ok {
		t.Fatal("empty child membership")
	}
	emptyChild, ok := value.issuer.IssueDenominator(value.child, emptyMembership, partitionContent(t, "empty-child-evidence"))
	if !ok {
		t.Fatal("empty child witness")
	}
	entries := map[model.RowID]binding.DenominatorWitness{value.populationRow: emptyChild}
	directory, ok := value.issuer.IssuePartitionDirectory(partitionContent(t, "partition"), value.population, value.child, population, entries)
	if !ok || !directory.Available() || !directory.ValidFor(value.fence) {
		t.Fatal("directory")
	}
	posting, ok := directory.Lookup(value.populationRow)
	if !ok || !posting.Available() || posting.Len() != 0 || !posting.Matches(value.child) {
		t.Fatal("authenticated empty posting was not redeemable")
	}
	if _, ok := directory.Lookup(value.foreignRow); ok {
		t.Fatal("foreign q row accepted")
	}
	entries[value.populationRow] = value.foreignWitness
	posting, ok = directory.Lookup(value.populationRow)
	if !ok || posting.Len() != 0 {
		t.Fatal("directory retained caller map alias")
	}

	emptyPopulationMembership, ok := binding.NewMembershipView(value.relation, []model.RowID{})
	if !ok {
		t.Fatal("empty population membership")
	}
	emptyPopulation, ok := value.issuer.IssueDenominator(value.population, emptyPopulationMembership, partitionContent(t, "empty-population-evidence"))
	if !ok {
		t.Fatal("empty population witness")
	}
	emptyDirectory, ok := value.issuer.IssuePartitionDirectory(partitionContent(t, "empty-partition"), value.population, value.child, emptyPopulation, map[model.RowID]binding.DenominatorWitness{})
	if !ok || !emptyDirectory.Available() || emptyDirectory.Digest() == (identity.ContentID{}) {
		t.Fatal("authenticated empty directory")
	}
}

func TestPartitionDirectoryRejectsMissingForeignAndStaleEvidence(t *testing.T) {
	value := newPartitionDirectoryFixture(t)
	populationMembership, _ := binding.NewMembershipView(value.relation, []model.RowID{value.populationRow, value.secondPopulationRow})
	population, _ := value.issuer.IssueDenominator(value.population, populationMembership, partitionContent(t, "population-evidence/hostile"))
	childMembership, _ := binding.NewMembershipView(value.relation, []model.RowID{value.childRow})
	child, _ := value.issuer.IssueDenominator(value.child, childMembership, partitionContent(t, "child-evidence/hostile"))
	if _, ok := value.issuer.IssuePartitionDirectory(partitionContent(t, "missing"), value.population, value.child, population, map[model.RowID]binding.DenominatorWitness{value.populationRow: child}); ok {
		t.Fatal("missing population entry accepted")
	}
	foreignRow := value.foreignRow
	if _, ok := value.issuer.IssuePartitionDirectory(partitionContent(t, "foreign"), value.population, value.child, population, map[model.RowID]binding.DenominatorWitness{value.populationRow: child, value.secondPopulationRow: child, foreignRow: child}); ok {
		t.Fatal("foreign population entry accepted")
	}
	foreignChildRelation, _ := model.IssueRelationID(value.owner, partitionContent(t, "foreign-child-relation"))
	foreignChildKey, _ := model.IssueKeyID(foreignChildRelation, partitionContent(t, "foreign-child-key"))
	foreignChildRef, _ := model.NewDenominatorRef(foreignChildRelation, foreignChildKey)
	foreignMembership, _ := binding.NewMembershipView(foreignChildRelation, []model.RowID{value.foreignChildRow})
	foreignChild, _ := value.issuer.IssueDenominator(foreignChildRef, foreignMembership, partitionContent(t, "foreign-child-evidence"))
	if _, ok := value.issuer.IssuePartitionDirectory(partitionContent(t, "foreign-child"), value.population, value.child, population, map[model.RowID]binding.DenominatorWitness{value.populationRow: foreignChild, value.secondPopulationRow: child}); ok {
		t.Fatal("foreign child witness accepted")
	}
	staleFence, _ := binding.NewFence(value.schema, value.fence.Mount(), value.fence.Generation()+1)
	if directory, ok := value.issuer.IssuePartitionDirectory(partitionContent(t, "stale-check"), value.population, value.child, population, map[model.RowID]binding.DenominatorWitness{value.populationRow: child, value.secondPopulationRow: child}); ok && directory.ValidFor(staleFence) {
		t.Fatal("stale directory fence accepted")
	}
}

type partitionDirectoryFixture struct {
	owner               model.OwnerID
	schema              model.SchemaID
	relation            model.RelationID
	population          model.DenominatorRef
	child               model.DenominatorRef
	populationRow       model.RowID
	secondPopulationRow model.RowID
	childRow            model.RowID
	foreignRow          model.RowID
	foreignChildRow     model.RowID
	foreignWitness      binding.DenominatorWitness
	fence               binding.Fence
	issuer              binding.Issuer
}

func newPartitionDirectoryFixture(t *testing.T) partitionDirectoryFixture {
	t.Helper()
	owner, _ := model.IssueOwnerID(partitionContent(t, "owner"))
	schema, _ := model.IssueSchemaID(owner, partitionContent(t, "schema"))
	relation, _ := model.IssueRelationID(owner, partitionContent(t, "relation"))
	populationKey, _ := model.IssueKeyID(relation, partitionContent(t, "population-key"))
	childKey, _ := model.IssueKeyID(relation, partitionContent(t, "child-key"))
	population, _ := model.NewDenominatorRef(relation, populationKey)
	child, _ := model.NewDenominatorRef(relation, childKey)
	populationRow, _ := model.IssueRowID(relation, partitionContent(t, "population-row"))
	secondPopulationRow, _ := model.IssueRowID(relation, partitionContent(t, "population-row/second"))
	childRow, _ := model.IssueRowID(relation, partitionContent(t, "child-row"))
	foreignRow, _ := model.IssueRowID(relation, partitionContent(t, "foreign-row"))
	foreignChildRow, _ := model.IssueRowID(relation, partitionContent(t, "foreign-child-row"))
	fence, _ := binding.NewFence(schema, identity.MountID{1}, identity.Generation(1))
	issuer, _ := binding.NewIssuer(fence)
	foreignMembership, _ := binding.NewMembershipView(relation, []model.RowID{childRow})
	foreignWitness, _ := issuer.IssueDenominator(population, foreignMembership, partitionContent(t, "foreign-witness"))
	return partitionDirectoryFixture{owner: owner, schema: schema, relation: relation, population: population, child: child, populationRow: populationRow, secondPopulationRow: secondPopulationRow, childRow: childRow, foreignRow: foreignRow, foreignChildRow: foreignChildRow, foreignWitness: foreignWitness, fence: fence, issuer: issuer}
}

func partitionContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("semantic-binding-partition-directory-law/v1", []byte(label))
	if !ok {
		t.Fatal("partition content")
	}
	return value
}
