package coordinate

import (
	"sync/atomic"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/lower"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestTableOnlyAdmitsEntryOrSelectedBodyActivation(t *testing.T) {
	l, shard, p := coordinateLink(t, `
local function f()
  return 1
end
f()
return 0
`)
	table, ok := New(l)
	if !ok {
		t.Fatal("New rejected sealed Link")
	}
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("missing Program Entry")
	}
	root, ok := table.InternRoot(shard, entry)
	if !ok {
		t.Fatal("entry root rejected")
	}
	if candidate, gotShard, gotTerm, ok := table.Semantic(root); !ok || candidate != (link.Candidate{}) || gotShard != shard || gotTerm != entry {
		t.Fatalf("root semantic=(%v,%v,%v,%v)", candidate, gotShard, gotTerm, ok)
	}
	entryLocal, ok := p.BodyFirst(entry)
	if !ok {
		t.Fatal("missing ordinary entry-body occurrence")
	}
	rootLocal, ok := table.InternRoot(shard, entryLocal)
	if !ok {
		t.Fatal("ordinary entry-body occurrence rejected")
	}
	if rootLocal == root {
		t.Fatal("distinct entry Program locals merged")
	}
	if candidate, gotShard, gotTerm, ok := table.Semantic(rootLocal); !ok || candidate != (link.Candidate{}) || gotShard != shard || gotTerm != entryLocal {
		t.Fatalf("entry local semantic=(%v,%v,%v,%v)", candidate, gotShard, gotTerm, ok)
	}
	for _, exit := range []struct {
		name string
		at   func(program.Term) (program.Term, bool)
	}{
		{name: "normal", at: p.BodyNormalExit},
		{name: "return", at: p.BodyReturnExit},
		{name: "throw", at: p.BodyThrowExit},
		{name: "yield", at: p.BodyYieldExit},
		{name: "cancel", at: p.BodyCancelExit},
	} {
		term, ok := exit.at(entry)
		if !ok {
			t.Fatalf("missing declared entry %s exit", exit.name)
		}
		if _, ok := table.InternRoot(shard, term); !ok {
			t.Fatalf("entry %s exit rejected", exit.name)
		}
	}
	_, body := coordinateFunctionBody(t, p, 0)
	if _, ok := table.InternRoot(shard, body); ok {
		t.Fatal("nested function Body became a project root")
	}
	nestedReturn, ok := p.BodyReturnExit(body)
	if !ok {
		t.Fatal("missing nested return exit")
	}
	if _, ok := table.InternRoot(shard, nestedReturn); ok {
		t.Fatal("nested Function outcome became a project root")
	}
	cell, ok := p.CellAt(0)
	if !ok {
		t.Fatal("missing static Cell")
	}
	if _, ok := table.InternRoot(shard, cell); ok {
		t.Fatal("non-executable Cell became a project root")
	}
	candidate := coordinateCandidatesForBody(t, l, shard, body)[0]
	inside, ok := p.BodyFirst(body)
	if !ok {
		t.Fatal("missing function body occurrence")
	}
	coordinate, ok := table.InternCandidate(candidate, shard, inside)
	if !ok {
		t.Fatal("selected body occurrence rejected")
	}
	if gotCandidate, gotShard, gotTerm, ok := table.Semantic(coordinate); !ok || gotShard != shard || gotTerm != inside || compareCandidates(t, l, gotCandidate, candidate) != 0 {
		t.Fatalf("candidate semantic=(%v,%v,%v,%v)", gotCandidate, gotShard, gotTerm, ok)
	}
	for _, exit := range []struct {
		name string
		at   func(program.Term) (program.Term, bool)
	}{
		{name: "normal", at: p.BodyNormalExit},
		{name: "return", at: p.BodyReturnExit},
		{name: "throw", at: p.BodyThrowExit},
		{name: "yield", at: p.BodyYieldExit},
		{name: "cancel", at: p.BodyCancelExit},
	} {
		term, ok := exit.at(body)
		if !ok {
			t.Fatalf("missing declared selected-body %s exit", exit.name)
		}
		if _, ok := table.InternCandidate(candidate, shard, term); !ok {
			t.Fatalf("selected-body %s exit rejected", exit.name)
		}
	}
	if _, ok := table.InternCandidate(candidate, shard, entry); ok {
		t.Fatal("caller activation was admitted under callee Candidate")
	}
	if _, ok := table.InternCandidate(link.Candidate{}, shard, inside); ok {
		t.Fatal("zero Candidate was admitted as a body selection")
	}
	if _, ok := table.InternCandidate(candidate, 0, inside); ok {
		t.Fatal("zero Shard was admitted")
	}
	if _, ok := table.InternCandidate(candidate, shard, 0); ok {
		t.Fatal("zero Term was admitted")
	}
}

