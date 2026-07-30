package program_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestLabelAndGotoAreTypedSourcePositions(t *testing.T) {
	b, entry := entryBuilder(t)
	first := b.Label(program.Span{}, entry)
	second := b.Label(program.Span{}, entry)
	if !b.SetLabelCursor(first, 0) || !b.SetLabelCursor(second, 0) {
		t.Fatal("place Labels")
	}
	jump := b.Goto(program.Span{}, entry, second)
	if jump == 0 || !b.SetBody(entry, jump) {
		t.Fatal("build Goto")
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []program.Term{first, second} {
		owner, cursor, ok := p.Label(label)
		if !ok || owner != entry || cursor != 0 {
			t.Fatalf("Label(%v) = %v, %d, %v", label, owner, cursor, ok)
		}
	}
	owner, target, ok := p.Goto(jump)
	if !ok || owner != entry || target != second {
		t.Fatalf("Goto = %v, %v, %v", owner, target, ok)
	}
	// Both labels anchor the same cyclic cursor. The earliest existing Label is
	// the one canonical Mu head; there is no duplicate point or Mu Term.
	for _, term := range []program.Term{entry, first, second, jump} {
		head, ok := p.Mu(term)
		if !ok || head != first {
			t.Fatalf("Mu(%v) = %v, %v; want %v", term, head, ok, first)
		}
	}
}

func TestForwardGotoReachesImplicitBodyTailWithoutOutcomeRow(t *testing.T) {
	b, entry := entryBuilder(t)
	label := b.Label(program.Span{}, entry)
	if !b.SetLabelCursor(label, 2) {
		t.Fatal("place Label")
	}
	jump := b.Goto(program.Span{}, entry, label)
	values := b.Values(program.Span{}, entry, nil, 0)
	deadReturn := b.Return(program.Span{}, entry, values)
	b.SetBody(entry, jump, deadReturn)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []program.Term{label, jump, deadReturn} {
		if head, ok := p.Mu(term); ok || head != 0 {
			t.Fatalf("acyclic Mu(%v) = %v, %v", term, head, ok)
		}
	}
}

func TestDeadBackwardGotoDoesNotCreateMu(t *testing.T) {
	b, entry := entryBuilder(t)
	values := b.Values(program.Span{}, entry, nil, 0)
	returned := b.Return(program.Span{}, entry, values)
	label := b.Label(program.Span{}, entry)
	b.SetLabelCursor(label, 1)
	jump := b.Goto(program.Span{}, entry, label)
	b.SetBody(entry, returned, jump)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []program.Term{label, jump} {
		if head, ok := p.Mu(term); ok || head != 0 {
			t.Fatalf("dead cycle Mu(%v) = %v, %v", term, head, ok)
		}
	}
}

func TestMixedLoopGotoSCCUsesOneCanonicalHead(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	label := b.Label(program.Span{}, entry)
	b.SetLabelCursor(label, 0)
	jump := b.Goto(program.Span{}, body, label)
	b.SetBody(body, jump)
	condition := b.Bool(program.Span{}, entry, true)
	loop := b.Loop(program.Span{}, entry, body, condition, nil, program.LoopWhile)
	b.SetBody(entry, loop)
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []program.Term{entry, label, loop, jump} {
		head, ok := p.Mu(term)
		if !ok || head != label {
			t.Fatalf("Mu(%v) = %v, %v; want %v", term, head, ok, label)
		}
	}
}

func TestGotoCannotEnterLiveLocal(t *testing.T) {
	b, entry := entryBuilder(t)
	label := b.Label(program.Span{}, entry)
	b.SetLabelCursor(label, 2)
	jump := b.Goto(program.Span{}, entry, label)
	cell := b.Cell(program.Span{}, entry)
	initial := b.Values(program.Span{}, entry, nil, 0)
	bind := b.Bind(program.Span{}, entry, []program.Term{cell}, initial)
	assigned := b.Values(program.Span{}, entry, nil, 0)
	assign := b.Assign(program.Span{}, entry, []program.Term{cell}, assigned)
	b.SetBody(entry, jump, bind, assign)
	if _, err := b.Seal(); err == nil || !strings.Contains(err.Error(), "enters a live local scope") {
		t.Fatalf("Seal error = %v", err)
	}
}

func TestOrdinaryEndLabelIsOutsideBodyLocals(t *testing.T) {
	b, entry := entryBuilder(t)
	label := b.Label(program.Span{}, entry)
	b.SetLabelCursor(label, 2)
	jump := b.Goto(program.Span{}, entry, label)
	cell := b.Cell(program.Span{}, entry)
	initial := b.Values(program.Span{}, entry, nil, 0)
	bind := b.Bind(program.Span{}, entry, []program.Term{cell}, initial)
	b.SetBody(entry, jump, bind)
	if _, err := b.Seal(); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatEndLabelRetainsLocalsThroughUntil(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	label := b.Label(program.Span{}, body)
	b.SetLabelCursor(label, 2)
	jump := b.Goto(program.Span{}, body, label)
	cell := b.Cell(program.Span{}, body)
	initial := b.Values(program.Span{}, body, nil, 0)
	bind := b.Bind(program.Span{}, body, []program.Term{cell}, initial)
	b.SetBody(body, jump, bind)
	condition := b.Bool(program.Span{}, body, false)
	loop := b.Loop(program.Span{}, entry, body, condition, nil, program.LoopRepeat)
	b.SetBody(entry, loop)
	if _, err := b.Seal(); err == nil || !strings.Contains(err.Error(), "enters a live local scope") {
		t.Fatalf("Seal error = %v", err)
	}
}

func TestLabelPlacementFailsClosed(t *testing.T) {
	t.Run("unplaced", func(t *testing.T) {
		b, entry := entryBuilder(t)
		b.Label(program.Span{}, entry)
		b.SetBody(entry)
		if _, err := b.Seal(); err == nil || !strings.Contains(err.Error(), "unplaced Label") {
			t.Fatalf("Seal error = %v", err)
		}
	})
	t.Run("outside Body", func(t *testing.T) {
		b, entry := entryBuilder(t)
		label := b.Label(program.Span{}, entry)
		b.SetLabelCursor(label, 1)
		b.SetBody(entry)
		if _, err := b.Seal(); err == nil || !strings.Contains(err.Error(), "outside Body") {
			t.Fatalf("Seal error = %v", err)
		}
	})
	t.Run("one shot", func(t *testing.T) {
		b, entry := entryBuilder(t)
		label := b.Label(program.Span{}, entry)
		if !b.SetLabelCursor(label, 0) || b.SetLabelCursor(label, 0) {
			t.Fatal("SetLabelCursor one-shot law failed")
		}
		b.SetBody(entry)
		if _, err := b.Seal(); err == nil {
			t.Fatal("poisoned Builder sealed")
		}
	})
}

func TestGotoCannotEnterNestedBodyOrFunction(t *testing.T) {
	t.Run("nested Body", func(t *testing.T) {
		b, entry := entryBuilder(t)
		child := b.Body(program.Span{})
		label := b.Label(program.Span{}, child)
		b.SetLabelCursor(label, 0)
		jump := b.Goto(program.Span{}, entry, label)
		b.SetBody(child)
		b.SetBody(entry, jump, child)
		if _, err := b.Seal(); err == nil || !strings.Contains(err.Error(), "not lexically visible") {
			t.Fatalf("Seal error = %v", err)
		}
	})
	t.Run("Function", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		label := b.Label(program.Span{}, body)
		b.SetLabelCursor(label, 0)
		b.SetBody(body)
		function := b.Function(program.Span{}, entry, body, nil, 0, nil)
		functionValues := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		cell := b.Cell(program.Span{}, entry)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, functionValues)
		jump := b.Goto(program.Span{}, entry, label)
		b.SetBody(entry, jump, bind)
		if _, err := b.Seal(); err == nil || !strings.Contains(err.Error(), "not lexically visible") {
			t.Fatalf("Seal error = %v", err)
		}
	})
}

