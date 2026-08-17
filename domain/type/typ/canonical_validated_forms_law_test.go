package typ

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

// canonicalHostileForms are byte strings that are not lawful canonical scoped
// forms. They are the record's adversary: each must be refused every time it is
// presented, whatever the record has admitted in between.
func canonicalHostileForms(t *testing.T) map[string][]byte {
	t.Helper()
	valid, err := EncodeCanonicalFormals(context.Background(), NewTuple(String, Number), nil)
	if err != nil {
		t.Fatal(err)
	}
	badExternal := appendCanonicalFormalsDomain(nil)
	badExternal = binary.AppendUvarint(badExternal, canonicalScopedTypeVersion)
	badExternal = appendCanonicalFormalDefinition(badExternal, 0, []byte{canonicalTypeParam, canonicalScopedExternalFormal, 1, 0}, 0)

	badLocalCycle := appendCanonicalFormalsDomain(nil)
	badLocalCycle = binary.AppendUvarint(badLocalCycle, canonicalScopedTypeVersion)
	badLocalCycle = appendCanonicalFormalDefinition(badLocalCycle, 0, []byte{canonicalTypeParam, canonicalScopedLocalFormal, 0, 0}, 1)
	badLocalCycle = appendCanonicalFormalReference(badLocalCycle, 0)

	duplicateNode := appendCanonicalFormalsDomain(nil)
	duplicateNode = binary.AppendUvarint(duplicateNode, canonicalScopedTypeVersion)
	duplicateNode = appendCanonicalFormalDefinition(duplicateNode, 0, []byte{canonicalTuple, 2}, 2)
	duplicateNode = appendCanonicalFormalDefinition(duplicateNode, 1, []byte{canonicalString}, 0)
	duplicateNode = appendCanonicalFormalDefinition(duplicateNode, 2, []byte{canonicalString}, 0)

	return map[string][]byte{
		"external ordinal outside scope": badExternal,
		"malformed local binder cycle":   badLocalCycle,
		"noncanonical duplicate node":    duplicateNode,
		"trailing byte":                  append(append([]byte(nil), valid...), 0),
		"unknown scalar":                 appendCanonicalFormalDefinition(appendCanonicalFormalsVersion(nil), 0, []byte{0xff}, 0),
		"wrong domain":                   appendCanonicalFormalDefinition(nil, 0, []byte{canonicalString}, 0),
	}
}

// canonicalLawfulForms is a vocabulary of lawful scoped forms used to fill the
// validated-form record between two sightings of a hostile one.
func canonicalLawfulForms(t *testing.T) [][]byte {
	t.Helper()
	external := NewTypeParam("T", LiteralString("constraint"))
	local := NewTypeParam("local", nil)
	element := NewTypeParam("item", nil)
	channel := NewGeneric("Channel", []*TypeParam{element}, NewArray(element))
	corpus := []struct {
		value   Type
		formals []*TypeParam
	}{
		{NewTuple(String, Number), nil},
		{NewArray(String), nil},
		{NewMap(String, Integer), nil},
		{channel, nil},
		{Instantiate(channel, String), nil},
		{NewTuple(external, NewArray(external)), []*TypeParam{external}},
		{Func().TypeParamRef(local).Param("value", local).Returns(external, local).Build(), []*TypeParam{external}},
	}
	forms := make([][]byte, 0, len(corpus))
	for index, entry := range corpus {
		encoded, err := EncodeCanonicalFormals(context.Background(), entry.value, entry.formals)
		if err != nil {
			t.Fatalf("lawful form %d: %v", index, err)
		}
		if err := ValidateCanonicalFormals(encoded, len(entry.formals)); err != nil {
			t.Fatalf("lawful form %d rejected: %v", index, err)
		}
		forms = append(forms, encoded)
	}
	return forms
}

// TestValidatedFormRecordNeverAdmitsAnInvalidForm is the record's safety law.
// Validity is a property of the bytes, so a form the record has never admitted
// is judged in full every time it is presented: a hostile form is refused on
// first sight, refused again after the record has filled with lawful forms, and
// refused on every repetition. Nothing about the record's contents can make an
// unlawful byte string lawful.
func TestValidatedFormRecordNeverAdmitsAnInvalidForm(t *testing.T) {
	hostile := canonicalHostileForms(t)
	for name, encoded := range hostile {
		if err := ValidateCanonicalFormals(encoded, 0); !errors.Is(err, ErrInvalidCanonicalType) {
			t.Fatalf("%s admitted on first sight: %v", name, err)
		}
	}
	canonicalLawfulForms(t)
	for name, encoded := range hostile {
		for sighting := 0; sighting < 3; sighting++ {
			if err := ValidateCanonicalFormals(encoded, 0); !errors.Is(err, ErrInvalidCanonicalType) {
				t.Fatalf("%s admitted at sighting %d after lawful forms: %v", name, sighting+1, err)
			}
			if _, err := DecodeCanonicalFormals(context.Background(), encoded, nil); err == nil {
				t.Fatalf("%s decoded at sighting %d after lawful forms", name, sighting+1)
			}
		}
		if canonicalValidatedWireForms.admits(encoded, 0) {
			t.Fatalf("%s entered the validated-form record", name)
		}
	}
}

