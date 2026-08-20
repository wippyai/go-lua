package authored

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type claimTerms struct {
	body      keyspace.Term
	nil       keyspace.Term
	typeValue keyspace.Term
	claim     keyspace.Term
}

func claimFixture() (Input, claimTerms) {
	terms := claimTerms{
		body:      keyspace.MakeTerm(keyspace.FamilyBody, 1),
		nil:       keyspace.MakeTerm(keyspace.FamilyNil, 1),
		typeValue: keyspace.MakeTerm(keyspace.FamilyTypeValue, 1),
		claim:     keyspace.MakeTerm(keyspace.FamilyValueClaim, 1),
	}
	input := Input{}
	input.Counts[keyspace.FamilyBody] = 1
	input.Counts[keyspace.FamilyNil] = 1
	input.Counts[keyspace.FamilyTypeValue] = 1
	input.Counts[keyspace.FamilyValueClaim] = 3
	input.Claims = []ValueClaim{
		{Owner: terms.body, Operand: terms.nil, Kind: kind.ValueClaimTypeAs},
		{Owner: terms.body, Operand: terms.typeValue, Kind: kind.ValueClaimTypeColonColon},
		{Owner: terms.body, Operand: terms.claim, Kind: kind.ValueClaimNonNil},
	}
	input.TypeValues = []TypeValue{{Owner: terms.body}}
	return input, terms
}

func TestClaimsAndTypeValuesAuthoredLaws(t *testing.T) {
	input, terms := claimFixture()
	component := buildFlowForTest(t, input)
	claims := component.Claims()
	if claims.Count() != 3 {
		t.Fatalf("Claims Count = %d, want 3", claims.Count())
	}
	for index, claimKind := range []kind.ValueClaimKind{kind.ValueClaimTypeAs, kind.ValueClaimTypeColonColon, kind.ValueClaimNonNil} {
		term, ok := claims.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyValueClaim, uint32(index+1)) {
			t.Fatalf("Claim At(%d) = %08x, %v", index, uint32(term), ok)
		}
		owner, operand, got, ok := claims.Get(term)
		if !ok || owner != terms.body || got != claimKind {
			t.Fatalf("Claim %d = owner %08x kind %v ok %v", index, uint32(owner), got, ok)
		}
		if index == 1 && operand != terms.typeValue {
			t.Fatalf("TypeValue claim operand = %08x", uint32(operand))
		}
		if index == 2 && operand != terms.claim {
			t.Fatalf("ValueClaim claim operand = %08x", uint32(operand))
		}
	}
	typeValues := component.TypeValues()
	if typeValues.Count() != 1 {
		t.Fatalf("TypeValues Count = %d, want 1", typeValues.Count())
	}
	if term, ok := typeValues.At(0); !ok || term != terms.typeValue {
		t.Fatalf("TypeValue At = %08x, %v", uint32(term), ok)
	}
	if owner, ok := typeValues.Get(terms.typeValue); !ok || owner != terms.body {
		t.Fatalf("TypeValue = owner %08x ok %v", uint32(owner), ok)
	}
}

func TestValuesAdmitTypeValuesAndClaims(t *testing.T) {
	input, terms := claimFixture()
	input.Counts[keyspace.FamilyValues] = 1
	input.Values = ValuesInput{
		Rows:  []Value{{Owner: terms.body, Fixed: Range{Start: 0, End: 2}}},
		Terms: []keyspace.Term{terms.typeValue, terms.claim},
	}
	if _, err := Build(input); err != nil {
		t.Fatalf("Values rejected scalar TypeValue or ValueClaim: %v", err)
	}
}

