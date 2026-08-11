package candidates

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestCandidateGenericLoopFixedHeaderLaw(t *testing.T) {
	fixture := openCandidateFixture(t, genericLoopCandidateSpec())
	result, err := Seal(fixture.sourceView.Identity(), fixture.flowView, fixture.proof,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	loop := candidateTerm(keyspace.FamilyLoop, 1)
	if result.GenericLoop().Count() != 1 || !result.GenericLoop().Contains(loop) {
		t.Fatalf("GenericLoop = %d/%v, want one fixed-header GenericFor", result.GenericLoop().Count(), result.GenericLoop().Contains(loop))
	}
	if result.GenericLoop().Contains(candidateTerm(keyspace.FamilyValues, 1)) || result.GenericLoop().Contains(candidateTerm(keyspace.FamilyLoop, 2)) {
		t.Fatal("GenericLoop accepted a non-loop or out-of-range term")
	}
}

func TestCandidateGenericLoopFixedPrefixOpenTailLaw(t *testing.T) {
	fixture := openCandidateFixture(t, genericLoopFixedPrefixOpenTailSpec())
	result, err := Seal(fixture.sourceView.Identity(), fixture.flowView, fixture.proof,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	loop := candidateTerm(keyspace.FamilyLoop, 1)
	view := result.GenericLoop()
	if view.Count() != 1 || !view.Contains(loop) {
		t.Fatalf("GenericLoop = %d/%v, want one fixed-prefix open-tail GenericFor", view.Count(), view.Contains(loop))
	}
	if got, ok := view.At(0); !ok || got != loop {
		t.Fatalf("GenericLoop.At(0) = %08x/%v, want %08x/true", uint32(got), ok, uint32(loop))
	}
}

func TestCandidateGenericLoopRejectsOpenTailOtherFormsAndDeadRows(t *testing.T) {
	for _, row := range []struct {
		name string
		spec func() candidateSpec
	}{
		{name: "open-tail-only", spec: genericLoopOpenTailSpec},
		{name: "numeric-for", spec: genericLoopNumericSpec},
		{name: "while", spec: genericLoopWhileSpec},
		{name: "dead", spec: genericLoopDeadSpec},
	} {
		t.Run(row.name, func(t *testing.T) {
			fixture := openCandidateFixture(t, row.spec())
			result, err := Seal(fixture.sourceView.Identity(), fixture.flowView, fixture.proof,
				fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
			if err != nil {
				t.Fatalf("candidates.Seal: %v", err)
			}
			loop := candidateTerm(keyspace.FamilyLoop, 1)
			if result.GenericLoop().Count() != 0 || result.GenericLoop().Contains(loop) {
				t.Fatalf("GenericLoop = %d/%v, want absent", result.GenericLoop().Count(), result.GenericLoop().Contains(loop))
			}
		})
	}
}

func TestCandidateGenericLoopQueriesAreAllocationFree(t *testing.T) {
	loop := candidateTerm(keyspace.FamilyLoop, 1)
	result := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets:  bucketStore{genericLoop: []keyspace.Term{loop}},
		classes:  classStore{loopClass: []uint8{genericLoopCandidate}},
	}
	view := result.GenericLoop()
	if allocations := testing.AllocsPerRun(1000, func() {
		if view.Count() != 1 || !view.Contains(loop) {
			t.Fatal("stable GenericLoop query returned an incorrect result")
		}
		_, _ = view.At(0)
	}); allocations != 0 {
		t.Fatalf("GenericLoop query allocated %v objects per run", allocations)
	}
}

func TestCandidateGenericLoopPermutationCapacityAndAPILaws(t *testing.T) {
	first := candidateTerm(keyspace.FamilyLoop, 1)
	second := candidateTerm(keyspace.FamilyLoop, 2)
	result := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets:  bucketStore{genericLoop: []keyspace.Term{second, first}},
		classes:  classStore{loopClass: []uint8{genericLoopCandidate, genericLoopCandidate}},
	}
	view := result.GenericLoop()
	if view.Count() != 2 {
		t.Fatalf("GenericLoop.Count() = %d, want 2", view.Count())
	}
	for index, want := range []keyspace.Term{second, first} {
		got, ok := view.At(index)
		if !ok || got != want || !view.Contains(want) {
			t.Fatalf("GenericLoop[%d] = %08x/%v, Contains = %v; want %08x/true", index, uint32(got), ok, view.Contains(want), uint32(want))
		}
	}
	if _, ok := view.At(-1); ok {
		t.Fatal("GenericLoop.At(-1) accepted")
	}
	if _, ok := view.At(view.Count()); ok {
		t.Fatal("GenericLoop.At(Count()) accepted")
	}
	for _, foreign := range []keyspace.Term{
		0,
		candidateTerm(keyspace.FamilyUnary, 1),
		candidateTerm(keyspace.FamilyValues, 1),
		candidateTerm(keyspace.FamilyLoop, 3),
	} {
		if view.Contains(foreign) {
			t.Fatalf("GenericLoop.Contains(%08x) accepted foreign/out-of-range Term", uint32(foreign))
		}
	}

	const members = 10_000
	scaled := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		classes:  classStore{loopClass: make([]uint8, members)},
	}
	for ordinal := 1; ordinal <= members; ordinal++ {
		scaled.buckets.genericLoop = append(scaled.buckets.genericLoop, candidateTerm(keyspace.FamilyLoop, uint32(ordinal)))
		scaled.classes.loopClass[ordinal-1] = genericLoopCandidate
	}
	if got := scaled.GenericLoop().Count(); got != members || cap(scaled.buckets.genericLoop) > members*2 {
		t.Fatalf("GenericLoop count/capacity = %d/%d for %d members", got, cap(scaled.buckets.genericLoop), members)
	}
}

