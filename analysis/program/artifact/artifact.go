package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Artifact is immutable after Compile succeeds. Its fields are the sealed
// owner columns copied from Program; consumers access them through the
// owner-named row surfaces in this package.
type Artifact struct {
	key                    CompileKey
	id                     identity.ContentID
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
	callTargets            []CallTargetRow
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
	ruleOccurrences        map[RuleRole][]RuleOccurrence
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
