package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestImplicitReadIsSparseOccurrenceEvidence(t *testing.T) {
	b, entry := entryBuilder(t)
	global := b.Global(program.Span{File: "implicit.lua", StartLine: 1, StartCol: 1}, "unbound")
	explicit := b.Read(program.Span{File: "implicit.lua", StartLine: 2, StartCol: 1}, entry, global)
	firstSpan := program.Span{File: "implicit.lua", StartLine: 3, StartCol: 2}
	first := b.ImplicitRead(firstSpan, entry, global)
	ordinary := b.Read(program.Span{File: "implicit.lua", StartLine: 4, StartCol: 3}, entry, global)
	second := b.ImplicitRead(program.Span{File: "implicit.lua", StartLine: 5, StartCol: 4}, entry, global)
	values := b.Values(program.Span{}, entry, []program.Term{explicit, first, ordinary, second}, 0)
	finishAtTail(t, b, entry, b.Return(program.Span{}, entry, values))

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.ImplicitReadCount(); got != 2 {
		t.Fatalf("ImplicitReadCount = %d, want 2", got)
	}
	for index, want := range [...]program.Term{first, second} {
		got, ok := p.ImplicitReadAt(index)
		if !ok || got != want {
			t.Fatalf("ImplicitReadAt(%d) = %v, %v; want %v", index, got, ok, want)
		}
		owner, source, ok := p.Read(got)
		if !ok || owner != entry || source != global {
			t.Fatalf("implicit Read = %v, %v, %v", owner, source, ok)
		}
	}
	if got, ok := p.ImplicitReadAt(2); ok || got != 0 {
		t.Fatalf("ImplicitReadAt out of range = %v, %v", got, ok)
	}
	if got, ok := p.Span(first); !ok || got != firstSpan {
		t.Fatalf("implicit Read Span = %#v, %v", got, ok)
	}
	if explicit == first || ordinary == second {
		t.Fatal("implicit evidence did not mint ordinary distinct Read occurrences")
	}
}

func TestImplicitReadFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		build func(*program.Builder, program.Term) program.Term
	}{
		{
			name: "lexical Cell",
			build: func(b *program.Builder, entry program.Term) program.Term {
				return b.ImplicitRead(program.Span{}, entry, b.Cell(program.Span{}, entry))
			},
		},
		{
			name: "Lens",
			build: func(b *program.Builder, entry program.Term) program.Term {
				base := b.Table(program.Span{}, entry, nil, nil, nil)
				key := b.String(program.Span{}, entry, "x")
				return b.ImplicitRead(program.Span{}, entry, b.LensExact(program.Span{}, entry, base, key, program.FieldExact))
			},
		},
		{
			name: "non Body owner",
			build: func(b *program.Builder, entry program.Term) program.Term {
				return b.ImplicitRead(program.Span{}, b.Global(program.Span{}, "x"), b.Global(program.Span{}, "y"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, entry := entryBuilder(t)
			if got := test.build(b, entry); got != 0 {
				t.Fatalf("ImplicitRead = %v", got)
			}
			finishAtTail(t, b, entry)
			if _, err := b.Seal(); err == nil {
				t.Fatal("invalid implicit-global evidence was accepted")
			}
		})
	}
}

func TestImplicitReadSparseOrderAndSnapshotIsolation(t *testing.T) {
	b, entry := entryBuilder(t)
	firstGlobal := b.Global(program.Span{}, "first")
	first := b.ImplicitRead(program.Span{}, entry, firstGlobal)
	secondGlobal := b.Global(program.Span{}, "second")
	ordinary := b.Read(program.Span{}, entry, secondGlobal)
	second := b.ImplicitRead(program.Span{}, entry, secondGlobal)
	values := b.Values(program.Span{}, entry, []program.Term{first, ordinary, second}, 0)
	finishAtTail(t, b, entry, b.Return(program.Span{}, entry, values))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range [...]program.Term{first, second} {
		got, ok := p.ImplicitReadAt(index)
		if !ok || got != want {
			t.Fatalf("source evidence order at %d = %v, %v; want %v", index, got, ok, want)
		}
	}
	if late := b.ImplicitRead(program.Span{}, entry, firstGlobal); late == 0 {
		t.Fatal("post-Seal implicit read")
	}
	if got := p.ImplicitReadCount(); got != 2 {
		t.Fatalf("sealed implicit evidence aliased Builder: %d", got)
	}
}

func TestImplicitReadQueriesDoNotAllocate(t *testing.T) {
	b, entry := entryBuilder(t)
	global := b.Global(program.Span{}, "x")
	read := b.ImplicitRead(program.Span{}, entry, global)
	values := b.Values(program.Span{}, entry, []program.Term{read}, 0)
	finishAtTail(t, b, entry, b.Return(program.Span{}, entry, values))
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		queryInt = p.ImplicitReadCount()
		queryTerm, queryOK = p.ImplicitReadAt(0)
	})
	if allocations != 0 {
		t.Fatalf("implicit-global queries allocated: %g", allocations)
	}
}

func TestImplicitReadSealStorageIsLinear(t *testing.T) {
	sealCost := func(count int) (int64, int64) {
		b, entry := entryBuilder(t)
		global := b.Global(program.Span{}, "x")
		reads := make([]program.Term, 0, count)
		for i := 0; i < count; i++ {
			read := b.ImplicitRead(program.Span{}, entry, global)
			if read == 0 {
				t.Fatalf("ImplicitRead %d", i)
			}
			reads = append(reads, read)
		}
		values := b.Values(program.Span{}, entry, reads, 0)
		finishAtTail(t, b, entry, b.Return(program.Span{}, entry, values))
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
		t.Fatalf("implicit-global Seal allocations scaled: %d -> %d", smallAllocs, largeAllocs)
	}
	const scale = largeCount / smallCount
	if largeBytes > smallBytes*scale+64<<10 {
		t.Fatalf("implicit-global Seal bytes are superlinear: %d -> %d", smallBytes, largeBytes)
	}
}
