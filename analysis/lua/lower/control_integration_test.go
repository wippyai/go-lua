package lower_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

var (
	edgeCountSink int
	edgeSink      keyspace.Term
	edgeBoolSink  bool
)

// Causal Edges are immutable Flow rows.  They are selected through a Body or
// activation owner; there is no Program-level edge capability or parallel CFG.
func TestFlowCausalEdgesAreIndexedByExactBodyAndActivation(t *testing.T) {
	p := parseBindLower(t, `
if first() then second() end
while third() do fourth() end
return fifth()
`)
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	edges := p.Flow().Causal().Edges()
	bodyCount, bodyOK := edges.BodyCount(entry)
	activationCount, activationOK := edges.ActivationCount(entry)
	if !bodyOK || !activationOK || bodyCount == 0 || activationCount == 0 {
		t.Fatalf("entry edge counts body=%d/%v activation=%d/%v", bodyCount, bodyOK, activationCount, activationOK)
	}
	calls := p.Flow().Authored().Calls()
	for index := 0; index < activationCount; index++ {
		edge, edgeOK := edges.ActivationAt(entry, index)
		if !edgeOK || edge.From == 0 || edge.To == 0 {
			t.Fatalf("activation Edge[%d] = %#v/%v", index, edge, edgeOK)
		}
		if _, _, _, _, callOK := calls.Get(edge.From); callOK {
			t.Fatalf("Call %v bypasses its Causal boundary through Edge %#v", edge.From, edge)
		}
	}
	if edge, edgeOK := edges.ActivationAt(entry, activationCount); edgeOK || edge.From != 0 {
		t.Fatalf("ActivationAt past end = %#v/%v", edge, edgeOK)
	}
	for index := 0; index < bodyCount; index++ {
		edge, edgeOK := edges.BodyAt(entry, index)
		if !edgeOK || edge.From == 0 || edge.To == 0 {
			t.Fatalf("body Edge[%d] = %#v/%v", index, edge, edgeOK)
		}
	}
}

func TestFlowCausalBoundariesCutExecutableCalls(t *testing.T) {
	p := parseBindLower(t, `
if invoke() then end
while retry() do break end
return finish()
`)
	edges := p.Flow().Causal().Edges()
	boundaries := p.Flow().Causal().Boundaries()
	calls := p.Flow().Authored().Calls()
	if calls.Count() != 3 {
		t.Fatalf("Call count = %d, want 3", calls.Count())
	}
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		boundary, boundaryOK := boundaries.For(call)
		if !boundaryOK || boundary.Call != call || boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
			t.Fatalf("Call boundary[%d] = %#v/%v", index, boundary, boundaryOK)
		}
		if index == calls.Count()-1 {
			if boundary.Normal != 0 || boundary.TailReturn == 0 {
				t.Fatalf("tail Call boundary[%d] = %#v", index, boundary)
			}
		} else if boundary.Normal == 0 || boundary.TailReturn != 0 {
			t.Fatalf("ordinary Call boundary[%d] = %#v", index, boundary)
		}
		for edgeIndex := 0; edgeIndex < edges.Count(); edgeIndex++ {
			edge, edgeOK := edges.At(edgeIndex)
			if edgeOK && edge.From == call {
				t.Fatalf("Call %v bypasses boundary through Edge %#v", call, edge)
			}
		}
	}
}

func TestFlowCausalEdgeQueriesDoNotAllocate(t *testing.T) {
	p := parseBindLower(t, `while test() do tick() end`)
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	edges := p.Flow().Causal().Edges()
	count, ok := edges.ActivationCount(entry)
	if !ok || count == 0 {
		t.Fatalf("ActivationCount = %d/%v", count, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		edgeCountSink, edgeBoolSink = edges.ActivationCount(entry)
		edge, edgeOK := edges.ActivationAt(entry, 0)
		edgeBoolSink = edgeOK
		edgeSink = edge.From
		edgeCountSink, edgeBoolSink = edges.BodyCount(entry)
	}); allocations != 0 {
		t.Fatalf("Flow Causal edge queries allocate %v times", allocations)
	}
}

