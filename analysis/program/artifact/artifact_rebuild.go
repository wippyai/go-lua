package artifact

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// compiler is the private one-shot assembly state used by the artifact
// compiler. It is kept beside rebuild because both are construction-only
// ownership code; no separate runtime authority or public state object exists.
type compiler struct {
	input                      *program.Program
	key                        CompileKey
	counts                     denominator.CountRows
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
	localStages                map[identity.ContentID]identity.ContentID
	computationStages          map[identity.ContentID][]computationStage
	callStages                 map[identity.ContentID]callStageSet
	pointIDsBySite             map[identity.ContentID][]identity.ContentID
	pointDecisionAdds          map[identity.ContentID][]identity.ContentID
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
}

// rebuild injects the dense source/flow universes into the authored child
// Inputs. Child codecs intentionally decode only their owner relations; the
// four-way term census remains Source's canonical denominator.
func rebuild(
	sourceInput source.Input,
	flowInput flow.Input,
	staticInput static.Input,
	moduleInput imports.Input,
	entry keyspace.Term,
) (*program.Program, error) {
	counts := sourceCounts(sourceInput)
	if !ownerDenominatorsAgree(counts, flowInput, staticInput, moduleInput) {
		return nil, errors.New("artifact: child denominators disagree with Source")
	}
	flowInput.Counts = flowCounts(counts, flowInput)
	staticInput.Counts = staticCounts(counts, flowInput, staticInput)

	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		return nil, err
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		return nil, errors.Join(err, abortUnclaimedDrafts(sourceDraft, nil, nil))
	}
	moduleDraft, err := imports.Build(moduleInput)
	if err != nil {
		return nil, errors.Join(err, abortUnclaimedDrafts(sourceDraft, staticDraft, nil))
	}

	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		return nil, errors.Join(err, abortUnclaimedDrafts(sourceDraft, staticDraft, moduleDraft))
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		cleanup := errors.Join(sourceFinalizer.Abort(), abortUnclaimedDrafts(nil, staticDraft, moduleDraft))
		return nil, errors.Join(err, cleanup)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		cleanup := errors.Join(staticFinalizer.Abort(), sourceFinalizer.Abort(), abortUnclaimedDrafts(nil, nil, moduleDraft))
		return nil, errors.Join(err, cleanup)
	}
	// Flow is built only after every sibling Draft has either been claimed or
	// terminalized. This removes the otherwise-unabortable unclaimed Flow
	// window: a Flow Build failure can cleanly abort the three already-claimed
	// owner finalizers, and every later failure is inside Assemble's lifecycle.
	flowDraft, err := flow.Build(flowInput)
	if err != nil {
		cleanup := errors.Join(abortModuleFinalizer(moduleFinalizer), staticFinalizer.Abort(), sourceFinalizer.Abort())
		return nil, errors.Join(err, cleanup)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, entry)
	if err != nil {
		// Assemble normally aborts every claimed owner itself. These finalizers
		// are also aborted here for the pre-claim failure path: it is valid for
		// the flow draft to reject before it owns the three other drafts. Abort
		// is terminal and idempotent at the owner boundary, so this does not
		// reopen or otherwise alter Flow ownership.
		_ = moduleFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		return nil, err
	}
	return program.Publish(assembly)
}

func abortModuleFinalizer(finalizer imports.Finalizer) error {
	if finalizer.Abort() {
		return nil
	}
	return errors.New("program/module: finalizer abort failed")
}