func TestTableKeepsDistinctCallsiteCandidatesForOneBody(t *testing.T) {
	l, shard, p := coordinateLink(t, `
local function f()
  return 1
end
f()
f()
`)
	_, body := coordinateFunctionBody(t, p, 0)
	candidates := coordinateCandidatesForBody(t, l, shard, body)
	if len(candidates) != 2 {
		t.Fatalf("body Candidates=%d, want 2", len(candidates))
	}
	table, ok := New(l)
	if !ok {
		t.Fatal("New rejected sealed Link")
	}
	first, ok := table.InternCandidate(candidates[0], shard, body)
	if !ok {
		t.Fatal("first Candidate rejected")
	}
	duplicate, ok := table.InternCandidate(candidates[0], shard, body)
	if !ok || duplicate != first {
		t.Fatal("exact Candidate/Term pair did not deduplicate")
	}
	second, ok := table.InternCandidate(candidates[1], shard, body)
	if !ok {
		t.Fatal("second Candidate rejected")
	}
	if second == first {
		t.Fatal("two callsites selecting one body merged coordinates")
	}
	if got := table.Count(); got != 2 {
		t.Fatalf("materialized coordinates=%d, want 2", got)
	}
}

func TestTableOrderDoesNotDependOnDemandOrder(t *testing.T) {
	l, shard, p := coordinateLink(t, `
local function f()
  return 1
end
f()
f()
`)
	_, body := coordinateFunctionBody(t, p, 0)
	candidates := coordinateCandidatesForBody(t, l, shard, body)
	if len(candidates) != 2 {
		t.Fatalf("body Candidates=%d, want 2", len(candidates))
	}
	first, _ := New(l)
	second, _ := New(l)
	if _, ok := first.InternCandidate(candidates[0], shard, body); !ok {
		t.Fatal("forward first Candidate rejected")
	}
	if _, ok := first.InternCandidate(candidates[1], shard, body); !ok {
		t.Fatal("forward second Candidate rejected")
	}
	if _, ok := second.InternCandidate(candidates[1], shard, body); !ok {
		t.Fatal("reverse second Candidate rejected")
	}
	if _, ok := second.InternCandidate(candidates[0], shard, body); !ok {
		t.Fatal("reverse first Candidate rejected")
	}
	for index := 0; index < first.Count(); index++ {
		left, _ := first.OrderedAt(index)
		right, _ := second.OrderedAt(index)
		leftCandidate, leftShard, leftTerm, leftOK := first.Semantic(left)
		rightCandidate, rightShard, rightTerm, rightOK := second.Semantic(right)
		if !leftOK || !rightOK || leftShard != rightShard || leftTerm != rightTerm || compareCandidates(t, l, leftCandidate, rightCandidate) != 0 {
			t.Fatalf("canonical order differs at %d", index)
		}
	}
}

func TestCoordinateCompareUsesSemanticOrderRatherThanDemandSlot(t *testing.T) {
	l, shard, p := coordinateLink(t, `
local function f()
  return 1
end
f()
f()
`)
	_, body := coordinateFunctionBody(t, p, 0)
	candidates := coordinateCandidatesForBody(t, l, shard, body)
	if len(candidates) != 2 || compareCandidates(t, l, candidates[0], candidates[1]) >= 0 {
		t.Fatal("fixture Candidates are not canonically ordered")
	}
	table, ok := New(l)
	if !ok {
		t.Fatal("New rejected sealed Link")
	}
	// Demand the later semantic location first. Its storage slot is therefore
	// smaller, but Compare must retain Link's canonical Candidate order.
	later, ok := table.InternCandidate(candidates[1], shard, body)
	if !ok {
		t.Fatal("later Candidate rejected")
	}
	earlier, ok := table.InternCandidate(candidates[0], shard, body)
	if !ok {
		t.Fatal("earlier Candidate rejected")
	}
	laterSlot, laterOK := table.Slot(later)
	earlierSlot, earlierOK := table.Slot(earlier)
	if !laterOK || !earlierOK || laterSlot >= earlierSlot {
		t.Fatal("fixture did not retain reverse demand slots")
	}
	if order := earlier.Compare(later); order >= 0 {
		t.Fatalf("semantic Coordinate order=%d, want earlier Candidate before later", order)
	}
	if order := later.Compare(earlier); order <= 0 {
		t.Fatalf("reverse semantic Coordinate order=%d, want later Candidate after earlier", order)
	}
}

func TestTableOwnerMintFailsClosedAtExhaustion(t *testing.T) {
	var sequence atomic.Uint64
	sequence.Store(^uint64(0))
	if owner, ok := mintOwner(&sequence); ok || owner != 0 {
		t.Fatal("Table owner mint wrapped instead of failing closed")
	}
}