func entrySource(t *testing.T, p *program.Program, index int) keyspace.Term {
	t.Helper()
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("Program has no Source entry")
	}
	term, ok := p.Source().Order().BodyAt(entry, index)
	if !ok {
		t.Fatalf("entry Source order has no term %d", index)
	}
	return term
}

func TestFlowStorageAssignKeepsExactWriteAndDynamicLens(t *testing.T) {
	p := parseBindLower(t, "a()[f()] = g()")
	assign := entrySource(t, p, 0)
	assigns := p.Flow().Authored().Storage().Assigns()
	writes := p.Flow().Authored().Storage().Writes()
	_, values, assignOK := assigns.Get(assign)
	if !assignOK || values == 0 {
		t.Fatalf("Assign = values %v/%v", values, assignOK)
	}
	if count, ok := assigns.WriteCount(assign); !ok || count != 1 {
		t.Fatalf("Assign WriteCount = %d/%v, want one", count, ok)
	}
	write, _ := assigns.WriteAt(assign, 0)
	parent, target, writeOK := writes.Get(write)
	if !writeOK || parent != assign || target == 0 {
		t.Fatalf("Write = parent %v target %v ok %v", parent, target, writeOK)
	}
	_, base, key, lensOK := p.Flow().Authored().Access().Dynamic().Get(target)
	if !lensOK || base == 0 || key == 0 {
		t.Fatalf("Write target dynamic Lens = base %v key %v ok %v", base, key, lensOK)
	}
	if span, ok := p.Source().Identity().Span(write); !ok || span.StartLine != 1 || span.StartCol != 1 {
		t.Fatalf("Write Source span = %#v/%v", span, ok)
	}
	rhs := valueAt(t, p, values, 0)
	if _, _, _, _, ok := p.Flow().Authored().Calls().Get(rhs); !ok {
		t.Fatalf("Assign RHS = %v, want authored Call", rhs)
	}
}

func TestFlowSelectKeepsGuardedRightCallInCausalEdges(t *testing.T) {
	p := parseBindLower(t, "return x and f()")
	returned := entrySource(t, p, 0)
	_, values, returnOK := p.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("missing Return Values")
	}
	selectTerm := valueAt(t, p, values, 0)
	_, op, left, right, selectOK := p.Flow().Authored().Operators().Selects().Get(selectTerm)
	if !selectOK || op != kind.SelectAnd || left == 0 || right == 0 {
		t.Fatalf("Select = op %v left %v right %v ok %v", op, left, right, selectOK)
	}
	if _, _, _, _, ok := p.Flow().Authored().Calls().Get(right); !ok {
		t.Fatalf("guarded RHS %v is not f() Call", right)
	}
	edges := p.Flow().Causal().Edges()
	truthy, falsy := false, false
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		if !edgeOK || edge.Decision != selectTerm {
			continue
		}
		if edge.Truth {
			truthy = true
		} else {
			falsy = true
		}
	}
	if !truthy || !falsy {
		t.Fatalf("Select Causal alternatives truthy=%v falsy=%v", truthy, falsy)
	}
}

func TestFlowTableFieldsKeepExactKindsAndValues(t *testing.T) {
	p := parseBindLower(t, "return {[f()] = g(), h()}")
	returned := entrySource(t, p, 0)
	_, values, returnOK := p.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("missing Return Values")
	}
	table := valueAt(t, p, values, 0)
	tables := p.Flow().Authored().Tables()
	fields := p.Flow().Authored().Fields()
	if _, ok := tables.Get(table); !ok {
		t.Fatalf("return value %v is not Table", table)
	}
	first, firstOK := tables.FieldAt(table, 0)
	second, secondOK := tables.FieldAt(table, 1)
	if !firstOK || !secondOK {
		t.Fatalf("Table fields = %v/%v %v/%v", first, firstOK, second, secondOK)
	}
	parent, key, fieldValues, fieldKind, firstOK := fields.Get(first)
	if !firstOK || parent != table || fieldKind != kind.FieldKey || key == 0 || fieldValues == 0 {
		t.Fatalf("first TableField = parent %v key %v values %v kind %v ok %v", parent, key, fieldValues, fieldKind, firstOK)
	}
	if _, _, _, _, ok := p.Flow().Authored().Calls().Get(key); !ok {
		t.Fatalf("first TableField key %v is not f()", key)
	}
	_, _, secondValues, secondKind, secondRowOK := fields.Get(second)
	if !secondRowOK || secondKind != kind.FieldList {
		t.Fatalf("second TableField kind = %v/%v", secondKind, secondRowOK)
	}
	if _, finalOpen, ok := fields.Values(second); !ok || !finalOpen {
		t.Fatalf("final list field open tail = %v/%v", finalOpen, ok)
	}
	if tail := valuesTail(t, p, secondValues); tail == 0 {
		t.Fatal("final list field did not retain h() tail")
	}
	for _, term := range []keyspace.Term{key, first, valueAt(t, p, fieldValues, 0)} {
		if span, ok := p.Source().Identity().Span(term); !ok || span.StartLine != 1 {
			t.Fatalf("Table term %v has no line-one Source span", term)
		}
	}
}