func TestDeepGotoControlIsIterative(t *testing.T) {
	const depth = 8 * 1024
	b, entry := entryBuilder(t)
	bodies := make([]program.Term, depth+1)
	bodies[0] = entry
	for i := 1; i <= depth; i++ {
		bodies[i] = b.Body(program.Span{})
	}
	label := b.Label(program.Span{}, entry)
	b.SetLabelCursor(label, 0)
	jump := b.Goto(program.Span{}, bodies[depth], label)
	b.SetBody(bodies[depth], jump)
	for i := depth - 1; i >= 0; i-- {
		b.SetBody(bodies[i], bodies[i+1])
	}
	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []program.Term{entry, bodies[1], bodies[depth], jump, label} {
		head, ok := p.Mu(term)
		if !ok || head != label {
			t.Fatalf("Mu(%v) = %v, %v; want %v", term, head, ok, label)
		}
	}
}

func TestWideGotoSCCIsDeterministicAndAllocationBounded(t *testing.T) {
	build := func(count int) (*program.Builder, []program.Term, []program.Term) {
		b, entry := entryBuilder(t)
		labels := make([]program.Term, count)
		for i := range labels {
			labels[i] = b.Label(program.Span{}, entry)
			if !b.SetLabelCursor(labels[i], i) {
				t.Fatal("place Label")
			}
		}
		jumps := make([]program.Term, count)
		for i := range jumps {
			jumps[i] = b.Goto(program.Span{}, entry, labels[(i+1)%count])
		}
		b.SetBody(entry, jumps...)
		return b, labels, jumps
	}

	first, firstLabels, firstJumps := build(4 * 1024)
	second, secondLabels, secondJumps := build(4 * 1024)
	left, err := first.Seal()
	if err != nil {
		t.Fatal(err)
	}
	right, err := second.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{0, 1, len(firstLabels) / 2, len(firstLabels) - 1} {
		if firstLabels[index] != secondLabels[index] || firstJumps[index] != secondJumps[index] {
			t.Fatalf("nondeterministic Terms at %d", index)
		}
		leftHead, leftOK := left.Mu(firstJumps[index])
		rightHead, rightOK := right.Mu(secondJumps[index])
		if !leftOK || !rightOK || leftHead != firstLabels[0] || rightHead != secondLabels[0] {
			t.Fatalf("Mu[%d] = %v/%v, %v/%v", index, leftHead, leftOK, rightHead, rightOK)
		}
	}

	sealAllocations := func(count int) float64 {
		builder, _, _ := build(count)
		return testing.AllocsPerRun(20, func() {
			if _, err := builder.Seal(); err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := sealAllocations(64), sealAllocations(512)
	if large > small+2 {
		t.Fatalf("Goto Seal allocations grew per relation: small=%g large=%g", small, large)
	}

	sealBytes := func(count int) (int64, int64) {
		builder, _, _ := build(count)
		result := testing.Benchmark(func(bench *testing.B) {
			for i := 0; i < bench.N; i++ {
				var err error
				queryProgram, err = builder.Seal()
				if err != nil {
					bench.Fatal(err)
				}
			}
		})
		return result.AllocsPerOp(), result.AllocedBytesPerOp()
	}
	const smallCount, largeCount = 1024, 2048
	smallAllocs, smallBytes := sealBytes(smallCount)
	largeAllocs, largeBytes := sealBytes(largeCount)
	if largeAllocs > smallAllocs+2 {
		t.Fatalf("Goto Seal allocation count scaled: %d -> %d", smallAllocs, largeAllocs)
	}
	const slack = int64(64 << 10)
	if largeBytes > 2*smallBytes+slack {
		t.Fatalf("Goto Seal bytes are superlinear: %d -> %d", smallBytes, largeBytes)
	}
	const maxBytesPerRelation = int64(512)
	if largeBytes > largeCount*maxBytesPerRelation {
		t.Fatalf("Goto Seal bytes = %d; linear bound = %d", largeBytes, largeCount*maxBytesPerRelation)
	}
}
