package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
)

func assertNoFunctionMu(t *testing.T, p *program.Program, functions ...program.Term) {
	t.Helper()
	for _, function := range functions {
		if head, ok := p.Mu(function); ok || head != 0 {
			t.Fatalf("Mu(%v) = %v, %v; want no direct-call recurrence", function, head, ok)
		}
	}
}

func TestDeadDirectSelfCallDoesNotCreateMu(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	outer := b.Cell(program.Span{}, entry)
	inner := b.Cell(program.Span{}, body)
	function := b.Function(
		program.Span{}, entry, body, nil, 0,
		[]program.Capture{{Inner: inner, Outer: outer}},
	)
	returnedValues := b.Values(program.Span{}, body, nil, 0)
	returned := b.Return(program.Span{}, body, returnedValues)
	callee := b.Read(program.Span{}, body, inner)
	actuals := b.Values(program.Span{}, body, nil, 0)
	call := b.Call(program.Span{}, body, callee, 0, actuals)
	finishAtTail(t, b, body, returned, call)
	bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
	finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	assertNoFunctionMu(t, p, function)
	_, _, _, _, direct, ok := p.Call(call)
	if !ok || direct != 0 {
		t.Fatalf("dead Call direct evidence = %v, %v; want none", direct, ok)
	}
}

func TestDeadMutualForwardCapturesAreRejected(t *testing.T) {
	b, entry := entryBuilder(t)
	bodies := []program.Term{b.Body(program.Span{}), b.Body(program.Span{})}
	outers := []program.Term{b.Cell(program.Span{}, entry), b.Cell(program.Span{}, entry)}
	inners := []program.Term{
		b.Cell(program.Span{}, bodies[0]),
		b.Cell(program.Span{}, bodies[1]),
	}
	functions := []program.Term{
		b.Function(program.Span{}, entry, bodies[0], nil, 0, []program.Capture{{Inner: inners[0], Outer: outers[1]}}),
		b.Function(program.Span{}, entry, bodies[1], nil, 0, []program.Capture{{Inner: inners[1], Outer: outers[0]}}),
	}
	for i := range functions {
		returnedValues := b.Values(program.Span{}, bodies[i], nil, 0)
		returned := b.Return(program.Span{}, bodies[i], returnedValues)
		callee := b.Read(program.Span{}, bodies[i], inners[i])
		actuals := b.Values(program.Span{}, bodies[i], nil, 0)
		call := b.Call(program.Span{}, bodies[i], callee, 0, actuals)
		finishAtTail(t, b, bodies[i], returned, call)
	}
	roots := make([]program.Term, len(functions))
	for i, function := range functions {
		bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		roots[i] = b.Bind(program.Span{}, entry, []program.Term{outers[i]}, bound)
	}
	finishAtTail(t, b, entry, roots...)

	if _, err := b.Seal(); err == nil {
		t.Fatal("dead control must not make forward Captures structurally valid")
	}
}

func TestGotoRevivesDirectSelfCall(t *testing.T) {
	b, entry := entryBuilder(t)
	body := b.Body(program.Span{})
	outer := b.Cell(program.Span{}, entry)
	inner := b.Cell(program.Span{}, body)
	function := b.Function(
		program.Span{}, entry, body, nil, 0,
		[]program.Capture{{Inner: inner, Outer: outer}},
	)
	label := b.Label(program.Span{}, body)
	if !b.SetLabelCursor(label, 2) {
		t.Fatal("place Label")
	}
	jump := b.Goto(program.Span{}, body, label)
	returnedValues := b.Values(program.Span{}, body, nil, 0)
	returned := b.Return(program.Span{}, body, returnedValues)
	callee := b.Read(program.Span{}, body, inner)
	actuals := b.Values(program.Span{}, body, nil, 0)
	call := b.Call(program.Span{}, body, callee, 0, actuals)
	finishAtTail(t, b, body, jump, returned, call)
	bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
	finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("Mu(%v) = %v, %v; want revived self edge", function, head, ok)
	}
}

