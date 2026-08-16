package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func openPendingDeepFixture(t *testing.T, depth int) *pendingFixture {
	t.Helper()
	if depth < 2 {
		t.Fatal("deep fixture requires at least two Unary rows")
	}
	body := pendingTerm(keyspace.FamilyBody, 1)
	returnCounts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 2, keyspace.FamilyValues: 1,
		keyspace.FamilyUnary: uint32(depth + 1), keyspace.FamilyReturn: 1,
	}
	unaries := make([]authored.Unary, depth+1)
	for index := range unaries {
		ordinal := uint32(index + 1)
		operand := pendingTerm(keyspace.FamilyNil, 1)
		if ordinal > 1 && ordinal <= uint32(depth) {
			operand = pendingTerm(keyspace.FamilyUnary, ordinal-1)
		} else if ordinal == uint32(depth+1) {
			operand = pendingTerm(keyspace.FamilyNil, 2)
		}
		unaries[index] = authored.Unary{Owner: body, Op: kind.UnaryNeg, Operand: operand}
	}
	return openPendingFixture(t, "pending-deep.lua", returnCounts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}},
				Terms: []keyspace.Term{pendingTerm(keyspace.FamilyUnary, uint32(depth)), pendingTerm(keyspace.FamilyUnary, uint32(depth+1))},
			},
			Operators: authored.OperatorsInput{Unaries: unaries},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: pendingTerm(keyspace.FamilyValues, 1)}}},
		}, nil, nil, nil, pendingSourceExtras{})
}

func openPendingWideFixture(t *testing.T, width int) *pendingFixture {
	t.Helper()
	if width < 2 {
		t.Fatal("wide fixture requires at least two payload terms")
	}
	body := pendingTerm(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: uint32(width + 2),
		keyspace.FamilyValues: 1, keyspace.FamilyBinary: 1, keyspace.FamilyReturn: 1,
	}
	terms := make([]keyspace.Term, width+1)
	for index := 0; index < width; index++ {
		terms[index] = pendingTerm(keyspace.FamilyNil, uint32(index+1))
	}
	terms[width] = pendingTerm(keyspace.FamilyBinary, 1)
	return openPendingFixture(t, "pending-wide.lua", counts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: uint32(len(terms))}}},
				Terms: terms,
			},
			Operators: authored.OperatorsInput{Binaries: []authored.Binary{{
				Owner: body, Op: kind.BinaryAdd,
				Left: pendingTerm(keyspace.FamilyNil, uint32(width+1)), Right: pendingTerm(keyspace.FamilyNil, uint32(width+2)),
			}}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: pendingTerm(keyspace.FamilyValues, 1)}}},
		}, nil, nil, nil, pendingSourceExtras{})
}

func TestSealPendingProductionDeepAndWideFixturesRemainQueryable(t *testing.T) {
	const deepDepth = 4096
	deep := openPendingDeepFixture(t, deepDepth)
	deepSubject := pendingTerm(keyspace.FamilyUnary, deepDepth+1)
	deepPrefixCount, deepOK := deep.pending.Count(deepSubject)
	if !deepOK || deepPrefixCount != 1 {
		t.Fatalf("deep final Unary pending = %d/%v, want one retained deep predecessor", deepPrefixCount, deepOK)
	}
	if got, ok := deep.pending.At(deepSubject, 0); !ok || got != pendingTerm(keyspace.FamilyUnary, deepDepth) {
		t.Fatalf("deep final Unary At(0) = %v/%v, want Unary%d/true", got, ok, deepDepth)
	}
	if !deep.executable.Executable(pendingTerm(keyspace.FamilyUnary, deepDepth)) {
		t.Fatal("deep executable closure did not reach the innermost authored Unary")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if count, ok := deep.pending.Count(deepSubject); !ok || count != 1 {
			t.Fatal("deep Count changed during allocation probe")
		}
		if _, ok := deep.pending.At(deepSubject, 0); !ok {
			t.Fatal("deep At changed during allocation probe")
		}
	}); allocations != 0 {
		t.Fatalf("deep Pending queries allocated %v objects per run", allocations)
	}

	const width = 2048
	wide := openPendingWideFixture(t, width)
	wideSubject := pendingTerm(keyspace.FamilyBinary, 1)
	wideCount, wideOK := wide.pending.Count(wideSubject)
	if !wideOK || wideCount != width {
		t.Fatalf("wide Binary pending = %d/%v, want %d retained payloads", wideCount, wideOK, width)
	}
	for _, index := range []int{0, width / 2, width - 1} {
		want := pendingTerm(keyspace.FamilyNil, uint32(index+1))
		if got, ok := wide.pending.At(wideSubject, index); !ok || got != want {
			t.Fatalf("wide Binary At(%d) = %v/%v, want Nil%d/true", index, got, ok, index+1)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if count, ok := wide.pending.Count(wideSubject); !ok || count != width {
			t.Fatal("wide Count changed during allocation probe")
		}
		if _, ok := wide.pending.At(wideSubject, width/2); !ok {
			t.Fatal("wide At changed during allocation probe")
		}
	}); allocations != 0 {
		t.Fatalf("wide Pending queries allocated %v objects per run", allocations)
	}
}
