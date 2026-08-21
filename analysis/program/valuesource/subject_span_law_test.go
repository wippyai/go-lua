package valuesource

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSubjectSpanOwnsLiteralAndTypeValueSpans(t *testing.T) {
	input := valueSourceLawProgram(t)
	families := []keyspace.Family{
		keyspace.FamilyNil,
		keyspace.FamilyBool,
		keyspace.FamilyInteger,
		keyspace.FamilyFloat,
		keyspace.FamilyString,
		keyspace.FamilyTypeValue,
	}
	for _, family := range families {
		for index := 0; index < Count(input, family); index++ {
			_, want, term, identityOK := IdentityAt(input, family, index)
			if !identityOK {
				continue // Authored TypeValue denominators may contain dead candidates.
			}
			got, gotOK := SubjectSpan(input, term)
			if !gotOK || got != want {
				t.Fatalf("SubjectSpan(%d, %08x) = %v/%v, want %v/true", family, uint32(term), got, gotOK, want)
			}
		}
	}
}

func TestSubjectSpanDelegatesDirectEvaluationForOtherTerms(t *testing.T) {
	input := valueSourceLawProgram(t)
	calls := input.Flow().Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		term, termOK := calls.At(index)
		want, _, _, spanOK := input.EvaluationSpan(term)
		if !termOK || !spanOK {
			continue
		}
		got, gotOK := SubjectSpan(input, term)
		if !gotOK || got != want {
			t.Fatalf("SubjectSpan(call %08x) = %v/%v, want %v/true", uint32(term), got, gotOK, want)
		}
		return
	}
	t.Fatal("value-source law fixture has no direct-evaluation call span")
}

func TestSubjectSpanRejectsInvalidTermsAndNilInput(t *testing.T) {
	input := valueSourceLawProgram(t)
	invalid := []keyspace.Term{
		0,
		keyspace.Term(1),
		keyspace.MakeTerm(keyspace.FamilyCount, 1),
		keyspace.MakeTerm(keyspace.FamilyString, uint32(Count(input, keyspace.FamilyString)+1)),
		keyspace.MakeTerm(keyspace.FamilyTypeValue, uint32(Count(input, keyspace.FamilyTypeValue)+1)),
	}
	for _, term := range invalid {
		if got, ok := SubjectSpan(input, term); ok || got.Available() {
			t.Fatalf("SubjectSpan(%08x) = %v/%v, want unavailable", uint32(term), got, ok)
		}
	}
	if got, ok := SubjectSpan(nil, keyspace.MakeTerm(keyspace.FamilyString, 1)); ok || got.Available() {
		t.Fatalf("SubjectSpan(nil) = %v/%v, want unavailable", got, ok)
	}
}
