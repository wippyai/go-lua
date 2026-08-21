package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	"github.com/wippyai/go-lua/analysis/schema/program/staticnode"
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
	// EntryBodyID is the scalar handoff from the compiler's sealed Flow Body
	// relation. It is transferred into Artifact/Program at publication; it is
	// not reconstructed from the ordered Bodies plane.
	EntryBodyID              identity.ContentID
	CallTargets              []calltarget.Target
	HeapAllocations          []heapallocation.Allocation
	HeapFields               []heapallocation.Field
	Values                   []programschema.Values
	ValuesMembers            []programschema.ValuesMember
	HeapIndexes              []heapindex.Index
	Occurrences              []programschema.Occurrence
	OccurrencePoints         []programschema.OccurrencePoint
	OccurrenceInputs         []programschema.OccurrenceInput
	RuleOccurrences          []programschema.RuleOccurrence
	ExactScalarSummaries     []programschema.ExactScalarSummary
	ArithmeticSummaries      []programschema.ArithmeticSummary
	UnarySummaries           []programschema.UnarySummary
	Points                   []programschema.Point
	PointDecisions           []programschema.PointDecision
	Calls                    []programschema.Call
	CallOperands             []programschema.CallOperand
	CallArguments            []programschema.CallArgument
	CallTypeArguments        []programschema.CallTypeArgument
	EnvironmentEdges         []programschema.EnvironmentEdge
	EnvironmentResets        []programschema.EnvironmentReset
	StaticTypeValues         []programschema.StaticTypeValue
	StaticExpressions        []programschema.StaticExpression
	StaticInputs             []programschema.StaticInput
	Diagnostic               programdiagnostic.Publication
	Static                   staticnode.Publication
	Lifecycle                lifecycle.Publication
	Regions                  []programschema.Region
	RegionMembers            []programschema.RegionMember
	WTOEvents                []programschema.WTOEvent
	Bodies                   []programschema.Body
	BodyEntries              []programschema.BodyEntry
	BodyRoots                []programschema.BodyRoot
	Outcomes                 []programschema.Outcome
	OutcomeReturnValues      []programschema.OutcomeReturnValue
	OutcomePoints            []programschema.OutcomePoint
	FunctionBoundaries       []programschema.FunctionBoundary
	FunctionFormals          []programschema.FunctionFormal
	FunctionVarargs          []programschema.FunctionVararg
	FunctionCaptures         []programschema.FunctionCapture
	LocalTransfers           []programschema.LocalTransfer
	LocalTransferWrites      []programschema.LocalTransferWrite
	CallResults              []programschema.CallResult
	ModuleImports            []programschema.ModuleImport
	ModuleRequests           []programschema.ModuleRequest
	ModuleEntries            []programschema.ModuleEntry
	ModuleEntryRootCells     []programschema.ModuleEntryRootCell
	ModuleEntryMembers       []programschema.ModuleEntryMember
	ModuleEntryRootFunctions []programschema.ModuleEntryRootFunction
	CallResultSlots          []programschema.CallResultSlot
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
	sealed := calltarget.Family().Put(&builder, publication.CallTargets, catalog) &&
		heapallocation.AllocationFamily().Put(&builder, publication.HeapAllocations, catalog) &&
		heapallocation.FieldFamily().Put(&builder, publication.HeapFields, catalog) &&
		programschema.ValuesFamily().Put(&builder, publication.Values, catalog) &&
		programschema.ValuesMemberFamily().Put(&builder, publication.ValuesMembers, catalog) &&
		heapindex.Family().Put(&builder, publication.HeapIndexes, catalog) &&
		programschema.ExactScalarSummaryFamily().Put(&builder, publication.ExactScalarSummaries, catalog) &&
		programschema.ArithmeticSummaryFamily().Put(&builder, publication.ArithmeticSummaries, catalog) &&
		programschema.UnarySummaryFamily().Put(&builder, publication.UnarySummaries, catalog) &&
		programschema.PointFamily().Put(&builder, publication.Points, catalog) &&
		programschema.PointDecisionFamily().Put(&builder, publication.PointDecisions, catalog) &&
		programschema.CallFamily().Put(&builder, publication.Calls, catalog) &&
		programschema.CallOperandFamily().Put(&builder, publication.CallOperands, catalog) &&
		programschema.CallArgumentFamily().Put(&builder, publication.CallArguments, catalog) &&
		programschema.CallTypeArgumentFamily().Put(&builder, publication.CallTypeArguments, catalog) &&
		programschema.EnvironmentEdgeFamily().Put(&builder, publication.EnvironmentEdges, catalog) &&
		programschema.EnvironmentResetFamily().Put(&builder, publication.EnvironmentResets, catalog) &&
		programschema.StaticTypeValueFamily().Put(&builder, publication.StaticTypeValues, catalog) &&
		programschema.StaticExpressionFamily().Put(&builder, publication.StaticExpressions, catalog) &&
		programschema.RegionFamily().Put(&builder, publication.Regions, catalog) &&
		programschema.RegionMemberFamily().Put(&builder, publication.RegionMembers, catalog) &&
		programschema.WTOEventFamily().Put(&builder, publication.WTOEvents, catalog) &&
		programschema.BodyFamily().Put(&builder, publication.Bodies, catalog) &&
		programschema.BodyEntryFamily().Put(&builder, publication.BodyEntries, catalog) &&
		programschema.BodyRootFamily().Put(&builder, publication.BodyRoots, catalog) &&
		programschema.OutcomeFamily().Put(&builder, publication.Outcomes, catalog) &&
		programschema.OutcomeReturnValueFamily().Put(&builder, publication.OutcomeReturnValues, catalog) &&
		programschema.OutcomePointFamily().Put(&builder, publication.OutcomePoints, catalog) &&
		programschema.FunctionBoundaryFamily().Put(&builder, publication.FunctionBoundaries, catalog) &&
		programschema.FunctionFormalFamily().Put(&builder, publication.FunctionFormals, catalog) &&
		programschema.FunctionVarargFamily().Put(&builder, publication.FunctionVarargs, catalog) &&
		programschema.FunctionCaptureFamily().Put(&builder, publication.FunctionCaptures, catalog) &&
		programschema.StaticInputFamily().Put(&builder, publication.StaticInputs, catalog) &&
		programschema.LocalTransferFamily().Put(&builder, publication.LocalTransfers, catalog) &&
		programschema.LocalTransferWriteFamily().Put(&builder, publication.LocalTransferWrites, catalog) &&
		programschema.OccurrenceFamily().Put(&builder, publication.Occurrences, catalog) &&
		programschema.OccurrencePointFamily().Put(&builder, publication.OccurrencePoints, catalog) &&
		programschema.OccurrenceInputFamily().Put(&builder, publication.OccurrenceInputs, catalog) &&
		programschema.RuleOccurrenceFamily().Put(&builder, publication.RuleOccurrences, catalog) &&
		publication.Diagnostic.Append(&builder, catalog) &&
		publication.Static.Append(&builder, catalog) &&
		programschema.CallResultFamily().Put(&builder, publication.CallResults, catalog) &&
		publication.Lifecycle.Append(&builder, catalog) &&
		programschema.ModuleImportFamily().Put(&builder, publication.ModuleImports, catalog) &&
		programschema.ModuleRequestFamily().Put(&builder, publication.ModuleRequests, catalog) &&
		programschema.ModuleEntryFamily().Put(&builder, publication.ModuleEntries, catalog) &&
		programschema.ModuleEntryRootCellFamily().Put(&builder, publication.ModuleEntryRootCells, catalog) &&
		programschema.ModuleEntryMemberFamily().Put(&builder, publication.ModuleEntryMembers, catalog) &&
		programschema.ModuleEntryRootFunctionFamily().Put(&builder, publication.ModuleEntryRootFunctions, catalog) &&
		programschema.CallResultSlotFamily().Put(&builder, publication.CallResultSlots, catalog)
	if !sealed {
		return snapshot.Frozen{}, false
	}
	frozen, err := builder.Seal()
	if err != nil {
		return snapshot.Frozen{}, false
	}
	return frozen, true
}
