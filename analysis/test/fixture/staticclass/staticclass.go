// Package staticclass seals one minimal real Static class authority for
// domain-law fixtures. It is test support, not a domain API or a composition
// assertion surface.
package staticclass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
	"github.com/wippyai/go-lua/program/testfixture"
)

// Capsule contains only real sealed semantic authorities and their public
// opaque class universe.
type Capsule struct {
	Program *program.Program
	Target  *target.Contract
	Link    *link.Link
	Types   *typeauthority.Authority
	Static  *static.Authority
	Classes *static.ClassSet
}

// Seal constructs the smallest real sealed authority usable by domain laws.
func Seal() (*Capsule, error) {
	p, err := testfixture.Minimal()
	return sealProgram(p, err)
}

// SealPositionalTransfer returns a real sealed capsule whose Program contains
// a nonzero PackTransfer adjustment. Domain laws consume the capsule only;
// this support package owns its semantic sealing pipeline.
func SealPositionalTransfer() (*Capsule, error) {
	p, err := testfixture.PositionalTransfer()
	return sealProgram(p, err)
}

// SealReceiverInputs returns a real receiver-call capsule with fixed Actual,
// Tail, and NilFill ApplicationInput occurrences. The Target boot ledger makes obj a
// structural global root; its op member is the one sealed operation.
func SealReceiverInputs() (*Capsule, error) {
	p, err := lower.Lower(lower.Source{
		Name: "receiver_inputs",
		Text: []byte("local function many(...) return ... end; obj:op(1, many()); obj:op(1)"),
	})
	if err != nil {
		return nil, fmt.Errorf("staticclass: program: %w", err)
	}
	binding := target.BindingSpec{Namespace: target.BindingModule, Owner: []string{"obj"}, Member: []string{"op"}}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{{
			Bindings: []target.BindingSpec{binding},
			Input: target.ValuesSpec{
				Fixed: []typ.Type{typ.Any, typ.Any, typ.Any, typ.Any},
				Tail:  target.ValuesClosed,
			},
			Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
			Effects:  target.RowSpec{Tail: target.RowClosed},
		}},
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{
			bootEntry("_G", target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}),
			bootEntry("__link_absent", target.InitialValueSpec{Kind: target.InitialValueAbsent}),
			bootEntry("obj", target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}),
		},
		InitialBindings: []target.InitialBindingSpec{
			bootBinding("_G"),
			bootBinding("__link_absent"),
			bootBinding("obj"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("staticclass: target: %w", err)
	}
	return sealWithContract(p, contract)
}

func bootEntry(key string, value target.InitialValueSpec) target.InitialEntrySpec {
	return target.InitialEntrySpec{
		Root:       "GlobalEnvRoot",
		Key:        keyspace.LiteralValue{Kind: keyspace.LiteralString, String: key},
		Value:      value,
		Mutability: target.InitialMutable,
	}
}

func bootBinding(name string) target.InitialBindingSpec {
	return target.InitialBindingSpec{
		Name: name,
		Root: "GlobalEnvRoot",
		Key:  keyspace.LiteralValue{Kind: keyspace.LiteralString, String: name},
	}
}

func sealProgram(p *program.Program, err error) (*Capsule, error) {
	if err != nil {
		return nil, fmt.Errorf("staticclass: program: %w", err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		return nil, fmt.Errorf("staticclass: target: %w", err)
	}
	return sealWithContract(p, contract)
}

func sealWithContract(p *program.Program, contract *target.Contract) (*Capsule, error) {
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "semantic_fixture", Program: p}}})
	if err != nil {
		return nil, fmt.Errorf("staticclass: link: %w", err)
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		return nil, fmt.Errorf("staticclass: type authority")
	}
	authority, _, err := static.Seal(source, types)
	if err != nil {
		return nil, fmt.Errorf("staticclass: authority: %w", err)
	}
	classes := authority.Classes()
	if classes == nil {
		return nil, fmt.Errorf("staticclass: classes")
	}
	return &Capsule{Program: p, Target: contract, Link: source, Types: types, Static: authority, Classes: classes}, nil
}
