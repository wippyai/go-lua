package numeric

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func boundaryAtoms(t testing.TB, fixture numericFixture) (Key, [3]Atom) {
	t.Helper()
	for keyIndex := 0; keyIndex < fixture.algebra.KeyCount(); keyIndex++ {
		key, ok := fixture.algebra.KeyAt(keyIndex)
		if !ok {
			continue
		}
		var selected [3]Atom
		count := 0
		for atomIndex := 0; atomIndex < fixture.algebra.AtomCount(key); atomIndex++ {
			atom, ok := fixture.algebra.AtomAt(key, atomIndex)
			if !ok {
				continue
			}
			scalar, ordinary := atom.Scalar()
			if !ordinary {
				continue
			}
			if scalarLiteral(fixture, scalar) {
				continue
			}
			selected[count] = atom
			count++
			if count == len(selected) {
				return key, selected
			}
		}
	}
	t.Fatal("numeric fixture lacks three key-local nonliteral atoms")
	return Key{}, [3]Atom{}
}

func literalAtom(t testing.TB, fixture numericFixture) (Key, Atom) {
	t.Helper()
	for keyIndex := 0; keyIndex < fixture.algebra.KeyCount(); keyIndex++ {
		key, ok := fixture.algebra.KeyAt(keyIndex)
		if !ok {
			continue
		}
		for atomIndex := 0; atomIndex < fixture.algebra.AtomCount(key); atomIndex++ {
			atom, ok := fixture.algebra.AtomAt(key, atomIndex)
			if !ok {
				continue
			}
			scalar, ordinary := atom.Scalar()
			if !ordinary {
				continue
			}
			if scalarLiteral(fixture, scalar) {
				return key, atom
			}
		}
	}
	t.Fatal("numeric fixture lacks a literal atom")
	return Key{}, Atom{}
}

func scalarLiteral(fixture numericFixture, scalar Scalar) bool {
	p, ok := fixture.source.Project().Mounts().Program(scalar.Shard())
	if !ok || p == nil {
		return false
	}
	term := scalar.Term()
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return false
	}
	literals := p.Source().Literals()
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyInteger:
		returned, owner, _, present := literals.Integers().At(int(ordinal - 1))
		return present && returned == term && owner == scalar.Body()
	case keyspace.FamilyFloat:
		returned, owner, _, present := literals.Floats().At(int(ordinal - 1))
		return present && returned == term && owner == scalar.Body()
	default:
		return false
	}
}

func TestEligibilityContributionAndBoundarySubstitutionAreExact(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := boundaryAtoms(t, fixture)
	contribution, ok := fixture.algebra.EligibilityAt(key, atoms[0], MayInteger)
	if !ok {
		t.Fatal("exact eligibility contribution rejected")
	}
	if mask, ok := contribution.Eligibility(atoms[0]); !ok || mask != MayInteger {
		t.Fatalf("eligibility contribution = %v/%v, want exact integer", mask, ok)
	}
	identity, ok := IdentitySubstitution(fixture.algebra)
	if !ok {
		t.Fatal("identity substitution")
	}
	unchanged, ok := fixture.algebra.Substitute(Fact{Key: key, Value: contribution}, identity)
	if !ok || unchanged.Key != key || !fixture.algebra.Equal(unchanged.Value, contribution) {
		t.Fatal("identity substitution changed exact Numeric fact")
	}

	first, ok := NewSubstitution(fixture.algebra, [][2]Key{{key, key}}, [][2]Atom{{atoms[0], atoms[1]}})
	if !ok {
		t.Fatal("first exact substitution")
	}
	second, ok := NewSubstitution(fixture.algebra, nil, [][2]Atom{{atoms[1], atoms[2]}})
	if !ok {
		t.Fatal("second exact substitution")
	}
	composed, ok := first.Compose(second)
	if !ok {
		t.Fatal("substitution composition")
	}
	image, ok := fixture.algebra.Substitute(Fact{Key: key, Value: contribution}, composed)
	if !ok || image.Key != key {
		t.Fatal("composed substitution rejected exact fact")
	}
	if mask, ok := image.Value.Eligibility(atoms[2]); !ok || mask != MayInteger {
		t.Fatalf("composed atom image = %v/%v, want exact integer", mask, ok)
	}
	if mask, ok := image.Value.Eligibility(atoms[0]); !ok || mask != allEligibility {
		t.Fatalf("source atom survived substitution = %v/%v", mask, ok)
	}

	literalKey, literal := literalAtom(t, fixture)
	if _, ok := fixture.algebra.EligibilityAt(literalKey, literal, MayFiniteFloat); ok {
		t.Fatal("eligibility contribution clipped an impossible sealed literal kind")
	}
}

