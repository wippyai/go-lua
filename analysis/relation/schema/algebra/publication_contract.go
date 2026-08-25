package algebra

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// ApplyContract declares one authenticated semantic operation boundary. The
// operation identity is owned by semantic/signature. Capability, argument
// shape, and result shape are resolved by the semantic signature layer.
type ApplyContract struct {
	operation signature.Identity
}

// NewApplyContract constructs an operation boundary without semantic
// validation.
func NewApplyContract(operation signature.Identity) ApplyContract {
	return ApplyContract{operation: operation}
}

// Operation returns the stable operation identity.
func (contract ApplyContract) Operation() signature.Identity { return contract.operation }

func (contract ApplyContract) digestBytes() []byte {
	return appendOperation(nil, contract.operation)
}

// PublishContract names one logical destination relation and key. Invocation
// cardinality and atomic commit policy belong to the semantic and transaction
// layers, not to this expression contract.
type PublishContract struct {
	destination model.RelationID
	key         model.KeyID
}

// NewPublishContract constructs a publication declaration without checking
// destination ownership or key authority.
func NewPublishContract(destination model.RelationID, key model.KeyID) PublishContract {
	return PublishContract{destination: destination, key: key}
}

// Destination returns the logical destination relation.
func (contract PublishContract) Destination() model.RelationID { return contract.destination }

// Key returns the destination key used by the declaration.
func (contract PublishContract) Key() model.KeyID { return contract.key }

func (contract PublishContract) digestBytes() []byte {
	parts := appendRelation(nil, contract.destination)
	return appendKey(parts, contract.key)
}
