package exactscalar

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestCompileEmptyInputOwnsCanonicalEmptyBundle(t *testing.T) {
	bundle, fault := Compile(Input{})
	if fault.Available() || bundle == nil {
		t.Fatalf("empty compile fault=%v bundle=%v", fault, bundle)
	}
	if rows := bundle.Rows(); len(rows) != 0 {
		t.Fatalf("empty rows = %d, want 0", len(rows))
	}
	if _, ok := bundle.Exact(identity.ContentID{}); ok {
		t.Fatal("zero identity unexpectedly had an exact scalar")
	}
	bundle.ReleaseFacts()
	if rows := bundle.Rows(); len(rows) != 0 {
		t.Fatalf("released bundle rows = %d, want 0", len(rows))
	}
}

func TestCompileAcyclicFiniteClosureHasNoCardinalityCutoff(t *testing.T) {
	const count = 257
	valueID := identity.ContentID{1}
	body := identity.ContentID{99}
	inputs := make([]programschema.OccurrenceInput, count)
	occurrences := make([]programschema.Occurrence, count)
	for index := 0; index < count; index++ {
		input, inputOK := programschema.NewOccurrenceInput(valueID)
		if !inputOK {
			t.Fatalf("input[%d] unavailable", index)
		}
		inputs[index] = input
		occurrence, occurrenceOK := programschema.NewOccurrence(
			programschema.OccurrenceValueSource, valueID, body, 0,
			0, 0, uint32(index), 1, keyspace.FamilyInteger,
			keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: int64(index)}, true,
		)
		if !occurrenceOK {
			t.Fatalf("occurrence[%d] unavailable", index)
		}
		occurrences[index] = occurrence
	}
	bundle, fault := Compile(Input{Occurrences: occurrences, OccurrenceInputs: inputs})
	if fault.Available() || bundle == nil {
		t.Fatalf("acyclic exact-scalar compile fault=%v bundle=%v", fault, bundle)
	}
	state := bundle.states[valueID]
	if state.unknown || len(state.values) != count {
		t.Fatalf("acyclic state=%+v, want %d finite literals", state, count)
	}
}

func TestCompileStableCopyCycleRemainsFinite(t *testing.T) {
	body := identity.ContentID{99}
	valueID := identity.ContentID{1}
	aliasID := identity.ContentID{2}
	dummyID := identity.ContentID{3}
	inputs := make([]programschema.OccurrenceInput, 5)
	for index, id := range []identity.ContentID{valueID, dummyID, valueID, dummyID, aliasID} {
		input, ok := programschema.NewOccurrenceInput(id)
		if !ok {
			t.Fatalf("input[%d] unavailable", index)
		}
		inputs[index] = input
	}
	seed, seedOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, valueID, body, 0,
		0, 0, 0, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7}, true,
	)
	aliasFromValue, aliasFromValueOK := programschema.NewOccurrence(
		programschema.OccurrenceValuesMember, aliasID, body, 0,
		0, 0, 1, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	valueFromAlias, valueFromAliasOK := programschema.NewOccurrence(
		programschema.OccurrenceValuesMember, valueID, body, 0,
		0, 0, 3, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	if !seedOK || !aliasFromValueOK || !valueFromAliasOK {
		t.Fatal("failed to construct stable copy rows")
	}
	bundle, fault := Compile(Input{
		Occurrences:      []programschema.Occurrence{seed, aliasFromValue, valueFromAlias},
		OccurrenceInputs: inputs,
	})
	if fault.Available() || bundle == nil {
		t.Fatalf("stable copy compile fault=%v bundle=%v", fault, bundle)
	}
	state := bundle.states[valueID]
	aliasState := bundle.states[aliasID]
	if state.unknown || len(state.values) != 1 || aliasState.unknown || len(aliasState.values) != 1 {
		t.Fatalf("stable copy states value=%+v alias=%+v, want finite singleton states", state, aliasState)
	}
	if value, exact := bundle.Exact(valueID); !exact || value.Integer != 7 {
		t.Fatalf("stable copy value exact=%+v/%v, want integer 7", value, exact)
	}
	if value, exact := bundle.Exact(aliasID); !exact || value.Integer != 7 {
		t.Fatalf("stable copy alias exact=%+v/%v, want integer 7", value, exact)
	}
}

func TestCompileExternalArithmeticInsideCopyCycleRemainsFinite(t *testing.T) {
	body := identity.ContentID{99}
	leftID := identity.ContentID{1}
	rightID := identity.ContentID{2}
	valueID := identity.ContentID{3}
	aliasID := identity.ContentID{4}
	dummyID := identity.ContentID{5}
	inputs := make([]programschema.OccurrenceInput, 8)
	for index, id := range []identity.ContentID{leftID, rightID, leftID, rightID, dummyID, valueID, dummyID, aliasID} {
		input, ok := programschema.NewOccurrenceInput(id)
		if !ok {
			t.Fatalf("input[%d] unavailable", index)
		}
		inputs[index] = input
	}
	left, leftOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, leftID, body, 0,
		0, 0, 0, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}, true,
	)
	right, rightOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, rightID, body, 0,
		0, 0, 1, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 2}, true,
	)
	arithmetic, arithmeticOK := programschema.NewOccurrence(
		programschema.OccurrenceBinaryArithmetic, valueID, body, uint64(flowkind.BinaryAdd),
		0, 0, 2, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	alias, aliasOK := programschema.NewOccurrence(
		programschema.OccurrenceValuesMember, aliasID, body, 0,
		0, 0, 4, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	back, backOK := programschema.NewOccurrence(
		programschema.OccurrenceValuesMember, valueID, body, 0,
		0, 0, 6, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	if !leftOK || !rightOK || !arithmeticOK || !aliasOK || !backOK {
		t.Fatal("failed to construct external arithmetic copy cycle")
	}
	bundle, fault := Compile(Input{
		Occurrences:      []programschema.Occurrence{left, right, arithmetic, alias, back},
		OccurrenceInputs: inputs,
	})
	if fault.Available() || bundle == nil {
		t.Fatalf("external arithmetic copy cycle fault=%v bundle=%v", fault, bundle)
	}
	for _, id := range []identity.ContentID{valueID, aliasID} {
		value, exact := bundle.Exact(id)
		if !exact || value.Integer != 3 {
			t.Fatalf("external arithmetic copy cycle exact[%v]=%+v/%v, want integer 3", id, value, exact)
		}
	}
}

func TestCompileStableArithmeticRecurrenceWithNoNoveltyRemainsFinite(t *testing.T) {
	body := identity.ContentID{99}
	valueID := identity.ContentID{1}
	zeroID := identity.ContentID{2}
	inputs := make([]programschema.OccurrenceInput, 4)
	for index, id := range []identity.ContentID{valueID, zeroID, valueID, zeroID} {
		input, ok := programschema.NewOccurrenceInput(id)
		if !ok {
			t.Fatalf("input[%d] unavailable", index)
		}
		inputs[index] = input
	}
	seed, seedOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, valueID, body, 0,
		0, 0, 0, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 3}, true,
	)
	zero, zeroOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, zeroID, body, 0,
		0, 0, 1, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 0}, true,
	)
	recurrence, recurrenceOK := programschema.NewOccurrence(
		programschema.OccurrenceBinaryArithmetic, valueID, body, uint64(flowkind.BinaryAdd),
		0, 0, 2, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	if !seedOK || !zeroOK || !recurrenceOK {
		t.Fatal("failed to construct stable arithmetic recurrence")
	}
	bundle, fault := Compile(Input{
		Occurrences:      []programschema.Occurrence{seed, zero, recurrence},
		OccurrenceInputs: inputs,
	})
	if fault.Available() || bundle == nil {
		t.Fatalf("stable arithmetic recurrence fault=%v bundle=%v", fault, bundle)
	}
	state := bundle.states[valueID]
	if state.unknown || len(state.values) != 1 {
		t.Fatalf("stable arithmetic state=%+v, want finite singleton", state)
	}
	if value, exact := bundle.Exact(valueID); !exact || value.Integer != 3 {
		t.Fatalf("stable arithmetic exact=%+v/%v, want integer 3", value, exact)
	}
}

