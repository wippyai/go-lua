package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// DeliveryRequirement records one ordered semantic input slot.  Input is the
// sealed generic signature contract; keeping that type here avoids inventing a
// second delivery/presence/key-vector vocabulary at mount.
type DeliveryRequirement struct {
	operation signature.Identity
	index     uint32
	input     signature.Input
}

func newDeliveryRequirement(operation signature.Identity, index int, input signature.Input) (DeliveryRequirement, bool) {
	if !operation.Available() || index < 0 || uint64(index) > uint64(^uint32(0)) || !input.Available() {
		return DeliveryRequirement{}, false
	}
	return DeliveryRequirement{operation: operation, index: uint32(index), input: input}, true
}

// Operation returns the logical semantic operation identity.
func (requirement DeliveryRequirement) Operation() signature.Identity { return requirement.operation }

// Index returns the ordered input position in the sealed signature.
func (requirement DeliveryRequirement) Index() uint32 { return requirement.index }

// Input returns the immutable generic input contract.
func (requirement DeliveryRequirement) Input() signature.Input { return requirement.input }

// Access returns the one physical logical access shared by this semantic
// input. Delivery shape and denominator are intentionally not part of Access
// identity, so scalar and span requirements over the same key/vector resolve
// to one mounted handle.
func (requirement DeliveryRequirement) Access() (Access, bool) {
	if !requirement.Available() {
		return Access{}, false
	}
	return deliveryAccess(requirement.input)
}

func deliveryAccess(input signature.Input) (Access, bool) {
	if !input.Available() {
		return Access{}, false
	}
	return newAccess(input.Relation, input.Denominator.Key(), []model.ColumnID{input.Column})
}

// Relation, Column, Type, Denominator, Delivery, and Presence are convenient
// projections retaining the names and types already owned by signature.Input.
func (requirement DeliveryRequirement) Relation() model.RelationID { return requirement.input.Relation }
func (requirement DeliveryRequirement) Column() model.ColumnID     { return requirement.input.Column }
func (requirement DeliveryRequirement) Type() model.TypeID         { return requirement.input.Type }
func (requirement DeliveryRequirement) Denominator() model.DenominatorRef {
	return requirement.input.Denominator
}
func (requirement DeliveryRequirement) Delivery() signature.Delivery {
	return requirement.input.Delivery
}
func (requirement DeliveryRequirement) Presence() signature.PresenceContract {
	return requirement.input.Presence
}

// Available reports whether this requirement carries a complete sealed input.
func (requirement DeliveryRequirement) Available() bool {
	return requirement.operation.Available() && requirement.input.Available()
}

func (requirement DeliveryRequirement) equal(other DeliveryRequirement) bool {
	return requirement.operation == other.operation && requirement.index == other.index && equalInput(requirement.input, other.input)
}

func equalInput(left, right signature.Input) bool {
	return left.Relation == right.Relation && left.Column == right.Column && left.Type == right.Type && left.Presence == right.Presence && left.Delivery == right.Delivery && left.Denominator == right.Denominator
}

func deliveryRequirementLess(left, right DeliveryRequirement) bool {
	if compared := compareOperation(left.operation, right.operation); compared != 0 {
		return compared < 0
	}
	if left.index != right.index {
		return left.index < right.index
	}
	if compared := compareRelation(left.input.Relation, right.input.Relation); compared != 0 {
		return compared < 0
	}
	if compared := compareColumn(left.input.Column, right.input.Column); compared != 0 {
		return compared < 0
	}
	if compared := compareType(left.input.Type, right.input.Type); compared != 0 {
		return compared < 0
	}
	if left.input.Presence != right.input.Presence {
		if left.input.Presence < right.input.Presence {
			return true
		}
		return false
	}
	if compared := compareDelivery(left.input.Delivery, right.input.Delivery); compared != 0 {
		return compared < 0
	}
	return compareDenominator(left.input.Denominator, right.input.Denominator) < 0
}

func compareOperation(left, right signature.Identity) int {
	if compared := compareNominal(left.Operation.Owner().Content(), left.Operation.Content(), right.Operation.Owner().Content(), right.Operation.Content()); compared != 0 {
		return compared
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}

func compareDenominator(left, right model.DenominatorRef) int {
	if !left.Available() && !right.Available() {
		return 0
	}
	if !left.Available() {
		return -1
	}
	if !right.Available() {
		return 1
	}
	if compared := compareRelation(left.Relation(), right.Relation()); compared != 0 {
		return compared
	}
	return compareKey(left.Key(), right.Key())
}

func compareDelivery(left, right signature.Delivery) int {
	if left.Kind != right.Kind {
		if left.Kind < right.Kind {
			return -1
		}
		return 1
	}
	if left.Bound != right.Bound {
		if left.Bound < right.Bound {
			return -1
		}
		return 1
	}
	return compareKey(left.Order, right.Order)
}

func compareType(left, right model.TypeID) int {
	return compareNominal(left.Owner().Content(), left.Content(), right.Owner().Content(), right.Content())
}

func deliveryRequirementDigest(value DeliveryRequirement) []byte {
	parts := make([]byte, 0, 32+8+32*8)
	appendID := func(owner, content identity.ContentID) {
		parts = append(parts, owner[:]...)
		parts = append(parts, content[:]...)
	}
	appendID(value.operation.Operation.Owner().Content(), value.operation.Operation.Content())
	var version [8]byte
	version[0] = byte(value.operation.Version >> 56)
	version[1] = byte(value.operation.Version >> 48)
	version[2] = byte(value.operation.Version >> 40)
	version[3] = byte(value.operation.Version >> 32)
	version[4] = byte(value.operation.Version >> 24)
	version[5] = byte(value.operation.Version >> 16)
	version[6] = byte(value.operation.Version >> 8)
	version[7] = byte(value.operation.Version)
	parts = append(parts, version[:]...)
	var index [4]byte
	index[0] = byte(value.index >> 24)
	index[1] = byte(value.index >> 16)
	index[2] = byte(value.index >> 8)
	index[3] = byte(value.index)
	parts = append(parts, index[:]...)
	appendID(value.input.Relation.Owner().Content(), value.input.Relation.Content())
	appendID(value.input.Column.Relation().Owner().Content(), value.input.Column.Content())
	appendID(value.input.Type.Owner().Content(), value.input.Type.Content())
	parts = append(parts, byte(value.input.Presence), byte(value.input.Delivery.Kind))
	var bound [4]byte
	bound[0] = byte(value.input.Delivery.Bound >> 24)
	bound[1] = byte(value.input.Delivery.Bound >> 16)
	bound[2] = byte(value.input.Delivery.Bound >> 8)
	bound[3] = byte(value.input.Delivery.Bound)
	parts = append(parts, bound[:]...)
	if value.input.Delivery.Order.Available() {
		appendID(value.input.Delivery.Order.Relation().Owner().Content(), value.input.Delivery.Order.Content())
	} else {
		parts = append(parts, make([]byte, 64)...)
	}
	if value.input.Denominator.Available() {
		appendID(value.input.Denominator.Relation().Owner().Content(), value.input.Denominator.Relation().Content())
		appendID(value.input.Denominator.Key().Relation().Owner().Content(), value.input.Denominator.Key().Content())
	}
	return parts
}
