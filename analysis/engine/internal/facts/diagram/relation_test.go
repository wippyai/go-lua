package diagram

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type relationKey uint64
type relationFactor uint64

const relationFactorID relationFactor = 1

type relationFixture struct {
	diagram *Diagram[relationFactor, relationKey, uint8]
	all     support.Mask
	atom    support.Mask
	values  map[uint8]terminal.ID[uint8]
}

func newRelationFixture(t testing.TB) relationFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("support work")
	}
	all := regions.True()
	atom, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("support setup")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for _, value := range []uint8{10, 20, 30} {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("admit %d", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal")
	}
	diagram, ok := New(Config[relationFactor, relationKey, uint8]{
		Factors:   []relationFactor{relationFactorID},
		Terminals: values,
		Guards:    manager,
	})
	if !ok {
		t.Fatal("diagram")
	}
	return relationFixture{diagram: diagram, all: all, atom: atom, values: ids}
}

type relationWrite struct {
	key   relationKey
	when  support.Mask
	value uint8
}

func (fixture relationFixture) sealed(t testing.TB, writes ...relationWrite) Root[relationFactor, relationKey, uint8] {
	t.Helper()
	builder := fixture.diagram.Begin()
	if builder == nil {
		t.Fatal("diagram builder")
	}
	root := fixture.diagram.Empty()
	for _, write := range writes {
		var ok bool
		root, ok = builder.Set(root, relationFactorID, write.key, write.when, fixture.values[write.value])
		if !ok {
			t.Fatalf("set key %d", write.key)
		}
	}
	root, ok := builder.Seal(root)
	if !ok {
		t.Fatal("root seal")
	}
	return root
}

type relationVisit struct {
	key         relationKey
	left, right terminal.ID[uint8]
}

func TestRelateSoleFactorUnderStreamsUnionInOrderAndStops(t *testing.T) {
	fixture := newRelationFixture(t)
	left := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 3, when: fixture.all, value: 20},
		relationWrite{key: 7, when: fixture.all, value: 30},
	)
	right := fixture.sealed(t,
		relationWrite{key: 2, when: fixture.all, value: 20},
		relationWrite{key: 3, when: fixture.all, value: 30},
		relationWrite{key: 8, when: fixture.all, value: 10},
	)
	var visits []relationVisit
	if !fixture.diagram.RelateSoleFactorUnder(left, right, fixture.all, func(key relationKey, left, right terminal.ID[uint8]) bool {
		visits = append(visits, relationVisit{key: key, left: left, right: right})
		return true
	}) {
		t.Fatal("relation")
	}
	want := []relationVisit{
		{key: 1, left: fixture.values[10]},
		{key: 2, right: fixture.values[20]},
		{key: 3, left: fixture.values[20], right: fixture.values[30]},
		{key: 7, left: fixture.values[30]},
		{key: 8, right: fixture.values[10]},
	}
	if !reflect.DeepEqual(visits, want) {
		t.Fatalf("union visits = %#v, want %#v", visits, want)
	}
	visits = nil
	if fixture.diagram.RelateSoleFactorUnder(left, right, fixture.all, func(key relationKey, left, right terminal.ID[uint8]) bool {
		visits = append(visits, relationVisit{key: key, left: left, right: right})
		return key != 2
	}) {
		t.Fatal("relation completed after callback rejection")
	}
	if want := []relationKey{1, 2}; len(visits) != len(want) || visits[0].key != want[0] || visits[1].key != want[1] {
		t.Fatalf("early visits = %#v, want keys %v", visits, want)
	}
}

func TestRelateSoleFactorUnderDoesNotSkipIdenticalRoots(t *testing.T) {
	fixture := newRelationFixture(t)
	root := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 1, when: fixture.atom, value: 20},
	)
	visits := 0
	if !fixture.diagram.RelateSoleFactorUnder(root, root, fixture.all, func(_ relationKey, left, right terminal.ID[uint8]) bool {
		visits++
		return left == right
	}) {
		t.Fatal("identical root relation")
	}
	if visits != 2 {
		t.Fatalf("identical root visits = %d, want one pair per supported FDD branch", visits)
	}
}