func TestAssignmentStabilityUsesSourceReachability(t *testing.T) {
	build := func(t *testing.T, reviveAssign bool) (*program.Program, program.Term) {
		t.Helper()
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, body)
		function := b.Function(
			program.Span{}, entry, body, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		callee := b.Read(program.Span{}, body, inner)
		actuals := b.Values(program.Span{}, body, nil, 0)
		call := b.Call(program.Span{}, body, callee, 0, actuals)
		result := b.Values(program.Span{}, body, nil, call)
		finishAtTail(t, b, body, b.Return(program.Span{}, body, result))

		bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{outer}, bound)
		label := b.Label(program.Span{}, entry)
		jump := b.Goto(program.Span{}, entry, label)
		nilValue := b.Nil(program.Span{}, entry)
		replacement := b.Values(program.Span{}, entry, []program.Term{nilValue}, 0)
		assign := b.Assign(program.Span{}, entry, []program.Term{outer}, replacement)
		if reviveAssign {
			if !b.SetLabelCursor(label, 3) {
				t.Fatal("place revived Assign Label")
			}
			deadValues := b.Values(program.Span{}, entry, nil, 0)
			deadReturn := b.Return(program.Span{}, entry, deadValues)
			finishAtTail(t, b, entry, bind, jump, deadReturn, assign)
		} else {
			if !b.SetLabelCursor(label, 3) {
				t.Fatal("place end Label")
			}
			finishAtTail(t, b, entry, bind, jump, assign)
		}
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		return p, function
	}

	dead, deadFunction := build(t, false)
	if head, ok := dead.Mu(deadFunction); !ok || head != deadFunction {
		t.Fatalf("dead Assign erased direct recurrence: Mu = %v, %v", head, ok)
	}
	revived, revivedFunction := build(t, true)
	assertNoFunctionMu(t, revived, revivedFunction)
}

func TestRepeatConditionCallUsesDecisionReachability(t *testing.T) {
	build := func(t *testing.T, tailReachable bool) (*program.Program, program.Term) {
		t.Helper()
		b, entry := entryBuilder(t)
		functionBody := b.Body(program.Span{})
		repeatBody := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, functionBody)
		function := b.Function(
			program.Span{}, entry, functionBody, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		if tailReachable {
			finishAtTail(t, b, repeatBody)
		} else {
			values := b.Values(program.Span{}, repeatBody, nil, 0)
			finishAtTail(t, b, repeatBody, b.Return(program.Span{}, repeatBody, values))
		}
		callee := b.Read(program.Span{}, repeatBody, inner)
		actuals := b.Values(program.Span{}, repeatBody, nil, 0)
		condition := b.Call(program.Span{}, repeatBody, callee, 0, actuals)
		loop := b.Loop(
			program.Span{}, functionBody, repeatBody, condition, nil, program.LoopRepeat,
		)
		finishAtTail(t, b, functionBody, loop)
		bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		return p, function
	}

	dead, deadFunction := build(t, false)
	assertNoFunctionMu(t, dead, deadFunction)
	reachable, reachableFunction := build(t, true)
	if head, ok := reachable.Mu(reachableFunction); !ok || head != reachableFunction {
		t.Fatalf("reachable Repeat condition lost direct recurrence: Mu = %v, %v", head, ok)
	}
}

