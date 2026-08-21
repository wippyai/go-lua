package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// CallResult is the neutral output geometry of one authored Call.  A Call
// which is used in a fixed Values position names its ValuesMember; an open
// Values tail names its ValuesTail instead.  The target outcome/result is
// deliberately absent here: that identity belongs to the mounted
// Program/Target join, while this row preserves the reusable Program shape
// once.
//
// The row uses the parent-issued Call identity as its own key.  There is no
// second result identity or zero-filled tuple for a call that has no value
// context (for example a statement call whose results are discarded).
type CallResultForm uint8

const (
	CallResultInvalid CallResultForm = iota
	CallResultValue
	CallResultValues
	// CallResultDirectValue is a scalar-adjusted Call consumed directly by
	// another expression/control row rather than through an authored Values
	// member. Its existing Call evaluation span is the result ValueID.
	CallResultDirectValue
)

func (form CallResultForm) Valid() bool {
	return form >= CallResultValue && form <= CallResultDirectValue
}

// CallResultMultiplicity is the consumer-side cardinality of a Call's
// authored output.  Exact rows name the finite number of result ordinals
// admitted by their consumer (the first ordinal is always zero); Open rows
// preserve Lua's unbounded final-expression expansion.  This is deliberately
// separate from CallResultForm: a Call can be the Values tail while its
// enclosing Bind/Assign consumes only a finite suffix of that tail.
type CallResultMultiplicity uint8

const (
	CallResultMultiplicityInvalid CallResultMultiplicity = iota
	CallResultMultiplicityExact
	CallResultMultiplicityOpen
)

func (multiplicity CallResultMultiplicity) Valid() bool {
	return multiplicity == CallResultMultiplicityExact || multiplicity == CallResultMultiplicityOpen
}

type CallResult struct {
	call     identity.ContentID
	values   identity.ContentID
	value    identity.ContentID
	tail     identity.ContentID
	position uint32
	form     CallResultForm
	// multiplicity/count are the target-independent consumer geometry. Count
	// is meaningful for Exact and may be zero when the tail is fully discarded;
	// Open carries no finite count.
	multiplicity CallResultMultiplicity
	count        uint32
	// slotOffset/slotCount name the contiguous CallResultSlot child span.
	// Open tails have no finite slots and therefore always carry a zero span.
	slotOffset uint32
	slotCount  uint32
}

// NewCallResultWithMultiplicity copies one exact/open authored Call result
// admission. Exact count zero is valid as construction evidence but is not
// emitted by the compiler because no output coordinate is consumed. The
// optional slotSpan argument accepts exactly two values (offset,count) for
// callers that already know the dense child placement. Omitting it derives
// the canonical count from the result shape and uses offset zero, preserving
// the historical constructor contract.
func NewCallResultWithMultiplicity(call, values, value, tail identity.ContentID, position uint32, form CallResultForm, multiplicity CallResultMultiplicity, count uint32, slotSpan ...uint32) (CallResult, bool) {
	slotOffset, slotCount, spanOK := callResultDefaultSlotSpan(form, multiplicity, count)
	if len(slotSpan) != 0 {
		if len(slotSpan) != 2 {
			return CallResult{}, false
		}
		slotOffset, slotCount = slotSpan[0], slotSpan[1]
	}
	if !spanOK {
		return CallResult{}, false
	}
	row := CallResult{call: call, values: values, value: value, tail: tail, position: position, form: form, multiplicity: multiplicity, count: count, slotOffset: slotOffset, slotCount: slotCount}
	return row, row.Available()
}

// NewCallResultWithMultiplicityAndSlots is the explicit spelling for callers
// that want to make the child span visible at the call site.
func NewCallResultWithMultiplicityAndSlots(call, values, value, tail identity.ContentID, position uint32, form CallResultForm, multiplicity CallResultMultiplicity, count, slotOffset, slotCount uint32) (CallResult, bool) {
	return NewCallResultWithMultiplicity(call, values, value, tail, position, form, multiplicity, count, slotOffset, slotCount)
}

// NewCallResult retains the original fixed/open convenience constructor. A
// trailing optional slot span follows the same (offset,count) convention as
// NewCallResultWithMultiplicity.
func NewCallResult(call, values, value, tail identity.ContentID, position uint32, form CallResultForm, slotSpan ...uint32) (CallResult, bool) {
	multiplicity := CallResultMultiplicityOpen
	count := uint32(0)
	if form == CallResultValue || form == CallResultDirectValue {
		multiplicity = CallResultMultiplicityExact
		count = 1
	}
	return NewCallResultWithMultiplicity(call, values, value, tail, position, form, multiplicity, count, slotSpan...)
}

