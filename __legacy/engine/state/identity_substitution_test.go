package state

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestIdentitySubstitutionIsSimultaneousAndAllocationOwned(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("identity-substitution-authority"))
	callee := lexicalidentity.FunctionBody(namespace, 1)
	caller := lexicalidentity.RootBody(namespace)
	first := identity.NewFormalVar(identity.NewFormalSchemaID(callee, 1), formal.Input)
	second := identity.NewFormalVar(identity.NewFormalSchemaID(callee, 2), formal.Middle)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)

	formalRename, ok := identity.NewSubstitution([]identity.Binding{
		{Variable: first, Image: identity.SingletonTerm(identity.FormalTerm(second))},
		{Variable: second, Image: identity.SingletonTerm(identity.AllocationTerm(template))},
	})
	if !ok {
		t.Fatal("substitution")
	}
	symbolic := NewIdentitySubstitutionAuthority(formalRename, nil)
	got, err := symbolic.Image(identity.FormalTerm(first))
	if err != nil {
		t.Fatal(err)
	}
	gotTerm, exact := got.Term()
	gotFormal, formal := gotTerm.Formal()
	if !exact || !formal || gotFormal != second {
		t.Fatalf("simultaneous Formal->Formal image = %v", got)
	}
	got, err = symbolic.Image(identity.FormalTerm(second))
	if err != nil {
		t.Fatal(err)
	}
	gotTerm, exact = got.Term()
	if _, allocation := gotTerm.Allocation(); !exact || !allocation {
		t.Fatalf("symbolic allocation image = %v", got)
	}
	if _, concrete := symbolic.MaterializeSingleton(identity.FormalTerm(second)); concrete {
		t.Fatal("symbolic allocation materialized without frame authority")
	}

	allocations, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 9, 1), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	root := NewIdentitySubstitutionAuthority(formalRename, allocations)
	want, ok := allocations.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation image")
	}
	if got, ok := root.MaterializeSingleton(identity.FormalTerm(second)); !ok || got != want {
		t.Fatalf("root materialization = %#v/%v, want %#v", got, ok, want)
	}
	if got, err := root.Image(identity.AllocationTerm(template)); err != nil {
		t.Fatal(err)
	} else if concrete, ok := got.ID(); !ok || concrete != want {
		t.Fatalf("direct allocation image = %v", got)
	}
}

func TestIdentitySubstitutionRegisteredTopAndBottomLaws(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-substitution-laws")), 1)
	variable := identity.NewFormalVar(identity.NewFormalSchemaID(body, 1), formal.Input)
	term := identity.FormalTerm(variable)
	value := identityvalue.PresentTerm(reg, term)
	slot := statekey.SymbolValue(symbol.ID(9901))
	input := Domain(reg).Bottom().WriteValue(reg, slot, value)
	input.heapTableIdentity = input.heapTableIdentity.withTerm(term, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value}))
	input.placement = input.placement.withTerm(term, placement.Stack)
	input.frozenTables, _ = input.frozenTables.freezeTerm(term)

	topSubstitution, ok := identity.NewSubstitution([]identity.Binding{{Variable: variable, Image: identity.Top()}})
	if !ok {
		t.Fatal("Top substitution")
	}
	got, unreachable, err := ApplyIdentitySubstitution(
		context.Background(), reg, keys, NewIdentitySubstitutionAuthority(topSubstitution, nil), input,
	)
	if err != nil || unreachable {
		t.Fatalf("Top substitution = unreachable %v, error %v", unreachable, err)
	}
	if !product.Get(reg, got.ReadValue(reg, slot), identity.Key).IsTop() {
		t.Fatal("embedded product identity did not take exact Top image")
	}
	if !got.heapTableIdentity.top || !got.placement.top {
		t.Fatal("identity-keyed may map did not take exact whole-map Top image")
	}
	if got.frozenTables.contains(term) {
		t.Fatal("must-set retained a definite proof for an unknown identity image")
	}

	bottomSubstitution, ok := identity.NewSubstitution([]identity.Binding{{Variable: variable, Image: identity.Bottom()}})
	if !ok {
		t.Fatal("Bottom substitution")
	}
	_, unreachable, err = ApplyIdentitySubstitution(
		context.Background(), reg, keys, NewIdentitySubstitutionAuthority(bottomSubstitution, nil), input,
	)
	if err != nil || !unreachable {
		t.Fatalf("Bottom substitution = unreachable %v, error %v", unreachable, err)
	}
}