func TestDeepNestedDirectCallContainmentIsIterativeAndLinear(t *testing.T) {
	build := func(depth int) (*program.Builder, program.Term) {
		b := program.NewBuilder()
		entry := b.Body(program.Span{})
		b.SetEntry(entry)
		body := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, body)
		function := b.Function(
			program.Span{}, entry, body, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		callee := b.Read(program.Span{}, body, inner)
		actuals := b.Values(program.Span{}, body, nil, 0)
		value := b.Call(program.Span{}, body, callee, 0, actuals)
		for i := 0; i < depth; i++ {
			value = b.Unary(program.Span{}, body, program.UnaryNot, value)
		}
		values := b.Values(program.Span{}, body, []program.Term{value}, 0)
		b.SetBody(body, b.Return(program.Span{}, body, values))
		bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		b.SetBody(entry, b.Bind(program.Span{}, entry, []program.Term{outer}, bound))
		return b, function
	}

	deep, function := build(8 * 1024)
	p, err := deep.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if head, ok := p.Mu(function); !ok || head != function {
		t.Fatalf("deep direct-call Mu = %v, %v", head, ok)
	}

	sealAllocations := func(depth int) float64 {
		b, _ := build(depth)
		return testing.AllocsPerRun(20, func() {
			var err error
			queryProgram, err = b.Seal()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := sealAllocations(64), sealAllocations(512)
	if large > small+2 {
		t.Fatalf("nested containment allocations grew with depth: %g -> %g", small, large)
	}

	sealBytes := func(depth int) int64 {
		b, _ := build(depth)
		result := testing.Benchmark(func(bench *testing.B) {
			for i := 0; i < bench.N; i++ {
				var err error
				queryProgram, err = b.Seal()
				if err != nil {
					bench.Fatal(err)
				}
			}
		})
		return result.AllocedBytesPerOp()
	}
	smallBytes, largeBytes := sealBytes(1024), sealBytes(2048)
	const slack = int64(64 << 10)
	if largeBytes > 2*smallBytes+slack {
		t.Fatalf("nested containment bytes are superlinear: %d -> %d", smallBytes, largeBytes)
	}
}

func TestBodyTermMapsToItsOwnEntryGap(t *testing.T) {
	t.Run("internal cycle", func(t *testing.T) {
		b, entry := entryBuilder(t)
		child := b.Body(program.Span{})
		label := b.Label(program.Span{}, child)
		b.SetLabelCursor(label, 0)
		jump := b.Goto(program.Span{}, child, label)
		finishAtTail(t, b, child, jump)
		finishAtTail(t, b, entry, child)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if head, ok := p.Mu(child); !ok || head != label {
			t.Fatalf("Mu(child) = %v, %v; want internal Label", head, ok)
		}
		if head, ok := p.Mu(entry); ok || head != 0 {
			t.Fatalf("internal child cycle leaked to entry: %v, %v", head, ok)
		}
	})

	t.Run("enclosing cycle", func(t *testing.T) {
		b, entry := entryBuilder(t)
		child := b.Body(program.Span{})
		finishAtTail(t, b, child)
		label := b.Label(program.Span{}, entry)
		b.SetLabelCursor(label, 0)
		jump := b.Goto(program.Span{}, entry, label)
		finishAtTail(t, b, entry, child, jump)
		p, err := b.Seal()
		if err != nil {
			t.Fatal(err)
		}
		for _, term := range []program.Term{entry, child, jump} {
			if head, ok := p.Mu(term); !ok || head != label {
				t.Fatalf("Mu(%v) = %v, %v; want enclosing Label", term, head, ok)
			}
		}
	})
}

func TestCellFrontiersRejectAuthoredUseBeforeBind(t *testing.T) {
	t.Run("self Read in initializer", func(t *testing.T) {
		b, entry := entryBuilder(t)
		cell := b.Cell(program.Span{}, entry)
		read := b.Read(program.Span{}, entry, cell)
		values := b.Values(program.Span{}, entry, []program.Term{read}, 0)
		finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{cell}, values))
		if _, err := b.Seal(); err == nil {
			t.Fatal("Bind initializer read its own inactive Cell")
		}
	})

	t.Run("Read before Bind", func(t *testing.T) {
		b, entry := entryBuilder(t)
		cell := b.Cell(program.Span{}, entry)
		read := b.Read(program.Span{}, entry, cell)
		readValues := b.Values(program.Span{}, entry, []program.Term{read}, 0)
		returned := b.Return(program.Span{}, entry, readValues)
		bound := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, bound)
		finishAtTail(t, b, entry, returned, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("Read crossed a later Bind frontier")
		}
	})

	t.Run("Assign before Bind", func(t *testing.T) {
		b, entry := entryBuilder(t)
		cell := b.Cell(program.Span{}, entry)
		values := b.Values(program.Span{}, entry, nil, 0)
		assign := b.Assign(program.Span{}, entry, []program.Term{cell}, values)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, values)
		finishAtTail(t, b, entry, assign, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("Assign crossed a later Bind frontier")
		}
	})

	t.Run("nested Body before ancestor Bind", func(t *testing.T) {
		b, entry := entryBuilder(t)
		child := b.Body(program.Span{})
		cell := b.Cell(program.Span{}, entry)
		read := b.Read(program.Span{}, child, cell)
		values := b.Values(program.Span{}, child, []program.Term{read}, 0)
		finishAtTail(t, b, child, b.Return(program.Span{}, child, values))
		bound := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, bound)
		finishAtTail(t, b, entry, child, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("nested Body read a later ancestor Cell")
		}
	})

	t.Run("Function before ancestor Bind", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, body)
		function := b.Function(
			program.Span{}, entry, body, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		finishAtTail(t, b, body)
		functionValues := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		returned := b.Return(program.Span{}, entry, functionValues)
		bound := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{outer}, bound)
		finishAtTail(t, b, entry, returned, bind)
		if _, err := b.Seal(); err == nil {
			t.Fatal("Function captured a later ancestor Cell")
		}
	})
}