func TestBoundarySubstitutionCollisionUsesCarrierJoin(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := boundaryAtoms(t, fixture)
	source, ok := fixture.algebra.AdmitAt(key, map[Atom]Eligibility{
		atoms[0]: MayInteger,
		atoms[1]: MayFiniteFloat,
	}, nil, nil, nil, nil)
	if !ok {
		t.Fatal("source eligibility relation")
	}
	substitution, ok := NewSubstitution(fixture.algebra, nil, [][2]Atom{{atoms[0], atoms[2]}, {atoms[1], atoms[2]}})
	if !ok {
		t.Fatal("collapsing substitution")
	}
	image, ok := fixture.algebra.Substitute(Fact{Key: key, Value: source}, substitution)
	if !ok {
		t.Fatal("collapsing substitution rejected")
	}
	if mask, ok := image.Value.Eligibility(atoms[2]); !ok || mask != MayInteger|MayFiniteFloat {
		t.Fatalf("colliding eligibility = %v/%v, want carrier join", mask, ok)
	}
	if !fixture.algebra.Admits(image.Key, image.Value) {
		t.Fatal("colliding substitution escaped destination key support")
	}
	rank, ok := fixture.algebra.WidenRank(image.Key)
	if !ok || rank.Width() == 0 {
		t.Fatal("substitution image lost finite rank")
	}
	widened := fixture.algebra.Widen(image.Value, fixture.algebra.Default())
	if !fixture.algebra.Equal(widened, fixture.algebra.Join(image.Value, fixture.algebra.Default())) || !fixture.algebra.LessOrEq(image.Value, widened) {
		t.Fatal("substitution image escaped finite join/widen termination law")
	}
}

func TestBoundarySubstitutionRejectsForeignOwnerCoordinates(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := boundaryAtoms(t, fixture)
	foreign := loopFixture(t)
	foreignKey, foreignAtoms := boundaryAtoms(t, foreign)
	if _, ok := NewSubstitution(fixture.algebra, [][2]Key{{key, foreignKey}}, nil); ok {
		t.Fatal("foreign key entered Numeric substitution")
	}
	if _, ok := NewSubstitution(fixture.algebra, nil, [][2]Atom{{atoms[0], foreignAtoms[0]}}); ok {
		t.Fatal("foreign atom entered Numeric substitution")
	}
	foreignSubstitution, ok := IdentitySubstitution(foreign.algebra)
	if !ok {
		t.Fatal("foreign identity substitution")
	}
	value, ok := fixture.algebra.EligibilityAt(key, atoms[0], MayInteger)
	if !ok {
		t.Fatal("local eligibility")
	}
	if _, ok := fixture.algebra.Substitute(Fact{Key: key, Value: value}, foreignSubstitution); ok {
		t.Fatal("foreign substitution crossed Numeric owner fence")
	}
	if _, ok := fixture.algebra.EligibilityAt(foreignKey, foreignAtoms[0], MayInteger); ok {
		t.Fatal("foreign key/atom accepted at local Numeric boundary")
	}
}

func wideBoundaryFixture(t testing.TB) (numericFixture, Key, []Atom) {
	t.Helper()
	var source strings.Builder
	for index := 0; index < 96; index++ {
		source.WriteString("local v")
		source.WriteString(string(rune('a' + index%26)))
		source.WriteString("_")
		source.WriteString(string(rune('a' + (index/26)%26)))
		source.WriteString(" = ")
		source.WriteString("1\n")
	}
	source.WriteString("return ")
	for index := 0; index < 96; index++ {
		if index != 0 {
			source.WriteString(" + ")
		}
		source.WriteString("v")
		source.WriteString(string(rune('a' + index%26)))
		source.WriteString("_")
		source.WriteString(string(rune('a' + (index/26)%26)))
	}
	fixture := newNumericFixture(t, "numeric_substitution_wide", source.String())
	var key Key
	for index := 0; index < fixture.algebra.KeyCount(); index++ {
		candidate, ok := fixture.algebra.KeyAt(index)
		if ok && fixture.algebra.AtomCount(candidate) > fixture.algebra.AtomCount(key) {
			key = candidate
		}
	}
	if !key.Valid() || fixture.algebra.AtomCount(key) < 65 {
		t.Fatalf("wide Numeric support = %d, want more than a fixed small cap", fixture.algebra.AtomCount(key))
	}
	atoms := make([]Atom, 0, fixture.algebra.AtomCount(key))
	for index := 0; index < fixture.algebra.AtomCount(key); index++ {
		atom, ok := fixture.algebra.AtomAt(key, index)
		if !ok {
			t.Fatal("wide Numeric atom")
		}
		atoms = append(atoms, atom)
	}
	return fixture, key, atoms
}

