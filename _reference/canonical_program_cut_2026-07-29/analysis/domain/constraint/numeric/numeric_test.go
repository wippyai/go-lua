package numeric

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

func TestLe(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")
	c := Le{X: x, Y: y, C: 5}

	if c.NumKind() != NumLe {
		t.Errorf("expected NumLe, got %v", c.NumKind())
	}

	keys := c.Keys()
	if len(keys) != 2 || keys[0] != x || keys[1] != y {
		t.Error("unexpected keys")
	}

	if c.Hash() == 0 {
		t.Error("hash should be non-zero")
	}

	c2 := Le{X: x, Y: y, C: 5}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := Le{X: x, Y: y, C: 10}
	if c.Equals(c3) {
		t.Error("different C should not be equal")
	}
}

func TestLt(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")
	c := Lt{X: x, Y: y}

	if c.NumKind() != NumLt {
		t.Errorf("expected NumLt, got %v", c.NumKind())
	}

	if len(c.Keys()) != 2 {
		t.Error("expected 2 keys")
	}

	c2 := Lt{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := Lt{X: y, Y: x}
	if c.Equals(c3) {
		t.Error("swapped keys should not be equal")
	}
}

func TestGe(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")
	c := Ge{X: x, Y: y}

	if c.NumKind() != NumGe {
		t.Errorf("expected NumGe, got %v", c.NumKind())
	}

	c2 := Ge{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestGt(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")
	c := Gt{X: x, Y: y}

	if c.NumKind() != NumGt {
		t.Errorf("expected NumGt, got %v", c.NumKind())
	}

	c2 := Gt{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestEq(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")
	c := Eq{X: x, Y: y}

	if c.NumKind() != NumEq {
		t.Errorf("expected NumEq, got %v", c.NumKind())
	}

	c2 := Eq{X: x, Y: y}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestEqConst(t *testing.T) {
	x := pathdom.PathKey("x")
	c := EqConst{X: x, C: 42}

	if c.NumKind() != NumEqConst {
		t.Errorf("expected NumEqConst, got %v", c.NumKind())
	}

	if len(c.Keys()) != 1 {
		t.Error("expected 1 key")
	}

	c2 := EqConst{X: x, C: 42}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := EqConst{X: x, C: 100}
	if c.Equals(c3) {
		t.Error("different C should not be equal")
	}
}

func TestLeConst(t *testing.T) {
	x := pathdom.PathKey("x")
	c := LeConst{X: x, C: 10}

	if c.NumKind() != NumLeConst {
		t.Errorf("expected NumLeConst, got %v", c.NumKind())
	}

	c2 := LeConst{X: x, C: 10}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestGeConst(t *testing.T) {
	x := pathdom.PathKey("x")
	c := GeConst{X: x, C: 0}

	if c.NumKind() != NumGeConst {
		t.Errorf("expected NumGeConst, got %v", c.NumKind())
	}

	c2 := GeConst{X: x, C: 0}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}
}

func TestModEq(t *testing.T) {
	x := pathdom.PathKey("x")
	c := ModEq{X: x, M: 3, R: 1}

	if c.NumKind() != NumModEq {
		t.Errorf("expected NumModEq, got %v", c.NumKind())
	}

	if len(c.Keys()) != 1 {
		t.Error("expected 1 key")
	}

	c2 := ModEq{X: x, M: 3, R: 1}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := ModEq{X: x, M: 3, R: 2}
	if c.Equals(c3) {
		t.Error("different R should not be equal")
	}

	c4 := ModEq{X: x, M: 5, R: 1}
	if c.Equals(c4) {
		t.Error("different M should not be equal")
	}
}

func TestSumLe(t *testing.T) {
	i := pathdom.PathKey("i")
	j := pathdom.PathKey("j")
	n := pathdom.PathKey("n")

	c := NewSumLe(i, j, n, 0)

	if c.NumKind() != NumSumLe {
		t.Errorf("expected NumSumLe, got %v", c.NumKind())
	}

	keys := c.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[2] != n {
		t.Errorf("third key should be the negative operand z, got %v", keys[2])
	}

	if c.Hash() == 0 {
		t.Error("hash should be non-zero")
	}

	// Commutative positive operands canonicalize: i+j and j+i are equal.
	if !c.Equals(NewSumLe(j, i, n, 0)) {
		t.Error("SumLe(i,j,n) should equal canonicalized SumLe(j,i,n)")
	}
	if c.Hash() != NewSumLe(j, i, n, 0).Hash() {
		t.Error("canonicalized commutative sums should hash equally")
	}

	if c.Equals(NewSumLe(i, j, n, 1)) {
		t.Error("different C should not be equal")
	}
	if c.Equals(NewSumLe(i, j, pathdom.PathKey("other"), 0)) {
		t.Error("different Z should not be equal")
	}
	if c.Equals(Le{X: i, Y: j, C: 0}) {
		t.Error("SumLe should not equal Le")
	}
}

func TestScaledLe(t *testing.T) {
	i := pathdom.PathKey("i")
	j := pathdom.PathKey("j")
	n := pathdom.PathKey("n")

	c := NewScaledLe(2, i, 3, j, n, 0)

	if c.NumKind() != NumSumLe {
		t.Errorf("expected NumSumLe, got %v", c.NumKind())
	}

	keys := c.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[2] != n {
		t.Errorf("third key should be the negative operand z, got %v", keys[2])
	}

	if c.Hash() == 0 {
		t.Error("hash should be non-zero")
	}

	// Commutative scaled operands canonicalize: 2i+3j equals 3j+2i.
	if !c.Equals(NewScaledLe(3, j, 2, i, n, 0)) {
		t.Error("ScaledLe(2,i,3,j,n) should equal canonicalized ScaledLe(3,j,2,i,n)")
	}
	if c.Hash() != NewScaledLe(3, j, 2, i, n, 0).Hash() {
		t.Error("canonicalized commutative scaled sums should hash equally")
	}

	// Coefficients are part of identity.
	if c.Equals(NewScaledLe(2, i, 2, j, n, 0)) {
		t.Error("different CoY should not be equal")
	}
	if c.Equals(NewScaledLe(1, i, 3, j, n, 0)) {
		t.Error("different CoX should not be equal")
	}

	// A unit NewSumLe is the CoX=CoY=1 special case.
	unit := NewSumLe(i, j, n, 0)
	if unit.CoX != 1 || unit.CoY != 1 {
		t.Errorf("NewSumLe should default coefficients to 1, got CoX=%d CoY=%d", unit.CoX, unit.CoY)
	}
}

func TestScaledLeAbsentSecondOperand(t *testing.T) {
	i := pathdom.PathKey("i")
	n := pathdom.PathKey("n")

	c := NewScaledLe(2, i, 0, "", n, 0)
	keys := c.Keys()
	if len(keys) != 2 || keys[0] != i || keys[1] != n {
		t.Fatalf("absent second operand should yield 2 keys, got %#v", keys)
	}
	if !c.Equals(NewScaledLe(2, i, 0, "", n, 0)) {
		t.Error("equal scaled constraints should be equal")
	}
}

func TestNumericConstraintHashUniqueness(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")

	constraints := []NumericConstraint{
		Le{X: x, Y: y, C: 5},
		Lt{X: x, Y: y},
		Ge{X: x, Y: y},
		Gt{X: x, Y: y},
		Eq{X: x, Y: y},
		EqConst{X: x, C: 5},
		LeConst{X: x, C: 5},
		GeConst{X: x, C: 5},
		ModEq{X: x, M: 3, R: 1},
	}

	hashes := make(map[uint64]int)

	for i, c := range constraints {
		h := c.Hash()
		if prev, exists := hashes[h]; exists {
			t.Errorf("hash collision between constraint %d and %d", prev, i)
		}

		hashes[h] = i
	}
}

func TestNumericConstraintEqualsWrongType(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")

	le := Le{X: x, Y: y, C: 5}
	lt := Lt{X: x, Y: y}

	if le.Equals(lt) {
		t.Error("Le should not equal Lt")
	}

	if lt.Equals(le) {
		t.Error("Lt should not equal Le")
	}
}

func TestNumKindValues(t *testing.T) {
	if NumInvalid != 0 {
		t.Error("NumInvalid should be 0")
	}

	kinds := []NumKind{NumLe, NumLt, NumGe, NumGt, NumEq, NumEqConst, NumLeConst, NumGeConst, NumModEq, NumLeLenOf, NumLenLeConst, NumLenGeConst, NumSumLe}
	seen := make(map[NumKind]bool)

	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate NumKind value: %d", k)
		}

		seen[k] = true

		if k == NumInvalid {
			t.Errorf("kind should not be NumInvalid")
		}
	}
}

func TestLeLenOf(t *testing.T) {
	x := pathdom.PathKey("x")
	arr := pathdom.PathKey("arr")
	c := LeLenOf{X: x, Array: arr}

	if c.NumKind() != NumLeLenOf {
		t.Errorf("expected NumLeLenOf, got %v", c.NumKind())
	}

	keys := c.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != x || keys[1] != arr {
		t.Error("unexpected keys")
	}

	if c.Hash() == 0 {
		t.Error("hash should be non-zero")
	}

	c2 := LeLenOf{X: x, Array: arr}
	if !c.Equals(c2) {
		t.Error("equal constraints should be equal")
	}

	c3 := LeLenOf{X: x, Array: pathdom.PathKey("other")}
	if c.Equals(c3) {
		t.Error("different array should not be equal")
	}

	c4 := LeLenOf{X: pathdom.PathKey("other"), Array: arr}
	if c.Equals(c4) {
		t.Error("different X should not be equal")
	}
}

func TestLenConstBounds(t *testing.T) {
	arr := pathdom.PathKey("arr")
	le := LenLeConst{Array: arr, C: 3}
	if le.NumKind() != NumLenLeConst {
		t.Errorf("expected NumLenLeConst, got %v", le.NumKind())
	}
	if keys := le.Keys(); len(keys) != 1 || keys[0] != arr {
		t.Fatalf("unexpected LenLeConst keys: %#v", keys)
	}
	if le.Hash() == 0 {
		t.Fatal("LenLeConst hash should be non-zero")
	}
	if !le.Equals(LenLeConst{Array: arr, C: 3}) {
		t.Fatal("equal LenLeConst constraints should be equal")
	}
	if le.Equals(LenLeConst{Array: arr, C: 4}) {
		t.Fatal("different LenLeConst constants should not be equal")
	}

	ge := LenGeConst{Array: arr, C: 1}
	if ge.NumKind() != NumLenGeConst {
		t.Errorf("expected NumLenGeConst, got %v", ge.NumKind())
	}
	if keys := ge.Keys(); len(keys) != 1 || keys[0] != arr {
		t.Fatalf("unexpected LenGeConst keys: %#v", keys)
	}
	if ge.Hash() == 0 {
		t.Fatal("LenGeConst hash should be non-zero")
	}
	if !ge.Equals(LenGeConst{Array: arr, C: 1}) {
		t.Fatal("equal LenGeConst constraints should be equal")
	}
	if ge.Equals(LenGeConst{Array: pathdom.PathKey("other"), C: 1}) {
		t.Fatal("different LenGeConst arrays should not be equal")
	}
}

func TestNumericKeysMethod(t *testing.T) {
	x := pathdom.PathKey("x")
	y := pathdom.PathKey("y")

	tests := []struct {
		name    string
		c       NumericConstraint
		wantLen int
	}{
		{"Le", Le{X: x, Y: y, C: 5}, 2},
		{"Lt", Lt{X: x, Y: y}, 2},
		{"Ge", Ge{X: x, Y: y}, 2},
		{"Gt", Gt{X: x, Y: y}, 2},
		{"Eq", Eq{X: x, Y: y}, 2},
		{"EqConst", EqConst{X: x, C: 42}, 1},
		{"LeConst", LeConst{X: x, C: 10}, 1},
		{"GeConst", GeConst{X: x, C: 0}, 1},
		{"ModEq", ModEq{X: x, M: 3, R: 1}, 1},
		{"LeLenOf", LeLenOf{X: x, Array: y}, 2},
		{"LenLeConst", LenLeConst{Array: y, C: 3}, 1},
		{"LenGeConst", LenGeConst{Array: y, C: 1}, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys := tc.c.Keys()
			if len(keys) != tc.wantLen {
				t.Errorf("Keys() returned %d keys, want %d", len(keys), tc.wantLen)
			}
		})
	}
}
