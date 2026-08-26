package acceptance_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

var (
	edgeResetCountSink int
	edgeResetTermSink  keyspace.Term
	edgeResetBoolSink  bool
)

// Reset support belongs to one final Causal Edge.  The index returned by the
// sealed Edges owner is the only handle accepted by ResetCount/At/Contains;
// no Program-wide Mu or decision-set forwarding surface remains.
func TestFlowCausalRecurrenceEdgesCarryTheirOwnResetSupport(t *testing.T) {
	p := parseBindLower(t, `while again() do tick() end`)
	loop, ok := p.Flow().Authored().Control().Loops().At(0)
	if !ok {
		t.Fatal("missing while Loop")
	}
	edges := p.Flow().Causal().Edges()
	feedback := -1
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if edgeOK && edge.Mu == loop {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("while recurrence has no final Causal Edge")
	}
	count, ok := edges.ResetCount(feedback)
	if !ok || count == 0 || !edges.ResetContains(feedback, loop) {
		t.Fatalf("while reset support = %d/%v, contains Loop=%v", count, ok, edges.ResetContains(feedback, loop))
	}
	for index := 0; index < count; index++ {
		decision, decisionOK := edges.ResetAt(feedback, index)
		if !decisionOK || decision == 0 {
			t.Fatalf("while reset[%d] = %v/%v", index, decision, decisionOK)
		}
	}
	if decision, decisionOK := edges.ResetAt(feedback, count); decisionOK || decision != 0 {
		t.Fatalf("while reset past end = %v/%v", decision, decisionOK)
	}
}

func TestFlowCausalGotoRecurrenceCanRetainEmptyResetSupport(t *testing.T) {
	p := parseBindLower(t, `::again::; goto again`)
	label, ok := p.Flow().Authored().Control().Labels().At(0)
	if !ok {
		t.Fatal("missing label")
	}
	edges := p.Flow().Causal().Edges()
	feedback := -1
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if edgeOK && edge.Mu == label {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("backward goto has no final recurrence Edge")
	}
	if count, ok := edges.ResetCount(feedback); !ok || count != 0 {
		t.Fatalf("empty goto reset = %d/%v, want 0/true", count, ok)
	}
	if decision, ok := edges.ResetAt(feedback, 0); ok || decision != 0 {
		t.Fatalf("empty goto reset[0] = %v/%v", decision, ok)
	}
}

func TestFlowCausalNestedRecurrencesKeepSeparateEdgeSupport(t *testing.T) {
	p := parseBindLower(t, `
while outer() do
  while inner() do tick() end
end
`)
	loops := p.Flow().Authored().Control().Loops()
	outer, outerOK := loops.At(0)
	inner, innerOK := loops.At(1)
	if !outerOK || !innerOK {
		t.Fatalf("nested Loops = %v/%v, %v/%v", outer, outerOK, inner, innerOK)
	}
	edges := p.Flow().Causal().Edges()
	outerFeedback, innerFeedback := -1, -1
	for index := 0; index < edges.Count(); index++ {
		_, edgeOK := edges.At(index)
		if !edgeOK || !edges.ResetContains(index, inner) {
			continue
		}
		if edges.ResetContains(index, outer) {
			outerFeedback = index
		} else {
			innerFeedback = index
		}
	}
	if outerFeedback < 0 || innerFeedback < 0 {
		t.Fatalf("nested recurrence edges outer=%d inner=%d", outerFeedback, innerFeedback)
	}
	if !edges.ResetContains(outerFeedback, outer) || !edges.ResetContains(outerFeedback, inner) {
		t.Fatal("outer recurrence reset did not cover its nested Loop decision")
	}
	if !edges.ResetContains(innerFeedback, inner) {
		t.Fatal("inner recurrence reset did not cover its own Loop decision")
	}
}

