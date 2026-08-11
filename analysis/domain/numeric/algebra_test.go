package numeric

import (
	"math"
	"strconv"
	"strings"
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type numericFixture struct {
	source   *link.Link
	program  *program.Program
	contract *target.Contract
	algebra  *Algebra
}

func newNumericFixture(t testing.TB, name, text string) numericFixture {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := New(source)
	if !ok {
		t.Fatal("numeric Algebra rejected sealed Link")
	}
	return numericFixture{source: source, program: p, contract: contract, algebra: algebra}
}

func loopFixture(t testing.TB) numericFixture {
	t.Helper()
	return newNumericFixture(t, "numeric_loop", `
local seed = 0
local limit = 2
local step = 1
local compare = seed < limit
for index = seed, limit, step do
  local copy = index
end
`)
}

func loopOperands(t testing.TB, fixture numericFixture) (Key, [3]Atom) {
	t.Helper()
	for index := 0; index < fixture.algebra.KeyCount(); index++ {
		key, _ := fixture.algebra.KeyAt(index)
		var candidates []Atom
		for atomIndex := 0; atomIndex < fixture.algebra.AtomCount(key); atomIndex++ {
			atom, ok := fixture.algebra.AtomAt(key, atomIndex)
			if ok {
				if _, ordinary := atom.Scalar(); ordinary {
					candidates = append(candidates, atom)
				}
			}
		}
		for first := 0; first < len(candidates); first++ {
			for second := first + 1; second < len(candidates); second++ {
				for third := second + 1; third < len(candidates); third++ {
					atoms := [3]Atom{candidates[first], candidates[second], candidates[third]}
					if _, ok := fixture.algebra.Pair(atoms[0], atoms[1]); !ok {
						continue
					}
					if _, ok := fixture.algebra.Pair(atoms[1], atoms[2]); !ok {
						continue
					}
					if _, ok := fixture.algebra.Pair(atoms[0], atoms[2]); !ok {
						continue
					}
					if _, ok := fixture.algebra.Pair(atoms[1], atoms[0]); !ok {
						continue
					}
					if _, ok := fixture.algebra.Pair(atoms[2], atoms[1]); !ok {
						continue
					}
					if _, ok := fixture.algebra.Pair(atoms[2], atoms[0]); !ok {
						continue
					}
					return key, atoms
				}
			}
		}
	}
	t.Fatal("no exact body root contains three pair-connected loop operands")
	return Key{}, [3]Atom{}
}

func TestNumericAtomForScalarBoundaryLaws(t *testing.T) {
	fixture := loopFixture(t)
	rebuilt, ok := New(fixture.source)
	if !ok || !fixture.algebra.Equivalent(rebuilt) || fixture.algebra.ContentID() != rebuilt.ContentID() ||
		fixture.algebra.fingerprint != rebuilt.fingerprint || fixture.algebra.Default().Hash() != rebuilt.Default().Hash() {
		t.Fatal("same sealed Link produced a non-deterministic Numeric fingerprint")
	}

	var scalar Scalar
	for _, candidate := range fixture.algebra.atomScalars {
		if candidate == (Scalar{}) {
			continue
		}
		atom, found := fixture.algebra.AtomFor(candidate)
		back, projected := atom.Scalar()
		if !found || !atom.Valid() || !projected || back != candidate {
			t.Fatal("scalar-to-atom round trip changed its exact Program occurrence")
		}
		scalar = candidate
	}
	if scalar == (Scalar{}) {
		t.Fatal("fixture has no scalar support")
	}
	if atom, found := fixture.algebra.AtomFor(Scalar{}); found || atom.Valid() {
		t.Fatal("unsupported scalar acquired a Numeric atom")
	}
	foreign := loopFixture(t)
	var foreignScalar Scalar
	for _, candidate := range foreign.algebra.atomScalars {
		if candidate != (Scalar{}) {
			foreignScalar = candidate
			break
		}
	}
	if foreignScalar == (Scalar{}) {
		t.Fatal("foreign fixture has no scalar support")
	}
	if atom, found := fixture.algebra.AtomFor(foreignScalar); found || atom.Valid() {
		t.Fatal("foreign Link scalar crossed the Numeric owner fence")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, found := fixture.algebra.AtomFor(scalar); !found {
			t.Fatal("sealed scalar lookup failed")
		}
	}); allocations != 0 {
		t.Fatalf("AtomFor allocated %.1f times", allocations)
	}
}

