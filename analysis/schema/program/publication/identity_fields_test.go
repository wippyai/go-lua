package publication

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

type identityManifestOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
}

type identityManifestOperations []identityManifestOperation

func (operations *identityManifestOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, identityManifestOperation{kind: 'i', id: id})
	return true
}

func (operations *identityManifestOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, identityManifestOperation{kind: 'u', value: value})
	return true
}

func (operations *identityManifestOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, identityManifestOperation{kind: 'b', value: encoded})
	return true
}

func (operations *identityManifestOperations) WriteString(value string) bool {
	*operations = append(*operations, identityManifestOperation{kind: 's', value: uint64(len(value))})
	return true
}

// TestArtifactIdentityManifestEmptyKAT pins the complete owner-segment order
// independently of Artifact. The artifact identity laws exercise populated
// payloads through this same manifest; this KAT makes any reorder of empty
// segments visible before a row-specific law is needed.
func TestArtifactIdentityManifestEmptyKAT(t *testing.T) {
	catalog, catalogOK := programcatalog.CatalogID(identity.ContentID{200})
	if !catalogOK {
		t.Fatal("catalog")
	}
	frozen, sealed := (Publication{}).Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("seal publication")
	}
	state, stateOK := programstate.New(frozen, catalog)
	if !stateOK {
		t.Fatal("cold state")
	}
	var got identityManifestOperations
	if !WriteArtifactIdentityFields(state, &got) {
		t.Fatal("write artifact identity manifest")
	}
	u := func(value uint64) identityManifestOperation {
		return identityManifestOperation{kind: 'u', value: value}
	}
	want := identityManifestOperations{
		// Point, Values.
		u(programschema.PointGeometryLawVersion), u(2), u(0), u(0),
		// Lifecycle: storage lifetime, the liveness span plane, the yield
		// boundary sequence it is a range over, subject events, alias scope
		// (two counts), alias candidates.
		u(2), u(0), u(1), u(0), u(1), u(0), u(1), u(0), u(1), u(0), u(0), u(1), u(0),
		// Call, CallResultSlot, Body, Module, Occurrence, Summary.
		u(2), u(0), u(4), u(0), u(2), u(0), u(0), u(4), u(0), u(0), u(0), u(0),
		u(1), u(0), u(0), u(0), u(0), u(0), u(0),
		u(0), u(0), u(0), u(0),
		// Heap allocation/index, diagnostic, static type value/node/expression/input.
		u(0), u(0), u(7), u(0), u(0), u(0), u(0), u(0),
		// Environment/local transfer, rule occurrence, region/WTO.
		u(0), u(0), u(0), u(0), u(0),
	}
	if !reflect.DeepEqual(got, want) {
		gotValues := make([]uint64, len(got))
		for index, operation := range got {
			gotValues[index] = operation.value
		}
		wantValues := make([]uint64, len(want))
		for index, operation := range want {
			wantValues[index] = operation.value
		}
		t.Fatalf("identity manifest values = %v, want %v", gotValues, wantValues)
	}
}