func TestRelateSoleFactorUnderMasksUnsupportedAndVisitsLowFirst(t *testing.T) {
	fixture := newRelationFixture(t)
	left := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 1, when: fixture.atom, value: 20},
	)
	right := fixture.sealed(t, relationWrite{key: 1, when: fixture.all, value: 10})
	var supported []relationVisit
	if !fixture.diagram.RelateSoleFactorUnder(left, right, fixture.atom, func(key relationKey, left, right terminal.ID[uint8]) bool {
		supported = append(supported, relationVisit{key: key, left: left, right: right})
		return left == fixture.values[20] && right == fixture.values[10]
	}) {
		t.Fatalf("supported branch did not remain observable: %#v", supported)
	}
	if len(supported) != 1 {
		t.Fatalf("supported visits = %#v, want one high branch", supported)
	}
	visits := 0
	if fixture.diagram.RelateSoleFactorUnder(left, right, fixture.all, func(_ relationKey, left, right terminal.ID[uint8]) bool {
		visits++
		// The false cofactor is 10/10; the true cofactor is 20/10. If the
		// walker ever changes its low-before-high order this assertion sees
		// the mismatch first and the count no longer proves the law.
		return left == right
	}) {
		t.Fatal("full relation accepted unequal high branch")
	}
	if visits != 2 {
		t.Fatalf("full relation visits = %d, want low then high", visits)
	}
	lowMismatch := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 20},
		relationWrite{key: 1, when: fixture.atom, value: 10},
	)
	visits = 0
	if fixture.diagram.RelateSoleFactorUnder(lowMismatch, right, fixture.all, func(_ relationKey, left, right terminal.ID[uint8]) bool {
		visits++
		return left == right
	}) {
		t.Fatal("low mismatch accepted")
	}
	if visits != 1 {
		t.Fatalf("low mismatch visits = %d, want early low-branch exit", visits)
	}
}

func TestCompareSoleFactorUnderAgreesWithConcreteOneAtomReference(t *testing.T) {
	fixture := newRelationFixture(t)
	left := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 1, when: fixture.atom, value: 20},
		relationWrite{key: 2, when: fixture.atom, value: 30},
	)
	right := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 2, when: fixture.all, value: 30},
	)
	scratch := NewSoleScratch[relationKey, uint8]()
	relation := func(left, right terminal.ID[uint8]) bool { return left == right }
	want := true
	for _, atom := range []bool{false, true} {
		for _, key := range []relationKey{1, 2, 3} {
			leftValue, leftPresent, leftValid := fixture.diagram.At(left, relationFactorID, key, func(_ guard.Atom) bool { return atom })
			rightValue, rightPresent, rightValid := fixture.diagram.At(right, relationFactorID, key, func(_ guard.Atom) bool { return atom })
			if !leftValid || !rightValid {
				t.Fatal("concrete reference root")
			}
			if !leftPresent {
				leftValue = terminal.ID[uint8]{}
			}
			if !rightPresent {
				rightValue = terminal.ID[uint8]{}
			}
			want = want && relation(leftValue, rightValue)
		}
	}
	if got := fixture.diagram.CompareSoleFactorUnder(left, right, fixture.all, scratch, relation); got != want {
		t.Fatalf("direct comparison = %t, concrete reference = %t", got, want)
	}
}

func TestCompareSoleFactorUnderIsLowFirstAndDoesNotChangeVisitorMultiplicity(t *testing.T) {
	fixture := newRelationFixture(t)
	left := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 20},
		relationWrite{key: 1, when: fixture.atom, value: 10},
	)
	right := fixture.sealed(t, relationWrite{key: 1, when: fixture.all, value: 10})
	scratch := NewSoleScratch[relationKey, uint8]()
	visits := 0
	if fixture.diagram.CompareSoleFactorUnder(left, right, fixture.all, scratch, func(left, right terminal.ID[uint8]) bool {
		visits++
		return left == right
	}) {
		t.Fatal("low mismatch accepted")
	}
	if visits != 1 {
		t.Fatalf("direct comparison visits = %d, want first low failure", visits)
	}
	streamed := 0
	if fixture.diagram.RelateSoleFactorUnder(left, right, fixture.all, func(_ relationKey, left, right terminal.ID[uint8]) bool {
		streamed++
		return left == right
	}) {
		t.Fatal("visitor relation accepted")
	}
	if streamed != 1 {
		t.Fatalf("visitor multiplicity changed = %d, want 1", streamed)
	}
}

