package signature

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// Identity and Fence carry canonical model identities and exact solve fences;
// semantic does not issue logical relation/column identities.
type Identity struct {
	Operation model.OperationID
	Version   uint64
}

func (value Identity) Available() bool {
	return value.Operation.Available() && value.Version != 0
}

type Fence struct {
	Owner  model.OwnerID
	Schema model.SchemaID
}

func (fence Fence) Available() bool {
	return fence.Owner.Available() && fence.Schema.Available()
}

// DeliveryKind is the closed shape vocabulary for one ordered input slot.
// ScalarDelivery carries one cell. BoundedSpanDelivery carries at most Bound
// cells, while CompleteSpanDelivery carries the complete mounted denominator
// range. Span order is logical and keyed; it is never a physical ordinal.
type DeliveryKind uint8

const (
	InvalidDelivery DeliveryKind = iota
	ScalarDelivery
	BoundedSpanDelivery
	CompleteSpanDelivery
)

// Delivery is the compact frame-shape contract for one input. Order is
// required for both span forms and names the logical key whose mounted range
// order the frame must preserve.
type Delivery struct {
	Kind  DeliveryKind
	Bound uint32
	Order model.KeyID
}

func NewScalarDelivery() (Delivery, bool) {
	return Delivery{Kind: ScalarDelivery}, true
}

func NewBoundedSpanDelivery(bound uint32, order model.KeyID) (Delivery, bool) {
	if bound == 0 || !order.Available() {
		return Delivery{}, false
	}
	return Delivery{Kind: BoundedSpanDelivery, Bound: bound, Order: order}, true
}

func NewCompleteSpanDelivery(order model.KeyID) (Delivery, bool) {
	if !order.Available() {
		return Delivery{}, false
	}
	return Delivery{Kind: CompleteSpanDelivery, Order: order}, true
}

func (delivery Delivery) Available() bool {
	switch delivery.Kind {
	case ScalarDelivery:
		return delivery.Bound == 0 && !delivery.Order.Available()
	case BoundedSpanDelivery:
		return delivery.Bound != 0 && delivery.Order.Available()
	case CompleteSpanDelivery:
		return delivery.Bound == 0 && delivery.Order.Available()
	default:
		return false
	}
}

func (delivery Delivery) IsScalar() bool {
	return delivery.Available() && delivery.Kind == ScalarDelivery
}
func (delivery Delivery) IsSpan() bool {
	return delivery.Available() && (delivery.Kind == BoundedSpanDelivery || delivery.Kind == CompleteSpanDelivery)
}
func (delivery Delivery) IsComplete() bool {
	return delivery.Available() && delivery.Kind == CompleteSpanDelivery
}
func (delivery Delivery) Limit() (uint32, bool) {
	if !delivery.Available() || delivery.Kind != BoundedSpanDelivery {
		return 0, false
	}
	return delivery.Bound, true
}
func (delivery Delivery) OrderKey() model.KeyID { return delivery.Order }

// PresenceContract is a closed input/output enum.
type PresenceContract uint8

const (
	RequirePresent PresenceContract = iota + 1
	AllowMissing
	RequireOpaque
	ProducePresent
	ProduceOptional
	ProduceAbsent
	ProduceOpaque
)

func (contract PresenceContract) Input() bool {
	return contract == RequirePresent || contract == AllowMissing || contract == RequireOpaque
}

func (contract PresenceContract) Output() bool {
	return contract >= ProducePresent && contract <= ProduceOpaque
}

func (contract PresenceContract) Allows(presence model.Presence) bool {
	if !presence.Available() || presence.Is(model.Refused) {
		return false
	}
	switch contract {
	case RequirePresent, ProducePresent:
		return presence.Is(model.Present)
	case AllowMissing:
		return presence.Is(model.Present) || presence.Is(model.ProvenAbsent) || presence.Is(model.UnprovenMissing) || presence.Is(model.AuthenticatedOpaque)
	case RequireOpaque, ProduceOpaque:
		return presence.Is(model.AuthenticatedOpaque)
	case ProduceOptional:
		return presence.Is(model.Present) || presence.Is(model.ProvenAbsent)
	case ProduceAbsent:
		return presence.Is(model.ProvenAbsent)
	default:
		return false
	}
}

type Input struct {
	Relation    model.RelationID
	Column      model.ColumnID
	Type        model.TypeID
	Presence    PresenceContract
	Delivery    Delivery
	Denominator model.DenominatorRef
}

func (input Input) Available() bool {
	return input.Relation.Available() && input.Column.Available() && input.Type.Available() && input.Presence.Input() && input.Delivery.Available() && input.Denominator.Available()
}

type Output struct {
	Relation model.RelationID
	Column   model.ColumnID
	Type     model.TypeID
	Presence PresenceContract
}

func (output Output) Available() bool {
	return output.Relation.Available() && output.Column.Available() && output.Type.Available() && output.Presence.Output()
}

type Spec struct {
	Identity    Identity
	Fence       Fence
	Inputs      []Input
	Outputs     []Output
	Authority   OutputAuthority
	Cardinality model.Cardinality
	Outcomes    outcome.Set
}