func TestNumericProjectShardFenceAndPortableRootReplay(t *testing.T) {
	fixture := loopFixture(t)
	key, ok := fixture.algebra.KeyAt(0)
	if !ok {
		t.Fatal("first Numeric key")
	}
	root, ok := key.Root()
	if !ok {
		t.Fatal("first Numeric root")
	}
	ref, ok := key.Ref()
	if !ok || ref.LinkID() != fixture.source.ContentID() || ref.Shard() == 0 || ref.Shard() != ref.ShardOrdinal() {
		t.Fatal("root replay identity did not retain the canonical Project ordinal")
	}
	foreign := loopFixture(t)
	foreignShard, ok := foreign.source.Project().Mounts().At(0)
	if !ok {
		t.Fatal("foreign Project shard")
	}
	if _, ok := fixture.algebra.RootFor(foreignShard, root.Body()); ok {
		t.Fatal("foreign Project shard crossed Numeric Root owner fence")
	}
	if rebound, ok := foreign.algebra.FindKey(ref); !ok {
		t.Fatal("portable Numeric root did not rebind through exact receiving Project")
	} else if reboundRoot, rootOK := rebound.Root(); !rootOK || reboundRoot.Body() != root.Body() {
		t.Fatal("portable Numeric root rebound to the wrong body")
	}
	for atomIndex := 0; atomIndex < fixture.algebra.AtomCount(key); atomIndex++ {
		atom, atomOK := fixture.algebra.AtomAt(key, atomIndex)
		scalar, scalarOK := atom.Scalar()
		if !atomOK || !scalarOK {
			continue
		}
		if _, scalarOK := fixture.algebra.ScalarFor(foreignShard, scalar.Body(), scalar.Term()); scalarOK {
			t.Fatal("foreign Project shard crossed Numeric Scalar owner fence")
		}
		break
	}
}

func TestNumericScalarLiteralUsesExactTypedOrdinal(t *testing.T) {
	fixture := newNumericFixture(t, "numeric_literal_ordinal", `
local first = 1
local second = 2.5
local third = 3
return first, second, third
`)
	var selected Scalar
	selectedOrdinal := uint32(0)
	for _, scalar := range fixture.algebra.atomScalars {
		if scalar == (Scalar{}) {
			continue
		}
		family := keyspace.TermFamily(scalar.term)
		if family != keyspace.FamilyInteger && family != keyspace.FamilyFloat {
			continue
		}
		if ordinal := keyspace.TermOrdinal(scalar.term); ordinal > selectedOrdinal {
			selected = scalar
			selectedOrdinal = ordinal
		}
	}
	if selected == (Scalar{}) {
		t.Fatal("fixture has no typed literal occurrence")
	}
	if _, _, _, ok := scalarNumberLiteral(fixture.source, selected); !ok {
		t.Fatal("highest typed literal ordinal was not resolved")
	}
	wrongBody := selected
	wrongBody.body = keyspace.MakeTerm(keyspace.FamilyBody, keyspace.TermOrdinal(selected.body)+1)
	if _, _, _, ok := scalarNumberLiteral(fixture.source, wrongBody); ok {
		t.Fatal("typed literal lookup ignored its lexical owner")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, _, _, ok := scalarNumberLiteral(fixture.source, selected); !ok {
			t.Fatal("typed literal lookup became unavailable")
		}
	}); allocations != 0 {
		t.Fatalf("typed literal lookup allocated %.1f times", allocations)
	}
}