func TestCandidateGenericLoopForeignProvenanceRejected(t *testing.T) {
	first := openCandidateFixture(t, genericLoopCandidateSpec())
	foreign := openCandidateFixture(t, candidateIntegrationSpec())
	if _, err := Seal(first.sourceView.Identity(), foreign.flowView, first.proof,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("GenericLoop Seal accepted a foreign Flow proof")
	}
	if _, err := Seal(foreign.sourceView.Identity(), first.flowView, first.proof,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("GenericLoop Seal accepted a foreign Source identity")
	}
}

func genericLoopCandidateSpec() candidateSpec {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody:   2,
		keyspace.FamilyNil:    1,
		keyspace.FamilyValues: 1,
		keyspace.FamilyCell:   1,
		keyspace.FamilyLoop:   1,
	}
	body := candidateTerm(keyspace.FamilyBody, 1)
	child := candidateTerm(keyspace.FamilyBody, 2)
	values := candidateTerm(keyspace.FamilyValues, 1)
	cell := candidateTerm(keyspace.FamilyCell, 1)
	return candidateSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{candidateTerm(keyspace.FamilyLoop, 1)}, nil},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{candidateTerm(keyspace.FamilyNil, 1)},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: child}},
			},
			Control: authored.ControlInput{
				Loops: []authored.Loop{{Owner: body, Body: child, Kind: kind.LoopGenericFor, Control: values, Cells: authored.Range{End: 1}}},
				Cells: []keyspace.Term{cell},
			},
		},
	}
}

func genericLoopOpenTailSpec() candidateSpec {
	spec := genericLoopCandidateSpec()
	body := candidateTerm(keyspace.FamilyBody, 1)
	child := candidateTerm(keyspace.FamilyBody, 2)
	vararg := candidateTerm(keyspace.FamilyVararg, 1)
	varargCell := candidateTerm(keyspace.FamilyCell, 2)
	spec.counts[keyspace.FamilyVararg] = 1
	spec.counts[keyspace.FamilyCell] = 2
	spec.counts[keyspace.FamilyValues] = 2
	spec.counts[keyspace.FamilyReturn] = 1
	returnValues := candidateTerm(keyspace.FamilyValues, 2)
	returnTerm := candidateTerm(keyspace.FamilyReturn, 1)
	spec.rows[0] = []keyspace.Term{returnTerm, candidateTerm(keyspace.FamilyLoop, 1)}
	spec.flow.Values.Rows[0] = authored.Value{Owner: body, Tail: vararg}
	spec.flow.Values.Rows = append(spec.flow.Values.Rows, authored.Value{Owner: body, Fixed: authored.Range{End: 1}})
	spec.flow.Values.Terms = []keyspace.Term{candidateTerm(keyspace.FamilyNil, 1)}
	spec.flow.Control.Returns = []authored.Return{{Owner: body, Values: returnValues}}
	spec.flow.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: child},
		{Kind: authored.CellLocal, Body: body},
	}
	spec.flow.Storage.Varargs = []authored.Vararg{{Owner: body, Cell: varargCell}}
	return spec
}