func TestFlowCausalResetQueriesDoNotAllocate(t *testing.T) {
	p := parseBindLower(t, `while left() and right() do tick() end`)
	loop, ok := p.Flow().Authored().Control().Loops().At(0)
	if !ok {
		t.Fatal("missing Loop")
	}
	edges := p.Flow().Causal().Edges()
	feedback := -1
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if edgeOK && edge.Mu == loop {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("missing recurrence Edge")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		edgeResetCountSink, edgeResetBoolSink = edges.ResetCount(feedback)
		edgeResetTermSink, edgeResetBoolSink = edges.ResetAt(feedback, 0)
		edgeResetBoolSink = edges.ResetContains(feedback, loop)
	}); allocations != 0 {
		t.Fatalf("Causal reset queries allocate %v times", allocations)
	}
}

// recurrenceResetEdge finds the one retained final Edge that carries a Mu
// head. Reset membership belongs to that Edge; there is no Program-wide Mu
// decision query.
func recurrenceResetEdge(t *testing.T, p *program.Program, head keyspace.Term) int {
	t.Helper()
	edges := p.Flow().Causal().Edges()
	for index := 0; index < edges.Count(); index++ {
		edge, ok := edges.At(index)
		if ok && edge.Mu == head {
			return index
		}
	}
	t.Fatalf("no final causal Edge carries Mu %v", head)
	return -1
}

func requireEdgeReset(t *testing.T, p *program.Program, edgeIndex int, want ...keyspace.Term) {
	t.Helper()
	edges := p.Flow().Causal().Edges()
	count, ok := edges.ResetCount(edgeIndex)
	if !ok || count != len(want) {
		t.Fatalf("Edge %d ResetCount = %d/%v, want %d", edgeIndex, count, ok, len(want))
	}
	for offset := 0; offset < count; offset++ {
		decision, decisionOK := edges.ResetAt(edgeIndex, offset)
		if !decisionOK || !edges.ResetContains(edgeIndex, decision) {
			t.Fatalf("Edge %d ResetAt(%d) = %v/%v without membership", edgeIndex, offset, decision, decisionOK)
		}
	}
	for _, decision := range want {
		if !edges.ResetContains(edgeIndex, decision) {
			t.Fatalf("Edge %d reset omits decision %v", edgeIndex, decision)
		}
	}
}

func TestFinalLoopResetOwnsOnlyReevaluatedDecisions(t *testing.T) {
	p := parseBindLower(t, `while left() and right() do tick() end`)
	loop, ok := p.Flow().Authored().Control().Loops().At(0)
	if !ok {
		t.Fatal("Loop is absent")
	}
	selection, ok := p.Flow().Authored().Operators().Selects().At(0)
	if !ok {
		t.Fatal("loop short-circuit Select is absent")
	}
	recurrence := recurrenceResetEdge(t, p, loop)
	requireEdgeReset(t, p, recurrence, selection, loop)
}

func TestFinalNestedLoopResetsRemainHeadLocal(t *testing.T) {
	p := parseBindLower(t, `
while outer() do
  while inner() do tick() end
end
`)
	loops := p.Flow().Authored().Control().Loops()
	outer, outerOK := loops.At(0)
	inner, innerOK := loops.At(1)
	if !outerOK || !innerOK {
		t.Fatal("nested Loops are absent")
	}
	requireEdgeReset(t, p, recurrenceResetEdge(t, p, outer), outer, inner)
	requireEdgeReset(t, p, recurrenceResetEdge(t, p, inner), inner)
}

func TestFinalBackwardGotoRetainsAnEmptyResetInterval(t *testing.T) {
	p := parseBindLower(t, "::again::; goto again")
	label, ok := p.Flow().Authored().Control().Labels().At(0)
	if !ok {
		t.Fatal("Label is absent")
	}
	recurrence := recurrenceResetEdge(t, p, label)
	requireEdgeReset(t, p, recurrence)
}

