package summary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// PlacementSummaryParentColumns are the typed columns used by the parent
// summary operation. QuerySite is deliberately only an address descriptor:
// the parent never decodes or stores a query-site payload. Its row address is
// obtained from the scalar input frame by relbindgen itself.
//
// The three child columns are the one child relation's complete allocation
// denominator. They are adopted from the already sealed child relation; this
// type creates no relation, denominator, or identity authority.
type PlacementSummaryParentColumns struct {
	QuerySiteColumn model.ColumnID
	QuerySiteType   model.TypeID
	AllocationID    *relbindgen.Column[identity.ContentID]
	Fact            *relbindgen.Column[placementdomain.Fact]
	Evidence        *relbindgen.Column[placementdomain.AllocationEvidence]
	Output          ParentCodec
}

// NewPlacementSummaryParentColumns adopts the address-only QuerySite
// descriptor, the three child codecs, and the already declared parent output
// codec. No QuerySite value codec is accepted because the parent semantics use
// only its authenticated row address.
func NewPlacementSummaryParentColumns(
	querySiteColumn model.ColumnID,
	querySiteType model.TypeID,
	allocationID *relbindgen.Column[identity.ContentID],
	fact *relbindgen.Column[placementdomain.Fact],
	evidence *relbindgen.Column[placementdomain.AllocationEvidence],
	output ParentColumns,
) (PlacementSummaryParentColumns, bool) {
	columns := PlacementSummaryParentColumns{
		QuerySiteColumn: querySiteColumn,
		QuerySiteType:   querySiteType,
		AllocationID:    allocationID,
		Fact:            fact,
		Evidence:        evidence,
	}
	codec, ok := NewParentCodec(output)
	if !ok {
		return PlacementSummaryParentColumns{}, false
	}
	columns.Output = codec
	if !columns.Available() {
		return PlacementSummaryParentColumns{}, false
	}
	return columns, true
}

// Available reports whether the address descriptor, child codecs, and parent
// output codec are complete.
func (columns PlacementSummaryParentColumns) Available() bool {
	return columns.QuerySiteColumn.Available() &&
		columns.QuerySiteType.Available() &&
		columns.AllocationID.Available() &&
		columns.Fact.Available() &&
		columns.Evidence.Available() &&
		columns.Output.Available()
}

// PlacementSummaryParentArgument is the borrowed child relation delivered to
// one query-site parent invocation. The QuerySite scalar is not present here:
// it is an address-only input, consumed by the binding's declared address
// slot, and therefore is not a second semantic payload in the argument.
type PlacementSummaryParentArgument struct {
	AllocationIDs relbindgen.Span[identity.ContentID]
	Facts         relbindgen.Span[placementdomain.Fact]
	Evidence      relbindgen.Span[placementdomain.AllocationEvidence]
}

// PlacementSummaryParentJudgment is the owner mathematics for the parent
// answer. The concrete operation below is the only implementation in this
// package; the interface keeps the binding independent of its constructor.
type PlacementSummaryParentJudgment interface {
	relbindgen.Operation[PlacementSummaryParentArgument, ParentAnswer]
	Available() bool
}

// BindPlacementSummaryParent admits the parent query under the address-only
// scalar plus three complete optional child columns contract.
func BindPlacementSummaryParent(
	operation signature.Signature,
	judgment PlacementSummaryParentJudgment,
	columns PlacementSummaryParentColumns,
	refusal model.RefusalID,
) (binding.Factory, bool) {
	if judgment == nil || !judgment.Available() || !columns.Available() || !placementSummaryParentShape(operation, columns) {
		return nil, false
	}
	return relbindgen.Bind(relbindgen.Spec[PlacementSummaryParentArgument, ParentAnswer]{
		Signature: operation,
		Decoder:   placementSummaryParentDecoder{columns: columns},
		Encoder:   placementSummaryParentEncoder{columns: columns},
		Operation: judgment,
		Refusal:   refusal,
	})
}

func placementSummaryParentShape(operation signature.Signature, columns PlacementSummaryParentColumns) bool {
	if !operation.Available() || !columns.Available() || operation.InputLen() != 4 || operation.OutputLen() != 1 {
		return false
	}

	querySite, ok := operation.InputAt(0)
	if !ok || !querySite.Delivery.IsScalar() || querySite.Presence != signature.RequireOpaque ||
		querySite.Column != columns.QuerySiteColumn || querySite.Type != columns.QuerySiteType {
		return false
	}

	childTypes := [...]model.TypeID{
		columns.AllocationID.Type(),
		columns.Fact.Type(),
		columns.Evidence.Type(),
	}
	var childDenominator model.DenominatorRef
	var childOrder model.KeyID
	for index, wantType := range childTypes {
		input, inputOK := operation.InputAt(index + 1)
		if !inputOK || !input.Delivery.IsComplete() || input.Presence != signature.AllowMissing || input.Type != wantType {
			return false
		}
		if index == 0 {
			childDenominator = input.Denominator
			childOrder = input.Delivery.OrderKey()
			continue
		}
		if !sameDenominator(input.Denominator, childDenominator) || input.Delivery.OrderKey() != childOrder {
			return false
		}
	}

	output := operation.Outputs()[0]
	if output.Type != columns.Output.Columns().PlacementSchemaID.Type() || output.Presence != signature.ProduceOpaque {
		return false
	}
	// The parent row is addressed by QuerySite's scalar row. The output must
	// therefore use the same query-site denominator, while the child spans use
	// their independent allocation-root denominator.
	if !sameDenominator(output.Denominator, querySite.Denominator) {
		return false
	}
	cardinality := operation.Cardinality()
	return cardinality.Available() && cardinality.Kind() == model.Optional
}

func sameDenominator(left, right model.DenominatorRef) bool {
	return left.Available() && right.Available() && left.Relation() == right.Relation() && left.Key() == right.Key()
}

type placementSummaryParentDecoder struct {
	columns PlacementSummaryParentColumns
}

func (decoder placementSummaryParentDecoder) Decode(inputs relbindgen.Inputs) (PlacementSummaryParentArgument, bool) {
	// QuerySite is an address-only scalar. Require its authenticated opaque
	// cell/address, but intentionally do not call ScalarAt or retain its
	// payload. The canonical Q owner contract is RequireOpaque: a payload is
	// not part of this parent ABI.
	presence, ok := inputs.PresenceAt(0)
	if !ok || !presence.Is(model.AuthenticatedOpaque) {
		return PlacementSummaryParentArgument{}, false
	}
	if _, ok := inputs.RowKeyAt(0); !ok {
		return PlacementSummaryParentArgument{}, false
	}
	allocationIDs, ok := relbindgen.SpanAt(inputs, 1, decoder.columns.AllocationID)
	if !ok {
		return PlacementSummaryParentArgument{}, false
	}
	facts, ok := relbindgen.SpanAt(inputs, 2, decoder.columns.Fact)
	if !ok {
		return PlacementSummaryParentArgument{}, false
	}
	evidence, ok := relbindgen.SpanAt(inputs, 3, decoder.columns.Evidence)
	if !ok {
		return PlacementSummaryParentArgument{}, false
	}
	return PlacementSummaryParentArgument{AllocationIDs: allocationIDs, Facts: facts, Evidence: evidence}, true
}

type placementSummaryParentEncoder struct {
	columns PlacementSummaryParentColumns
}

func (encoder placementSummaryParentEncoder) Encode(outputs relbindgen.Outputs, value ParentAnswer) bool {
	return encoder.columns.Output.Encode(outputs, value)
}
