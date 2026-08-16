package static

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/identity"
)

// coverageFixtureAtom declares one member of the finite observation universe
// and the atoms it is directly below. The harness closes the declaration
// reflexively and transitively, so a fixture states a preorder rather than an
// already-materialized relation.
type coverageFixtureAtom struct {
	name   string
	opaque bool
	upper  []string
}

// coverageFixtureClass declares one sealed Class row by the atoms of its
// principal basis.
type coverageFixtureClass struct {
	name  string
	atoms []string
}

// coverageFixture is the descriptor universe of the coverage laws. It is a
// small lattice with a top, a nil atom, two incomparable structural families,
// a shared literal below one of them, and one opaque residual.
func coverageFixture() ([]coverageFixtureAtom, []coverageFixtureClass) {
	atoms := []coverageFixtureAtom{
		{name: "unknown"},
		{name: "nil", upper: []string{"unknown"}},
		{name: "number", upper: []string{"unknown"}},
		{name: "integer", upper: []string{"number"}},
		{name: "literal-one", upper: []string{"integer"}},
		{name: "string", upper: []string{"unknown"}},
		{name: "literal-name", upper: []string{"string"}},
		{name: "residual", opaque: true, upper: []string{"unknown"}},
	}
	classes := []coverageFixtureClass{
		{name: "nil", atoms: []string{"nil"}},
		{name: "number", atoms: []string{"number"}},
		{name: "integer", atoms: []string{"integer"}},
		{name: "literal-one", atoms: []string{"literal-one"}},
		{name: "string", atoms: []string{"string"}},
		{name: "literal-name", atoms: []string{"literal-name"}},
		{name: "optional-string", atoms: []string{"nil", "string"}},
		{name: "residual", atoms: []string{"residual"}},
		{name: "unknown", atoms: []string{"unknown"}},
	}
	return atoms, classes
}

// TestClassCoverageIdentityIsDeclarationOrderInvariant is law (a). The dense
// universe is ordered by portable atom identity, so permuting the order in
// which atoms and rows are declared must leave every class with the same
// coverage identity, rank, and nil admission.
func TestClassCoverageIdentityIsDeclarationOrderInvariant(t *testing.T) {
	atoms, classes := coverageFixture()
	reference := newCoverageFixtureSet(t, atoms, classes)
	permutations := [][]int{
		{7, 6, 5, 4, 3, 2, 1, 0},
		{3, 0, 6, 1, 7, 4, 2, 5},
		{5, 2, 7, 0, 4, 6, 3, 1},
	}
	for permutation, order := range permutations {
		permutedAtoms := make([]coverageFixtureAtom, len(atoms))
		for target, source := range order {
			permutedAtoms[target] = atoms[source]
		}
		permutedClasses := make([]coverageFixtureClass, len(classes))
		for index := range classes {
			permutedClasses[len(classes)-1-index] = classes[index]
		}
		candidate := newCoverageFixtureSet(t, permutedAtoms, permutedClasses)
		if candidate.universeID != reference.universeID {
			t.Fatalf("permutation %d installed a different universe identity", permutation)
		}
		for _, class := range classes {
			expected := reference.class(t, class.name)
			actual := candidate.class(t, class.name)
			expectedID, expectedOK := reference.set.Identity(expected)
			actualID, actualOK := candidate.set.Identity(actual)
			if !expectedOK || !actualOK || expectedID != actualID {
				t.Fatalf("permutation %d changed the identity of class %s", permutation, class.name)
			}
			if reference.set.Rank(expected) != candidate.set.Rank(actual) {
				t.Fatalf("permutation %d changed the rank of class %s", permutation, class.name)
			}
			if reference.set.CanBeNil(expected) != candidate.set.CanBeNil(actual) {
				t.Fatalf("permutation %d changed nil admission of class %s", permutation, class.name)
			}
		}
	}
}