func TestFinalResetQueriesAreAllocationFree(t *testing.T) {
	p := parseBindLower(t, `while left() and right() do tick() end`)
	loop, _ := p.Flow().Authored().Control().Loops().At(0)
	edge := recurrenceResetEdge(t, p, loop)
	if count, ok := p.Flow().Causal().Edges().ResetCount(edge); !ok || count != 2 {
		t.Fatalf("ResetCount = %d/%v, want 2", count, ok)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Causal().Edges().ResetCount(edge)
		_, _ = p.Flow().Causal().Edges().ResetAt(edge, 0)
		_ = p.Flow().Causal().Edges().ResetContains(edge, loop)
	})
	if allocations != 0 {
		t.Fatalf("final Edge reset queries allocate %f times", allocations)
	}
}

// TestRecurrenceExitArmLowersThroughEvaluationPorts keeps the real fixture
// that exposed the Repeat-control owner frontier on the named regression
// path. The fixture contains nested Repeat conditions authored by the loop
// Body; lowering must seal those ports without weakening the owner proof.
func TestRecurrenceExitArmLowersThroughEvaluationPorts(t *testing.T) {
	const fixtureName = "testdata/fixtures/soundness/recurrence-exit-arm/main.lua"
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve recurrence fixture test location")
	}
	repository := repositoryRoot(t, filepath.Dir(thisFile))
	text, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(fixtureName)))
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(programlower.Source{
		Name: fixtureName,
		Text: text,
	})
	if err != nil {
		t.Fatal(err)
	}
	flow := p.Flow()
	loops := flow.Authored().Control().Loops()
	repeatCount := 0
	for ordinal := 0; ordinal < loops.Count(); ordinal++ {
		loop, ok := loops.At(ordinal)
		if !ok {
			t.Fatalf("missing Loop %d", ordinal+1)
		}
		owner, body, loopKind, control, ok := loops.Get(loop)
		if !ok {
			t.Fatalf("Loop %v has no authored row", loop)
		}
		if loopKind != kind.LoopRepeat {
			continue
		}
		repeatCount++
		if owner == body || keyspace.TermFamily(control) != keyspace.FamilyBinary {
			t.Fatalf("Repeat %v owner/body/control = %v/%v/%v; want distinct Body-owned Binary control", loop, owner, body, control)
		}
		binaryOwner, _, left, right, ok := flow.Authored().Operators().Binaries().Get(control)
		if !ok || binaryOwner != body || left == 0 || right == 0 {
			t.Fatalf("Repeat %v control %v owner/operands = %v/%v/%v/%v; want Body %v", loop, control, binaryOwner, left, right, ok, body)
		}
		if entry, ok := flow.Ports().Entry(loop); !ok || entry != body {
			t.Fatalf("Repeat %v Entry = %v/%v; want loop Body %v", loop, entry, ok, body)
		}
		if finish, ok := flow.Ports().Finish(loop); !ok || finish != loop {
			t.Fatalf("Repeat %v Finish = %v/%v; want Loop", loop, finish, ok)
		}
		controlEntry, controlOK := flow.Ports().Entry(control)
		leftEntry, leftOK := flow.Ports().Entry(left)
		if !controlOK || !leftOK || controlEntry != leftEntry {
			t.Fatalf("Repeat %v control/left Entry = %v/%v and %v/%v; want same evaluated port", loop, controlEntry, controlOK, leftEntry, leftOK)
		}
		if finish, ok := flow.Ports().Finish(control); !ok || finish != control {
			t.Fatalf("Repeat %v control Finish = %v/%v; want control", loop, finish, ok)
		}
	}
	if repeatCount != 1 {
		t.Fatalf("recurrence-exit-arm Repeat count = %d, want one frozen control frontier", repeatCount)
	}
}