// abortUnclaimedDrafts closes every owner Draft that has been built but not
// yet claimed by a Finalizer. Build is intentionally performed before any
// cross-owner assembly, so a later sibling failure must not leave an earlier
// construction capability open. The helper is used only on failure paths;
// successful drafts are claimed below or consumed by Assemble.
func abortUnclaimedDrafts(
	sourceDraft *source.Draft,
	staticDraft *static.Draft,
	moduleDraft *imports.Draft,
) error {
	var cleanup error
	if sourceDraft != nil {
		finalizer, err := sourceDraft.Finalizer()
		if err == nil {
			err = finalizer.Abort()
		}
		cleanup = errors.Join(cleanup, err)
	}
	if staticDraft != nil {
		finalizer, err := staticDraft.Finalizer()
		if err == nil {
			err = finalizer.Abort()
		}
		cleanup = errors.Join(cleanup, err)
	}
	if moduleDraft != nil {
		finalizer, err := moduleDraft.Finalizer()
		if err == nil && !finalizer.Abort() {
			err = errors.New("program/module: draft abort failed")
		}
		cleanup = errors.Join(cleanup, err)
	}
	return cleanup
}

// ownerDenominatorsAgree closes the one shared dense Term census before any
// child Build can reinterpret a foreign row. Sparse Static ClaimTarget rows
// are intentionally checked as a subset below; all other owner relations are
// dense by their canonical family.
func ownerDenominatorsAgree(
	counts [keyspace.FamilyCount]uint32,
	flowInput flow.Input,
	staticInput static.Input,
	moduleInput imports.Input,
) bool {
	length := func(family keyspace.Family, value int) bool {
		return value >= 0 && uint64(value) == uint64(counts[family])
	}
	if !length(keyspace.FamilyValues, len(flowInput.Values.Rows)) ||
		!length(keyspace.FamilyLensExact, len(flowInput.Access.Exact)) ||
		!length(keyspace.FamilyLensKey, len(flowInput.Access.Dynamic)) ||
		!length(keyspace.FamilyCell, len(flowInput.Storage.Cells)) ||
		!length(keyspace.FamilyRead, len(flowInput.Storage.Reads)) ||
		!length(keyspace.FamilyVararg, len(flowInput.Storage.Varargs)) ||
		!length(keyspace.FamilyBind, len(flowInput.Storage.Binds)) ||
		!length(keyspace.FamilyAssign, len(flowInput.Storage.Assigns)) ||
		!length(keyspace.FamilyWrite, len(flowInput.Storage.Writes)) ||
		!length(keyspace.FamilyTable, len(flowInput.Tables.Rows)) ||
		!length(keyspace.FamilyTableField, len(flowInput.Tables.Fields)) ||
		!length(keyspace.FamilyUnary, len(flowInput.Operators.Unaries)) ||
		!length(keyspace.FamilyBinary, len(flowInput.Operators.Binaries)) ||
		!length(keyspace.FamilySelect, len(flowInput.Operators.Selects)) ||
		!length(keyspace.FamilyFunction, len(flowInput.Functions.Rows)) ||
		!length(keyspace.FamilyCall, len(flowInput.Calls)) ||
		!length(keyspace.FamilyReturn, len(flowInput.Control.Returns)) ||
		!length(keyspace.FamilyBreak, len(flowInput.Control.Breaks)) ||
		!length(keyspace.FamilyLabel, len(flowInput.Control.Labels)) ||
		!length(keyspace.FamilyGoto, len(flowInput.Control.Gotos)) ||
		!length(keyspace.FamilyBranch, len(flowInput.Control.Branches)) ||
		!length(keyspace.FamilyLoop, len(flowInput.Control.Loops)) ||
		!length(keyspace.FamilyValueClaim, len(flowInput.Claims)) ||
		!length(keyspace.FamilyTypeValue, len(flowInput.TypeValues)) ||
		!length(keyspace.FamilyImport, len(moduleInput.Imports)) {
		return false
	}
	if uint64(len(staticInput.Operands.Claim)) > uint64(counts[keyspace.FamilyValueClaim]) {
		return false
	}
	staticLength := func(family keyspace.Family, value int) bool {
		return value >= 0 && uint64(value) == uint64(counts[family])
	}
	return staticLength(keyspace.FamilyTypePrimitive, len(staticInput.Types.Primitive)) &&
		staticLength(keyspace.FamilyTypeLiteral, len(staticInput.Types.Literal)) &&
		staticLength(keyspace.FamilyTypeOptional, len(staticInput.Types.Optional)) &&
		staticLength(keyspace.FamilyTypeUnion, len(staticInput.Types.Union)) &&
		staticLength(keyspace.FamilyTypeIntersection, len(staticInput.Types.Intersection)) &&
		staticLength(keyspace.FamilyTypeRef, len(staticInput.References.TypeRef)) &&
		staticLength(keyspace.FamilyTypeGeneric, len(staticInput.Types.Generic)) &&
		staticLength(keyspace.FamilyTypeArray, len(staticInput.Types.Array)) &&
		staticLength(keyspace.FamilyTypeMap, len(staticInput.Types.Map)) &&
		staticLength(keyspace.FamilyTypeRecord, len(staticInput.Types.Record)) &&
		staticLength(keyspace.FamilyTypeField, len(staticInput.Types.Field)) &&
		staticLength(keyspace.FamilyTypeAlias, len(staticInput.Declarations.Alias)) &&
		staticLength(keyspace.FamilyTypeParam, len(staticInput.Declarations.TypeParam)) &&
		staticLength(keyspace.FamilyTypeInterface, len(staticInput.Declarations.Interface)) &&
		staticLength(keyspace.FamilyDeclaredType, len(staticInput.Declarations.DeclaredType)) &&
		staticLength(keyspace.FamilyTypeFunction, len(staticInput.Signatures.TypeFunction)) &&
		staticLength(keyspace.FamilyTypeAsserts, len(staticInput.Signatures.TypeAsserts)) &&
		staticLength(keyspace.FamilyFunction, len(staticInput.Contracts.Function)) &&
		staticLength(keyspace.FamilyCall, len(staticInput.Contracts.Call)) &&
		staticLength(keyspace.FamilyTypePublication, len(staticInput.Publications.Type)) &&
		staticLength(keyspace.FamilyTypeValue, len(staticInput.Operands.TypeValue)) &&
		staticLength(keyspace.FamilyAnnotation, len(staticInput.Operands.Annotation)) &&
		staticLength(keyspace.FamilyTypeOf, len(staticInput.Operators.TypeOf)) &&
		staticLength(keyspace.FamilyTypeKeyOf, len(staticInput.Operators.KeyOf)) &&
		staticLength(keyspace.FamilyTypeIndexAccess, len(staticInput.Operators.IndexAccess)) &&
		staticLength(keyspace.FamilyTypeConditional, len(staticInput.Operators.Conditional))
}