func TestFlowNestedOutcomePropagationStaysInOutcomeOwner(t *testing.T) {
	p := parseBindLower(t, "local function f() do do return 1 end end end")
	function, ok := p.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing Function")
	}
	_, functionBody, _, functionOK := p.Flow().Authored().Functions().Get(function)
	if !functionOK {
		t.Fatal("missing Function Body")
	}
	returned := controlSourceAt(t, p, functionBody, 0)
	for {
		if _, _, ok := p.Flow().Authored().Control().Returns().Get(returned); ok {
			break
		}
		child, childOK := p.Source().Order().BodyAt(returned, 0)
		if !childOK {
			t.Fatal("nested Return is absent")
		}
		returned = child
	}
	exit, exitOK := p.Flow().Outcomes().ReturnExit(returned)
	outcome, outcomeOK := p.Flow().Outcomes().Get(exit)
	if !exitOK || !outcomeOK || outcome.Kind != kind.OutcomeReturn {
		t.Fatalf("Return outcome = %#v/%v", outcome, outcomeOK)
	}
	if functionBody == outcome.Body {
		t.Fatal("nested Return did not retain its immediate Body owner")
	}
}

// The witness stays at the four final owner vocabularies.  It deliberately
// does not reconstruct the retired per-term Program Mu/continuation planes.
func TestSourceControlExactWitnesses(t *testing.T) {
	for _, sample := range []struct {
		name        string
		input       string
		loops       int
		branches    int
		returns     int
		breaks      int
		labels      int
		gotos       int
		faults      int
		staticFault bool
	}{
		{"assign", "value = 1", 0, 0, 0, 0, 0, 0, 0, false},
		{"local-assign", "local value = 1", 0, 0, 0, 0, 0, 0, 0, false},
		{"call", "invoke()", 0, 0, 0, 0, 0, 0, 0, false},
		{"while", "while true do local value = 1 end", 1, 0, 0, 0, 0, 0, 0, false},
		{"repeat", "repeat local value = 1 until true", 1, 0, 0, 0, 0, 0, 0, false},
		{"if", "if true then local yes = 1 else local no = 2 end", 0, 1, 0, 0, 0, 0, 0, false},
		{"numeric-for", "for index = 1, 2 do local value = index end", 1, 0, 0, 0, 0, 0, 0, false},
		{"generic-for", "for key in iterate() do local seen = key end", 1, 0, 0, 0, 0, 0, 0, false},
		{"return", "return 1", 0, 0, 1, 0, 0, 0, 0, false},
		{"legal-break", "while true do break end", 1, 0, 0, 1, 0, 0, 0, false},
		{"label", "::done::", 0, 0, 0, 0, 1, 0, 0, false},
		{"backward-goto", "::again::\ngoto again", 0, 0, 0, 0, 1, 1, 0, false},
		{"undefined-goto", "goto missing", 0, 0, 0, 0, 0, 0, 1, false},
		{"static-undefined-goto", "type Snapshot = typeof(function()\ngoto missing\nend)", 0, 0, 0, 0, 0, 0, 1, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			entry, ok := p.Source().Index().Entry()
			if !ok {
				t.Fatal("missing Source entry")
			}
			if count, ok := p.Source().Order().BodyLen(entry); !ok || count == 0 {
				t.Fatalf("entry Source order = %d/%v", count, ok)
			}
			control := p.Flow().Authored().Control()
			if got := control.Loops().Count(); got != sample.loops {
				t.Fatalf("Loop count = %d, want %d", got, sample.loops)
			}
			if got := control.Branches().Count(); got != sample.branches {
				t.Fatalf("Branch count = %d, want %d", got, sample.branches)
			}
			if got := control.Returns().Count(); got != sample.returns {
				t.Fatalf("Return count = %d, want %d", got, sample.returns)
			}
			if got := control.Breaks().Count(); got != sample.breaks {
				t.Fatalf("Break count = %d, want %d", got, sample.breaks)
			}
			if got := control.Labels().Count(); got != sample.labels {
				t.Fatalf("Label count = %d, want %d", got, sample.labels)
			}
			if got := control.Gotos().Count(); got != sample.gotos {
				t.Fatalf("Goto count = %d, want %d", got, sample.gotos)
			}
			if got := p.Source().Faults().Count(); got != sample.faults {
				t.Fatalf("ControlFault count = %d, want %d", got, sample.faults)
			}
			if sample.staticFault {
				fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
				row, faultOK := p.Source().Faults().At(fault)
				if !faultOK || row.Kind != source.ControlFaultUndefinedGoto || !p.Flow().Containment().Static(fault) {
					t.Fatalf("static fault = %#v/%v static=%v", row, faultOK, p.Flow().Containment().Static(fault))
				}
			}
		})
	}
}

