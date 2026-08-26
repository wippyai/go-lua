package model_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestTypeCapabilityIsExplicitAndDigestBound(t *testing.T) {
	ownerToken, ok := identity.DeriveContentID("model-type-capability-law/v1", []byte("owner"))
	if !ok {
		t.Fatal("owner token")
	}
	owner, ok := model.IssueOwnerID(ownerToken)
	if !ok {
		t.Fatal("owner")
	}
	typeToken, ok := identity.DeriveContentID("model-type-capability-law/v1", []byte("type"))
	if !ok {
		t.Fatal("type token")
	}
	typeID, ok := model.IssueTypeID(owner, typeToken)
	if !ok {
		t.Fatal("type")
	}
	decodeOnly, ok := model.NewDecodeOnlyCapability(typeID)
	if !ok || !decodeOnly.Available() || !decodeOnly.DecodeOnly() || decodeOnly.Ascending() {
		t.Fatal("decode-only capability was not sealed")
	}
	ascending, ok := model.NewAscendingCapability(typeID)
	if !ok || !ascending.Available() || !ascending.Ascending() || ascending.DecodeOnly() {
		t.Fatal("ascending capability was not sealed")
	}
	equatable, ok := model.NewEquatableCapability(typeID)
	if !ok || !equatable.Available() || !equatable.Equatable() || equatable.Ascending() {
		t.Fatal("equatable capability was not sealed")
	}
	if decodeOnly.Equatable() {
		t.Fatal("decode-only capability granted semantic equality")
	}
	if decodeOnly.Digest() == ascending.Digest() {
		t.Fatal("capability kind did not participate in digest")
	}
	if equatable.Digest() == decodeOnly.Digest() || equatable.Digest() == ascending.Digest() {
		t.Fatal("capability levels did not participate in digest")
	}
	if decodeOnly.Digest() == (identity.ContentID{}) || equatable.Digest() == (identity.ContentID{}) || ascending.Digest() == (identity.ContentID{}) {
		t.Fatal("capability digest unavailable")
	}
}

func TestTypeCapabilityRejectsUnavailableOrInvalidPolicy(t *testing.T) {
	if capability, ok := model.NewTypeCapability(model.TypeID{}, model.DecodeOnly); ok || capability.Available() {
		t.Fatal("zero TypeID accepted")
	}
	ownerToken, _ := identity.DeriveContentID("model-type-capability-law/v1", []byte("owner-invalid"))
	owner, _ := model.IssueOwnerID(ownerToken)
	typeToken, _ := identity.DeriveContentID("model-type-capability-law/v1", []byte("type-invalid"))
	typeID, _ := model.IssueTypeID(owner, typeToken)
	if capability, ok := model.NewTypeCapability(typeID, model.InvalidTypeCapability); ok || capability.Available() {
		t.Fatal("invalid capability kind accepted")
	}
}