// TestValidatedFormRecordSeparatesFormalScopes pins the record's key to the
// whole question. The same bytes read against a different external scope are a
// different question, and admitting one scope must not answer another.
func TestValidatedFormRecordSeparatesFormalScopes(t *testing.T) {
	formal := NewTypeParam("T", LiteralString("constraint"))
	encoded, err := EncodeCanonicalFormals(context.Background(), NewTuple(formal, NewArray(formal)), []*TypeParam{formal})
	if err != nil {
		t.Fatal(err)
	}
	for sighting := 0; sighting < 2; sighting++ {
		if err := ValidateCanonicalFormals(encoded, 1); err != nil {
			t.Fatalf("lawful form rejected at sighting %d: %v", sighting+1, err)
		}
	}
	for sighting := 0; sighting < 2; sighting++ {
		if err := ValidateCanonicalFormals(encoded, 0); !errors.Is(err, ErrInvalidCanonicalType) {
			t.Fatalf("form admitted outside its receiver scope at sighting %d: %v", sighting+1, err)
		}
		if err := ValidateCanonicalFormals(encoded, 2); err != nil {
			t.Fatalf("form rejected under a wider receiver scope at sighting %d: %v", sighting+1, err)
		}
	}
}

// TestValidatedFormRecordDerivesEachFormOnce is the cost half of the same law.
// A second sighting of a form must ask the record and stop: the miss counter
// counts exactly the questions the record could not answer, so an unchanged
// miss count across a repeated validation is the statement that nothing was
// re-derived.
func TestValidatedFormRecordDerivesEachFormOnce(t *testing.T) {
	value := NewTuple(NewArray(String), NewMap(String, Integer), MaterializeUnion([]Type{String, Integer}))
	encoded, err := EncodeCanonicalFormals(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCanonicalFormals(encoded, 0); err != nil {
		t.Fatal(err)
	}
	misses := canonicalValidatedWireForms.misses.Load()
	sightings := canonicalValidatedWireForms.sightings.Load()
	for sighting := 0; sighting < 4; sighting++ {
		if err := ValidateCanonicalFormals(encoded, 0); err != nil {
			t.Fatalf("admitted form rejected at sighting %d: %v", sighting+1, err)
		}
	}
	if repeated := canonicalValidatedWireForms.misses.Load(); repeated != misses {
		t.Fatalf("repeated validation of one admitted form missed the record %d times, want none", repeated-misses)
	}
	if asked := canonicalValidatedWireForms.sightings.Load(); asked != sightings+4 {
		t.Fatalf("four repeated validations asked the record %d times, want 4", asked-sightings)
	}
}

// TestValidatedGraphFormRecordDerivesEachEncodedFormOnce is the encoder-side
// counterpart. The scoped scope laws are applied to the graph that produced the
// emitted bytes, so a second encode of the same value must find its own form
// already admitted and apply nothing.
func TestValidatedGraphFormRecordDerivesEachEncodedFormOnce(t *testing.T) {
	formal := NewTypeParam("T", nil)
	value := RebuildRecord(RecordParts{
		Fields:       []Field{{Name: "head", Type: formal}, {Name: "tail", Type: NewArray(formal)}},
		MapKey:       String,
		MapValue:     Integer,
		AssumeSorted: true,
	})
	first, err := EncodeCanonicalFormals(context.Background(), value, []*TypeParam{formal})
	if err != nil {
		t.Fatal(err)
	}
	misses := canonicalValidatedGraphForms.misses.Load()
	second, err := EncodeCanonicalFormals(context.Background(), value, []*TypeParam{formal})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("two encodes of one value differ:\n%x\n%x", first, second)
	}
	if repeated := canonicalValidatedGraphForms.misses.Load(); repeated != misses {
		t.Fatalf("a second encode of one value missed the graph record %d times, want none", repeated-misses)
	}
}

// TestValidatedGraphFormRecordStillRejectsUnlawfulGraphs proves the encoder's
// scope laws still hold after admission moved behind emission. An unlawful
// scoped graph must be refused on every encode, and its bytes must never reach
// the record.
func TestValidatedGraphFormRecordStillRejectsUnlawfulGraphs(t *testing.T) {
	escaped := NewTypeParam("Escaped", nil)
	inner := Func().TypeParamRef(escaped).Param("value", escaped).Returns(escaped).Build()
	// The binder owning Escaped is the inner function, so naming the same
	// parameter outside that binder is an out-of-scope lexical reference.
	unlawful := NewTuple(inner, escaped)
	for sighting := 0; sighting < 3; sighting++ {
		encoded, err := EncodeCanonicalFormals(context.Background(), unlawful, nil)
		if err == nil {
			t.Fatalf("out-of-scope local formal encoded at sighting %d: %x", sighting+1, encoded)
		}
		if !errors.Is(err, ErrInvalidCanonicalType) {
			t.Fatalf("out-of-scope local formal rejected with %v, want %v", err, ErrInvalidCanonicalType)
		}
	}
}

// TestValidatedFormRecordHitPathAllocatesNothing keeps the record cheap enough
// to consult before every validation. A hit reads the bytes in place, so it must
// not allocate at all.
func TestValidatedFormRecordHitPathAllocatesNothing(t *testing.T) {
	encoded, err := EncodeCanonicalFormals(context.Background(), NewTuple(NewArray(String), NewMap(String, Integer)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCanonicalFormals(encoded, 0); err != nil {
		t.Fatal(err)
	}
	if !canonicalValidatedWireForms.admits(encoded, 0) {
		t.Fatal("a validated form is absent from the record")
	}
	allocations := testing.AllocsPerRun(200, func() {
		if err := ValidateCanonicalFormals(encoded, 0); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("validating an admitted form allocated %v times per run, want 0", allocations)
	}
}
