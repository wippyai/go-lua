package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Pending is the evaluation-owned immutable payload set.  It is queried from
// Flow directly; the retired Program-wide continuation-value forwarding plane
// is intentionally not recreated here.
func TestFlowPendingKeepsEarlierOperandsAtCallBoundaries(t *testing.T) {
	p := parseBindLower(t, `local result = left + yieldfn()`)
	call := continuationNamedCall(t, p, "yieldfn")
	binary, ok := p.Flow().Authored().Operators().Binaries().At(0)
	if !ok {
		t.Fatal("missing binary relation")
	}
	_, _, left, _, ok := p.Flow().Authored().Operators().Binaries().Get(binary)
	if !ok {
		t.Fatal("invalid binary relation")
	}
	continuationExactPending(t, p, call, left)

	p = parseBindLower(t, `object[keyfn()] = first(), yieldfn()`)
	call = continuationNamedCall(t, p, "yieldfn")
	first := continuationNamedCall(t, p, "first")
	assign, ok := p.Flow().Authored().Storage().Assigns().At(0)
	if !ok {
		t.Fatal("missing assignment")
	}
	write, ok := p.Flow().Authored().Storage().Assigns().WriteAt(assign, 0)
	if !ok {
		t.Fatal("missing assignment write")
	}
	_, target, ok := p.Flow().Authored().Storage().Writes().Get(write)
	if !ok {
		t.Fatal("invalid assignment write")
	}
	continuationExactPending(t, p, call, target, first)
}

// Continuation is the final Flow owner for lexical Cells and reaching Guards;
// a Call is merely the exact subject term, not a global continuation identity.
func TestFlowContinuationCellsFollowLexicalScope(t *testing.T) {
	t.Run("before and after locals", func(t *testing.T) {
		p := parseBindLower(t, `
local before = 1
yieldfn()
local after = 2
`)
		call := continuationNamedCall(t, p, "yieldfn")
		before := continuationCellAtLine(t, p, 2)
		after := continuationCellAtLine(t, p, 4)
		continuationExactCells(t, p, call, before)
		continuationLacksCell(t, p, call, after)
	})

	t.Run("captured outer cell is not an inner activation cell", func(t *testing.T) {
		p := parseBindLower(t, `
local outer = 0
local function worker()
  yieldfn()
  return outer
end
`)
		call := continuationNamedCall(t, p, "yieldfn")
		function, ok := p.Flow().Authored().Functions().At(0)
		if !ok {
			t.Fatal("missing worker Function")
		}
		inner, outer, ok := p.Flow().Authored().Functions().CaptureAt(function, 0)
		if !ok {
			t.Fatal("missing worker capture")
		}
		continuationExactCells(t, p, call, inner)
		continuationLacksCell(t, p, call, outer)
	})

	t.Run("dead and static calls have no final continuation root", func(t *testing.T) {
		staticProgram := parseBindLower(t, `type Snapshot = typeof(staticfn())`)
		staticCall := continuationNamedCall(t, staticProgram, "staticfn")
		if !staticProgram.Flow().Containment().Static(staticCall) {
			t.Fatal("typeof Call escaped static containment")
		}
		if count, ok := staticProgram.Flow().Continuation().CellCount(staticCall); ok || count != 0 {
			t.Fatalf("static Call continuation Cells = %d/%v, want absent", count, ok)
		}
		if count, ok := staticProgram.Flow().Pending().Count(staticCall); ok || count != 0 {
			t.Fatalf("static Call pending payloads = %d/%v, want absent", count, ok)
		}

		deadProgram := parseBindLower(t, `
goto done
unreachablefn()
::done::
`)
		deadCall := continuationNamedCall(t, deadProgram, "unreachablefn")
		if deadProgram.Flow().Executable().Contains(deadCall) {
			t.Fatal("terminally unreachable Call became executable")
		}
		if count, ok := deadProgram.Flow().Continuation().CellCount(deadCall); ok || count != 0 {
			t.Fatalf("dead Call continuation Cells = %d/%v, want absent", count, ok)
		}
		if count, ok := deadProgram.Flow().Pending().Count(deadCall); ok || count != 0 {
			t.Fatalf("dead Call pending payloads = %d/%v, want absent", count, ok)
		}
	})
}

