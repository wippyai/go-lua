package boot

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	sealedrows "github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Compile freezes and validates one boot ledger. It returns a complete
// immutable owner value; no target-owned mutable table is appended later.
func Compile(input Input) (Table, error) {
	cloned := cloneInput(input)
	operations, err := freezeOperations(cloned.Operations)
	if err != nil {
		return Table{}, err
	}
	ledger, err := freezeLedger(cloned, cloned.Keys, operations)
	if err != nil {
		return Table{}, err
	}
	table := Table{
		roots: sealedrows.NewRows(ledger.roots), shapes: sealedrows.NewRows(ledger.shapes), values: sealedrows.NewRows(ledger.values),
		valueBinds: sealedrows.NewRows(ledger.valueBinds), bindingKeys: ledger.bindingKeys, segments: ledger.segments,
		entries: sealedrows.NewRows(ledger.entries), bindings: sealedrows.NewRows(ledger.bindings), metatables: sealedrows.NewRows(ledger.metatables),
		globalRoot: ledger.globalRoot,
		absent:     ledger.absent,
	}
	table.valueIDs, err = table.sealValueIdentities(cloned.Keys)
	if err != nil {
		return Table{}, err
	}
	table.bootID, err = table.sealBootIdentity()
	if err != nil {
		return Table{}, err
	}
	return table, nil
}

func cloneInput(input Input) Input {
	out := Input{
		InitialRoots:      append([]vocabulary.InitialRootSpec(nil), input.InitialRoots...),
		InitialEntries:    append([]vocabulary.InitialEntrySpec(nil), input.InitialEntries...),
		InitialBindings:   append([]vocabulary.InitialBindingSpec(nil), input.InitialBindings...),
		InitialMetatables: append([]vocabulary.InitialMetatableAttachmentSpec(nil), input.InitialMetatables...),
		Keys:              input.Keys,
		Operations:        input.Operations,
	}
	return out
}

func cloneBinding(input vocabulary.BindingSpec) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{
		Namespace: input.Namespace,
		Owner:     append([]string(nil), input.Owner...),
		Member:    append([]string(nil), input.Member...),
	}
}

type frozenOperation struct {
	handle   vocabulary.Operation
	bindings []vocabulary.BindingSpec
	anchor   identity.ContentID
}

func checkedHandle(what string, index int) (uint32, error) {
	return vocabulary.CheckedStoredLength(what, index+1)
}

func freezeOperations(input operation.Core) ([]frozenOperation, error) {
	if _, err := vocabulary.CheckedStoredLength("operation input geometry", input.SourceCount()); err != nil {
		return nil, err
	}
	out := make([]frozenOperation, input.SourceCount())
	for index := range out {
		op, ok := input.SourceOperation(index)
		if !ok {
			return nil, errors.New("target/boot: missing source operation coordinate")
		}
		bindings := make([]vocabulary.BindingSpec, input.BindingCount(op))
		for bindingIndex := range bindings {
			binding, bindingOK := input.BindingAt(op, bindingIndex)
			if !bindingOK || !vocabulary.ValidBinding(binding) {
				return nil, errors.New("target/boot: invalid operation binding")
			}
			bindings[bindingIndex] = binding
		}
		anchor, anchorOK := input.Anchor(op)
		if !anchorOK {
			return nil, errors.New("target/boot: missing operation anchor")
		}
		out[index] = frozenOperation{handle: op, bindings: bindings, anchor: anchor}
	}
	return out, nil
}

func operationForBinding(operations []frozenOperation, binding vocabulary.BindingSpec) (vocabulary.Operation, bool) {
	for _, operation := range operations {
		for _, candidate := range operation.bindings {
			if compareBinding(candidate, binding) == 0 {
				return operation.handle, true
			}
		}
	}
	return 0, false
}
