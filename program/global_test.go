package program_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestGlobalIsCanonicalCellStorage(t *testing.T) {
	b, entry := entryBuilder(t)
	firstSpan := program.Span{File: "global.lua", StartLine: 1, StartCol: 1}
	global := b.Global(firstSpan, "x")
	if global == 0 {
		t.Fatal("Global")
	}
	if duplicate := b.Global(program.Span{File: "global.lua", StartLine: 2, StartCol: 3}, "x"); duplicate != global {
		t.Fatalf("duplicate Global = %v, want %v", duplicate, global)
	}
	other := b.Global(program.Span{}, "X")
	if other == 0 || other == global {
		t.Fatalf("distinct Global = %v, first = %v", other, global)
	}

	name := b.Name(program.Span{}, entry, "x")
	fieldValue := b.Integer(program.Span{}, entry, 1)
	fieldValues := b.Values(program.Span{}, entry, []program.Term{fieldValue}, 0)
	namedTable := b.Table(
		program.Span{},
		entry,
		[]program.Term{name},
		[]program.Term{fieldValues},
		[]program.FieldKind{program.FieldName},
	)
	base := b.Table(program.Span{}, entry, nil, nil, nil)
	exactText := b.String(program.Span{}, entry, "x")
	lens := b.LensExact(program.Span{}, entry, base, exactText, program.FieldExact)
	lensRead := b.Read(program.Span{}, entry, lens)
	globalRead := b.Read(program.Span{}, entry, global)
	returnValues := b.Values(program.Span{}, entry, []program.Term{globalRead, namedTable, lensRead}, 0)
	result := b.Return(program.Span{}, entry, returnValues)
	assignedValue := b.Integer(program.Span{}, entry, 2)
	assignedValues := b.Values(program.Span{}, entry, []program.Term{assignedValue}, 0)
	assign := b.Assign(program.Span{}, entry, []program.Term{global}, assignedValues)
	finishAtTail(t, b, entry, assign, result)

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	gotName, globalKey, ok := p.Global(global)
	if !ok || gotName != "x" || globalKey == 0 {
		t.Fatalf("Global = %q, %v, %v", gotName, globalKey, ok)
	}
	otherName, otherKey, ok := p.Global(other)
	if !ok || otherName != "X" || otherKey == 0 || otherKey == globalKey {
		t.Fatalf("case-distinct Global = %q, %v, %v", otherName, otherKey, ok)
	}
	if owner, ok := p.Cell(global); !ok || owner != 0 {
		t.Fatalf("global Cell owner = %v, %v", owner, ok)
	}
	if got, ok := p.Span(global); !ok || got != firstSpan {
		t.Fatalf("Global Span = %#v, %v", got, ok)
	}
	if count := p.CellCount(); count != 2 {
		t.Fatalf("CellCount = %d", count)
	}
	if count := p.GlobalCount(); count != 2 {
		t.Fatalf("GlobalCount = %d", count)
	}
	if got, ok := p.CellAt(0); !ok || got != global {
		t.Fatalf("CellAt(0) = %v, %v", got, ok)
	}
	if got, ok := p.GlobalAt(0); !ok || got != global {
		t.Fatalf("GlobalAt(0) = %v, %v", got, ok)
	}
	if got, ok := p.GlobalAt(1); !ok || got != other {
		t.Fatalf("GlobalAt(1) = %v, %v", got, ok)
	}
	if got, ok := p.GlobalAt(2); ok || got != 0 {
		t.Fatalf("GlobalAt(2) = %v, %v", got, ok)
	}
	_, _, _, fieldKey, ok := p.Field(namedTable, 0)
	if !ok || fieldKey != globalKey {
		t.Fatalf("name field Key = %v, want %v", fieldKey, globalKey)
	}
	_, _, _, _, lensKey, ok := p.Lens(lens)
	if !ok || lensKey != globalKey {
		t.Fatalf("exact string Lens Key = %v, want %v", lensKey, globalKey)
	}
	if owner, source, ok := p.Read(globalRead); !ok || owner != entry || source != global {
		t.Fatalf("Read = %v, %v, %v", owner, source, ok)
	}
	if got, ok := p.Target(assign, 0); !ok || got != global {
		t.Fatalf("Target = %v, %v", got, ok)
	}
	if _, _, ok := p.Global(entry); ok {
		t.Fatal("Body selected Global relation")
	}
}