func genericLoopFixedPrefixOpenTailSpec() candidateSpec {
	spec := genericLoopCandidateSpec()
	body := candidateTerm(keyspace.FamilyBody, 1)
	child := candidateTerm(keyspace.FamilyBody, 2)
	vararg := candidateTerm(keyspace.FamilyVararg, 1)
	varargCell := candidateTerm(keyspace.FamilyCell, 2)
	spec.counts[keyspace.FamilyVararg] = 1
	spec.counts[keyspace.FamilyCell] = 2
	spec.counts[keyspace.FamilyValues] = 2
	spec.counts[keyspace.FamilyReturn] = 1
	spec.counts[keyspace.FamilyNil] = 2
	spec.nilOwners = []keyspace.Term{body, body}
	spec.flow.Values.Rows[0] = authored.Value{Owner: body, Fixed: authored.Range{End: 1}, Tail: vararg}
	spec.flow.Values.Rows = append(spec.flow.Values.Rows, authored.Value{Owner: body, Fixed: authored.Range{Start: 1, End: 2}})
	spec.flow.Values.Terms = []keyspace.Term{
		candidateTerm(keyspace.FamilyNil, 1),
		candidateTerm(keyspace.FamilyNil, 2),
	}
	returnTerm := candidateTerm(keyspace.FamilyReturn, 1)
	spec.rows[0] = []keyspace.Term{candidateTerm(keyspace.FamilyLoop, 1), returnTerm}
	spec.flow.Control.Returns = []authored.Return{{Owner: body, Values: candidateTerm(keyspace.FamilyValues, 2)}}
	spec.flow.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: child},
		{Kind: authored.CellLocal, Body: body},
	}
	spec.flow.Storage.Varargs = []authored.Vararg{{Owner: body, Cell: varargCell}}
	return spec
}

func genericLoopNumericSpec() candidateSpec {
	spec := genericLoopCandidateSpec()
	body := candidateTerm(keyspace.FamilyBody, 1)
	spec.counts[keyspace.FamilyNil] = 2
	spec.flow.Values.Rows[0].Fixed.End = 2
	spec.flow.Values.Terms = []keyspace.Term{
		candidateTerm(keyspace.FamilyNil, 1),
		candidateTerm(keyspace.FamilyNil, 2),
	}
	spec.flow.Control.Loops[0].Kind = kind.LoopNumericFor
	spec.nilOwners = []keyspace.Term{body, body}
	return spec
}

func genericLoopWhileSpec() candidateSpec {
	spec := genericLoopCandidateSpec()
	body := candidateTerm(keyspace.FamilyBody, 1)
	spec.counts[keyspace.FamilyValues] = 0
	spec.counts[keyspace.FamilyCell] = 0
	spec.flow.Values = authored.ValuesInput{}
	spec.flow.Storage.Cells = nil
	spec.flow.Control.Cells = nil
	spec.flow.Control.Loops[0].Kind = kind.LoopWhile
	spec.flow.Control.Loops[0].Control = candidateTerm(keyspace.FamilyNil, 1)
	spec.flow.Control.Loops[0].Cells = authored.Range{}
	spec.nilOwners = []keyspace.Term{body}
	return spec
}

func genericLoopDeadSpec() candidateSpec {
	spec := genericLoopCandidateSpec()
	body := candidateTerm(keyspace.FamilyBody, 1)
	returnTerm := candidateTerm(keyspace.FamilyReturn, 1)
	values := candidateTerm(keyspace.FamilyValues, 2)
	spec.counts[keyspace.FamilyReturn] = 1
	spec.counts[keyspace.FamilyValues] = 2
	spec.counts[keyspace.FamilyNil] = 2
	spec.rows[0] = []keyspace.Term{returnTerm, candidateTerm(keyspace.FamilyLoop, 1)}
	spec.flow.Values.Rows = append(spec.flow.Values.Rows, authored.Value{Owner: body, Fixed: authored.Range{Start: 1, End: 2}})
	spec.flow.Values.Terms = []keyspace.Term{candidateTerm(keyspace.FamilyNil, 1), candidateTerm(keyspace.FamilyNil, 2)}
	spec.flow.Control.Returns = []authored.Return{{Owner: body, Values: values}}
	return spec
}
