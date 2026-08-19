package executable

import (
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestResultDenominatorStaticAndFailClosedLaws(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 3
	counts[keyspace.FamilyCell] = 4
	counts[keyspace.FamilyTypeAlias] = 100_000
	counts[keyspace.FamilyKey] = 100_000
	counts[keyspace.FamilyControlFault] = 100_000
	counts[keyspace.FamilyOutcome] = 7
	r := newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4))
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	if !r.mark(body) || !r.mark(cell) {
		t.Fatal("runtime terms were not admitted to the dense planes")
	}
	if !r.Contains(body) || !r.Contains(cell) || r.Count() != 2 {
		t.Fatal("runtime membership query is not exact")
	}
	if r.Contains(keyspace.MakeTerm(keyspace.FamilyBody, 3)) {
		t.Fatal("unreachable Body entered executable membership")
	}
	for _, family := range []keyspace.Family{
		keyspace.FamilyTypeAlias, keyspace.FamilyKey, keyspace.FamilyControlFault,
	} {
		if got := r.FamilyCount(family); got != 100_000 {
			t.Fatalf("FamilyCount(%d) = %d, want denominator 100000", family, got)
		}
		if r.Contains(keyspace.MakeTerm(family, 1)) {
			t.Fatalf("static/namespace family %d entered executable membership", family)
		}
		if r.bits[family] != nil {
			t.Fatalf("static/namespace family %d retained a dead membership plane", family)
		}
	}
	for _, term := range []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyOutcome, 1),
		keyspace.MakeTerm(keyspace.FamilyBody, 4),
		keyspace.MakeTerm(keyspace.FamilyCell, 5),
		keyspace.MakeTerm(keyspace.FamilyCount, 1),
	} {
		if r.Contains(term) {
			t.Fatalf("Executable(%08x) = true for malformed/Outcome term", uint32(term))
		}
	}
	if r.FamilyCount(keyspace.FamilyOutcome) != 0 || r.bits[keyspace.FamilyOutcome] != nil {
		t.Fatal("Outcome retained a pre-Outcome membership denominator or bit plane")
	}
}

func TestResultPermutationAndRootlessCellLaws(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyCell] = 3
	counts[keyspace.FamilyValues] = 2
	terms := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyBody, 1),
		keyspace.MakeTerm(keyspace.FamilyCell, 1),
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyCell, 2),
	}
	left, right := newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)), newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4))
	for _, term := range terms {
		left.mark(term)
	}
	for index := len(terms) - 1; index >= 0; index-- {
		right.mark(terms[index])
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			if left.Contains(term) != right.Contains(term) {
				t.Fatalf("permutation changed membership for %08x", uint32(term))
			}
		}
	}
	if left.Contains(keyspace.MakeTerm(keyspace.FamilyCell, 3)) {
		t.Fatal("rootless Cell became executable")
	}
}

func TestExecutableQueryIsAllocationFree(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	r := newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4))
	r.mark(keyspace.MakeTerm(keyspace.FamilyBody, 1))
	term := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if allocations := testing.AllocsPerRun(1000, func() {
		if !r.Contains(term) {
			t.Fatal("stable executable query returned false")
		}
	}); allocations != 0 {
		t.Fatalf("Executable allocated %v objects per query", allocations)
	}
	if unsafe.Sizeof(r.sourceID) != unsafe.Sizeof(identity.ContentID{}) {
		t.Fatal("provenance is not a fixed-width Source identity")
	}
	if unsafe.Sizeof(r.flowID) != unsafe.Sizeof(identity.ContentID{}) || !Matches(r, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)) {
		t.Fatal("authored Flow provenance was not retained as the narrow value fence")
	}
	foreignSource := flowtest.ContentIDAt(1)
	foreignSource[0] ^= 0xff
	foreignFlowID := flowtest.ContentIDAt(2)
	foreignFlowID[0] ^= 0xff
	foreignStatic := flowtest.ContentIDAt(3)
	foreignStatic[0] ^= 0xff
	foreignModule := flowtest.ContentIDAt(4)
	foreignModule[0] ^= 0xff
	if Matches(r, foreignSource, flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)) ||
		Matches(r, flowtest.ContentIDAt(1), foreignFlowID, flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)) ||
		Matches(r, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), foreignStatic, flowtest.ContentIDAt(4)) ||
		Matches(r, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), foreignModule) {
		t.Fatal("Result accepted a foreign owner identity")
	}
	foreignFlow := newResult(counts, flowtest.ContentIDAt(1), identity.ContentID{}, flowtest.ContentIDAt(3), flowtest.ContentIDAt(4))
	if foreignFlow.Contains(term) || foreignFlow.Count() != 0 || Matches(foreignFlow, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)) {
		t.Fatal("missing authored Flow provenance did not fail closed")
	}
	for name, malformed := range map[string]*Result{
		"source": newResult(counts, identity.ContentID{}, flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)),
		"flow":   foreignFlow,
		"static": newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), identity.ContentID{}, flowtest.ContentIDAt(4)),
		"module": newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), identity.ContentID{}),
	} {
		if malformed.Count() != 0 || malformed.Contains(term) || malformed.FamilyCount(keyspace.FamilyBody) != 0 ||
			Matches(malformed, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)) {
			t.Fatalf("%s-unavailable Result was queryable or matched owners", name)
		}
	}
}

func TestDeepClosureUsesIterativeWorklist(t *testing.T) {
	const depth = 50_000
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = depth
	result := newResult(counts, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4))
	work := make([]keyspace.Term, 0, depth)
	for ordinal := uint32(1); ordinal <= depth; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		result.mark(term)
		work = append(work, term)
	}
	closed, err := closeOperands(structuredEmptyAuthored(), structuredEmptySource(), nil, counts, rootSeed{result: result, work: work})
	if err != nil {
		t.Fatalf("iterative closure rejected deep body worklist: %v", err)
	}
	if closed.Count() != depth || !closed.Contains(keyspace.MakeTerm(keyspace.FamilyBody, depth)) {
		t.Fatal("deep worklist lost executable membership")
	}
}

// These zero views are sufficient for the Body-only deep-worklist law; the
// real Seal path validates both owners before constructing a walker.
func structuredEmptyAuthored() authored.View { return authored.View{} }
func structuredEmptySource() source.View     { return source.View{} }
