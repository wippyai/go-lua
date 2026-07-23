package userlattice

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	semanticschema "github.com/wippyai/go-lua/analysis/semantic/schema"
)

// Inventory projects the verified runtime extensions into the canonical,
// registration-order-independent semantic extension inventory.
func Inventory(reg *axis.Registry) (semanticschema.ExtensionInventory, error) {
	runtime := RuntimeFor(reg)
	descriptors := make([]semanticschema.ExtensionDescriptor, 0, runtime.Len())
	for index := 0; index < runtime.Len(); index++ {
		current := runtime.AxisAt(index)
		descriptors = append(descriptors, semanticschema.ExtensionDescriptor{
			FamilyID: "engine.state.userlattice", InstanceID: string(current.ID()),
			DescriptorVersion: "v1", CodecID: "userlattice.v1", ProjectionID: "userlattice.project.v1",
			Config: encodeAxisConfig(current),
			Leaves: []semanticschema.LeafDescriptor{
				{ID: "value", SchemaID: "userlattice.value.v1"},
				{ID: "on-assign", SchemaID: "userlattice.assign.v1"},
				{ID: "on-call-boundary", SchemaID: "userlattice.call-boundary.v1"},
				{ID: "on-claim", SchemaID: "userlattice.claim.v1"},
				{ID: "on-join", SchemaID: "userlattice.join.v1"},
			},
		})
	}
	return semanticschema.NewExtensionInventory(descriptors)
}

func encodeAxisConfig(current Axis) []byte {
	names := append([]ElementID(nil), current.spec.elements...)
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	var out bytes.Buffer
	writeInventoryString(&out, "go-lua.userlattice.config/v1")
	writeInventoryString(&out, string(current.ID()))
	writeInventoryString(&out, string(current.ElementName(current.Bottom())))
	writeInventoryString(&out, string(current.ElementName(current.Top())))
	writeInventoryCount(&out, len(names))
	for _, name := range names {
		writeInventoryString(&out, string(name))
	}
	for _, leftName := range names {
		left, _ := current.Element(leftName)
		for _, rightName := range names {
			right, _ := current.Element(rightName)
			if current.LessOrEq(left, right) {
				out.WriteByte(1)
			} else {
				out.WriteByte(0)
			}
			writeInventoryString(&out, string(current.ElementName(current.Join(left, right))))
		}
		writeInventoryString(&out, string(current.ElementName(current.Assign(left))))
		writeInventoryString(&out, string(current.ElementName(current.CallBoundary(left))))
	}
	claims := make([]string, 0, len(current.spec.claims))
	for claim := range current.spec.claims {
		claims = append(claims, claim)
	}
	sort.Strings(claims)
	writeInventoryCount(&out, len(claims))
	for _, claim := range claims {
		writeInventoryString(&out, claim)
		writeInventoryString(&out, string(current.ElementName(current.spec.claims[claim])))
	}
	return out.Bytes()
}

func writeInventoryCount(out *bytes.Buffer, count int) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], uint32(count))
	out.Write(encoded[:])
}

func writeInventoryString(out *bytes.Buffer, value string) {
	writeInventoryCount(out, len(value))
	out.WriteString(value)
}
