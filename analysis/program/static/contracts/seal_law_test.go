package contracts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func sealTable(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return table
}

// TestContractsPreserveDenseTypedSidecars proves both sidecars survive the
// seal with their columns intact, and that the retained per-call identity is
// stable across reads.
func TestContractsPreserveDenseTypedSidecars(t *testing.T) {
	contracts := sealTable(t, ledgerInput()).View()
	function := term(keyspace.FamilyFunction, 1)
	call := term(keyspace.FamilyCall, 1)
	if contracts.Functions().Count() != 2 || contracts.Calls().Count() != 2 {
		t.Fatalf("dense counts = (%d, %d)", contracts.Functions().Count(), contracts.Calls().Count())
	}
	if known, ok := contracts.Functions().Get(function); !ok || !known {
		t.Fatalf("function header = (%v, %v)", known, ok)
	}
	if count, ok := contracts.Functions().TypeParamCount(function); !ok || count != 2 {
		t.Fatalf("function type parameter count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Functions().TypeParamAt(function, 0); !ok || got != term(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("function type parameter = (%v, %v)", got, ok)
	}
	if count, ok := contracts.Functions().ReturnCount(function); !ok || count != 2 {
		t.Fatalf("function return count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Functions().ReturnAt(function, 0); !ok || got != primitive(1) {
		t.Fatalf("function return = (%v, %v)", got, ok)
	}
	if count, ok := contracts.Calls().TypeArgumentCount(call); !ok || count != 2 {
		t.Fatalf("call argument count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Calls().TypeArgumentAt(call, 1); !ok || got != primitive(4) {
		t.Fatalf("call argument = (%v, %v)", got, ok)
	}
	id, idOK := contracts.Calls().TypeArgumentID(call)
	if !idOK || !id.Available() {
		t.Fatal("call type-argument identity unavailable")
	}
	if replay, replayOK := contracts.Calls().TypeArgumentID(call); !replayOK || replay != id {
		t.Fatal("call type-argument identity was not stable")
	}
}

// TestContractsPreserveOmittedAndKnownEmptyReturns proves an empty row is
// meaningful: an omitted return clause and an explicit known-empty one are
// distinct authored facts on one dense Function identity.
func TestContractsPreserveOmittedAndKnownEmptyReturns(t *testing.T) {
	for _, known := range []bool{false, true} {
		input := ledgerInput()
		input.Function[0].ReturnsKnown = known
		input.Function[0].Returns = nil
		input.Call[0].TypeArguments = nil
		contracts := sealTable(t, input).View()
		got, ok := contracts.Functions().Get(term(keyspace.FamilyFunction, 1))
		if !ok || got != known {
			t.Fatalf("ReturnsKnown = (%v, %v), want (%v, true)", got, ok, known)
		}
		if count, ok := contracts.Calls().TypeArgumentCount(term(keyspace.FamilyCall, 1)); !ok || count != 0 {
			t.Fatalf("empty call type arguments = (%d, %v)", count, ok)
		}
	}
}

// TestContractsRejectInvalidRows proves the admissions this vertical owns.
// TypeParam ownership and the containment forest span three verticals and
// belong to the enclosing owner's joint laws, not here.
func TestContractsRejectInvalidRows(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"omitted returns with child", func(in *Input) { in.Function[0].ReturnsKnown = false }},
		{"foreign function return", func(in *Input) { in.Function[0].Returns[0] = primitive(9) }},
		{"non-node function return", func(in *Input) {
			in.Function[0].Returns[0] = term(keyspace.FamilyCall, 1)
		}},
		{"foreign call type argument", func(in *Input) { in.Call[0].TypeArguments[0] = primitive(9) }},
		{"non-node call type argument", func(in *Input) {
			in.Call[0].TypeArguments[0] = term(keyspace.FamilyFunction, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput()
			test.edit(&input)
			if _, err := Build(input, ledgerCounts()); err == nil {
				t.Fatal("Build() accepted an invalid contract relation")
			}
		})
	}
}

// TestContractsCopyFencesBoundsAndQueriesDoNotAllocate proves the seal takes a
// copy, every column read is total, and the hot queries allocate nothing.
func TestContractsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := ledgerInput()
	table := sealTable(t, input)
	input.Function[0].TypeParams[0] = 0
	input.Function[0].Returns[0] = 0
	input.Call[0].TypeArguments[0] = 0

	contracts := table.View()
	function := term(keyspace.FamilyFunction, 1)
	call := term(keyspace.FamilyCall, 1)
	if got, ok := contracts.Functions().TypeParamAt(function, 0); !ok || got == 0 {
		t.Fatalf("type parameter copy fence = (%v, %v)", got, ok)
	}
	if got, ok := contracts.Functions().ReturnAt(function, 0); !ok || got == 0 {
		t.Fatalf("return copy fence = (%v, %v)", got, ok)
	}
	if got, ok := contracts.Calls().TypeArgumentAt(call, 0); !ok || got == 0 {
		t.Fatalf("type argument copy fence = (%v, %v)", got, ok)
	}
	if _, ok := contracts.Functions().ReturnAt(function, -1); ok {
		t.Fatal("ReturnAt accepted negative index")
	}
	if _, ok := contracts.Calls().TypeArgumentAt(call, 2); ok {
		t.Fatal("TypeArgumentAt accepted out-of-range index")
	}
	if _, ok := contracts.Functions().Get(term(keyspace.FamilyFunction, 9)); ok {
		t.Fatal("Functions.Get accepted unknown term")
	}
	if _, ok := contracts.Functions().Get(term(keyspace.FamilyCall, 1)); ok {
		t.Fatal("Functions.Get accepted foreign family")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		contracts.Functions().Get(function)
		contracts.Functions().TypeParamAt(function, 0)
		contracts.Functions().ReturnAt(function, 0)
		contracts.Calls().TypeArgumentAt(call, 0)
	}); allocations != 0 {
		t.Fatalf("contract queries allocated %.2f times", allocations)
	}
}

