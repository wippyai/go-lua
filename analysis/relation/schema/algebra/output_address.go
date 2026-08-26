package algebra

// OutputAddress is the sealed destination geometry of one Apply expression.
// It is deliberately part of the algebra rather than a semantic signature or
// generated binding: the same operation may be placed by different plans,
// while one plan must carry exactly one authenticated destination source.
//
// A source names an already-retained Apply child/cell coordinate.  The checker
// proves that coordinate is retained by the expression and that its row
// content is the row population of every published output.  Mount then
// resolves it once to a borrowed runtime row view.  OwnerNamed is the sole
// form that lets an owner operation choose a row identity itself.
type OutputAddress struct {
	kind   outputAddressKind
	source SlotSource
}

type outputAddressKind uint8

const (
	outputAddressInvalid outputAddressKind = iota
	outputAddressScalar
	outputAddressSpan
	outputAddressOwnerNamed
)

// ScalarSource addresses one output row from a retained scalar child/cell
// source.  The source must be proven by the Apply checker; this constructor
// only records the closed declaration.
func ScalarSource(source SlotSource) OutputAddress {
	return OutputAddress{kind: outputAddressScalar, source: source}
}

// SpanSource addresses output rows sequentially from a retained complete span
// child/cell source.  The source must be proven against the bounded destination
// denominator by the Apply checker.
func SpanSource(source SlotSource) OutputAddress {
	return OutputAddress{kind: outputAddressSpan, source: source}
}

// OwnerNamed declares the only output mode in which the owner operation may
// supply destination row identities through PutAt/PutAbsentAt.
func OwnerNamed() OutputAddress {
	return OutputAddress{kind: outputAddressOwnerNamed}
}

// Available reports whether this is one of the three closed address forms.
// Positional bounds and row/type relationships are checker obligations, not
// a second local interpretation of the plan.
func (address OutputAddress) Available() bool {
	switch address.kind {
	case outputAddressScalar, outputAddressSpan:
		return true
	case outputAddressOwnerNamed:
		return address.source == (SlotSource{})
	default:
		return false
	}
}

func (address OutputAddress) IsScalarSource() bool {
	return address.Available() && address.kind == outputAddressScalar
}

func (address OutputAddress) IsSpanSource() bool {
	return address.Available() && address.kind == outputAddressSpan
}

func (address OutputAddress) IsOwnerNamed() bool {
	return address.Available() && address.kind == outputAddressOwnerNamed
}

// Source returns the retained positional source for ScalarSource and
// SpanSource. OwnerNamed has no positional source.
func (address OutputAddress) Source() (SlotSource, bool) {
	if !address.IsScalarSource() && !address.IsSpanSource() {
		return SlotSource{}, false
	}
	return address.source, true
}

// digestBytes is consumed by ApplyContract's existing algebra digest. There
// is intentionally no independent address digest or generated certificate.
func (address OutputAddress) digestBytes() []byte {
	parts := appendUint8(nil, uint8(address.kind))
	if address.IsScalarSource() || address.IsSpanSource() {
		parts = appendUint32(parts, address.source.child)
		parts = appendUint32(parts, address.source.cell)
	}
	return parts
}
