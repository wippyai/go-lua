package numeric

import "testing"

func TestMustRawRelationsRemainExactAndOwnerFenced(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	equal, ok := fixture.algebra.MustRawEqual(key, atoms[0], atoms[1])
	if !ok {
		t.Fatal("sealed raw equality rejected")
	}
	pair, _ := fixture.algebra.Pair(atoms[0], atoms[1])
	if !equal.MustEqual(pair) || equal.MustIntegralEqual(pair) {
		t.Fatal("raw equality changed its numeric meaning")
	}
	unequal, ok := fixture.algebra.MustRawUnequal(key, atoms[0], atoms[1])
	if !ok || !unequal.MustUnequal(pair) {
		t.Fatal("sealed raw disequality rejected")
	}
	foreign := loopFixture(t)
	foreignKey, foreignAtoms := loopOperands(t, foreign)
	if _, ok := fixture.algebra.MustRawEqual(foreignKey, foreignAtoms[0], foreignAtoms[1]); ok {
		t.Fatal("foreign numeric coordinates crossed the owner fence")
	}
}

func TestMustIntegralEqualityDoesNotEraseRepresentation(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	value, ok := fixture.algebra.MustIntegralEqual(key, atoms[0], atoms[1])
	if !ok {
		t.Fatal("sealed integral equality rejected")
	}
	pair, _ := fixture.algebra.Pair(atoms[0], atoms[1])
	if !value.MustEqual(pair) || !value.MustIntegralEqual(pair) {
		t.Fatal("integral equality did not retain both required relations")
	}
	left, _ := value.Eligibility(atoms[0])
	right, _ := value.Eligibility(atoms[1])
	if left != numericIntegralEligibility || right != numericIntegralEligibility {
		t.Fatalf("integral equality representation masks = %v/%v", left, right)
	}
}

func TestIntegerDifferenceAndTranslationUseOnlySealedThresholds(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	difference, ok := fixture.algebra.IntegerDifference(key, atoms[0], atoms[1], -1)
	if !ok {
		t.Fatal("sealed integer difference rejected")
	}
	forward, _ := fixture.algebra.Pair(atoms[0], atoms[1])
	if bound, infinite, found := difference.Bound(forward); !found || infinite || bound != -1 {
		t.Fatalf("integer difference = %d/%t/%t", bound, infinite, found)
	}
	translation, ok := fixture.algebra.IntegerTranslation(key, atoms[0], atoms[1], 1)
	if !ok {
		t.Fatal("sealed integer translation rejected")
	}
	reverse, _ := fixture.algebra.Pair(atoms[1], atoms[0])
	if bound, infinite, found := translation.Bound(forward); !found || infinite || bound != 1 {
		t.Fatalf("translation forward = %d/%t/%t", bound, infinite, found)
	}
	if bound, infinite, found := translation.Bound(reverse); !found || infinite || bound != -1 {
		t.Fatalf("translation reverse = %d/%t/%t", bound, infinite, found)
	}
	if _, ok := fixture.algebra.IntegerTranslation(key, atoms[0], atoms[1], -9223372036854775808); ok {
		t.Fatal("MinInt64 translation overflow admitted")
	}
	if _, ok := fixture.algebra.IntegerDifference(key, atoms[0], atoms[1], 7); ok {
		t.Fatal("unsealed threshold rounded into the relation")
	}
}

func TestTransferKernelsRespectFiniteWideningLaw(t *testing.T) {
	fixture := loopFixture(t)
	key, atoms := loopOperands(t, fixture)
	left, ok := fixture.algebra.IntegerDifference(key, atoms[0], atoms[1], -1)
	if !ok {
		t.Fatal("left transfer")
	}
	right, ok := fixture.algebra.IntegerTranslation(key, atoms[0], atoms[1], 1)
	if !ok {
		t.Fatal("right transfer")
	}
	join := fixture.algebra.Join(left, right)
	widen := fixture.algebra.Widen(left, right)
	if !fixture.algebra.Equal(join, widen) || !fixture.algebra.LessOrEq(left, widen) || !fixture.algebra.LessOrEq(right, widen) {
		t.Fatal("transfer images violated the finite carrier widening law")
	}
}