func TestGlobalCellFailsClosed(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		b, entry := entryBuilder(t)
		if got := b.Global(program.Span{}, ""); got != 0 {
			t.Fatalf("empty Global = %v", got)
		}
		finishAtTail(t, b, entry)
		if _, err := b.Seal(); err == nil {
			t.Fatal("empty Global did not poison Builder")
		}
	})

	t.Run("duplicate cannot bypass span validation", func(t *testing.T) {
		b, entry := entryBuilder(t)
		if got := b.Global(program.Span{}, "x"); got == 0 {
			t.Fatal("valid Global")
		}
		if got := b.Global(program.Span{StartLine: 1}, "x"); got != 0 {
			t.Fatalf("invalid duplicate Global = %v", got)
		}
		finishAtTail(t, b, entry)
		if _, err := b.Seal(); err == nil {
			t.Fatal("invalid duplicate span did not poison Builder")
		}
	})

	t.Run("duplicate cannot bypass prior poison", func(t *testing.T) {
		b, entry := entryBuilder(t)
		if got := b.Global(program.Span{}, "x"); got == 0 {
			t.Fatal("valid Global")
		}
		if got := b.Values(program.Span{}, entry, []program.Term{entry}, 0); got != 0 {
			t.Fatalf("invalid Values = %v", got)
		}
		if got := b.Global(program.Span{}, "x"); got != 0 {
			t.Fatalf("poisoned duplicate Global = %v", got)
		}
		finishAtTail(t, b, entry)
		if _, err := b.Seal(); err == nil {
			t.Fatal("prior poison was accepted")
		}
	})

	t.Run("Cell is storage not a value", func(t *testing.T) {
		b, entry := entryBuilder(t)
		global := b.Global(program.Span{}, "x")
		if got := b.Values(program.Span{}, entry, []program.Term{global}, 0); got != 0 {
			t.Fatalf("Values accepted Cell storage %v", got)
		}
		finishAtTail(t, b, entry)
		if _, err := b.Seal(); err == nil {
			t.Fatal("Cell value position did not poison Builder")
		}
	})

	t.Run("Global Cell cannot take a lexical role", func(t *testing.T) {
		b, entry := entryBuilder(t)
		global := b.Global(program.Span{}, "x")
		values := b.Values(program.Span{}, entry, nil, 0)
		if got := b.Bind(program.Span{}, entry, []program.Term{global}, values); got != 0 {
			t.Fatalf("Bind accepted Global Cell %v", got)
		}
		finishAtTail(t, b, entry)
		if _, err := b.Seal(); err == nil {
			t.Fatal("Global lexical role did not poison Builder")
		}
	})
}

func TestGlobalCellSparseEnumerationInterleavesLexicalCells(t *testing.T) {
	b, entry := entryBuilder(t)
	firstLocal := b.Cell(program.Span{}, entry)
	firstGlobal := b.Global(program.Span{}, "first")
	secondLocal := b.Cell(program.Span{}, entry)
	secondGlobal := b.Global(program.Span{}, "second")
	values := b.Values(program.Span{}, entry, nil, 0)
	bind := b.Bind(program.Span{}, entry, []program.Term{firstLocal, secondLocal}, values)
	finishAtTail(t, b, entry, bind)

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	wantCells := [...]program.Term{firstLocal, firstGlobal, secondLocal, secondGlobal}
	for index, want := range wantCells {
		if got, ok := p.CellAt(index); !ok || got != want {
			t.Fatalf("CellAt(%d) = %v, %v; want %v", index, got, ok, want)
		}
	}
	wantGlobals := [...]struct {
		cell program.Term
		name string
	}{
		{cell: firstGlobal, name: "first"},
		{cell: secondGlobal, name: "second"},
	}
	var priorKey program.Key
	for index, want := range wantGlobals {
		cell, ok := p.GlobalAt(index)
		if !ok || cell != want.cell {
			t.Fatalf("GlobalAt(%d) = %v, %v; want %v", index, cell, ok, want.cell)
		}
		name, key, ok := p.Global(cell)
		if !ok || name != want.name || key == 0 || key == priorKey {
			t.Fatalf("Global(%d) = %q, %v, %v", index, name, key, ok)
		}
		priorKey = key
	}
	if owner, ok := p.Cell(firstLocal); !ok || owner != entry {
		t.Fatalf("lexical Cell = %v, %v", owner, ok)
	}
	if owner, ok := p.Cell(firstGlobal); !ok || owner != 0 {
		t.Fatalf("global Cell = %v, %v", owner, ok)
	}
}

