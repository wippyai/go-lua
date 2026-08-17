package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
)

type compiler struct {
	input                      *program.Program
	key                        CompileKey
	pointAttachments           []PointAttachmentRow
	points                     map[identity.ContentID]struct{}
	environment                []EnvironmentEdge
	localTransfers             []LocalTransfer
	regions                    []Region
	events                     []WTOEvent
	values                     []ValuesRow
	calls                      []CallRow
	callOperands               []CallOperandRow
	callArguments              []CallArgumentRow
	callTypeArguments          []CallTypeArgumentRow
	bodies                     []BodyRow
	functionBoundaries         []FunctionBoundaryRow
	callTargets                []CallTargetRow
	boundaries                 []BoundaryRow
	outcomes                   []OutcomeRow
	returnValues               []ReturnValue
	heapAllocations            []HeapAllocationRow
	heapIndexes                []HeapIndexRow
	allocationRows             []allocationCompileRow
	occurrences                []OccurrenceRow
	exactScalarSummaries       []ExactScalarSummaryRow
	exactScalarStates          map[identity.ContentID]exactScalarState
	arithmeticSummaries        []ArithmeticSummaryRow
	unarySummaries             []UnarySummaryRow
	ruleOccurrences            map[RuleRole][]RuleOccurrence
	diagnosticObservations     []DiagnosticObservationRow
	staticTypeArguments        []StaticTypeArgumentRow
	staticTypeValues           []StaticTypeValueRow
	staticTypeNodes            []StaticTypeNodeRow
	staticExpressions          []StaticExpressionRow
	staticInputs               []StaticInputRow
	diagnosticObservationByID  map[identity.ContentID]int
	pointGeometry              map[identity.ContentID]Point
	occurrenceSpans            map[occurrenceLookup]occurrenceSpanGeometry
	routeOccurrences           map[identity.ContentID]identity.ContentID
	localStages                map[identity.ContentID]identity.ContentID
	computationStages          map[identity.ContentID][]computationStage
	callStages                 map[identity.ContentID]callStageSet
	pointIDsBySite             map[identity.ContentID][]identity.ContentID
	pointDecisionAdds          map[identity.ContentID][]identity.ContentID
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
}
