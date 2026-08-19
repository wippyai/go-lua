package boot

import (
	sealedrows "github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Compile freezes and validates one boot ledger. It returns a complete
// immutable owner value; no target-owned mutable table is appended later.
func Compile(input Input) (Table, error) {
	cloned := cloneInput(input)
	ledger, err := freezeLedger(cloned, cloned.Keys, cloned.Operations)
	if err != nil {
		return Table{}, err
	}
	table := Table{
		roots: sealedrows.NewRows(ledger.roots), shapes: sealedrows.NewRows(ledger.shapes), values: sealedrows.NewRows(ledger.values),
		valueBinds: sealedrows.NewRows(ledger.valueBinds), keys: cloned.Keys, bindingKeys: ledger.bindingKeys,
		entries: sealedrows.NewRows(ledger.entries), bindings: sealedrows.NewRows(ledger.bindings), metatables: sealedrows.NewRows(ledger.metatables),
		globalRoot: ledger.globalRoot,
		absent:     ledger.absent,
	}
	table.valueIDs, err = table.sealValueIdentities(cloned.Keys, cloned.Operations)
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

func checkedHandle(what string, index int) (uint32, error) {
	return vocabulary.CheckedStoredLength(what, index+1)
}
