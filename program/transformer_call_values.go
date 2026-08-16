package program

import (
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// CallValues is the exact ordered Values projection owned by one
// CallOccurrence.Actuals proof. It retains no Values row or member table;
// Count/At forward the immutable Program-sealed Values receipt.
type CallValues struct {
	occurrence CallOccurrence
	input      TransformerInput
	call       keyspace.Term
	receipt    ValuesOccurrence
	callID     keyspace.ContentID
	valuesID   keyspace.ContentID
	width      int
	span       Span
}

// Values returns the call's exact authored actual-values sequence.
func (call CallOccurrence) Values() (CallValues, bool) {
	if !call.Available() || call.values.catalog == nil || call.values.catalog.input != call.input || !call.values.Available() {
		return CallValues{}, false
	}
	row, rowOK := call.values.row()
	if !rowOK {
		return CallValues{}, false
	}
	values := CallValues{occurrence: call, input: call.input, call: call.call, receipt: call.values, callID: call.semanticID, valuesID: call.valuesID, width: row.width, span: row.span}
	return values, values.Available()
}

func (values CallValues) Available() bool {
	if !values.input.Available() || values.occurrence.input != values.input ||
		values.occurrence.call != values.call || values.width < 0 || !values.callID.Available() || !values.valuesID.Available() ||
		values.occurrence.semanticID != values.callID || values.occurrence.valuesID != values.valuesID || values.receipt.catalog == nil ||
		values.receipt.catalog.owner != values.input.owner || values.input.owner.valuesCatalog != values.receipt.catalog || !values.receipt.Available() || values.receipt.catalog.input != values.input {
		return false
	}
	row, rowOK := values.receipt.row()
	return rowOK && values.receipt.ID() == values.valuesID && row.spanOK && row.width == values.width && row.span == values.span && values.occurrence.values == values.receipt
}

func (values CallValues) ContextID() keyspace.ContentID {
	if !values.Available() {
		return keyspace.ContentID{}
	}
	valuesID := values.receipt.ID()
	return transformerSemanticID("program/transformer/call-values", func(writer *canonical.Writer) bool {
		return writer.Bytes(values.callID[:]) == nil && writer.Bytes(valuesID[:]) == nil
	})
}

// Span returns the exact authored root span of the actual-values sequence.
// The Values term itself remains private.
func (values CallValues) Span() (Span, bool) {
	if !values.Available() {
		return Span{}, false
	}
	return values.span, true
}

func (values CallValues) Count() int {
	if !values.Available() {
		return 0
	}
	return values.receipt.Count()
}

func (values CallValues) At(index int) (CallArgument, bool) {
	if !values.Available() || index < 0 || index >= values.width {
		return CallArgument{}, false
	}
	member, memberOK := values.receipt.At(index)
	argument := CallArgument{values: values, index: index, member: member}
	return argument, memberOK && argument.Available()
}

// Tail returns the exact open-tail producer receipt when this Values sequence is
// open. Closed Values deliberately return no proof; callers distinguish that
// from an unavailable CallValues parent through values.Available().
func (values CallValues) Tail() (TailProducer, bool) {
	if !values.Available() {
		return TailProducer{}, false
	}
	producer, ok := values.receipt.Tail()
	if !ok {
		return TailProducer{}, false
	}
	return producer, true
}

func (input TransformerInput) OwnsCallValues(values CallValues) bool {
	return input.Available() && values.input == input && values.Available()
}

// Equal compares the exact immutable CallValues receipt without relying on
// Go struct comparability (the shared Values receipt owns sealed slices).
func (values CallValues) Equal(other CallValues) bool {
	return values.Available() && other.Available() && values.input == other.input && values.call == other.call &&
		values.receipt == other.receipt && values.callID == other.callID && values.valuesID == other.valuesID && values.width == other.width && values.span == other.span
}

// CallArgument is one opaque, position-fenced member of CallValues. Its
// authored value term remains private; consumers carry this proof onward.
type CallArgument struct {
	values CallValues
	index  int
	member ValuesMember
}

func (argument CallArgument) Available() bool {
	if !argument.values.Available() || argument.index < 0 || argument.index >= argument.values.width || !argument.member.Available() {
		return false
	}
	return argument.member.index == argument.index && argument.member.values == argument.values.receipt
}

func (argument CallArgument) ContextID() keyspace.ContentID {
	if !argument.Available() {
		return keyspace.ContentID{}
	}
	valuesID := argument.values.ContextID()
	memberID := argument.member.ID()
	return transformerSemanticID("program/transformer/call-argument", func(writer *canonical.Writer) bool {
		return writer.Bytes(valuesID[:]) == nil && writer.Bytes(memberID[:]) == nil
	})
}

// Span returns this exact ordered argument member's Program occurrence span.
// The underlying Values member term remains private.
func (argument CallArgument) Span() (Span, bool) {
	if !argument.Available() {
		return Span{}, false
	}
	return argument.member.Span()
}

func (argument CallArgument) Values() (CallValues, bool) {
	if !argument.Available() {
		return CallValues{}, false
	}
	return argument.values, true
}

func (argument CallArgument) Position() (int, bool) {
	if !argument.Available() {
		return 0, false
	}
	return argument.index, true
}

// IssuedArgumentPosition authenticates an already-issued exact child against
// this exact CallValues capability in O(1). CallValues and CallArgument are
// opaque immutable Program proofs: their complete Flow membership was checked
// at issuance, so a sealed parent may thread this narrow receipt without
// replaying Values.Member or the occurrence proof on every hot lookup.
func (values CallValues) IssuedArgumentPosition(argument CallArgument) (int, bool) {
	if values.input.owner == nil || !values.Available() || values.width < 0 || argument.values.input != values.input || argument.values.call != values.call ||
		argument.values.receipt != values.receipt || !argument.Available() || argument.index < 0 || argument.index >= values.width ||
		argument.member.values != values.receipt || argument.member.index != argument.index {
		return 0, false
	}
	return argument.index, true
}

func (input TransformerInput) OwnsCallArgument(argument CallArgument) bool {
	return input.Available() && argument.values.input == input && argument.Available()
}

func exactCallValuesSpan(input TransformerInput, span Span, actuals keyspace.Term) bool {
	if !input.Available() || !span.Available() || !input.OwnsSpan(span) {
		return false
	}
	want, ok := input.Span(actuals)
	return ok && span.Equal(want)
}