func nestedRootSupportSource(width, depth int) string {
	var source strings.Builder
	for index := 0; index < width; index++ {
		source.WriteString("local root")
		source.WriteString(strconv.Itoa(index))
		source.WriteString(" = ")
		source.WriteString(strconv.Itoa(index + 1))
		source.WriteByte('\n')
	}
	for branch := 0; branch < width; branch++ {
		for level := 0; level < depth; level++ {
			source.WriteString("if true then\n")
		}
		for level := 0; level < depth; level++ {
			source.WriteString("end\n")
		}
	}
	source.WriteString("return root0\n")
	return source.String()
}

func syntheticBodyGroups(depth, siblings, groupWidth int) ([]uint32, [][]Scalar) {
	bodyCount := depth + siblings
	parents := make([]uint32, bodyCount+1)
	groups := make([][]Scalar, bodyCount+1)
	for ordinal := 1; ordinal <= depth; ordinal++ {
		if ordinal > 1 {
			parents[ordinal] = uint32(ordinal - 1)
		}
		group := make([]Scalar, ordinal%5+1)
		for index := range group {
			group[index] = Scalar{
				body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal)),
				term: keyspace.MakeTerm(keyspace.FamilyInteger, uint32(ordinal*100+index+1)),
			}
		}
		groups[ordinal] = group
	}
	for ordinal := depth + 1; ordinal <= bodyCount; ordinal++ {
		group := make([]Scalar, groupWidth)
		for index := range group {
			group[index] = Scalar{
				body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal)),
				term: keyspace.MakeTerm(keyspace.FamilyInteger, uint32(ordinal*100+index+1)),
			}
		}
		groups[ordinal] = group
	}
	return parents, groups
}

func TestAppendAncestorGroupsDenseDepthProof(t *testing.T) {
	for _, depth := range []int{256, 512} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			parents, groups := syntheticBodyGroups(depth, 2048, 64)
			root := keyspace.MakeTerm(keyspace.FamilyBody, uint32(depth))
			seed := Scalar{term: keyspace.MakeTerm(keyspace.FamilyInteger, 1)}
			beforeSibling := append([]Scalar(nil), groups[depth+1]...)
			emitted := 0
			for ordinal := depth; ordinal > 0; ordinal-- {
				emitted += len(groups[ordinal])
			}
			result, visits, ok := appendAncestorGroups([]Scalar{seed}, root, parents, groups)
			if !ok || visits != depth || len(result) != emitted+1 {
				t.Fatalf("dense ancestor walk = visits %d/%v emitted %d, want %d/%v %d", visits, ok, len(result)-1, depth, true, emitted)
			}
			at := 1
			for ordinal := depth; ordinal > 0; ordinal-- {
				for _, scalar := range groups[ordinal] {
					if result[at] != scalar {
						t.Fatalf("emitted scalar %d = %#v, want %#v", at, result[at], scalar)
					}
					at++
				}
			}
			if got := groups[depth+1]; len(got) != len(beforeSibling) || len(got) == 0 || got[0] != beforeSibling[0] || got[len(got)-1] != beforeSibling[len(beforeSibling)-1] {
				t.Fatal("unrelated sibling group was modified")
			}

			cycleParents := append([]uint32(nil), parents...)
			cycleParents[2], cycleParents[3] = 3, 2
			if _, _, ok := appendAncestorGroups(nil, keyspace.MakeTerm(keyspace.FamilyBody, 2), cycleParents, groups); ok {
				t.Fatal("cyclic body parent chain accepted")
			}
			selfParents := append([]uint32(nil), parents...)
			selfParents[1] = 1
			if _, _, ok := appendAncestorGroups(nil, keyspace.MakeTerm(keyspace.FamilyBody, 1), selfParents, groups); ok {
				t.Fatal("self body parent accepted")
			}
			badParents := append([]uint32(nil), parents...)
			badParents[2] = uint32(len(badParents))
			if _, _, ok := appendAncestorGroups(nil, keyspace.MakeTerm(keyspace.FamilyBody, 2), badParents, groups); ok {
				t.Fatal("out-of-range body parent accepted")
			}
			if _, _, ok := appendAncestorGroups(nil, keyspace.MakeTerm(keyspace.FamilyBody, uint32(len(parents))), parents, groups); ok {
				t.Fatal("out-of-range body ordinal accepted")
			}
		})
	}
}

