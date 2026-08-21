package programschema

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// CallResultSlotSourceKind identifies which authored Call output supplied a
// finite result coordinate. A fixed Values member is a source in its own
// right; a Values tail is a producer source and is never copied into a scalar
// ValuesTailID field on this row.
type CallResultSlotSourceKind uint8

const (
	CallResultSlotSourceInvalid CallResultSlotSourceKind = iota
	CallResultSlotSourceValue
	CallResultSlotSourceValuesTail
	CallResultSlotSourceCallValue

	// Short aliases keep the vocabulary readable at call sites while the
	// longer names remain the canonical exported spellings.
	CallResultSlotSourceTail   = CallResultSlotSourceValuesTail
	CallResultSlotSourceMember = CallResultSlotSourceValue
	CallResultSlotSourceFixed  = CallResultSlotSourceValue
)

func (kind CallResultSlotSourceKind) Valid() bool {
	return kind >= CallResultSlotSourceValue && kind <= CallResultSlotSourceCallValue
}

// CallResultSlotConsumerKind identifies the consumer coordinate that admits
// one result ordinal. The schema deliberately keeps this closed and neutral:
// storage cells and lenses are existing identities supplied by their owning
// relation, not synthetic Value coordinates invented by this family.
type CallResultSlotConsumerKind uint8

const (
	CallResultSlotConsumerInvalid CallResultSlotConsumerKind = iota
	CallResultSlotConsumerValuesMember
	CallResultSlotConsumerCell
	CallResultSlotConsumerLens
	CallResultSlotConsumerStructural

	// Common vocabulary aliases. They are aliases, rather than additional
	// ordinals, so identity and validation cannot acquire duplicate meanings.
	CallResultSlotConsumerValue       = CallResultSlotConsumerValuesMember
	CallResultSlotConsumerValues      = CallResultSlotConsumerValuesMember
	CallResultSlotConsumerStorageCell = CallResultSlotConsumerCell
	CallResultSlotConsumerLoopCell    = CallResultSlotConsumerCell
	CallResultSlotConsumerAccessLens  = CallResultSlotConsumerLens
	CallResultSlotConsumerNonValue    = CallResultSlotConsumerStructural
	CallResultSlotConsumerSink        = CallResultSlotConsumerStructural
)

func (kind CallResultSlotConsumerKind) Valid() bool {
	return kind >= CallResultSlotConsumerValuesMember && kind <= CallResultSlotConsumerStructural
}