func TestFlowContinuationCellOrderIsStableAndAllocationFree(t *testing.T) {
	const depth = 256
	p := continuationDeepProgram(t, depth)
	call := continuationNamedCall(t, p, "yielddeep")
	continuation := p.Flow().Continuation()
	want, ok := continuation.CellCount(call)
	if !ok || want != depth {
		t.Fatalf("deep continuation Cell count = %d/%v, want %d", want, ok, depth)
	}
	for index := 0; index < want; index++ {
		cell, cellOK := continuation.CellAt(call, index)
		if !cellOK || cell == 0 {
			t.Fatalf("continuation Cell %d = %v/%v", index, cell, cellOK)
		}
		if index == 0 || index == want-1 {
			span, spanOK := p.Source().Identity().Span(cell)
			if !spanOK {
				t.Fatalf("continuation Cell %d has no Source span", index)
			}
			wantLine := depth - index*(depth-1)
			if int(span.StartLine) != wantLine {
				t.Fatalf("continuation Cell %d line = %d, want %d", index, span.StartLine, wantLine)
			}
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		for index := 0; index < want; index++ {
			continuationCellSink, _ = continuation.CellAt(call, index)
		}
	}); allocations != 0 {
		t.Fatalf("deep continuation Cell enumeration allocations = %v, want 0", allocations)
	}
}

func TestFlowContinuationContentIDHandlesDeepScope(t *testing.T) {
	p := continuationDeepProgram(t, 256)
	first := p.ContentID()
	if !first.Available() {
		t.Fatal("deep continuation Program has no ContentID")
	}
	if second := p.ContentID(); second != first {
		t.Fatalf("deep continuation ContentID is not deterministic: %x != %x", second, first)
	}
}

var continuationCellSink keyspace.Term

type continuationTestingT interface {
	Helper()
	Fatalf(string, ...any)
}

func continuationDeepProgram(t continuationTestingT, depth int) *program.Program {
	t.Helper()
	if depth <= 0 {
		t.Fatalf("invalid continuation depth %d", depth)
	}
	var input strings.Builder
	for index := 0; index < depth; index++ {
		input.WriteString("do local local")
		input.WriteString(strconv.Itoa(index))
		input.WriteString(" = ")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("\n")
	}
	input.WriteString("yielddeep()\n")
	for index := 0; index < depth; index++ {
		input.WriteString("end\n")
	}
	p, err := lowerSource(input.String())
	if err != nil {
		t.Fatalf("deep continuation lower: %v", err)
	}
	return p
}

func continuationNamedCall(t continuationTestingT, p *program.Program, name string) keyspace.Term {
	t.Helper()
	flowView := p.Flow()
	calls := flowView.Authored().Calls()
	reads := flowView.Authored().Storage().Reads()
	cells := flowView.Authored().Storage().Cells()
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		_, callee, _, _, ok := calls.Get(call)
		if !ok {
			continue
		}
		_, cell, _, ok := reads.Get(callee)
		if !ok {
			continue
		}
		_, _, key, ok := cells.Get(cell)
		if !ok {
			continue
		}
		literal, ok := p.Source().Keys().Exact(key)
		if ok && literal.Kind == keyspace.LiteralString && literal.String == name {
			return call
		}
	}
	t.Fatalf("missing Call to %q", name)
	return 0
}

func continuationCellAtLine(t *testing.T, p *program.Program, line int) keyspace.Term {
	t.Helper()
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		cell, _ := cells.At(index)
		span, ok := p.Source().Identity().Span(cell)
		if ok && int(span.StartLine) == line {
			return cell
		}
	}
	t.Fatalf("missing Cell at line %d", line)
	return 0
}

func continuationExactCells(t *testing.T, p *program.Program, call keyspace.Term, want ...keyspace.Term) {
	t.Helper()
	continuation := p.Flow().Continuation()
	count, ok := continuation.CellCount(call)
	if !ok || count != len(want) {
		t.Fatalf("Call %v continuation Cell count = %d/%v, want %d", call, count, ok, len(want))
	}
	for _, expected := range want {
		found := false
		for index := 0; index < count; index++ {
			got, gotOK := continuation.CellAt(call, index)
			found = found || gotOK && got == expected
		}
		if !found {
			t.Fatalf("Call %v continuation Cells do not contain %v", call, expected)
		}
	}
}

func continuationLacksCell(t *testing.T, p *program.Program, call, unwanted keyspace.Term) {
	t.Helper()
	continuation := p.Flow().Continuation()
	count, ok := continuation.CellCount(call)
	if !ok {
		t.Fatalf("Call %v has no continuation Cell projection", call)
	}
	for index := 0; index < count; index++ {
		got, gotOK := continuation.CellAt(call, index)
		if gotOK && got == unwanted {
			t.Fatalf("Call %v continuation Cells unexpectedly contain %v", call, unwanted)
		}
	}
}

func continuationExactPending(t *testing.T, p *program.Program, call keyspace.Term, want ...keyspace.Term) {
	t.Helper()
	pending := p.Flow().Pending()
	count, ok := pending.Count(call)
	if !ok || count != len(want) {
		t.Fatalf("Call %v pending payload count = %d/%v, want %d", call, count, ok, len(want))
	}
	for _, expected := range want {
		found := false
		for index := 0; index < count; index++ {
			got, gotOK := pending.At(call, index)
			found = found || gotOK && got == expected
		}
		if !found {
			t.Fatalf("Call %v pending payloads do not contain %v", call, expected)
		}
	}
}