func TestNumericNestedRootSupportScalesWithWidthAndDepth(t *testing.T) {
	narrow := newNumericFixture(t, "numeric_nested_narrow", nestedRootSupportSource(4, 4))
	wide := newNumericFixture(t, "numeric_nested_wide", nestedRootSupportSource(8, 4))
	deep := newNumericFixture(t, "numeric_nested_deep", nestedRootSupportSource(4, 8))
	if wide.algebra.KeyCount() <= narrow.algebra.KeyCount() || deep.algebra.KeyCount() <= narrow.algebra.KeyCount() {
		t.Fatalf("nested root count did not scale: narrow=%d wide=%d deep=%d", narrow.algebra.KeyCount(), wide.algebra.KeyCount(), deep.algebra.KeyCount())
	}
	for name, caseData := range map[string]struct {
		fixture numericFixture
		width   int
	}{
		"narrow": {fixture: narrow, width: 4},
		"wide":   {fixture: wide, width: 8},
		"deep":   {fixture: deep, width: 4},
	} {
		for index := 0; index < caseData.fixture.algebra.KeyCount(); index++ {
			key, ok := caseData.fixture.algebra.KeyAt(index)
			if !ok || caseData.fixture.algebra.AtomCount(key) < caseData.width+1 {
				t.Fatalf("%s root %d support = %d, want at least %d", name, index, caseData.fixture.algebra.AtomCount(key), caseData.width+1)
			}
		}
	}
}

func TestNumericSparseDifferenceClosureAndEquality(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	first, ok := fixture.algebra.Pair(atoms[0], atoms[1])
	if !ok {
		t.Fatal("first loop pair")
	}
	second, ok := fixture.algebra.Pair(atoms[1], atoms[2])
	if !ok {
		t.Fatal("second loop pair")
	}
	goal, ok := fixture.algebra.Pair(atoms[0], atoms[2])
	if !ok {
		t.Fatal("transitive loop pair")
	}
	value, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayInteger, atoms[1]: MayInteger, atoms[2]: MayInteger},
		[]Pair{first, second}, nil,
		[]Pair{first, second},
		map[Pair]int64{first: 0, second: 0},
	)
	if !ok || !value.MustEqual(first) || !value.MustEqual(goal) {
		t.Fatal("normalized primitive equality did not close over sealed observations")
	}
	if bound, infinite, found := value.Bound(goal); !found || infinite || bound != 0 {
		t.Fatalf("transitive bound = %d/%t/%t", bound, infinite, found)
	}
	if _, accepted := fixture.algebra.AdmitAt(key, nil, nil, nil, nil, map[Pair]int64{goal: 7}); accepted {
		t.Fatal("unsealed difference threshold admitted")
	}
	reverseGoal, ok := fixture.algebra.Pair(atoms[2], atoms[0])
	if !ok {
		t.Fatal("reverse transitive loop pair")
	}
	directed, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayInteger, atoms[1]: MayInteger, atoms[2]: MayInteger},
		nil, nil, nil,
		map[Pair]int64{first: -1, second: 1},
	)
	if !ok {
		t.Fatal("directed difference relation")
	}
	if bound, infinite, found := directed.Bound(goal); !found || infinite || bound != 0 {
		t.Fatalf("directed transitive bound = %d/%t/%t", bound, infinite, found)
	}
	if _, infinite, found := directed.Bound(reverseGoal); !found || !infinite {
		t.Fatal("difference closure reversed x-y orientation")
	}
}

