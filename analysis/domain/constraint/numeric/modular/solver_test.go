package modular

import "testing"

func TestModularSolver_FilterEvenOdd(t *testing.T) {
	solver := NewModularSolver()

	// Array of 10 elements, indices 0-9
	solver.AddRange("idx", 0, 9)

	evenCount := solver.CountInRange("idx", 2, 0) // 0,2,4,6,8
	oddCount := solver.CountInRange("idx", 2, 1)  // 1,3,5,7,9

	if evenCount != 5 {
		t.Errorf("expected 5 even indices, got %d", evenCount)
	}

	if oddCount != 5 {
		t.Errorf("expected 5 odd indices, got %d", oddCount)
	}
}

func TestModularSolver_DivisibleBy3(t *testing.T) {
	solver := NewModularSolver()
	solver.AddRange("x", 1, 100)

	div3 := solver.CountInRange("x", 3, 0) // 3,6,9,...,99

	// From 1 to 100: 3,6,9,...,99 = 33 numbers
	if div3 != 33 {
		t.Errorf("expected 33 numbers divisible by 3 in [1,100], got %d", div3)
	}
}

func TestModularSolver_ComplexPredicate(t *testing.T) {
	// x % 6 == 0 means divisible by both 2 and 3
	solver := NewModularSolver()
	solver.AddRange("x", 1, 30)

	div6 := solver.CountInRange("x", 6, 0) // 6,12,18,24,30

	if div6 != 5 {
		t.Errorf("expected 5 numbers divisible by 6 in [1,30], got %d", div6)
	}
}
