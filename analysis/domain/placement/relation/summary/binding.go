package summary

import (
	heapsummary "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/suspension"
)

// PlacementSummaryAllocationColumns are the owner column codecs used by the
// terminal allocation-child derivation.
//
// The input columns are borrowed through complete sealed spans.  Metadata is
// allocation-only; HeapRoot is the full Heap-root view (including Boot);
// Placement and SuspensionEvidence are the corresponding allocation
// projections.  Heap's sealed schema remains the authority for the logical
// key used while traversing the root graph; a second HeapKey payload column is
// intentionally not part of this ABI.  Output is the one canonical
// AllocationRow codec.  This type owns no stores and issues no relation or
// denominator identity.
type PlacementSummaryAllocationColumns struct {
	AllocationID       *relbindgen.Column[identity.ContentID]
	AllocationSource   *relbindgen.Column[heapsummary.Source]
	HeapRoot           *relbindgen.Column[heapdomain.Value]
	Placement          *relbindgen.Column[placementdomain.Fact]
	SuspensionEvidence *relbindgen.Column[suspension.Evidence]
	Output             AllocationCodec
}

// NewPlacementSummaryAllocationColumns adopts the complete input columns and
// the already-declared child output columns.  In particular, it does not make
// a metadata+Fact result relation: metadata-to-base-evidence is a helper, and
// only the terminal operation may publish AllocationRow.
func NewPlacementSummaryAllocationColumns(
	allocationID *relbindgen.Column[identity.ContentID],
	allocationSource *relbindgen.Column[heapsummary.Source],
	heapRoot *relbindgen.Column[heapdomain.Value],
	placement *relbindgen.Column[placementdomain.Fact],
	suspensionEvidence *relbindgen.Column[suspension.Evidence],
	output AllocationColumns,
) (PlacementSummaryAllocationColumns, bool) {
	codec, ok := NewAllocationCodec(output)
	if !ok {
		return PlacementSummaryAllocationColumns{}, false
	}
	columns := PlacementSummaryAllocationColumns{
		AllocationID:       allocationID,
		AllocationSource:   allocationSource,
		HeapRoot:           heapRoot,
		Placement:          placement,
		SuspensionEvidence: suspensionEvidence,
		Output:             codec,
	}
	if !columns.Available() {
		return PlacementSummaryAllocationColumns{}, false
	}
	return columns, true
}

// Available reports whether all complete input codecs and both terminal
// output codecs are live.
func (columns PlacementSummaryAllocationColumns) Available() bool {
	return columns.AllocationID.Available() &&
		columns.AllocationSource.Available() &&
		columns.HeapRoot.Available() &&
		columns.Placement.Available() &&
		columns.SuspensionEvidence.Available() &&
		columns.Output.Available()
}

// PlacementSummaryAllocationArgument is one query-site invocation.  Every
// input is a complete borrowed span, so the Heap containment graph is built
// once per site rather than once per allocation root.  Span order is
// authenticated by each input's sealed denominator/order key.  The semantic
// operation obtains Heap coordinates from its exact sealed Heap schema while
// it consumes the full Heap-root value span; this binding does not create a
// parallel HeapKey column or re-derived coordinate source.
type PlacementSummaryAllocationArgument struct {
	AllocationIDs      relbindgen.Span[identity.ContentID]
	AllocationSources  relbindgen.Span[heapsummary.Source]
	HeapRoots          relbindgen.Span[heapdomain.Value]
	PlacementFacts     relbindgen.Span[placementdomain.Fact]
	SuspensionEvidence relbindgen.Span[suspension.Evidence]
}

// MetadataAt joins Heap's two physical metadata columns at one authenticated
// allocation ordinal.  Heap's AllocationCodec is the sole metadata writer;
// this accessor is only the typed read-side product and never stores a second
// AllocationRow relation.  The two columns must agree on presence, and the
// row ID is the existing allocation coordinate rather than a re-derived key.
func (argument PlacementSummaryAllocationArgument) MetadataAt(index int) (heapsummary.AllocationRow, bool, bool) {
	allocationID, idPresent, idOK := argument.AllocationIDs.At(index)
	allocationSource, sourcePresent, sourceOK := argument.AllocationSources.At(index)
	if !idOK || !sourceOK {
		return heapsummary.AllocationRow{}, false, false
	}
	if idPresent != sourcePresent {
		return heapsummary.AllocationRow{}, false, false
	}
	if !idPresent {
		return heapsummary.AllocationRow{}, false, true
	}
	row := heapsummary.AllocationRow{AllocationID: allocationID, Source: allocationSource}
	if !row.Valid() {
		return heapsummary.AllocationRow{}, false, false
	}
	return row, true, true
}

// PlacementSummaryAllocationJudgment is Placement's terminal child
// mathematics.  The operation owns evidence composition; this binding owns
// only typed span delivery and transactional publication.
type PlacementSummaryAllocationJudgment interface {
	relbindgen.Operation[PlacementSummaryAllocationArgument, AllocationRow]
	Available() bool
}