func sourceCounts(input source.Input) [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	for _, row := range input.Families {
		if row.Family > keyspace.FamilyInvalid && row.Family < keyspace.FamilyCount {
			counts[row.Family] = uint32(len(row.Spans))
		}
	}
	return counts
}

func flowCounts(counts [keyspace.FamilyCount]uint32, input flow.Input) [keyspace.FamilyCount]uint32 {
	counts[keyspace.FamilyValues] = uint32(len(input.Values.Rows))
	counts[keyspace.FamilyLensExact] = uint32(len(input.Access.Exact))
	counts[keyspace.FamilyLensKey] = uint32(len(input.Access.Dynamic))
	counts[keyspace.FamilyCell] = uint32(len(input.Storage.Cells))
	counts[keyspace.FamilyRead] = uint32(len(input.Storage.Reads))
	counts[keyspace.FamilyVararg] = uint32(len(input.Storage.Varargs))
	counts[keyspace.FamilyBind] = uint32(len(input.Storage.Binds))
	counts[keyspace.FamilyAssign] = uint32(len(input.Storage.Assigns))
	counts[keyspace.FamilyWrite] = uint32(len(input.Storage.Writes))
	counts[keyspace.FamilyTable] = uint32(len(input.Tables.Rows))
	counts[keyspace.FamilyTableField] = uint32(len(input.Tables.Fields))
	counts[keyspace.FamilyUnary] = uint32(len(input.Operators.Unaries))
	counts[keyspace.FamilyBinary] = uint32(len(input.Operators.Binaries))
	counts[keyspace.FamilySelect] = uint32(len(input.Operators.Selects))
	counts[keyspace.FamilyFunction] = uint32(len(input.Functions.Rows))
	counts[keyspace.FamilyCall] = uint32(len(input.Calls))
	counts[keyspace.FamilyReturn] = uint32(len(input.Control.Returns))
	counts[keyspace.FamilyBreak] = uint32(len(input.Control.Breaks))
	counts[keyspace.FamilyLabel] = uint32(len(input.Control.Labels))
	counts[keyspace.FamilyGoto] = uint32(len(input.Control.Gotos))
	counts[keyspace.FamilyBranch] = uint32(len(input.Control.Branches))
	counts[keyspace.FamilyLoop] = uint32(len(input.Control.Loops))
	counts[keyspace.FamilyValueClaim] = uint32(len(input.Claims))
	counts[keyspace.FamilyTypeValue] = uint32(len(input.TypeValues))
	return counts
}