func callResultDefaultSlotSpan(form CallResultForm, multiplicity CallResultMultiplicity, count uint32) (uint32, uint32, bool) {
	switch form {
	case CallResultValue, CallResultDirectValue:
		return 0, 1, multiplicity == CallResultMultiplicityExact && count == 1
	case CallResultValues:
		if multiplicity == CallResultMultiplicityOpen {
			return 0, 0, count == 0
		}
		if multiplicity == CallResultMultiplicityExact {
			return 0, count, true
		}
	}
	return 0, 0, false
}

func (row CallResult) Available() bool {
	if !row.call.Available() || !row.form.Valid() || !row.multiplicity.Valid() {
		return false
	}
	switch row.form {
	case CallResultValue:
		return row.values.Available() && row.value.Available() && !row.tail.Available() && row.multiplicity == CallResultMultiplicityExact && row.count == 1 && row.slotCount == 1
	case CallResultValues:
		return row.values.Available() && !row.value.Available() && row.tail.Available() && row.position == 0 &&
			(row.multiplicity == CallResultMultiplicityExact ||
				(row.multiplicity == CallResultMultiplicityOpen && row.count == 0)) &&
			(row.multiplicity == CallResultMultiplicityOpen && row.slotOffset == 0 && row.slotCount == 0 ||
				row.multiplicity == CallResultMultiplicityExact && row.slotCount == row.count)
	case CallResultDirectValue:
		return !row.values.Available() && row.value.Available() && !row.tail.Available() && row.position == 0 &&
			row.multiplicity == CallResultMultiplicityExact && row.count == 1 && row.slotCount == 1
	default:
		return false
	}
}

// ID is the existing authored Call identity.  CallResult is a one-to-one
// output projection, so issuing another identity would create a competing
// occurrence vocabulary.
func (row CallResult) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

func (row CallResult) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

func (row CallResult) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}

// ValueID returns the exact fixed-position Value member, when the call is
// consumed as one scalar expression.
func (row CallResult) ValueID() (identity.ContentID, bool) {
	return row.value, row.Available() && (row.form == CallResultValue || row.form == CallResultDirectValue)
}

// ValuesTailID returns the exact open Values tail, when the call supplies the
// result sequence of its enclosing Values row.
func (row CallResult) ValuesTailID() (identity.ContentID, bool) {
	return row.tail, row.Available() && row.form == CallResultValues
}

func (row CallResult) Form() CallResultForm {
	if !row.Available() {
		return CallResultInvalid
	}
	return row.form
}

// Multiplicity reports the consumer-side result admission shape.
func (row CallResult) Multiplicity() CallResultMultiplicity {
	if !row.Available() {
		return CallResultMultiplicityInvalid
	}
	return row.multiplicity
}

// ResultCount returns the exact finite number of admitted result ordinals.
// Open rows intentionally have no count and return false.
func (row CallResult) ResultCount() (uint32, bool) {
	return row.count, row.Available() && row.multiplicity == CallResultMultiplicityExact
}

// SlotSpan names the contiguous finite CallResultSlot children. An open tail
// is a producer expansion and intentionally reports a zero-width span.
func (row CallResult) SlotSpan() (offset, count uint32, ok bool) {
	return row.slotOffset, row.slotCount, row.Available()
}

func (row CallResult) SlotOffset() (uint32, bool) {
	return row.slotOffset, row.Available()
}

func (row CallResult) SlotCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.slotCount)
}

// ResultsOpen reports whether this Call expands without a finite consumer
// bound. The second return authenticates the row itself.
func (row CallResult) ResultsOpen() (bool, bool) {
	return row.multiplicity == CallResultMultiplicityOpen, row.Available()
}

// AdmitsResult is the canonical target-independent ordinal admission law.
// Consumers use the same law after joining a Target OutcomeResult identity.
func (row CallResult) AdmitsResult(result uint32) bool {
	if !row.Available() {
		return false
	}
	if row.form == CallResultValue || row.form == CallResultDirectValue {
		return result == 0
	}
	return row.multiplicity == CallResultMultiplicityOpen || result < row.count
}

// Position is meaningful only for a fixed member.  An open tail has no
// fabricated fixed position; the mounted Target join supplies the target
// result ordinal.
func (row CallResult) Position() (uint32, bool) {
	return row.position, row.Available() && row.form == CallResultValue
}