// crossedGotoSource has two entries into one cyclic label component and both
// forward-crossing and backedge transfers at every label. It is deliberately
// source-valid (no local scope crossings) while defeating structured-CFG-only
// dominance assumptions.
func crossedGotoSource(width int) string {
	var source strings.Builder
	source.Grow(96 + width*96)
	source.WriteString("local f\nf = function() end\nif entry() then goto n0 else goto n1 end\n")
	for index := 0; index < width; index++ {
		next := (index + 1) % width
		cross := (index + width/2 + 1) % width
		source.WriteString("::n")
		source.WriteString(strconv.Itoa(index))
		source.WriteString("::\nif step")
		source.WriteString(strconv.Itoa(index))
		source.WriteString("() then goto n")
		source.WriteString(strconv.Itoa(next))
		source.WriteString(" end\nif cross")
		source.WriteString(strconv.Itoa(index))
		source.WriteString("() then goto n")
		source.WriteString(strconv.Itoa(cross))
		source.WriteString(" end\ngoto done\n")
	}
	source.WriteString("::done::\nf()\n")
	return source.String()
}

func assertCrossedGotoDominance(t *testing.T, p *program.Program, width int) {
	t.Helper()
	flow := p.Flow()
	functions := flow.Authored().Functions()
	calls := flow.Authored().Calls()
	labels := flow.Authored().Control().Labels()
	function, ok := functions.At(0)
	if !ok {
		t.Fatal("missing installation Function")
	}
	call, ok := calls.At(calls.Count() - 1)
	if !ok {
		t.Fatal("missing final direct Call")
	}
	direct, directOK := flow.DirectFunctions().Call(call)
	if !directOK || direct != function {
		t.Fatalf("crossed-goto final Call direct = %v/%v, want %v", direct, directOK, function)
	}
	if labels.Count() != width+1 { // the cyclic labels plus ::done::
		t.Fatalf("crossed-goto LabelCount = %d, want %d", labels.Count(), width+1)
	}
	head, ok := labels.At(0)
	if !ok || head == 0 {
		t.Fatal("crossed-goto canonical head Label is absent")
	}
	seenMu := false
	edges := flow.Causal().Edges()
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if !edgeOK || edge.Mu == 0 {
			continue
		}
		seenMu = true
		if edge.Mu != head {
			t.Fatalf("crossed-goto Edge[%d] Mu = %v, want canonical Label %v", index, edge.Mu, head)
		}
	}
	if !seenMu {
		t.Fatal("crossed-goto Flow Causal Edges retained no recurrence route")
	}
}

func TestSourceDominanceCrossedGotoScaleAndSemantics(t *testing.T) {
	for _, width := range []int{2, 7, 31} {
		t.Run("reference_"+strconv.Itoa(width), func(t *testing.T) {
			first := parseBindLower(t, crossedGotoSource(width))
			second := parseBindLower(t, crossedGotoSource(width))
			assertCrossedGotoDominance(t, first, width)
			assertCrossedGotoDominance(t, second, width)
			if first.ContentID() != second.ContentID() {
				t.Fatalf("crossed-goto width %d changed ContentID across equivalent seals", width)
			}
		})
	}

	// This large generated SCC is the scale law: the same semantic assertions
	// exercise thousands of crossed entries/backedges without recursive DFS or
	// a traversal cap. A quadratic changed-loop dominator implementation is
	// exposed here by the dense irreducible predecessor family.
	const wide = 1024
	wideProgram := parseBindLower(t, crossedGotoSource(wide))
	assertCrossedGotoDominance(t, wideProgram, wide)
}

func recursiveCaptureReadSource(captures, selfReads int) string {
	var source strings.Builder
	source.Grow(64 + (captures+selfReads)*32)
	source.WriteString("local f\n")
	for index := 0; index < captures; index++ {
		source.WriteString("local capture")
		source.WriteString(strconv.Itoa(index))
		source.WriteString(" = ")
		source.WriteString(strconv.Itoa(index))
		source.WriteByte('\n')
	}
	source.WriteString("f = function()\n")
	// Establish all non-self captures before the self capture. The former
	// per-occurrence scan therefore had to walk the whole capture set for each
	// following f() Read.
	for index := 0; index < captures; index++ {
		source.WriteString("local held")
		source.WriteString(strconv.Itoa(index))
		source.WriteString(" = capture")
		source.WriteString(strconv.Itoa(index))
		source.WriteByte('\n')
	}
	for index := 0; index < selfReads; index++ {
		source.WriteString("f()\n")
	}
	source.WriteString("end\nreturn f\n")
	return source.String()
}

