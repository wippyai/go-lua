package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// compiler is the private one-shot assembly state used by the artifact
// compiler. It exists only during construction; the sealed Artifact is the
// sole retained result.
type compiler struct {
	input                      *program.Program
	key                        CompileKey
	counts                     denominator.CountRows
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
	callsByID                  map[identity.ContentID]CallRow
	bodies                     []BodyRow
	functionBoundaries         []FunctionBoundaryRow
	callTargets                []cold.CallTarget
	boundaries                 []BoundaryRow
	outcomes                   []OutcomeRow
	returnValues               []ReturnValue
	heapAllocations            []HeapAllocationRow
	heapIndexes                []HeapIndexRow
	allocationRows             []allocationCompileRow
	occurrences                []OccurrenceRow
	exactScalarSummaries       []cold.ExactScalarSummary
	exactScalarStates          map[identity.ContentID]exactScalarState
	arithmeticSummaries        []cold.ArithmeticSummary
	unarySummaries             []cold.UnarySummary
	ruleOccurrences            []RuleOccurrence
	issuance                   IssuanceDirectory
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
	predecessorStages          map[identity.ContentID]identity.ContentID
	localStages                map[identity.ContentID]identity.ContentID
	computationStages          map[identity.ContentID][]computationStage
	callStages                 map[identity.ContentID]callStageSet
	pointIDsBySite             map[identity.ContentID][]identity.ContentID
	pointDecisionAdds          map[identity.ContentID][]identity.ContentID
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
}