// TestClassCoverageJoinIsParenthesizationInvariant is law (b). Coverage join
// is a bitwise union, so it must be associative, commutative, and idempotent
// through the whole ClassSet plumbing: every parenthesization and every order
// of a multi-way join must produce one identity.
func TestClassCoverageJoinIsParenthesizationInvariant(t *testing.T) {
	fixture := newDefaultCoverageFixtureSet(t)
	names := []string{"literal-one", "literal-name", "nil", "residual"}
	members := make([]Class, len(names))
	for index, name := range names {
		members[index] = fixture.class(t, name)
	}
	set := fixture.set

	expected := members[0]
	for _, member := range members[1:] {
		expected = set.Join(expected, member)
	}
	expectedID, ok := set.Identity(expected)
	if !ok {
		t.Fatal("multi-way join has no identity")
	}

	for _, order := range [][]int{
		{0, 1, 2, 3}, {3, 2, 1, 0}, {2, 0, 3, 1}, {1, 3, 0, 2},
	} {
		// Left-nested, right-nested, and balanced parenthesizations of the
		// same permutation must all agree.
		left := members[order[0]]
		for _, index := range order[1:] {
			left = set.Join(left, members[index])
		}
		right := members[order[len(order)-1]]
		for index := len(order) - 2; index >= 0; index-- {
			right = set.Join(members[order[index]], right)
		}
		balanced := set.Join(
			set.Join(members[order[0]], members[order[1]]),
			set.Join(members[order[2]], members[order[3]]),
		)
		for name, candidate := range map[string]Class{"left": left, "right": right, "balanced": balanced} {
			id, idOK := set.Identity(candidate)
			if !idOK || id != expectedID {
				t.Fatalf("%s parenthesization of %v produced a different join identity", name, order)
			}
			if !set.Equal(candidate, expected) {
				t.Fatalf("%s parenthesization of %v is not join-equal", name, order)
			}
		}
	}

	for _, member := range members {
		if joined := set.Join(member, member); !set.Equal(joined, member) {
			t.Fatal("join is not idempotent")
		}
	}
	for _, left := range members {
		for _, right := range members {
			forward, reverse := set.Join(left, right), set.Join(right, left)
			if !set.Equal(forward, reverse) {
				t.Fatal("join is not commutative")
			}
			if !set.LessOrEq(left, forward) || !set.LessOrEq(right, forward) {
				t.Fatal("join is not an upper bound")
			}
			// Least upper bound: a join must never resolve onto a merely
			// containing row, which is what the coverage index probe would do
			// if it trusted its hash instead of confirming the candidate.
			for _, bound := range members {
				if set.LessOrEq(left, bound) && set.LessOrEq(right, bound) && !set.LessOrEq(forward, bound) {
					t.Fatal("join is not the least upper bound")
				}
			}
		}
	}
}

// TestClassCoverageIndexProbeConfirmsItsCandidate states the collision guard
// of the coverage index: a hash bucket is a candidate list, so a candidate is
// admitted only when it states the coverage word for word. A merely
// containing or contained candidate must be rejected.
func TestClassCoverageIndexProbeConfirmsItsCandidate(t *testing.T) {
	fixture := newDefaultCoverageFixtureSet(t)
	set := fixture.set
	nilCoverage, _ := set.classCoverage(fixture.class(t, "nil"))
	stringCoverage, _ := set.classCoverage(fixture.class(t, "string"))
	optionalCoverage, _ := set.classCoverage(fixture.class(t, "optional-string"))
	totalCoverage, _ := set.classCoverage(set.AnyValue())

	if !coverageEqualsJoin(optionalCoverage, nilCoverage, stringCoverage) {
		t.Fatal("the exact union was not accepted as the joined coverage")
	}
	if coverageEqualsJoin(totalCoverage, nilCoverage, stringCoverage) {
		t.Fatal("a strictly containing candidate was accepted as the joined coverage")
	}
	if coverageEqualsJoin(stringCoverage, nilCoverage, stringCoverage) {
		t.Fatal("a strictly contained candidate was accepted as the joined coverage")
	}
	if coverageEqual(optionalCoverage, totalCoverage) || coverageEqual(optionalCoverage, stringCoverage) {
		t.Fatal("distinct coverages compared equal")
	}
	if !coverageSubset(stringCoverage, optionalCoverage) || coverageSubset(optionalCoverage, stringCoverage) {
		t.Fatal("coverage containment is not antisymmetric on this pair")
	}
}