func TestFlowControlRowsKeepExactOperandsAndOutcomes(t *testing.T) {
	p := parseBindLower(t, "while condition() do break end\nreturn 1")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing entry")
	}
	loop := controlSourceAt(t, p, entry, 0)
	returned := controlSourceAt(t, p, entry, 1)
	control := p.Flow().Authored().Control()
	owner, body, loopKind, condition, loopOK := control.Loops().Get(loop)
	if !loopOK || owner != entry || body == 0 || condition == 0 || loopKind != kind.LoopWhile {
		t.Fatalf("Loop = owner %v body %v kind %v condition %v ok %v", owner, body, loopKind, condition, loopOK)
	}
	breakTerm, breakOK := control.Breaks().At(0)
	if !breakOK {
		t.Fatal("missing Break")
	}
	if target, ok := control.Breaks().Get(breakTerm); !ok || target != loop {
		t.Fatalf("Break target = %v/%v, want %v", target, ok, loop)
	}
	exit, exitOK := p.Flow().Outcomes().BreakExit(breakTerm)
	outcome, outcomeOK := p.Flow().Outcomes().Get(exit)
	if !exitOK || !outcomeOK || outcome.Body != body || outcome.Kind != kind.OutcomeBreak || outcome.Target != loop {
		t.Fatalf("Break outcome = %#v/%v", outcome, outcomeOK)
	}
	if _, values, returnOK := control.Returns().Get(returned); !returnOK || values == 0 {
		t.Fatalf("Return = values %v/%v", values, returnOK)
	}
}

func TestFlowCausalRecurrenceIsAttachedToBackwardGotoEdge(t *testing.T) {
	p := parseBindLower(t, "::again::\ngoto again")
	label, labelOK := p.Flow().Authored().Control().Labels().At(0)
	jump, jumpOK := p.Flow().Authored().Control().Gotos().At(0)
	if !labelOK || !jumpOK {
		t.Fatalf("label/goto = %v/%v %v/%v", label, labelOK, jump, jumpOK)
	}
	if _, target, ok := p.Flow().Authored().Control().Gotos().Get(jump); !ok || target != label {
		t.Fatalf("Goto target = %v/%v, want %v", target, ok, label)
	}
	edges := p.Flow().Causal().Edges()
	found := false
	for index := 0; index < edges.Count(); index++ {
		edge, edgeOK := edges.At(index)
		found = found || edgeOK && edge.Mu == label
	}
	if !found {
		t.Fatal("backward Goto has no recurrence Edge")
	}
}

