package semantic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestJoinContributionsManyMatchesFixedBinaryOrderAndAdmitsOnlyFinalValue(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	all := regions.True()
	if !regions.Seal() {
		t.Fatal("support seal")
	}
	fingerprints := 0
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal: func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 {
			fingerprints++
			return uint64(value)
		},
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for _, value := range []uint8{0, 1, 2, 3, 4, 8} {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("admit %d", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{Factors: []semanticFactor{semanticColumn}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}
	join := func(left, right uint8) (uint8, bool) {
		return left | right, true
	}
	domain, ok := New(facts, values, Operations[uint8]{
		Default: 0, Equal: func(left, right uint8) bool { return left == right }, Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join: join, Widen: join, Narrow: func(_, right uint8) (uint8, bool) { return right, true }, LessOrEq: func(left, right uint8) bool { return left&right == left },
	})
	if !ok {
		t.Fatal("domain")
	}
	planes := make([]Plane[semanticFactor, semanticKey, uint8], 4)
	for index, value := range []uint8{1, 2, 4, 8} {
		builder := facts.Begin()
		root, written := builder.Set(facts.Empty(), semanticColumn, 7, all, ids[value])
		if !written {
			t.Fatalf("write %d", value)
		}
		root, written = builder.Seal(root)
		if !written {
			t.Fatalf("seal root %d", value)
		}
		planes[index], written = domain.Plane(root)
		if !written {
			t.Fatalf("plane %d", value)
		}
	}

	boolean := support.New(manager)
	if boolean == nil {
		t.Fatal("boolean work")
	}
	fingerprints = 0
	many, ok := domain.JoinContributionsMany(planes[0], planes, diagram.NewSoleScratch[semanticKey, uint8](), boolean, func(key semanticKey, output []support.Mask) bool {
		if key != 7 || len(output) != len(planes) {
			return false
		}
		for index := range output {
			output[index] = all
		}
		return true
	})
	if !ok {
		t.Fatal("many fold")
	}
	if fingerprints != 1 {
		t.Fatalf("candidate terminal fingerprints = %d, want exactly one final admission", fingerprints)
	}

	sequential := planes[0]
	for index := 1; index < len(planes); index++ {
		var joined bool
		sequential, joined = domain.JoinContributions(sequential, planes[index], diagram.NewSoleScratch[semanticKey, uint8](), boolean, func(semanticKey, support.Mask) bool { return true }, func(key semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
			return all, all, all, key == 7
		})
		if !joined {
			t.Fatalf("sequential fold %d", index)
		}
	}
	if !domain.Same(many, sequential) {
		got, gotPresent := domain.summaryAt(many, semanticColumn, 7)
		want, wantPresent := domain.summaryAt(sequential, semanticColumn, 7)
		t.Fatalf("many = %d/%t, sequential = %d/%t", got, gotPresent, want, wantPresent)
	}
}

// sealedTerminals counts the whole sealed terminal universe of one owner. It
// is the direct measure of admission: a fold that reuses an already interned
// value leaves this population unchanged, whatever it costs to look the value
// up.
func sealedTerminals[V any](t testing.TB, values *terminal.Arena[V]) int {
	t.Helper()
	count := 0
	if !values.Every(func(V) bool { count++; return true }) {
		t.Fatal("sealed terminal audit failed")
	}
	return count
}

func TestJoinContributionsManyReusesPriorNovelAggregateWithoutReadmission(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	all := regions.True()
	if !regions.Seal() {
		t.Fatal("region seal")
	}
	admissions := 0
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal: func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 {
			admissions++
			return uint64(value)
		},
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for _, value := range []uint8{0, 1, 2} {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("admit %d", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{Factors: []semanticFactor{semanticColumn}, Terminals: values, Guards: manager})
	if !ok {
		t.Fatal("diagram")
	}
	join := func(left, right uint8) (uint8, bool) { return left | right, true }
	domain, ok := New(facts, values, Operations[uint8]{
		Default: 0, Equal: func(left, right uint8) bool { return left == right }, Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join: join, Widen: join, Narrow: func(_, right uint8) (uint8, bool) { return right, true }, LessOrEq: func(left, right uint8) bool { return left&right == left },
	})
	if !ok {
		t.Fatal("domain")
	}
	plane := func(value uint8) Plane[semanticFactor, semanticKey, uint8] {
		t.Helper()
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("plane builder")
		}
		root, written := builder.Set(facts.Empty(), semanticColumn, 7, all, ids[value])
		if !written {
			t.Fatal("plane write")
		}
		root, written = builder.Seal(root)
		if !written {
			t.Fatal("plane seal")
		}
		result, valid := domain.Plane(root)
		if !valid {
			t.Fatal("plane")
		}
		return result
	}
	left, right := plane(1), plane(2)
	empty, ok := domain.Empty()
	if !ok {
		t.Fatal("empty")
	}
	covers := func(key semanticKey, output []support.Mask) bool {
		if key != 7 || len(output) != 2 {
			return false
		}
		output[0], output[1] = all, all
		return true
	}
	population := sealedTerminals(t, values)
	admissions = 0
	first, valid := domain.JoinContributionsMany(empty, []Plane[semanticFactor, semanticKey, uint8]{left, right}, diagram.NewSoleScratch[semanticKey, uint8](), support.New(manager), covers)
	if !valid || admissions != 1 {
		t.Fatalf("first novel fold valid=%t admissions=%d", valid, admissions)
	}
	if grown := sealedTerminals(t, values); grown != population+1 {
		t.Fatalf("first novel fold sealed %d terminals, want exactly one aggregate", grown-population)
	}
	population = sealedTerminals(t, values)
	second, valid := domain.JoinContributionsMany(first, []Plane[semanticFactor, semanticKey, uint8]{left, right}, diagram.NewSoleScratch[semanticKey, uint8](), support.New(manager), covers)
	if !valid || second.Root() != first.Root() {
		t.Fatalf("prior representative valid=%t roots=%t", valid, second.Root() == first.Root())
	}
	if grown := sealedTerminals(t, values); grown != population {
		t.Fatalf("repeated fold readmitted %d terminals for an already interned aggregate", grown-population)
	}
}

func TestJoinContributionsManyDistinguishesAbsentFromPresentDefault(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	root4 := fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 3})
	value, ok := domain.Plane(root4)
	if !ok {
		t.Fatal("value plane")
	}
	empty, ok := domain.Empty()
	if !ok {
		t.Fatal("empty plane")
	}
	boolean := support.New(fixture.diagram.Guards())
	if boolean == nil {
		t.Fatal("boolean work")
	}
	fold := func(defaultPresent bool) Plane[semanticFactor, semanticKey, uint8] {
		t.Helper()
		result, valid := domain.JoinContributionsMany(value, []Plane[semanticFactor, semanticKey, uint8]{value, empty}, diagram.NewSoleScratch[semanticKey, uint8](), boolean, func(key semanticKey, output []support.Mask) bool {
			if key != 7 || len(output) != 2 {
				return false
			}
			output[0] = fixture.atom
			if defaultPresent {
				output[1] = fixture.atom
			} else {
				output[1], _ = support.FromGuard(fixture.diagram.Guards(), fixture.diagram.Guards().False())
			}
			return output[1].Valid()
		})
		if !valid {
			t.Fatal("many fold")
		}
		return result
	}
	withoutDefault := fold(false)
	withDefault := fold(true)
	if got, present := fixture.at(t, withoutDefault, true); !present || got != 3 {
		t.Fatalf("Absent polluted Present(3): got %d/%t", got, present)
	}
	if got, present := fixture.at(t, withDefault, true); present || got != 0 {
		t.Fatalf("Present(Default) result was not sparsified: got %d/%t", got, present)
	}
}