// TestClassCoverageRankDescendsOnStrictGrowth is law (c). Rank is
// 1+|P|-|C|, so strict coverage growth must strictly decrease it. This is the
// well-foundedness contract Measure.descends relies on: no recurrent widening
// chain can be infinite.
func TestClassCoverageRankDescendsOnStrictGrowth(t *testing.T) {
	fixture := newDefaultCoverageFixtureSet(t)
	set := fixture.set
	names := []string{"nil", "number", "integer", "literal-one", "string", "literal-name", "optional-string", "residual", "unknown"}
	classes := make([]Class, len(names))
	for index, name := range names {
		classes[index] = fixture.class(t, name)
	}
	strictPairs := 0
	for leftIndex, left := range classes {
		for rightIndex, right := range classes {
			if !set.LessOrEq(left, right) || set.Equal(left, right) {
				continue
			}
			strictPairs++
			if set.Rank(right) >= set.Rank(left) {
				t.Fatalf("strict growth %s -> %s did not descend: %d -> %d",
					names[leftIndex], names[rightIndex], set.Rank(left), set.Rank(right))
			}
		}
	}
	if strictPairs == 0 {
		t.Fatal("fixture states no strict coverage growth")
	}
	// A join that strictly grows both operands must descend below both.
	for _, left := range classes {
		for _, right := range classes {
			joined := set.Join(left, right)
			if !set.Equal(joined, left) && set.Rank(joined) >= set.Rank(left) {
				t.Fatal("join did not descend below its left operand")
			}
			if !set.Equal(joined, right) && set.Rank(joined) >= set.Rank(right) {
				t.Fatal("join did not descend below its right operand")
			}
			if set.Rank(joined) == 0 {
				t.Fatal("join produced an unranked class")
			}
		}
	}
}

// TestClassCoverageHotOperationsAllocateNothing is law (e). Equality, order,
// rank, and the recurrent join forms are bitwise queries over sealed state:
// they must not allocate. Only a coverage no sealed row states constructs a
// value, and that construction is deliberately outside this law.
func TestClassCoverageHotOperationsAllocateNothing(t *testing.T) {
	fixture := newDefaultCoverageFixtureSet(t)
	set := fixture.set
	integer := fixture.class(t, "integer")
	number := fixture.class(t, "number")
	literalName := fixture.class(t, "literal-name")
	nilClass := fixture.class(t, "nil")
	optionalString := fixture.class(t, "optional-string")

	// A join whose result is a sealed row must resolve through the coverage
	// index rather than materialize a descriptor.
	if joined := set.Join(nilClass, fixture.class(t, "string")); !set.Equal(joined, optionalString) {
		t.Fatal("join did not resolve onto the sealed optional-string row")
	}

	for name, operation := range map[string]func(){
		"equal-same":        func() { _ = set.Equal(integer, integer) },
		"equal-distinct":    func() { _ = set.Equal(integer, literalName) },
		"less-or-eq-true":   func() { _ = set.LessOrEq(integer, number) },
		"less-or-eq-false":  func() { _ = set.LessOrEq(integer, literalName) },
		"compare":           func() { _ = set.Compare(integer, literalName) },
		"rank":              func() { _ = set.Rank(integer) },
		"can-be-nil":        func() { _ = set.CanBeNil(optionalString) },
		"join-contained":    func() { _ = set.Join(integer, number) },
		"join-sealed-row":   func() { _ = set.Join(nilClass, fixture.class(t, "string")) },
		"may-runtime-kinds": func() { _, _ = set.MayRuntimeKinds(integer) },
	} {
		if allocations := testing.AllocsPerRun(500, operation); allocations != 0 {
			t.Fatalf("%s allocated %.2f objects/run", name, allocations)
		}
	}
}

