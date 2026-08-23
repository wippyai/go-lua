package compiler

import (
	"errors"
	"fmt"

	bootvalue "github.com/wippyai/go-lua/analysis/program/target/boot"
	contractvalue "github.com/wippyai/go-lua/analysis/program/target/contract"
	declarationvalue "github.com/wippyai/go-lua/analysis/program/target/declaration"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
	typeindexvalue "github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Seal validates, freezes, and canonically orders one target declaration.
// Declaration owns destructive Spec consumption; Compiler owns every draft
// and resolution phase, then hands only complete sealed subordinate values to
// Contract's atomic constructor.
func Seal(spec *declarationvalue.Spec) (*contractvalue.Contract, error) {
	input, err := spec.Consume()
	if err != nil {
		return nil, err
	}
	if err := schematype.ValidateSemantics(input.Semantics); err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	for index, declaration := range input.Types {
		if !declaration.Declaration.Available() {
			return nil, fmt.Errorf("target: qualified type %d (%q) has no declaration", index, declaration.Name)
		}
		if err := input.Semantics.Validate(declaration.Declaration, nil); err != nil {
			return nil, fmt.Errorf("target: qualified type %d (%q): %w", index, declaration.Name, err)
		}
	}
	types, err := typeindexvalue.Compile(input.Types)
	if err != nil {
		return nil, err
	}

	_, err = vocabulary.CheckedStoredTotal("operation table", len(input.Operations), 1)
	if err != nil {
		return nil, err
	}
	drafts := make([]operationDraft, len(input.Operations))
	for index := range input.Operations {
		draft, err := freezeOperation(index, input.Operations[index], input.Semantics)
		if err != nil {
			return nil, err
		}
		drafts[index] = draft
	}
	geometry, err := operationvalue.CompileGeometry(operationGeometryInput(drafts))
	if err != nil {
		return nil, err
	}
	sourceOperation := make([]vocabulary.Operation, len(drafts))
	for source := range sourceOperation {
		handle, handleOK := geometry.SourceOperation(source)
		if !handleOK {
			return nil, errors.New("target: operation source map is incomplete")
		}
		sourceOperation[source] = handle
	}
	orderedDrafts := make([]operationDraft, len(drafts))
	for source, handle := range sourceOperation {
		if handle == 0 || uint64(handle) > uint64(len(orderedDrafts)) {
			return nil, errors.New("target: operation core returned invalid canonical handle")
		}
		orderedDrafts[uint32(handle)-1] = drafts[source]
	}
	drafts = orderedDrafts
	for index := range drafts {
		if err := drafts[index].resolveEffects(drafts, sourceOperation); err != nil {
			return nil, err
		}
		if err := drafts[index].resolveCallbackReleases(drafts, sourceOperation); err != nil {
			return nil, err
		}
		if err := drafts[index].resolveProduced(sourceOperation); err != nil {
			return nil, err
		}
		if err := drafts[index].resolveSuspensions(); err != nil {
			return nil, err
		}
	}
	if err := validateProducedResumes(drafts, sourceOperation); err != nil {
		return nil, err
	}
	if err := validateSpawnAuthority(drafts); err != nil {
		return nil, err
	}
	exactKeys, err := freezeExactKeys(drafts, input.InitialRoots, input.InitialEntries, input.InitialBindings)
	if err != nil {
		return nil, err
	}
	operationCore, err := operationvalue.CompileAnchors(geometry, exactKeys)
	if err != nil {
		return nil, err
	}
	protocols, err := protocolvalue.Compile(protocolvalue.Input{
		Protocols: input.Protocols, Operations: operationCore,
	})
	if err != nil {
		return nil, err
	}
	bootTable, err := bootvalue.Compile(bootvalue.Input{
		InitialRoots: input.InitialRoots, InitialEntries: input.InitialEntries,
		InitialBindings: input.InitialBindings, InitialMetatables: input.InitialMetatables,
		Operations: operationCore, Keys: exactKeys,
	})
	if err != nil {
		return nil, err
	}
	if err := resolveSubedgeInitialReads(drafts, &bootTable, exactKeys); err != nil {
		return nil, err
	}

	queryBuilder, err := operationvalue.BeginQuery(operationCore)
	if err != nil {
		return nil, err
	}
	for index := range drafts {
		op, ok := operationCore.OperationAt(index)
		if !ok {
			return nil, errors.New("target: operation core missing canonical handle")
		}
		callbackIDs := make([]vocabulary.CallbackID, len(drafts[index].callbacks))
		for callbackIndex := range callbackIDs {
			callback, callbackOK := operationCore.CallbackAt(op, callbackIndex)
			if !callbackOK {
				return nil, errors.New("target: operation core missing callback handle")
			}
			callbackIDs[callbackIndex] = callback
		}
		if err := appendOperation(queryBuilder, operationCore, op, &drafts[index], exactKeys, callbackIDs); err != nil {
			return nil, err
		}
	}
	releases, releaseErr := callbackReleaseInputs(drafts)
	if releaseErr != nil {
		return nil, releaseErr
	}
	if err := queryBuilder.AppendCallbackReleases(releases); err != nil {
		return nil, err
	}
	opaque, opaqueOK := operationCore.Opaque()
	if !opaqueOK {
		return nil, errors.New("target: operation core missing opaque handle")
	}
	if err := appendOpaque(queryBuilder, operationCore, opaque); err != nil {
		return nil, err
	}
	queryCore, err := queryBuilder.FinishQuery()
	if err != nil {
		return nil, err
	}
	// The semantic column is derived here, over the finished operation core
	// and before the Contract pointer exists, so the contract identity frames
	// a column that was sealed exactly once from exactly this read surface.
	column, err := contractvalue.SealColumn(input.Semantics, queryCore)
	if err != nil {
		return nil, err
	}
	return contractvalue.New(contractvalue.Input{
		Table: bootTable, Operations: queryCore, Protocols: protocols, ExactKeys: exactKeys,
		Types:  types,
		Column: column,
	})
}

func operationGeometryInput(drafts []operationDraft) operationvalue.Input {
	input := operationvalue.Input{Operations: make([]operationvalue.OperationInput, len(drafts))}
	for index := range drafts {
		draft := drafts[index]
		outcomes := make([]operationvalue.OutcomeInput, len(draft.outcomes))
		for outcomeIndex := range draft.outcomes {
			outcomes[outcomeIndex] = operationvalue.OutcomeInput{
				ValueSlots: uint32(len(draft.outcomes[outcomeIndex].values.types)),
				Anchor:     []byte(draft.outcomes[outcomeIndex].anchor),
			}
		}
		callbacks := make([]operationvalue.CallbackInput, len(draft.callbacks))
		for callbackIndex := range draft.callbacks {
			callbacks[callbackIndex] = operationvalue.CallbackInput{
				Source:    draft.callbacks[callbackIndex].source,
				Function:  draft.callbacks[callbackIndex].function,
				Lifecycle: draft.callbacks[callbackIndex].lifecycle,
			}
		}
		produced := make([]operationvalue.ProducedInput, len(draft.outcomes))
		produced = produced[:0]
		for outcomeIndex := range draft.outcomes {
			for _, edge := range draft.outcomes[outcomeIndex].produced {
				produced = append(produced, operationvalue.ProducedInput{
					TargetSource: int(edge.targetSource) - 1,
					Outcome:      uint32(outcomeIndex),
					Result:       edge.result,
				})
			}
		}
		input.Operations[index] = operationvalue.OperationInput{
			Source: draft.source, Bindings: draft.bindings,
			InputFormalCount: len(draft.input.types), TypeFormalCount: len(draft.formals), RowFormalCount: int(draft.rowFormals), ValuesVars: draft.valuesVars,
			OutcomeValueSlots: outcomes, Callbacks: callbacks, Produced: produced,
		}
	}
	return input
}
