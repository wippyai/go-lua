package typestate

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// typestateProtocolSource is a minimal semantic fixture: acquire creates one
// protocol value and close consumes that same value through a formal input.
// It deliberately builds only the public Program, Target, and Link inputs;
// it does not reconstruct any former lifecycle-holder representation.
func typestateProtocolSource(t testing.TB) *link.Link {
	t.Helper()
	p, err := lower.Lower(lower.Source{
		Name: "typestate-protocol.lua",
		Text: []byte(`
local resource = acquire()
close(resource)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	closed := target.ValuesSpec{Tail: target.ValuesClosed}
	contract, err := target.Seal(&target.Spec{
		Operations: []target.OperationSpec{
			{
				Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"acquire"}}},
				Input:    closed,
				Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed}}},
				Effects:  target.RowSpec{Tail: target.RowClosed},
			},
			{
				Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"close"}}},
				Input:    target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed},
				Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed}},
				Effects:  target.RowSpec{Tail: target.RowClosed},
			},
		},
		Protocols: []target.ProtocolSpec{{
			Acquisitions: []target.AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
			States:       []target.StateSpec{{Name: "open"}},
			Transitions: []target.TransitionSpec{{
				Operation: 2,
				Input:     target.InputSource{Kind: target.InputSourceValueFormal},
				From:      1,
				Outcomes:  []target.TransitionOutcomeSpec{{Outcome: 0, To: 1}},
			}},
		}},
		InitialRoots: []target.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: target.BootShapeSpec{
				Aggregate: target.BootAggregateTable,
				Value:     target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
		InitialEntries: []target.InitialEntrySpec{
			{Root: "GlobalEnvRoot", Key: protocolString("_G"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: protocolString("__link_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: protocolString("acquire"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"acquire"}}}, Mutability: target.InitialMutable},
			{Root: "GlobalEnvRoot", Key: protocolString("close"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"close"}}}, Mutability: target.InitialMutable},
		},
		InitialBindings: []target.InitialBindingSpec{
			{Name: "_G", Root: "GlobalEnvRoot", Key: protocolString("_G")},
			{Name: "__link_absent", Root: "GlobalEnvRoot", Key: protocolString("__link_absent")},
			{Name: "acquire", Root: "GlobalEnvRoot", Key: protocolString("acquire")},
			{Name: "close", Root: "GlobalEnvRoot", Key: protocolString("close")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func protocolString(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}
