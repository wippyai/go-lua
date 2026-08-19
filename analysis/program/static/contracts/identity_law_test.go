package contracts

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sectionDomain  = "program/static/contracts-law"
	sectionVersion = 1
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyTypePrimitive] = 4
	counts[keyspace.FamilyTypeParam] = 2
	counts[keyspace.FamilyFunction] = 2
	counts[keyspace.FamilyCall] = 2
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func primitive(ordinal uint32) keyspace.Term { return term(keyspace.FamilyTypePrimitive, ordinal) }

// ledgerInput authors both sidecars, with function-side columns present so the
// call segment does not start at column zero.
func ledgerInput() Input {
	return Input{
		Function: []FunctionContract{
			{
				TypeParams:   []keyspace.Term{term(keyspace.FamilyTypeParam, 1), term(keyspace.FamilyTypeParam, 2)},
				ReturnsKnown: true,
				Returns:      []keyspace.Term{primitive(1), primitive(2)},
			},
			{},
		},
		Call: []CallContract{
			{TypeArguments: []keyspace.Term{primitive(3), primitive(4)}},
			{},
		},
	}
}

func sectionBytes(t *testing.T, input Input) []byte {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func sectionReader(t *testing.T, data []byte) *framing.Reader {
	t.Helper()
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

// TestAuthoredDistinctionsReachTheSection proves the section byte stream, which
// is the same schema the Static ContentID digests, separates every authored
// field, arity, and order distinction this vertical retains.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*Input)
	}{
		{"function-contract.type-param", func(in *Input) {
			in.Function[0].TypeParams[0] = term(keyspace.FamilyTypeParam, 2)
		}},
		{"function-contract.type-param-arity", func(in *Input) {
			in.Function[0].TypeParams = in.Function[0].TypeParams[:1]
		}},
		{"function-contract.type-param-order", func(in *Input) {
			in.Function[0].TypeParams[0], in.Function[0].TypeParams[1] =
				in.Function[0].TypeParams[1], in.Function[0].TypeParams[0]
		}},
		{"function-contract.returns-known", func(in *Input) { in.Function[1].ReturnsKnown = true }},
		{"function-contract.return", func(in *Input) { in.Function[0].Returns[0] = primitive(3) }},
		{"function-contract.return-arity", func(in *Input) {
			in.Function[0].Returns = in.Function[0].Returns[:1]
		}},
		{"function-contract.return-order", func(in *Input) {
			in.Function[0].Returns[0], in.Function[0].Returns[1] =
				in.Function[0].Returns[1], in.Function[0].Returns[0]
		}},
		{"function-contract.row-order", func(in *Input) {
			in.Function[0], in.Function[1] = in.Function[1], in.Function[0]
		}},
		{"call-contract.argument", func(in *Input) { in.Call[0].TypeArguments[0] = primitive(1) }},
		{"call-contract.argument-arity", func(in *Input) {
			in.Call[0].TypeArguments = in.Call[0].TypeArguments[:1]
		}},
		{"call-contract.argument-order", func(in *Input) {
			in.Call[0].TypeArguments[0], in.Call[0].TypeArguments[1] =
				in.Call[0].TypeArguments[1], in.Call[0].TypeArguments[0]
		}},
		{"call-contract.row-order", func(in *Input) { in.Call[0], in.Call[1] = in.Call[1], in.Call[0] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := sectionBytes(t, ledgerInput())
			perturbed := ledgerInput()
			test.perturb(&perturbed)
			if bytes.Equal(base, sectionBytes(t, perturbed)) {
				t.Fatal("authored distinction is absent from the section stream")
			}
		})
	}
}

// TestSectionRoundTripPreservesEveryAuthoredRow proves the section decoder
// recovers exactly the authored input the writer emitted.
func TestSectionRoundTripPreservesEveryAuthoredRow(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	decoded, err := Decode(reader)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(encoded, sectionBytes(t, decoded)) {
		t.Fatal("round-tripped input did not reproduce the section stream")
	}
}

// TestScanValidatesWithoutRetainingRows proves the preflight half consumes the
// same stream shape as Decode.
func TestScanValidatesWithoutRetainingRows(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	if err := Scan(reader); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Scan left the stream unconsumed: %v", err)
	}
}

// TestCallTypeArgumentWidthCountsOnlyTheCallSegment proves the sealed width is
// the call segment of the shared term column, not the whole column: the
// function contracts above deliberately occupy the column first.
func TestCallTypeArgumentWidthCountsOnlyTheCallSegment(t *testing.T) {
	table, err := Build(ledgerInput(), ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	calls := table.View().Calls()
	walked := 0
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok {
			t.Fatalf("Calls().At(%d) failed", index)
		}
		count, ok := calls.TypeArgumentCount(call)
		if !ok {
			t.Fatalf("TypeArgumentCount(%v) failed", call)
		}
		walked += count
	}
	if walked == 0 {
		t.Fatal("fixture authors no call type arguments, so the sealed width proves nothing")
	}
	if got := table.CallTypeArgumentWidth(); got != walked {
		t.Fatalf("sealed call type-argument width = %d, want %d", got, walked)
	}

	functions := table.View().Functions()
	functionTerms := 0
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			t.Fatalf("Functions().At(%d) failed", index)
		}
		params, paramsOK := functions.TypeParamCount(function)
		returns, returnsOK := functions.ReturnCount(function)
		if !paramsOK || !returnsOK {
			t.Fatalf("function contract %v column counts failed", function)
		}
		functionTerms += params + returns
	}
	if functionTerms == 0 {
		t.Fatal("fixture shares no term column with function contracts, so the segment offset proves nothing")
	}
}

// TestCallTypeArgumentIdentitySeparatesAuthoredSequences proves the retained
// per-call identity is a function of the authored sequence, so it is neither
// a query derivative nor recomputable-at-read state.
func TestCallTypeArgumentIdentitySeparatesAuthoredSequences(t *testing.T) {
	table, err := Build(ledgerInput(), ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	calls := table.View().Calls()
	first, ok := calls.TypeArgumentID(term(keyspace.FamilyCall, 1))
	if !ok || !first.Available() {
		t.Fatal("call 1 has no sealed type-argument identity")
	}
	empty, ok := calls.TypeArgumentID(term(keyspace.FamilyCall, 2))
	if !ok || !empty.Available() {
		t.Fatal("an empty call type-argument column has no sealed identity")
	}
	if first == empty {
		t.Fatal("distinct authored type-argument sequences share one identity")
	}

	permuted := ledgerInput()
	permuted.Call[0].TypeArguments[0], permuted.Call[0].TypeArguments[1] =
		permuted.Call[0].TypeArguments[1], permuted.Call[0].TypeArguments[0]
	other, err := Build(permuted, ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	swapped, ok := other.View().Calls().TypeArgumentID(term(keyspace.FamilyCall, 1))
	if !ok || swapped == first {
		t.Fatal("call type-argument identity ignored authored order")
	}
	if id, ok := calls.TypeArgumentID(term(keyspace.FamilyCall, 9)); ok || id.Available() {
		t.Fatal("TypeArgumentID admitted a term past the sealed denominator")
	}
}
