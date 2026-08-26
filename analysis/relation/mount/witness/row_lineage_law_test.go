package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

// TestMountedRowLineageIsTheCanonicalAdmittedAtom exercises the complete
// row-directory projection: every admitted row has one stable atom, the atom
// is the row's owner/content identity, and the mounted authority validates it.
func TestMountedRowLineageIsTheCanonicalAdmittedAtom(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	lineageOwner := issueOwner(t, "row-lineage-authority")
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	mounted, ok := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("row-lineage mount refused")
	}
	authority, ok := mounted.Lineage()
	if !ok || authority == nil {
		t.Fatal("mounted lineage authority missing")
	}
	denominatorLineage, denominatorOK := mounted.DenominatorLineage(fixture.denominator)
	if !denominatorOK || !denominatorLineage.Available() || !authority.Validate(denominatorLineage) {
		t.Fatal("admitted denominator has no valid mounted lineage")
	}
	canonicalDenominator, canonicalDenominatorOK := model.IssueLineageRef(fixture.relation.Owner(), fixture.evidence.evidence)
	if !canonicalDenominatorOK || denominatorLineage != canonicalDenominator {
		t.Fatalf("denominator lineage = %#v, want evidence atom %#v", denominatorLineage, canonicalDenominator)
	}
	for index := 0; ; index++ {
		row, rowOK := mounted.RowAt(fixture.relation, index)
		if !rowOK {
			break
		}
		rowLineage, lineageOK := mounted.RowLineage(row)
		if !lineageOK || !rowLineage.Available() || !authority.Validate(rowLineage) {
			t.Fatalf("admitted row %d has no valid mounted lineage", index)
		}
		canonical, canonicalOK := model.IssueLineageRef(row.Owner(), row.Content())
		if !canonicalOK || rowLineage != canonical {
			t.Fatalf("row %d lineage = %#v, want canonical owner/content atom %#v", index, rowLineage, canonical)
		}
	}
	if _, ok := mounted.RowLineage(model.RowID{}); ok {
		t.Fatal("zero row received a mounted lineage")
	}
	foreignRelation := issueRelation(t, fixture.owner, "row-lineage-foreign-relation")
	foreignRow, ok := model.IssueRowID(foreignRelation, fixture.row.Content())
	if !ok {
		t.Fatal("foreign row")
	}
	if _, ok := mounted.RowLineage(foreignRow); ok {
		t.Fatal("foreign row crossed mounted row-lineage directory")
	}
	foreignKey := issueKey(t, foreignRelation, "row-lineage-foreign-key")
	foreignDenominator, ok := model.NewDenominatorRef(foreignRelation, foreignKey)
	if !ok {
		t.Fatal("foreign denominator")
	}
	if _, ok := mounted.DenominatorLineage(foreignDenominator); ok {
		t.Fatal("foreign denominator crossed mounted denominator-lineage directory")
	}
}

func TestMountedRowLineageSpecializationIsDeterministic(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	lineageOwner := issueOwner(t, "row-lineage-deterministic-authority")
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	newMount := func() witness.Mounted {
		mounted, mountOK := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, lineageFactory)
		if !mountOK || !mounted.Available() {
			t.Fatal("deterministic row-lineage mount refused")
		}
		return mounted
	}
	first, second := newMount(), newMount()
	if !first.Same(second) || first.Digest() != second.Digest() {
		t.Fatal("row-lineage specialization was not deterministic")
	}
	row, ok := first.RowAt(fixture.relation, 0)
	if !ok {
		t.Fatal("admitted row")
	}
	left, leftOK := first.RowLineage(row)
	right, rightOK := second.RowLineage(row)
	if !leftOK || !rightOK || left != right {
		t.Fatal("deterministic specializations produced different row lineage")
	}
	left, leftOK = first.DenominatorLineage(fixture.denominator)
	right, rightOK = second.DenominatorLineage(fixture.denominator)
	if !leftOK || !rightOK || left != right {
		t.Fatal("deterministic specializations produced different denominator lineage")
	}
}