func TestGlobalCellCallNeverDerivesDirectEvidence(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	global := b.Global(program.Span{}, "callback")
	function := b.DeclareFunction(program.Span{}, entry)
	if !b.FillFunction(function, body, nil, 0, nil) {
		t.Fatal("FillFunction")
	}
	recursiveCallee := b.Read(program.Span{}, body, global)
	recursiveActuals := b.Values(program.Span{}, body, nil, 0)
	recursiveCall := b.Call(program.Span{}, body, recursiveCallee, 0, recursiveActuals)
	bodyValues := b.Values(program.Span{}, body, nil, recursiveCall)
	finishAtTail(t, b, body, b.Return(program.Span{}, body, bodyValues))

	assignedValues := b.Values(program.Span{}, entry, []program.Term{function}, 0)
	assign := b.Assign(program.Span{}, entry, []program.Term{global}, assignedValues)
	finishAtTail(t, b, entry, assign)

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, direct, ok := p.Call(recursiveCall)
	if !ok || direct != 0 {
		t.Fatalf("recursive Global Call direct = %v, %v", direct, ok)
	}
	if head, ok := p.Mu(function); ok || head != 0 {
		t.Fatalf("Global-bound Function Mu = %v, %v", head, ok)
	}
}

func TestGlobalCellDeterminismAndSnapshotIsolation(t *testing.T) {
	build := func() (*program.Builder, *program.Program, program.Term, program.Key) {
		t.Helper()
		b, entry := entryBuilder(t)
		first := b.Global(program.Span{}, "canonical_name")
		if duplicate := b.Global(program.Span{}, "canonical_name"); duplicate != first {
			t.Fatalf("duplicate = %v, first = %v", duplicate, first)
		}
		finishAtTail(t, b, entry)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		_, key, ok := p.Global(first)
		if !ok {
			t.Fatal("Global query")
		}
		return b, p, first, key
	}

	leftBuilder, left, leftGlobal, leftKey := build()
	_, right, rightGlobal, rightKey := build()
	if leftGlobal != rightGlobal || leftKey != rightKey || left.TermCount() != right.TermCount() {
		t.Fatal("Global Cell minting is not deterministic")
	}
	if got := leftBuilder.Global(program.Span{}, "later"); got == 0 {
		t.Fatal("post-Seal Global")
	}
	if left.CellCount() != 1 || left.GlobalCount() != 1 ||
		right.CellCount() != 1 || right.GlobalCount() != 1 {
		t.Fatalf(
			"sealed counts changed: left=%d/%d right=%d/%d",
			left.CellCount(), left.GlobalCount(), right.CellCount(), right.GlobalCount(),
		)
	}
}

func TestGlobalCellQueriesDoNotAllocate(t *testing.T) {
	b, entry := entryBuilder(t)
	global := b.Global(program.Span{}, "x")
	finishAtTail(t, b, entry)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}

	allocations := testing.AllocsPerRun(1000, func() {
		queryString, queryKey, queryOK = p.Global(global)
		queryTerm, queryOK = p.Cell(global)
		queryInt = p.GlobalCount()
		queryTerm, queryOK = p.GlobalAt(0)
	})
	if allocations != 0 {
		t.Fatalf("Global Cell queries allocated: %g", allocations)
	}
}

func TestGlobalCellSealStorageIsLinear(t *testing.T) {
	sealCost := func(count int) (int64, int64) {
		b, entry := entryBuilder(t)
		for i := 0; i < count; i++ {
			if term := b.Global(program.Span{}, fmt.Sprintf("global_%d", i)); term == 0 {
				t.Fatalf("Global %d", i)
			}
		}
		finishAtTail(t, b, entry)
		result := testing.Benchmark(func(benchmark *testing.B) {
			for i := 0; i < benchmark.N; i++ {
				if _, err := b.Seal(); err != nil {
					benchmark.Fatal(err)
				}
			}
		})
		return result.AllocsPerOp(), result.AllocedBytesPerOp()
	}

	const smallCount = 128
	const largeCount = 1024
	smallAllocs, smallBytes := sealCost(smallCount)
	largeAllocs, largeBytes := sealCost(largeCount)
	if largeAllocs > smallAllocs+2 {
		t.Fatalf("Global Cell Seal allocations scaled: %d -> %d", smallAllocs, largeAllocs)
	}
	const scale = largeCount / smallCount
	if largeBytes > smallBytes*scale+64<<10 {
		t.Fatalf("Global Cell Seal bytes are superlinear: %d -> %d", smallBytes, largeBytes)
	}
}