func TestNumericRawAndIntegralEqualityRemainDistinct(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	pair, ok := fixture.algebra.Pair(atoms[0], atoms[1])
	if !ok {
		t.Fatal("pair")
	}
	raw, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayFiniteFloat, atoms[1]: MayFiniteFloat},
		[]Pair{pair}, nil, nil, nil)
	if !ok || !raw.MustEqual(pair) || raw.MustIntegralEqual(pair) {
		t.Fatal("raw finite-float equality became integral")
	}
	integral, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayInteger, atoms[1]: MayFiniteFloat},
		nil, nil, []Pair{pair}, nil)
	if !ok || !integral.MustEqual(pair) || !integral.MustIntegralEqual(pair) {
		t.Fatal("integral int/float equality lost raw or integral relation")
	}
	if bound, infinite, present := integral.Bound(pair); !present || infinite || bound != 0 {
		t.Fatal("integral equality did not induce zero difference")
	}
}

func TestNumericRejectsNaNEqualityAndForeignCoordinates(t *testing.T) {
	first := loopFixture(t)
	second := loopFixture(t)
	key, atoms := loopOperands(t, first)
	pair, _ := first.algebra.Pair(atoms[0], atoms[1])
	if _, ok := first.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayFiniteFloat | MayNaN, atoms[1]: MayFiniteFloat},
		[]Pair{pair}, nil, nil, nil,
	); ok {
		t.Fatal("may-NaN primitive equality accepted")
	}
	foreignKey, foreignAtoms := loopOperands(t, second)
	if _, ok := first.algebra.AdmitAt(foreignKey, map[Atom]Eligibility{foreignAtoms[0]: MayInteger}, nil, nil, nil, nil); ok {
		t.Fatal("foreign owner coordinates admitted")
	}
	if got := first.algebra.Join(first.algebra.Default(), second.algebra.Default()); got.valid() {
		t.Fatal("cross-owner Join produced a valid image")
	}
}

func TestNumericSelfDisequalityRefinesExactlyToNaN(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	self, ok := fixture.algebra.Pair(atoms[0], atoms[0])
	if !ok {
		t.Fatal("self observation")
	}
	value, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayFiniteFloat | MayNaN}, nil, []Pair{self}, nil, nil)
	mask, masked := value.Eligibility(atoms[0])
	if !ok || !masked || mask != MayNaN || !value.MustUnequal(self) || len(value.unequal) != 0 {
		t.Fatal("x ~= x did not retain exactly the Lua NaN case")
	}
	if _, accepted := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayInteger}, nil, []Pair{self}, nil, nil); accepted {
		t.Fatal("integral x ~= x accepted")
	}
}

