package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
)

func callResultSlotLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestCallResultSlotIdentityFramesEveryCoordinate(t *testing.T) {
	call, consumer, value := callResultSlotLawID(1), callResultSlotLawID(33), callResultSlotLawID(65)
	id, ok := CallResultSlotIdentity(call, 2, CallResultSlotSourceValue, CallResultSlotConsumerValuesMember, consumer, 4, value)
	if !ok || !id.Available() {
		t.Fatal("fixed call-result slot identity was unavailable")
	}
	row, rowOK := NewCallResultSlot(id, call, 2, CallResultSlotSourceValue, CallResultSlotConsumerValuesMember, consumer, 4, value)
	if !rowOK || !row.Available() {
		t.Fatal("fixed call-result slot row was unavailable")
	}
	if row.ID() != id || row.CallID() != call || row.Index() != 2 || row.SourceKind() != CallResultSlotSourceValue || row.ConsumerKind() != CallResultSlotConsumerValuesMember || row.ConsumerID() != consumer {
		t.Fatal("fixed call-result slot lost a framed coordinate")
	}
	if position, positionOK := row.ConsumerPosition(); !positionOK || position != 4 {
		t.Fatal("fixed call-result slot lost consumer position")
	}
	if got, valueOK := row.ValueID(); !valueOK || got != value {
		t.Fatal("fixed call-result slot lost ValuesMember identity")
	}
	other, otherOK := CallResultSlotIdentity(call, 3, CallResultSlotSourceValue, CallResultSlotConsumerValuesMember, consumer, 4, value)
	if !otherOK || other == id {
		t.Fatal("result ordinal was not part of the slot identity")
	}
}

func TestCallResultSlotTailNeverCarriesTheProducerTailAsAValue(t *testing.T) {
	call, consumer, tail := callResultSlotLawID(9), callResultSlotLawID(41), callResultSlotLawID(73)
	id, idOK := CallResultSlotIdentity(call, 0, CallResultSlotSourceValuesTail, CallResultSlotConsumerCell, consumer, 0, tail)
	if !idOK {
		t.Fatal("tail slot identity")
	}
	row, rowOK := NewCallResultSlot(id, call, 0, CallResultSlotSourceValuesTail, CallResultSlotConsumerCell, consumer, 0, tail)
	if !rowOK || !row.Available() {
		t.Fatal("tail slot with a consumer-backed value was not constructed")
	}
	if got, valueOK := row.ValueID(); !valueOK || got != tail {
		t.Fatal("row accessor should expose only the supplied consumer identity")
	}
	// Publication validation compares this optional value against the parent
	// ValuesTailID. The row itself intentionally has no ValuesTailID accessor.
	if _, hasTailScalar := any(row).(interface {
		ValuesTailID() (identity.ContentID, bool)
	}); hasTailScalar {
		t.Fatal("CallResultSlot exposed a scalar ValuesTailID")
	}
	withoutValueID, withoutValueOK := NewDerivedCallResultSlot(callResultSlotLawID(10), 1, CallResultSlotSourceValuesTail, CallResultSlotConsumerLens, callResultSlotLawID(42), 3, identity.ContentID{})
	if !withoutValueOK || !withoutValueID.Available() {
		t.Fatal("structural tail slot should allow an absent consumer ValueID")
	}
	if _, valueOK := withoutValueID.ValueID(); valueOK {
		t.Fatal("absent structural ValueID was reported as present")
	}
}

func TestCallResultSlotDirectScalarCarriesExistingCallValue(t *testing.T) {
	call, consumer, value := callResultSlotLawID(11), callResultSlotLawID(43), callResultSlotLawID(75)
	row, ok := NewDerivedCallResultSlot(
		call, 0, CallResultSlotSourceCallValue,
		CallResultSlotConsumerStructural, consumer, 0, value,
	)
	if !ok || !row.Available() || row.SourceKind() != CallResultSlotSourceCallValue ||
		row.ConsumerKind() != CallResultSlotConsumerStructural {
		t.Fatal("direct scalar Call slot was unavailable")
	}
	if got, valueOK := row.ValueID(); !valueOK || got != value {
		t.Fatal("direct scalar Call slot lost its existing evaluation-span value")
	}
	if malformed, malformedOK := NewDerivedCallResultSlot(
		callResultSlotLawID(12), 0, CallResultSlotSourceCallValue,
		CallResultSlotConsumerStructural, consumer, 0, identity.ContentID{},
	); malformedOK || malformed.Available() {
		t.Fatal("direct scalar Call slot accepted an absent ValueID")
	}
}

func TestCallResultSlotFamilyRejectsUnavailableRows(t *testing.T) {
	catalog, catalogOK := programcatalog.CatalogID(callResultSlotLawID(101))
	if !catalogOK {
		t.Fatal("slot law catalog")
	}
	id, idOK := CallResultSlotIdentity(callResultSlotLawID(5), 0, CallResultSlotSourceValue, CallResultSlotConsumerValuesMember, callResultSlotLawID(37), 0, callResultSlotLawID(69))
	if !idOK {
		t.Fatal("slot law identity")
	}
	row, rowOK := NewCallResultSlot(id, callResultSlotLawID(5), 0, CallResultSlotSourceValue, CallResultSlotConsumerValuesMember, callResultSlotLawID(37), 0, callResultSlotLawID(69))
	if !rowOK {
		t.Fatal("slot law row")
	}
	if _, sealed := CallResultSlotFamily().Content([]CallResultSlot{row, CallResultSlot{}}, catalog); sealed {
		t.Fatal("slot family admitted an unavailable row")
	}
	if _, sealed := CallResultSlotFamily().Content([]CallResultSlot{row}, identity.ContentID{}); sealed {
		t.Fatal("slot family sealed under an unavailable catalog")
	}
}

func TestCallResultSlotParentSpanDistinguishesExactAndOpenTails(t *testing.T) {
	call, values, tail := callResultSlotLawID(121), callResultSlotLawID(153), callResultSlotLawID(185)
	exact, exactOK := NewCallResultWithMultiplicity(call, values, identity.ContentID{}, tail, 0, CallResultValues, CallResultMultiplicityExact, 3, 7, 3)
	open, openOK := NewCallResultWithMultiplicity(callResultSlotLawID(122), values, identity.ContentID{}, tail, 0, CallResultValues, CallResultMultiplicityOpen, 0)
	if !exactOK || !openOK {
		t.Fatal("call-result span fixtures")
	}
	if offset, count, spanOK := exact.SlotSpan(); !spanOK || offset != 7 || count != 3 || exact.SlotCount() != 3 {
		t.Fatal("exact tail did not retain its finite slot span")
	}
	if offset, count, spanOK := open.SlotSpan(); !spanOK || offset != 0 || count != 0 || open.SlotCount() != 0 {
		t.Fatal("open tail fabricated a slot span")
	}
	if malformed, malformedOK := NewCallResultWithMultiplicity(callResultSlotLawID(123), values, identity.ContentID{}, tail, 0, CallResultValues, CallResultMultiplicityOpen, 0, 1, 0); malformedOK || malformed.Available() {
		t.Fatal("open tail accepted a nonzero slot offset")
	}
}
