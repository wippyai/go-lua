package target

import (
	"errors"
	"fmt"
	bootvalue "github.com/wippyai/go-lua/analysis/program/target/boot"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Seal validates, freezes, and canonically orders one target contract. Spec
// is consumed on its first attempt, including a failing attempt.
func Seal(spec *Spec) (*Contract, error) {
	if spec == nil {
		return nil, errors.New("target: nil spec")
	}
	if spec.consumed {
		return nil, errors.New("target: consumed spec")
	}
	defer func() { *spec = Spec{consumed: true} }()
	if err := schematype.ValidateSemantics(spec.Semantics); err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}

	operationCount, err := vocabulary.CheckedStoredTotal("operation table", len(spec.Operations), 1)
	if err != nil {
		return nil, err
	}
	drafts := make([]operationDraft, len(spec.Operations))
	for index := range spec.Operations {
		draft, err := freezeOperation(index, spec.Operations[index], spec.Semantics)
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
	exactKeys, err := freezeExactKeys(drafts, spec.InitialRoots, spec.InitialEntries, spec.InitialBindings)
	if err != nil {
		return nil, err
	}
	operationCore, err := operationvalue.CompileAnchors(geometry, exactKeys)
	if err != nil {
		return nil, err
	}
	protocols, err := protocolvalue.Compile(protocolvalue.Input{
		Protocols: spec.Protocols, Operations: operationCore,
	})
	if err != nil {
		return nil, err
	}
	bootTable, err := bootvalue.Compile(bootvalue.Input{
		InitialRoots: spec.InitialRoots, InitialEntries: spec.InitialEntries,
		InitialBindings: spec.InitialBindings, InitialMetatables: spec.InitialMetatables,
		Operations: operationCore, Keys: exactKeys,
	})
	if err != nil {
		return nil, err
	}
	if err := resolveSubedgeInitialReads(drafts, &bootTable, exactKeys); err != nil {
		return nil, err
	}

	// Contract is staging-only until the final return. Any failed append drops
	// it whole, so Seal never exposes a partially converted representation.
	contract := &Contract{Table: bootTable, exactKeys: exactKeys, operationCore: operationCore,
		operations: make([]operationRow, 0, operationCount), protocols: protocols}
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
		if err := contract.appendOperation(op, &drafts[index], exactKeys, callbackIDs); err != nil {
			return nil, err
		}
	}
	if err := contract.appendCallbackReleases(drafts); err != nil {
		return nil, err
	}
	opaque, opaqueOK := operationCore.Opaque()
	if !opaqueOK {
		return nil, errors.New("target: operation core missing opaque handle")
	}
	if err := contract.appendOpaque(opaque); err != nil {
		return nil, err
	}
	if err := contract.sealSemanticIdentities(); err != nil {
		return nil, err
	}
	// Content identity is available only after every immutable table and its
	// derived lookup authority has been completely assembled.
	contract.sealed = true
	counts, countsErr := contract.buildCountRows()
	if countsErr != nil {
		return nil, countsErr
	}
	contract.counts = counts
	return contract, nil
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
			InputFormalCount: len(draft.input.types), ValuesVars: draft.valuesVars,
			OutcomeValueSlots: outcomes, Callbacks: callbacks, Produced: produced,
		}
	}
	return input
}
