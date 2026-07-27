// Package schema owns backend-neutral semantic descriptor identities.
package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const ExtensionInventorySchema = "go-lua.semantic.extension-inventory/v1"

var ErrInvalid = errors.New("semantic schema: invalid")

// LeafDescriptor names one independently covered semantic leaf of a runtime
// extension instance. A carrier or component descriptor never implies leaf
// coverage.
type LeafDescriptor struct {
	ID       string
	SchemaID string
}

// ExtensionDescriptor is the immutable, data-only description of one runtime
// lattice instance. Config is canonical codec output, never a Go value or
// registration-order ordinal.
type ExtensionDescriptor struct {
	FamilyID          string
	InstanceID        string
	DescriptorVersion string
	CodecID           string
	ProjectionID      string
	Config            []byte
	Leaves            []LeafDescriptor
}

// ExtensionInventory is sorted by (family, instance) and detached from all
// caller-owned storage.
type ExtensionInventory struct {
	descriptors []ExtensionDescriptor
	canonical   []byte
	digest      [sha256.Size]byte
}

// NewExtensionInventory validates and canonically seals runtime extensions.
func NewExtensionInventory(input []ExtensionDescriptor) (ExtensionInventory, error) {
	descriptors := make([]ExtensionDescriptor, len(input))
	for i, descriptor := range input {
		descriptors[i] = cloneDescriptor(descriptor)
		if err := validateDescriptor(&descriptors[i]); err != nil {
			return ExtensionInventory{}, err
		}
	}
	sort.Slice(descriptors, func(i, j int) bool {
		if descriptors[i].FamilyID != descriptors[j].FamilyID {
			return descriptors[i].FamilyID < descriptors[j].FamilyID
		}
		return descriptors[i].InstanceID < descriptors[j].InstanceID
	})
	for i := 1; i < len(descriptors); i++ {
		previous, current := descriptors[i-1], descriptors[i]
		if previous.FamilyID == current.FamilyID && previous.InstanceID == current.InstanceID {
			return ExtensionInventory{}, invalid("extension", fmt.Errorf("duplicate %s/%s", current.FamilyID, current.InstanceID))
		}
	}
	canonical, err := encodeExtensions(descriptors)
	if err != nil {
		return ExtensionInventory{}, err
	}
	return ExtensionInventory{descriptors: descriptors, canonical: canonical, digest: sha256.Sum256(canonical)}, nil
}

func (i ExtensionInventory) Descriptors() []ExtensionDescriptor {
	out := make([]ExtensionDescriptor, len(i.descriptors))
	for index, descriptor := range i.descriptors {
		out[index] = cloneDescriptor(descriptor)
	}
	return out
}

func (i ExtensionInventory) CanonicalBytes() []byte    { return append([]byte(nil), i.canonical...) }
func (i ExtensionInventory) Digest() [sha256.Size]byte { return i.digest }

// DescriptorUniverseID commits schema-only inventory and extension meaning.
// Backend implementations are deliberately excluded; engine/config adds them
// when it seals RuntimeUniverseID.
type DescriptorUniverseID [sha256.Size]byte

func NewDescriptorUniverseID(baseInventoryCanonical []byte, extensions ExtensionInventory) (DescriptorUniverseID, error) {
	if len(baseInventoryCanonical) == 0 {
		return DescriptorUniverseID{}, invalid("descriptor universe", errors.New("empty base inventory"))
	}
	var out bytes.Buffer
	if err := writeFrame(&out, []byte("go-lua.semantic.descriptor-universe/v1")); err != nil {
		return DescriptorUniverseID{}, err
	}
	if err := writeFrame(&out, baseInventoryCanonical); err != nil {
		return DescriptorUniverseID{}, err
	}
	if err := writeFrame(&out, extensions.canonical); err != nil {
		return DescriptorUniverseID{}, err
	}
	return sha256.Sum256(out.Bytes()), nil
}

func validateDescriptor(descriptor *ExtensionDescriptor) error {
	for name, value := range map[string]string{
		"family": descriptor.FamilyID, "instance": descriptor.InstanceID,
		"version": descriptor.DescriptorVersion, "codec": descriptor.CodecID,
		"projection": descriptor.ProjectionID,
	} {
		if !validID(value) {
			return invalid("extension "+name, fmt.Errorf("invalid id %q", value))
		}
	}
	if descriptor.Config == nil {
		return invalid("extension config", errors.New("nil canonical config"))
	}
	if len(descriptor.Leaves) == 0 {
		return invalid("extension leaves", errors.New("empty leaf expansion"))
	}
	sort.Slice(descriptor.Leaves, func(i, j int) bool { return descriptor.Leaves[i].ID < descriptor.Leaves[j].ID })
	for index, leaf := range descriptor.Leaves {
		if !validID(leaf.ID) || !validID(leaf.SchemaID) {
			return invalid("extension leaf", fmt.Errorf("invalid leaf %q schema %q", leaf.ID, leaf.SchemaID))
		}
		if index > 0 && descriptor.Leaves[index-1].ID == leaf.ID {
			return invalid("extension leaf", fmt.Errorf("duplicate %q", leaf.ID))
		}
	}
	return nil
}

func cloneDescriptor(descriptor ExtensionDescriptor) ExtensionDescriptor {
	if descriptor.Config != nil {
		config := make([]byte, len(descriptor.Config))
		copy(config, descriptor.Config)
		descriptor.Config = config
	}
	descriptor.Leaves = append([]LeafDescriptor(nil), descriptor.Leaves...)
	return descriptor
}

func encodeExtensions(descriptors []ExtensionDescriptor) ([]byte, error) {
	var out bytes.Buffer
	if err := writeFrame(&out, []byte(ExtensionInventorySchema)); err != nil {
		return nil, err
	}
	if err := writeCount(&out, len(descriptors)); err != nil {
		return nil, err
	}
	for _, descriptor := range descriptors {
		for _, value := range [][]byte{
			[]byte(descriptor.FamilyID), []byte(descriptor.InstanceID),
			[]byte(descriptor.DescriptorVersion), []byte(descriptor.CodecID),
			[]byte(descriptor.ProjectionID), descriptor.Config,
		} {
			if err := writeFrame(&out, value); err != nil {
				return nil, err
			}
		}
		if err := writeCount(&out, len(descriptor.Leaves)); err != nil {
			return nil, err
		}
		for _, leaf := range descriptor.Leaves {
			if err := writeFrame(&out, []byte(leaf.ID)); err != nil {
				return nil, err
			}
			if err := writeFrame(&out, []byte(leaf.SchemaID)); err != nil {
				return nil, err
			}
		}
	}
	return out.Bytes(), nil
}

func writeCount(out *bytes.Buffer, count int) error {
	if count < 0 || uint64(count) > uint64(^uint32(0)) {
		return invalid("canonical encoding", errors.New("count exceeds uint32"))
	}
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], uint32(count))
	out.Write(encoded[:])
	return nil
}

func writeFrame(out *bytes.Buffer, value []byte) error {
	if err := writeCount(out, len(value)); err != nil {
		return err
	}
	out.Write(value)
	return nil
}

func validID(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '.' || character == '/' {
			continue
		}
		return false
	}
	return true
}

func invalid(part string, err error) error { return fmt.Errorf("%w: %s: %v", ErrInvalid, part, err) }
