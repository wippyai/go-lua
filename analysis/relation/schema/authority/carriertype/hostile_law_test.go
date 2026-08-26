package carriertype_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority/carriertype"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func carrierTypeLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("wippy.analysis/relation/schema/authority/carriertype/test", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func carrierTypeLawOwner(t *testing.T, key schema.Key, token string) authority.Owner {
	t.Helper()
	owner, ok := authority.NewOwner(
		schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key},
		carrierTypeLawID(t, token),
	)
	if !ok {
		t.Fatalf("owner %q unavailable", key)
	}
	return owner
}

func TestTypeCarriesExactOwnerAndAuthorityContent(t *testing.T) {
	owner := carrierTypeLawOwner(t, "carrier-type-owner", "owner")
	issued, ok := carrier.Issue(owner.Entry, carrier.Authority{Carrier: "carrier/type/value", Capability: carrier.Equatable})
	if !ok {
		t.Fatal("carrier authority unavailable")
	}

	typeID, ok := carriertype.Type(owner, issued)
	if !ok || !typeID.Available() {
		t.Fatalf("carrier type unavailable: %+v/%t", typeID, ok)
	}
	wantContent := identity.ContentID(issued.ID())
	if typeID.Owner() != owner.ID() || typeID.Content() != wantContent {
		t.Fatalf("carrier type changed authority: owner=%v/%v content=%v/%v", typeID.Owner(), owner.ID(), typeID.Content(), wantContent)
	}

	// The projection is deterministic and does not create a second content
	// identity or depend on declaration order.
	again, againOK := carriertype.Type(owner, issued)
	if !againOK || again != typeID {
		t.Fatal("replaying an exact carrier changed its TypeID")
	}
}

func TestTypeRejectsMissingUnissuedForeignAndForgedAuthorities(t *testing.T) {
	owner := carrierTypeLawOwner(t, "carrier-type-owner", "owner")
	foreign := carrierTypeLawOwner(t, "carrier-type-foreign", "foreign-owner")
	raw := carrier.Authority{Carrier: "carrier/type/value", Capability: carrier.DecodeOnly}
	issued, ok := carrier.Issue(owner.Entry, raw)
	if !ok {
		t.Fatal("carrier authority unavailable")
	}
	foreignIssued, ok := carrier.Issue(foreign.Entry, raw)
	if !ok {
		t.Fatal("foreign carrier authority unavailable")
	}

	cases := []struct {
		name  string
		owner authority.Owner
		value carrier.Authority
	}{
		{name: "zero owner", owner: authority.Owner{}, value: issued},
		{name: "zero authority", owner: owner, value: carrier.Authority{}},
		{name: "unissued", owner: owner, value: raw},
		{name: "foreign owner", owner: owner, value: foreignIssued},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if typeID, ok := carriertype.Type(test.owner, test.value); ok || typeID.Available() {
				t.Fatalf("hostile authority admitted: %+v/%t", typeID, ok)
			}
		})
	}

	forgedCarrier := issued
	forgedCarrier.Carrier = "carrier/type/changed"
	if typeID, ok := carriertype.Type(owner, forgedCarrier); ok || typeID.Available() {
		t.Fatal("authority with a changed carrier key was admitted")
	}

	forgedCapability := issued
	forgedCapability.Capability = carrier.Ascending
	if typeID, ok := carriertype.Type(owner, forgedCapability); ok || typeID.Available() {
		t.Fatal("authority with a changed capability was admitted")
	}

	wrongEntry := owner
	wrongEntry.Entry = schema.EntryReference{Surface: schema.SurfaceKindQuery, Key: "carrier-type-owner"}
	if typeID, ok := carriertype.Type(wrongEntry, issued); ok || typeID.Available() {
		t.Fatal("authority crossed a changed owner entry")
	}
}

func TestTypeRejectsUnavailableOwnerIdentity(t *testing.T) {
	entry := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "carrier-type-owner"}
	owner := authority.Owner{Entry: entry, Token: identity.ContentID{1}}
	issued, ok := carrier.Issue(entry, carrier.Authority{Carrier: "carrier/type/value", Capability: carrier.DecodeOnly})
	if !ok {
		t.Fatal("carrier authority unavailable")
	}
	// Owner.ID is valid for any non-zero supplied token by design, but an
	// unavailable Entry must still stop the carrier replay.
	owner.Entry = schema.EntryReference{}
	if typeID, ok := carriertype.Type(owner, issued); ok || typeID.Available() {
		t.Fatal("authority admitted with an unavailable owner entry")
	}

	if typeID, ok := carriertype.Type(authority.Owner{}, carrier.Authority{}); ok || typeID != (model.TypeID{}) {
		t.Fatal("zero inputs produced a type")
	}
}