func TestBoundarySubstitutionLookupIsAllocationFreeAndUncapped(t *testing.T) {
	fixture, key, atoms := wideBoundaryFixture(t)
	pairs := make([][2]Atom, 0, len(atoms))
	for _, atom := range atoms {
		pairs = append(pairs, [2]Atom{atom, atom})
	}
	substitution, ok := NewSubstitution(fixture.algebra, [][2]Key{{key, key}}, pairs)
	if !ok {
		t.Fatal("wide identity substitution")
	}
	atom := atoms[len(atoms)-1]
	if allocations := testing.AllocsPerRun(1_000, func() {
		if _, ok := substitution.Key(key); !ok {
			panic("key lookup")
		}
		if _, ok := substitution.Atom(atom); !ok {
			panic("atom lookup")
		}
	}); allocations != 0 {
		t.Fatalf("sealed substitution lookup allocated %.2f times", allocations)
	}
}

func TestBoundarySubstitutionCompilesOnceAndApplicationAllocationDoesNotScaleWithWidth(t *testing.T) {
	fixture, key, support := wideBoundaryFixture(t)
	atoms := make([]Atom, 0, len(support))
	for _, atom := range support {
		if fixture.algebra.baseEligibility(int(atom.slot-1)) == allEligibility {
			atoms = append(atoms, atom)
		}
	}
	if len(atoms) < 65 {
		t.Fatalf("wide substitutable Numeric support = %d, want at least 65", len(atoms))
	}

	narrowPairs := [][2]Atom{{atoms[0], atoms[1]}, {atoms[1], atoms[0]}}
	widePairs := make([][2]Atom, len(atoms))
	for index, atom := range atoms {
		widePairs[index] = [2]Atom{atom, atoms[(index+1)%len(atoms)]}
	}
	var built Substitution
	narrowBuildAllocs := testing.AllocsPerRun(100, func() {
		var accepted bool
		built, accepted = NewSubstitution(fixture.algebra, nil, narrowPairs)
		if !accepted {
			panic("narrow compiled substitution")
		}
	})
	wideBuildAllocs := testing.AllocsPerRun(100, func() {
		var accepted bool
		built, accepted = NewSubstitution(fixture.algebra, nil, widePairs)
		if !accepted {
			panic("wide compiled substitution")
		}
	})
	if !built.valid() {
		t.Fatal("compiled substitution sink is invalid")
	}
	if narrowBuildAllocs != wideBuildAllocs {
		t.Fatalf("compiled image allocations scale with bindings: narrow %.2f, wide %.2f", narrowBuildAllocs, wideBuildAllocs)
	}

	wideSubstitution, ok := NewSubstitution(fixture.algebra, nil, widePairs)
	if !ok {
		t.Fatal("wide compiled substitution")
	}
	narrowMasks := make(map[Atom]Eligibility, 32)
	for index, atom := range atoms[:32] {
		if index%2 == 0 {
			narrowMasks[atom] = MayInteger
		} else {
			narrowMasks[atom] = MayFiniteFloat
		}
	}
	narrowValue, ok := fixture.algebra.AdmitAt(key, narrowMasks, nil, nil, nil, nil)
	if !ok {
		t.Fatal("narrow substitution value")
	}
	wideMasks := make(map[Atom]Eligibility, len(atoms))
	for index, atom := range atoms {
		if index%2 == 0 {
			wideMasks[atom] = MayInteger
		} else {
			wideMasks[atom] = MayFiniteFloat
		}
	}
	wideValue, ok := fixture.algebra.AdmitAt(key, wideMasks, nil, nil, nil, nil)
	if !ok {
		t.Fatal("wide substitution value")
	}
	var image Fact
	narrowApplyAllocs := testing.AllocsPerRun(250, func() {
		var accepted bool
		image, accepted = fixture.algebra.Substitute(Fact{Key: key, Value: narrowValue}, wideSubstitution)
		if !accepted {
			panic("narrow substitution application")
		}
	})
	wideApplyAllocs := testing.AllocsPerRun(250, func() {
		var accepted bool
		image, accepted = fixture.algebra.Substitute(Fact{Key: key, Value: wideValue}, wideSubstitution)
		if !accepted {
			panic("wide substitution application")
		}
	})
	if !fixture.algebra.Admits(image.Key, image.Value) {
		t.Fatal("wide compiled substitution image escaped destination support")
	}
	if narrowApplyAllocs != wideApplyAllocs {
		t.Fatalf("repeated application allocations scale with facts: narrow %.2f, wide %.2f", narrowApplyAllocs, wideApplyAllocs)
	}
	t.Logf("compiled image allocations %.0f; repeated application allocations %.0f", wideBuildAllocs, wideApplyAllocs)
}
