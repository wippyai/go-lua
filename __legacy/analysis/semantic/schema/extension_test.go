package schema

import (
	"bytes"
	"errors"
	"slices"
	"testing"
)

func TestExtensionInventoryIsPermutationInvariantAndDetached(t *testing.T) {
	first := ExtensionDescriptor{
		FamilyID: "engine.state.userlattice", InstanceID: "taint",
		DescriptorVersion: "v1", CodecID: "userlattice.v1", ProjectionID: "userlattice.project.v1",
		Config: []byte{1, 2}, Leaves: []LeafDescriptor{{ID: "value", SchemaID: "userlattice.value.v1"}, {ID: "hook", SchemaID: "userlattice.hook.v1"}},
	}
	second := ExtensionDescriptor{
		FamilyID: "engine.state.userlattice", InstanceID: "ownership",
		DescriptorVersion: "v1", CodecID: "userlattice.v1", ProjectionID: "userlattice.project.v1",
		Config: []byte{3}, Leaves: []LeafDescriptor{{ID: "value", SchemaID: "userlattice.value.v1"}},
	}
	left, err := NewExtensionInventory([]ExtensionDescriptor{first, second})
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(first.Leaves)
	right, err := NewExtensionInventory([]ExtensionDescriptor{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left.CanonicalBytes(), right.CanonicalBytes()) || left.Digest() != right.Digest() {
		t.Fatal("registration or leaf order changed extension identity")
	}
	copy := left.Descriptors()
	copy[0].Config[0] ^= 0xff
	copy[0].Leaves[0].ID = "mutated"
	if bytes.Equal(copy[0].Config, left.Descriptors()[0].Config) || left.Descriptors()[0].Leaves[0].ID == "mutated" {
		t.Fatal("descriptor accessor aliases sealed inventory")
	}
}

func TestExtensionInventoryRejectsDuplicateAndMissingLeaves(t *testing.T) {
	base := ExtensionDescriptor{FamilyID: "family", InstanceID: "instance", DescriptorVersion: "v1", CodecID: "codec.v1", ProjectionID: "projection.v1", Config: []byte{}, Leaves: []LeafDescriptor{{ID: "value", SchemaID: "value.v1"}}}
	if _, err := NewExtensionInventory([]ExtensionDescriptor{base, base}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate error = %v", err)
	}
	base.Leaves = nil
	if _, err := NewExtensionInventory([]ExtensionDescriptor{base}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty leaves error = %v", err)
	}
}

func TestDescriptorUniverseSeparatesBaseAndExtensionMeaning(t *testing.T) {
	empty, err := NewExtensionInventory(nil)
	if err != nil {
		t.Fatal(err)
	}
	withExtension, err := NewExtensionInventory([]ExtensionDescriptor{{FamilyID: "family", InstanceID: "instance", DescriptorVersion: "v1", CodecID: "codec.v1", ProjectionID: "projection.v1", Config: []byte{}, Leaves: []LeafDescriptor{{ID: "value", SchemaID: "value.v1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	baseID, err := NewDescriptorUniverseID([]byte("base"), empty)
	if err != nil {
		t.Fatal(err)
	}
	extendedID, err := NewDescriptorUniverseID([]byte("base"), withExtension)
	if err != nil {
		t.Fatal(err)
	}
	otherBaseID, err := NewDescriptorUniverseID([]byte("other"), empty)
	if err != nil {
		t.Fatal(err)
	}
	if baseID == extendedID || baseID == otherBaseID {
		t.Fatal("descriptor universe does not domain-separate semantic inputs")
	}
}
