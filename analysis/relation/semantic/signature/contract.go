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
	Relation model.RelationID
	Column   model.ColumnID
	Type     model.TypeID
	Presence PresenceContract
	Delivery Delivery
	// Denominator is the carrier population for this delivery.  For a span it
	// owns the exact range and its logical order; it is deliberately not
	// inferred from the source cell's row.
	Denominator model.DenominatorRef
	// SourceAuthority is zero for the canonical homogeneous variant, where
	// the source cell and delivery carrier share Denominator.  A non-zero
	// value is the joined variant and names the distinct source population
	// that authenticates Relation/Column cells.  Its representation is sealed
	// so callers cannot manufacture a third/fallback authority shape.
	SourceAuthority SourceAuthority
}

func (input Input) Available() bool {
	if !input.Relation.Available() || !input.Column.Available() || !input.Type.Available() || !input.Presence.Input() || !input.Delivery.Available() || !input.Denominator.Available() {
		return false
	}
	switch input.AuthorityKind() {
	case HomogeneousAuthority:
		// The absent source authority is not a compatibility fallback: it is
		// the compact representation of the homogeneous sum member.
		return input.Relation == input.Denominator.Relation()
	case JoinedAuthority:
		source, ok := input.SourceAuthority.Denominator()
		// A joined authority is meaningful only across relations.  Allowing a
		// redundant second denominator over the same relation would make the
		// two variants observationally ambiguous.
		return ok && source.Relation() == input.Relation && source.Relation() != input.Denominator.Relation()
	default:
		return false
	}
}

// InputAuthorityKind is the closed source/carrier authority vocabulary for
// an input.  HomogeneousAuthority stores no redundant source denominator;
// JoinedAuthority carries one sealed, distinct source authority.
type InputAuthorityKind uint8

const (
	InvalidAuthority InputAuthorityKind = iota
	HomogeneousAuthority
	JoinedAuthority
)

// SourceAuthority is the present arm of an input's closed authority sum.
// Its denominator is intentionally private: a joined input can only be
// authored through NewJoinedInput, while the zero value remains the explicit
// compact representation of the homogeneous arm.
type SourceAuthority struct {
	denominator model.DenominatorRef
}

// NewSourceAuthority constructs the present joined-source authority.  It is
// exposed separately so schema assemblers can inspect the exact capability,
// but Input retains ownership of deciding whether it is legal for a carrier.
func NewSourceAuthority(denominator model.DenominatorRef) (SourceAuthority, bool) {
	if !denominator.Available() {
		return SourceAuthority{}, false
	}
	return SourceAuthority{denominator: denominator}, true
}

// Declared reports whether this input uses the joined arm of the authority
// sum.  The zero value is deliberately Homogeneous rather than an unknown
// compatibility state.
func (authority SourceAuthority) Declared() bool { return authority.denominator.Available() }

// Available is the ordinary value-availability spelling for callers that
// inspect the present joined arm.  Use Declared when distinguishing it from
// the intentional zero-tag homogeneous arm in diagnostics.
func (authority SourceAuthority) Available() bool { return authority.Declared() }

// Denominator returns the source cell population for a declared joined arm.
// Homogeneous inputs have no redundant stored source denominator.
func (authority SourceAuthority) Denominator() (model.DenominatorRef, bool) {
	if !authority.Declared() {
		return model.DenominatorRef{}, false
	}
	return authority.denominator, true
}

func (authority SourceAuthority) Same(other SourceAuthority) bool {
	return authority.denominator == other.denominator
}

// AuthorityKind identifies the closed source/carrier variant without
// guessing from relation or key identity.  Availability checks the exact
// relation invariants separately so malformed authored specs still have one
// deterministic rejection boundary.
func (input Input) AuthorityKind() InputAuthorityKind {
	if input.SourceAuthority.Declared() {
		return JoinedAuthority
	}
	return HomogeneousAuthority
}

func (input Input) IsHomogeneous() bool {
	return input.Available() && input.AuthorityKind() == HomogeneousAuthority
}
func (input Input) IsJoined() bool {
	return input.Available() && input.AuthorityKind() == JoinedAuthority
}

// SourceDenominator returns the authority for input cell addresses.  In the
// homogeneous variant it is exactly the carrier denominator; a joined input
// instead returns its explicitly sealed source authority.
func (input Input) SourceDenominator() (model.DenominatorRef, bool) {
	if input.AuthorityKind() == JoinedAuthority {
		return input.SourceAuthority.Denominator()
	}
	if !input.Denominator.Available() {
		return model.DenominatorRef{}, false
	}
	return input.Denominator, true
}

// CarrierDenominator names the delivery range authority.  It is an accessor
// rather than an alias so callers that need both authorities do not silently
// treat Input.Denominator as the source-cell authority.
func (input Input) CarrierDenominator() model.DenominatorRef { return input.Denominator }

// NewHomogeneousInput authors the compact one-authority input variant.  It
// is the explicit constructor for ordinary existing inputs; direct literals
// retain this same zero-tag member only when relation and carrier agree.
func NewHomogeneousInput(relation model.RelationID, column model.ColumnID, typeID model.TypeID, presence PresenceContract, delivery Delivery, denominator model.DenominatorRef) (Input, bool) {
	input := Input{Relation: relation, Column: column, Type: typeID, Presence: presence, Delivery: delivery, Denominator: denominator}
	return input, input.Available()
}

// NewJoinedInput authors the dual-authority input variant.  source
// authenticates the delivered cell, while carrier remains Input.Denominator
// and authenticates the span range/order.  The two relations must differ;
// same-relation double declarations are refused as ambiguous.
func NewJoinedInput(relation model.RelationID, column model.ColumnID, typeID model.TypeID, presence PresenceContract, delivery Delivery, source model.DenominatorRef, carrier model.DenominatorRef) (Input, bool) {
	authority, ok := NewSourceAuthority(source)
	if !ok {
		return Input{}, false
	}
	input := Input{Relation: relation, Column: column, Type: typeID, Presence: presence, Delivery: delivery, Denominator: carrier, SourceAuthority: authority}
	return input, input.Available()
}

// Same compares the complete sealed ABI-facing input contract, including its
// source/carrier authority variant.  Mount and runtime callers use it where
// accepting a same-looking homogeneous input as a joined input would erase a
// semantic authority boundary.
func (input Input) Same(other Input) bool {
	return input.Relation == other.Relation && input.Column == other.Column && input.Type == other.Type && input.Presence == other.Presence && input.Delivery == other.Delivery && input.Denominator == other.Denominator && input.SourceAuthority.Same(other.SourceAuthority)
}

type Output struct {
	Relation model.RelationID
	Column   model.ColumnID
	Type     model.TypeID
	Presence PresenceContract
	// Denominator is the owner-issued destination population for this output.
	// It is mandatory at authoring and is never inherited from another
	// operation field.
	Denominator model.DenominatorRef
}

func (output Output) Available() bool {
	return output.Relation.Available() && output.Column.Available() && output.Type.Available() && output.Presence.Output() && output.Denominator.Available()
}

type Spec struct {
	Identity    Identity
	Fence       Fence
	Inputs      []Input
	Outputs     []Output
	Cardinality model.Cardinality
	Outcomes    outcome.Set
}