func TestJoinContributionsManyReusesSemanticReferenceAcrossSiblingTerminals(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	setup := support.New(manager)
	if setup == nil {
		t.Fatal("support setup")
	}
	all := setup.True()
	on, ok := setup.Literal(1, true)
	if !ok {
		t.Fatal("on support")
	}
	off, ok := setup.Not(on)
	if !ok || !setup.Seal() {
		t.Fatal("off support")
	}
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}

	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	if _, ok := values.Admit(10); !ok || !values.Seal() {
		t.Fatal("default terminal")
	}
	// Candidate isolation keeps simultaneously open Works from observing each
	// other, so these equal values intentionally acquire three different raw
	// sibling identities that publication then canonicalizes onto one.
	admitSiblings := func(count int) []terminal.ID[uint8] {
		t.Helper()
		works := make([]*terminal.Work[uint8], count)
		ids := make([]terminal.ID[uint8], count)
		for index := range works {
			works[index] = values.Begin()
			if works[index] == nil {
				t.Fatal("terminal work")
			}
		}
		for index, work := range works {
			var admitted bool
			ids[index], admitted = work.Admit(3)
			if !admitted {
				t.Fatal("sibling terminal")
			}
		}
		for _, work := range works {
			if _, sealed := work.Seal(); !sealed {
				t.Fatal("sibling terminal seal")
			}
		}
		return ids
	}
	siblings := admitSiblings(3)
	referenceID, lowID, highID := siblings[0], siblings[1], siblings[2]
	if referenceID == lowID || referenceID == highID || lowID == highID {
		t.Fatal("isolated candidate pages shared one terminal identity")
	}
	if values.Canonical(referenceID) != values.Canonical(lowID) || values.Canonical(referenceID) != values.Canonical(highID) {
		t.Fatal("published equal sibling terminals kept separate canonical identities")
	}

	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{
		Factors: []semanticFactor{semanticColumn}, Terminals: values, Guards: manager,
	})
	if !ok {
		t.Fatal("diagram")
	}
	join := func(left, right uint8) (uint8, bool) {
		if left >= right {
			return left, true
		}
		return right, true
	}
	domain, ok := New(facts, values, Operations[uint8]{
		Default:     10,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        join,
		Widen:       join,
		Narrow:      func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq:    func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	plane := func(when support.Mask, id terminal.ID[uint8]) Plane[semanticFactor, semanticKey, uint8] {
		t.Helper()
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("root builder")
		}
		root, written := builder.Set(facts.Empty(), semanticColumn, 7, when, id)
		if !written {
			t.Fatal("root write")
		}
		root, written = builder.Seal(root)
		if !written {
			t.Fatal("root seal")
		}
		result, valid := domain.Plane(root)
		if !valid {
			t.Fatal("plane")
		}
		return result
	}
	reference := plane(all, referenceID)
	low := plane(on, lowID)
	high := plane(off, highID)
	noValue, ok := domain.Empty()
	if !ok {
		t.Fatal("empty plane")
	}

	t.Run("exclusive_carries_preserve_semantic_value", func(t *testing.T) {
		regions := support.New(manager)
		if regions == nil {
			t.Fatal("regions")
		}
		population := sealedTerminals(t, values)
		folded, valid := domain.JoinContributionsMany(reference, []Plane[semanticFactor, semanticKey, uint8]{low, high, noValue}, diagram.NewSoleScratch[semanticKey, uint8](), regions, func(key semanticKey, output []support.Mask) bool {
			if key != 7 || len(output) != 3 {
				return false
			}
			output[0], output[1], output[2] = on, off, empty
			return true
		})
		if !valid {
			t.Fatal("many fold")
		}
		if grown := sealedTerminals(t, values); grown != population {
			t.Fatalf("exclusive carries admitted %d candidate terminals", grown-population)
		}

		binaryRegions := support.New(manager)
		if binaryRegions == nil {
			t.Fatal("binary regions")
		}
		binary, valid := domain.JoinContributions(low, high, diagram.NewSoleScratch[semanticKey, uint8](), binaryRegions, func(semanticKey, support.Mask) bool { return true }, func(key semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
			return on, off, on, key == 7
		})
		at := func(want bool) func(guard.Atom) bool {
			return func(atom guard.Atom) bool { return atom == 1 && want }
		}
		if !valid || !domain.EqualAt(folded, at(false), binary, at(false)) || !domain.EqualAt(folded, at(true), binary, at(true)) {
			t.Fatal("direct fold diverged from fixed binary left fold")
		}
	})

	t.Run("overlap_reuses_operand_before_admission", func(t *testing.T) {
		regions := support.New(manager)
		if regions == nil {
			t.Fatal("regions")
		}
		population := sealedTerminals(t, values)
		left, right := plane(all, lowID), plane(all, highID)
		folded, valid := domain.JoinContributionsMany(reference, []Plane[semanticFactor, semanticKey, uint8]{left, right}, diagram.NewSoleScratch[semanticKey, uint8](), regions, func(key semanticKey, output []support.Mask) bool {
			if key != 7 || len(output) != 2 {
				return false
			}
			output[0], output[1] = all, all
			return true
		})
		if !valid {
			t.Fatal("many fold")
		}
		if !domain.EqualAt(folded, func(guard.Atom) bool { return true }, left, func(guard.Atom) bool { return true }) {
			t.Fatal("equal overlap changed the operand semantic value")
		}
		if grown := sealedTerminals(t, values); grown != population {
			t.Fatalf("reference-equivalent aggregate admitted %d candidate terminals", grown-population)
		}
	})
}

