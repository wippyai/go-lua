package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication is the whole cold catalog's payload: one emitted plane for each
// declared family. It exists so that publishing is total over the catalog --
// a family declared in this package has a field here, and a compilation that
// does not fill it publishes an empty plane rather than no column at all.
//
// A sealed publication is total over every family, which is what lets a
// consumer read an ordinal past the end of a plane as a proven absence. A
// declaration with no field here would seal nothing, so the catalog and the
// publication cannot drift apart.
type Publication struct {
	CallTargets                              []CallTarget
	HeapAllocations                          []HeapAllocation
	HeapFields                               []HeapField
	Values                                   []Values
	ValuesMembers                            []ValuesMember
	HeapIndexes                              []HeapIndex
	Occurrences                              []Occurrence
	OccurrencePoints                         []OccurrencePoint
	OccurrenceInputs                         []OccurrenceInput
	RuleOccurrences                          []RuleOccurrence
	ExactScalarSummaries                     []ExactScalarSummary
	ArithmeticSummaries                      []ArithmeticSummary
	UnarySummaries                           []UnarySummary
	Points                                   []Point
	PointDecisions                           []PointDecision
	Calls                                    []Call
	CallOperands                             []CallOperand
	CallArguments                            []CallArgument
	CallTypeArguments                        []CallTypeArgument
	EnvironmentEdges                         []EnvironmentEdge
	EnvironmentResets                        []EnvironmentReset
	StaticTypeValues                         []StaticTypeValue
	StaticExpressions                        []StaticExpression
	StaticInputs                             []StaticInput
	StaticTypeNodes                          []StaticTypeNode
	StaticTypeNodeUnionMembers               []StaticTypeNodeUnionMember
	StaticTypeNodeIntersectionMembers        []StaticTypeNodeIntersectionMember
	StaticTypeNodeGenericArguments           []StaticTypeNodeGenericArgument
	StaticTypeNodeAliasParameters            []StaticTypeNodeAliasParameter
	StaticTypeNodeInterfaceExtends           []StaticTypeNodeInterfaceExtend
	StaticTypeNodeInterfaceMembers           []StaticTypeNodeInterfaceMember
	StaticTypeNodeTypeFunctionTypeParameters []StaticTypeNodeTypeFunctionTypeParameter
	StaticTypeNodeTypeFunctionParameters     []StaticTypeNodeTypeFunctionParameter
	StaticTypeNodeTypeFunctionReturns        []StaticTypeNodeTypeFunctionReturn
	StaticTypeNodeRecordFields               []StaticTypeNodeRecordField
	StaticTypeNodeReferenceSourceKeys        []StaticTypeNodeReferenceSourceKey
	StaticTypeNodeReferenceCanonicalKeys     []StaticTypeNodeReferenceCanonicalKey
	Regions                                  []Region
	RegionMembers                            []RegionMember
	WTOEvents                                []WTOEvent
	Bodies                                   []Body
	BodyEntries                              []BodyEntry
	BodyRoots                                []BodyRoot
	Outcomes                                 []Outcome
	OutcomeReturnValues                      []OutcomeReturnValue
	OutcomePoints                            []OutcomePoint
	FunctionBoundaries                       []FunctionBoundary
	FunctionFormals                          []FunctionFormal
	FunctionVarargs                          []FunctionVararg
	FunctionCaptures                         []FunctionCapture
	LocalTransfers                           []LocalTransfer
	LocalTransferWrites                      []LocalTransferWrite
	DiagnosticObservations                   []DiagnosticObservation
	DiagnosticEvidence                       []DiagnosticEvidence
	DiagnosticPaths                          []DiagnosticPath
}

