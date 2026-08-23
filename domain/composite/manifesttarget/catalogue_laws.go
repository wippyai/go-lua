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
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

// PreviewAmendment is one composition-owned addition to a reference operation
// the composition mounts but does not author.
//
// A module path belongs to exactly one declaring provider, so a composition
// that needs a consequence its reference manifest does not state cannot answer
// by declaring the path twice. It states the addition here instead, against
// the reference operation's canonical path. Effect carries signature-level
// ownership labels; Law carries the operational envelope. Naming an operation
// the catalogue does not hold is a composition error, not a silent no-op.
type PreviewAmendment struct {
	Operation string
	Effect    []effect.Label
	Law       moduleio.Operation
	HasLaw    bool
}

// SealCatalogue is the sole manifest-to-analysis entry point. Providers own
// declarations; target only validates and freezes their analysis projection.
// Preview amendments are applied to the reference declarations they name
// before any projection is derived from them.
func SealCatalogue(declarations *manifest.Catalogue, amendments ...PreviewAmendment) (*contract.Contract, error) {
	spec, err := compileCatalogue(declarations, amendments...)
	if err != nil {
		return nil, err
	}
	return compiler.Seal(&spec)
}

// amendedFunctions applies every preview amendment onto the reference
// declaration it names and returns the complete function set the projection
// is derived from.
func amendedFunctions(declarations *manifest.Catalogue, amendments []PreviewAmendment) ([]manifest.Function, error) {
	functions := declarations.Functions()
	if len(amendments) == 0 {
		return functions, nil
	}
	byPath := make(map[string]int, len(functions))
	for index, function := range functions {
		byPath[function.CanonicalPath()] = index
	}
	for _, amendment := range amendments {
		index, ok := byPath[amendment.Operation]
		if !ok {
			return nil, fmt.Errorf("target catalogue: preview amendment names unknown operation %q", amendment.Operation)
		}
		amended, err := functions[index].Amend(amendment.Effect, amendment.Law, amendment.HasLaw)
		if err != nil {
			return nil, fmt.Errorf("target catalogue: preview amendment for %q: %w", amendment.Operation, err)
		}
		functions[index] = amended
	}
	return functions, nil
}

// compileCatalogue returns the one-shot authored form used by compiler.Seal. It is
// exposed for contract-law tests and tools that need to inspect the projection
// before it becomes immutable.
func compileCatalogue(declarations *manifest.Catalogue, amendments ...PreviewAmendment) (declaration.Spec, error) {
	if declarations == nil {
		return declaration.Spec{}, fmt.Errorf("target: nil declaration catalogue")
	}
	functions, err := amendedFunctions(declarations, amendments)
	if err != nil {
		return declaration.Spec{}, err
	}
	if err := callbackConformance(functions); err != nil {
		return declaration.Spec{}, err
	}
	catalogue, err := operations(functions)
	if err != nil {
		return declaration.Spec{}, err
	}
	if err := outcomeSuffixCarriage(&catalogue); err != nil {
		return declaration.Spec{}, err
	}
	if err := catalogue.selfEffects(declarations); err != nil {
		return declaration.Spec{}, err
	}
	boot, err := bootLedger(catalogue, declarations)
	if err != nil {
		return declaration.Spec{}, err
	}
	protocolSpecs, err := protocols(&catalogue, functions, declarations)
	if err != nil {
		return declaration.Spec{}, err
	}
	namedTypes, err := qualifiedTypes(declarations)
	if err != nil {
		return declaration.Spec{}, err
	}
	return declaration.Spec{
		Semantics:         domaincontract.NewSemantics(),
		Types:             namedTypes,
		Operations:        catalogue.operations,
		Protocols:         protocolSpecs,
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
	// lifecycles holds the protocol relations read out of each declaration's
	// signature effect row, keyed by the same canonical path as names. The
	// protocol pass is their only consumer: it owns the state machines they
	// name and the operation geometry they resolve against.
	lifecycles map[string][]lifecycleDeclaration
}

func (catalogue *authoredCatalogue) add(name string, operation vocabulary.OperationSpec, lifecycles []lifecycleDeclaration) {
	if catalogue.names == nil {
		catalogue.names = make(map[string]operationRef)
	}
	ref := operationRef(len(catalogue.operations) + 1)
	catalogue.names[name] = ref
	catalogue.operations = append(catalogue.operations, operation)
	if len(lifecycles) == 0 {
		return
	}
	if catalogue.lifecycles == nil {
		catalogue.lifecycles = make(map[string][]lifecycleDeclaration)
	}
	catalogue.lifecycles[name] = lifecycles
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
			// Ownership labels are declaration-level formal metadata. They do
			// not denote an invocation effect occurrence; only a non-ownership
			// label or an open signature row makes the operation effectful here.
			declaredEffects[bindingKey(binding)] = hasInvocationEffects(function.Signature().Effect)
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
		// An explicit invocation row is the provider's complete occurrence
		// authority. Do not append a generic self row merely because the
		// signature also carries an open/unknown effect label or SelfEffect.
		if len(op.Effects.Occurrences) != 0 {
			continue
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

// hasInvocationEffects keeps the signature-to-operation distinction explicit:
// ownership labels are formal contracts and are lowered to FormalEffects, not
// to an operation's invocation Effects row. Other known labels retain the
// existing self-effect projection, while any open/unknown tail remains an
// invocation effect because its contents cannot be proven empty.
func hasInvocationEffects(row effect.Row) bool {
	if row.Tail != nil {
		return true
	}
	for _, label := range row.Labels {
		normalized := effect.NormalizeLabel(label)
		switch normalized.(type) {
		case ownership.Borrow, ownership.Retain, ownership.Store, ownership.BorrowAll,
			ownership.Send, ownership.SendParam, ownership.Export, ownership.Opaque, ownership.Freeze:
			continue
		default:
			if normalized != nil {
				return true
			}
		}
	}
	return false
}

func bindingKey(binding vocabulary.BindingSpec) string {
	return fmt.Sprintf("%d/%q/%q", binding.Namespace, binding.Owner, binding.Member)
}
