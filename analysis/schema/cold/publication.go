package cold

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
	CallTargets          []CallTarget
	HeapAllocations      []HeapAllocation
	HeapFields           []HeapField
	Values               []Values
	ValuesMembers        []ValuesMember
	HeapIndexes          []HeapIndex
	ExactScalarSummaries []ExactScalarSummary
	ArithmeticSummaries  []ArithmeticSummary
	UnarySummaries       []UnarySummary
	Points               []Point
	PointDecisions       []PointDecision
	Calls                []Call
	CallOperands         []CallOperand
	CallArguments        []CallArgument
	CallTypeArguments    []CallTypeArgument
	EnvironmentEdges     []EnvironmentEdge
	EnvironmentResets    []EnvironmentReset
	StaticTypeValues     []StaticTypeValue
	StaticExpressions    []StaticExpression
	Regions              []Region
	RegionMembers        []RegionMember
	WTOEvents            []WTOEvent
	Bodies               []Body
	BodyEntries          []BodyEntry
	BodyRoots            []BodyRoot
	Outcomes             []Outcome
	OutcomeReturnValues  []OutcomeReturnValue
	OutcomePoints        []OutcomePoint
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
		RegionFamily().Put(&builder, publication.Regions, catalog) &&
		RegionMemberFamily().Put(&builder, publication.RegionMembers, catalog) &&
		WTOEventFamily().Put(&builder, publication.WTOEvents, catalog) &&
		BodyFamily().Put(&builder, publication.Bodies, catalog) &&
		BodyEntryFamily().Put(&builder, publication.BodyEntries, catalog) &&
		BodyRootFamily().Put(&builder, publication.BodyRoots, catalog) &&
		OutcomeFamily().Put(&builder, publication.Outcomes, catalog) &&
		OutcomeReturnValueFamily().Put(&builder, publication.OutcomeReturnValues, catalog) &&
		OutcomePointFamily().Put(&builder, publication.OutcomePoints, catalog)
	if !sealed {
		return snapshot.Frozen{}, false
	}
	frozen, err := builder.Seal()
	if err != nil {
		return snapshot.Frozen{}, false
	}
	return frozen, true
}