// Seal publishes every family of this catalog into one frozen store. The
// planes are sealed in slot order, and a plane holding an unavailable row
// seals nothing: a compiled program either proved every row it emitted or it
// did not compile.
func (publication Publication) Seal(catalog identity.ContentID, store identity.StoreID) (snapshot.Frozen, bool) {
	if !catalog.Available() || !store.Available() {
		return snapshot.Frozen{}, false
	}
	builder := snapshot.NewFrozen(catalog, store)
	sealed := CallTargetFamily().Put(&builder, publication.CallTargets, catalog) &&
		HeapAllocationFamily().Put(&builder, publication.HeapAllocations, catalog) &&
		HeapFieldFamily().Put(&builder, publication.HeapFields, catalog) &&
		ValuesFamily().Put(&builder, publication.Values, catalog) &&
		ValuesMemberFamily().Put(&builder, publication.ValuesMembers, catalog) &&
		HeapIndexFamily().Put(&builder, publication.HeapIndexes, catalog) &&
		OccurrenceFamily().Put(&builder, publication.Occurrences, catalog) &&
		OccurrencePointFamily().Put(&builder, publication.OccurrencePoints, catalog) &&
		OccurrenceInputFamily().Put(&builder, publication.OccurrenceInputs, catalog) &&
		RuleOccurrenceFamily().Put(&builder, publication.RuleOccurrences, catalog) &&
		ExactScalarSummaryFamily().Put(&builder, publication.ExactScalarSummaries, catalog) &&
		ArithmeticSummaryFamily().Put(&builder, publication.ArithmeticSummaries, catalog) &&
		UnarySummaryFamily().Put(&builder, publication.UnarySummaries, catalog) &&
		PointFamily().Put(&builder, publication.Points, catalog) &&
		PointDecisionFamily().Put(&builder, publication.PointDecisions, catalog) &&
		CallFamily().Put(&builder, publication.Calls, catalog) &&
		CallOperandFamily().Put(&builder, publication.CallOperands, catalog) &&
		CallArgumentFamily().Put(&builder, publication.CallArguments, catalog) &&
		CallTypeArgumentFamily().Put(&builder, publication.CallTypeArguments, catalog) &&
		EnvironmentEdgeFamily().Put(&builder, publication.EnvironmentEdges, catalog) &&
		EnvironmentResetFamily().Put(&builder, publication.EnvironmentResets, catalog) &&
		StaticTypeValueFamily().Put(&builder, publication.StaticTypeValues, catalog) &&
		StaticExpressionFamily().Put(&builder, publication.StaticExpressions, catalog) &&
		StaticInputFamily().Put(&builder, publication.StaticInputs, catalog) &&
		StaticTypeNodeFamily().Put(&builder, publication.StaticTypeNodes, catalog) &&
		StaticTypeNodeUnionMemberFamily().Put(&builder, publication.StaticTypeNodeUnionMembers, catalog) &&
		StaticTypeNodeIntersectionMemberFamily().Put(&builder, publication.StaticTypeNodeIntersectionMembers, catalog) &&
		StaticTypeNodeGenericArgumentFamily().Put(&builder, publication.StaticTypeNodeGenericArguments, catalog) &&
		StaticTypeNodeAliasParameterFamily().Put(&builder, publication.StaticTypeNodeAliasParameters, catalog) &&
		StaticTypeNodeInterfaceExtendFamily().Put(&builder, publication.StaticTypeNodeInterfaceExtends, catalog) &&
		StaticTypeNodeInterfaceMemberFamily().Put(&builder, publication.StaticTypeNodeInterfaceMembers, catalog) &&
		StaticTypeNodeTypeFunctionTypeParameterFamily().Put(&builder, publication.StaticTypeNodeTypeFunctionTypeParameters, catalog) &&
		StaticTypeNodeTypeFunctionParameterFamily().Put(&builder, publication.StaticTypeNodeTypeFunctionParameters, catalog) &&
		StaticTypeNodeTypeFunctionReturnFamily().Put(&builder, publication.StaticTypeNodeTypeFunctionReturns, catalog) &&
		StaticTypeNodeRecordFieldFamily().Put(&builder, publication.StaticTypeNodeRecordFields, catalog) &&
		StaticTypeNodeReferenceSourceKeyFamily().Put(&builder, publication.StaticTypeNodeReferenceSourceKeys, catalog) &&
		StaticTypeNodeReferenceCanonicalKeyFamily().Put(&builder, publication.StaticTypeNodeReferenceCanonicalKeys, catalog) &&
		RegionFamily().Put(&builder, publication.Regions, catalog) &&
		RegionMemberFamily().Put(&builder, publication.RegionMembers, catalog) &&
		WTOEventFamily().Put(&builder, publication.WTOEvents, catalog) &&
		BodyFamily().Put(&builder, publication.Bodies, catalog) &&
		BodyEntryFamily().Put(&builder, publication.BodyEntries, catalog) &&
		BodyRootFamily().Put(&builder, publication.BodyRoots, catalog) &&
		OutcomeFamily().Put(&builder, publication.Outcomes, catalog) &&
		OutcomeReturnValueFamily().Put(&builder, publication.OutcomeReturnValues, catalog) &&
		OutcomePointFamily().Put(&builder, publication.OutcomePoints, catalog) &&
		FunctionBoundaryFamily().Put(&builder, publication.FunctionBoundaries, catalog) &&
		FunctionFormalFamily().Put(&builder, publication.FunctionFormals, catalog) &&
		FunctionVarargFamily().Put(&builder, publication.FunctionVarargs, catalog) &&
		FunctionCaptureFamily().Put(&builder, publication.FunctionCaptures, catalog) &&
		LocalTransferFamily().Put(&builder, publication.LocalTransfers, catalog) &&
		LocalTransferWriteFamily().Put(&builder, publication.LocalTransferWrites, catalog) &&
		DiagnosticObservationFamily().Put(&builder, publication.DiagnosticObservations, catalog) &&
		DiagnosticEvidenceFamily().Put(&builder, publication.DiagnosticEvidence, catalog) &&
		DiagnosticPathFamily().Put(&builder, publication.DiagnosticPaths, catalog)
	if !sealed {
		return snapshot.Frozen{}, false
	}
	frozen, err := builder.Seal()
	if err != nil {
		return snapshot.Frozen{}, false
	}
	return frozen, true
}
