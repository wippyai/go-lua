package target

import (
	"errors"
	"fmt"
	"sort"

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

	operationCount, err := checkedStoredTotal("operation table", len(spec.Operations), 1)
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
	if err := canonicalizeBindings(drafts); err != nil {
		return nil, err
	}
	if err := deriveProducedAnchors(drafts); err != nil {
		return nil, err
	}
	sort.Slice(drafts, func(left, right int) bool {
		return compareOperationDraft(drafts[left], drafts[right]) < 0
	})
	for index := 1; index < len(drafts); index++ {
		if compareOperationDraft(drafts[index-1], drafts[index]) == 0 {
			return nil, errors.New("target: duplicate operation anchor")
		}
	}

	sourceOperation := make([]Operation, len(drafts))
	for index := range drafts {
		handle, handleErr := checkedStoredHandle("operation handle", index)
		if handleErr != nil {
			return nil, handleErr
		}
		sourceOperation[drafts[index].source] = Operation(handle)
	}
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
	protocols, err := freezeProtocols(spec.Protocols)
	if err != nil {
		return nil, err
	}
	if err := resolveProtocols(protocols, drafts, sourceOperation); err != nil {
		return nil, err
	}
	boot, err := freezeBoot(spec.InitialRoots, spec.InitialEntries, spec.InitialBindings, spec.InitialMetatables, drafts, sourceOperation)
	if err != nil {
		return nil, err
	}
	if err := resolveSubedgeInitialReads(drafts, boot); err != nil {
		return nil, err
	}
	exactKeys, exactKeyHandles, err := freezeExactKeys(drafts, boot)
	if err != nil {
		return nil, err
	}

	// Contract is staging-only until the final return. Any failed append drops
	// it whole, so Seal never exposes a partially converted representation.
	contract := &Contract{operations: make([]operationRow, 0, operationCount)}
	if err := contract.appendExactKeys(exactKeys); err != nil {
		return nil, err
	}
	for index := range drafts {
		handle, handleErr := checkedStoredHandle("operation handle", index)
		if handleErr != nil {
			return nil, handleErr
		}
		op := Operation(handle)
		if err := contract.appendOperation(op, &drafts[index], exactKeyHandles); err != nil {
			return nil, err
		}
	}
	if err := resolveProtocolCallbackHolders(protocols, drafts); err != nil {
		return nil, err
	}
	if err := contract.appendCallbackReleases(drafts); err != nil {
		return nil, err
	}
	if err := contract.appendOpaque(); err != nil {
		return nil, err
	}
	if err := contract.appendProtocols(protocols); err != nil {
		return nil, err
	}
	if err := contract.appendBoot(boot, exactKeyHandles); err != nil {
		return nil, err
	}
	if err := contract.buildLookup(); err != nil {
		return nil, err
	}
	if err := contract.sealSemanticIdentities(); err != nil {
		return nil, err
	}
	// Content identity is available only after every immutable table and its
	// derived lookup authority has been completely assembled.
	contract.sealed = true
	sourceViews, viewsOK := buildTargetSourceViews(contract)
	if !viewsOK {
		return nil, errors.New("target: unavailable semantic-source rows")
	}
	contract.sourceViews = sourceViews
	return contract, nil
}
