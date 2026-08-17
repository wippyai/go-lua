package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
)

// publishedQueryFamilies is the analyzer's published query identity space,
// spelled by the exported constants a consumer opens a result slot with. It is
// the independent statement of what the sealed surface owes: a family declared
// under no published key, or a published key no family declares, is a
// disagreement between the inventory and the identities the analyzer exports.
func publishedQueryFamilies() []schema.Key {
	return []schema.Key{QueryFamilyValueSummary, QueryFamilyEffectExact}
}

// TestQueryTableSeals states that the authored query inventory is admitted and
// sealed by the one declaration root, and that every family reads a coordinate
// space the same table declares.
func TestQueryTableSeals(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	specs, specsOK := queryRegistrationSpecs(roles)
	if !rolesOK || !specsOK {
		t.Fatal("declared query identities did not resolve")
	}
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
	view, viewOK := sealed.Surface(schema.SurfaceKindQuery)
	if !axesOK || !viewOK {
		t.Fatal("sealed table holds no axis or query surface")
	}
	families := publishedQueryFamilies()
	if view.Count() != len(families) {
		t.Fatalf("sealed query surface holds %d rows for %d published families", view.Count(), len(families))
	}
	for _, family := range families {
		if _, declared := view.ByID(schema.NewEntryID(schema.SurfaceKindQuery, family)); !declared {
			t.Fatalf("published family %q is declared by no sealed row", family)
		}
	}
	for position, spec := range specs {
		row, rowOK := view.At(position)
		registration, registrationOK := row.(*query.Registration)
		if !rowOK || !registrationOK || registration.Key() != spec.Family {
			t.Fatalf("query row %d is not the authored family %q", position, spec.Family)
		}
		for index := 0; index < registration.SubjectCount(); index++ {
			subject, subjectOK := registration.SubjectAt(index)
			if !subjectOK {
				t.Fatalf("family %q holds no subject at %d", spec.Family, index)
			}
			if _, declared := axes.ByID(schema.NewEntryID(schema.SurfaceKindAxis, subject)); !declared {
				t.Fatalf("family %q reads axis %q, which is not declared", spec.Family, subject)
			}
		}
	}
}

// TestQueryCodecsAreTheSchemaFreezerIdentities is the drift law of this
// inventory: a family is published under the same freezer identity the sealed
// schema opens its query slot with, so the declaration and the slot cannot
// name two contracts.
func TestQueryCodecsAreTheSchemaFreezerIdentities(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	specs, specsOK := queryRegistrationSpecs(roles)
	if !rolesOK || !specsOK {
		t.Fatal("declared query identities did not resolve")
	}
	declared := make(map[schema.Key]identity.ContentID)
	for _, spec := range specs {
		declared[spec.Family] = spec.Codec
	}
	valueCodec, valueCodecOK := roles.Key("semantic/query-result/value-summary")
	effectCodec, effectCodecOK := roles.Key("semantic/query-result/effect-exact")
	if !valueCodecOK || !effectCodecOK {
		t.Fatal("declared query codec roles did not resolve")
	}
	for family, codec := range map[schema.Key]identity.ContentID{
		QueryFamilyValueSummary: identity.ContentID(valueCodec.Digest()),
		QueryFamilyEffectExact:  identity.ContentID(effectCodec.Digest()),
	} {
		if declared[family] != codec {
			t.Fatalf("family %q is declared under a codec the schema does not freeze its results with", family)
		}
	}
}
