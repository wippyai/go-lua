package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// activationTransportLawOwner is a Schema whose only structural Rule is one
// activation Rule, declared beside enough Factors to order transport vectors of
// several arities.
type activationTransportLawOwner struct {
	schema  *Schema
	factors []*FactorSlot[uint64]
	rule    *SchemaActivationRuleSlot
}

func newActivationTransportLawOwner(t testing.TB, count int, salt uint64) activationTransportLawOwner {
	t.Helper()
	builder := NewSchema()
	owner := activationTransportLawOwner{factors: make([]*FactorSlot[uint64], count)}
	for index := range owner.factors {
		factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(salt+10+uint64(index)))
		if !factorOK || factor == nil {
			t.Fatal("activation transport law factor")
		}
		owner.factors[index] = factor
	}
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(salt+2))
	rule, ruleOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic:   coldKey(salt + 3),
		Activation: family,
	})
	schema, schemaOK := builder.Seal()
	if !familyOK || !ruleOK || !schemaOK || schema == nil {
		t.Fatal("activation transport law schema")
	}
	owner.schema, owner.rule = schema, rule
	return owner
}

// activationTransportLawSemantics is the sealed cold Factor identity of every
// declared Factor, in declaration order.
func activationTransportLawSemantics(t testing.TB, owner activationTransportLawOwner) []composition.Key {
	t.Helper()
	semantics := make([]composition.Key, len(owner.factors))
	for index, factor := range owner.factors {
		ordinal, ordinalOK := factor.Ordinal()
		if !ordinalOK {
			t.Fatal("activation transport law factor ordinal")
		}
		semantics[index] = owner.schema.factorSemanticAt(ordinal)
		if !semantics[index].Available() {
			t.Fatal("activation transport law factor semantic")
		}
	}
	return semantics
}

// openActivationTransportLawBinding binds every Factor and the activation Rule
// and stops before Seal, which is the phase a transport vector is declared in.
func openActivationTransportLawBinding(t testing.TB, owner activationTransportLawOwner) *SchemaBinding {
	t.Helper()
	binding := NewSchemaBinding(owner.schema)
	for _, factor := range owner.factors {
		if !BindFactor(binding, factor, hotUintFactorSpec()) {
			t.Fatal("activation transport law factor binding")
		}
	}
	if !BindActivationRule(binding, owner.rule, HotActivationSpec{
		Fold: func(frame ActivationFrame) ActivationResult {
			return Activated(frame)
		},
	}) {
		t.Fatal("activation transport law activation binding")
	}
	return binding
}

// TestMountedActivationTransportVectorArityIsDeclared proves the engine holds no
// transport arity of its own: a vector of any arity the Schema's declared
// Factors cover binds, and the issuer retains exactly the Factor semantics the
// caller ordered.
func TestMountedActivationTransportVectorArityIsDeclared(t *testing.T) {
	owner := newActivationTransportLawOwner(t, 4, 948_100)
	semantics := activationTransportLawSemantics(t, owner)
	export := owner.factors[len(owner.factors)-1]
	for arity := 1; arity < len(owner.factors); arity++ {
		binding := openActivationTransportLawBinding(t, owner)
		imports := make([]AnyFactorRef, arity)
		for index := range imports {
			imports[index] = owner.factors[index].Ref().Any()
		}
		issuer, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, imports, export.Ref().Any())
		if !ok || issuer == nil {
			t.Fatalf("arity %d transport vector refused", arity)
		}
		if len(issuer.imports) != arity || issuer.export != semantics[len(owner.factors)-1] {
			t.Fatalf("arity %d issuer kept %d imports and export %v", arity, len(issuer.imports), issuer.export)
		}
		for index, key := range issuer.imports {
			if key != semantics[index] {
				t.Fatalf("arity %d import %d escaped its declared order", arity, index)
			}
		}
		if !binding.Seal() {
			t.Fatalf("arity %d binding seal", arity)
		}
	}
}

// TestMountedActivationTransportVectorRefusesUnusableDeclarations proves every
// vector defect fails closed at bind: an empty import vector, a Factor named
// twice, an export repeating an import, a Factor of another Schema, and an
// unavailable reference all leave the Binding without an issuer.
func TestMountedActivationTransportVectorRefusesUnusableDeclarations(t *testing.T) {
	owner := newActivationTransportLawOwner(t, 3, 948_200)
	foreign := newActivationTransportLawOwner(t, 3, 948_300)
	binding := openActivationTransportLawBinding(t, owner)
	first, second, third := owner.factors[0].Ref().Any(), owner.factors[1].Ref().Any(), owner.factors[2].Ref().Any()
	if _, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, nil, third); ok {
		t.Fatal("an empty import vector was admitted")
	}
	if _, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{first, first}, third); ok {
		t.Fatal("one Factor was admitted twice in an import vector")
	}
	if _, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{first, second}, first); ok {
		t.Fatal("an export repeating an import was admitted")
	}
	if _, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{first, foreign.factors[1].Ref().Any()}, third); ok {
		t.Fatal("a Factor of another Schema was admitted")
	}
	if _, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{first, second}, AnyFactorRef{}); ok {
		t.Fatal("an unavailable export was admitted")
	}
	issuer, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{first, second}, third)
	if !ok || issuer == nil {
		t.Fatal("declared transport vector refused after refused declarations")
	}
	if !binding.Seal() {
		t.Fatal("binding seal after refused declarations")
	}
}