func TestCompareSoleFactorUnderCachesOnlyThePureTriple(t *testing.T) {
	fixture := newRelationFixture(t)
	root := fixture.sealed(t,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 1, when: fixture.atom, value: 20},
		relationWrite{key: 2, when: fixture.all, value: 10},
		relationWrite{key: 2, when: fixture.atom, value: 20},
	)
	scratch := NewSoleScratch[relationKey, uint8]()
	pureVisits := 0
	if !fixture.diagram.CompareSoleFactorUnder(root, root, fixture.all, scratch, func(terminal.ID[uint8], terminal.ID[uint8]) bool {
		pureVisits++
		return true
	}) {
		t.Fatal("pure comparison")
	}
	if pureVisits != 2 {
		t.Fatalf("pure terminal checks = %d, want one shared two-branch FDD proof", pureVisits)
	}
	streamed := 0
	if !fixture.diagram.RelateSoleFactorUnder(root, root, fixture.all, func(_ relationKey, _ terminal.ID[uint8], _ terminal.ID[uint8]) bool {
		streamed++
		return true
	}) {
		t.Fatal("visitor relation")
	}
	if streamed != 4 {
		t.Fatalf("visitor occurrences = %d, want two branches at each key", streamed)
	}
}

func TestCompareSoleFactorUnderHandlesDeepPublicInput(t *testing.T) {
	const atoms = 96
	order := make([]guard.Atom, atoms)
	for index := range order {
		order[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(order)
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("support work")
	}
	all := regions.True()
	literals := make([]support.Mask, atoms)
	for index := range literals {
		var ok bool
		literals[index], ok = regions.Literal(guard.Atom(index+1), true)
		if !ok {
			t.Fatalf("literal %d", index)
		}
	}
	if !regions.Seal() {
		t.Fatal("support seal")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	base, ok := values.Admit(10)
	if !ok {
		t.Fatal("base terminal")
	}
	changed, ok := values.Admit(20)
	if !ok || !values.Seal() {
		t.Fatal("changed terminal")
	}
	diagram, ok := New(Config[relationFactor, relationKey, uint8]{
		Factors: []relationFactor{relationFactorID}, Terminals: values, Guards: manager,
	})
	if !ok {
		t.Fatal("diagram")
	}
	builder := diagram.Begin()
	root := diagram.Empty()
	root, ok = builder.Set(root, relationFactorID, 1, all, base)
	if !ok {
		t.Fatal("base write")
	}
	for _, literal := range literals {
		root, ok = builder.Set(root, relationFactorID, 1, literal, changed)
		if !ok {
			t.Fatal("deep write")
		}
	}
	root, ok = builder.Seal(root)
	if !ok {
		t.Fatal("root seal")
	}
	scratch := NewSoleScratch[relationKey, uint8]()
	relation := func(left, right terminal.ID[uint8]) bool { return left == right }
	if !diagram.CompareSoleFactorUnder(root, root, all, scratch, relation) {
		t.Fatal("warm direct comparison")
	}
	if !diagram.CompareSoleFactorUnder(root, root, all, scratch, relation) {
		t.Fatal("reused direct comparison")
	}
	work := values.Begin()
	mergeBuilder := diagram.BeginWithTerminals(work)
	merged, ok := mergeBuilder.MergeSoleFactor(root, root, all, all, all, scratch, nil, func(_ relationKey, left, _ terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		return left, true
	}, nil, nil)
	if !ok {
		t.Fatal("deep fused merge")
	}
	merged, ok = mergeBuilder.Seal(merged)
	if !ok {
		t.Fatal("deep fused merge seal")
	}
	if !diagram.Equal(merged, root) {
		t.Fatal("deep proved no-op did not preserve its fact relation")
	}
}

func BenchmarkCompareSoleFactorUnderWarm(b *testing.B) {
	fixture := newRelationFixture(b)
	left := fixture.sealed(b,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 1, when: fixture.atom, value: 20},
		relationWrite{key: 2, when: fixture.all, value: 30},
	)
	right := fixture.sealed(b,
		relationWrite{key: 1, when: fixture.all, value: 10},
		relationWrite{key: 2, when: fixture.all, value: 30},
	)
	scratch := NewSoleScratch[relationKey, uint8]()
	relation := func(terminal.ID[uint8], terminal.ID[uint8]) bool { return true }
	if !fixture.diagram.CompareSoleFactorUnder(left, right, fixture.all, scratch, relation) {
		b.Fatal("warm comparison")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !fixture.diagram.CompareSoleFactorUnder(left, right, fixture.all, scratch, relation) {
			b.Fatal("comparison")
		}
	}
}