func TestSourceControlFaultVerticalResolvesEachLexicalBodyIndependently(t *testing.T) {
	p := parseBindLower(t, `do
  goto inner
  local inner = 1
  ::inner::
  inner = inner
end
goto outer
local outer = 2
::outer::
outer = outer`)
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("Entry is absent")
	}
	innerBody := controlSourceAt(t, p, entry, 0)
	if parent, ok := p.Source().Index().BodyParent(innerBody); !ok || parent != entry {
		t.Fatalf("inner Body parent = %v/%v, want %v", parent, ok, entry)
	}
	assertEnteredLocalFault(t, p, innerBody, 0)
	assertEnteredLocalFault(t, p, entry, 1)
	if p.Source().Faults().Count() != 2 {
		t.Fatalf("ControlFaultCount = %d, want one per lexical Body", p.Source().Faults().Count())
	}
	if p.Flow().Authored().Control().Gotos().Count() != 0 {
		t.Fatalf("invalid controls became executable Gotos: %d", p.Flow().Authored().Control().Gotos().Count())
	}
}

func assertEnteredLocalFault(t *testing.T, p *program.Program, body keyspace.Term, sourceIndex int) {
	t.Helper()
	fault := controlSourceAt(t, p, body, sourceIndex)
	bind := controlSourceAt(t, p, body, sourceIndex+1)
	label := controlSourceAt(t, p, body, sourceIndex+2)
	blocker := boundCell(t, p, bind, 0)
	got, ok := p.Source().Faults().At(fault)
	if !ok || got.Owner != body || got.Kind != source.ControlFaultGotoEntersLocal ||
		got.Label != label || got.Blocker != blocker {
		t.Fatalf(
			"ControlFault(%v) = owner %v kind %v label %v blocker %v ok %v, want owner %v kind %v label %v blocker %v",
			fault, got.Owner, got.Kind, got.Label, got.Blocker, ok,
			body, source.ControlFaultGotoEntersLocal, label, blocker,
		)
	}
}

func TestSourceBodiesKeepOrderAndParents(t *testing.T) {
	p := parseBindLower(t, "\ndo\n  local normal = 1\n  do local nested = 2 end\nend\ndo return 3 end\nreturn 4\n")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry")
	}
	if count, ok := p.Source().Order().BodyLen(entry); !ok || count != 3 {
		t.Fatalf("entry Source order = %d/%v, want 3", count, ok)
	}
	normal := controlSourceAt(t, p, entry, 0)
	terminal := controlSourceAt(t, p, entry, 1)
	for _, body := range []keyspace.Term{normal, terminal} {
		if parent, ok := p.Source().Index().BodyParent(body); !ok || parent != entry {
			t.Fatalf("Body parent = %v/%v, want %v", parent, ok, entry)
		}
	}
	nested := controlSourceAt(t, p, normal, 1)
	if parent, ok := p.Source().Index().BodyParent(nested); !ok || parent != normal {
		t.Fatalf("nested Body parent = %v/%v, want %v", parent, ok, normal)
	}
	if body, offset, _, ok := p.Source().Index().Position(nested); !ok || body != normal || offset != 1 {
		t.Fatalf("nested Source position = body %v offset %d ok %v", body, offset, ok)
	}
}

func TestFlowDirectFunctionsDistinguishDirectAndOrdinaryCalls(t *testing.T) {
	for _, sample := range []struct {
		name  string
		input string
		want  bool
	}{
		{"direct-recursion", "local function f() return f() end", true},
		{"ordinary-initializer", "local f = function() return f() end", false},
		{"preinstallation", "local f\nf()\nf = function() return 1 end", false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			call, ok := p.Flow().Authored().Calls().At(0)
			if !ok {
				t.Fatal("missing Call")
			}
			direct, directOK := p.Flow().DirectFunctions().Call(call)
			if sample.want && (!directOK || direct == 0) {
				t.Fatalf("DirectFunctions.Call = %v/%v, want Function", direct, directOK)
			}
			if !sample.want && (directOK || direct != 0) {
				t.Fatalf("DirectFunctions.Call = %v/%v, want absent", direct, directOK)
			}
		})
	}
}

