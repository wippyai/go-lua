package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program"
)

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
