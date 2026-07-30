package state

import (
	"context"
	"errors"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFactorIdentitySubstitutionUsesCompleteTupleInverseFiber(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	body := lexicalidentity.FunctionBody(
		lexicalidentity.UnitNamespaceFromContent([]byte("factor-identity-complete-fiber")), 1,
	)
	first := identity.NewFormalVar(identity.NewFormalSchemaID(body, 1), formal.Input)
	second := identity.NewFormalVar(identity.NewFormalSchemaID(body, 2), formal.Middle)
	firstTerm := identity.FormalTerm(first)
	secondTerm := identity.FormalTerm(second)
	target := identity.ID{Kind: "table", Site: "factor-identity-complete-fiber", Index: 1}
	substitution, ok := identity.NewSubstitution([]identity.Binding{
		{Variable: first, Image: identity.Singleton(target)},
		{Variable: second, Image: identity.Singleton(target)},
	})
	if !ok {
		t.Fatal("substitution")
	}
	authority := NewIdentitySubstitutionAuthority(substitution, nil)

	apply := func(t *testing.T, freezeSecond bool) State {
		t.Helper()
		input := domain.Lattice().Bottom().WriteValue(
			reg,
			statekey.SymbolValue(symbol.ID(9911)),
			identityvalue.PresentTerm(reg, secondTerm),
		)
		input.frozenTables, _ = input.frozenTables.freezeTerm(firstTerm)
		if freezeSecond {
			input.frozenTables, _ = input.frozenTables.freezeTerm(secondTerm)
		}
		input = domain.Normalize(input)
		residual, values := DecomposeValueLane(domain.Lattice(), input)
		factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
		if err != nil {
			t.Fatal(err)
		}
		nextValues, nextFactors, unreachable, err := ApplyIdentitySubstitutionTuple(
			context.Background(), domain, keys, authority, values, factors,
		)
		if err != nil || unreachable {
			t.Fatalf("transaction = unreachable %v, error %v", unreachable, err)
		}
		nextResidual, err := domain.ComposeSparse(nextFactors)
		if err != nil {
			t.Fatal(err)
		}
		return RecomposeValueLane(reg, domain.Lattice(), nextResidual, nextValues)
	}

	if got := apply(t, false); got.IsTableFrozen(target) {
		t.Fatal("quotient retained target although one active preimage lacked the must proof")
	}
	if got := apply(t, true); !got.IsTableFrozen(target) {
		t.Fatal("quotient dropped target although every active preimage had the must proof")
	}
}

func TestFactorIdentitySubstitutionRejectsIncompleteReorderedAndForeignTuple(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	input := domain.Lattice().Bottom()
	residual, values := DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	authority := NewIdentitySubstitutionAuthority(identity.Substitution{}, nil)
	assertIncomplete := func(name string, candidate []LaneFactor) {
		t.Helper()
		_, _, err := SealIdentitySubstitutionPlan(context.Background(), domain, keys, authority, values, candidate)
		if !errors.Is(err, ErrIncompleteLaneFactors) {
			t.Fatalf("%s error = %v, want ErrIncompleteLaneFactors", name, err)
		}
	}
	assertIncomplete("omitted", factors[:len(factors)-1])
	reordered := append([]LaneFactor(nil), factors...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	assertIncomplete("reordered", reordered)
	duplicated := append([]LaneFactor(nil), factors...)
	duplicated[1] = duplicated[0]
	assertIncomplete("duplicated", duplicated)

	foreignReg, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	foreignDomain := RegisteredProductDomain(foreignReg)
	foreignResidual, _ := DecomposeValueLane(foreignDomain.Lattice(), foreignDomain.Lattice().Bottom())
	foreignFactors, err := foreignDomain.DecomposeLanes(foreignResidual, foreignDomain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	foreign := append([]LaneFactor(nil), factors...)
	foreign[0] = foreignFactors[0]
	assertIncomplete("foreign", foreign)
}

func TestFactorIdentitySubstitutionReusesUnchangedTuple(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	input := domain.Lattice().Bottom()
	residual, values := DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	nextValues, nextFactors, unreachable, err := ApplyIdentitySubstitutionTuple(
		context.Background(), domain, keys,
		NewIdentitySubstitutionAuthority(identity.Substitution{}, nil), values, factors,
	)
	if err != nil || unreachable {
		t.Fatalf("transaction = unreachable %v, error %v", unreachable, err)
	}
	if !ValueFactorLattice[statekey.Value](reg).Same(values, nextValues) {
		t.Fatal("unchanged Values factor was not reused")
	}
	for index := range factors {
		equal, err := domain.LaneEqual(factors[index], nextFactors[index])
		if err != nil || !equal {
			t.Fatalf("factor %d equality = %v, %v", index, equal, err)
		}
	}
}