func TestCellFrontiersAreValidatedBeforeDeadDirectEvidenceIsCleared(t *testing.T) {
	t.Run("dead direct Read before Bind", func(t *testing.T) {
		b, entry := entryBuilder(t)
		functionBody := b.Body(program.Span{})
		cell := b.Cell(program.Span{}, entry)
		function := b.Function(program.Span{}, entry, functionBody, nil, 0, nil)
		finishAtTail(t, b, functionBody)

		empty := b.Values(program.Span{}, entry, nil, 0)
		returned := b.Return(program.Span{}, entry, empty)
		read := b.Read(program.Span{}, entry, cell)
		actuals := b.Values(program.Span{}, entry, nil, 0)
		call := b.Call(program.Span{}, entry, read, 0, actuals)
		bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, bound)
		finishAtTail(t, b, entry, returned, call, bind)

		if _, err := b.Seal(); err == nil {
			t.Fatal("dead direct Call read a Cell before its later Bind")
		}
	})

	t.Run("dead Assign before Bind with direct candidate", func(t *testing.T) {
		b, entry := entryBuilder(t)
		probeBody := b.Body(program.Span{})
		cell := b.Cell(program.Span{}, entry)
		probe := b.Function(program.Span{}, entry, probeBody, nil, 0, nil)
		finishAtTail(t, b, probeBody)

		empty := b.Values(program.Span{}, entry, nil, 0)
		returned := b.Return(program.Span{}, entry, empty)
		actuals := b.Values(program.Span{}, entry, nil, 0)
		probeCall := b.Call(program.Span{}, entry, probe, 0, actuals)
		replacement := b.Values(program.Span{}, entry, nil, probeCall)
		assign := b.Assign(program.Span{}, entry, []program.Term{cell}, replacement)
		bound := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{cell}, bound)
		finishAtTail(t, b, entry, returned, assign, bind)

		if _, err := b.Seal(); err == nil {
			t.Fatal("dead Assign targeted a Cell before its later Bind")
		}
	})

	t.Run("dead direct Function capture before Bind", func(t *testing.T) {
		b, entry := entryBuilder(t)
		functionBody := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, functionBody)
		function := b.Function(
			program.Span{}, entry, functionBody, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		finishAtTail(t, b, functionBody)

		empty := b.Values(program.Span{}, entry, nil, 0)
		returned := b.Return(program.Span{}, entry, empty)
		actuals := b.Values(program.Span{}, entry, nil, 0)
		call := b.Call(program.Span{}, entry, function, 0, actuals)
		bound := b.Values(program.Span{}, entry, nil, 0)
		bind := b.Bind(program.Span{}, entry, []program.Term{outer}, bound)
		finishAtTail(t, b, entry, returned, call, bind)

		if _, err := b.Seal(); err == nil {
			t.Fatal("dead direct Function captured a Cell before its later Bind")
		}
	})
}

func TestRecursiveLocalExceptionIsExactAndAligned(t *testing.T) {
	t.Run("aligned direct Function", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, body)
		function := b.Function(
			program.Span{}, entry, body, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		finishAtTail(t, b, body)
		values := b.Values(program.Span{}, entry, []program.Term{function}, 0)
		finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{outer}, values))
		if _, err := b.Seal(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("misaligned tuple Cell", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		left, right := b.Cell(program.Span{}, entry), b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, body)
		function := b.Function(
			program.Span{}, entry, body, nil, 0,
			[]program.Capture{{Inner: inner, Outer: right}},
		)
		finishAtTail(t, b, body)
		nilValue := b.Nil(program.Span{}, entry)
		values := b.Values(program.Span{}, entry, []program.Term{function, nilValue}, 0)
		finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{left, right}, values))
		if _, err := b.Seal(); err == nil {
			t.Fatal("Function captured a different Cell from its Bind tuple slot")
		}
	})

	t.Run("wrapped Function expression", func(t *testing.T) {
		b, entry := entryBuilder(t)
		body := b.Body(program.Span{})
		outer := b.Cell(program.Span{}, entry)
		inner := b.Cell(program.Span{}, body)
		function := b.Function(
			program.Span{}, entry, body, nil, 0,
			[]program.Capture{{Inner: inner, Outer: outer}},
		)
		finishAtTail(t, b, body)
		wrapped := b.Unary(program.Span{}, entry, program.UnaryNot, function)
		values := b.Values(program.Span{}, entry, []program.Term{wrapped}, 0)
		finishAtTail(t, b, entry, b.Bind(program.Span{}, entry, []program.Term{outer}, values))
		if _, err := b.Seal(); err == nil {
			t.Fatal("wrapped Function received recursive-local authority")
		}
	})
}