// TestClassCoverageDerivedClassIsCanonical proves a coverage that no sealed
// row states becomes one canonical derived class: its identity, rank, and nil
// admission come from final coverage, and rebuilding it by a different join
// route reproduces the same identity.
func TestClassCoverageDerivedClassIsCanonical(t *testing.T) {
	fixture := newDefaultCoverageFixtureSet(t)
	set := fixture.set
	literalOne := fixture.class(t, "literal-one")
	literalName := fixture.class(t, "literal-name")
	residual := fixture.class(t, "residual")

	derived := set.Join(literalOne, literalName)
	if kind, ok := set.Kind(derived); !ok || kind != ClassDerived {
		t.Fatalf("join of two incomparable literals = %v/%v, want a derived class", kind, ok)
	}
	derivedID, ok := set.Identity(derived)
	if !ok {
		t.Fatal("derived class has no identity")
	}
	if again, _ := set.Identity(set.Join(literalName, literalOne)); again != derivedID {
		t.Fatal("derived class identity depends on join order")
	}
	if set.CanBeNil(derived) {
		t.Fatal("a literal union admitted nil")
	}
	if !set.CanBeNil(set.Join(derived, residual)) {
		t.Fatal("an opaque residual did not admit nil")
	}
	if !set.LessOrEq(derived, fixture.class(t, "unknown")) {
		t.Fatal("derived class is not below the total coverage")
	}
	if set.LessOrEq(fixture.class(t, "integer"), derived) {
		t.Fatal("derived literal union absorbed a strictly wider class")
	}
}

func BenchmarkClassSetJoin(b *testing.B) {
	fixture, err := buildCoverageFixtureSet(coverageFixture())
	if err != nil {
		b.Fatalf("coverage fixture: %v", err)
	}
	set := fixture.set
	contained := [2]Class{fixture.mustClass("literal-one"), fixture.mustClass("number")}
	sealed := [2]Class{fixture.mustClass("nil"), fixture.mustClass("string")}
	b.Run("contained", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			_ = set.Join(contained[0], contained[1])
		}
	})
	b.Run("sealed-row", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			_ = set.Join(sealed[0], sealed[1])
		}
	})
	b.Run("less-or-eq", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			_ = set.LessOrEq(contained[0], contained[1])
		}
	})
}

type coverageFixtureSet struct {
	set        *ClassSet
	universeID identity.ContentID
	byName     map[string]Class
}

func (fixture coverageFixtureSet) class(t *testing.T, name string) Class {
	t.Helper()
	class, ok := fixture.byName[name]
	if !ok {
		t.Fatalf("fixture class %s is absent", name)
	}
	return class
}

func (fixture coverageFixtureSet) mustClass(name string) Class {
	class, ok := fixture.byName[name]
	if !ok {
		panic("static: fixture class " + name + " is absent")
	}
	return class
}

// newCoverageFixtureSet drives the production universe installation,
// principal materialization, and descriptor finalization over a declared atom
// preorder. Only that preorder is supplied by the fixture; every coverage,
// identity, rank, and index below it is built by the sealed path.
func newDefaultCoverageFixtureSet(t *testing.T) coverageFixtureSet {
	t.Helper()
	atoms, classes := coverageFixture()
	return newCoverageFixtureSet(t, atoms, classes)
}

func newCoverageFixtureSet(t *testing.T, atoms []coverageFixtureAtom, classes []coverageFixtureClass) coverageFixtureSet {
	t.Helper()
	fixture, err := buildCoverageFixtureSet(atoms, classes)
	if err != nil {
		t.Fatalf("coverage fixture: %v", err)
	}
	return fixture
}

