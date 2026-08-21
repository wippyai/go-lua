package publication

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/composite"
)

// TestPlacementPublicationAdversariesStopAtTheGenericOwnerFence states why
// Open does not rescan a detached Result for a second contract authority.
// Result's family table is private, and the only public-side family issuer is
// the composite QueryPublication capability. Its contract, encoder, and
// family ordinal are private too. Consequently a caller cannot manufacture a
// missing family, a duplicate same-key family, or a same-key foreign
// contract/codec through the generic Result APIs.
func TestPlacementPublicationAdversariesStopAtTheGenericOwnerFence(t *testing.T) {
	if _, ok := Open(nil); ok {
		t.Fatal("Open accepted a missing generic Result family")
	}
	var zero result.Result
	if _, ok := Open(&zero); ok {
		t.Fatal("Open accepted an unsealed Result with no placement family")
	}

	resultType := reflect.TypeOf(result.Result{})
	families, familiesOK := resultType.FieldByName("families")
	if !familiesOK || families.PkgPath == "" {
		t.Fatal("generic Result exposed its mutable family table")
	}
	familyType := families.Type.Elem()
	for _, name := range []string{"key", "contract", "queries"} {
		field, found := familyType.FieldByName(name)
		if !found || field.PkgPath == "" {
			t.Fatalf("generic Result family field %q crossed the owner fence", name)
		}
	}

	publicationType := reflect.TypeOf(composite.QueryPublication{})
	for _, name := range []string{"contract", "encode", "ordinal"} {
		field, found := publicationType.FieldByName(name)
		if !found || field.PkgPath == "" {
			t.Fatalf("QueryPublication field %q crossed the owner fence", name)
		}
	}
}