func TestCompileIntegerLoopCounterDriftWidensAtRecurrence(t *testing.T) {
	body := identity.ContentID{99}
	cellID := identity.ContentID{1}
	oneID := identity.ContentID{2}
	incrementID := identity.ContentID{3}
	writeID := identity.ContentID{4}
	dummyID := identity.ContentID{5}
	inputs := make([]programschema.OccurrenceInput, 7)
	for index, id := range []identity.ContentID{cellID, oneID, cellID, oneID, dummyID, incrementID, cellID} {
		input, ok := programschema.NewOccurrenceInput(id)
		if !ok {
			t.Fatalf("input[%d] unavailable", index)
		}
		inputs[index] = input
	}
	seed, seedOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, cellID, body, 0,
		0, 0, 0, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 0}, true,
	)
	one, oneOK := programschema.NewOccurrence(
		programschema.OccurrenceValueSource, oneID, body, 0,
		0, 0, 1, 1, keyspace.FamilyInteger,
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}, true,
	)
	increment, incrementOK := programschema.NewOccurrence(
		programschema.OccurrenceBinaryArithmetic, incrementID, body, uint64(flowkind.BinaryAdd),
		0, 0, 2, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	write, writeOK := programschema.NewOccurrence(
		programschema.OccurrenceStorageWrite, writeID, body, 0,
		0, 0, 4, 3, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	if !seedOK || !oneOK || !incrementOK || !writeOK {
		t.Fatal("failed to construct counter-drift rows")
	}
	bundle, fault := Compile(Input{
		Occurrences:      []programschema.Occurrence{seed, one, increment, write},
		OccurrenceInputs: inputs,
	})
	if fault.Available() || bundle == nil {
		t.Fatalf("counter-drift compile fault=%v bundle=%v", fault, bundle)
	}
	for _, id := range []identity.ContentID{cellID, incrementID} {
		state := bundle.states[id]
		if !state.unknown || state.values != nil {
			t.Fatalf("counter-drift state[%v]=%+v, want explicit unknown", id, state)
		}
	}
	if value, exact := bundle.Exact(oneID); !exact || value.Integer != 1 {
		t.Fatalf("counter increment literal exact=%+v/%v, want integer 1", value, exact)
	}
}