func buildCoverageFixtureSet(atoms []coverageFixtureAtom, classes []coverageFixtureClass) (coverageFixtureSet, error) {
	set := &ClassSet{rows: []classRow{{kind: ClassAnyValue}}}

	// Runtime atoms take one-based dense handles in declaration order and
	// opaque atoms take the reserved high-bit encoding. The universe order is
	// derived from portable atom identity, never from these handles.
	atomOf := make(map[string]uint64, len(atoms))
	nameOf := make(map[uint64]string, len(atoms))
	universeRows := make([]descriptorUniverseRow, 0, len(atoms))
	var runtimeOrdinal, opaqueOrdinal uint32
	for _, atom := range atoms {
		var handle uint64
		if atom.opaque {
			opaqueOrdinal++
			handle = opaqueClassAtom(opaqueOrdinal)
		} else {
			runtimeOrdinal++
			handle = runtimeClassAtom(runtimeOrdinal)
		}
		atomOf[atom.name] = handle
		nameOf[handle] = atom.name
		key := "\x00" + atom.name
		universeRows = append(universeRows, descriptorUniverseRow{atom: handle, key: key, id: descriptorAtomID(key)})
	}
	set.unknownAtom = atomOf["unknown"]
	if err := set.installDescriptorUniverse(universeRows); err != nil {
		return coverageFixtureSet{}, err
	}

	above := coverageFixtureClosure(atoms)
	relation := func(left, right uint64) (bool, bool) {
		leftName, leftOK := nameOf[left]
		rightName, rightOK := nameOf[right]
		if !leftOK || !rightOK {
			return false, false
		}
		return above[leftName][rightName], true
	}
	if err := set.sealDescriptorPrincipals(relation); err != nil {
		return coverageFixtureSet{}, err
	}

	total := make([]uint64, set.coverageStride)
	for position := range set.universe {
		coverageSet(total, position)
	}
	set.descriptors = []classDescriptor{{owner: set, coverage: total}}
	declared := make(map[string][]uint64, len(classes))
	for _, class := range classes {
		coverage := make([]uint64, set.coverageStride)
		for _, name := range class.atoms {
			handle, ok := atomOf[name]
			if !ok || !set.addPrincipal(coverage, handle) {
				return coverageFixtureSet{}, errors.New("static: malformed coverage fixture atom")
			}
		}
		if class.name == "nil" {
			set.nil = Class{owner: set, index: uint32(len(set.rows))}
		}
		set.rows = append(set.rows, classRow{kind: ClassConcrete})
		set.descriptors = append(set.descriptors, classDescriptor{owner: set, coverage: coverage})
		declared[class.name] = coverage
	}
	if set.nil.owner == nil {
		return coverageFixtureSet{}, errors.New("static: coverage fixture lacks a nil class")
	}
	set.ranks = make([]uint64, len(set.rows))
	set.nilable = make([]bool, len(set.rows))
	if err := set.finalizeDescriptors(); err != nil {
		return coverageFixtureSet{}, err
	}
	set.runtimeKinds = make([]runtimekind.Set, len(set.rows))
	set.runtimeKinds[0] = runtimekind.All
	set.id = coverageFixtureSetID(set)
	if err := set.sealClassIdentities(); err != nil {
		return coverageFixtureSet{}, err
	}

	// Coverage-equal rows merge, so fixture handles are resolved through the
	// sealed coverage index rather than kept at their declared ordinals.
	byName := make(map[string]Class, len(declared))
	for name, coverage := range declared {
		class, found := set.classForCoverage(coverage)
		if !found {
			return coverageFixtureSet{}, errors.New("static: coverage fixture class " + name + " lost its sealed row")
		}
		byName[name] = class
	}
	return coverageFixtureSet{set: set, universeID: set.universeID, byName: byName}, nil
}

// coverageFixtureClosure states the reflexive transitive closure of the
// declared atom preorder. A fixture declares only direct edges, so no law can
// rely on a hand-written relation table.
func coverageFixtureClosure(atoms []coverageFixtureAtom) map[string]map[string]bool {
	above := make(map[string]map[string]bool, len(atoms))
	for _, atom := range atoms {
		above[atom.name] = map[string]bool{atom.name: true}
	}
	for _, atom := range atoms {
		for _, upper := range atom.upper {
			above[atom.name][upper] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, atom := range atoms {
			for middle := range above[atom.name] {
				for upper := range above[middle] {
					if !above[atom.name][upper] {
						above[atom.name][upper] = true
						changed = true
					}
				}
			}
		}
	}
	return above
}

// coverageFixtureSetID stands in for the sealed Link identity a production
// ClassSet mints. Class identities below it are still derived by the
// production path from coverage identity alone.
func coverageFixtureSetID(set *ClassSet) (id identity.ContentID) {
	copy(id[:], set.universeID[:])
	id[0] |= 1
	return id
}
