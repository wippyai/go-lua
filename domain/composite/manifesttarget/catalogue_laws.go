// Package manifesttarget composes canonical provider manifests into the
// neutral Target ABI. Providers own declarations; this package owns the
// Lua-domain operational projection and boot policy.
package manifesttarget

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
)

// SealCatalogue is the sole manifest-to-analysis entry point. Providers own
// declarations; target only validates and freezes their analysis projection.
func SealCatalogue(declarations *manifest.Catalogue) (*contract.Contract, error) {
	spec, err := compileCatalogue(declarations)
	if err != nil {
		return nil, err
	}
	return compiler.Seal(&spec)
}

// compileCatalogue returns the one-shot authored form used by compiler.Seal. It is
// exposed for contract-law tests and tools that need to inspect the projection
// before it becomes immutable.
func compileCatalogue(declarations *manifest.Catalogue) (declaration.Spec, error) {
	catalogue, err := operations(declarations)
	if err != nil {
		return declaration.Spec{}, err
	}
	if err := catalogue.selfEffects(declarations); err != nil {
		return declaration.Spec{}, err
	}
	boot, err := bootLedger(catalogue, declarations)
	if err != nil {
		return declaration.Spec{}, err
	}
	return declaration.Spec{
		Semantics:         domaincontract.NewSemantics(),
		Operations:        catalogue.operations,
		InitialRoots:      boot.roots,
		InitialEntries:    boot.entries,
		InitialBindings:   boot.bindings,
		InitialMetatables: boot.metatables,
	}, nil
}

type operationRef uint32

// authoredCatalogue owns the one closed name-to-operation identity table.
// A zero operationRef is invalid, so an absent name can never accidentally
// become SpecRef(1), the first valid authored operation.
type authoredCatalogue struct {
	operations []vocabulary.OperationSpec
	names      map[string]operationRef
}

func (catalogue *authoredCatalogue) add(name string, operation vocabulary.OperationSpec) {
	if catalogue.names == nil {
		catalogue.names = make(map[string]operationRef)
	}
	ref := operationRef(len(catalogue.operations) + 1)
	catalogue.names[name] = ref
	catalogue.operations = append(catalogue.operations, operation)
}

func (catalogue *authoredCatalogue) lookup(name string) (operationRef, bool) {
	ref, ok := catalogue.names[name]
	if !ok || ref == 0 || int(ref) > len(catalogue.operations) {
		return 0, false
	}
	return ref, true
}

func (catalogue *authoredCatalogue) require(name string) (operationRef, error) {
	ref, ok := catalogue.lookup(name)
	if !ok {
		return 0, fmt.Errorf("target catalogue: unknown authored operation %q", name)
	}
	return ref, nil
}

func (catalogue *authoredCatalogue) at(ref operationRef) *vocabulary.OperationSpec {
	return &catalogue.operations[uint32(ref)-1]
}

func values(fixed []typ.Type, open bool, variable vocabulary.ValuesVar) vocabulary.ValuesSpec {
	tail := vocabulary.ValuesClosed
	var tailType schematype.Type
	if open {
		tail = vocabulary.ValuesVariable
		tailType = portable(typ.Any)
	}
	return vocabulary.ValuesSpec{Fixed: portableList(fixed), Tail: tail, Var: variable, TailType: tailType}
}

// portable is the only place this Lua catalogue crosses into Program's
// neutral authored ABI. All interpretation and validation stays in the Lua
// type-domain adapter; target receives only schema/typecontract declarations.
func portable(value typ.Type) schematype.Type {
	encoded, err := domaincontract.EncodeStorage(context.Background(), value, nil)
	if err != nil {
		panic(fmt.Sprintf("target catalogue: portable type: %v", err))
	}
	return encoded
}

func portableList(values []typ.Type) []schematype.Type {
	if len(values) == 0 {
		return nil
	}
	out := make([]schematype.Type, len(values))
	for index, value := range values {
		out[index] = portable(value)
	}
	return out
}

func (catalogue *authoredCatalogue) selfEffects(declarations *manifest.Catalogue) error {
	declaredEffects := make(map[string]bool)
	producedEffects := make(map[string]bool)
	for _, function := range declarations.Functions() {
		for _, binding := range bindingsFromDeclaration(function) {
			declaredEffects[bindingKey(binding)] = !function.Signature().Effect.Pure()
		}
		if law, ok := function.Operation(); ok && law.SelfEffect {
			producedEffects[function.CanonicalPath()] = true
		}
	}
	for index := range catalogue.operations {
		ref := operationRef(index + 1)
		op := catalogue.at(ref)
		declared := false
		for _, binding := range op.Bindings {
			if declaredEffects[bindingKey(binding)] {
				declared = true
				break
			}
		}
		if !declared {
			name := ""
			for candidate, candidateRef := range catalogue.names {
				if candidateRef == ref {
					name = candidate
					break
				}
			}
			if !producedEffects[name] {
				continue
			}
		}
		values := make([]vocabulary.ValueFormal, len(op.Input.Fixed))
		for i := range values {
			values[i] = vocabulary.ValueFormal(i)
		}
		vars := make([]vocabulary.ValuesVar, op.ValuesVars)
		for i := range vars {
			vars[i] = vocabulary.ValuesVar(i)
		}
		op.Effects = vocabulary.RowSpec{Occurrences: []vocabulary.EffectSpec{{Target: vocabulary.SpecRef(ref), ValueArgs: values, ValuesArgs: vars}}, Tail: vocabulary.RowClosed}
	}
	return nil
}

func bindingKey(binding vocabulary.BindingSpec) string {
	return fmt.Sprintf("%d/%q/%q", binding.Namespace, binding.Owner, binding.Member)
}