func TestNumberLiteralLawsAreIntrinsic(t *testing.T) {
	fixture := newNumericFixture(t, "numeric_literals", `
local whole = 1
local fraction = 1.5
local floatZero = 0.0
local same = 1 == 1
return whole < fraction, same, floatZero
`)
	var whole, fraction, floatZero Atom
	var ones []Atom
	for index, literal := range fixture.algebra.atomLiterals {
		switch {
		case literal.kind == literalInteger && literal.integer == 1:
			whole = fixture.algebra.atoms[index]
			ones = append(ones, whole)
		case literal.kind == literalFloat && literal.float == 1.5:
			fraction = fixture.algebra.atoms[index]
		case literal.kind == literalFloat && literal.float == 0:
			floatZero = fixture.algebra.atoms[index]
		}
	}
	if !whole.Valid() || !fraction.Valid() || !floatZero.Valid() {
		t.Fatal("fixture lost exact literal atoms")
	}
	var key Key
	for index := 0; index < fixture.algebra.KeyCount(); index++ {
		candidate, _ := fixture.algebra.KeyAt(index)
		if containsUint32(fixture.algebra.keyAtoms[candidate.slot-1], whole.slot) &&
			containsUint32(fixture.algebra.keyAtoms[candidate.slot-1], fraction.slot) {
			key = candidate
			break
		}
	}
	if !key.Valid() {
		t.Fatal("no root supports exact literals")
	}
	foundEqualLiteralPair := false
	for left := range ones {
		for right := left + 1; right < len(ones); right++ {
			same, present := fixture.algebra.Pair(ones[left], ones[right])
			if !present {
				continue
			}
			foundEqualLiteralPair = true
			if _, accepted := fixture.algebra.AdmitAt(key, nil, nil, []Pair{same}, nil, nil); accepted {
				t.Fatal("known-equal literals admitted as disequal")
			}
		}
	}
	if !foundEqualLiteralPair {
		t.Fatal("fixture did not seal the literal equality pair")
	}
	wholeMask, _ := fixture.algebra.Default().Eligibility(whole)
	fractionMask, _ := fixture.algebra.Default().Eligibility(fraction)
	if wholeMask != MayInteger || fractionMask != MayFiniteFloat {
		t.Fatalf("literal base masks = %v/%v", wholeMask, fractionMask)
	}
	selfFraction, _ := fixture.algebra.Pair(fraction, fraction)
	if _, ok := fixture.algebra.AdmitAt(key, nil, nil, nil, []Pair{selfFraction}, nil); ok {
		t.Fatal("non-integral float admitted as definitely integral")
	}
	zero, _ := fixture.algebra.Zero()
	selfZero, _ := fixture.algebra.Pair(zero, zero)
	if _, ok := fixture.algebra.AdmitAt(key, nil, nil, []Pair{selfZero}, nil, nil); ok {
		t.Fatal("mathematical zero admitted as NaN")
	}
	floatZeroPair, paired := fixture.algebra.Pair(floatZero, zero)
	if !paired {
		t.Fatal("integral float literal lost its zero anchor")
	}
	zeroRelation, ok := fixture.algebra.AdmitAt(key, nil, nil, nil, []Pair{floatZeroPair}, nil)
	if !ok || !zeroRelation.MustEqual(floatZeroPair) || !zeroRelation.MustIntegralEqual(floatZeroPair) {
		t.Fatal("integer/float zero equality was not exact")
	}
	wholeZero, ok := fixture.algebra.Pair(whole, zero)
	if !ok {
		t.Fatal("literal anchor template")
	}
	if _, ok := fixture.algebra.AdmitAt(key, nil, nil, nil, nil, map[Pair]int64{wholeZero: 0}); ok {
		t.Fatal("false finite literal bound admitted")
	}
}

func TestNumericNegativeCycleProofSurvivesInt64Underflow(t *testing.T) {
	graph := component{atoms: []int{0, 1}}
	if !hasNegativeCycleExact(graph, []edge{{from: 0, to: 1, weight: math.MinInt64}, {from: 1, to: 0, weight: -1}}) {
		t.Fatal("negative cycle disappeared at the int64 boundary")
	}
	if hasNegativeCycleExact(graph, []edge{{from: 0, to: 1, weight: math.MinInt64}}) {
		t.Fatal("acyclic underflow path reported a negative cycle")
	}
}

func TestNumericSparseClosureSelectsOnlyActiveSources(t *testing.T) {
	if roots := activeBoundRoots(nil, nil); len(roots) != 0 {
		t.Fatalf("zero-edge image selected %d closure roots", len(roots))
	}
	edges := []edge{{from: 2, to: 8, weight: 1}, {from: 2, to: 9, weight: 0}}
	roots := activeBoundRoots(make([]int, 0, 2), edges)
	if len(roots) != 1 || roots[0] != 2 {
		t.Fatalf("one-source image selected roots %v", roots)
	}
}

func BenchmarkNumericSparseNormalization(b *testing.B) {
	fixture := loopFixture(b)
	key, atoms := loopOperands(b, fixture)
	pair, ok := fixture.algebra.Pair(atoms[0], atoms[1])
	if !ok {
		b.Fatal("missing loop pair")
	}
	zeroMasks := map[Atom]Eligibility{atoms[0]: MayInteger}
	oneMasks := map[Atom]Eligibility{atoms[0]: MayInteger, atoms[1]: MayInteger}
	oneBounds := map[Pair]int64{pair: 0}
	b.Run("zero_edge", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if _, ok := fixture.algebra.AdmitAt(key, zeroMasks, nil, nil, nil, nil); !ok {
				b.Fatal("zero-edge admission")
			}
		}
	})
	b.Run("one_edge", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if _, ok := fixture.algebra.AdmitAt(key, oneMasks, nil, nil, nil, oneBounds); !ok {
				b.Fatal("one-edge admission")
			}
		}
	})
}