// CallResultSlotIdentity derives the stable coordinate identity for one
// canonical slot. Every scalar is framed independently; in particular the
// optional ValueID carries an explicit presence bit so an absent consumer
// value cannot collide with an unavailable identity's zero bytes.
func CallResultSlotIdentity(
	call identity.ContentID,
	ordinal uint32,
	sourceKind CallResultSlotSourceKind,
	consumerKind CallResultSlotConsumerKind,
	consumerID identity.ContentID,
	consumerPosition uint32,
	valueID identity.ContentID,
) (identity.ContentID, bool) {
	if !call.Available() || !sourceKind.Valid() || !consumerKind.Valid() || !consumerID.Available() {
		return identity.ContentID{}, false
	}
	if (sourceKind == CallResultSlotSourceValue || sourceKind == CallResultSlotSourceCallValue) && !valueID.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/call-result-slot-v2", 2) != nil ||
		writer.Record(1) != nil ||
		writer.Bytes(call[:]) != nil ||
		writer.Uint(uint64(ordinal)) != nil ||
		writer.Uint(uint64(sourceKind)) != nil ||
		writer.Uint(uint64(consumerKind)) != nil ||
		writer.Bytes(consumerID[:]) != nil ||
		writer.Uint(uint64(consumerPosition)) != nil ||
		writer.Bool(valueID.Available()) != nil ||
		writer.Bytes(valueID[:]) != nil ||
		writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// CallResultSlotSyntheticValueIdentity derives the semantic Value identity
// for a finite tail slot whose consumer is an existing storage Cell.  The
// slot's Cell remains the consumer endpoint; this separate identity is the
// Value coordinate that carries the producer result before storage emits its
// explicit slot-to-Cell transfer.  Keeping the two identities distinct is
// essential: a Cell may already contain an unrelated Top contribution (for
// example the call-base effect), while the bounded Call result starts at its
// own coordinate.
func CallResultSlotSyntheticValueIdentity(
	call identity.ContentID,
	ordinal uint32,
	consumerKind CallResultSlotConsumerKind,
	consumerID identity.ContentID,
	consumerPosition uint32,
) (identity.ContentID, bool) {
	if !call.Available() || !consumerKind.Valid() || !consumerID.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/call-result-slot-value-v1", 1) != nil ||
		writer.Record(1) != nil ||
		writer.Bytes(call[:]) != nil ||
		writer.Uint(uint64(ordinal)) != nil ||
		writer.Uint(uint64(consumerKind)) != nil ||
		writer.Bytes(consumerID[:]) != nil ||
		writer.Uint(uint64(consumerPosition)) != nil ||
		writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// CallResultSlot is one exact, finite result coordinate admitted by a Call's
// consumer. The row is intentionally not a Values row and has no ValuesTailID
// scalar: an open tail remains a producer identity on CallResult, while an
// exact bounded tail may contribute one row per admitted ordinal.
type CallResultSlot struct {
	id               identity.ContentID
	call             identity.ContentID
	ordinal          uint32
	sourceKind       CallResultSlotSourceKind
	consumerKind     CallResultSlotConsumerKind
	consumer         identity.ContentID
	consumerPosition uint32
	value            identity.ContentID
}

// NewCallResultSlot copies one compiler-proved slot. The supplied ID must be
// the framed identity of the remaining immutable coordinates; constructors do
// not mint a second identity authority.
func NewCallResultSlot(
	id,
	call identity.ContentID,
	ordinal uint32,
	sourceKind CallResultSlotSourceKind,
	consumerKind CallResultSlotConsumerKind,
	consumerID identity.ContentID,
	consumerPosition uint32,
	valueID identity.ContentID,
) (CallResultSlot, bool) {
	row := CallResultSlot{
		id: id, call: call, ordinal: ordinal, sourceKind: sourceKind,
		consumerKind: consumerKind, consumer: consumerID,
		consumerPosition: consumerPosition, value: valueID,
	}
	return row, row.Available()
}

// NewDerivedCallResultSlot derives the row identity and then copies the
// complete slot. It is useful to construction code that owns all coordinates
// but does not need to carry a transient identity result separately.
func NewDerivedCallResultSlot(
	call identity.ContentID,
	ordinal uint32,
	sourceKind CallResultSlotSourceKind,
	consumerKind CallResultSlotConsumerKind,
	consumerID identity.ContentID,
	consumerPosition uint32,
	valueID identity.ContentID,
) (CallResultSlot, bool) {
	id, ok := CallResultSlotIdentity(call, ordinal, sourceKind, consumerKind, consumerID, consumerPosition, valueID)
	if !ok {
		return CallResultSlot{}, false
	}
	return NewCallResultSlot(id, call, ordinal, sourceKind, consumerKind, consumerID, consumerPosition, valueID)
}

func (row CallResultSlot) Available() bool {
	if !row.id.Available() || !row.call.Available() || !row.sourceKind.Valid() || !row.consumerKind.Valid() || !row.consumer.Available() {
		return false
	}
	if (row.sourceKind == CallResultSlotSourceValue || row.sourceKind == CallResultSlotSourceCallValue) && !row.value.Available() {
		return false
	}
	id, ok := CallResultSlotIdentity(row.call, row.ordinal, row.sourceKind, row.consumerKind, row.consumer, row.consumerPosition, row.value)
	return ok && id == row.id
}

func (row CallResultSlot) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row CallResultSlot) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

// Ordinal is the result ordinal relative to the parent CallResult. It is
// required even when the optional consumer ValueID is unavailable.
func (row CallResultSlot) Ordinal() (uint32, bool) {
	return row.ordinal, row.Available()
}

// ResultOrdinal is a descriptive alias for Ordinal.
func (row CallResultSlot) ResultOrdinal() (uint32, bool) { return row.Ordinal() }

// Index is the dense result ordinal. Invalid rows return zero, matching the
// other required ordinal accessors in the Program schema.
func (row CallResultSlot) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.ordinal
}

func (row CallResultSlot) SourceKind() CallResultSlotSourceKind {
	if !row.Available() {
		return CallResultSlotSourceInvalid
	}
	return row.sourceKind
}

func (row CallResultSlot) Source() CallResultSlotSourceKind { return row.SourceKind() }

func (row CallResultSlot) ConsumerKind() CallResultSlotConsumerKind {
	if !row.Available() {
		return CallResultSlotConsumerInvalid
	}
	return row.consumerKind
}

func (row CallResultSlot) Consumer() CallResultSlotConsumerKind { return row.ConsumerKind() }

func (row CallResultSlot) ConsumerID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.consumer
}

func (row CallResultSlot) ConsumerPosition() (uint32, bool) {
	return row.consumerPosition, row.Available()
}

func (row CallResultSlot) Position() (uint32, bool) { return row.ConsumerPosition() }

// ValueID is optional for structural/lens consumers. For a fixed Values
// source it is always the existing ValuesMember identity. A bounded tail
// Cell carries a synthetic result-coordinate identity; the Cell itself is
// retained separately by ConsumerID.
func (row CallResultSlot) ValueID() (identity.ContentID, bool) {
	return row.value, row.Available() && row.value.Available()
}

func (row CallResultSlot) HasValue() bool {
	_, ok := row.ValueID()
	return ok
}
