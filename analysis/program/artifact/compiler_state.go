package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// compiler is the private one-shot assembly state used by the artifact
// compiler. It exists only during construction; the sealed Artifact is the
// sole retained result.
type compiler struct {
	input                      *program.Program
	key                        CompileKey
	counts                     denominator.CountRows
	environment                []EnvironmentEdge
	localTransfers             []LocalTransfer
	regions                    []Region
	events                     []WTOEvent
	values                     []ValuesRow
	calls                      []programschema.Call
	callOperands               []programschema.CallOperand
	callArguments              []programschema.CallArgument
	callTypeArguments          []programschema.CallTypeArgument
	callsByID                  map[identity.ContentID]programschema.Call
	bodies                     []programschema.Body
	bodyEntries                []programschema.BodyEntry
	bodyRoots                  []programschema.BodyRoot
	functionBoundaries         []programschema.FunctionBoundary
	functionFormals            []programschema.FunctionFormal
	functionVarargs            []programschema.FunctionVararg
	functionCaptures           []programschema.FunctionCapture
	callTargets                []programschema.CallTarget
	outcomes                   []programschema.Outcome
	outcomeReturnValues        []programschema.OutcomeReturnValue
	outcomePoints              []programschema.OutcomePoint
	heapAllocations            []HeapAllocationRow
	heapIndexes                []HeapIndexRow
	allocationRows             []allocationCompileRow
	occurrences                []OccurrenceRow
	exactScalarSummaries       []programschema.ExactScalarSummary
	exactScalarStates          map[identity.ContentID]exactScalarState
	arithmeticSummaries        []programschema.ArithmeticSummary
	unarySummaries             []programschema.UnarySummary
	ruleOccurrences            []RuleOccurrence
	issuance                   IssuanceDirectory
	diagnosticObservations     []DiagnosticObservationRow
	staticTypeValues           []StaticTypeValueRow
	staticTypeNodes            []StaticTypeNodeRow
	staticExpressions          []StaticExpressionRow
	staticInputs               []StaticInputRow
	diagnosticObservationByID  map[identity.ContentID]int
	pointGeometry              map[identity.ContentID]Point
	occurrenceSpans            map[occurrenceLookup]occurrenceSpanGeometry
	predecessorStages          map[identity.ContentID]identity.ContentID
	localStages                map[identity.ContentID]identity.ContentID
	computationStages          map[identity.ContentID][]computationStage
	callStages                 map[identity.ContentID]callStageSet
	pointIDsBySite             map[identity.ContentID][]identity.ContentID
	pointDecisionAdds          map[identity.ContentID][]identity.ContentID
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
}
