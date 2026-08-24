package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func correspondentAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "correspondent"}
}

// correspondingBase is the specimen axis with its candidate directory stating
// that its own sealed order enumerates the same subjects a foreign axis's
// order does.
func correspondingBase() Definition {
	source := specimenBase()
	source.Relations[0].Correspondences = []member.RelationRef{{Axis: correspondentAxis(), Member: "correspondent/candidates"}}
	return source
}

// TestADeclaredCorrespondenceSurvivesIntoTheColdCatalog is this altitude's
// whole obligation. The statement carries no Go symbol and no source name to
// resolve, so composition has nothing to validate and everything to carry: a
// child Program reads the composed catalog and never this owner's source, and
// the statement that a foreign candidate addresses these rows has to arrive
// with it. The laws of the statement are the catalog's, stated once where the
// composed rows live.
func TestADeclaredCorrespondenceSurvivesIntoTheColdCatalog(t *testing.T) {
	catalog, ok := correspondingBase().Catalog()
	if !ok {
		t.Fatal("a definition stating one correspondence composes")
	}
	relation, relationOK := catalog.Relation("specimen/candidates")
	if !relationOK || len(relation.Correspondences) != 1 {
		t.Fatalf("the cold relation carries exactly its declared correspondences, got %+v", relation.Correspondences)
	}
	stated := relation.Correspondences[0]
	if stated.Axis.Key != "correspondent" || stated.Member != "correspondent/candidates" {
		t.Fatalf("the cold correspondence names the foreign order it was declared against, got %+v", stated)
	}
	plain, plainOK := specimenBase().Catalog()
	if !plainOK {
		t.Fatal("the specimen base composes")
	}
	base, baseOK := plain.Relation("specimen/candidates")
	if !baseOK || base.Correspondences != nil {
		t.Fatalf("a relation that correlates with nothing carries no correspondence, got %+v", base.Correspondences)
	}
}

// TestTwoDeclarationsOfOneRelationStateTheSameCorrespondences keeps the roster
// fold honest: a relation the base already declares may be repeated verbatim
// by a contribution, and a repeat that names different foreign orders is two
// declarations of one relation rather than one.
func TestTwoDeclarationsOfOneRelationStateTheSameCorrespondences(t *testing.T) {
	left := correspondingBase().Relations[0]
	right := left
	right.Correspondences = nil
	if relationsAgree(left, right) {
		t.Fatal("a repeat that drops a correspondence disagrees with the declaration it repeats")
	}
	if !relationsAgree(left, correspondingBase().Relations[0]) {
		t.Fatal("two identical declarations of one relation agree")
	}
}