func TestNumericLatticeAndKeySpecificRank(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	pair, _ := fixture.algebra.Pair(atoms[0], atoms[1])
	left, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayInteger, atoms[1]: MayInteger},
		[]Pair{pair}, nil, []Pair{pair}, map[Pair]int64{pair: 0})
	if !ok {
		t.Fatal("left relation")
	}
	right, ok := fixture.algebra.AdmitAt(key,
		map[Atom]Eligibility{atoms[0]: MayInteger | MayFiniteFloat, atoms[1]: MayInteger},
		nil, nil, nil, nil)
	if !ok {
		t.Fatal("right relation")
	}
	domain := fixture.algebra.Lattice()
	latticelaws.LawSuite[Value]{
		Name:   "numeric/equality",
		Domain: domain,
		Sample: []Value{
			fixture.algebra.Bottom(), fixture.algebra.Default(), left, right,
			fixture.algebra.Join(left, right), fixture.algebra.Meet(left, right),
		},
	}.Run(t)
	rank, ok := fixture.algebra.WidenRank(key)
	widened := fixture.algebra.Widen(left, right)
	if !ok || rank.Width() != fixture.algebra.AtomCount(key)+4*fixture.algebra.PairCount(key) {
		t.Fatal("key-specific rank shape")
	}
	foundDescent := false
	for component := 0; component < rank.Width(); component++ {
		before, beforeOK := rank.At(left, component)
		after, afterOK := rank.At(widened, component)
		if !beforeOK || !afterOK {
			t.Fatal("rank rejected admitted relation")
		}
		if before != after {
			if after >= before {
				t.Fatalf("rank[%d] = %d -> %d", component, before, after)
			}
			foundDescent = true
			break
		}
	}
	if !foundDescent {
		t.Fatal("strict widening had no descent witness")
	}
}

func TestNumericDefaultAndHotImagesStaySparse(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	value, ok := fixture.algebra.AdmitAt(key, map[Atom]Eligibility{atoms[0]: MayInteger}, nil, nil, nil, nil)
	if !ok || len(value.masks) != 1 || len(value.equal) > 1 || len(value.unequal) != 0 || len(value.integral) > 1 || len(value.bounds) > 1 ||
		len(value.masks)+len(value.equal)+len(value.integral)+len(value.bounds) >= len(fixture.algebra.atoms) {
		t.Fatalf("one fact expanded: ok=%t masks=%d equal=%d unequal=%d integral=%d bounds=%d atoms=%d",
			ok, len(value.masks), len(value.equal), len(value.unequal), len(value.integral), len(value.bounds), len(fixture.algebra.atoms))
	}
	if got := testing.AllocsPerRun(100, func() {
		if joined := fixture.algebra.Join(fixture.algebra.Default(), value); !joined.IsDefault() {
			t.Fatal("Default Join law")
		}
	}); got != 0 {
		t.Fatalf("Default Join allocated %.1f times", got)
	}
}

func TestNumericReplayRebindAndContentFence(t *testing.T) {
	fixture := loopFixture(t)
	data, err := link.EncodeArtifact(fixture.source)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := link.DecodeArtifact(data, fixture.contract, map[keyspace.ContentID]*program.Program{
		fixture.program.ContentID(): fixture.program,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, ok := New(replayed)
	if !ok || !fixture.algebra.Equivalent(other) || fixture.algebra.ContentID() != other.ContentID() {
		t.Fatal("byte replay changed the Numeric schema")
	}
	rebound, ok := other.Rebind(fixture.algebra.Default())
	if !ok || !other.Equal(rebound, other.Default()) {
		t.Fatal("equivalent replay did not rebind the relation")
	}
	different := newNumericFixture(t, "different_numeric", `return 9`)
	if different.algebra.Equivalent(fixture.algebra) {
		t.Fatal("different Link content passed exact replay validation")
	}
}