func TestTableRejectsForeignHandleAndDoesNotPreMaterialize(t *testing.T) {
	l, shard, p := coordinateLink(t, `local x = 1`)
	first, ok := New(l)
	if !ok {
		t.Fatal("first New rejected sealed Link")
	}
	second, ok := New(l)
	if !ok {
		t.Fatal("second New rejected sealed Link")
	}
	if first.Count() != 0 || second.Count() != 0 {
		t.Fatal("New materialized an inventory")
	}
	entry, _ := p.Entry()
	handle, ok := first.InternRoot(shard, entry)
	if !ok {
		t.Fatal("root rejected")
	}
	if slot, ok := first.Slot(handle); !ok || slot != 0 {
		t.Fatalf("root storage slot=(%d,%v), want 0/true", slot, ok)
	}
	if _, _, _, ok := second.Semantic(handle); ok {
		t.Fatal("foreign table accepted a handle")
	}
	if _, ok := second.Compare(handle, handle); ok {
		t.Fatal("foreign table ordered a handle")
	}
	if _, ok := second.Slot(handle); ok {
		t.Fatal("foreign table exposed a storage slot")
	}
	if second.Count() != 0 {
		t.Fatal("foreign handle lookup materialized state")
	}
}

func TestTableHotReuseDoesNotAllocate(t *testing.T) {
	l, shard, p := coordinateLink(t, ``)
	table, ok := New(l)
	if !ok {
		t.Fatal("New rejected sealed Link")
	}
	entry, _ := p.Entry()
	handle, ok := table.InternRoot(shard, entry)
	if !ok {
		t.Fatal("root rejected")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		got, ok := table.InternRoot(shard, entry)
		if !ok || got != handle {
			panic("root reuse failed")
		}
		if _, _, _, ok := table.Semantic(got); !ok {
			panic("semantic lookup failed")
		}
	}); allocations != 0 {
		t.Fatalf("hot root reuse allocations=%v", allocations)
	}
}

func TestCoordinateHotCompareDoesNotAllocate(t *testing.T) {
	l, shard, p := coordinateLink(t, `local first = 1; local second = 2`)
	table, ok := New(l)
	if !ok {
		t.Fatal("New rejected sealed Link")
	}
	firstTerm, secondTerm := coordinateIntegers(t, p)
	first, ok := table.InternRoot(shard, firstTerm)
	if !ok {
		t.Fatal("first root rejected")
	}
	second, ok := table.InternRoot(shard, secondTerm)
	if !ok {
		t.Fatal("second root rejected")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if first.Compare(second) == 0 || second.Compare(first) == 0 {
			panic("distinct Coordinates compared equal")
		}
	}); allocations != 0 {
		t.Fatalf("Coordinate.Compare allocated %.1f times", allocations)
	}
}

func coordinateLink(t *testing.T, text string) (*link.Link, link.Shard, *program.Program) {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "coordinate", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	l, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := l.ShardAt(0)
	if !ok {
		t.Fatal("missing Link shard")
	}
	return l, shard, p
}

func coordinateFunctionBody(t *testing.T, p *program.Program, index int) (program.Term, program.Term) {
	t.Helper()
	function, ok := p.FunctionAt(index)
	if !ok {
		t.Fatalf("missing Function %d", index)
	}
	_, body, _, ok := p.Function(function)
	if !ok {
		t.Fatalf("malformed Function %d", index)
	}
	return function, body
}

func coordinateIntegers(t *testing.T, p *program.Program) (program.Term, program.Term) {
	t.Helper()
	first, ok := p.IntegerAt(0)
	if !ok {
		t.Fatal("missing first Integer")
	}
	second, ok := p.IntegerAt(1)
	if !ok {
		t.Fatal("missing second Integer")
	}
	return first, second
}

func coordinateCandidatesForBody(t *testing.T, l *link.Link, shard link.Shard, body program.Term) []link.Candidate {
	t.Helper()
	var result []link.Candidate
	for applicationIndex := 0; applicationIndex < l.ApplicationCount(); applicationIndex++ {
		application, ok := l.ApplicationAt(applicationIndex)
		if !ok {
			t.Fatal("missing Link Application")
		}
		for candidateIndex := 0; candidateIndex < l.ApplicationCandidateCount(application); candidateIndex++ {
			candidate, ok := l.ApplicationCandidateAt(application, candidateIndex)
			if !ok {
				t.Fatal("missing Link Candidate")
			}
			candidateShard, candidateBody, ok := l.CandidateBody(candidate)
			if ok && candidateShard == shard && candidateBody == body {
				result = append(result, candidate)
			}
		}
	}
	if len(result) == 0 {
		t.Fatal("missing static body Candidate")
	}
	return result
}

func compareCandidates(t *testing.T, l *link.Link, left, right link.Candidate) int {
	t.Helper()
	order, ok := l.CompareCandidate(left, right)
	if !ok {
		t.Fatal("invalid Candidate comparison")
	}
	return order
}
