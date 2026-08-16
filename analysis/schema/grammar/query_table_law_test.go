package grammar

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/queryreg"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// TestQueryTableSeals states that the authored query inventory is admitted and
// sealed by the one declaration root, and that every family reads a coordinate
// space the same table declares.
func TestQueryTableSeals(t *testing.T) {
	bundle, bundleOK := vocabulary.New()
	if !bundleOK {
		t.Fatal("closed semantic vocabulary unavailable")
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
	specs := queryRegistrationSpecs(bundle)
	if view.Count() != len(specs) {
		t.Fatalf("sealed query surface holds %d of %d authored families", view.Count(), len(specs))
	}
	for position, spec := range specs {
		row, rowOK := view.At(position)
		registration, registrationOK := row.(*queryreg.Registration)
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
	bundle, bundleOK := vocabulary.New()
	if !bundleOK {
		t.Fatal("closed semantic vocabulary unavailable")
	}
	declared := make(map[schema.Key]identity.ContentID)
	for _, spec := range queryRegistrationSpecs(bundle) {
		declared[spec.Family] = spec.Codec
	}
	for family, codec := range map[schema.Key]identity.ContentID{
		QueryFamilyValueSummary: identity.ContentID(bundle.ValueCodec.Digest()),
		QueryFamilyEffectExact:  identity.ContentID(bundle.EffectCodec.Digest()),
	} {
		if declared[family] != codec {
			t.Fatalf("family %q is declared under a codec the schema does not freeze its results with", family)
		}
	}
}
