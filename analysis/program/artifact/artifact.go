package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Artifact is immutable after Compile succeeds. Its fields are the sealed
// owner columns copied from Program; consumers access them through the
// owner-named row surfaces in this package.
type Artifact struct {
	key CompileKey
	id  identity.ContentID
	// frozen is this program's cold publication: the families that have moved
	// onto the shared publication substrate, sealed once here and shared by
	// reference with every Link that mounts this artifact. It is not a second
	// copy of anything -- a family published here is not also retained as a
	// slice above, because two authorities for one family is exactly what the
	// frozen publication exists to remove.
	frozen snapshot.Frozen
	// coldCatalog is the identity the frozen publication is sealed under. It
	// is derived from the declaration catalog this artifact was compiled
	// against, so a cold column cannot be addressed by an axis of another
	// catalog and cannot be addressed against a runtime snapshot at all.
	coldCatalog            identity.ContentID
	sealed                 identity.ContentID
	counts                 denominator.CountRows
	pointAttachments       []PointAttachmentRow
	points                 []Point
	environment            []EnvironmentEdge
	localTransfers         []LocalTransfer
	regions                []Region
	events                 []WTOEvent
	values                 []ValuesRow
	calls                  []CallRow
	callOperands           []CallOperandRow
	callArguments          []CallArgumentRow
	callTypeArguments      []CallTypeArgumentRow
	bodies                 []BodyRow
	functionBoundaries     []FunctionBoundaryRow
	boundaries             []BoundaryRow
	outcomes               []OutcomeRow
	returnValues           []ReturnValue
	occurrences            []OccurrenceRow
	exactScalarSummaries   []ExactScalarSummaryRow
	arithmeticSummaries    []ArithmeticSummaryRow
	unarySummaries         []UnarySummaryRow
	heapAllocations        []HeapAllocationRow
	heapIndexes            []HeapIndexRow
	occurrenceByID         map[occurrenceLookup]uint32
	ruleOccurrences        []RuleOccurrence
	diagnosticObservations []DiagnosticObservationRow
	staticTypeArguments    []StaticTypeArgumentRow
	staticTypeValues       []StaticTypeValueRow
	staticTypeNodes        []StaticTypeNodeRow
	staticExpressions      []StaticExpressionRow
	staticInputs           []StaticInputRow
	occurrenceByKind       map[OccurrenceKind][]uint32
	functionBoundaryByBody map[identity.ContentID]uint32
}

func (artifact *Artifact) Available() bool {
	return artifact != nil && artifact.key.Available() && artifact.id.Available() && artifact.counts.Available() && artifact.sealed == artifact.id
}

func (artifact *Artifact) CompileKey() CompileKey {
	if !artifact.Available() {
		return CompileKey{}
	}
	return artifact.key
}

func (artifact *Artifact) ID() identity.ContentID {
	if !artifact.Available() {
		return identity.ContentID{}
	}
	return artifact.id
}

// CountRows returns the immutable Program denominator rows frozen into this
// artifact. The rows are keyed by schema EntryID and contain no owner payload.
func (artifact *Artifact) CountRows() denominator.CountRows {
	if !artifact.Available() {
		return denominator.CountRows{}
	}
	return artifact.counts
}