// BindPlacementSummaryAllocation admits the terminal child operation under a
// complete-span, keyed-bounded-many contract.  The explicit shape checks are
// intentional: a metadata+Fact helper or an independently published HeapKey
// column is not a legal replacement for this final ABI.
func BindPlacementSummaryAllocation(
	operation signature.Signature,
	judgment PlacementSummaryAllocationJudgment,
	columns PlacementSummaryAllocationColumns,
	refusal model.RefusalID,
) (binding.Factory, bool) {
	if judgment == nil || !judgment.Available() || !columns.Available() || !placementSummaryAllocationShape(operation, columns) {
		return nil, false
	}
	return relbindgen.Bind(relbindgen.Spec[PlacementSummaryAllocationArgument, AllocationRow]{
		Signature: operation,
		Decoder:   placementSummaryAllocationDecoder{columns: columns},
		Encoder:   placementSummaryAllocationEncoder{columns: columns},
		Operation: judgment,
		Refusal:   refusal,
	})
}

// placementSummaryAllocationShape is the one local ABI guard for the child
// shell. Relation IDs, keys, denominators, and operation identity remain the
// sealed signature's authority; this guard only prevents the typed binding
// from silently accepting a different number/delivery of slots or a
// non-optional output that cannot carry the dense child status. A legacy
// two-column child remains valid, while the parent-facing ABI explicitly
// carries AllocationID, Fact, and Evidence as three output slots.
func placementSummaryAllocationShape(operation signature.Signature, columns PlacementSummaryAllocationColumns) bool {
	wantOutputLen := 2
	if columns.Output.Columns().AllocationID.Available() {
		wantOutputLen = 3
	}
	if !operation.Available() || !columns.Available() || operation.InputLen() != 5 || operation.OutputLen() != wantOutputLen {
		return false
	}
	inputTypes := [...]model.TypeID{
		columns.AllocationID.Type(),
		columns.AllocationSource.Type(),
		columns.HeapRoot.Type(),
		columns.Placement.Type(),
		columns.SuspensionEvidence.Type(),
	}
	for index, wantType := range inputTypes {
		input, ok := operation.InputAt(index)
		if !ok || !input.Delivery.IsComplete() || input.Type != wantType {
			return false
		}
		// A complete vector can contain an owner-authenticated sparse cell. The
		// decoder must therefore receive absence as data, not have Frame.Validate
		// reject the entire query site before the owner fold can interpret it.
		if index < 2 {
			if input.Presence != signature.RequirePresent {
				return false
			}
		} else if input.Presence != signature.AllowMissing {
			return false
		}
	}
	outputs := operation.Outputs()
	wantTypes := make([]model.TypeID, 0, wantOutputLen)
	if columns.Output.Columns().AllocationID.Available() {
		wantTypes = append(wantTypes, columns.Output.Columns().AllocationID.Type())
	}
	wantTypes = append(wantTypes, columns.Output.Columns().Fact.Type(), columns.Output.Columns().Evidence.Type())
	for index, wantType := range wantTypes {
		if outputs[index].Type != wantType || outputs[index].Presence != signature.ProduceOptional {
			return false
		}
	}
	cardinality := operation.Cardinality()
	if !cardinality.Available() || cardinality.Kind() != model.BoundedMany {
		return false
	}
	bound, ok := cardinality.Bound()
	return ok && bound != 0
}

type placementSummaryAllocationDecoder struct {
	columns PlacementSummaryAllocationColumns
}

func (decoder placementSummaryAllocationDecoder) Decode(inputs relbindgen.Inputs) (PlacementSummaryAllocationArgument, bool) {
	allocationIDs, ok := relbindgen.SpanAt(inputs, 0, decoder.columns.AllocationID)
	if !ok {
		return PlacementSummaryAllocationArgument{}, false
	}
	allocationSources, ok := relbindgen.SpanAt(inputs, 1, decoder.columns.AllocationSource)
	if !ok {
		return PlacementSummaryAllocationArgument{}, false
	}
	heapRoots, ok := relbindgen.SpanAt(inputs, 2, decoder.columns.HeapRoot)
	if !ok {
		return PlacementSummaryAllocationArgument{}, false
	}
	placementFacts, ok := relbindgen.SpanAt(inputs, 3, decoder.columns.Placement)
	if !ok {
		return PlacementSummaryAllocationArgument{}, false
	}
	suspensionEvidence, ok := relbindgen.SpanAt(inputs, 4, decoder.columns.SuspensionEvidence)
	if !ok {
		return PlacementSummaryAllocationArgument{}, false
	}
	return PlacementSummaryAllocationArgument{
		AllocationIDs:      allocationIDs,
		AllocationSources:  allocationSources,
		HeapRoots:          heapRoots,
		PlacementFacts:     placementFacts,
		SuspensionEvidence: suspensionEvidence,
	}, true
}

type placementSummaryAllocationEncoder struct {
	columns PlacementSummaryAllocationColumns
}

func (encoder placementSummaryAllocationEncoder) Encode(outputs relbindgen.Outputs, value AllocationRow) bool {
	return encoder.columns.Output.Encode(outputs, value)
}
