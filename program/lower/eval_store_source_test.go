package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

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