func TestNestedImmediateFunctionCanCloseDirectCallSCC(t *testing.T) {
	b, entry := entryBuilder(t)
	outerBody := b.Body(program.Span{})
	innerBody := b.Body(program.Span{})
	outerBinding := b.Cell(program.Span{}, entry)
	outerCapture := b.Cell(program.Span{}, outerBody)
	innerCapture := b.Cell(program.Span{}, innerBody)
	outerFunction := b.Function(
		program.Span{}, entry, outerBody, nil, 0,
		[]program.Capture{{Inner: outerCapture, Outer: outerBinding}},
	)
	innerFunction := b.Function(
		program.Span{}, outerBody, innerBody, nil, 0,
		[]program.Capture{{Inner: innerCapture, Outer: outerCapture}},
	)
	innerCallee := b.Read(program.Span{}, innerBody, innerCapture)
	innerActuals := b.Values(program.Span{}, innerBody, nil, 0)
	innerCall := b.Call(program.Span{}, innerBody, innerCallee, 0, innerActuals)
	innerResults := b.Values(program.Span{}, innerBody, nil, innerCall)
	finishAtTail(t, b, innerBody, b.Return(program.Span{}, innerBody, innerResults))

	outerActuals := b.Values(program.Span{}, outerBody, nil, 0)
	outerCall := b.Call(program.Span{}, outerBody, innerFunction, 0, outerActuals)
	outerResults := b.Values(program.Span{}, outerBody, nil, outerCall)
	finishAtTail(t, b, outerBody, b.Return(program.Span{}, outerBody, outerResults))
	bound := b.Values(program.Span{}, entry, []program.Term{outerFunction}, 0)
	finishAtTail(
		t, b, entry,
		b.Bind(program.Span{}, entry, []program.Term{outerBinding}, bound),
	)

	p, err := b.Seal()
	if err != nil {
		t.Fatal(err)
	}
	for _, function := range []program.Term{outerFunction, innerFunction} {
		if head, ok := p.Mu(function); !ok || head != outerFunction {
			t.Fatalf("Mu(%v) = %v, %v; want nested SCC head %v", function, head, ok, outerFunction)
		}
	}
}

func TestForLoopControlIsReachableAndRetainsTransferKind(t *testing.T) {
	for _, kind := range []program.LoopKind{program.LoopNumericFor, program.LoopGenericFor} {
		t.Run(map[program.LoopKind]string{
			program.LoopNumericFor: "numeric",
			program.LoopGenericFor: "generic",
		}[kind], func(t *testing.T) {
			b, entry := entryBuilder(t)
			functionBody := b.Body(program.Span{})
			loopBody := b.Body(program.Span{})
			outer := b.Cell(program.Span{}, entry)
			capture := b.Cell(program.Span{}, functionBody)
			iteration := b.Cell(program.Span{}, loopBody)
			function := b.Function(
				program.Span{}, entry, functionBody, nil, 0,
				[]program.Capture{{Inner: capture, Outer: outer}},
			)
			finishAtTail(t, b, loopBody)
			callee := b.Read(program.Span{}, functionBody, capture)
			actuals := b.Values(program.Span{}, functionBody, nil, 0)
			controlCall := b.Call(program.Span{}, functionBody, callee, 0, actuals)
			fixed := []program.Term{controlCall}
			if kind == program.LoopNumericFor {
				fixed = append(fixed, b.Integer(program.Span{}, functionBody, 1))
			}
			control := b.Values(program.Span{}, functionBody, fixed, 0)
			loop := b.Loop(
				program.Span{}, functionBody, loopBody, control,
				[]program.Term{iteration}, kind,
			)
			finishAtTail(t, b, functionBody, loop)
			bound := b.Values(program.Span{}, entry, []program.Term{function}, 0)
			finishAtTail(
				t, b, entry,
				b.Bind(program.Span{}, entry, []program.Term{outer}, bound),
			)

			p, err := b.Seal()
			if err != nil {
				t.Fatal(err)
			}
			if head, ok := p.Mu(function); !ok || head != function {
				t.Fatalf("preheader direct Call was not reachable: Mu(Function) = %v, %v", head, ok)
			}
			if head, ok := p.Mu(loop); !ok || head != loop {
				t.Fatalf("Loop decision/body recurrence = %v, %v", head, ok)
			}
			gotOwner, gotBody, gotControl, gotKind, ok := p.Loop(loop)
			if !ok || gotOwner != functionBody || gotBody != loopBody ||
				gotControl != control || gotKind != kind {
				t.Fatalf(
					"Loop transfer = %v, %v, %v, %v, %v",
					gotOwner, gotBody, gotControl, gotKind, ok,
				)
			}
		})
	}
}