func staticCounts(counts [keyspace.FamilyCount]uint32, flowInput flow.Input, input static.Input) [keyspace.FamilyCount]uint32 {
	counts[keyspace.FamilyTypePrimitive] = uint32(len(input.Types.Primitive))
	counts[keyspace.FamilyTypeLiteral] = uint32(len(input.Types.Literal))
	counts[keyspace.FamilyTypeOptional] = uint32(len(input.Types.Optional))
	counts[keyspace.FamilyTypeUnion] = uint32(len(input.Types.Union))
	counts[keyspace.FamilyTypeIntersection] = uint32(len(input.Types.Intersection))
	counts[keyspace.FamilyTypeRef] = uint32(len(input.References.TypeRef))
	counts[keyspace.FamilyTypeGeneric] = uint32(len(input.Types.Generic))
	counts[keyspace.FamilyTypeArray] = uint32(len(input.Types.Array))
	counts[keyspace.FamilyTypeMap] = uint32(len(input.Types.Map))
	counts[keyspace.FamilyTypeRecord] = uint32(len(input.Types.Record))
	counts[keyspace.FamilyTypeField] = uint32(len(input.Types.Field))
	counts[keyspace.FamilyTypeAlias] = uint32(len(input.Declarations.Alias))
	counts[keyspace.FamilyTypeParam] = uint32(len(input.Declarations.TypeParam))
	counts[keyspace.FamilyTypeInterface] = uint32(len(input.Declarations.Interface))
	counts[keyspace.FamilyDeclaredType] = uint32(len(input.Declarations.DeclaredType))
	counts[keyspace.FamilyTypeFunction] = uint32(len(input.Signatures.TypeFunction))
	counts[keyspace.FamilyTypeAsserts] = uint32(len(input.Signatures.TypeAsserts))
	counts[keyspace.FamilyTypePublication] = uint32(len(input.Publications.Type))
	counts[keyspace.FamilyTypeValue] = uint32(len(input.Operands.TypeValue))
	counts[keyspace.FamilyAnnotation] = uint32(len(input.Operands.Annotation))
	counts[keyspace.FamilyTypeOf] = uint32(len(input.Operators.TypeOf))
	counts[keyspace.FamilyTypeKeyOf] = uint32(len(input.Operators.KeyOf))
	counts[keyspace.FamilyTypeIndexAccess] = uint32(len(input.Operators.IndexAccess))
	counts[keyspace.FamilyTypeConditional] = uint32(len(input.Operators.Conditional))
	// Static's dense Function/Call universe and sparse ValueClaim relation are
	// owned by Flow; preserve those exact rows from the already decoded input.
	counts[keyspace.FamilyFunction] = uint32(len(flowInput.Functions.Rows))
	counts[keyspace.FamilyCall] = uint32(len(flowInput.Calls))
	counts[keyspace.FamilyValueClaim] = uint32(len(flowInput.Claims))
	return counts
}
