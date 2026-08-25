package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestACoordinateIsNamedByTheIdentityItsSealAssigned states that naming a
// coordinate is a read of an identity the schema already issued and never a
// second numbering. The law holds over the whole sealed range in both
// directions, so a consumer that must address a coordinate row by content has
// exactly one name available to it and it is the owner's.
func TestACoordinateIsNamedByTheIdentityItsSealAssigned(t *testing.T) {
	schema := relationfixture.New(t).Values
	count := schema.CoordinateCount()
	if count == 0 {
		t.Fatal("the fixture sealed no coordinate")
	}
	seen := make(map[identity.ContentID]bool, count)
	for index := 0; index < count; index++ {
		coordinate, ok := schema.CoordinateAt(index)
		if !ok {
			t.Fatalf("coordinate %d is not issued", index)
		}
		named, ok := schema.CoordinateContentID(coordinate)
		if !ok || !named.Available() {
			t.Fatalf("coordinate %d carries no owner-issued name", index)
		}
		if seen[named] {
			t.Fatalf("coordinate %d shares its name with another coordinate", index)
		}
		seen[named] = true
		returned, ok := schema.CoordinateForID(named)
		if !ok {
			t.Fatalf("the name of coordinate %d does not resolve back to a coordinate", index)
		}
		if returned != coordinate {
			t.Fatalf("the name of coordinate %d resolves to a different coordinate", index)
		}
	}
	if len(seen) != count {
		t.Fatalf("the sealed range holds %d coordinates and %d names", count, len(seen))
	}
}

// TestAForeignCoordinateIsNotNamed states the fence. A coordinate is issued by
// one exact schema, so another schema of the same content answers no name for
// it and the two numberings never merge.
func TestAForeignCoordinateIsNotNamed(t *testing.T) {
	first := relationfixture.New(t).Values
	second := relationfixture.New(t).Values
	coordinate, ok := first.CoordinateAt(0)
	if !ok {
		t.Fatal("the fixture sealed no coordinate")
	}
	if _, named := second.CoordinateContentID(coordinate); named {
		t.Fatal("a schema named a coordinate another schema issued")
	}
	var unsealed *valuedomain.Schema
	if _, named := unsealed.CoordinateContentID(coordinate); named {
		t.Fatal("an unsealed schema named a coordinate")
	}
}