// TestDecoderRetainsFunctionAndCallRows proves the decoded rows map each wire
// field back to the relation it names.
func TestDecoderRetainsFunctionAndCallRows(t *testing.T) {
	decoded, err := Decode(sectionReader(t, sectionBytes(t, ledgerInput())))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.Function) != 2 || len(decoded.Call) != 2 {
		t.Fatalf("decoded counts = (%d, %d), want (2, 2)", len(decoded.Function), len(decoded.Call))
	}
	first := decoded.Function[0]
	if !first.ReturnsKnown || len(first.TypeParams) != 2 || first.TypeParams[1] != term(keyspace.FamilyTypeParam, 2) ||
		len(first.Returns) != 2 || first.Returns[1] != primitive(2) {
		t.Fatalf("decoded function contract = %+v", first)
	}
	if decoded.Function[1].ReturnsKnown || len(decoded.Function[1].Returns) != 0 {
		t.Fatalf("decoded omitted-return contract = %+v", decoded.Function[1])
	}
	if len(decoded.Call[0].TypeArguments) != 2 || decoded.Call[0].TypeArguments[0] != primitive(3) {
		t.Fatalf("decoded call contract = %+v", decoded.Call[0])
	}
	if len(decoded.Call[1].TypeArguments) != 0 {
		t.Fatalf("decoded empty call contract = %+v", decoded.Call[1])
	}
}

// TestZeroViewsFailClosed proves an unavailable view answers nothing.
func TestZeroViewsFailClosed(t *testing.T) {
	var view View
	function := term(keyspace.FamilyFunction, 1)
	call := term(keyspace.FamilyCall, 1)
	if view.Available() || view.Functions().Count() != 0 || view.Calls().Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, ok := view.Functions().At(0); ok {
		t.Fatal("zero View minted a function term")
	}
	if _, ok := view.Functions().Get(function); ok {
		t.Fatal("zero View returned a function row")
	}
	if _, ok := view.Functions().TypeParamCount(function); ok {
		t.Fatal("zero View counted type parameters")
	}
	if _, ok := view.Calls().TypeArgumentCount(call); ok {
		t.Fatal("zero View counted type arguments")
	}
	if id, ok := view.Calls().TypeArgumentID(call); ok || id.Available() {
		t.Fatal("zero View returned a call identity")
	}
}