func TestClaimsAndTypeValuesRejectInvalidAuthoredRows(t *testing.T) {
	_, terms := claimFixture()
	for _, claimKind := range []kind.ValueClaimKind{0, kind.ValueClaimNonNil + 1} {
		input, _ := claimFixture()
		input.Claims[0].Kind = claimKind
		if _, err := Build(input); err == nil {
			t.Fatalf("ValueClaimKind %d accepted", claimKind)
		}
	}
	input, _ := claimFixture()
	input.Claims[0].Owner = input.Claims[0].Operand
	if _, err := Build(input); err == nil {
		t.Fatal("ValueClaim non-Body owner accepted")
	}
	input, _ = claimFixture()
	input.Claims[0].Operand = input.Claims[0].Owner
	if _, err := Build(input); err == nil {
		t.Fatal("ValueClaim non-scalar operand accepted")
	}
	input, _ = claimFixture()
	input.TypeValues[0].Owner = input.Claims[0].Operand
	if _, err := Build(input); err == nil {
		t.Fatal("TypeValue non-Body owner accepted")
	}
	input, _ = claimFixture()
	input.Counts[keyspace.FamilyValueClaim]--
	if _, err := Build(input); err == nil {
		t.Fatal("ValueClaim count mismatch accepted")
	}
	input, _ = claimFixture()
	input.Counts[keyspace.FamilyTypeValue]--
	if _, err := Build(input); err == nil {
		t.Fatal("TypeValue count mismatch accepted")
	}
	for _, scalar := range []keyspace.Term{terms.typeValue, terms.claim} {
		input, _ = claimFixture()
		input.Counts[keyspace.FamilyValues] = 1
		input.Values = ValuesInput{Rows: []Value{{Owner: terms.body, Tail: scalar}}}
		if _, err := Build(input); err == nil {
			t.Fatalf("Values accepted scalar %08x as an open tail", uint32(scalar))
		}
	}
}

func TestClaimsAndTypeValuesCopyContentAndQueryBounds(t *testing.T) {
	input, terms := claimFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Claims[0].Operand = terms.typeValue
	input.TypeValues[0].Owner = terms.nil
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, err := finalizer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if _, operand, _, ok := component.Claims().Get(terms.claim); !ok || operand != terms.nil {
		t.Fatal("Build retained caller ValueClaim storage")
	}
	if owner, ok := component.TypeValues().Get(terms.typeValue); !ok || owner != terms.body {
		t.Fatal("Build retained caller TypeValue storage")
	}

	first := buildFlowForTest(t, claimInput())
	if first.Cold().ContentID() != buildFlowForTest(t, claimInput()).Cold().ContentID() {
		t.Fatal("equal Claim and TypeValue authored content has unstable identity")
	}
	changed := claimInput()
	changed.Claims[0].Kind = kind.ValueClaimTypeColonColon
	if first.Cold().ContentID() == buildFlowForTest(t, changed).Cold().ContentID() {
		t.Fatal("ValueClaim authored content did not change identity")
	}
	changed = claimInput()
	changed.TypeValues[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 2)
	changed.Counts[keyspace.FamilyBody] = 2
	if first.Cold().ContentID() == buildFlowForTest(t, changed).Cold().ContentID() {
		t.Fatal("TypeValue authored content did not change identity")
	}

	claims, typeValues := component.Claims(), component.TypeValues()
	for _, index := range []int{-1, claims.Count()} {
		if term, ok := claims.At(index); ok || term != 0 {
			t.Fatalf("Claim At(%d) = %08x, %v", index, uint32(term), ok)
		}
	}
	for _, index := range []int{-1, typeValues.Count()} {
		if term, ok := typeValues.At(index); ok || term != 0 {
			t.Fatalf("TypeValue At(%d) = %08x, %v", index, uint32(term), ok)
		}
	}
	if _, _, _, ok := claims.Get(terms.typeValue); ok {
		t.Fatal("Claims Get accepted TypeValue term")
	}
	if _, ok := typeValues.Get(terms.claim); ok {
		t.Fatal("TypeValues Get accepted ValueClaim term")
	}
}

func claimInput() Input {
	input, _ := claimFixture()
	return input
}
