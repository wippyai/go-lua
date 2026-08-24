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
// does, keyed by a Key projection over those same rows.
func correspondingBase() Definition {
	source := specimenBase()
	source.Projections = append(source.Projections, Projection{
		Name:              "SeedCorrespondent",
		Key:               "specimen/correspondent-key",
		Relation:          "Candidates",
		CandidateProvider: source.Relations[0].CandidateProvider,
		Role:              member.Key,
		Result:            "KeyCarrier",
		Accessor:          specimenMethod("Correspondent", "Seed"),
	})
	source.Relations[0].Correspondences = []RelationCorrespondence{{
		Foreign:    member.RelationRef{Axis: correspondentAxis(), Member: "correspondent/candidates"},
		Coordinate: "SeedCorrespondent",
	}}
	return source
}

// TestADeclaredCorrespondenceResolvesTheProjectionItNames holds composition to
// the source vocabulary it is written in. Source names are the definition's
// own: a correspondence keyed by a projection name this source does not
// declare resolves to no catalog key, so composition refuses rather than
// emitting a coordinate no consumer can reach.
//
// The laws of the statement itself - that the order is foreign, that the key
// is a Key projection over these rows, that one axis is named at most once -
// are the catalog's, stated once where the composed rows live. This altitude
// states only what it alone knows.
func TestADeclaredCorrespondenceResolvesTheProjectionItNames(t *testing.T) {
	source := correspondingBase()
	source.Relations[0].Correspondences[0].Coordinate = "SeedAbsent"
	if _, ok := source.Catalog(); ok {
		t.Fatal("a correspondence naming a projection this source does not declare composes into nothing")
	}
}

// TestACorrespondenceSurvivesIntoTheColdCatalog is the consumer law. A child
// Program reads the composed catalog and never this owner's source names, so
// the statement that a foreign candidate addresses these rows has to arrive
// carrying the projection KEY the catalog holds.
func TestACorrespondenceSurvivesIntoTheColdCatalog(t *testing.T) {
	catalog, ok := correspondingBase().Catalog()
	if !ok {
		t.Fatal("a definition stating one correspondence composes")
	}
	relation, relationOK := catalog.Relation("specimen/candidates")
	if !relationOK || len(relation.Correspondences) != 1 {
		t.Fatalf("the cold relation carries exactly its declared correspondences, got %+v", relation.Correspondences)
	}
	stated := relation.Correspondences[0]
	if stated.Foreign.Axis.Key != "correspondent" || stated.Foreign.Member != "correspondent/candidates" {
		t.Fatalf("the cold correspondence names the foreign order it was declared against, got %+v", stated.Foreign)
	}
	if stated.Coordinate != "specimen/correspondent-key" {
		t.Fatalf("the cold correspondence carries the projection key, not the source name, got %q", stated.Coordinate)
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