func TestSourceRecursiveSelfProofWideCaptureReadScale(t *testing.T) {
	const captures = 1024
	const selfReads = 1024
	p := parseBindLower(t, recursiveCaptureReadSource(captures, selfReads))
	flow := p.Flow()
	functions := flow.Authored().Functions()
	calls := flow.Authored().Calls()
	function, _ := functions.At(0)
	if count, ok := functions.CaptureCount(function); !ok || count != captures+1 {
		t.Fatalf("wide Function captures = %d/%v, want %d including self", count, ok, captures+1)
	}
	if calls.Count() != selfReads {
		t.Fatalf("wide recursive Calls = %d, want %d", calls.Count(), selfReads)
	}
	for _, index := range []int{0, selfReads / 2, selfReads - 1} {
		call, _ := calls.At(index)
		direct, directOK := flow.DirectFunctions().Call(call)
		if !directOK || direct != function {
			t.Fatalf("wide recursive Call %d direct = %v/%v, want %v", index, direct, directOK, function)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		call, _ := calls.At(selfReads - 1)
		_, _ = flow.DirectFunctions().Call(call)
	}); allocations != 0 {
		t.Fatalf("wide recursive direct query allocations = %v, want zero", allocations)
	}
}

// A Repeat Body is entered unconditionally, so a Body that always leaves
// through a Return still lowers and seals: the Loop keeps its initial
// Loop -> Body route, and the condition it can never reach publishes no
// guarded route. Nesting one such Repeat inside another carries the same law
// outward, because the outer Body then leaves through the inner Loop.
func TestRepeatWithTerminalBodyLowersWithoutConditionRoutes(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		source string
		want   int
	}{
		{name: "terminal-body", source: "local x = 1\nrepeat return x until x > 0\n", want: 1},
		{name: "nested-terminal-body", source: "local x = 1\nrepeat repeat return x until x > 0 until x > 1\n", want: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			p := parseBindLower(t, testCase.source)
			loops := p.Flow().Authored().Control().Loops()
			successors := p.Flow().Causal().Successors()
			repeats := 0
			for ordinal := 0; ordinal < loops.Count(); ordinal++ {
				loop, ok := loops.At(ordinal)
				if !ok {
					t.Fatalf("missing Loop %d", ordinal+1)
				}
				_, body, loopKind, control, rowOK := loops.Get(loop)
				if !rowOK {
					t.Fatalf("Loop %v has no authored row", loop)
				}
				if loopKind != kind.LoopRepeat {
					continue
				}
				repeats++
				initial := false
				for index := 0; index < successors.Count(loop); index++ {
					successor, successorOK := successors.At(loop, index)
					if successorOK && !successor.IsBoundary() && successor.To == body && successor.Decision == 0 {
						initial = true
					}
				}
				if !initial {
					t.Fatalf("Repeat %v has no initial Loop -> Body route among %d successors", loop, successors.Count(loop))
				}
				for index := 0; index < successors.Count(control); index++ {
					successor, successorOK := successors.At(control, index)
					if successorOK && !successor.IsBoundary() && successor.Decision == loop {
						t.Fatalf("Repeat %v published a guarded route from its unreached condition: %v -> %v", loop, successor.From, successor.To)
					}
				}
			}
			if repeats != testCase.want {
				t.Fatalf("Repeat count = %d, want %d", repeats, testCase.want)
			}
		})
	}
}
