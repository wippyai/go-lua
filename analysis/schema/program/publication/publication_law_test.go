package publication

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
)

func publicationLawID(t *testing.T) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("program-publication-law/catalog", nil)
	if !ok {
		t.Fatal("derive catalog")
	}
	return id
}

func TestSealWritesEveryManifestSlotInOrder(t *testing.T) {
	catalog, ok := programcatalog.CatalogID(publicationLawID(t))
	if !ok {
		t.Fatal("catalog identity")
	}
	frozen, sealed := (Publication{}).Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("empty aggregate publication did not seal")
	}
	for index, definition := range programcatalog.Manifest() {
		denominator, derived := definition.Denominator(catalog)
		if !derived {
			t.Fatalf("manifest definition %d has no denominator", index)
		}
		if width, published := frozen.Denominators().Size(denominator); !published || width != 0 {
			t.Fatalf("manifest slot %d width/published = %d/%v", index, width, published)
		}
	}
}

// The publication is total over the catalog: sealing every declared family,
// including the empty ones, publishes one column per declaration. A family
// whose slot collided with another's would fail here rather than at the first
// program that emitted rows into both.
func TestProgramPublicationSealsEveryDeclaredFamily(t *testing.T) {
	catalog, derived := programcatalog.CatalogID(publicationLawID(t))
	if !derived {
		t.Fatal("catalog identity")
	}
	frozen, sealed := (Publication{}).Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("the empty publication did not seal every declared family")
	}
	if _, published := programschema.PointFamily().Count(&frozen, catalog); !published {
		t.Fatal("point family is not published")
	}
	if _, published := programschema.WTOEventFamily().Count(&frozen, catalog); !published {
		t.Fatal("event family is not published")
	}
	if _, published := programschema.OutcomePointFamily().Count(&frozen, catalog); !published {
		t.Fatal("outcome point family is not published")
	}
	if _, published := programschema.LocalTransferWriteFamily().Count(&frozen, catalog); !published {
		t.Fatal("local-transfer write family is not published")
	}
}

// Program authenticates the cold catalog sealed into Frozen against the
// runtime schema it names. A published snapshot under a foreign catalog is
// not a Program and cannot be handed to semantic child readers.
func TestProgramColdStateRejectsForeignCatalog(t *testing.T) {
	runtimeSchema := publicationLawID(t)
	catalogID, derived := programcatalog.CatalogID(runtimeSchema)
	if !derived {
		t.Fatal("catalog identity")
	}
	frozen, sealed := (Publication{}).Seal(catalogID, identity.StoreID(2))
	if !sealed {
		t.Fatal("publication")
	}
	program := programschema.Program{
		Frozen:     frozen,
		ArtifactID: publicationLawContentID(t, "state-artifact"),
		ProgramID:  publicationLawContentID(t, "state-program"),
		SchemaID:   runtimeSchema,
	}
	state, opened := program.ColdState()
	if !program.Available() || !opened || !state.Available() || state.CatalogID() != catalogID {
		t.Fatal("matching Program did not expose its authenticated cold state")
	}

	program.SchemaID = publicationLawContentID(t, "foreign-runtime-schema")
	if program.Available() {
		t.Fatal("Program accepted a Frozen sealed under a foreign cold catalog")
	}
	if _, opened := program.ColdState(); opened {
		t.Fatal("foreign cold catalog escaped through ColdState")
	}
}

func publicationLawContentID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("program-publication-law/"+name, nil)
	if !derived {
		t.Fatalf("derive %s", name)
	}
	return id
}