func TestJoinContributionsManyCanonicalBucketsCheckFingerprintCollisions(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	setup := support.New(manager)
	if setup == nil {
		t.Fatal("support setup")
	}
	on, ok := setup.Literal(1, true)
	if !ok {
		t.Fatal("on support")
	}
	off, ok := setup.Not(on)
	if !ok || !setup.Seal() {
		t.Fatal("off support")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(uint8) uint64 { return 0 },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for _, value := range []uint8{10, 3, 5} {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("admit %d", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{
		Factors: []semanticFactor{semanticColumn}, Terminals: values, Guards: manager,
	})
	if !ok {
		t.Fatal("diagram")
	}
	join := func(left, right uint8) (uint8, bool) {
		if left >= right {
			return left, true
		}
		return right, true
	}
	domain, ok := New(facts, values, Operations[uint8]{
		Default:     10,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(uint8) uint64 { return 0 },
		Join:        join,
		Widen:       join,
		Narrow:      func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq:    func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	plane := func(when support.Mask, value uint8) Plane[semanticFactor, semanticKey, uint8] {
		t.Helper()
		builder := facts.Begin()
		if builder == nil {
			t.Fatal("builder")
		}
		root, written := builder.Set(facts.Empty(), semanticColumn, 7, when, ids[value])
		if !written {
			t.Fatal("write")
		}
		root, written = builder.Seal(root)
		if !written {
			t.Fatal("seal")
		}
		result, valid := domain.Plane(root)
		if !valid {
			t.Fatal("plane")
		}
		return result
	}
	empty, ok := domain.Empty()
	if !ok {
		t.Fatal("empty")
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	folded, valid := domain.JoinContributionsMany(empty, []Plane[semanticFactor, semanticKey, uint8]{plane(on, 3), plane(off, 5)}, diagram.NewSoleScratch[semanticKey, uint8](), regions, func(key semanticKey, output []support.Mask) bool {
		if key != 7 || len(output) != 2 {
			return false
		}
		output[0], output[1] = on, off
		return true
	})
	if !valid {
		t.Fatal("many fold")
	}
	valueAt := func(want bool) uint8 {
		t.Helper()
		id, present, readable := facts.At(folded.Root(), semanticColumn, 7, func(atom guard.Atom) bool { return atom == 1 && want })
		if !readable || !present {
			t.Fatalf("result terminal %t unreadable/present: %t/%t", want, readable, present)
		}
		value, readable := values.Value(id)
		if !readable {
			t.Fatalf("result value %t unreadable", want)
		}
		return value
	}
	if got := valueAt(true); got != 3 {
		t.Fatalf("collision bucket reused wrong terminal on branch: got %d, want 3", got)
	}
	if got := valueAt(false); got != 5 {
		t.Fatalf("collision bucket reused wrong terminal off branch: got %d, want 5", got)
	}
}
