package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// compiler is the private one-shot assembly state used by the artifact
// compiler. It exists only during construction; the sealed Artifact is the
// sole retained result.
type compiler struct {
	input                                    *program.Program
	key                                      programartifact.CompileKey
	counts                                   denominator.CountRows
	environment                              []environmentEdgeDraft
	localTransfers                           []localTransferDraft
	regions                                  []regionDraft
	events                                   []wtoEventDraft
	values                                   []programschema.Values
	valuesMembers                            []programschema.ValuesMember
	calls                                    []programschema.Call
	callOperands                             []programschema.CallOperand
	callArguments                            []programschema.CallArgument
	callTypeArguments                        []programschema.CallTypeArgument
	bodies                                   []programschema.Body
	bodyEntries                              []programschema.BodyEntry
	bodyRoots                                []programschema.BodyRoot
	functionBoundaries                       []programschema.FunctionBoundary
	functionFormals                          []programschema.FunctionFormal
	functionVarargs                          []programschema.FunctionVararg
	functionCaptures                         []programschema.FunctionCapture
	callTargets                              []programschema.CallTarget
	outcomes                                 []programschema.Outcome
	outcomeReturnValues                      []programschema.OutcomeReturnValue
	outcomePoints                            []programschema.OutcomePoint
	heapAllocations                          []heapAllocationDraft
	heapIndexes                              []heapIndexDraft
	allocationRows                           []allocationCompileRow
	allocationRowsByTerm                     map[keyspace.Term]int
	occurrences                              []programschema.Occurrence
	occurrencePoints                         []programschema.OccurrencePoint
	occurrenceInputs                         []programschema.OccurrenceInput
	exactScalarSummaries                     []programschema.ExactScalarSummary
	exactScalarStates                        map[identity.ContentID]exactScalarState
	arithmeticSummaries                      []programschema.ArithmeticSummary
	unarySummaries                           []programschema.UnarySummary
	ruleOccurrences                          []programschema.RuleOccurrence
	issuance                                 IssuanceDirectory
	diagnosticObservations                   []programschema.DiagnosticObservation
	diagnosticEvidence                       []programschema.DiagnosticEvidence
	diagnosticPaths                          []programschema.DiagnosticPath
	branchScopeRewriteComputed               bool
	branchScopeRewriteWellFormed             bool
	branchScopeRewriteOwners                 map[keyspace.Term]struct{}
	diagnosticEvidenceScratch                map[identity.ContentID]struct{}
	selectedDirectCalleeBodiesComputed       bool
	selectedDirectCalleeBodiesOK             bool
	selectedDirectCalleeBodiesValue          map[identity.ContentID]struct{}
	staticTypeValues                         []staticTypeValueDraft
	staticTypeNodes                          []programschema.StaticTypeNode
	staticTypeNodeUnionMembers               []programschema.StaticTypeNodeUnionMember
	staticTypeNodeIntersectionMembers        []programschema.StaticTypeNodeIntersectionMember
	staticTypeNodeGenericArguments           []programschema.StaticTypeNodeGenericArgument
	staticTypeNodeAliasParameters            []programschema.StaticTypeNodeAliasParameter
	staticTypeNodeInterfaceExtends           []programschema.StaticTypeNodeInterfaceExtend
	staticTypeNodeInterfaceMembers           []programschema.StaticTypeNodeInterfaceMember
	staticTypeNodeTypeFunctionTypeParameters []programschema.StaticTypeNodeTypeFunctionTypeParameter
	staticTypeNodeTypeFunctionParameters     []programschema.StaticTypeNodeTypeFunctionParameter
	staticTypeNodeTypeFunctionReturns        []programschema.StaticTypeNodeTypeFunctionReturn
	staticTypeNodeRecordFields               []programschema.StaticTypeNodeRecordField
	staticTypeNodeReferenceSourceKeys        []programschema.StaticTypeNodeReferenceSourceKey
	staticTypeNodeReferenceCanonicalKeys     []programschema.StaticTypeNodeReferenceCanonicalKey
	staticExpressions                        []programschema.StaticExpression
	staticInputs                             []programschema.StaticInput
	diagnosticObservationByID                map[identity.ContentID]int
	staticTypeTermsByID                      map[identity.ContentID]keyspace.Term
	pointGeometry                            map[identity.ContentID]pointDraft
	occurrenceSpans                          map[occurrenceLookup]occurrenceSpanGeometry
	predecessorStages                        map[identity.ContentID]identity.ContentID
	localStages                              map[identity.ContentID]identity.ContentID
	computationStages                        map[identity.ContentID][]computationStage
	callStages                               map[identity.ContentID]callStageSet
	pointIDsBySite                           map[identity.ContentID][]identity.ContentID
	pointDecisionAdds                        map[identity.ContentID][]identity.ContentID
	environmentByRoute                       map[identity.ContentID]environmentEdgeDraft
	environmentRouteDuplicates               map[identity.ContentID]struct{}
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }

func artifactFormat() uint64 { return programartifact.ArtifactFormatVersion }

func contentIDBefore(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

// valueRowForTerm resolves the canonical Values column by the authored Flow
// ordinal. The compiler retains no nested Values draft or member slice.
func (compiler *compiler) valueRowForTerm(term keyspace.Term) (programschema.Values, bool) {
	if compiler == nil || keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 {
		return programschema.Values{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	if index < 0 || index >= len(compiler.values) {
		return programschema.Values{}, false
	}
	row := compiler.values[index]
	return row, row.Available()
}

// valueMemberAt reads one member directly from the canonical dense member
// column named by a Values row's span.
func (compiler *compiler) valueMemberAt(row programschema.Values, index int) (programschema.ValuesMember, bool) {
	if compiler == nil || !row.Available() || index < 0 {
		return programschema.ValuesMember{}, false
	}
	offset, count, spanOK := row.MemberSpan()
	if !spanOK || index >= int(count) || uint64(offset)+uint64(index) >= uint64(len(compiler.valuesMembers)) {
		return programschema.ValuesMember{}, false
	}
	member := compiler.valuesMembers[int(offset)+index]
	return member, member.Available()
}