func TestSourceExactKeysNormalizeWithoutSharingOccurrences(t *testing.T) {
	p := parseBindLower(t, "local t = {}\nt[-0.0] = 1\nt[0] = 2\nreturn t")
	exact := p.Flow().Authored().Access().Exact()
	if exact.Count() != 2 {
		t.Fatalf("exact Lens count = %d, want 2", exact.Count())
	}
	left, _ := exact.At(0)
	right, _ := exact.At(1)
	if left == right {
		t.Fatal("distinct numeric Lens occurrences were shared")
	}
	_, _, leftSource, leftKind, leftOK := exact.Get(left)
	_, _, rightSource, rightKind, rightOK := exact.Get(right)
	if !leftOK || !rightOK || leftKind != kind.FieldExact || rightKind != kind.FieldExact {
		t.Fatalf("exact Lens rows = %v/%v %v/%v", leftKind, leftOK, rightKind, rightOK)
	}
	leftKey, leftKeyOK := exactLiteralKey(t, p, leftSource)
	rightKey, rightKeyOK := exactLiteralKey(t, p, rightSource)
	if !leftKeyOK || !rightKeyOK || leftKey != rightKey {
		t.Fatalf("-0.0/0 exact keys = %v/%v %v/%v", leftKey, leftKeyOK, rightKey, rightKeyOK)
	}
	value, ok := p.Source().Keys().Exact(leftKey)
	if !ok || value != (keyspace.LiteralValue{Kind: keyspace.LiteralInteger}) {
		t.Fatalf("normalized zero key = %#v/%v", value, ok)
	}
}

func TestSourceExactKeyEnumerationIsCanonicalAndAllocationFree(t *testing.T) {
	p := parseBindLower(t, "return {[true] = 1, [false] = 2, [7] = 3, [1.5] = 4, field = 5, [-0.0] = 6, [0] = 7, [7.0] = 8}")
	keys := p.Source().Keys()
	want := []keyspace.LiteralValue{
		{Kind: keyspace.LiteralBool},
		{Kind: keyspace.LiteralBool, Bool: true},
		{Kind: keyspace.LiteralInteger},
		{Kind: keyspace.LiteralInteger, Integer: 7},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1.5)},
		{Kind: keyspace.LiteralString, String: "field"},
	}
	if keys.ExactCount() != len(want) {
		t.Fatalf("Source exact key count = %d, want %d", keys.ExactCount(), len(want))
	}
	for index, expected := range want {
		key, value, ok := keys.ExactAt(index)
		if !ok || key == 0 || value != expected {
			t.Fatalf("Source ExactAt(%d) = %v/%#v/%v, want nonzero/%#v/true", index, key, value, ok, expected)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		exactKeySink, _, exactKeyOK = keys.ExactAt(3)
	}); allocations != 0 {
		t.Fatalf("Source ExactAt allocations = %v, want 0", allocations)
	}
}

func TestFlowImplicitReadsKeepFirstGlobalOccurrenceOrder(t *testing.T) {
	p := parseBindLower(t, "return first, second, first, second")
	reads := p.Flow().Authored().Storage().Reads()
	if reads.ImplicitCount() != 2 {
		t.Fatalf("implicit Read count = %d, want 2", reads.ImplicitCount())
	}
	for index, want := range []string{"first", "second"} {
		read, _ := reads.ImplicitAt(index)
		_, cell, _, readOK := reads.Get(read)
		_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(cell)
		value, keyOK := p.Source().Keys().Exact(key)
		if !readOK || !cellOK || !keyOK || value.Kind != keyspace.LiteralString || value.String != want {
			t.Fatalf("implicit Read[%d] = cell %v key %#v read/cell/key=%v/%v/%v, want %q", index, cell, value, readOK, cellOK, keyOK, want)
		}
	}
}

func TestStaticFunctionContractsKeepOmittedAndExplicitEmptyReturns(t *testing.T) {
	p := parseBindLower(t, "\nlocal function inferred() end\nlocal function empty(): () end\nreturn inferred, empty\n")
	functions := p.Flow().Authored().Functions()
	for index, wantKnown := range []bool{false, true} {
		function, _ := functions.At(index)
		known, ok := p.Static().Contracts().Functions().Get(function)
		if !ok || known != wantKnown {
			t.Fatalf("Function contract[%d] = %v/%v, want %v/true", index, known, ok, wantKnown)
		}
		if count, ok := p.Static().Contracts().Functions().ReturnCount(function); !ok || count != 0 {
			t.Fatalf("Function return count[%d] = %d/%v, want 0/true", index, count, ok)
		}
	}
}

var (
	exactKeySink keyspace.Key
	exactKeyOK   bool
)

var _ flow.CellKind
